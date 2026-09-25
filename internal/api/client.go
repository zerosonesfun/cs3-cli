package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const maxErrorBody = 280
const maxPostLength = 10000
const maxCommentLength = 3000
const maxLinksPerPost = 3

// Approximate server TextService::countLinks: markdown links + bare http(s)/www URLs.
var mdLinkPattern = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)\s]+)\)`)
var bareURLPattern = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s]+`)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	UserAgent  string
}

func New(baseURL, token string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	parsed, _ := url.Parse(baseURL)
	c := &Client{
		BaseURL:   baseURL,
		Token:     token,
		UserAgent: "cs3-cli/1.0",
	}
	c.HTTPClient = &http.Client{
		Timeout: 45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			// Never follow off-host redirects with Authorization.
			if parsed == nil || req.URL.Host != parsed.Host || req.URL.Scheme != parsed.Scheme {
				return fmt.Errorf("refusing redirect to %s", req.URL.Redacted())
			}
			return nil
		},
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		},
	}
	return c
}

type User struct {
	Username                 string `json:"username"`
	UITheme                  string `json:"ui_theme"`
	UIFont                   string `json:"ui_font"`
	Timezone                 string `json:"timezone"`
	Bio                      string `json:"bio"`
	PreferredBodyColor       string `json:"preferred_body_color"`
	SoundEnabled             bool   `json:"sound_enabled"`
	AllowProfilePosts        bool   `json:"allow_profile_posts"`
	BlockClickInvites        bool   `json:"block_click_invites"`
	AllowPings               bool   `json:"allow_pings"`
	AllowPostCommentPings    bool   `json:"allow_post_comment_pings"`
	AllowCommentReplyPings   bool   `json:"allow_comment_reply_pings"`
	EmailPingDigest          bool   `json:"email_ping_digest"`
	HideBubbledPosts         bool   `json:"hide_bubbled_posts"`
	FederateGlobal           bool   `json:"federate_global"`
	UsernameChangeCount      int    `json:"username_change_count"`
	UsernameChangesRemaining int    `json:"username_changes_remaining"`
}

type Pet struct {
	Name    *string `json:"name"`
	Named   bool    `json:"named"`
	Mood    string  `json:"mood"`
	Face    string  `json:"face"`
	Speech  *string `json:"speech"`
	CanFeed bool    `json:"can_feed"`
}

type Post struct {
	ID                   string `json:"id"`
	Username             string `json:"username"`
	Body                 string `json:"body"`
	BodyColor            string `json:"body_color"`
	ArtColors            string `json:"art_colors"`
	IsArt                bool   `json:"is_art"`
	IsPoll               bool   `json:"is_poll"`
	Scope                string `json:"scope"`
	ClickSlug            string `json:"click_slug"`
	ClickName            string `json:"click_name"`
	Status               string `json:"status"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
	CommentCount         int    `json:"comment_count"`
	CanEdit              bool   `json:"can_edit"`
	CanDelete            bool   `json:"can_delete"`
	EditRequiresReason   bool   `json:"edit_requires_reason"`
	DeleteRequiresReason bool   `json:"delete_requires_reason"`
	WallQuoteCount       int    `json:"wall_quote_count"`
	IsBubbled            bool   `json:"is_bubbled"`
}

type Click struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsMember    bool   `json:"is_member,omitempty"`
}

type SearchUser struct {
	Username string `json:"username"`
}

type MyPostsPage struct {
	Posts   []Post
	Page    int
	HasMore bool
}

type Comment struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Body           string `json:"body"`
	CreatedAt      string `json:"created_at"`
	Status         string `json:"status"`
	WallQuoteCount int    `json:"wall_quote_count"`
}

type Ping struct {
	ID                  int64  `json:"id"`
	Kind                string `json:"kind"`
	Summary             string `json:"summary"`
	ActorUsername       string `json:"actor_username"`
	CreatedAt           string `json:"created_at"`
	OpenedAt            string `json:"opened_at"`
	PostID              string `json:"post_id"`
	CommentID           string `json:"comment_id"`
	ClickSlug           string `json:"click_slug"`
	RelatedCommentCount int    `json:"related_comment_count"`
}

type PingTarget struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	CommentID string `json:"comment_id"`
	URL       string `json:"url"`
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e == nil {
		return "request failed"
	}
	switch e.Status {
	case 403:
		if e.Message != "" {
			return e.Message
		}
		return "you do not have permission"
	case 404:
		if e.Message != "" {
			return e.Message
		}
		return "not found"
	case 422:
		if e.Message != "" {
			return e.Message
		}
		return "invalid request"
	case 429:
		if e.Message != "" {
			return e.Message
		}
		return "Too many attempts. Please wait."
	case 503:
		if e.Message != "" {
			return e.Message
		}
		return "server busy — try again shortly"
	}
	if e.Message != "" {
		return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

func (e *APIError) Unauthorized() bool {
	return e != nil && e.Status == 401
}

func (c *Client) Login(ctx context.Context, login, password string) (token string, user User, err error) {
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Token string `json:"token"`
		User  User   `json:"user"`
	}
	err = c.doJSON(ctx, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"login":    login,
		"password": password,
		"client":   "cli",
	}, false, &resp)
	if err != nil {
		return "", User{}, err
	}
	if !resp.OK || resp.Token == "" {
		msg := resp.Error
		if msg == "" {
			msg = "login failed"
		}
		return "", User{}, &APIError{Status: 401, Message: msg}
	}
	return resp.Token, resp.User, nil
}

func (c *Client) Logout(ctx context.Context) error {
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	return c.doJSON(ctx, http.MethodPost, "/api/v1/auth/logout", map[string]any{}, true, &resp)
}

func (c *Client) Me(ctx context.Context) (User, error) {
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		User  User   `json:"user"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/me", nil, true, &resp); err != nil {
		return User{}, err
	}
	if !resp.OK {
		return User{}, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.User, nil
}

func (c *Client) Pet(ctx context.Context) (Pet, error) {
	return c.pet(ctx, false)
}

// PetPeek loads companion name/mood without starting a hungry window.
func (c *Client) PetPeek(ctx context.Context) (Pet, error) {
	return c.pet(ctx, true)
}

func (c *Client) pet(ctx context.Context, peek bool) (Pet, error) {
	path := "/api/v1/me/pet"
	if peek {
		path += "?peek=1"
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Pet   Pet    `json:"pet"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return Pet{}, err
	}
	if !resp.OK {
		return Pet{}, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.Pet, nil
}

func (c *Client) NamePet(ctx context.Context, name string) (Pet, error) {
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Pet   Pet    `json:"pet"`
	}
	err := c.doMutateJSON(ctx, http.MethodPost, "/api/v1/me/pet/name", map[string]any{"name": name}, "", &resp)
	if err != nil {
		return Pet{}, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not save pet name"
		}
		return Pet{}, &APIError{Status: 422, Message: msg}
	}
	return resp.Pet, nil
}

func (c *Client) UpdateSettings(ctx context.Context, patch map[string]any) (User, []string, error) {
	var resp struct {
		OK       bool     `json:"ok"`
		Error    string   `json:"error"`
		Warnings []string `json:"warnings"`
		User     User     `json:"user"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/me/settings", patch, true, &resp); err != nil {
		return User{}, nil, err
	}
	if !resp.OK {
		return User{}, nil, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.User, resp.Warnings, nil
}

func (c *Client) UpdateUsername(ctx context.Context, username, password string) (User, error) {
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		User  User   `json:"user"`
	}
	body := map[string]any{
		"username": username,
		"password": password,
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/me/username", body, true, &resp); err != nil {
		return User{}, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not update username"
		}
		return User{}, &APIError{Status: 422, Message: msg}
	}
	return resp.User, nil
}

func (c *Client) Feed(ctx context.Context, page int) ([]Post, error) {
	if page < 1 {
		page = 1
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Posts []Post `json:"posts"`
	}
	path := fmt.Sprintf("/api/v1/feed?page=%d&wave=0", page)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.Posts, nil
}

func (c *Client) Search(ctx context.Context, q string) (posts []Post, users []SearchUser, clicks []Click, err error) {
	q = strings.TrimSpace(q)
	var resp struct {
		OK     bool         `json:"ok"`
		Error  string       `json:"error"`
		Posts  []Post       `json:"posts"`
		Users  []SearchUser `json:"users"`
		Clicks []Click      `json:"clicks"`
	}
	path := "/api/v1/search?q=" + url.QueryEscape(q)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return nil, nil, nil, err
	}
	if !resp.OK {
		return nil, nil, nil, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.Posts, resp.Users, resp.Clicks, nil
}

func (c *Client) Click(ctx context.Context, slug string) (click Click, posts []Post, gate bool, hasMore bool, err error) {
	slug = strings.TrimSpace(slug)
	if slug == "" || strings.ContainsAny(slug, "/\\?") {
		return Click{}, nil, false, false, fmt.Errorf("invalid click slug")
	}
	var resp struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Click   Click  `json:"click"`
		Posts   []Post `json:"posts"`
		Gate    bool   `json:"gate"`
		HasMore bool   `json:"has_more"`
	}
	path := "/api/v1/clicks/" + url.PathEscape(slug)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return Click{}, nil, false, false, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not open Click"
		}
		return Click{}, nil, false, false, &APIError{Status: 400, Message: msg}
	}
	return resp.Click, resp.Posts, resp.Gate, resp.HasMore, nil
}

func (c *Client) ClickSearch(ctx context.Context, slug, q string, page int) ([]Post, bool, error) {
	slug = strings.TrimSpace(slug)
	q = strings.TrimSpace(q)
	if slug == "" || strings.ContainsAny(slug, "/\\?") {
		return nil, false, fmt.Errorf("invalid click slug")
	}
	if page < 1 {
		page = 1
	}
	var resp struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Posts   []Post `json:"posts"`
		Page    int    `json:"page"`
		HasMore bool   `json:"has_more"`
	}
	path := fmt.Sprintf(
		"/api/v1/clicks/%s/search?q=%s&page=%d",
		url.PathEscape(slug),
		url.QueryEscape(q),
		page,
	)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return nil, false, err
	}
	if !resp.OK {
		return nil, false, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.Posts, resp.HasMore, nil
}

func (c *Client) Post(ctx context.Context, id string) (Post, []Comment, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, "/\\?") {
		return Post{}, nil, fmt.Errorf("invalid post id")
	}
	var resp struct {
		OK       bool      `json:"ok"`
		Error    string    `json:"error"`
		Post     Post      `json:"post"`
		Comments []Comment `json:"comments"`
	}
	path := "/api/v1/posts/" + url.PathEscape(id)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return Post{}, nil, err
	}
	if !resp.OK {
		return Post{}, nil, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.Post, resp.Comments, nil
}

func (c *Client) WallQuote(ctx context.Context, username string) (int, error) {
	username = strings.TrimSpace(username)
	username = strings.TrimPrefix(username, "@")
	if username == "" || strings.ContainsAny(username, "/\\?") {
		return 0, fmt.Errorf("invalid username")
	}
	var resp struct {
		OK          bool   `json:"ok"`
		Error       string `json:"error"`
		Username    string `json:"username"`
		UnreadCount int    `json:"unread_count"`
	}
	path := "/api/v1/users/" + url.PathEscape(username) + "/wall-quote"
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return 0, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "no unread wall quote"
		}
		return 0, &APIError{Status: 404, Message: msg}
	}
	if resp.UnreadCount < 1 {
		return 0, &APIError{Status: 404, Message: "no unread wall quote"}
	}
	return resp.UnreadCount, nil
}

func (c *Client) CreateComment(ctx context.Context, postID, body, idempotencyKey string) (Comment, error) {
	postID = strings.TrimSpace(postID)
	if postID == "" || strings.ContainsAny(postID, "/\\?") {
		return Comment{}, fmt.Errorf("invalid post id")
	}
	var resp struct {
		OK      bool    `json:"ok"`
		Error   string  `json:"error"`
		Comment Comment `json:"comment"`
	}
	path := "/api/v1/posts/" + url.PathEscape(postID) + "/comments"
	err := c.doMutateJSON(ctx, http.MethodPost, path, map[string]any{"body": body}, idempotencyKey, &resp)
	if err != nil {
		return Comment{}, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not post comment"
		}
		return Comment{}, &APIError{Status: 422, Message: msg}
	}
	return resp.Comment, nil
}

func (c *Client) Wall(ctx context.Context, username string, page int) ([]Post, error) {
	username = strings.TrimSpace(username)
	if username == "" || strings.ContainsAny(username, "/\\?") {
		return nil, fmt.Errorf("invalid username")
	}
	if page < 1 {
		page = 1
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Posts []Post `json:"posts"`
	}
	path := fmt.Sprintf("/api/v1/users/%s?tab=wall&page=%d", url.PathEscape(username), page)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.Posts, nil
}

func (c *Client) Pings(ctx context.Context) ([]Ping, int, error) {
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Pings []Ping `json:"pings"`
		Count int    `json:"count"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/pings", nil, true, &resp); err != nil {
		return nil, 0, err
	}
	if !resp.OK {
		return nil, 0, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.Pings, resp.Count, nil
}

func (c *Client) PingsPast(ctx context.Context, page int) ([]Ping, bool, error) {
	if page < 1 {
		page = 1
	}
	var resp struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Pings   []Ping `json:"pings"`
		HasMore bool   `json:"has_more"`
	}
	path := fmt.Sprintf("/api/v1/pings/past?page=%d", page)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return nil, false, err
	}
	if !resp.OK {
		return nil, false, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.Pings, resp.HasMore, nil
}

func (c *Client) PingsPastClear(ctx context.Context) error {
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := c.doMutateJSON(ctx, http.MethodPost, "/api/v1/pings/past/clear", map[string]any{}, "", &resp); err != nil {
		return err
	}
	if !resp.OK {
		return &APIError{Status: 400, Message: resp.Error}
	}
	return nil
}

func (c *Client) PingGo(ctx context.Context, id int64, dismissRelated bool) (PingTarget, error) {
	var resp struct {
		OK     bool       `json:"ok"`
		Error  string     `json:"error"`
		Target PingTarget `json:"target"`
	}
	body := map[string]any{"dismiss_related": dismissRelated}
	path := fmt.Sprintf("/api/v1/pings/%d/go", id)
	if err := c.doMutateJSON(ctx, http.MethodPost, path, body, "", &resp); err != nil {
		return PingTarget{}, err
	}
	if !resp.OK {
		return PingTarget{}, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.Target, nil
}

func ValidateThoughtBody(body string) error {
	return validateBody(body, maxPostLength, "post")
}

func ValidateCommentBody(body string) error {
	return validateBody(body, maxCommentLength, "comment")
}

func validateBody(body string, maxLen int, what string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("write something first")
	}
	if utf8.RuneCountInString(body) > maxLen {
		return fmt.Errorf("%s is too long (max %d characters)", what, maxLen)
	}
	if countLinks(body) > maxLinksPerPost {
		return fmt.Errorf("a %s may contain at most %d links", what, maxLinksPerPost)
	}
	return nil
}

func countLinks(body string) int {
	n := 0
	type span struct{ start, end int }
	var mdSpans []span
	for _, loc := range mdLinkPattern.FindAllStringSubmatchIndex(body, -1) {
		if len(loc) >= 4 {
			n++
			mdSpans = append(mdSpans, span{loc[0], loc[1]})
		}
	}
	for _, loc := range bareURLPattern.FindAllStringIndex(body, -1) {
		overlaps := false
		for _, s := range mdSpans {
			if loc[0] < s.end && loc[1] > s.start {
				overlaps = true
				break
			}
		}
		if !overlaps {
			n++
		}
	}
	return n
}

func NewIdempotencyKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("cs3-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (c *Client) CreateGlobalPost(ctx context.Context, body, bodyColor, idempotencyKey string) (Post, error) {
	payload := map[string]any{"body": body}
	if bodyColor != "" {
		payload["body_color"] = bodyColor
	}
	return c.createPost(ctx, "/api/v1/posts", payload, idempotencyKey)
}

func (c *Client) CreateWallPost(ctx context.Context, username, body, bodyColor, idempotencyKey string) (Post, error) {
	username = strings.TrimSpace(username)
	if username == "" || strings.ContainsAny(username, "/\\?") {
		return Post{}, fmt.Errorf("invalid username")
	}
	payload := map[string]any{"body": body}
	if bodyColor != "" {
		payload["body_color"] = bodyColor
	}
	path := "/api/v1/users/" + url.PathEscape(username) + "/wall"
	return c.createPost(ctx, path, payload, idempotencyKey)
}

func (c *Client) CreateClickPost(ctx context.Context, slug, body, bodyColor, idempotencyKey string) (Post, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" || strings.ContainsAny(slug, "/\\?") {
		return Post{}, fmt.Errorf("invalid click slug")
	}
	payload := map[string]any{"body": body}
	if bodyColor != "" {
		payload["body_color"] = bodyColor
	}
	path := "/api/v1/clicks/" + url.PathEscape(slug) + "/posts"
	return c.createPost(ctx, path, payload, idempotencyKey)
}

func (c *Client) createPost(ctx context.Context, path string, payload map[string]any, idempotencyKey string) (Post, error) {
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Post  Post   `json:"post"`
	}
	err := c.doMutateJSON(ctx, http.MethodPost, path, payload, idempotencyKey, &resp)
	if err != nil {
		return Post{}, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not create post"
		}
		return Post{}, &APIError{Status: 422, Message: msg}
	}
	return resp.Post, nil
}

func (c *Client) UpdatePost(ctx context.Context, id, body string, bodyColor *string, reason, idempotencyKey string) (Post, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, "/\\?") {
		return Post{}, fmt.Errorf("invalid post id")
	}
	payload := map[string]any{"body": body}
	if bodyColor != nil {
		payload["body_color"] = *bodyColor
	}
	if reason != "" {
		payload["reason"] = reason
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Post  Post   `json:"post"`
	}
	path := "/api/v1/posts/" + url.PathEscape(id) + "/update"
	err := c.doMutateJSON(ctx, http.MethodPost, path, payload, idempotencyKey, &resp)
	if err != nil {
		return Post{}, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not update post"
		}
		return Post{}, &APIError{Status: 422, Message: msg}
	}
	return resp.Post, nil
}

func (c *Client) DeletePost(ctx context.Context, id, reason, idempotencyKey string) error {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, "/\\?") {
		return fmt.Errorf("invalid post id")
	}
	payload := map[string]any{}
	if reason != "" {
		payload["reason"] = reason
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	path := "/api/v1/posts/" + url.PathEscape(id) + "/delete"
	err := c.doMutateJSON(ctx, http.MethodPost, path, payload, idempotencyKey, &resp)
	if err != nil {
		return err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not delete post"
		}
		return &APIError{Status: 422, Message: msg}
	}
	return nil
}

func (c *Client) MyPosts(ctx context.Context, page int) (MyPostsPage, error) {
	if page < 1 {
		page = 1
	}
	var resp struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Posts   []Post `json:"posts"`
		Page    int    `json:"page"`
		HasMore bool   `json:"has_more"`
	}
	path := fmt.Sprintf("/api/v1/me/posts?page=%d", page)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return MyPostsPage{}, err
	}
	if !resp.OK {
		return MyPostsPage{}, &APIError{Status: 400, Message: resp.Error}
	}
	return MyPostsPage{Posts: resp.Posts, Page: resp.Page, HasMore: resp.HasMore}, nil
}

func (c *Client) MyClicks(ctx context.Context) ([]Click, error) {
	var resp struct {
		OK     bool    `json:"ok"`
		Error  string  `json:"error"`
		Clicks []Click `json:"clicks"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/me/clicks", nil, true, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, &APIError{Status: 400, Message: resp.Error}
	}
	return resp.Clicks, nil
}

func (c *Client) doMutateJSON(ctx context.Context, method, path string, body any, idempotencyKey string, out any) error {
	if idempotencyKey == "" {
		idempotencyKey = NewIdempotencyKey()
	}
	headers := map[string]string{
		"X-CS3-Client":    "cli",
		"Idempotency-Key": idempotencyKey,
	}
	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, 750*time.Millisecond); err != nil {
				return err
			}
		}
		err := c.doJSONWithHeaders(ctx, method, path, body, true, headers, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !shouldRetryMutate(err) {
			return err
		}
	}
	return lastErr
}

func shouldRetryMutate(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == 503 {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporary") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "eof")
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, auth bool, out any) error {
	return c.doJSONWithHeaders(ctx, method, path, body, auth, nil, out)
}

func (c *Client) doJSONWithHeaders(ctx context.Context, method, path string, body any, auth bool, extra map[string]string, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("X-CS3-Timezone", LocalTimezone())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		if c.Token == "" {
			return &APIError{Status: 401, Message: "not logged in"}
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	for k, v := range extra {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}

	var env struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(raw, &env)

	if res.StatusCode == 429 {
		msg := env.Error
		if msg == "" {
			msg = "Too many attempts. Please wait."
		}
		return &APIError{Status: 429, Message: msg}
	}
	if res.StatusCode >= 400 {
		msg := env.Error
		if msg == "" {
			msg = sanitizeErrorBody(raw)
		}
		if msg == "" {
			msg = "request failed"
		}
		return &APIError{Status: res.StatusCode, Message: msg}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func sanitizeErrorBody(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return ""
	}
	// Avoid dumping HTML / huge payloads into the terminal.
	if strings.HasPrefix(s, "<") || strings.Contains(strings.ToLower(s), "<html") {
		return "unexpected non-JSON error response"
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxErrorBody {
		return s[:maxErrorBody] + "…"
	}
	return s
}

// LocalTimezone returns an IANA timezone for X-CS3-Timezone (TZ env, /etc/localtime, or UTC).
func LocalTimezone() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" && tz != ":" {
		if looksLikeIANATimezone(tz) {
			return tz
		}
	}
	link, err := os.Readlink("/etc/localtime")
	if err != nil {
		return "UTC"
	}
	link = filepath.ToSlash(link)
	const marker = "zoneinfo/"
	if i := strings.Index(link, marker); i >= 0 {
		tz := link[i+len(marker):]
		if looksLikeIANATimezone(tz) {
			return tz
		}
	}
	return "UTC"
}

func looksLikeIANATimezone(tz string) bool {
	if tz == "UTC" || tz == "GMT" {
		return true
	}
	if strings.ContainsAny(tz, " \\") || strings.HasPrefix(tz, ".") {
		return false
	}
	if !strings.Contains(tz, "/") {
		return false
	}
	if len(tz) > 64 {
		return false
	}
	return true
}
