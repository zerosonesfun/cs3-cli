package offline

import (
	"strings"
	"testing"
	"time"

	"github.com/zerosonesfun/cs3-cli/internal/api"
)

func TestRecordBytesUTF8(t *testing.T) {
	short := RecordBytes("a")
	wide := RecordBytes("é")
	if wide <= short {
		t.Fatalf("utf-8 size %d should exceed %d", wide, short)
	}
}

func TestNormalizeLimit(t *testing.T) {
	if NormalizeLimit(5) != 5 || NormalizeLimit(10) != 10 || NormalizeLimit(20) != 20 {
		t.Fatal("expected 5, 10, and 20")
	}
	if NormalizeLimit(15) != 10 || NormalizeLimit(0) != 10 {
		t.Fatal("other sizes fall back to 10")
	}
}

func TestEvictsToLimitAndProtectsPin(t *testing.T) {
	withRoot(t)
	user := "ada"
	if err := SavePrefs(user, Prefs{Enabled: true, LimitMB: 5, KeepThreads: true}); err != nil {
		t.Fatal(err)
	}
	pinned := api.Post{ID: "pin", Username: user, Body: "kept", Scope: "click", CreatedAt: "2020-01-01 00:00:00", CommentCount: 1}
	comment := api.Comment{ID: "c1", Username: user, Body: "note", CreatedAt: "2020-01-01 00:00:01"}
	StoreThread(user, pinned, []api.Comment{comment})
	body := strings.Repeat("x", 900*1024)
	for i := 0; i < 8; i++ {
		StoreFeed(user, []api.Post{{
			ID:        "f" + string(rune('a'+i)),
			Username:  user,
			Body:      body,
			Scope:     "global",
			CreatedAt: time.Date(2024, 1, i+1, 0, 0, 0, 0, time.UTC).Format("2006-01-02 15:04:05"),
		}})
	}
	thread, ok := LoadThread(user, "pin")
	if !ok || thread.Post.ID != "pin" || len(thread.Comments) != 1 {
		t.Fatalf("pinned discussion missing: %+v %v", thread, ok)
	}
	page, ok := LoadFeed(user)
	if !ok {
		t.Fatal("expected some feed posts")
	}
	var total int
	for _, post := range page.Posts {
		total += len(post.Body)
	}
	if total > 5*1024*1024 {
		t.Fatalf("feed page exceeds 5MB: %d", total)
	}
}

func TestTwentyMegabyteCeiling(t *testing.T) {
	withRoot(t)
	user := "bea"
	if err := SavePrefs(user, Prefs{Enabled: true, LimitMB: 20, KeepThreads: true}); err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("y", 1024*1024)
	for i := 0; i < 22; i++ {
		id := "p" + strings.Repeat("z", i+1)
		StoreFeed(user, []api.Post{{
			ID:        id,
			Body:      body,
			Scope:     "global",
			CreatedAt: time.Date(2024, 2, 1, 0, i, 0, 0, time.UTC).Format("2006-01-02 15:04:05"),
		}})
	}
	if err := SavePrefs(user, Prefs{Enabled: true, LimitMB: 10, KeepThreads: true}); err != nil {
		t.Fatal(err)
	}
	page, _ := LoadFeed(user)
	var total int
	for _, post := range page.Posts {
		total += RecordBytes(post)
	}
	if total > 10*1024*1024 {
		t.Fatalf("lowering the limit should trim, have %d", total)
	}
}

func TestOversizedThreadSkipped(t *testing.T) {
	withRoot(t)
	user := "cy"
	if err := SavePrefs(user, Prefs{Enabled: true, LimitMB: 5, KeepThreads: true}); err != nil {
		t.Fatal(err)
	}
	StoreFeed(user, []api.Post{{
		ID: "small", Body: "ok", Scope: "global", CreatedAt: "2024-03-01 00:00:00",
	}})
	StoreThread(user, api.Post{
		ID: "huge", Body: strings.Repeat("z", 6*1024*1024), CreatedAt: "2024-03-02 00:00:00",
	}, nil)
	if _, ok := LoadThread(user, "huge"); ok {
		t.Fatal("oversized discussion should be skipped")
	}
	page, ok := LoadFeed(user)
	if !ok || len(page.Posts) != 1 || page.Posts[0].ID != "small" {
		t.Fatalf("small post should remain: %+v", page.Posts)
	}
}

func TestAccountSeparation(t *testing.T) {
	withRoot(t)
	for _, user := range []string{"one", "two"} {
		if err := SavePrefs(user, Prefs{Enabled: true, LimitMB: 5, KeepThreads: true}); err != nil {
			t.Fatal(err)
		}
		StoreFeed(user, []api.Post{{
			ID: user + "-post", Body: user, Scope: "global", CreatedAt: "2024-04-01 00:00:00",
		}})
	}
	page, ok := LoadFeed("one")
	if !ok || len(page.Posts) != 1 || page.Posts[0].ID != "one-post" {
		t.Fatalf("account one: %+v", page.Posts)
	}
	if err := DeleteAccount("two"); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadFeed("two"); ok {
		t.Fatal("deleted account should be empty")
	}
	page, ok = LoadFeed("one")
	if !ok || page.Posts[0].ID != "one-post" {
		t.Fatal("other account should remain")
	}
}

func TestFeedCeiling(t *testing.T) {
	withRoot(t)
	if err := SavePrefs("ada", Prefs{Enabled: true, LimitMB: 5, KeepThreads: true}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 510; i++ {
		when := time.Date(2024, 6, 1, 0, 0, i, 0, time.UTC).Format("2006-01-02 15:04:05")
		StoreFeed("ada", []api.Post{{
			ID:        "p" + strings.Repeat("a", i+1),
			Body:      "x",
			Scope:     "global",
			CreatedAt: when,
		}})
	}
	if n := storedFeedCount("ada"); n > MaxPosts {
		t.Fatalf("feed ceiling exceeded: %d", n)
	}
}

func TestClearKeepsPrefs(t *testing.T) {
	withRoot(t)
	if err := SavePrefs("ada", Prefs{Enabled: true, LimitMB: 20, KeepThreads: false}); err != nil {
		t.Fatal(err)
	}
	StoreFeed("ada", []api.Post{{ID: "a", Body: "a", Scope: "global", CreatedAt: "2024-05-01 00:00:00"}})
	if err := ClearContent("ada"); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadFeed("ada"); ok {
		t.Fatal("content should be gone")
	}
	prefs := LoadPrefs("ada")
	if !prefs.Enabled || prefs.LimitMB != 20 || prefs.KeepThreads {
		t.Fatalf("prefs changed: %+v", prefs)
	}
}

func withRoot(t *testing.T) {
	t.Helper()
	SetRoot(t.TempDir())
	t.Cleanup(func() { SetRoot("") })
}
