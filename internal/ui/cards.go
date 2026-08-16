package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/neyham/herdr-paddock/internal/layout"
)

var (
	reDateSeg = regexp.MustCompile(`^20\d\d-\d\d-\d\d$`)
	reOptRow  = regexp.MustCompile(`^\d+[.)][ \t]`)
	// agent chrome footers: token counters like "↑96k ↓37k R5.9M $0.020 12.6%/1.0M"
	reFooterUp = regexp.MustCompile(`[↑↓]\s*\d`)
	reCostPct  = regexp.MustCompile(`\$\d+(\.\d+)?|\d+(\.\d+)?%/`)
)

// parseCardTitle extracts the task from herdr terminal titles that follow the
// skill convention "<agent> - <date>｜<task>｜<subtask> - <dir>". Free-form
// titles fall through unchanged; callers fall back to the tab label when the
// result is too short to mean anything.
func parseCardTitle(title string) string {
	s := strings.TrimSpace(title)
	if s == "" {
		return ""
	}
	if i := strings.LastIndex(s, " - "); i > 0 {
		s = s[:i]
	}
	if i := strings.Index(s, " - "); i > 0 {
		head := s[:i]
		if !strings.Contains(head, "｜") && layout.Width(head) <= 6 {
			s = s[i+3:]
		}
	}
	parts := strings.Split(s, "｜")
	if len(parts) > 1 && reDateSeg.MatchString(strings.TrimSpace(parts[0])) {
		parts = parts[1:]
	}
	out := strings.TrimSpace(parts[len(parts)-1])
	if out == "" && len(parts) > 1 {
		out = strings.TrimSpace(parts[len(parts)-2])
	}
	return out
}

// questionStart finds where the agent's pending question begins in the cleaned
// preview lines: an option list tail (1. / 2. / ❯) climbing up to a line that
// ends with a question mark, or a bare question in the last few lines. -1 when
// nothing looks like a question.
func questionStart(lines []string) int {
	isOpt := func(s string) bool {
		s = strings.TrimSpace(s)
		return reOptRow.MatchString(s) || strings.HasPrefix(s, "❯")
	}
	isQ := func(s string) bool {
		s = strings.TrimSpace(s)
		return strings.HasSuffix(s, "?") || strings.HasSuffix(s, "？") ||
			strings.Contains(s, "[Y/n]") || strings.Contains(s, "[y/N]") ||
			strings.Contains(s, "(y/n)") || strings.Contains(s, "(Y/n)")
	}
	i := len(lines) - 1
	for i >= 0 && isOpt(lines[i]) {
		i--
	}
	if i >= 0 && i < len(lines)-1 && isQ(lines[i]) {
		return i
	}
	for j := len(lines) - 1; j >= 0 && j >= len(lines)-3; j-- {
		if isQ(lines[j]) {
			return j
		}
	}
	return -1
}

func fmtAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// looksLikeChrome reports terminal noise that must never reach a card:
// spinner rows, agent footer status bars, interrupt hints, prompt residue.
func looksLikeChrome(ln string) bool {
	if ln == "" {
		return false
	}
	r := []rune(ln)[0]
	if r >= 0x2800 && r <= 0x28FF { // braille spinner frames
		return true
	}
	if reFooterUp.MatchString(ln) && reCostPct.MatchString(ln) {
		return true
	}
	if strings.Contains(ln, "↑") && strings.Contains(ln, "↓") && strings.ContainsAny(ln, "0123456789") {
		return true
	}
	low := strings.ToLower(ln)
	if strings.Contains(low, "esc to interrupt") || strings.Contains(low, "ctrl+c to interrupt") ||
		strings.Contains(low, "? for shortcuts") || strings.Contains(low, "auto-compact") ||
		strings.Contains(low, "context left") {
		return true
	}
	if (strings.HasPrefix(low, "thinking") || strings.HasPrefix(low, "working") || strings.HasPrefix(low, "loading")) &&
		len(ln) <= 24 && (strings.HasSuffix(ln, "…") || strings.HasSuffix(ln, "...")) {
		return true
	}
	trimmed := strings.TrimSpace(ln)
	switch trimmed {
	case "❯", ">", "›", "$", "~", "▼", "▲", "»", "→":
		return true
	}
	if strings.HasPrefix(trimmed, "» ") { // codex input placeholder
		return true
	}
	return false
}

// barrierLine reports a row that is mostly box/block-drawing glyphs — the
// rules, input-box borders, and half-block dividers agent TUIs draw around
// their own chrome — even when a label is embedded ("╭── title ──╮",
// "─ Worked for 11m ────"). Content rows with a few borders don't qualify.
func barrierLine(ln string) bool {
	boxy, text := 0, 0
	for _, r := range ln {
		switch {
		case r == ' ' || r == '\t':
		case strings.ContainsRune("─━│┃┄┅┈┉╌╍╭╮╰╯┌┐└┘├┤┬┴┼═║╔╗╚╝╠╣╦╩╬▀▄█▁▔▕▏", r):
			boxy++
		default:
			text++
		}
	}
	return boxy >= 4 && 2*boxy >= 3*text
}

// footerHint matches agent bottom-bar residue: keybinding cheat rows, token
// meters, model banners. Only applied while peeling the tail of a pane, so
// broad keywords are safe.
func footerHint(ln string) bool {
	low := strings.ToLower(ln)
	for _, kw := range []string{
		"ctrl+", "shift+tab", "for shortcuts", "tokens", "auto-compact",
		"context left", "add a follow-up", "esc to", "enter to send",
		"[default]", "to expand", "always-approve",
	} {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

// dropAgentChrome cuts off the agent CLI's own input box and bottom bars.
// Every agent TUI (pi/grok/codex/agy/cursor…) draws its input area under a
// rule or box border at the very bottom of the pane, so everything at or
// below the last barrier row is the agent's chrome, not its output. After the
// cut, trailing hint/blank/barrier rows are peeled until real content shows.
func dropAgentChrome(lines []string) []string {
	cut := -1
	for i, ln := range lines {
		if barrierLine(strings.TrimSpace(ln)) {
			cut = i
		}
	}
	// Input boxes hug the bottom of the pane; a lone divider higher up is
	// probably real output (shell panes), so leave those alone.
	if cut >= 0 && cut >= len(lines)-12 {
		lines = lines[:cut]
	}
	for len(lines) > 0 {
		ln := strings.TrimSpace(lines[len(lines)-1])
		if ln != "" && !barrierLine(ln) && !looksLikeChrome(ln) && !footerHint(ln) {
			break
		}
		lines = lines[:len(lines)-1]
	}
	return lines
}
