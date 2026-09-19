package ui

import (
	"strings"
	"testing"
)

func TestEmojiconGrinning(t *testing.T) {
	in := "hi \U0001F600 there"
	out := Emojicon(in)
	if out == in {
		t.Fatalf("expected replacement, got %q", out)
	}
	if !strings.Contains(out, ":D") && !strings.Contains(out, "^_^") && !strings.Contains(out, ":)") {
		t.Fatalf("unexpected out %q", out)
	}
}

func TestEmojiconLeavesPlain(t *testing.T) {
	in := "hello world"
	if Emojicon(in) != in {
		t.Fatalf("plain text changed")
	}
}

func TestEmojiconDoesNotSplitUnmappedZWJ(t *testing.T) {
	// Invented ZWJ cluster unlikely to be a named emoji in the map.
	zwj := "\u200D"
	seq := "\U0001F921" + zwj + "\U0001F916" // clown + ZWJ + robot
	if _, ok := emojiconMap[seq]; ok {
		t.Skip("unexpected: invented sequence is mapped")
	}
	out := Emojicon("x " + seq + " y")
	if !strings.Contains(out, seq) {
		t.Fatalf("ZWJ cluster was split: %q", out)
	}
	if strings.Contains(out, "(clown)") || strings.Contains(out, "(robot)") {
		t.Fatalf("partial replacement inside ZWJ cluster: %q", out)
	}
}
