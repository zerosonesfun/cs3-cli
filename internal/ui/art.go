package ui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

const (
	ArtWidth       = 40
	ArtPreviewRows = 20
	artMapChars    = "0123456789abcdefghij"
)

// FormatArt renders a stored art body as a 40-column character grid.
// When preview is true, leading blank rows are skipped and at most ArtPreviewRows
// rows are shown (website truncate_art_lines); continuation lines are indented
// with 4 spaces for feed list prefixes. Paint colors use ANSI when stdout is a TTY.
func FormatArt(body, artColors string, preview bool) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	rows := artRows(body)
	cellOffset := 0
	truncated := false
	if preview {
		rows, cellOffset, truncated = truncateArtRows(rows, ArtPreviewRows)
	}
	useColor := artColors != "" && term.IsTerminal(int(os.Stdout.Fd()))
	parsed := parseArtColors(artColors)
	if !useColor || parsed == nil {
		useColor = false
		parsed = nil
	}

	rowSep := "\n"
	if preview {
		rowSep = "\n    "
	}

	var b strings.Builder
	cell := cellOffset
	for i, row := range rows {
		if i > 0 {
			b.WriteString(rowSep)
		}
		for _, ch := range row {
			if useColor {
				b.WriteString(ansiPaint(ch, artColorAt(parsed, cell)))
				cell++
			} else {
				b.WriteRune(ch)
			}
		}
	}
	if truncated {
		if len(rows) > 0 {
			b.WriteString(rowSep)
		}
		b.WriteString("…")
	}
	return b.String()
}

func artRows(body string) [][]rune {
	if body == "" {
		return nil
	}
	// Defensive: one long line without newlines → wrap every ArtWidth cells.
	if !strings.Contains(body, "\n") {
		runes := []rune(body)
		if len(runes) > ArtWidth {
			var rows [][]rune
			for len(runes) > 0 {
				n := ArtWidth
				if n > len(runes) {
					n = len(runes)
				}
				rows = append(rows, runes[:n])
				runes = runes[n:]
			}
			return rows
		}
	}
	parts := strings.Split(body, "\n")
	rows := make([][]rune, 0, len(parts))
	for _, p := range parts {
		rows = append(rows, []rune(p))
	}
	return rows
}

// truncateArtRows mirrors helpers.php truncate_art_lines(..., skipLeadingBlank: true).
func truncateArtRows(rows [][]rune, maxRows int) (out [][]rune, cellOffset int, truncated bool) {
	if maxRows < 1 {
		maxRows = 1
	}
	origEmpty := len(rows) == 0
	for len(rows) > 0 && isBlankArtRow(rows[0]) {
		cellOffset += len(rows[0])
		rows = rows[1:]
	}
	if len(rows) == 0 {
		if origEmpty {
			return nil, 0, false
		}
		// All blank: show nothing; treat as truncated only if we stripped content.
		return nil, cellOffset, cellOffset > 0
	}
	if len(rows) <= maxRows {
		return rows, cellOffset, cellOffset > 0
	}
	return rows[:maxRows], cellOffset, true
}

func isBlankArtRow(row []rune) bool {
	for _, r := range row {
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

type artPalette struct {
	palette []string
	mapStr  string
}

func parseArtColors(raw string) *artPalette {
	raw = strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', ' ', '\t':
			return -1
		default:
			return r
		}
	}, raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "v2|") {
		rest := raw[3:]
		pipe := strings.LastIndex(rest, "|")
		if pipe < 0 {
			return nil
		}
		paletteStr := rest[:pipe]
		mapStr := rest[pipe+1:]
		if mapStr == "" || !artMapValid(mapStr) {
			return nil
		}
		if paletteStr == "" {
			return nil
		}
		parts := strings.Split(paletteStr, ",")
		if len(parts) > 20 {
			return nil
		}
		palette := make([]string, 0, len(parts))
		customCount := 0
		for _, part := range parts {
			tok := strings.ToLower(strings.TrimSpace(part))
			if tok == "" {
				return nil
			}
			switch tok {
			case "default", "blue", "green", "red":
				palette = append(palette, tok)
				continue
			}
			hex := normalizeHex(tok)
			if hex == "" {
				return nil
			}
			customCount++
			if customCount > 16 {
				return nil
			}
			palette = append(palette, hex)
		}
		if len(palette) == 0 {
			return nil
		}
		return &artPalette{palette: palette, mapStr: mapStr}
	}
	for _, r := range raw {
		if r < '0' || r > '3' {
			return nil
		}
	}
	return &artPalette{
		palette: []string{"default", "blue", "green", "red"},
		mapStr:  raw,
	}
}

func artMapValid(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune(artMapChars, r) {
			return false
		}
	}
	return true
}

func normalizeHex(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) == 7 && s[0] == '#' {
		for i := 1; i < 7; i++ {
			c := s[i]
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return ""
			}
		}
		return s
	}
	if len(s) == 4 && s[0] == '#' {
		a, b, c := s[1], s[2], s[3]
		hexDigit := func(d byte) bool {
			return (d >= '0' && d <= '9') || (d >= 'a' && d <= 'f')
		}
		if hexDigit(a) && hexDigit(b) && hexDigit(c) {
			return fmt.Sprintf("#%c%c%c%c%c%c", a, a, b, b, c, c)
		}
	}
	return ""
}

func artColorAt(p *artPalette, cellIndex int) string {
	if p == nil || cellIndex < 0 || cellIndex >= len(p.mapStr) {
		return "default"
	}
	idx := strings.IndexByte(artMapChars, p.mapStr[cellIndex])
	if idx < 0 || idx >= len(p.palette) {
		return "default"
	}
	return p.palette[idx]
}

func ansiPaint(ch rune, color string) string {
	if ch == ' ' || color == "" || color == "default" {
		return string(ch)
	}
	var code string
	switch color {
	case "blue":
		code = "\x1b[34m"
	case "green":
		code = "\x1b[32m"
	case "red":
		code = "\x1b[31m"
	default:
		if strings.HasPrefix(color, "#") && len(color) == 7 {
			r, _ := strconv.ParseUint(color[1:3], 16, 8)
			g, _ := strconv.ParseUint(color[3:5], 16, 8)
			b, _ := strconv.ParseUint(color[5:7], 16, 8)
			code = fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
		} else {
			return string(ch)
		}
	}
	return code + string(ch) + "\x1b[0m"
}
