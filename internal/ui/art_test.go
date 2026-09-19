package ui

import (
	"strings"
	"testing"
)

func TestFormatArtPreviewSkipsLeadingBlankAndCapsRows(t *testing.T) {
	var lines []string
	lines = append(lines, strings.Repeat(" ", 40))
	lines = append(lines, strings.Repeat(" ", 40))
	for i := 0; i < 25; i++ {
		row := strings.Repeat("▪", 40)
		lines = append(lines, row)
	}
	body := strings.Join(lines, "\n")
	out := FormatArt(body, "", true)
	got := strings.Split(out, "\n")
	// first line + 19 continuations indented + … line
	if len(got) != 21 {
		t.Fatalf("expected 20 preview rows + …, got %d lines:\n%q", len(got), out)
	}
	if !strings.HasSuffix(out, "…") {
		t.Fatalf("expected ellipsis, got %q", out)
	}
	if !strings.HasPrefix(got[1], "    ") {
		t.Fatalf("continuation should be indented: %q", got[1])
	}
}

func TestFormatArtFullNoIndent(t *testing.T) {
	body := "▪▪▪\n□□□\n●●●"
	out := FormatArt(body, "", false)
	if strings.Contains(out, "    ") {
		t.Fatalf("full art should not indent: %q", out)
	}
	if out != body {
		t.Fatalf("got %q want %q", out, body)
	}
}

func TestFormatArtWrapsLongLine(t *testing.T) {
	body := strings.Repeat("·", 80)
	out := FormatArt(body, "", false)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 rows, got %d: %q", len(lines), out)
	}
	if len([]rune(lines[0])) != 40 || len([]rune(lines[1])) != 40 {
		t.Fatalf("expected 40-wide rows: %q", out)
	}
}

func TestParseArtColorsLegacy(t *testing.T) {
	p := parseArtColors("0123")
	if p == nil {
		t.Fatal("expected palette")
	}
	if artColorAt(p, 2) != "green" {
		t.Fatalf("cell 2 want green, got %s", artColorAt(p, 2))
	}
}

func TestParseArtColorsV2(t *testing.T) {
	p := parseArtColors("v2|default,blue,#aabbcc|012")
	if p == nil {
		t.Fatal("expected palette")
	}
	if artColorAt(p, 2) != "#aabbcc" {
		t.Fatalf("cell 2 want #aabbcc, got %s", artColorAt(p, 2))
	}
}
