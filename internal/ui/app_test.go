package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/neyham/herdr-paddock/internal/model"
)

// Use this test executable as a portable mock CLI. Commands exercise the real
// subprocess path without ever sending a prompt to a running herdr instance.
func TestMain(m *testing.M) {
	if os.Getenv("PADDOCK_TEST_HERDR") == "1" {
		args := os.Args[1:]
		if len(args) == 2 && args[0] == "api" && args[1] == "snapshot" {
			fmt.Print(os.Getenv("PADDOCK_TEST_SNAPSHOT"))
			os.Exit(0)
		}
		if len(args) >= 3 && args[0] == "agent" && args[1] == "prompt" {
			if len(args) != 4 {
				os.Exit(2)
			}
			_ = json.NewEncoder(os.Stdout).Encode(args)
			os.Exit(0)
		}
		if len(args) >= 3 && args[1] == "read" {
			if args[2] == os.Getenv("PADDOCK_TEST_FAIL_PANE") {
				os.Exit(1)
			}
			fmt.Println("output from " + args[2])
			os.Exit(0)
		}
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func mockHerdr(t *testing.T) string {
	t.Helper()
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PADDOCK_TEST_HERDR", "1")
	return bin
}

func applyKey(m Model, key string) Model {
	next, _ := m.Update(keyMsg(key))
	return next.(Model)
}

func refreshSiblingSnapshot(t *testing.T, m Model, siblingStatus string, keepA bool) Model {
	t.Helper()
	agents := []map[string]any{
		{"tab_id": "shared", "pane_id": "pane-B", "agent": "claude", "agent_status": siblingStatus, "terminal_title_stripped": "Sibling B"},
	}
	if keepA {
		agents = append(agents, map[string]any{"tab_id": "shared", "pane_id": "pane-A", "agent": "codex", "agent_status": "working", "terminal_title_stripped": "Original A", "focused": true})
	}
	raw, err := json.Marshal(map[string]any{"result": map[string]any{"type": "session_snapshot", "snapshot": map[string]any{
		"tabs": []map[string]any{{"tab_id": "shared", "label": "split-tab"}}, "agents": agents,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PADDOCK_TEST_SNAPSHOT", string(raw))
	msg := fetchHerdr(m.bin)().(herdrMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestSiblingStatusChangeKeepsPinnedReplyTarget(t *testing.T) {
	for _, detail := range []bool{false, true} {
		t.Run(fmt.Sprintf("detail=%v", detail), func(t *testing.T) {
			m := New()
			m.selfTab, m.selfPane, m.bin = "", "", mockHerdr(t)
			m = refreshSiblingSnapshot(t, m, "idle", true)
			if detail {
				m = applyKey(m, "enter")
			}
			m = applyKey(applyKey(m, "i"), "continue original task")
			m = refreshSiblingSnapshot(t, m, "blocked", true)
			if !m.paneAlive("pane-A") {
				t.Error("live pane A disappeared when sibling B became blocked")
			}
			if detail {
				row := m.detailRow()
				if row == nil || row.PaneID != "pane-A" || row.Title != "Original A" || row.Status != "working" {
					t.Errorf("detail metadata drifted from pinned transcript: %+v", row)
				}
			}
			next, cmd := m.Update(keyMsg("enter"))
			if cmd == nil {
				t.Fatalf("reply rejected: %s", next.(Model).actionNote)
			}
			msg := cmd().(sendMsg)
			if msg.err != nil || msg.pane != "pane-A" || msg.text != "continue original task" {
				t.Fatalf("wrong delivery after sibling status change: %+v", msg)
			}
		})
	}
}

func TestSiblingStatusChangeKeepsDraftReachable(t *testing.T) {
	m := New()
	m.selfTab, m.selfPane, m.bin = "", "", mockHerdr(t)
	m = refreshSiblingSnapshot(t, m, "idle", true)
	m = applyKey(applyKey(m, "i"), "unsent draft for A")
	m = applyKey(m, "esc")
	m = refreshSiblingSnapshot(t, m, "blocked", true)
	feed := feedTabs(m)
	findPane := func(pane string) int {
		for i, row := range feed {
			if row.PaneID == pane {
				return i
			}
		}
		t.Fatalf("pane %s is no longer reachable in the feed", pane)
		return -1
	}
	m.moveTo(findPane("pane-B"))
	m = applyKey(m, "i")
	if m.input.Value() != "" {
		t.Fatal("A draft leaked into sibling B")
	}
	m = applyKey(m, "esc")
	m.moveTo(findPane("pane-A"))
	m = applyKey(m, "i")
	if m.input.Value() != "unsent draft for A" {
		t.Fatalf("A draft lost: %q", m.input.Value())
	}
}

func TestDetailFlipsBetweenSiblingPanes(t *testing.T) {
	m := New()
	m.selfTab, m.selfPane, m.bin = "", "", mockHerdr(t)
	m = refreshSiblingSnapshot(t, m, "blocked", true)
	m = applyKey(applyKey(m, "enter"), "i") // blocked B is first
	m = applyKey(applyKey(m, "draft B"), "esc")
	m = applyKey(m, "j")
	if row := m.detailRow(); row == nil || row.PaneID != "pane-A" || m.input.Value() != "" {
		t.Fatalf("flip within one tab did not reach A with its own draft: row=%+v draft=%q", row, m.input.Value())
	}
	m = applyKey(applyKey(m, "i"), "draft A")
	m = applyKey(applyKey(m, "esc"), "k")
	if row := m.detailRow(); row == nil || row.PaneID != "pane-B" || m.input.Value() != "draft B" {
		t.Fatalf("flip back lost B or its draft: row=%+v draft=%q", row, m.input.Value())
	}
	plain := stripANSI(render(m))
	if !strings.Contains(plain, "Sibling B") || !strings.Contains(plain, "1/2") {
		t.Fatalf("detail header/counter mixed siblings:\n%s", plain)
	}
}

func TestTapOnSiblingStaysPinnedAcrossReorder(t *testing.T) {
	m := New()
	m.selfTab, m.selfPane, m.bin = "", "", mockHerdr(t)
	m = refreshSiblingSnapshot(t, m, "idle", true)
	next, _ := m.Update(mouseMsg(2, 2, tea.MouseActionPress, tea.MouseButtonLeft))
	m = next.(Model)
	m = refreshSiblingSnapshot(t, m, "blocked", true) // B now occupies the first slot
	next, _ = m.Update(mouseMsg(2, 2, tea.MouseActionRelease, tea.MouseButtonLeft))
	m = next.(Model)
	if !m.detailOpen || m.transcriptOf != "pane-A" {
		t.Fatalf("tap changed targets during sibling reorder: open=%v pane=%s", m.detailOpen, m.transcriptOf)
	}
}

func TestWallComposeLabelsDepartedPane(t *testing.T) {
	m := New()
	m.width, m.height = 80, 24
	m.selfTab, m.selfPane, m.bin = "", "", mockHerdr(t)
	m = refreshSiblingSnapshot(t, m, "idle", true)
	m = applyKey(m, "i")
	m = refreshSiblingSnapshot(t, m, "blocked", false)
	plain := stripANSI(render(m))
	if !strings.Contains(plain, "agent left") || strings.Contains(plain, "reply Sibling B") {
		t.Fatalf("departed reply target is mislabeled:\n%s", plain)
	}
}

func TestRemovedPaneIsNotMistakenForSurvivingSibling(t *testing.T) {
	for _, detail := range []bool{false, true} {
		t.Run(fmt.Sprintf("detail=%v", detail), func(t *testing.T) {
			m := New()
			m.selfTab, m.selfPane, m.bin = "", "", mockHerdr(t)
			m = refreshSiblingSnapshot(t, m, "idle", true)
			if detail {
				m = applyKey(m, "enter")
			}
			m = applyKey(applyKey(m, "i"), "only for A")
			m = refreshSiblingSnapshot(t, m, "blocked", false)
			next, cmd := m.Update(keyMsg("enter"))
			m = next.(Model)
			if cmd != nil || m.input.Value() != "only for A" {
				t.Fatal("removed pane's reply was rerouted or its draft discarded")
			}
			if m.paneAlive("pane-A") || !m.paneAlive("pane-B") {
				t.Fatal("pane liveness conflated siblings")
			}
			if detail && m.detailRow() != nil {
				t.Fatalf("closed A detail borrowed B metadata: %+v", m.detailRow())
			}
		})
	}
}

func TestMouseComposeSendsToSelectedPane(t *testing.T) {
	for _, previous := range []bool{false, true} {
		t.Run(fmt.Sprintf("previous-compose=%v", previous), func(t *testing.T) {
			m := sampleModel(80, 24)
			m.bin = mockHerdr(t)
			m.selID = "p2"
			if previous {
				m = applyKey(applyKey(m, "i"), "esc")
			}
			m.moveTo(1) // hooks / p1, rather than pay / p2
			for _, action := range []tea.MouseAction{tea.MouseActionPress, tea.MouseActionRelease} {
				next, _ := m.Update(mouseMsg(2, m.height-2, action, tea.MouseButtonLeft))
				m = next.(Model)
			}
			m = applyKey(m, "approve hooks")
			next, cmd := m.Update(keyMsg("enter"))
			if cmd == nil {
				t.Fatalf("mouse compose could not send: %s", next.(Model).actionNote)
			}
			sent := cmd().(sendMsg)
			if sent.err != nil || sent.pane != "p1" || sent.text != "approve hooks" {
				t.Fatalf("wrong delivery: %+v", sent)
			}
		})
	}
}

func TestDraftsStayWithPaneAcrossWallAndDetail(t *testing.T) {
	m := sampleModel(80, 24)
	m.selID = "p2"
	m = applyKey(applyKey(m, "i"), "draft for pay")
	m = applyKey(m, "esc")
	m.moveTo(1)
	m = applyKey(m, "i")
	if m.input.Value() != "" {
		t.Fatalf("pay draft leaked into hooks: %q", m.input.Value())
	}
	m = applyKey(applyKey(m, "draft for hooks"), "esc")
	m = applyKey(m, "enter") // open hooks detail
	if m.input.Value() != "draft for hooks" {
		t.Fatal("wall draft was not available in the same pane's detail")
	}
	m = applyKey(m, "k") // back to pay
	if m.input.Value() != "draft for pay" {
		t.Fatalf("pay draft not restored: %q", m.input.Value())
	}
	m = applyKey(applyKey(m, "i"), " updated")
	m = applyKey(applyKey(m, "esc"), "j")
	if m.input.Value() != "draft for hooks" {
		t.Fatalf("detail flip did not restore hooks: %q", m.input.Value())
	}
	m = applyKey(m, "k")
	if m.input.Value() != "draft for pay updated" {
		t.Fatalf("edited draft was lost: %q", m.input.Value())
	}
	m = applyKey(m, "esc") // return to the wall on pay
	if m.input.Value() != "draft for pay updated" || m.draftPane != "p2" {
		t.Fatal("closing detail changed draft ownership")
	}
}

func TestDetailDraftDoesNotFollowFlip(t *testing.T) {
	m := sampleModel(80, 24)
	m = applyKey(applyKey(m, "enter"), "i")
	m = applyKey(m, "approve payment provider")
	m = applyKey(applyKey(m, "esc"), "j")
	if m.input.Value() != "" || m.transcriptOf != "p1" {
		t.Fatalf("draft followed flip: pane=%s draft=%q", m.transcriptOf, m.input.Value())
	}
	m = applyKey(m, "k")
	if m.input.Value() != "approve payment provider" {
		t.Fatal("original draft was not retained")
	}
}

func TestFailedSendRestoresOnlyItsOwnDraft(t *testing.T) {
	m := sampleModel(80, 24)
	m = applyKey(applyKey(m, "i"), "draft for pay")
	m = applyKey(m, "enter") // command intentionally left in flight
	m = applyKey(m, "esc")
	m.moveTo(1)
	m = applyKey(applyKey(m, "i"), "draft for hooks")
	next, _ := m.Update(sendMsg{pane: "p2", text: "draft for pay", err: errors.New("offline")})
	m = next.(Model)
	if m.input.Value() != "draft for hooks" {
		t.Fatal("late failure overwrote another pane's draft")
	}
	m = applyKey(m, "esc")
	m.moveTo(0)
	m = applyKey(m, "i")
	if m.input.Value() != "draft for pay" {
		t.Fatalf("failed draft lost: %q", m.input.Value())
	}
	m = applyKey(m, " revised")
	next, _ = m.Update(sendMsg{pane: "p2", text: "old draft", err: errors.New("offline")})
	if next.(Model).input.Value() != "draft for pay revised" {
		t.Fatal("late failure overwrote newer edits")
	}
}

func TestSentDraftDoesNotReappear(t *testing.T) {
	m := sampleModel(80, 24)
	m = applyKey(applyKey(m, "i"), "send once")
	m = applyKey(m, "esc")
	m.moveTo(1)
	m.moveTo(0) // restore a previously saved draft before sending
	m = applyKey(applyKey(m, "i"), "enter")
	m = applyKey(m, "esc")
	m.moveTo(1)
	m.moveTo(0)
	if m.input.Value() != "" {
		t.Fatal("sent text reappeared after switching panes")
	}
}

func TestPreviewRejectsOutOfOrderCompletions(t *testing.T) {
	for _, oldRev := range []int{11, 12} {
		m := sampleModel(80, 24)
		next, _ := m.Update(previewsMsg{byPane: map[string]string{"p1": "new"}, revs: map[string]int{"p1": 12}, seq: 2})
		m = next.(Model)
		next, _ = m.Update(previewsMsg{byPane: map[string]string{"p1": "old"}, revs: map[string]int{"p1": oldRev}, seq: 1})
		m = next.(Model)
		if m.lastRev["p1"] != 12 || m.previews["p1"] != "new" {
			t.Fatalf("older completion replaced the latest cache: %+v", m.previews)
		}
	}
}

func TestPreviewAndDetailShareCacheOrdering(t *testing.T) {
	m := sampleModel(80, 24)
	m = applyKey(m, "enter")
	next, _ := m.Update(readMsg{pane: "p2", text: "long fresh read", seq: m.detailSeq, cacheSeq: 3})
	m = next.(Model)
	next, _ = m.Update(previewsMsg{byPane: map[string]string{"p2": "old preview"}, revs: map[string]int{"p2": 1}, seq: 2})
	m = next.(Model)
	if m.previews["p2"] != "long fresh read" {
		t.Fatal("old preview replaced a newer detail read")
	}
	next, _ = m.Update(previewsMsg{byPane: map[string]string{"p2": "new preview"}, revs: map[string]int{"p2": 2}, seq: 4})
	m = next.(Model)
	next, _ = m.Update(readMsg{pane: "p2", text: "old long read", seq: m.detailSeq, cacheSeq: 3})
	if next.(Model).previews["p2"] != "new preview" {
		t.Fatal("old detail read replaced a newer preview")
	}
}

func TestPreviewBatchesDoNotStarveLaterCards(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%v", force), func(t *testing.T) {
			m := New()
			m.selfTab, m.selfPane = "", ""
			m.width, m.height, m.bin = 160, 80, mockHerdr(t)
			for i := 1; i <= 12; i++ {
				m.snap.Tabs = append(m.snap.Tabs, model.TabRow{TabID: fmt.Sprintf("t%d", i), PaneID: fmt.Sprintf("p%d", i), Agent: "codex", Status: "working", Revision: 1})
			}
			m.selID = "p1"
			for round := 0; round < 2; round++ {
				cmd := m.fetchFeedReads(force)
				if cmd == nil || len(m.previewBusy) > 8 {
					t.Fatalf("bad batch: cmd=%v in-flight=%d", cmd != nil, len(m.previewBusy))
				}
				next, _ := m.Update(cmd())
				m = next.(Model)
				for i := 0; i < 8; i++ {
					m.snap.Tabs[i].Revision++
				}
			}
			for i := 1; i <= 12; i++ {
				pane := fmt.Sprintf("p%d", i)
				if !strings.Contains(m.previews[pane], "output from "+pane) {
					t.Fatalf("visible %s was starved", pane)
				}
			}
		})
	}
}

func TestPreviewReadsDoNotOverlapAndFailuresRetry(t *testing.T) {
	m := sampleModel(80, 24)
	m.bin = mockHerdr(t)
	t.Setenv("PADDOCK_TEST_FAIL_PANE", "p1")
	cmd := m.fetchFeedReads(false)
	if cmd == nil {
		t.Fatal("missing first read")
	}
	if m.fetchFeedReads(true) != nil {
		t.Fatal("forced refresh overlapped active reads")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if len(m.previewBusy) != 0 {
		t.Fatal("failed read did not release its slot")
	}
	if _, ok := m.lastRev["p1"]; ok {
		t.Fatal("failed revision was cached")
	}
	t.Setenv("PADDOCK_TEST_FAIL_PANE", "")
	cmd = m.fetchFeedReads(false)
	if cmd == nil {
		t.Fatal("failed pane was not retried")
	}
	msg := cmd().(previewsMsg)
	if len(msg.byPane) != 1 || !strings.Contains(msg.byPane["p1"], "output from p1") {
		t.Fatalf("retry should only read failed pane: %+v", msg)
	}
}

func TestDetailScrollStopsAtTopForKeysAndWheel(t *testing.T) {
	for _, wheel := range []bool{false, true} {
		m := sampleModel(80, 24)
		m = applyKey(m, "enter")
		m.transcript = numberedTranscript(40, 1)
		for i := 0; i < 60; i++ {
			if wheel {
				next, _ := m.Update(mouseMsg(2, 2, tea.MouseActionPress, tea.MouseButtonWheelUp))
				m = next.(Model)
			} else {
				m = applyKey(m, "pgup")
			}
		}
		before := render(m)
		m = applyKey(m, "down")
		if render(m) == before {
			t.Fatalf("down was swallowed after overscrolling (wheel=%v)", wheel)
		}
	}
}

func TestDetailScrollClampsAfterResizeAndShorterRead(t *testing.T) {
	m := sampleModel(40, 16)
	m = applyKey(m, "enter")
	m.transcript = numberedTranscript(60, 1)
	for i := 0; i < 20; i++ {
		m = applyKey(m, "pgup")
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = next.(Model)
	if m.detailScroll != 25 {
		t.Fatalf("resize scroll=%d, want 25", m.detailScroll)
	}
	next, _ = m.Update(readMsg{pane: m.transcriptOf, seq: m.detailSeq, text: "short replacement"})
	m = next.(Model)
	if m.detailScroll != 0 {
		t.Fatalf("short read retained scroll=%d", m.detailScroll)
	}
}

// The plugin manifest is what herdr installs; the help panel must not show a
// different version.
func TestVersionMatchesPluginManifest(t *testing.T) {
	raw, err := os.ReadFile("../../herdr-plugin.toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, ln := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(ln), "version = "); ok {
			if strings.Trim(v, `"`) != version {
				t.Fatalf("herdr-plugin.toml has %s, ui.version is %q", v, version)
			}
			return
		}
	}
	t.Fatal("no version in herdr-plugin.toml")
}
