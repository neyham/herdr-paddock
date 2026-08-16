package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/neyham/herdr-paddock/internal/layout"
	"github.com/neyham/herdr-paddock/internal/model"
)

func sampleModel(w, h int) Model {
	m := New()
	m.width, m.height = w, h
	m.snap.Tabs = []model.TabRow{
		{TabID: "w5:t6", PaneID: "p1", WSLabel: "shop", TabLabel: "hooks", Agent: "agy", Status: "working", Title: "Inspect herdr snapshot JSON shape"},
		{TabID: "w2:t3", PaneID: "p2", WSLabel: "shop", TabLabel: "pay", Agent: "grok", Status: "blocked", Title: "Pick a payment provider"},
		{TabID: "w5:t9", WSLabel: "blog", TabLabel: "paddock", Agent: "", Status: "unknown", Title: "paddock", Focused: true},
	}
	m.previews = map[string]string{
		"p1": "snapshot has workspaces tabs and agents\nI can parse agent_status next",
		"p2": "compared the three payment providers\nneed your call on which one",
	}
	m.snap.Workspaces = 9
	return m
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestPhoneGlanceFits40(t *testing.T) {
	m := sampleModel(40, 16)
	out := render(m)
	plain := stripANSI(out)
	for i, line := range strings.Split(out, "\n") {
		if layout.Width(stripANSI(line)) > 40 {
			t.Fatalf("line %d width %d > 40: %q", i, layout.Width(stripANSI(line)), stripANSI(line))
		}
	}
	if strings.Contains(plain, "????") || strings.Contains(plain, "LAST KNOWN") {
		t.Fatalf("debug leftovers:\n%s", plain)
	}
	if !strings.Contains(plain, "╭") {
		t.Fatalf("missing card chrome:\n%s", plain)
	}
	if !strings.Contains(plain, "pay") || !strings.Contains(plain, "hooks") {
		t.Fatalf("phone feed should show more than one card:\n%s", plain)
	}
}

func TestGlanceCardsShowMultipleAgents(t *testing.T) {
	m := sampleModel(100, 24)
	out := render(m)
	plain := stripANSI(out)
	for i, line := range strings.Split(out, "\n") {
		if layout.Width(stripANSI(line)) > 100 {
			t.Fatalf("line %d width %d > 100: %q", i, layout.Width(stripANSI(line)), stripANSI(line))
		}
	}
	for _, need := range []string{"hooks", "pay", "snapshot has workspaces", "need your call", "╭", "grok"} {
		if !strings.Contains(plain, need) {
			t.Fatalf("missing %q:\n%s", need, plain)
		}
	}
	if !strings.Contains(plain, "agy") && !strings.Contains(plain, "antigravity") {
		t.Fatalf("missing agent name:\n%s", plain)
	}
}

func TestFullGlanceReadable(t *testing.T) {
	m := sampleModel(80, 24)
	plain := stripANSI(render(m))
	for _, bad := range []string{"????", "LAST KNOWN", "monitor"} {
		if strings.Contains(plain, bad) {
			t.Fatalf("ugly leftover %q:\n%s", bad, plain)
		}
	}
	for _, need := range []string{"hooks", "pay", "╭", "enter open", "o jump"} {
		if !strings.Contains(plain, need) {
			t.Fatalf("missing %q:\n%s", need, plain)
		}
	}
}

func TestGlanceGridShowsFour(t *testing.T) {
	m := sampleModel(80, 24)
	m.snap.Tabs = []model.TabRow{
		{TabID: "1", PaneID: "a", WSLabel: "shop", TabLabel: "pay", Agent: "grok", Status: "blocked", Title: "pick a provider"},
		{TabID: "2", PaneID: "b", WSLabel: "blog", TabLabel: "cdn", Agent: "agy", Status: "working", Title: "ship the cache"},
		{TabID: "3", PaneID: "c", WSLabel: "docs", TabLabel: "i18n", Agent: "codex", Status: "done", Title: "translate cards"},
		{TabID: "4", PaneID: "d", WSLabel: "infra", TabLabel: "cron", Agent: "pi", Status: "idle", Title: "watch the queue"},
	}
	m.previews = map[string]string{
		"a": "provider choice still open",
		"b": "shipping the cache cut",
		"c": "next card is 承知",
		"d": "watching the retry queue",
	}
	out := render(m)
	plain := stripANSI(out)
	for i, line := range strings.Split(out, "\n") {
		if layout.Width(stripANSI(line)) > 80 {
			t.Fatalf("line %d width %d > 80: %q", i, layout.Width(stripANSI(line)), stripANSI(line))
		}
	}
	for _, need := range []string{"pay", "cdn", "i18n", "cron", "provider choice", "shipping the cache", "next card", "retry"} {
		if !strings.Contains(plain, need) {
			t.Fatalf("missing %q:\n%s", need, plain)
		}
	}
}

func TestPreviewWrapsWithoutEllipsis(t *testing.T) {
	m := sampleModel(40, 16)
	m.previews["p2"] = "the snapshot has workspaces tabs agents and then a long tail about payment providers"
	plain := stripANSI(render(m))
	if !strings.Contains(plain, "providers") {
		t.Fatalf("lost wrapped tail:\n%s", plain)
	}
	lines := clipTranscript(m.previews["p2"], 16, 8)
	for _, ln := range lines {
		if strings.ContainsRune(ln, '…') {
			t.Fatalf("preview wrap added ellipsis: %q", ln)
		}
	}
	if !strings.Contains(strings.Join(lines, " "), "providers") {
		t.Fatalf("clipTranscript lost tail: %#v", lines)
	}
}

func TestStatusBarIsSinglePage(t *testing.T) {
	plain := stripANSI(render(sampleModel(80, 24)))
	// single page: no tab labels, just the brand and the hot counts
	for _, bad := range []string{"1 flock", "2 herdr", "3 usage"} {
		if strings.Contains(plain, bad) {
			t.Fatalf("page tabs should be gone, found %q:\n%s", bad, plain)
		}
	}
	if !strings.Contains(plain, "paddock") {
		t.Fatalf("missing brand:\n%s", plain)
	}
	if !strings.Contains(plain, "1 baa") || !strings.Contains(plain, "1 work") {
		t.Fatalf("missing hot counts:\n%s", plain)
	}
}

func TestWaterfallVariableHeights(t *testing.T) {
	m := sampleModel(80, 24)
	m.previews["p1"] = "one line only"
	m.previews["p2"] = strings.Repeat("a fairly long sentence about payment provider tradeoffs. ", 4)
	slots, _, feed := glanceSlots(m, 80, glanceFeedH(24))
	if len(slots) != 2 || len(feed) != 2 {
		t.Fatalf("want 2 cards, got %d", len(slots))
	}
	// feed[0] is the blocked pay card with the long preview
	if slots[0].h <= slots[1].h {
		t.Fatalf("long card should be taller: %d vs %d", slots[0].h, slots[1].h)
	}
	if slots[1].h < 5 {
		t.Fatalf("short card below minimum: %d", slots[1].h)
	}
}

func TestPhoneFeedShowsBothCards(t *testing.T) {
	m := sampleModel(40, 16)
	out := render(m)
	plain := stripANSI(out)
	if strings.Count(plain, "╭") < 2 {
		t.Fatalf("expected a compact card wall, got one pane:\n%s", plain)
	}
	if !strings.Contains(plain, "pay") || !strings.Contains(plain, "hooks") {
		t.Fatalf("both agents should be on screen:\n%s", plain)
	}
	for i, line := range strings.Split(out, "\n") {
		if layout.Width(stripANSI(line)) > 40 {
			t.Fatalf("line %d width %d > 40: %q", i, layout.Width(stripANSI(line)), stripANSI(line))
		}
	}
}

func TestStatusEdgeColors(t *testing.T) {
	if statusEdge("blocked", true, true).GetForeground() != cBad {
		t.Fatal("blocked should be herdr red")
	}
	if statusEdge("working", true, true).GetForeground() != cWarn {
		t.Fatal("working should be herdr yellow")
	}
	if statusEdge("done", true, true).GetForeground() != cBlue {
		t.Fatal("done should be herdr blue")
	}
	if statusEdge("idle", false, true).GetForeground() != cGood {
		t.Fatal("idle should be herdr green")
	}
}

func TestBlockedBorderBreathes(t *testing.T) {
	hi := statusEdge("blocked", false, true).GetForeground()
	lo := statusEdge("blocked", false, false).GetForeground()
	if hi == lo {
		t.Fatal("blocked border should breathe between two reds")
	}
	// only the bleating sheep moves: working stays steady
	if statusEdge("working", false, true).GetForeground() != statusEdge("working", false, false).GetForeground() {
		t.Fatal("working border must not breathe")
	}
}

func TestFlockBarShowsHerdMix(t *testing.T) {
	m := sampleModel(80, 24)
	bar := stripANSI(flockBar(m.snap.Tabs, 12))
	// one blocked + one working agent → one █ and one ▓
	if bar != "█▓" {
		t.Fatalf("flock bar should be one bleating one grazing, got %q", bar)
	}
	// a big flock squeezes proportionally but every state stays visible
	var tabs []model.TabRow
	for i := 0; i < 30; i++ {
		tabs = append(tabs, model.TabRow{TabID: fmt.Sprintf("t%d", i), Status: "working"})
	}
	tabs = append(tabs, model.TabRow{TabID: "b", Status: "blocked"})
	bar = stripANSI(flockBar(tabs, 10))
	if !strings.Contains(bar, "█") || layout.Width(bar) > 12 {
		t.Fatalf("squeezed bar must keep the blocked cell: %q", bar)
	}
}

func TestEmptyPastureShowsSleepingSheep(t *testing.T) {
	m := sampleModel(60, 20)
	m.snap.Tabs = nil
	plain := stripANSI(render(m))
	if !strings.Contains(plain, "the flock is quiet") || !strings.Contains(plain, "o<(oo)") {
		t.Fatalf("empty pasture should show the sleeping sheep:\n%s", plain)
	}
}

func TestFeedClickSelectsNeighbor(t *testing.T) {
	m := sampleModel(40, 16)
	i, ok := glanceIndexAt(m, 30, 2)
	if !ok || i != 1 {
		t.Fatalf("right column should be the second card, got %d ok=%v", i, ok)
	}
	i, ok = glanceIndexAt(m, 2, 2)
	if !ok || i != 0 {
		t.Fatalf("left column should be the first card, got %d ok=%v", i, ok)
	}
}

func TestIdleAgentStillOnGlance(t *testing.T) {
	m := sampleModel(80, 24)
	m.snap.Tabs = []model.TabRow{
		{TabID: "w1:t1", PaneID: "p9", WSLabel: "Learning", TabLabel: "N2", Agent: "grok", Status: "idle", Title: "drill cards"},
	}
	m.previews = map[string]string{"p9": "next card is 承知しました"}
	plain := stripANSI(render(m))
	if !strings.Contains(plain, "N2") || !strings.Contains(plain, "承知しました") {
		t.Fatalf("idle agent should still be a card:\n%s", plain)
	}
}

// --- v0.6 interaction model ---

func TestBrowseKeysNeverLeakIntoReply(t *testing.T) {
	m := sampleModel(80, 24)
	// browse: "j" moves selection, does not type
	next, _ := m.handleKey(keyMsg("j"))
	m = next.(Model)
	if m.input.Value() != "" {
		t.Fatalf("browse j typed into input: %q", m.input.Value())
	}
	// i enters input mode, then j/q/1 are just text
	next, _ = m.handleKey(keyMsg("i"))
	m = next.(Model)
	if !m.inputOn {
		t.Fatal("i should focus the reply box")
	}
	for _, ch := range []string{"j", "q", "1"} {
		next, _ = m.handleKey(keyMsg(ch))
		m = next.(Model)
	}
	if m.input.Value() != "jq1" {
		t.Fatalf("typed text lost: %q", m.input.Value())
	}
	// esc returns to browse and keeps the draft
	next, _ = m.handleKey(keyMsg("esc"))
	m = next.(Model)
	if m.inputOn || m.input.Value() != "jq1" {
		t.Fatalf("esc should keep draft, got on=%v %q", m.inputOn, m.input.Value())
	}
}

func TestEnterOpensDetailNotJump(t *testing.T) {
	m := sampleModel(80, 24)
	next, _ := m.handleKey(keyMsg("enter"))
	m = next.(Model)
	if !m.detailOpen {
		t.Fatal("enter on the wall should open the detail view")
	}
	if m.detailReply {
		t.Fatal("detail should open in view mode, not typing mode")
	}
	plain := stripANSI(render(m))
	if !strings.Contains(plain, "esc back") {
		t.Fatalf("detail hints missing:\n%s", plain)
	}
	// i switches to reply mode; typed keys go into the box
	next, _ = m.handleKey(keyMsg("i"))
	m = next.(Model)
	if !m.detailReply {
		t.Fatal("i should focus the reply box")
	}
	next, _ = m.handleKey(keyMsg("j"))
	m = next.(Model)
	if m.input.Value() != "j" {
		t.Fatalf("reply mode should type, got %q", m.input.Value())
	}
}

func TestDetailFlipsBetweenPosts(t *testing.T) {
	m := sampleModel(80, 24)
	next, _ := m.handleKey(keyMsg("enter")) // opens first card (blocked pay, w2:t3)
	m = next.(Model)
	if m.detailTab != "w2:t3" {
		t.Fatalf("expected the blocked card first, got %s", m.detailTab)
	}
	next, _ = m.handleKey(keyMsg("j"))
	m = next.(Model)
	if m.detailTab != "w5:t6" {
		t.Fatalf("j should flip to the next post, got %s", m.detailTab)
	}
	if m.selID != "w5:t6" {
		t.Fatal("wall selection should follow the flip")
	}
	next, _ = m.handleKey(keyMsg("k"))
	m = next.(Model)
	if m.detailTab != "w2:t3" {
		t.Fatalf("k should flip back, got %s", m.detailTab)
	}
}

func mouseMsg(x, y int, action tea.MouseAction, button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: action, Button: button}
}

func TestTapNeedsPressAndReleaseOnSameCell(t *testing.T) {
	m := sampleModel(80, 24)
	// press alone must not open anything (swipe start)
	next, _ := m.handleMouse(mouseMsg(2, 2, tea.MouseActionPress, tea.MouseButtonLeft))
	m = next.(Model)
	if m.detailOpen {
		t.Fatal("press alone should not open a card")
	}
	// release far away: the swipe is not a tap
	next, _ = m.handleMouse(mouseMsg(30, 10, tea.MouseActionRelease, tea.MouseButtonLeft))
	m = next.(Model)
	if m.detailOpen {
		t.Fatal("press+release on different cells should not open a card")
	}
	// clean tap: press and release on the same cell
	next, _ = m.handleMouse(mouseMsg(2, 2, tea.MouseActionPress, tea.MouseButtonLeft))
	m = next.(Model)
	next, _ = m.handleMouse(mouseMsg(2, 2, tea.MouseActionRelease, tea.MouseButtonLeft))
	m = next.(Model)
	if !m.detailOpen {
		t.Fatal("tap should zoom into the card")
	}
	if m.detailTab != "w2:t3" {
		t.Fatalf("tap on the left column should open the blocked card, got %s", m.detailTab)
	}
}

func TestScrollDoesNotSnapOnSelect(t *testing.T) {
	m := sampleModel(80, 24)
	m.snap.Tabs = nil
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("t%d", i)
		m.snap.Tabs = append(m.snap.Tabs, model.TabRow{
			TabID: id, PaneID: "p" + id, WSLabel: "infra", TabLabel: id, Agent: "pi", Status: "idle", Title: "task " + id,
		})
		m.previews["p"+id] = "line one\nline two\nline three"
	}
	// selecting the second card must not scroll: it is already visible
	next, _ := m.handleKey(keyMsg("j"))
	m = next.(Model)
	if m.feedScroll != 0 {
		t.Fatalf("selecting a visible card must not scroll, got %d", m.feedScroll)
	}
}

func TestSelectionFollowsTabID(t *testing.T) {
	m := sampleModel(80, 24)
	m.selID = "w5:t6" // hooks, currently working → second in feed
	// hooks turns blocked: it moves to the front, selection must follow
	m.snap.Tabs[0].Status = "blocked"
	feed := feedTabs(m)
	if idx := selIndex(feed, m.selID); idx != 0 {
		t.Fatalf("selection should follow the tab to its new slot, got %d", idx)
	}
}

func TestDetailShowsTranscriptAndEcho(t *testing.T) {
	m := sampleModel(60, 20)
	m.detailOpen = true
	m.detailTab = "w2:t3"
	m.transcriptOf = "p2"
	m.transcript = "the provider docs arrived\nwhich payment provider should I wire up?"
	m.echoes = append(m.echoes, echoLine{pane: "p2", text: "go with stripe"})
	plain := stripANSI(render(m))
	for _, need := range []string{"provider docs", "payment provider", "you › go with stripe", "esc back"} {
		if !strings.Contains(plain, need) {
			t.Fatalf("missing %q:\n%s", need, plain)
		}
	}
}

func TestChromeFilteredFromCards(t *testing.T) {
	raw := "real progress on the parser\n" +
		"⠋ Thinking…\n" +
		"↑96k ↓37k R5.9M CH99.5% $0.020 12.6%/1.0M (auto)  deepseek-v4 • max\n" +
		"esc to interrupt\n" +
		"❯\n"
	lines := clipTranscript(raw, 60, 8)
	joined := strings.Join(lines, " ")
	if !strings.Contains(joined, "real progress") {
		t.Fatalf("lost the real text: %#v", lines)
	}
	for _, bad := range []string{"↑96k", "Thinking", "interrupt", "⠋"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("chrome %q leaked: %#v", bad, lines)
		}
	}
}

func TestBlockedQuestionHighlighted(t *testing.T) {
	lines := []string{
		"I compared both drafts and they differ in tone.",
		"which draft should I send?",
		"1. formal",
		"2. casual",
	}
	if qs := questionStart(lines); qs != 1 {
		t.Fatalf("question should start at the ask line, got %d", qs)
	}
}

// Fixtures below mirror the shapes of real agent pane tails (synthetic
// content). The agent's own input box and bottom bars sit under the last
// rule/box border; none of that is agent output and none of it may reach a
// card.
func TestAgentInputBoxAndFooterDropped(t *testing.T) {
	cases := []struct {
		name, raw, want string
		bad             []string
	}{
		{
			name: "grok",
			raw: "结算页的文案已经过审，可以直接合并了。\n\n" +
				"                                          ▼\n" +
				"  ╭───────────────────── 2026-08-08｜Checkout Revamp｜Copy Review ──────────╮\n" +
				"  │ ❯                                                                       │\n" +
				"  ╰─────────────────────────────────────── Grok 4.6 (xhigh) · always-approve ─╯\n\n" +
				"  Shift+Tab:mode  │  Ctrl+.:shortcuts\n",
			want: "结算页",
			bad:  []string{"Grok 4.6", "Shift+Tab", "❯", "▼", "always-approve"},
		},
		{
			name: "codex",
			raw: "缓存命中率稳定在 92%，先不再调参。\n\n" +
				"─ Worked for 11m 37s ──────────────────────────────\n\n" +
				"» Improve documentation in @filename\n\n" +
				"  gpt-5.6-sol ultra · ~/projects/demo · Main [default]\n",
			want: "命中率",
			bad:  []string{"Worked for", "»", "[default]", "gpt-5.6"},
		},
		{
			name: "cursor",
			raw: "tap 需要 press 和 release 落在同一格。\n\n" +
				" ⠘⠣ Running  3.75k tokens\n" +
				"▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄\n" +
				"  → Add a follow-up   ctrl+c to stop\n" +
				"▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀\n" +
				"  1 task\n" +
				"  Fable 5 300K High · 18.1% · 8 files edited   Run Everything\n" +
				"  ~/projects/demo · main\n",
			want: "同一格",
			bad:  []string{"Add a follow-up", "1 task", "Fable 5", "tokens", "· main"},
		},
		{
			name: "agy",
			raw: "请确认 Comment 1 到 Comment 4 的修改口径。\n" +
				"──────────────────────────────\n" +
				">\n" +
				"──────────────────────────────\n" +
				"? for shortcuts                          Gemini 3.7 Flash · high\n",
			want: "修改口径",
			bad:  []string{"? for shortcuts", "Gemini"},
		},
		{
			name: "pi",
			raw: "顺便说一句，这套重试队列设计挺完整的\n\n" +
				"──────────────────────────────\n" +
				"~\n" +
				"↑96k ↓37k R5.9M CH99.5% $0.020 12.6%/1.0M (auto)   (opencode-go) deepseek-v4-flash • max\n",
			want: "重试队列",
			bad:  []string{"↑96k", "deepseek"},
		},
	}
	for _, c := range cases {
		lines := clipTranscript(c.raw, 60, 12)
		joined := strings.Join(lines, " ")
		if !strings.Contains(joined, c.want) {
			t.Fatalf("%s: lost real output %q: %#v", c.name, c.want, lines)
		}
		for _, bad := range c.bad {
			if strings.Contains(joined, bad) {
				t.Fatalf("%s: chrome %q leaked: %#v", c.name, bad, lines)
			}
		}
	}
}

func TestBlockedDialogQuestionSurvivesChromeCut(t *testing.T) {
	raw := "我比较了两版草稿。\n" +
		"╭──────────────────────────────╮\n" +
		"│ which draft should I send?   │\n" +
		"│ ❯ 1. formal                  │\n" +
		"│   2. casual                  │\n" +
		"╰──────────────────────────────╯\n"
	joined := strings.Join(clipTranscript(raw, 60, 12), " ")
	for _, need := range []string{"which draft", "1. formal", "2. casual"} {
		if !strings.Contains(joined, need) {
			t.Fatalf("blocked dialog content %q was cut: %q", need, joined)
		}
	}
}

func TestTapLockedAtPressSurvivesReflow(t *testing.T) {
	m := sampleModel(80, 24)
	// finger lands on the second column card (hooks / w5:t6)
	next, _ := m.handleMouse(mouseMsg(45, 2, tea.MouseActionPress, tea.MouseButtonLeft))
	m = next.(Model)
	if m.pressTab != "w5:t6" {
		t.Fatalf("press should lock the card under the finger, got %q", m.pressTab)
	}
	// tap wobble fires wheel events while the finger is down: must not scroll
	next, _ = m.handleMouse(mouseMsg(45, 2, tea.MouseActionPress, tea.MouseButtonWheelDown))
	m = next.(Model)
	if m.selID == "w5:t6" {
		t.Fatal("wheel during a pending tap must not move the selection")
	}
	// release a cell off (finger roll): still the locked card, straight in
	next, _ = m.handleMouse(mouseMsg(46, 3, tea.MouseActionRelease, tea.MouseButtonLeft))
	m = next.(Model)
	if !m.detailOpen || m.detailTab != "w5:t6" {
		t.Fatalf("tap should zoom into the pressed card, open=%v tab=%q", m.detailOpen, m.detailTab)
	}
}

// 4 equal cards on a 40-col phone → 2×2 grid:
//
//	t0 t1
//	t2 t3
func gridModel() Model {
	m := sampleModel(40, 16)
	m.snap.Tabs = nil
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("t%d", i)
		m.snap.Tabs = append(m.snap.Tabs, model.TabRow{
			TabID: id, PaneID: "p" + id, WSLabel: "infra", TabLabel: id, Agent: "pi", Status: "idle", Title: "task " + id,
		})
		m.previews["p"+id] = "line one\nline two"
	}
	m.selID = "t0"
	return m
}

func TestArrowsFollowGeometry(t *testing.T) {
	m := gridModel()
	step := func(key, want string) {
		next, _ := m.handleKey(keyMsg(key))
		m = next.(Model)
		if m.selID != want {
			t.Fatalf("%s should land on %s, got %s", key, want, m.selID)
		}
	}
	step("down", "t2")  // same column, one below
	step("right", "t3") // neighbor column, same row
	step("up", "t1")    // back up inside the right column
	// newspaper flow: off the top of a column resumes at the bottom of the
	// column to its left, off the bottom at the top of the column to its right
	step("up", "t2")   // top of right column -> bottom of left column
	step("down", "t1") // bottom of left column -> top of right column
	// only the wall's true corners stop
	m.selID = "t0"
	step("up", "t0")
	step("left", "t0")
	m.selID = "t3"
	step("down", "t3")
	step("right", "t3")
}

func TestSelectedCardFloats(t *testing.T) {
	m := sampleModel(80, 24)
	plain := stripANSI(render(m))
	// the sheep stands on the selected card, not just in the status bar
	if strings.Count(plain, "🐑") < 2 {
		t.Fatalf("selected card should carry the sheep marker:\n%s", plain)
	}
	// drop shadow: half blocks along the selected card's right and bottom
	if !strings.Contains(plain, "▌") || !strings.Contains(plain, "▀") {
		t.Fatalf("selected card should cast a shadow:\n%s", plain)
	}
}

func TestParseCardTitle(t *testing.T) {
	cases := map[string]string{
		"π - 2026-08-12｜Launchd Setup｜Takeover Launchd Tab - sam": "Takeover Launchd Tab",
		"2026-08-08｜Checkout Revamp｜Copy Review - grok":           "Copy Review",
		"Inspect herdr snapshot JSON shape":                       "Inspect herdr snapshot JSON shape",
		"π - sam":                                                 "π",
	}
	for in, want := range cases {
		if got := parseCardTitle(in); got != want {
			t.Fatalf("parseCardTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
