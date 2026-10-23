package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/neyham/herdr-paddock/internal/model"
)

type envelope struct {
	Result json.RawMessage `json:"result"`
}

type snapshotResult struct {
	Type     string   `json:"type"`
	Snapshot snapshot `json:"snapshot"`
}

type snapshot struct {
	Workspaces []workspace `json:"workspaces"`
	Tabs       []tab       `json:"tabs"`
	Agents     []agent     `json:"agents"`
}

type workspace struct {
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}

type tab struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	Focused     bool   `json:"focused"`
	AgentStatus string `json:"agent_status"`
}

type agent struct {
	TabID                 string `json:"tab_id"`
	Agent                 string `json:"agent"`
	AgentStatus           string `json:"agent_status"`
	Cwd                   string `json:"cwd"`
	ForegroundCwd         string `json:"foreground_cwd"`
	PaneID                string `json:"pane_id"`
	TerminalTitleStripped string `json:"terminal_title_stripped"`
	Focused               bool   `json:"focused"`
	Revision              int    `json:"revision"`
	StateChangeSeq        int    `json:"state_change_seq"`
}

func List(bin string) ([]model.TabRow, int, error) {
	if bin == "" {
		bin = "herdr"
	}
	raw, err := run(bin, 2*time.Second, "api", "snapshot")
	if err != nil {
		return nil, 0, err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, 0, fmt.Errorf("herdr json: %w", err)
	}
	var res snapshotResult
	if err := json.Unmarshal(env.Result, &res); err != nil {
		return nil, 0, fmt.Errorf("herdr snapshot: %w", err)
	}
	rows, workspaces := snapshotRows(res.Snapshot)
	return rows, workspaces, nil
}

func snapshotRows(s snapshot) ([]model.TabRow, int) {
	wsName := map[string]string{}
	for _, w := range s.Workspaces {
		wsName[w.WorkspaceID] = w.Label
	}
	// Keep every agent pane. Collapsing a split tab to one representative
	// makes live reply targets disappear whenever a sibling changes status.
	byTab := map[string][]agent{}
	for _, a := range s.Agents {
		byTab[a.TabID] = append(byTab[a.TabID], a)
	}
	rows := make([]model.TabRow, 0, len(s.Agents))
	for _, t := range s.Tabs {
		panes := byTab[t.TabID]
		if len(panes) == 0 {
			panes = []agent{{}} // retain non-agent tabs for session metadata
		}
		for _, a := range panes {
			status := a.AgentStatus
			if status == "" {
				status = t.AgentStatus
			}
			cwd := a.ForegroundCwd
			if cwd == "" {
				cwd = a.Cwd
			}
			rows = append(rows, model.TabRow{
				TabID:       t.TabID,
				WorkspaceID: t.WorkspaceID,
				WSLabel:     oneLine(wsName[t.WorkspaceID]),
				TabLabel:    oneLine(t.Label),
				Agent:       oneLine(a.Agent),
				Status:      status,
				Title:       oneLine(a.TerminalTitleStripped),
				Cwd:         oneLine(cwd),
				PaneID:      a.PaneID,
				Focused:     a.Focused || (len(panes) == 1 && t.Focused),
				Revision:    a.Revision,
				StateSeq:    a.StateChangeSeq,
			})
		}
	}
	return rows, len(s.Workspaces)
}

func Focus(bin, workspaceID, tabID, paneID string) error {
	if bin == "" {
		bin = "herdr"
	}
	if tabID == "" && paneID == "" {
		return fmt.Errorf("no tab")
	}
	if workspaceID != "" {
		_, _ = run(bin, 2*time.Second, "workspace", "focus", workspaceID)
	}
	if tabID != "" {
		if _, err := run(bin, 3*time.Second, "tab", "focus", tabID); err != nil {
			return err
		}
	}
	if paneID != "" {
		_, _ = run(bin, 2*time.Second, "agent", "focus", paneID)
	}
	return nil
}

func ReadRecent(bin, target string, lines int) (string, error) {
	if bin == "" {
		bin = "herdr"
	}
	if target == "" {
		return "", fmt.Errorf("no pane")
	}
	if lines <= 0 {
		lines = 40
	}
	out, err := run(bin, 3*time.Second, "agent", "read", target, "--source", "recent-unwrapped", "--lines", fmt.Sprintf("%d", lines), "--format", "text")
	if err != nil {
		out, err = run(bin, 3*time.Second, "pane", "read", target, "--source", "recent", "--lines", fmt.Sprintf("%d", lines), "--format", "text")
		if err != nil {
			return "", err
		}
	}
	return Sanitize(string(out)), nil
}

// ReadMany pulls recent text for several panes at once. Failed reads are
// omitted from the result (so the caller retries them later); a present
// empty string means the pane really is empty.
func ReadMany(bin string, targets []string, lines int) map[string]string {
	out := make(map[string]string, len(targets))
	if len(targets) == 0 {
		return out
	}
	type item struct {
		id, text string
		ok       bool
	}
	ch := make(chan item, len(targets))
	for _, id := range targets {
		id := id
		go func() {
			text, err := ReadRecent(bin, id, lines)
			ch <- item{id: id, text: text, ok: err == nil}
		}()
	}
	for range targets {
		it := <-ch
		if it.ok {
			out[it.id] = it.text
		}
	}
	return out
}

func Prompt(bin, target, text string) error {
	if bin == "" {
		bin = "herdr"
	}
	if target == "" {
		return fmt.Errorf("no pane")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("empty")
	}
	_, err := run(bin, 8*time.Second, "agent", "prompt", target, text)
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Code == "agent_blocked" {
		return answerBlocked(bin, target, text)
	}
	return err
}

// answerBlocked types a reply into an approval or question dialog. herdr's
// agent prompt refuses blocked agents, but answering that dialog is exactly
// what a glance reply is for, so the text goes in as keystrokes instead. A
// single character is one keypress — option menus act on it immediately, and
// a trailing Enter could confirm whatever dialog comes next. Anything longer
// is submitted with Enter, like typing it into the pane.
func answerBlocked(bin, target, text string) error {
	if _, err := run(bin, 3*time.Second, "pane", "send-text", target, text); err != nil {
		return err
	}
	if len([]rune(text)) == 1 {
		return nil
	}
	_, err := run(bin, 3*time.Second, "pane", "send-keys", target, "enter")
	return err
}

// Sanitize removes terminal escape sequences and control characters from
// text that came out of another agent's pane. That content is untrusted —
// agents read web pages and third-party repos all day — and a crafted
// OSC 52 or CSI sequence that reached our TTY could rewrite the clipboard
// or wreck the screen of whoever is SSH'd in. Complete ESC-introduced
// sequences (CSI, OSC, DCS, SOS, PM, APC, two-byte escapes) are dropped
// whole; C0/C1 control bytes are dropped except newline and tab.
func Sanitize(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool {
		return (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f || (r >= 0x80 && r <= 0x9f)
	}) {
		return s
	}
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == 0x1b:
			i = skipEscape(rs, i)
		case r == 0x9b: // C1 CSI
			i = skipCSI(rs, i)
		case r == 0x9d, r == 0x90, r == 0x98, r == 0x9e, r == 0x9f: // C1 OSC/DCS/SOS/PM/APC
			i = skipToST(rs, i)
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			// other C0/C1 controls: dropped
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// skipEscape consumes one ESC-introduced sequence starting at rs[i] == ESC
// and returns the index of its last rune.
func skipEscape(rs []rune, i int) int {
	if i+1 >= len(rs) {
		return len(rs)
	}
	switch rs[i+1] {
	case '[':
		return skipCSI(rs, i+1)
	case ']', 'P', 'X', '^', '_':
		return skipToST(rs, i+1)
	default:
		// two-byte escape, possibly with 0x20-0x2f intermediates (ESC ( B)
		j := i + 1
		for j < len(rs) && rs[j] >= 0x20 && rs[j] <= 0x2f {
			j++
		}
		if j >= len(rs) {
			return len(rs)
		}
		return j
	}
}

// skipCSI consumes a CSI body starting after the introducer at rs[i] and
// returns the index of the final byte (0x40-0x7e).
func skipCSI(rs []rune, i int) int {
	j := i + 1
	for j < len(rs) {
		if rs[j] >= 0x40 && rs[j] <= 0x7e {
			return j
		}
		if rs[j] >= 0x20 && rs[j] <= 0x3f {
			j++
			continue
		}
		return j - 1 // malformed: stop before the stray rune so it is re-examined
	}
	return len(rs)
}

// skipToST consumes an OSC/DCS/SOS/PM/APC body starting at rs[i] and returns
// the index of its terminator (BEL, or the backslash of ESC \).
func skipToST(rs []rune, i int) int {
	for j := i + 1; j < len(rs); j++ {
		if rs[j] == 0x07 || rs[j] == 0x9c { // BEL or C1 ST
			return j
		}
		if rs[j] == 0x1b {
			if j+1 < len(rs) && rs[j+1] == '\\' {
				return j + 1
			}
			return j - 1 // new escape starts: hand it back to the main loop
		}
	}
	return len(rs)
}

// oneLine sanitizes a short identifier-ish string (title, label, cwd) and
// flattens any whitespace runs to single spaces.
func oneLine(s string) string {
	return strings.Join(strings.Fields(Sanitize(s)), " ")
}

// stdout from herdr is bounded: agent panes can emit arbitrarily long lines
// and a runaway read must not balloon the TUI's memory.
const maxRunOutput = 4 << 20

type cappedBuilder struct {
	b strings.Builder
}

func (w *cappedBuilder) Write(p []byte) (int, error) {
	if room := maxRunOutput - w.b.Len(); room > 0 {
		if len(p) > room {
			w.b.Write(p[:room])
		} else {
			w.b.Write(p)
		}
	}
	return len(p), nil
}

// Error is a failure herdr reported in its JSON envelope.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	return shortErr(e.Message)
}

// apiError extracts {"error":{"code","message"}} from herdr's output; herdr
// prints it on stdout and leaves stderr empty.
func apiError(out string) *Error {
	var env struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &env) != nil || env.Error == nil || env.Error.Code == "" {
		return nil
	}
	msg := env.Error.Message
	if msg == "" {
		msg = env.Error.Code
	}
	return &Error{Code: env.Error.Code, Message: msg}
}

func run(bin string, timeout time.Duration, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	var stdout, stderr cappedBuilder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("no-herdr")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("timeout")
	case err := <-done:
		if err != nil {
			if apiErr := apiError(stdout.b.String()); apiErr != nil {
				if noHerdr(apiErr.Message) {
					return nil, fmt.Errorf("no-herdr")
				}
				return nil, apiErr
			}
			msg := strings.TrimSpace(stderr.b.String())
			if msg == "" {
				msg = err.Error()
			}
			if noHerdr(msg) {
				return nil, fmt.Errorf("no-herdr")
			}
			return nil, fmt.Errorf("%s", shortErr(msg))
		}
	}
	return []byte(stdout.b.String()), nil
}

func noHerdr(msg string) bool {
	return strings.Contains(msg, "not running") || strings.Contains(msg, "connection refused")
}

func shortErr(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 80 {
		return string(r[:80])
	}
	return s
}
