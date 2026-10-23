package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/neyham/herdr-paddock/internal/model"
)

// The test binary doubles as a fake herdr CLI (same trick as the ui tests):
// it logs each call and answers the way herdr 0.9 does, so the real
// subprocess path runs without touching a live session.
func TestMain(m *testing.M) {
	if os.Getenv("PADDOCK_FAKE_HERDR") != "1" {
		os.Exit(m.Run())
	}
	args := os.Args[1:]
	if f, err := os.OpenFile(os.Getenv("PADDOCK_FAKE_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		_ = json.NewEncoder(f).Encode(args)
		_ = f.Close()
	}
	call := strings.Join(args[:min(2, len(args))], " ")
	switch {
	case call == "api snapshot":
		raw, err := os.ReadFile(os.Getenv("PADDOCK_FAKE_SNAPSHOT"))
		if err != nil {
			os.Exit(2)
		}
		os.Stdout.Write(raw)
	case call == "agent prompt" && len(args) == 4:
		switch args[2] {
		case "blocked":
			fmt.Println(`{"error":{"code":"agent_blocked","message":"agent is blocked"},"id":"cli:agent:prompt"}`)
			os.Exit(1)
		case "missing":
			fmt.Println(`{"error":{"code":"agent_not_found","message":"agent target missing not found"},"id":"cli:agent:prompt"}`)
			os.Exit(1)
		}
		fmt.Println(`{"id":"cli:agent:prompt","result":{"type":"ok"}}`)
	case call == "pane send-text" && len(args) == 4, call == "pane send-keys" && len(args) >= 4:
		fmt.Println(`{"id":"cli:request","result":{"type":"ok"}}`)
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

// fakeHerdr points calls at the test binary and returns a reader for the
// commands it received.
func fakeHerdr(t *testing.T) (string, func() [][]string) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv("PADDOCK_FAKE_HERDR", "1")
	t.Setenv("PADDOCK_FAKE_LOG", log)
	return os.Args[0], func() [][]string {
		raw, _ := os.ReadFile(log)
		var calls [][]string
		for _, ln := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			var c []string
			if json.Unmarshal([]byte(ln), &c) == nil {
				calls = append(calls, c)
			}
		}
		return calls
	}
}

// Contract test against a herdr 0.9 session snapshot. agents[] omits
// terminal_title_stripped when the title is empty.
func TestListHerdr09Snapshot(t *testing.T) {
	bin, _ := fakeHerdr(t)
	t.Setenv("PADDOCK_FAKE_SNAPSHOT", filepath.Join("testdata", "snapshot-0.9.json"))
	rows, ws, err := List(bin)
	if err != nil {
		t.Fatal(err)
	}
	if ws != 1 || len(rows) != 3 {
		t.Fatalf("ws=%d rows=%+v", ws, rows)
	}
	p1, p2, notes := rows[0], rows[1], rows[2]
	if p1.PaneID != "w1:p1" || p1.Status != "blocked" || p1.Revision != 41 || p1.StateSeq != 7 {
		t.Fatalf("p1 = %+v", p1)
	}
	if !strings.Contains(p1.Title, "Pick a payment provider") {
		t.Fatalf("title = %q", p1.Title)
	}
	if p2.PaneID != "w1:p2" || p2.Title != "" || p2.Cwd != "/home/dev/shop/api" {
		t.Fatalf("p2 = %+v", p2)
	}
	if notes.PaneID != "" || notes.TabLabel != "notes" {
		t.Fatalf("non-agent tab = %+v", notes)
	}
}

func TestPromptAnswersBlockedAgent(t *testing.T) {
	cases := []struct {
		name, target, text string
		want               [][]string
	}{
		{"idle agent gets a normal prompt", "idle", "ship it", [][]string{
			{"agent", "prompt", "idle", "ship it"},
		}},
		{"blocked agent: text then enter", "blocked", "go tonight", [][]string{
			{"agent", "prompt", "blocked", "go tonight"},
			{"pane", "send-text", "blocked", "go tonight"},
			{"pane", "send-keys", "blocked", "enter"},
		}},
		{"blocked agent: one key picks an option, no enter", "blocked", "2", [][]string{
			{"agent", "prompt", "blocked", "2"},
			{"pane", "send-text", "blocked", "2"},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bin, calls := fakeHerdr(t)
			if err := Prompt(bin, c.target, c.text); err != nil {
				t.Fatal(err)
			}
			if got := calls(); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("calls = %q, want %q", got, c.want)
			}
		})
	}
}

// herdr reports errors as JSON on stdout with an empty stderr; the user must
// see herdr's message, not "exit status 1".
func TestRunSurfacesHerdrError(t *testing.T) {
	bin, calls := fakeHerdr(t)
	err := Prompt(bin, "missing", "hello")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "agent_not_found" || err.Error() != "agent target missing not found" {
		t.Fatalf("err = %v", err)
	}
	if n := len(calls()); n != 1 {
		t.Fatalf("non-blocked failure must not fall back to send-text, got %d calls", n)
	}
}

func TestSnapshotKeepsAllSiblingAgents(t *testing.T) {
	cases := []struct {
		name string
		a, b agent
	}{
		{"blocked and focused working", agent{PaneID: "blocked", AgentStatus: "blocked"}, agent{PaneID: "working", AgentStatus: "working", Focused: true}},
		{"both blocked", agent{PaneID: "a", AgentStatus: "blocked"}, agent{PaneID: "b", AgentStatus: "blocked", Focused: true}},
		{"both working", agent{PaneID: "a", AgentStatus: "working"}, agent{PaneID: "b", AgentStatus: "working"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.a.TabID, c.b.TabID = "tab", "tab"
			c.a.Agent, c.b.Agent = "codex", "claude"
			order := map[string]int{c.a.PaneID: 0, c.b.PaneID: 1}
			for _, agents := range [][]agent{{c.a, c.b}, {c.b, c.a}} {
				rows, _ := snapshotRows(snapshot{Tabs: []tab{{TabID: "tab"}}, Agents: agents})
				feed := model.FeedTabs(rows, "", "", order)
				if len(feed) != 2 || feed[0].PaneID != c.a.PaneID || feed[1].PaneID != c.b.PaneID {
					t.Fatalf("sibling missing or unstable order: %+v", feed)
				}
				if feed[0].Agent != c.a.Agent || feed[0].Status != c.a.AgentStatus || feed[1].Agent != c.b.Agent || feed[1].Status != c.b.AgentStatus {
					t.Fatalf("metadata mixed between siblings: %+v", feed)
				}
			}
		})
	}
}

func TestSanitizeStripsTerminalInjection(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"sgr colors":                  {"\x1b[32mgreen\x1b[0m text", "green text"},
		"non-m csi does not eat text": {"\x1b[2K hello message", " hello message"},
		"cursor and clear":            {"a\x1b[6nb\x1b[2Jc\x1b[10;20Hd", "abcd"},
		"osc52 clipboard":             {"safe\x1b]52;c;aGVsbG8=\x07after", "safeafter"},
		"osc with st terminator":      {"x\x1b]0;evil title\x1b\\y", "xy"},
		"dcs":                         {"x\x1bPq#0;2;0;0;0#0~~\x1b\\y", "xy"},
		"two byte escape":             {"x\x1b(By", "xy"},
		"c1 csi":                      {"x\u009b31mred", "xred"},
		"bel and c0":                  {"a\x07b\x08c\rd", "abcd"},
		"keeps newline and tab":       {"line1\n\tline2", "line1\n\tline2"},
		"unterminated osc":            {"x\x1b]52;c;steal", "x"},
		"plain":                       {"nothing to do 咩", "nothing to do 咩"},
	}
	for name, c := range cases {
		if got := Sanitize(c.in); got != c.want {
			t.Fatalf("%s: Sanitize(%q) = %q, want %q", name, c.in, got, c.want)
		}
	}
}

func TestOneLineFlattensTitles(t *testing.T) {
	if got := oneLine("evil\x1b]2;t\x07 title\nwith\nnewlines"); got != "evil title with newlines" {
		t.Fatalf("got %q", got)
	}
}

func TestShortErrIsRuneSafe(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "错"
	}
	got := shortErr(long)
	if len([]rune(got)) != 80 {
		t.Fatalf("want 80 runes, got %d", len([]rune(got)))
	}
	for _, r := range got {
		if r == '\uFFFD' {
			t.Fatal("shortErr split a rune")
		}
	}
}

func TestListLive(t *testing.T) {
	tabs, ws, err := List("herdr")
	if err != nil {
		t.Skipf("herdr not running: %v", err)
	}
	if ws == 0 || len(tabs) == 0 {
		t.Fatalf("empty snapshot ws=%d tabs=%d", ws, len(tabs))
	}
	foundID := false
	for _, tab := range tabs {
		if tab.TabID != "" {
			foundID = true
			break
		}
	}
	if !foundID {
		t.Fatal("no tab ids")
	}
}
