package ui

import (
	"strings"
	"unicode/utf8"
)

const (
	zwj          = 0x200D
	variationSel = 0xFE0F
	combiningKey = 0x20E3
	riFirst      = 0x1F1E6
	riLast       = 0x1F1FF
	modFirst     = 0x1F3FB
	modLast      = 0x1F3FF
)

// Emojicon replaces mapped emoji with emoticons/kaomoji (display-only).
// Walks emoji clusters (ZWJ / skin tone / RI / keycap) so unmapped sequences
// stay intact — same idea as web/iOS EMOJI_SEQ lookup.
func Emojicon(s string) string {
	if s == "" || !emojiconMaybe(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		cluster, adv := emojiconTakeCluster(s, i)
		if adv <= 0 {
			break
		}
		if hit, ok := emojiconLookup(cluster); ok {
			b.WriteString(hit)
		} else {
			b.WriteString(cluster)
		}
		i += adv
	}
	return b.String()
}

func emojiconLookup(seq string) (string, bool) {
	if hit, ok := emojiconMap[seq]; ok {
		return hit, true
	}
	stripped := strings.ReplaceAll(seq, "\uFE0F", "")
	if stripped != seq {
		if hit, ok := emojiconMap[stripped]; ok {
			return hit, true
		}
	}
	return "", false
}

func emojiconMaybe(s string) bool {
	for _, r := range s {
		if r > 0x7F {
			return true
		}
	}
	return false
}

func emojiconTakeCluster(s string, i int) (string, int) {
	if i >= len(s) {
		return "", 0
	}
	r, size := utf8.DecodeRuneInString(s[i:])
	if r == utf8.RuneError && size == 1 {
		return s[i : i+1], 1
	}

	if isRegionalIndicator(r) {
		r2, sz2 := utf8.DecodeRuneInString(s[i+size:])
		if isRegionalIndicator(r2) {
			return s[i : i+size+sz2], size + sz2
		}
		return s[i : i+size], size
	}

	if (r >= '0' && r <= '9') || r == '#' || r == '*' {
		j := i + size
		if j < len(s) {
			r2, sz2 := utf8.DecodeRuneInString(s[j:])
			if r2 == variationSel {
				j += sz2
			}
		}
		if j < len(s) {
			r3, sz3 := utf8.DecodeRuneInString(s[j:])
			if r3 == combiningKey {
				return s[i : j+sz3], j + sz3 - i
			}
		}
	}

	if !isEmojiBase(r) {
		return s[i : i+size], size
	}

	j := i + size
	j = consumeEmojiExtras(s, j)
	for j < len(s) {
		rz, szz := utf8.DecodeRuneInString(s[j:])
		if rz != zwj {
			break
		}
		j2 := j + szz
		if j2 >= len(s) {
			break
		}
		r2, sz2 := utf8.DecodeRuneInString(s[j2:])
		if !isEmojiBase(r2) && !isRegionalIndicator(r2) {
			break
		}
		j = j2 + sz2
		j = consumeEmojiExtras(s, j)
	}
	return s[i:j], j - i
}

func consumeEmojiExtras(s string, j int) int {
	for j < len(s) {
		r, sz := utf8.DecodeRuneInString(s[j:])
		if r == variationSel || isEmojiModifier(r) {
			j += sz
			continue
		}
		break
	}
	return j
}

func isRegionalIndicator(r rune) bool {
	return r >= riFirst && r <= riLast
}

func isEmojiModifier(r rune) bool {
	return r >= modFirst && r <= modLast
}

func isEmojiBase(r rune) bool {
	switch {
	case r == 0x00A9 || r == 0x00AE:
		return true
	case r >= 0x203C && r <= 0x3299:
		return true
	case r >= 0x1F000 && r <= 0x1FAFF:
		return true
	default:
		return false
	}
}
