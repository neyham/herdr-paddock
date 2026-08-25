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
	reFooterUp   = regexp.MustCompile(`[↑↓]\s*\d`)
	reCostPct    = regexp.MustCompile(`\$\d+(\.\d+)?|\d+(\.\d+)?%/`)
	reWorkedFor  = regexp.MustCompile(`(?i)worked for\s+\d`)
	reCwdBranch  = regexp.MustCompile(`(?i)(?:^|[\s])(?:~/|/|[a-zA-Z]:\\)\S.*\s·\s+[A-Za-z0-9._/-]+$`)
	reAutoStatus = regexp.MustCompile(`(?i)^auto\s·`)
	reTaskCount  = regexp.MustCompile(`(?i)^\d+\s+tasks?$`)
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
	// piwi-tui pet widget footer (pi plugin) - stable patterns from faceFor().
	// Stat rows always carry numeric gauges or bar glyphs, so require one
	// before the keywords count as chrome; prose that merely mentions these
	// words stays visible on the card.
	if strings.ContainsAny(ln, "0123456789█▓▒░▁▂▃▄▅▆▇") {
		if strings.Contains(low, "fullness") && strings.Contains(low, "joy") && strings.Contains(low, "energy") {
			return true
		}
		if strings.Contains(low, "sparks") && strings.Contains(low, "nook") {
			return true
		}
	}
	// pet ASCII art: faceFor() generates these across all moods/stages/accessories
	if strings.Contains(ln, "/\\___/\\") || strings.Contains(ln, "/\\_/\\") || strings.Contains(ln, "\\*___*/") || strings.Contains(ln, "/\\_+_/\\") || strings.Contains(ln, "~\\___/~") || strings.Contains(ln, "/\\_^_/\\") || strings.Contains(ln, "~\\_/\\") {
		if strings.Contains(ln, "(o.o)") || strings.Contains(ln, "(-.-)") || strings.Contains(ln, "(o.-)") || strings.Contains(ln, "(O.o)") || strings.Contains(ln, "(o.O)") || strings.Contains(ln, "(^.^)") || strings.Contains(ln, "(._.)") || strings.Contains(ln, "(u.u)") {
			return true
		}
		if strings.Contains(ln, "/ > <\\") || strings.Contains(ln, "< ^ >") || strings.Contains(ln, "/| |\\") {
			return true
		}
		// head-only line with status: /\___/\  name · Lv N · activity · wearing X
		if strings.Contains(ln, " · Lv ") && strings.Contains(ln, "wearing") {
			return true
		}
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
	t := strings.TrimSpace(ln)
	low := strings.ToLower(t)
	for _, kw := range []string{
		"ctrl+", "shift+tab", "for shortcuts", "tokens", "auto-compact",
		"context left", "add a follow-up", "esc to", "enter to send",
		"[default]", "to expand", "always-approve", "run everything",
		"files edited",
	} {
		if strings.Contains(low, kw) {
			return true
		}
	}
	if reWorkedFor.MatchString(ln) {
		return true
	}
	if reAutoStatus.MatchString(strings.TrimSpace(ln)) {
		return true
	}
	if reTaskCount.MatchString(t) {
		return true
	}
	return false
}

func tableRule(ln string) bool {
	t := strings.TrimSpace(ln)
	if t == "" {
		return false
	}
	r := []rune(t)[0]
	return r == '├' || r == '┤' || strings.ContainsRune(t, '┼')
}

// inputFrameLine is a TUI input-box border near the pane tail: corner-framed
// rows (even with a long title), half-block rules, or a ratio barrier that is
// not a markdown/table divider.
func inputFrameLine(ln string) bool {
	t := strings.TrimSpace(ln)
	if t == "" || tableRule(t) {
		return false
	}
	rs := []rune(t)
	first, last := rs[0], rs[len(rs)-1]
	if (first == '╭' || first == '┌') && (last == '╮' || last == '┐') {
		return true
	}
	if (first == '╰' || first == '└') && (last == '╯' || last == '┘') {
		return true
	}
	return barrierLine(t)
}

func cwdBranchLine(ln string) bool {
	return reCwdBranch.MatchString(strings.TrimSpace(ln))
}

func cursorFooterContext(lines []string) bool {
	seen := 0
	for i := len(lines) - 1; i >= 0 && seen < 8; i-- {
		ln := strings.TrimSpace(lines[i])
		if ln == "" {
			continue
		}
		seen++
		low := strings.ToLower(ln)
		if strings.Contains(low, "run everything") || strings.Contains(low, "files edited") ||
			strings.Contains(low, "add a follow-up") || reAutoStatus.MatchString(ln) {
			return true
		}
	}
	return false
}

func peelableChrome(ln string, cursorCtx bool) bool {
	t := strings.TrimSpace(ln)
	if t == "" || inputFrameLine(t) || barrierLine(t) || looksLikeChrome(t) || footerHint(t) {
		return true
	}
	if usefulLine(ln) == "" {
		return true
	}
	return cursorCtx && cwdBranchLine(t)
}

func frameHasChromeTail(lines []string, frame int, cursorCtx bool) bool {
	sawChrome := false
	for _, ln := range lines[frame+1:] {
		if peelableChrome(ln, cursorCtx) {
			sawChrome = true
			continue
		}
		// A meaningful line after a frame means that frame may belong to the
		// agent's actual answer. Cutting there would truncate the answer.
		return false
	}
	return sawChrome
}

// dropAgentChrome removes an agent CLI's input box and bottom bars when the
// tail is recognizable as chrome. A frame inside the answer is retained when
// meaningful text follows it; after a confirmed cut, trailing hints and bars
// are peeled until real content shows.
func dropAgentChrome(lines []string) []string {
	frameCtx := cursorFooterContext(lines)
	cut := -1
	for i, ln := range lines {
		if inputFrameLine(ln) && frameHasChromeTail(lines, i, frameCtx) {
			cut = i
			break
		}
	}
	// Only cut at a frame when everything after it is known chrome. A
	// decorative box in the agent's answer can sit near the tail too; cutting
	// blindly at the last barrier used to discard the answer after that box.
	if cut >= 0 {
		lines = lines[:cut]
	}
	// Cursor's cwd/branch footer is only chrome when the footer markers are
	// still present. Recompute after the frame cut so an answer line with the
	// same shape is not peeled from the new tail.
	ctx := cursorFooterContext(lines)
	for len(lines) > 0 {
		if !peelableChrome(lines[len(lines)-1], ctx) {
			break
		}
		lines = lines[:len(lines)-1]
	}
	return lines
}
