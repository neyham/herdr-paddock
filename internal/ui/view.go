package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/neyham/herdr-paddock/internal/herdr"
	"github.com/neyham/herdr-paddock/internal/layout"
	"github.com/neyham/herdr-paddock/internal/model"
)

func render(m Model) string {
	w, h := m.width, m.height
	if w < 20 {
		w = 20
	}
	if h < 8 {
		h = 8
	}
	var body []string
	switch {
	case m.showHelp:
		body = renderHelp(w, h-1)
	case m.detailOpen:
		body = renderDetail(m, w, h-1)
	default:
		body = renderGlance(m, w, h-1)
	}
	if m.quitConfirm {
		// clip first: the confirm line must survive even on a full screen
		body = append(clip(body, h-2), stErr.Render(" quit?  q again · esc cancel"))
	}
	body = clip(body, h-1)
	for len(body) < h-1 {
		body = append(body, "")
	}
	for i := range body {
		body[i] = exact(body[i], w)
	}
	body = append(body, exact(statusLine(m, w), w))
	return strings.Join(body, "\n")
}

// ---------------------------------------------------------------- glance

func renderGlance(m Model, w, h int) []string {
	if m.snap.Herdr.Health == model.HealthErr {
		ch := min(h, 8)
		return clip(cardPanel("🐑", "", []string{stErr.Render(herdrErr(m.snap.Herdr.ErrCode))}, w, ch, statusEdge("done", true, true)), h)
	}
	feed := feedTabs(m)
	if len(feed) == 0 {
		inner := append(sheepArt(), "", stDim.Render(" the flock is quiet"))
		ch := min(h, len(inner)+2)
		return clip(cardPanel("🐑", stDim.Render("zzz"), inner, w, ch, statusEdge("idle", true, true)), h)
	}
	feedH := glanceFeedH(m.height)
	slots, scroll, _ := glanceSlots(m, w, feedH)
	cur := selIndex(feed, m.selID)
	if cur < 0 {
		cur = 0
	}
	canvas := blankBlock(w, feedH)
	for _, s := range slots {
		// Full cards only, painted top-down; a top-crop peek of the next row
		// is fine, painting a card bottom-up is not.
		if s.y < scroll || s.y >= scroll+feedH {
			continue
		}
		card := renderAgentCard(m, feed[s.idx], s.idx, len(feed), s.w, s.h, s.idx == cur)
		blit(canvas, s.x, s.y-scroll, card)
	}
	// The selected card floats: paint its drop shadow over whatever sits
	// beside and beneath it, so moving the selection reads as a card lifting
	// off the wall rather than a border changing color.
	for _, s := range slots {
		if s.idx != cur || s.y < scroll || s.y >= scroll+feedH {
			continue
		}
		sy := s.y - scroll
		if s.x+s.w < w {
			for r := sy + 1; r < sy+s.h && r < feedH; r++ {
				blit(canvas, s.x+s.w, r, []string{stShadow.Render("▌")})
			}
		}
		// Bottom shadow only falls on empty ground: in a waterfall the next
		// card in the column starts right at y+h, and a shadow must never
		// eat that card's title row.
		below := s.y + s.h
		groundFree := true
		for _, o := range slots {
			if o.idx != s.idx && o.y <= below && below < o.y+o.h && o.x < s.x+s.w+1 && s.x < o.x+o.w {
				groundFree = false
				break
			}
		}
		if groundFree && sy+s.h < feedH {
			// the corner is a quarter block so it lines up with the
			// half-width right shadow column
			run := min(s.w-1, w-(s.x+1))
			if run > 0 {
				blit(canvas, s.x+1, sy+s.h, []string{stShadow.Render(strings.Repeat("▀", run))})
			}
			if s.x+s.w < w {
				blit(canvas, s.x+s.w, sy+s.h, []string{stShadow.Render("▘")})
			}
		}
	}
	if feedH < h {
		canvas = append(canvas, composeBar(m, w, feed[cur]))
	}
	return clip(canvas, h)
}

type cardSlot struct {
	idx, x, y, w, h int
}

func feedCols(w, n int) int {
	if n <= 1 || w < 36 {
		return 1
	}
	if w >= 120 && n >= 5 {
		return 4
	}
	if w >= 80 && n >= 3 {
		return 3
	}
	return 2
}

func maxBodyFor(b layout.Band) int {
	switch b {
	case layout.Phone:
		return 5
	case layout.Compact:
		return 8
	default:
		return 6
	}
}

// glanceSlots lays the feed out as a waterfall: fixed column widths, card
// height driven by its own content, next card dropped into the shortest
// column. The viewport moves only as far as needed to keep the selection
// visible (see glanceScroll).
func glanceSlots(m Model, w, viewH int) ([]cardSlot, int, []model.TabRow) {
	feed := feedTabs(m)
	n := len(feed)
	if n == 0 || w < 4 || viewH < 3 {
		return nil, 0, feed
	}
	cols := feedCols(w, n)
	gap := 0
	if w >= 42 && cols > 1 {
		gap = 1
	}
	inner := w - gap*(cols-1)
	cardW := inner / cols
	if cardW < 10 {
		cardW = 10
	}
	colX := make([]int, cols)
	colW := make([]int, cols)
	colH := make([]int, cols)
	x := 0
	for c := 0; c < cols; c++ {
		colX[c] = x
		if c == cols-1 {
			colW[c] = w - x
		} else {
			colW[c] = cardW
		}
		x += colW[c] + gap
	}
	maxBody := maxBodyFor(layout.BandOf(w))
	slots := make([]cardSlot, n)
	for i := 0; i < n; i++ {
		c := 0
		for j := 1; j < cols; j++ {
			if colH[j] < colH[c] {
				c = j
			}
		}
		ch := cardHeight(m, feed[i], colW[c], viewH, maxBody)
		slots[i] = cardSlot{idx: i, x: colX[c], y: colH[c], w: colW[c], h: ch}
		colH[c] += ch
	}
	cur := selIndex(feed, m.selID)
	if cur < 0 {
		cur = 0
	}
	return slots, glanceScroll(slots, cur, viewH, m.feedScroll), feed
}

// glanceScroll moves the viewport as little as possible: only when the
// selected card is off screen, and always to a card boundary so no card is
// ever cut off at the top. This keeps the wall from jumping under a finger.
func glanceScroll(slots []cardSlot, cur, viewH, prev int) int {
	if len(slots) == 0 || cur < 0 || cur >= len(slots) {
		return 0
	}
	maxY := 0
	for _, s := range slots {
		if s.y+s.h > maxY {
			maxY = s.y + s.h
		}
	}
	scroll := prev
	if scroll > maxY-viewH {
		scroll = maxY - viewH
	}
	if scroll < 0 {
		scroll = 0
	}
	sel := slots[cur]
	snapDown := func(target int) int { // largest card boundary <= target
		best := 0
		for _, s := range slots {
			if s.y <= target && s.y > best {
				best = s.y
			}
		}
		return best
	}
	snapUp := func(target, limit int) int { // smallest boundary in [target, limit]
		best := limit
		for _, s := range slots {
			if s.y >= target && s.y < best {
				best = s.y
			}
		}
		return best
	}
	switch {
	case sel.y < scroll:
		return sel.y
	case sel.y+sel.h > scroll+viewH:
		return snapUp(sel.y+sel.h-viewH, sel.y)
	default:
		scroll = snapDown(scroll)
		if sel.y+sel.h > scroll+viewH {
			return snapUp(sel.y+sel.h-viewH, sel.y)
		}
		return scroll
	}
}

func cardHeight(m Model, t model.TabRow, cardW, viewH, maxBody int) int {
	innerW := cardW - 2
	if innerW < 8 {
		innerW = 8
	}
	budget := innerW - 1
	if budget < 4 {
		budget = innerW
	}
	body := previewLines(m, t, budget, maxBody)
	bh := len(body)
	if bh < 1 {
		bh = 1
	}
	h := 2 + 1 + bh // border + meta + body
	if h < 5 {
		h = 5
	}
	peek := 2
	if viewH >= 8 && h > viewH-peek {
		h = viewH - peek
	}
	if h > viewH {
		h = viewH
	}
	if h < 4 {
		h = min(4, viewH)
	}
	return h
}

func previewLines(m Model, t model.TabRow, w, maxLines int) []string {
	return clipTranscript(cardPreview(m, t), w, maxLines)
}

func blit(dst []string, x, y int, src []string) {
	for i, line := range src {
		dy := y + i
		if dy < 0 || dy >= len(dst) {
			continue
		}
		dst[dy] = overlay(dst[dy], line, x)
	}
}

func overlay(base, piece string, x int) string {
	if x < 0 {
		x = 0
	}
	pw := layout.Width(stripANSI(piece))
	left, rest := layout.SplitCells(base, x)
	_, rest = layout.SplitCells(rest, pw)
	return left + piece + rest
}

func glanceIndexAt(m Model, x, y int) (int, bool) {
	feedH := glanceFeedH(m.height)
	slots, scroll, feed := glanceSlots(m, m.width, feedH)
	if len(feed) == 0 || m.width <= 0 || m.height <= 1 {
		return 0, false
	}
	for _, s := range slots {
		if s.y < scroll {
			continue
		}
		if x >= s.x && x < s.x+s.w && y+scroll >= s.y && y+scroll < s.y+s.h {
			return s.idx, true
		}
	}
	return 0, false
}

func glanceFeedH(viewH int) int {
	h := viewH - 1 // nav
	if h > 6 {
		h-- // compose dock
	}
	if h < 3 {
		h = 3
	}
	return h
}

func composeBar(m Model, w int, t model.TabRow) string {
	in := m.input
	if m.inputOn {
		who := t.TabLabel
		if who == "" {
			who = t.WSLabel
		}
		if who == "" {
			who = "agent"
		}
		in.Placeholder = "reply " + who + " · enter send · esc back"
	} else {
		in.Placeholder = "i reply · enter open · o jump · q quit"
	}
	in.Width = max(8, w-3)
	return exact(in.View(), w)
}

// statusEdge is the card border: color says herdr status, weight says
// selected. Blocked borders breathe with the 1s pulse — the sheep that
// bleats for you is the one that moves.
func statusEdge(status string, selected, bright bool) lipgloss.Style {
	var col lipgloss.Color
	switch status {
	case "working":
		col = cWarn // herdr yellow
	case "blocked":
		col = cBad // herdr red
		if !bright {
			col = cBadDim
		}
	case "done":
		col = cBlue // herdr blue
	case "idle":
		col = cGood // herdr green
	default:
		if selected {
			col = cLine
		} else {
			col = cFaint
		}
	}
	sty := lipgloss.NewStyle().Foreground(col)
	if selected {
		sty = sty.Bold(true)
	} else {
		sty = sty.Faint(true)
	}
	return sty
}

// chipFor tints the card's title chip with its status, xiaohongshu-cover
// style: the wall reads by color before it reads by text.
func chipFor(status string) lipgloss.Style {
	switch status {
	case "blocked":
		return stChipBlck
	case "working":
		return stChipWork
	case "done":
		return stChipDone
	case "idle":
		return stChipIdle
	default:
		return stSelR
	}
}

func statusWord(status string) string {
	switch status {
	case "working":
		return stWork.Render("work")
	case "blocked":
		return stBlck.Render("baa!")
	case "done":
		return stDone.Render("done")
	case "idle":
		return stIdle.Render("idle")
	case "now":
		return stKey.Render("now")
	default:
		return ""
	}
}

func plainStatusWord(status string) string {
	switch status {
	case "working":
		return "work"
	case "blocked":
		return "baa"
	case "done", "idle", "now":
		return status
	default:
		return ""
	}
}

func cardTitleFor(t model.TabRow) string {
	title := parseCardTitle(t.Title)
	if layout.Width(title) < 4 {
		title = t.TabLabel
	}
	if title == "" {
		title = t.WSLabel
	}
	if title == "" {
		title = "agent"
	}
	return title
}

func (m Model) ageOf(tabID string) string {
	if a, ok := m.ages[tabID]; ok {
		return fmtAge(time.Since(a.at))
	}
	return ""
}

func renderAgentCard(m Model, t model.TabRow, idx, total, w, h int, selected bool) []string {
	kind, status := tabKind(t)
	innerW := w - 2
	if innerW < 8 {
		innerW = 8
	}

	title := cardTitleFor(t)
	if selected {
		sheep := "🐑" // hops onto whichever card is picked…
		if m.pulse%4 == 3 {
			sheep = "🐏" // …and glances back now and then
		}
		title = sheep + " " + title
	}
	var extraBits []string
	if word := statusWord(status); word != "" {
		extraBits = append(extraBits, word)
	}
	if age := m.ageOf(t.TabID); age != "" && w >= 24 {
		extraBits = append(extraBits, stDim.Render(age))
	}
	extra := strings.Join(extraBits, " ")

	// meta line: where it lives, who is running, position in the deck.
	// Narrow cards keep the tab name whole instead of a truncated ws · tab.
	place := strings.TrimSpace(t.WSLabel)
	if innerW < 26 {
		place = t.TabLabel
		if place == "" {
			place = t.WSLabel
		}
	} else if t.TabLabel != "" && !strings.EqualFold(t.TabLabel, cardTitleFor(t)) {
		if place != "" {
			place += " · "
		}
		place += t.TabLabel
	}
	agent := displayAgent(kind, innerW < 30)
	if agent == "shell" {
		agent = ""
	}
	counter := ""
	if total > 1 && innerW >= 16 {
		counter = fmt.Sprintf("%d/%d", idx+1, total)
	}
	placeBudget := innerW - 1
	if agent != "" {
		placeBudget -= layout.Width(agent) + 2
	}
	if counter != "" {
		placeBudget -= layout.Width(counter) + 2
	}
	metaLeft := " " + stMute.Render(layout.Truncate(place, max(4, placeBudget)))
	if agent != "" {
		metaLeft += "  " + stDim.Render(agent)
	}
	meta := metaLeft
	if counter != "" {
		gap := innerW - layout.Width(stripANSI(metaLeft)) - layout.Width(counter) - 1
		if gap >= 1 {
			meta = metaLeft + strings.Repeat(" ", gap) + stDim.Render(counter) + " "
		}
	}

	budget := innerW - 1
	if budget < 4 {
		budget = innerW
	}
	bodyH := h - 3 // borders + meta
	if bodyH < 1 {
		bodyH = 1
	}
	raw := previewLines(m, t, budget, bodyH)
	var body []string
	if len(raw) == 0 {
		body = append(body, stDim.Render(" quiet"))
	} else {
		qs := -1
		if status == "blocked" {
			qs = questionStart(raw)
		}
		for i, ln := range raw {
			switch {
			case qs >= 0 && i >= qs:
				body = append(body, stText.Render(" "+ln))
			case qs >= 0:
				body = append(body, stDim.Render(" "+ln))
			default:
				body = append(body, stBody.Render(" "+ln))
			}
		}
	}

	inner := append([]string{meta}, body...)
	edge := statusEdge(status, selected, m.pulse%2 == 0)
	head := chipFor(status)
	return panelFrame(title, extra, inner, w, h, edge, head)
}

func cardPreview(m Model, t model.TabRow) string {
	if t.PaneID == "" {
		return ""
	}
	return m.previews[t.PaneID]
}

func blankBlock(w, h int) []string {
	out := make([]string, h)
	for i := range out {
		out[i] = strings.Repeat(" ", max(0, w))
	}
	return out
}

// ---------------------------------------------------------------- detail

// renderDetail is the "post" view: full transcript, reply box, no jump needed.
func renderDetail(m Model, w, h int) []string {
	row := m.detailRow()
	innerW := w - 2
	if innerW < 10 {
		innerW = 10
	}
	edge := stLine
	head := stHead
	title := "agent"
	extra := ""
	status := ""
	if row != nil {
		var kind string
		kind, status = tabKind(*row)
		title = cardTitleFor(*row)
		if word := plainStatusWord(status); word != "" {
			title = word + " · " + title
		}
		title = "🐑 " + title
		place := strings.TrimSpace(row.WSLabel + " · " + row.TabLabel)
		bits := []string{}
		if place != "·" && place != "" {
			bits = append(bits, place)
		}
		if a := displayAgent(kind, w < 60); a != "" && a != "shell" {
			bits = append(bits, a)
		}
		if age := m.ageOf(row.TabID); age != "" {
			bits = append(bits, age)
		}
		feed := feedTabs(m)
		if idx := selIndex(feed, m.detailTab); idx >= 0 && len(feed) > 1 {
			bits = append(bits, fmt.Sprintf("%d/%d", idx+1, len(feed)))
		}
		extra = stDim.Render(strings.Join(bits, " · "))
		edge = statusEdge(status, true, m.pulse%2 == 0)
		head = chipFor(status)
	}

	areaH := h - 4 // top border, separator, input, bottom border
	if areaH < 3 {
		areaH = 3
	}

	budget := innerW - 1
	var lines []string
	if row == nil {
		lines = []string{stErr.Render(" this tab is gone · esc back")}
	} else {
		raw := clipTranscript(m.transcript, budget, 4000)
		if len(raw) == 0 {
			lines = []string{stDim.Render(" nothing here yet · give it a second")}
		} else {
			qs := -1
			if status == "blocked" {
				qs = questionStart(raw)
			}
			for i, ln := range raw {
				if qs >= 0 && i >= qs {
					lines = append(lines, stText.Render(" "+ln))
				} else {
					lines = append(lines, stBody.Render(" "+ln))
				}
			}
		}
		for _, e := range m.echoes {
			if row.PaneID == "" || e.pane != row.PaneID {
				continue
			}
			sty := stKey
			prefix := "you › "
			if e.failed {
				sty = stErr
				prefix = "you ✗ "
			}
			for _, ln := range layout.Wrap(prefix+e.text, budget) {
				lines = append(lines, sty.Render(" "+ln))
			}
		}
	}

	eff := m.detailScroll
	if maxScroll := max(0, len(lines)-areaH); eff > maxScroll {
		eff = maxScroll
	}
	end := len(lines) - eff
	start := max(0, end-areaH)
	window := lines[start:end]
	// Long posts fill the window with the newest text just above the reply
	// box; short posts read from the top like a normal post instead of
	// floating at the bottom of an empty screen.
	for len(window) < areaH {
		window = append(window, "")
	}

	out := make([]string, 0, h)
	out = append(out, panelRule(true, title, extra, w, edge, head))
	for _, ln := range window {
		out = append(out, edge.Render("│")+exact(ln, innerW)+edge.Render("│"))
	}
	if eff > 0 {
		// scrolled up: replace separator with a "more below" marker
		out = append(out, edge.Render("├")+exact(stDim.Render(strings.Repeat("─", max(0, innerW-8))+" ↓ more "), innerW)+edge.Render("┤"))
	} else {
		out = append(out, edge.Render("├")+edge.Render(strings.Repeat("─", innerW))+edge.Render("┤"))
	}
	in := m.input
	hints := []string{" j/k flip · i reply · o jump · esc back ", " j/k flip · i reply · esc back ", " esc back "}
	if m.detailReply {
		in.Placeholder = "reply · enter send"
		hints = []string{" enter send · ctrl+o jump · esc done ", " enter send · esc done ", " esc done "}
	} else {
		in.Placeholder = "i reply"
	}
	in.Width = max(8, innerW-3)
	out = append(out, edge.Render("│")+exact(in.View(), innerW)+edge.Render("│"))
	hint := hints[len(hints)-1]
	for _, cand := range hints {
		if layout.Width(cand) <= innerW {
			hint = cand
			break
		}
	}
	dash := innerW - layout.Width(hint)
	if dash < 0 {
		dash = 0
	}
	out = append(out, edge.Render("╰")+stDim.Render(hint)+edge.Render(strings.Repeat("─", dash))+edge.Render("╯"))
	return clip(out, h)
}

// ---------------------------------------------------------------- help

// sheepArt is the resting sheep for quiet moments: empty pasture, help page.
func sheepArt() []string {
	return []string{
		stDim.Render("           z Z"),
		stWool.Render("     __  __"),
		stWool.Render("  o<(oo)____)~"),
		stWool.Render("    ''    ''"),
	}
}

func renderHelp(w, h int) []string {
	body := []string{
		stWool.Render("🐑") + "  " + stBrand.Render("PADDOCK") + stDim.Render("  glance over the flock"),
		"",
		"  " + stKey.Render("↑↓←→") + " move (hjkl)   " + stKey.Render("enter") + " zoom into the card",
		"  " + stKey.Render("o") + " jump to herdr   " + stKey.Render("i") + " quick reply",
		"  " + stDim.Render("zoomed: j/k next post · i reply · enter send · esc back"),
		"",
		"  " + stKey.Render("r") + " refresh   " + stKey.Render("q") + " quit   " + stKey.Render("?") + " close",
	}
	if h >= len(body)+len(sheepArt())+3 {
		body = append(body, "")
		body = append(body, sheepArt()...)
	}
	if w >= 56 {
		return panel("help", version, body, w, h)
	}
	return clip(body, h)
}

// ---------------------------------------------------------------- shared bits

func feedTabs(m Model) []model.TabRow {
	return model.FeedTabs(m.snap.Tabs, m.selfTab, m.selfPane, m.seen)
}

// clipTranscript cleans raw terminal text and keeps the tail, whole
// paragraphs first: the agent's last message survives intact instead of an
// arbitrary last-N-lines slice.
func clipTranscript(raw string, w, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	raw = strings.ReplaceAll(raw, "\r", "")
	var paras [][]string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			paras = append(paras, cur)
			cur = nil
		}
	}
	for _, ln := range dropAgentChrome(strings.Split(raw, "\n")) {
		useful := usefulLine(ln)
		if useful == "" {
			if strings.TrimSpace(ln) == "" {
				flush()
			}
			continue
		}
		cur = append(cur, layout.Wrap(useful, w)...)
	}
	flush()
	if len(paras) == 0 {
		return nil
	}
	kept := append([]string{}, paras[len(paras)-1]...)
	if len(kept) > maxLines {
		return kept[len(kept)-maxLines:]
	}
	for i := len(paras) - 2; i >= 0; i-- {
		need := len(paras[i]) + 1
		if len(kept)+need > maxLines {
			break
		}
		block := append([]string{}, paras[i]...)
		block = append(block, "")
		kept = append(block, kept...)
	}
	return kept
}

func usefulLine(ln string) string {
	ln = strings.TrimSpace(ln)
	if ln == "" {
		return ""
	}
	if barrierLine(ln) { // rules and box borders, labeled or not
		return ""
	}
	ln = strings.Trim(ln, "│┃|")
	ln = strings.TrimSpace(ln)
	if ln == "" || ln == "❯" || ln == "█" {
		return ""
	}
	// drop decorative box-only rows and lone "..." thinking leftovers
	stripped := strings.Trim(ln, "─━╭╮╰╯┌┐└┘═╔╗╚╝╠╣╦╩╬ ")
	if stripped == "" {
		return ""
	}
	if strings.Trim(ln, ".…·• ") == "" {
		return ""
	}
	if looksLikeChrome(ln) {
		return ""
	}
	return ln
}

func tabKind(t model.TabRow) (kind, status string) {
	kind, status = t.Agent, t.Status
	if kind == "" {
		kind = "shell"
	}
	if status == "unknown" || status == "" {
		if t.Focused {
			status = "now"
		} else {
			status = ""
		}
	}
	return kind, status
}

func displayAgent(kind string, short bool) string {
	switch kind {
	case "", "shell":
		if short {
			return ""
		}
		return "shell"
	case "agy":
		if short {
			return "agy"
		}
		return "antigravity"
	default:
		return kind
	}
}

// ---------------------------------------------------------------- status bar

// flockBar draws the herd as a strip of colored wool, one cell per sheep
// (proportionally squeezed when the flock outgrows the row): red bleating,
// yellow grazing, blue resting, green wandering.
func flockBar(tabs []model.TabRow, maxCells int) string {
	if maxCells < 4 {
		return ""
	}
	groups := []struct {
		n     int
		glyph string
		sty   lipgloss.Style
	}{
		{model.CountStatus(tabs, "blocked"), "█", stBlck},
		{model.CountStatus(tabs, "working"), "▓", stWork},
		{model.CountStatus(tabs, "done"), "▒", stDone},
		{model.CountStatus(tabs, "idle"), "░", stIdle},
	}
	total := 0
	for _, g := range groups {
		total += g.n
	}
	if total == 0 {
		return ""
	}
	scale := func(n int) int {
		if n == 0 {
			return 0
		}
		if total <= maxCells {
			return n
		}
		c := n * maxCells / total
		if c < 1 {
			c = 1 // every present state stays visible
		}
		return c
	}
	var b strings.Builder
	for _, g := range groups {
		if c := scale(g.n); c > 0 {
			b.WriteString(g.sty.Render(strings.Repeat(g.glyph, c)))
		}
	}
	return b.String()
}

func statusLine(m Model, w int) string {
	left := " " + stWool.Render("🐑") + " " + stBrand.Render("paddock")
	if n := len(feedTabs(m)); n > 0 {
		left += " " + stDim.Render(fmt.Sprintf("%d", n))
	}
	if bar := flockBar(m.snap.Tabs, min(12, max(4, w/6))); bar != "" && w >= 36 {
		left += " " + bar
	}

	sepR := " · "
	if w < 52 {
		sepR = " "
	}
	// display order; dropPri: what gives way first when the row is tight
	type bit struct {
		s       string
		dropPri int
	}
	var bits []bit
	if m.snap.Herdr.Health == model.HealthStale {
		// last poll failed; the wall still shows the previous good snapshot
		bits = append(bits, bit{stErr.Render("stale"), 3})
	}
	if m.actionNote != "" {
		sty := stKey
		if strings.Contains(m.actionNote, "failed") {
			sty = stErr
		}
		bits = append(bits, bit{sty.Render(layout.Truncate(m.actionNote, max(8, w/3))), 2})
	}
	nBlock := model.CountStatus(m.snap.Tabs, "blocked")
	nWork := model.CountStatus(m.snap.Tabs, "working")
	if nBlock > 0 {
		bits = append(bits, bit{stBlck.Render(fmt.Sprintf("%d baa", nBlock)), 3})
	}
	if nWork > 0 {
		bits = append(bits, bit{stWork.Render(fmt.Sprintf("%d work", nWork)), 1})
	}
	bits = append(bits, bit{stBar.Render(time.Now().Format("15:04")), 0})

	join := func() string {
		ss := make([]string, len(bits))
		for i, b := range bits {
			ss[i] = b.s
		}
		return strings.Join(ss, stDim.Render(sepR))
	}
	right := join()
	for len(bits) > 1 && w-lipgloss.Width(left)-lipgloss.Width(right) < 1 {
		drop, pri := -1, -1
		for i, b := range bits {
			if b.dropPri < 3 && b.dropPri > pri { // never drop the ask count
				drop, pri = i, b.dropPri
			}
		}
		if drop < 0 {
			break
		}
		bits = append(bits[:drop], bits[drop+1:]...)
		right = join()
	}

	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func herdrErr(code string) string {
	if code == "no-herdr" {
		return "herdr is not running"
	}
	return "herdr unavailable"
}

// ---------------------------------------------------------------- frames

func cardPanel(title, extra string, inner []string, width, height int, edge lipgloss.Style) []string {
	return panelFrame(title, extra, inner, width, height, edge, edge)
}

func panel(title, extra string, inner []string, width, height int) []string {
	return panelFrame(title, extra, inner, width, height, stLine, stHead)
}

func panelFrame(title, extra string, inner []string, width, height int, edge, head lipgloss.Style) []string {
	if width < 10 || height < 2 {
		return clip(inner, height)
	}
	innerW := width - 2
	innerH := height - 2
	if innerH < 1 {
		innerH = 1
	}
	body := make([]string, innerH)
	for i := 0; i < innerH; i++ {
		src := ""
		if i < len(inner) {
			src = inner[i]
		}
		body[i] = edge.Render("│") + exact(src, innerW) + edge.Render("│")
	}
	top := panelRule(true, title, extra, width, edge, head)
	bot := panelRule(false, "", "", width, edge, head)
	out := make([]string, 0, height)
	out = append(out, top)
	out = append(out, body...)
	out = append(out, bot)
	return out
}

func panelRule(top bool, title, extra string, width int, edge, head lipgloss.Style) string {
	left, right := "╭", "╮"
	if !top {
		left, right = "╰", "╯"
	}
	fill := width - 2
	if fill < 0 {
		fill = 0
	}
	mid := strings.Repeat("─", fill)
	if top && title != "" {
		rest := ""
		if extra != "" {
			rest = " " + extra + " "
		}
		restW := layout.Width(stripANSI(rest))
		if restW > fill-4 {
			rest = " " + layout.Truncate(stripANSI(rest), max(1, fill/3)) + " "
			restW = layout.Width(stripANSI(rest))
		}
		labelBudget := fill - restW - 1
		if labelBudget < 3 {
			labelBudget = 3
		}
		label := " " + layout.Truncate(title, max(1, labelBudget-2)) + " "
		dash := fill - layout.Width(label) - restW
		if dash < 0 {
			dash = 0
		}
		mid = head.Render(label) + edge.Render(strings.Repeat("─", dash)) + rest
	}
	return edge.Render(left) + mid + edge.Render(right)
}

func exact(s string, w int) string {
	s = stripNL(s)
	vis := lipgloss.Width(s)
	if vis == w {
		return s
	}
	if vis > w {
		return layout.Clip(stripANSI(s), w)
	}
	return s + strings.Repeat(" ", w-vis)
}

func clip(lines []string, h int) []string {
	if h < 0 {
		return nil
	}
	if len(lines) > h {
		return lines[:h]
	}
	return lines
}

func stripNL(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", "")
}

// stripANSI removes escape sequences for width math and tests. It delegates
// to the full herdr sanitizer so a non-SGR sequence (cursor moves, erase)
// can never swallow adjacent text the way a naive scan-to-'m' would.
func stripANSI(s string) string {
	if !strings.ContainsRune(s, 0x1b) {
		return s
	}
	return herdr.Sanitize(s)
}
