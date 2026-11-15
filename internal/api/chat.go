package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type ChatMessage struct {
	ID           int    `json:"id"`
	UserID       int    `json:"user_id"`
	Username     string `json:"username"`
	ProfileURL   string `json:"profile_url"`
	Body         string `json:"body"`
	BodyColor    string `json:"body_color"`
	BodyColorHex string `json:"body_color_hex"`
	CreatedAt    string `json:"created_at"`
	TimeAgo      string `json:"time_ago"`
	Status       string `json:"status"`
	IsOwn        bool   `json:"is_own"`
	CanDelete    bool   `json:"can_delete"`
	CanModDelete bool   `json:"can_mod_delete"`
	CanModBan    bool   `json:"can_mod_ban"`
}

type ChatScheduleSlot struct {
	Slot     int    `json:"slot"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
	Label    string `json:"label"`
}

type ChatPrefs struct {
	ChatEraseOnLogout      int    `json:"chat_erase_on_logout"`
	ChatErasePromptSeen    int    `json:"chat_erase_prompt_seen"`
	ChatErasePromptSession string `json:"chat_erase_prompt_session"`
	ChatBodyColor          string `json:"chat_body_color"`
}

type ChatState struct {
	Live            bool               `json:"live"`
	SessionStartsAt string             `json:"session_starts_at"`
	Schedule        []ChatScheduleSlot `json:"schedule"`
	PresenceCount   int                `json:"presence_count"`
	MaxPresence     int                `json:"max_presence"`
	CanJoin         bool               `json:"can_join"`
	Present         bool               `json:"present"`
	Full            bool               `json:"full"`
	Error           string             `json:"error"`
	Messages        []ChatMessage      `json:"messages"`
	Prefs           ChatPrefs          `json:"prefs"`
	IsMod           bool               `json:"is_mod"`
	MaxBody         int                `json:"max_body"`
}

type ChatPresenceUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

func (c *Client) ChatState(ctx context.Context) (ChatState, error) {
	var resp struct {
		OK bool `json:"ok"`
		ChatState
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/chat/state", nil, true, &resp); err != nil {
		return ChatState{}, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not load chat"
		}
		return ChatState{}, &APIError{Status: 400, Message: msg}
	}
	return resp.ChatState, nil
}

// ChatMessages long-polls for messages after afterID.
// An empty poll with timeout=true is success with no messages (not an offline error).
func (c *Client) ChatMessages(ctx context.Context, afterID int) (msgs []ChatMessage, timedOut bool, err error) {
	if afterID < 0 {
		afterID = 0
	}
	var resp struct {
		OK       bool          `json:"ok"`
		Error    string        `json:"error"`
		Messages []ChatMessage `json:"messages"`
		Timeout  bool          `json:"timeout"`
	}
	path := "/api/v1/chat/messages?after_id=" + strconv.Itoa(afterID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return nil, false, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not load chat messages"
		}
		return nil, false, &APIError{Status: 400, Message: msg}
	}
	return resp.Messages, resp.Timeout, nil
}

func (c *Client) ChatSend(ctx context.Context, body, bodyColor string) (ChatMessage, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return ChatMessage{}, fmt.Errorf("message cannot be empty")
	}
	payload := map[string]any{"body": body}
	if strings.TrimSpace(bodyColor) != "" {
		payload["body_color"] = strings.TrimSpace(bodyColor)
	}
	var resp struct {
		OK      bool        `json:"ok"`
		Error   string      `json:"error"`
		Message ChatMessage `json:"message"`
	}
	// No auto-retry: chat send is not server-idempotent.
	if err := c.doMutateJSONNoRetry(ctx, http.MethodPost, "/api/v1/chat/messages", payload, "", &resp); err != nil {
		return ChatMessage{}, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not send"
		}
		return ChatMessage{}, &APIError{Status: 400, Message: msg}
	}
	return resp.Message, nil
}

func (c *Client) ChatDeleteMessage(ctx context.Context, id int, reason string) (ChatMessage, error) {
	if id < 1 {
		return ChatMessage{}, fmt.Errorf("message id is required")
	}
	payload := map[string]any{}
	if strings.TrimSpace(reason) != "" {
		payload["reason"] = strings.TrimSpace(reason)
	}
	var resp struct {
		OK      bool         `json:"ok"`
		Error   string       `json:"error"`
		Message *ChatMessage `json:"message"`
	}
	path := "/api/v1/chat/messages/" + strconv.Itoa(id) + "/delete"
	if err := c.doMutateJSONNoRetry(ctx, http.MethodPost, path, payload, "", &resp); err != nil {
		return ChatMessage{}, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not delete"
		}
		return ChatMessage{}, &APIError{Status: 400, Message: msg}
	}
	if resp.Message == nil {
		return ChatMessage{}, nil
	}
	return *resp.Message, nil
}

func (c *Client) ChatPresence(ctx context.Context) (count int, err error) {
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Count int    `json:"count"`
	}
	if err := c.doMutateJSONNoRetry(ctx, http.MethodPost, "/api/v1/chat/presence", map[string]any{}, "", &resp); err != nil {
		return 0, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not join chat"
		}
		return 0, &APIError{Status: 400, Message: msg}
	}
	return resp.Count, nil
}

func (c *Client) ChatPresenceLeave(ctx context.Context) error {
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := c.doMutateJSONNoRetry(ctx, http.MethodPost, "/api/v1/chat/presence/leave", map[string]any{}, "", &resp); err != nil {
		return err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not leave chat"
		}
		return &APIError{Status: 400, Message: msg}
	}
	return nil
}

func (c *Client) ChatPresenceList(ctx context.Context) ([]ChatPresenceUser, error) {
	var resp struct {
		OK    bool               `json:"ok"`
		Error string             `json:"error"`
		Users []ChatPresenceUser `json:"users"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/chat/presence/list", nil, true, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not list presence"
		}
		return nil, &APIError{Status: 400, Message: msg}
	}
	return resp.Users, nil
}

func (c *Client) ChatPrefs(ctx context.Context, eraseOnLogout *bool, bodyColor string) error {
	payload := map[string]any{}
	if eraseOnLogout != nil {
		if *eraseOnLogout {
			payload["chat_erase_on_logout"] = 1
		} else {
			payload["chat_erase_on_logout"] = 0
		}
	}
	if strings.TrimSpace(bodyColor) != "" {
		payload["chat_body_color"] = strings.TrimSpace(bodyColor)
	}
	if len(payload) == 0 {
		return fmt.Errorf("nothing to update")
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := c.doMutateJSONNoRetry(ctx, http.MethodPost, "/api/v1/chat/prefs", payload, "", &resp); err != nil {
		return err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not save chat prefs"
		}
		return &APIError{Status: 400, Message: msg}
	}
	return nil
}

func (c *Client) ChatBan(ctx context.Context, userID int, reason, duration string) error {
	if userID < 1 {
		return fmt.Errorf("user id is required")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("reason is required")
	}
	duration = strings.TrimSpace(duration)
	if duration == "" {
		duration = "1h"
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := c.doMutateJSONNoRetry(ctx, http.MethodPost, "/api/v1/chat/ban", map[string]any{
		"user_id":  userID,
		"reason":   reason,
		"duration": duration,
	}, "", &resp); err != nil {
		return err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not ban"
		}
		return &APIError{Status: 400, Message: msg}
	}
	return nil
}

func (c *Client) ChatUnban(ctx context.Context, userID int) error {
	if userID < 1 {
		return fmt.Errorf("user id is required")
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := c.doMutateJSONNoRetry(ctx, http.MethodPost, "/api/v1/chat/unban", map[string]any{
		"user_id": userID,
	}, "", &resp); err != nil {
		return err
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "could not unban"
		}
		return &APIError{Status: 400, Message: msg}
	}
	return nil
}
