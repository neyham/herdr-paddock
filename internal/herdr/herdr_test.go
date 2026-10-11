package herdr

import (
	"testing"

	"github.com/neyham/herdr-paddock/internal/model"
)

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
