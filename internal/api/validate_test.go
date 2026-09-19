package api

import (
	"strings"
	"testing"
)

func TestValidateCommentBody(t *testing.T) {
	if err := ValidateCommentBody(""); err == nil {
		t.Fatal("empty comment should fail")
	}
	if err := ValidateCommentBody("   "); err == nil {
		t.Fatal("whitespace comment should fail")
	}
	if err := ValidateCommentBody("hello"); err != nil {
		t.Fatalf("short comment: %v", err)
	}
	long := strings.Repeat("a", maxCommentLength+1)
	if err := ValidateCommentBody(long); err == nil {
		t.Fatal("over-length comment should fail")
	}
	links := "see https://a.example https://b.example https://c.example https://d.example"
	if err := ValidateCommentBody(links); err == nil {
		t.Fatal("4 links should fail")
	}
	okLinks := "see https://a.example https://b.example https://c.example"
	if err := ValidateCommentBody(okLinks); err != nil {
		t.Fatalf("3 links: %v", err)
	}
}

func TestValidateThoughtBodyKeepsLeadingSpacesForLength(t *testing.T) {
	if err := ValidateThoughtBody("  hi"); err != nil {
		t.Fatalf("leading spaces are allowed: %v", err)
	}
}

func TestCountLinksMarkdownNotDoubleCounted(t *testing.T) {
	body := "read [docs](https://example.com/a) and https://example.com/b"
	if n := countLinks(body); n != 2 {
		t.Fatalf("got %d links, want 2", n)
	}
}
