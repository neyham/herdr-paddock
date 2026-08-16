package ui

import (
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/neyham/herdr-paddock/internal/herdr"
	"github.com/neyham/herdr-paddock/internal/model"
)

const version = "0.6.2"

type ageMark struct {
	seq int
	at  time.Time
}

type echoLine struct {
	pane   string
	text   string
	failed bool
	at     time.Time
}

type Model struct {
	width, height int
	selID         string // glance selection, by tab id so reorders don't steal it
	quitConfirm   bool
	showHelp      bool
	snap          model.Snapshot
	bin           string
	input         textinput.Model
	inputOn       bool
	detailOpen    bool
	detailReply   bool // detail sub-mode: false = view/flip, true = typing
	detailTicking bool // one 1s refresh chain at a time
	detailTab     string
	detailScroll  int
	detailSeq     int // per-fetch sequence; late/out-of-order reads are dropped
	feedScroll    int // glance viewport, moves minimally to keep selection visible
	pressX        int // pending mouse press; click fires on release at the same cell
	pressY        int
	pressTab      string // card under the finger at press time; release opens it
	transcript    string // detail transcript (long read)
	transcriptOf  string
	previews      map[string]string
	lastRev       map[string]int // pane -> revision at last successful read
	seen          map[string]int // tab -> first-seen order
	seenN         int
	ages          map[string]ageMark
	echoes        []echoLine
	actionNote    string
	pulse         int // 1s heartbeat: blocked borders breathe, the sheep glances back
	selfTab       string
	selfPane      string
	composePane   string // wall compose target, pinned when the input gains focus
	composeTab    string
	everSnapped   bool // at least one successful snapshot arrived
}

type herdrMsg struct {
	tabs []model.TabRow
	ws   int
	err  error
}

type focusMsg struct {
	err error
	ok  string
}

type readMsg struct {
	pane string
	text string
	seq  int
	err  error
}

type sendMsg struct {
	err  error
	pane string
	text string
}

type previewsMsg struct {
	byPane map[string]string
	revs   map[string]int
}

type tickHerdr time.Time
type tickSlow time.Time
type tickDetail time.Time
type tickPulse time.Time

func New() Model {
	ti := textinput.New()
	ti.Prompt = "▸ "
	// Never let the input fall back to the terminal default (white) — it is
	// the one thing that would break the theme, especially inside a popup.
	ti.PromptStyle = stKey
	ti.TextStyle = stText
	ti.PlaceholderStyle = stDim
	ti.Cursor.Style = stSel
	ti.CharLimit = 4000
	ti.Width = 48
	// Inside a herdr plugin invocation, HERDR_BIN_PATH points at the running
	// herdr binary; standalone runs fall back to PATH lookup.
	bin := os.Getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = "herdr"
	}
	return Model{
		width:    80,
		height:   24,
		bin:      bin,
		input:    ti,
		pressX:   -1,
		pressY:   -1,
		previews: map[string]string{},
		lastRev:  map[string]int{},
		seen:     map[string]int{},
		ages:     map[string]ageMark{},
		selfTab:  os.Getenv("HERDR_TAB_ID"),
		selfPane: os.Getenv("HERDR_PANE_ID"),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchHerdr(m.bin),
		tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickHerdr(t) }),
		tea.Tick(20*time.Second, func(t time.Time) tea.Msg { return tickSlow(t) }),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickPulse(t) }),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if msg.Width > 12 {
			m.input.Width = msg.Width/2 - 8
			if m.input.Width < 16 {
				m.input.Width = 16
			}
		}
		m.syncScroll()
		return m, nil
	case herdrMsg:
		if msg.err != nil {
			// One failed poll must not blank a wall that was fine a second
			// ago: keep the last good snapshot and flag it stale. Only a
			// session that never produced data shows the error card.
			if m.everSnapped {
				m.snap.Herdr.Health = model.HealthStale
			} else {
				m.snap.Herdr.Health = model.HealthErr
				m.snap.Tabs = nil
			}
			m.snap.Herdr.ErrCode = msg.err.Error()
		} else {
			m.everSnapped = true
			m.snap.Herdr.Health = model.HealthOK
			m.snap.Herdr.ErrCode = ""
			m.snap.Tabs = msg.tabs
			m.snap.Workspaces = msg.ws
			m.snap.Herdr.UpdatedAt = time.Now()
			now := time.Now()
			for _, t := range msg.tabs {
				if _, ok := m.seen[t.TabID]; !ok {
					m.seen[t.TabID] = m.seenN
					m.seenN++
				}
				if a, ok := m.ages[t.TabID]; !ok || a.seq != t.StateSeq {
					m.ages[t.TabID] = ageMark{seq: t.StateSeq, at: now}
				}
			}
			m.fixSelection()
		}
		return m, fetchFeedReads(m, false)
	case focusMsg:
		if msg.err != nil {
			m.actionNote = "jump failed: " + msg.err.Error()
		} else {
			m.actionNote = msg.ok
		}
		return m, fetchHerdr(m.bin)
	case readMsg:
		// Reads overlap (1s tick, 3s timeout): only the latest dispatched
		// fetch may land, or a slow old response would overwrite new state.
		if msg.pane != "" && msg.pane == m.transcriptOf && msg.seq == m.detailSeq {
			if msg.err != nil {
				if m.transcript == "" {
					m.actionNote = "read failed: " + msg.err.Error()
				}
			} else {
				m.transcript = msg.text
				m.previews[msg.pane] = msg.text // the wall benefits from the long read too
				m.dropEchoedReplies(msg.pane, msg.text)
				if strings.HasPrefix(m.actionNote, "read failed") {
					m.actionNote = ""
				}
			}
		}
		return m, nil
	case previewsMsg:
		// byPane only carries successful reads (failures are retried next
		// tick), so an empty string is a genuinely empty pane: store it to
		// clear any stale preview and record the revision so the pane is
		// not re-read every tick.
		for id, text := range msg.byPane {
			m.previews[id] = text
			if rev, ok := msg.revs[id]; ok {
				m.lastRev[id] = rev
			}
		}
		return m, nil
	case sendMsg:
		if msg.err != nil {
			m.actionNote = "send failed: " + msg.err.Error()
			m.markEchoFailed(msg.pane, msg.text)
			// Restore the draft only when the input is still aimed at the
			// pane the send was for — a late failure must not inject the old
			// pane's text into a reply meant for another agent.
			target := m.composePane
			if m.detailOpen {
				target = m.transcriptOf
			}
			if target == msg.pane && strings.TrimSpace(m.input.Value()) == "" {
				m.input.SetValue(msg.text)
				m.input.CursorEnd()
			}
		} else {
			m.actionNote = "baa~ delivered"
		}
		return m, nil
	case tickHerdr:
		return m, tea.Batch(fetchHerdr(m.bin), tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickHerdr(t) }))
	case tickSlow:
		// safety net: even if revisions sit still, refresh visible cards
		return m, tea.Batch(fetchFeedReads(m, true), tea.Tick(20*time.Second, func(t time.Time) tea.Msg { return tickSlow(t) }))
	case tickPulse:
		m.pulse++
		return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickPulse(t) })
	case tickDetail:
		if !m.detailOpen {
			m.detailTicking = false
			return m, nil
		}
		m.detailSeq++
		return m, tea.Batch(fetchDetail(m.bin, m.transcriptOf, m.detailSeq), tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickDetail(t) }))
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.showHelp {
		switch msg.String() {
		case "esc", "?", "q", "enter":
			m.showHelp = false
		}
		return m, nil
	}
	if m.quitConfirm {
		switch msg.String() {
		case "q", "y", "enter":
			return m, tea.Quit
		case "esc", "n":
			m.quitConfirm = false
		}
		return m, nil
	}
	if m.detailOpen {
		return m.handleDetailKey(msg)
	}
	if m.inputOn {
		return m.handleComposeKey(msg)
	}
	return m.handleBrowseKey(msg)
}

// handleBrowseKey: every key is a command; nothing leaks into the input box.
func (m Model) handleBrowseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitConfirm = true
		return m, nil
	case "?":
		m.showHelp = true
		return m, nil
	case "esc":
		m.actionNote = ""
		return m, nil
	case "r":
		return m, tea.Batch(fetchHerdr(m.bin), fetchFeedReads(m, true))
	case "j", "down":
		m.moveSpatial("down")
		m.actionNote = ""
		return m, fetchFeedReads(m, false)
	case "k", "up":
		m.moveSpatial("up")
		m.actionNote = ""
		return m, fetchFeedReads(m, false)
	case "h", "left":
		m.moveSpatial("left")
		m.actionNote = ""
		return m, fetchFeedReads(m, false)
	case "l", "right":
		m.moveSpatial("right")
		m.actionNote = ""
		return m, fetchFeedReads(m, false)
	case "g":
		m.moveTo(0)
		return m, fetchFeedReads(m, false)
	case "G":
		m.moveTo(len(feedTabs(m)) - 1)
		return m, fetchFeedReads(m, false)
	case "i", "/":
		row := m.selectedRow()
		if row == nil {
			return m, nil
		}
		// Pin the reply target now: the wall may reflow or remap the
		// selection while the user types, and the draft must never follow
		// it onto another agent's card.
		m.composeTab, m.composePane = row.TabID, row.PaneID
		m.inputOn = true
		m.input.Focus()
		m.actionNote = ""
		return m, nil
	case "o":
		return m.jumpSelected()
	case "enter", "ctrl+m":
		return m.openDetail()
	}
	return m, nil
}

// handleComposeKey: quick reply from the card wall. Everything types except
// esc (back to browse, draft kept) and enter (send). The reply goes to the
// pane pinned when compose opened, never to whatever the selection remapped
// to since.
func (m Model) handleComposeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputOn = false
		m.input.Blur()
		return m, nil
	case "enter", "ctrl+m":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		if m.composePane == "" {
			m.actionNote = "no agent pane"
			return m, nil
		}
		if !m.paneAlive(m.composePane) {
			// The agent left while the user was typing: keep the draft and
			// say so, instead of silently rerouting it to another sheep.
			m.inputOn = false
			m.input.Blur()
			m.actionNote = "agent left the paddock · draft kept"
			return m, nil
		}
		m.input.SetValue("")
		m.actionNote = "baa~ sent"
		return m, sendPrompt(m.bin, m.composePane, text)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// paneAlive reports whether a pane is still present in the latest snapshot.
func (m Model) paneAlive(pane string) bool {
	for _, t := range m.snap.Tabs {
		if t.PaneID == pane {
			return true
		}
	}
	return false
}

// handleDetailKey: inside a post, xiaohongshu style. View mode flips between
// posts with j/k; reply mode types freely. up/down always scroll the
// transcript (the single-line input never needs them).
func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.detailScroll++
		return m, nil
	case "down":
		if m.detailScroll > 0 {
			m.detailScroll--
		}
		return m, nil
	case "pgup":
		m.detailScroll += 8
		return m, nil
	case "pgdown":
		m.detailScroll = max(0, m.detailScroll-8)
		return m, nil
	case "ctrl+o":
		row := m.detailRow()
		if row == nil {
			return m, nil
		}
		return m, jumpTab(m.bin, *row)
	}
	if m.detailReply {
		switch msg.String() {
		case "esc":
			m.detailReply = false
			m.input.Blur()
			return m, nil
		case "enter", "ctrl+m":
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			// Reply to the pane whose transcript is on screen — if the
			// tab's pane got remapped since, the displayed text and the
			// target must not diverge.
			pane := m.transcriptOf
			if pane == "" {
				if row := m.detailRow(); row != nil {
					pane = row.PaneID
				}
			}
			if pane == "" || !m.paneAlive(pane) {
				m.actionNote = "agent gone · draft kept"
				return m, nil
			}
			m.input.SetValue("")
			m.detailScroll = 0
			m.echoes = append(m.echoes, echoLine{pane: pane, text: text, at: time.Now()})
			return m, sendPrompt(m.bin, pane, text)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	// view mode
	switch msg.String() {
	case "esc":
		m.closeDetail()
		return m, nil
	case "q":
		m.quitConfirm = true
		return m, nil
	case "?":
		m.showHelp = true
		return m, nil
	case "j", "right":
		return m.flipDetail(1)
	case "k", "left":
		return m.flipDetail(-1)
	case "i", "/", "enter", "ctrl+m":
		m.detailReply = true
		m.input.Focus()
		return m, nil
	case "o":
		row := m.detailRow()
		if row == nil {
			return m, nil
		}
		return m, jumpTab(m.bin, *row)
	case "r":
		m.detailSeq++
		return m, fetchDetail(m.bin, m.transcriptOf, m.detailSeq)
	}
	return m, nil
}

// flipDetail moves to the previous/next post without leaving the zoom view.
func (m Model) flipDetail(delta int) (tea.Model, tea.Cmd) {
	feed := feedTabs(m)
	if len(feed) == 0 {
		return m, nil
	}
	idx := selIndex(feed, m.detailTab)
	if idx < 0 {
		idx = selIndex(feed, m.selID)
	}
	if idx < 0 {
		idx = 0
	}
	idx += delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(feed) {
		idx = len(feed) - 1
	}
	row := feed[idx]
	if row.TabID == m.detailTab {
		return m, nil
	}
	m.selID = row.TabID
	m.detailTab = row.TabID
	m.detailScroll = 0
	m.transcriptOf = row.PaneID
	m.transcript = m.previews[row.PaneID]
	m.syncScroll()
	m.detailSeq++
	return m, fetchDetail(m.bin, row.PaneID, m.detailSeq)
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	ev := tea.MouseEvent(msg)
	switch ev.Button {
	case tea.MouseButtonWheelUp:
		if m.pressX >= 0 {
			return m, nil // finger still down: tap wobble, not a scroll
		}
		if m.detailOpen {
			m.detailScroll += 3
			return m, nil
		}
		m.moveSpatial("up")
		m.actionNote = ""
		return m, fetchFeedReads(m, false)
	case tea.MouseButtonWheelDown:
		if m.pressX >= 0 {
			return m, nil
		}
		if m.detailOpen {
			m.detailScroll = max(0, m.detailScroll-3)
			return m, nil
		}
		m.moveSpatial("down")
		m.actionNote = ""
		return m, fetchFeedReads(m, false)
	}
	// A tap is press + release on (nearly) the same spot. The target card is
	// locked at press time, so even if the wall reflows or scrolls under the
	// finger, release opens the card that was actually touched.
	switch ev.Action {
	case tea.MouseActionPress:
		if ev.Button == tea.MouseButtonLeft {
			m.pressX, m.pressY = ev.X, ev.Y
			m.pressTab = ""
			if !m.detailOpen && ev.Y < m.height-2 {
				if i, hit := glanceIndexAt(m, ev.X, ev.Y); hit {
					feed := feedTabs(m)
					if i >= 0 && i < len(feed) {
						m.pressTab = feed[i].TabID
					}
				}
			}
		}
		return m, nil
	case tea.MouseActionMotion:
		if m.pressX >= 0 && (absInt(ev.X-m.pressX) > 2 || absInt(ev.Y-m.pressY) > 2) {
			m.pressX, m.pressY, m.pressTab = -1, -1, "" // drag, not a tap
		}
		return m, nil
	case tea.MouseActionRelease:
		if ev.Button != tea.MouseButtonLeft && ev.Button != tea.MouseButtonNone {
			return m, nil
		}
		px, py, tab := m.pressX, m.pressY, m.pressTab
		m.pressX, m.pressY, m.pressTab = -1, -1, ""
		if px < 0 || absInt(ev.X-px) > 2 || absInt(ev.Y-py) > 2 {
			return m, nil
		}
		return m.clickAt(px, py, tab)
	}
	return m, nil
}

func (m Model) clickAt(x, y int, tab string) (tea.Model, tea.Cmd) {
	if m.detailOpen {
		// tapping the reply box focuses it, like tapping a comment field
		if y >= m.height-4 && y <= m.height-2 {
			m.detailReply = true
			m.input.Focus()
		}
		return m, nil
	}
	// Mid-compose taps are swallowed: opening another card would silently
	// re-aim the draft. esc first, then browse.
	if m.inputOn {
		return m, nil
	}
	// xiaohongshu tap: straight into the card that was under the finger
	if tab != "" && selIndex(feedTabs(m), tab) >= 0 {
		m.selID = tab
		m.actionNote = ""
		m.syncScroll()
		return m.openDetail()
	}
	if tab == "" && y == m.height-2 {
		m.inputOn = true
		m.input.Focus()
		return m, nil
	}
	return m, nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (m Model) openDetail() (tea.Model, tea.Cmd) {
	row := m.selectedRow()
	if row == nil || row.PaneID == "" {
		m.actionNote = "no agent pane"
		return m, nil
	}
	m.detailOpen = true
	m.detailTab = row.TabID
	m.detailScroll = 0
	m.detailReply = false // open zoomed to read; i / enter to reply
	m.inputOn = false
	m.input.Blur()
	m.transcript = m.previews[row.PaneID] // show something while the long read runs
	m.transcriptOf = row.PaneID
	m.actionNote = ""
	m.detailSeq++
	cmds := []tea.Cmd{fetchDetail(m.bin, row.PaneID, m.detailSeq)}
	if !m.detailTicking {
		m.detailTicking = true
		cmds = append(cmds, tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickDetail(t) }))
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) closeDetail() {
	m.detailOpen = false
	m.detailReply = false
	m.detailScroll = 0
	m.inputOn = false
	m.input.Blur()
}

// syncScroll persists the glance viewport after anything that can move the
// selection or reflow the wall.
func (m *Model) syncScroll() {
	_, scroll, _ := glanceSlots(*m, m.width, glanceFeedH(m.height))
	m.feedScroll = scroll
}

func (m Model) detailRow() *model.TabRow {
	for _, t := range m.snap.Tabs {
		if t.TabID == m.detailTab {
			row := t
			return &row
		}
	}
	return nil
}

func (m Model) jumpSelected() (tea.Model, tea.Cmd) {
	row := m.selectedRow()
	if row == nil {
		m.actionNote = "nothing selected"
		return m, nil
	}
	return m, jumpTab(m.bin, *row)
}

// moveSpatial walks the wall the way it looks, not the way the feed is
// ordered: up/down stay inside the column, and when the column runs out they
// continue column by column like a newspaper — off the top lands on the
// BOTTOM of the column to the left, off the bottom lands on the TOP of the
// column to the right. left/right hop to the vertically closest card of the
// neighboring column. Only the wall's first and last column corners stop.
func (m *Model) moveSpatial(dir string) {
	slots, _, feed := glanceSlots(*m, m.width, glanceFeedH(m.height))
	if len(feed) == 0 {
		m.selID = ""
		return
	}
	cur := selIndex(feed, m.selID)
	if cur < 0 {
		m.selID = feed[0].TabID
		m.syncScroll()
		return
	}
	var sel cardSlot
	for _, s := range slots {
		if s.idx == cur {
			sel = s
		}
	}
	center := func(s cardSlot) int { return s.y + s.h/2 }
	best, bestScore := -1, 1<<30
	consider := func(idx, score int) {
		if score < bestScore {
			best, bestScore = idx, score
		}
	}
	for _, s := range slots {
		if s.idx == cur {
			continue
		}
		dy := absInt(center(s) - center(sel))
		switch dir {
		case "up":
			if s.x != sel.x || s.y >= sel.y {
				continue
			}
			consider(s.idx, sel.y-s.y)
		case "down":
			if s.x != sel.x || s.y <= sel.y {
				continue
			}
			consider(s.idx, s.y-sel.y)
		case "left":
			if s.x >= sel.x {
				continue
			}
			consider(s.idx, (sel.x-s.x)*100+dy)
		case "right":
			if s.x <= sel.x {
				continue
			}
			consider(s.idx, (s.x-sel.x)*100+dy)
		}
	}
	// Column exhausted: continue column by column. Off the top of a column
	// resumes at the bottom of the adjacent column to the left; off the
	// bottom resumes at the top of the adjacent column to the right.
	if best < 0 && (dir == "up" || dir == "down") {
		hopX := -1
		for _, s := range slots {
			if dir == "up" && s.x < sel.x && s.x > hopX {
				hopX = s.x
			}
			if dir == "down" && s.x > sel.x && (hopX < 0 || s.x < hopX) {
				hopX = s.x
			}
		}
		for _, s := range slots {
			if s.x != hopX {
				continue
			}
			if dir == "up" {
				consider(s.idx, -s.y) // bottom-most card of that column
			} else {
				consider(s.idx, s.y) // top-most card of that column
			}
		}
	}
	if best >= 0 {
		m.selID = feed[best].TabID
		m.syncScroll()
	}
}

func (m *Model) moveTo(idx int) {
	feed := feedTabs(*m)
	if len(feed) == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(feed) {
		idx = len(feed) - 1
	}
	m.selID = feed[idx].TabID
	m.syncScroll()
}

func (m *Model) fixSelection() {
	feed := feedTabs(*m)
	if len(feed) == 0 {
		m.selID = ""
		return
	}
	if selIndex(feed, m.selID) < 0 {
		m.selID = feed[0].TabID
	}
	m.syncScroll()
}

func selIndex(feed []model.TabRow, id string) int {
	for i, t := range feed {
		if t.TabID == id {
			return i
		}
	}
	return -1
}

func (m Model) selectedRow() *model.TabRow {
	feed := feedTabs(m)
	idx := selIndex(feed, m.selID)
	if idx < 0 && len(feed) > 0 {
		idx = 0
	}
	if idx >= 0 && idx < len(feed) {
		t := feed[idx]
		return &t
	}
	return nil
}

func (m *Model) dropEchoedReplies(pane, transcript string) {
	if len(m.echoes) == 0 {
		return
	}
	flat := strings.Join(strings.Fields(transcript), " ")
	kept := m.echoes[:0]
	for _, e := range m.echoes {
		probe := strings.Join(strings.Fields(e.text), " ")
		if len(probe) > 40 {
			probe = probe[:40]
		}
		if e.pane == pane && probe != "" && strings.Contains(flat, probe) {
			continue // agent's terminal now shows the reply; echo served its purpose
		}
		if time.Since(e.at) > 2*time.Minute {
			continue
		}
		kept = append(kept, e)
	}
	m.echoes = kept
}

func (m *Model) markEchoFailed(pane, text string) {
	for i := len(m.echoes) - 1; i >= 0; i-- {
		if m.echoes[i].pane == pane && m.echoes[i].text == text {
			m.echoes[i].failed = true
			return
		}
	}
}

func (m Model) View() string {
	return render(m)
}

func fetchHerdr(bin string) tea.Cmd {
	return func() tea.Msg {
		tabs, ws, err := herdr.List(bin)
		return herdrMsg{tabs: tabs, ws: ws, err: err}
	}
}

func jumpTab(bin string, row model.TabRow) tea.Cmd {
	return func() tea.Msg {
		err := herdr.Focus(bin, row.WorkspaceID, row.TabID, row.PaneID)
		if err != nil {
			return focusMsg{err: err}
		}
		label := strings.TrimSpace(row.WSLabel + " · " + row.TabLabel)
		return focusMsg{ok: "herdr → " + label}
	}
}

// fetchFeedReads pulls previews for visible cards, but only for panes whose
// revision moved since the last successful read (force overrides).
func fetchFeedReads(m Model, force bool) tea.Cmd {
	candidates := visiblePanes(m)
	if len(candidates) == 0 {
		return nil
	}
	var ids []string
	revs := map[string]int{}
	for _, c := range candidates {
		if last, read := m.lastRev[c.pane]; !force && read && last == c.rev {
			continue
		}
		ids = append(ids, c.pane)
		revs[c.pane] = c.rev
	}
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > 8 {
		ids = ids[:8]
	}
	bin := m.bin
	return func() tea.Msg {
		return previewsMsg{byPane: herdr.ReadMany(bin, ids, 28), revs: revs}
	}
}

type paneRev struct {
	pane string
	rev  int
}

func visiblePanes(m Model) []paneRev {
	slots, scroll, feed := glanceSlots(m, m.width, glanceFeedH(m.height))
	if len(feed) == 0 {
		return nil
	}
	viewH := glanceFeedH(m.height)
	seen := map[string]bool{}
	var out []paneRev
	for _, s := range slots {
		if s.y+s.h <= scroll || s.y >= scroll+viewH {
			continue
		}
		t := feed[s.idx]
		if t.PaneID == "" || seen[t.PaneID] {
			continue
		}
		seen[t.PaneID] = true
		out = append(out, paneRev{pane: t.PaneID, rev: t.Revision})
	}
	if row := m.selectedRow(); row != nil && row.PaneID != "" && !seen[row.PaneID] {
		out = append(out, paneRev{pane: row.PaneID, rev: row.Revision})
	}
	return out
}

func fetchDetail(bin, pane string, seq int) tea.Cmd {
	if pane == "" {
		return nil
	}
	return func() tea.Msg {
		text, err := herdr.ReadRecent(bin, pane, 120)
		return readMsg{pane: pane, text: text, seq: seq, err: err}
	}
}

func sendPrompt(bin, pane, text string) tea.Cmd {
	return func() tea.Msg {
		return sendMsg{err: herdr.Prompt(bin, pane, text), pane: pane, text: text}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
