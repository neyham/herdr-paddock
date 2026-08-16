package layout

import (
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

type Band int

const (
	Phone Band = iota
	Compact
	Full
)

func BandOf(width int) Band {
	switch {
	case width <= 52:
		return Phone
	case width <= 79:
		return Compact
	default:
		return Full
	}
}

func (b Band) String() string {
	switch b {
	case Phone:
		return "phone"
	case Compact:
		return "compact"
	default:
		return "full"
	}
}

func Width(s string) int {
	return runewidth.StringWidth(s)
}

// Truncate to at most max display cells without splitting a wide rune.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if Width(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w <= 0 {
			continue
		}
		if used+w > max-1 {
			break
		}
		b.WriteRune(r)
		used += w
	}
	b.WriteRune('…')
	return b.String()
}

func Fit(s string, max int) string {
	return Truncate(s, max)
}

// Clip cuts to max cells. No ellipsis — preview text should not grow a fake "...".
func Clip(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if Width(s) <= max {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w <= 0 {
			continue
		}
		if used+w > max {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String()
}

// Wrap fills lines to max cells. Prefers breaking at spaces; CJK and overlong
// tokens can split. Never inserts an ellipsis.
func Wrap(s string, max int) []string {
	if max <= 0 {
		return nil
	}
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var lines []string
	var line, word strings.Builder
	lineW, wordW := 0, 0
	flushLine := func() {
		if line.Len() == 0 {
			return
		}
		lines = append(lines, strings.TrimRight(line.String(), " "))
		line.Reset()
		lineW = 0
	}
	flushWord := func() {
		if word.Len() == 0 {
			return
		}
		if lineW > 0 && lineW+wordW > max {
			flushLine()
		}
		if wordW > max {
			for _, r := range word.String() {
				w := runewidth.RuneWidth(r)
				if w <= 0 {
					continue
				}
				if lineW+w > max {
					flushLine()
				}
				line.WriteRune(r)
				lineW += w
			}
		} else {
			line.WriteString(word.String())
			lineW += wordW
		}
		word.Reset()
		wordW = 0
	}
	for _, r := range s {
		if r == '\n' {
			flushWord()
			flushLine()
			continue
		}
		w := runewidth.RuneWidth(r)
		if w <= 0 {
			continue
		}
		if r == ' ' {
			flushWord()
			if lineW > 0 && lineW+w <= max {
				line.WriteRune(' ')
				lineW += w
			}
			continue
		}
		// CJK / wide runes wrap on their own.
		if w > 1 {
			flushWord()
			if lineW+w > max {
				flushLine()
			}
			line.WriteRune(r)
			lineW += w
			continue
		}
		word.WriteRune(r)
		wordW += w
	}
	flushWord()
	flushLine()
	return lines
}

// SplitCuts s after n display cells. ANSI sequences stay with the side they belong to.
func SplitCells(s string, n int) (left, right string) {
	if n <= 0 {
		return "", s
	}
	var b strings.Builder
	used := 0
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				b.WriteString(s[i : j+1])
				i = j + 1
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w := runewidth.RuneWidth(r)
		if w <= 0 {
			i += size
			continue
		}
		if used+w > n {
			return b.String(), s[i:]
		}
		b.WriteRune(r)
		used += w
		i += size
		if used == n {
			return b.String(), s[i:]
		}
	}
	return b.String(), ""
}

func HotLimit(width int) int {
	if BandOf(width) == Phone {
		return 2
	}
	return 3
}
