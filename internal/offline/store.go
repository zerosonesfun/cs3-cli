package offline

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/zerosonesfun/cs3-cli/internal/api"
	"github.com/zerosonesfun/cs3-cli/internal/config"
)

const (
	MaxPosts    = 500
	MaxComments = 1000
	MaxThreads  = 100
	overhead    = 64
	pageSize    = 20
)

type Prefs struct {
	Enabled     bool `json:"enabled"`
	LimitMB     int  `json:"limit_mb"`
	KeepThreads bool `json:"keep_threads"`
}

type FeedPage struct {
	Posts   []api.Post
	SavedAt time.Time
}

type ThreadPage struct {
	Post     api.Post
	Comments []api.Comment
	SavedAt  time.Time
	Partial  bool
}

type indexPost struct {
	ID        string `json:"id"`
	Feed      bool   `json:"feed"`
	CreatedAt string `json:"created_at"`
	Bytes     int    `json:"bytes"`
}

type indexComment struct {
	ID        string `json:"id"`
	PostID    string `json:"post_id"`
	CreatedAt string `json:"created_at"`
	Bytes     int    `json:"bytes"`
}

type indexThread struct {
	PostID   string `json:"post_id"`
	OpenedAt int64  `json:"opened_at"`
}

type indexFile struct {
	Posts    []indexPost    `json:"posts"`
	Comments []indexComment `json:"comments"`
	Threads  []indexThread  `json:"threads"`
	Bytes    int            `json:"bytes"`
	SavedAt  int64          `json:"saved_at"`
}

var (
	mu       sync.Mutex
	rootHook string
)

func SetRoot(dir string) {
	mu.Lock()
	rootHook = dir
	mu.Unlock()
}

func IsNetwork(err error) bool {
	var uerr *url.Error
	return errors.As(err, &uerr)
}

func RecordBytes(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return overhead
	}
	return len(b) + overhead
}

func NormalizeLimit(mb int) int {
	switch mb {
	case 5, 10, 20:
		return mb
	default:
		return 10
	}
}

func LoadPrefs(username string) Prefs {
	mu.Lock()
	defer mu.Unlock()
	return readPrefs(username)
}

func SavePrefs(username string, prefs Prefs) error {
	mu.Lock()
	defer mu.Unlock()
	prefs.LimitMB = NormalizeLimit(prefs.LimitMB)
	if err := writePrefs(username, prefs); err != nil {
		return err
	}
	if !prefs.Enabled {
		return clearContentLocked(username)
	}
	if !prefs.KeepThreads {
		index := readIndex(username)
		index.Threads = nil
		if err := writeIndex(username, index); err != nil {
			return err
		}
	}
	return trimLocked(username, prefs.LimitMB*1024*1024, prefs.KeepThreads)
}

func ClearContent(username string) error {
	mu.Lock()
	defer mu.Unlock()
	return clearContentLocked(username)
}

func DeleteAccount(username string) error {
	mu.Lock()
	defer mu.Unlock()
	dir, err := accountDir(username)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func StoreFeed(username string, posts []api.Post) {
	prefs := LoadPrefs(username)
	if !prefs.Enabled || username == "" {
		return
	}
	wrote := false
	for _, post := range posts {
		if post.Scope != "" && post.Scope != "global" {
			continue
		}
		if storePost(username, post, true, prefs) {
			wrote = true
		}
	}
	if wrote {
		touchSaved(username)
	}
}

func StoreThread(username string, post api.Post, comments []api.Comment) {
	prefs := LoadPrefs(username)
	if !prefs.Enabled || username == "" {
		return
	}
	if !storePost(username, post, false, prefs) {
		return
	}
	for _, comment := range comments {
		storeComment(username, comment, post.ID, prefs)
	}
	pin(username, post.ID, prefs.KeepThreads)
	touchSaved(username)
}

func storedFeedCount(username string) int {
	mu.Lock()
	defer mu.Unlock()
	n := 0
	for _, post := range readIndex(username).Posts {
		if post.Feed {
			n++
		}
	}
	return n
}

func LoadFeed(username string) (FeedPage, bool) {
	mu.Lock()
	defer mu.Unlock()
	prefs := readPrefs(username)
	if !prefs.Enabled {
		return FeedPage{}, false
	}
	index := readIndex(username)
	rows := feedRows(index)
	if len(rows) == 0 {
		return FeedPage{}, false
	}
	if len(rows) > pageSize {
		rows = rows[:pageSize]
	}
	out := FeedPage{SavedAt: savedAt(index)}
	for _, row := range rows {
		post, ok := readPost(username, row.ID)
		if ok {
			out.Posts = append(out.Posts, post)
		}
	}
	return out, len(out.Posts) > 0
}

func LoadThread(username, postID string) (ThreadPage, bool) {
	mu.Lock()
	defer mu.Unlock()
	prefs := readPrefs(username)
	if !prefs.Enabled {
		return ThreadPage{}, false
	}
	post, ok := readPost(username, postID)
	if !ok {
		return ThreadPage{}, false
	}
	index := readIndex(username)
	rows := commentsFor(index, postID)
	out := ThreadPage{Post: post, SavedAt: savedAt(index), Partial: post.CommentCount > len(rows)}
	for _, row := range rows {
		comment, ok := readComment(username, row.ID)
		if ok {
			out.Comments = append(out.Comments, comment)
		}
	}
	if post.CommentCount > len(out.Comments) {
		out.Partial = true
	}
	return out, true
}

func storePost(username string, post api.Post, feed bool, prefs Prefs) bool {
	mu.Lock()
	defer mu.Unlock()
	data, err := json.Marshal(post)
	if err != nil {
		return false
	}
	index := readIndex(username)
	bytes := len(data) + overhead
	info := indexPost{ID: post.ID, Feed: feed, CreatedAt: post.CreatedAt, Bytes: bytes}
	for _, existing := range index.Posts {
		if existing.ID == post.ID && existing.Feed {
			info.Feed = true
		}
	}
	before := cloneIndex(index)
	if !makeRoom(&index, bytes, &info, nil, prefs.LimitMB*1024*1024, prefs.KeepThreads) {
		return hasPost(before, post.ID)
	}
	replaced := false
	for i := range index.Posts {
		if index.Posts[i].ID == post.ID {
			index.Bytes -= index.Posts[i].Bytes
			index.Posts[i] = info
			replaced = true
			break
		}
	}
	if !replaced {
		index.Posts = append(index.Posts, info)
	}
	index.Bytes += bytes
	if err := writeBlob(username, "posts", post.ID, data); err != nil {
		return hasPost(before, post.ID)
	}
	if err := writeIndex(username, index); err != nil {
		return hasPost(before, post.ID)
	}
	apply(username, before, index)
	return true
}

func storeComment(username string, comment api.Comment, postID string, prefs Prefs) {
	mu.Lock()
	defer mu.Unlock()
	index := readIndex(username)
	if !hasPost(index, postID) {
		return
	}
	data, err := json.Marshal(comment)
	if err != nil {
		return
	}
	bytes := len(data) + overhead
	info := indexComment{ID: comment.ID, PostID: postID, CreatedAt: comment.CreatedAt, Bytes: bytes}
	before := cloneIndex(index)
	if !makeRoom(&index, bytes, nil, &info, prefs.LimitMB*1024*1024, prefs.KeepThreads) {
		return
	}
	replaced := false
	for i := range index.Comments {
		if index.Comments[i].ID == comment.ID {
			index.Bytes -= index.Comments[i].Bytes
			index.Comments[i] = info
			replaced = true
			break
		}
	}
	if !replaced {
		index.Comments = append(index.Comments, info)
	}
	index.Bytes += bytes
	if writeBlob(username, "comments", comment.ID, data) != nil {
		return
	}
	if writeIndex(username, index) != nil {
		return
	}
	apply(username, before, index)
}

func pin(username, postID string, keep bool) {
	if !keep {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	index := readIndex(username)
	now := time.Now().Unix()
	found := false
	for i := range index.Threads {
		if index.Threads[i].PostID == postID {
			index.Threads[i].OpenedAt = now
			found = true
		}
	}
	if !found {
		index.Threads = append(index.Threads, indexThread{PostID: postID, OpenedAt: now})
	}
	sort.Slice(index.Threads, func(i, j int) bool {
		return index.Threads[i].OpenedAt > index.Threads[j].OpenedAt
	})
	if len(index.Threads) > MaxThreads {
		index.Threads = index.Threads[:MaxThreads]
	}
	_ = writeIndex(username, index)
}

func makeRoom(index *indexFile, extra int, post *indexPost, comment *indexComment, limit int, keep bool) bool {
	scratch := cloneIndex(*index)
	need := extra
	if post != nil {
		if old, ok := findPost(scratch, post.ID); ok {
			need -= old.Bytes
		}
	}
	if comment != nil {
		if old, ok := findComment(scratch, comment.ID); ok {
			need -= old.Bytes
		}
	}
	protected := map[string]bool{}
	if keep {
		for _, thread := range scratch.Threads {
			protected[thread.PostID] = true
		}
	}
	if post != nil {
		protected[post.ID] = true
	}
	hold := map[string]bool{}
	if post != nil {
		hold[post.ID] = true
	}
	if comment != nil {
		hold[comment.PostID] = true
	}
	for n := 0; scratch.Bytes+need > limit && n < 4000; n++ {
		if !evict(&scratch, protected, hold) {
			break
		}
	}
	for n := 0; exceedsCaps(scratch, post, comment) && n < 4000; n++ {
		if !evict(&scratch, protected, hold) {
			return false
		}
	}
	if scratch.Bytes+need > limit || exceedsCaps(scratch, post, comment) {
		return false
	}
	*index = scratch
	return true
}

func exceedsCaps(scratch indexFile, post *indexPost, comment *indexComment) bool {
	feed := 0
	alreadyFeed := false
	if post != nil {
		if old, ok := findPost(scratch, post.ID); ok && old.Feed {
			alreadyFeed = true
		}
	}
	for _, row := range scratch.Posts {
		if row.Feed {
			feed++
		}
	}
	if post != nil && post.Feed && !alreadyFeed {
		feed++
	}
	if feed > MaxPosts {
		return true
	}
	comments := len(scratch.Comments)
	if comment != nil && !hasComment(scratch, comment.ID) {
		comments++
	}
	return comments > MaxComments
}

func evict(index *indexFile, protected, hold map[string]bool) bool {
	var oldest *indexPost
	for i := range index.Posts {
		post := &index.Posts[i]
		if !post.Feed || protected[post.ID] || hold[post.ID] {
			continue
		}
		if oldest == nil || post.CreatedAt < oldest.CreatedAt {
			oldest = post
		}
	}
	if oldest != nil {
		dropPost(index, oldest.ID)
		return true
	}
	var oldComment *indexComment
	for i := range index.Comments {
		comment := &index.Comments[i]
		if protected[comment.PostID] || hold[comment.PostID] {
			continue
		}
		if oldComment == nil || comment.CreatedAt < oldComment.CreatedAt {
			oldComment = comment
		}
	}
	if oldComment != nil {
		dropComment(index, oldComment.ID)
		return true
	}
	var oldestThread *indexThread
	for i := range index.Threads {
		thread := &index.Threads[i]
		if hold[thread.PostID] {
			continue
		}
		if oldestThread == nil || thread.OpenedAt < oldestThread.OpenedAt {
			oldestThread = thread
		}
	}
	if oldestThread != nil {
		id := oldestThread.PostID
		delete(protected, id)
		dropPost(index, id)
		return true
	}
	for _, post := range index.Posts {
		if protected[post.ID] || hold[post.ID] {
			continue
		}
		dropPost(index, post.ID)
		return true
	}
	return false
}

func dropPost(index *indexFile, id string) {
	nextComments := index.Comments[:0]
	for _, comment := range index.Comments {
		if comment.PostID == id {
			index.Bytes -= comment.Bytes
			continue
		}
		nextComments = append(nextComments, comment)
	}
	index.Comments = nextComments
	nextPosts := index.Posts[:0]
	for _, post := range index.Posts {
		if post.ID == id {
			index.Bytes -= post.Bytes
			continue
		}
		nextPosts = append(nextPosts, post)
	}
	index.Posts = nextPosts
	nextThreads := index.Threads[:0]
	for _, thread := range index.Threads {
		if thread.PostID == id {
			continue
		}
		nextThreads = append(nextThreads, thread)
	}
	index.Threads = nextThreads
}

func dropComment(index *indexFile, id string) {
	next := index.Comments[:0]
	for _, comment := range index.Comments {
		if comment.ID == id {
			index.Bytes -= comment.Bytes
			continue
		}
		next = append(next, comment)
	}
	index.Comments = next
}

func apply(username string, before, after indexFile) {
	keepPosts := map[string]bool{}
	for _, post := range after.Posts {
		keepPosts[post.ID] = true
	}
	for _, post := range before.Posts {
		if !keepPosts[post.ID] {
			_ = os.Remove(blobPath(username, "posts", post.ID))
		}
	}
	keepComments := map[string]bool{}
	for _, comment := range after.Comments {
		keepComments[comment.ID] = true
	}
	for _, comment := range before.Comments {
		if !keepComments[comment.ID] {
			_ = os.Remove(blobPath(username, "comments", comment.ID))
		}
	}
}

func trimLocked(username string, limit int, keep bool) error {
	index := readIndex(username)
	before := cloneIndex(index)
	protected := map[string]bool{}
	if keep {
		for _, thread := range index.Threads {
			protected[thread.PostID] = true
		}
	}
	for n := 0; index.Bytes > limit && n < 4000; n++ {
		if !evict(&index, protected, map[string]bool{}) {
			break
		}
	}
	apply(username, before, index)
	return writeIndex(username, index)
}

func touchSaved(username string) {
	mu.Lock()
	defer mu.Unlock()
	index := readIndex(username)
	index.SavedAt = time.Now().Unix()
	_ = writeIndex(username, index)
}

func clearContentLocked(username string) error {
	dir, err := accountDir(username)
	if err != nil {
		return err
	}
	prefs := readPrefs(username)
	_ = os.RemoveAll(filepath.Join(dir, "posts"))
	_ = os.RemoveAll(filepath.Join(dir, "comments"))
	_ = os.Remove(filepath.Join(dir, "index.json"))
	return writePrefs(username, prefs)
}

func readPrefs(username string) Prefs {
	prefs := Prefs{LimitMB: 10, KeepThreads: true}
	b, err := os.ReadFile(prefsPath(username))
	if err != nil {
		return prefs
	}
	_ = json.Unmarshal(b, &prefs)
	prefs.LimitMB = NormalizeLimit(prefs.LimitMB)
	return prefs
}

func writePrefs(username string, prefs Prefs) error {
	dir, err := accountDir(username)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	return os.WriteFile(prefsPath(username), b, 0o600)
}

func readIndex(username string) indexFile {
	var index indexFile
	b, err := os.ReadFile(indexPath(username))
	if err != nil {
		return index
	}
	_ = json.Unmarshal(b, &index)
	return index
}

func writeIndex(username string, index indexFile) error {
	dir, err := accountDir(username)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(index)
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath(username), b, 0o600)
}

func writeBlob(username, folder, id string, data []byte) error {
	path := blobPath(username, folder, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readPost(username, id string) (api.Post, bool) {
	var post api.Post
	b, err := os.ReadFile(blobPath(username, "posts", id))
	if err != nil {
		return post, false
	}
	if json.Unmarshal(b, &post) != nil {
		return post, false
	}
	return post, true
}

func readComment(username, id string) (api.Comment, bool) {
	var comment api.Comment
	b, err := os.ReadFile(blobPath(username, "comments", id))
	if err != nil {
		return comment, false
	}
	if json.Unmarshal(b, &comment) != nil {
		return comment, false
	}
	return comment, true
}

func cloneIndex(index indexFile) indexFile {
	out := index
	out.Posts = append([]indexPost(nil), index.Posts...)
	out.Comments = append([]indexComment(nil), index.Comments...)
	out.Threads = append([]indexThread(nil), index.Threads...)
	return out
}

func feedRows(index indexFile) []indexPost {
	rows := make([]indexPost, 0, len(index.Posts))
	for _, post := range index.Posts {
		if post.Feed {
			rows = append(rows, post)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt > rows[j].CreatedAt })
	return rows
}

func commentsFor(index indexFile, postID string) []indexComment {
	rows := make([]indexComment, 0)
	for _, comment := range index.Comments {
		if comment.PostID == postID {
			rows = append(rows, comment)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt < rows[j].CreatedAt })
	return rows
}

func hasPost(index indexFile, id string) bool {
	_, ok := findPost(index, id)
	return ok
}

func hasComment(index indexFile, id string) bool {
	_, ok := findComment(index, id)
	return ok
}

func findPost(index indexFile, id string) (indexPost, bool) {
	for _, post := range index.Posts {
		if post.ID == id {
			return post, true
		}
	}
	return indexPost{}, false
}

func findComment(index indexFile, id string) (indexComment, bool) {
	for _, comment := range index.Comments {
		if comment.ID == id {
			return comment, true
		}
	}
	return indexComment{}, false
}

func savedAt(index indexFile) time.Time {
	if index.SavedAt == 0 {
		return time.Time{}
	}
	return time.Unix(index.SavedAt, 0)
}

func prefsPath(username string) string {
	dir, _ := accountDir(username)
	return filepath.Join(dir, "meta.json")
}

func indexPath(username string) string {
	dir, _ := accountDir(username)
	return filepath.Join(dir, "index.json")
}

func blobPath(username, folder, id string) string {
	dir, _ := accountDir(username)
	return filepath.Join(dir, folder, safeName(id)+".json")
}

func accountDir(username string) (string, error) {
	root, err := root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, safeName(strings.ToLower(username))), nil
}

func root() (string, error) {
	if rootHook != "" {
		return rootHook, nil
	}
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "offline"), nil
}

func safeName(id string) string {
	var b strings.Builder
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}
