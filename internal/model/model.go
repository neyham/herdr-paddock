package model

import (
	"sort"
	"time"
)

type Health string

const (
	HealthOK    Health = "ok"
	HealthStale Health = "stale"
	HealthErr   Health = "err"
)

type SourceState struct {
	Source    string
	Health    Health
	ErrCode   string
	UpdatedAt time.Time
}

type TabRow struct {
	TabID       string
	WorkspaceID string
	WSLabel     string
	TabLabel    string
	Agent       string
	Status      string
	Title       string
	Cwd         string
	PaneID      string
	Focused     bool
	Revision    int
	StateSeq    int
}

type Snapshot struct {
	Herdr      SourceState
	Tabs       []TabRow
	Workspaces int
}

func IsSelf(t TabRow, selfTab, selfPane string) bool {
	if selfTab != "" && t.TabID == selfTab {
		return true
	}
	if selfPane != "" && t.PaneID == selfPane {
		return true
	}
	return false
}

func IsAgentTab(t TabRow) bool {
	if t.Agent == "" || t.Agent == "shell" {
		return false
	}
	switch t.Status {
	case "working", "blocked", "idle", "done":
		return true
	default:
		return false
	}
}

// FeedTabs is the glance deck: every real agent except this paddock pane.
// blocked / working first so you flip into the ones that need you.
// Within a status group tabs keep first-seen order (via the order map) so
// cards don't swap places under the reader on every snapshot.
func FeedTabs(tabs []TabRow, selfTab, selfPane string, order map[string]int) []TabRow {
	var blocked, working, done, idle []TabRow
	for _, t := range tabs {
		if IsSelf(t, selfTab, selfPane) || !IsAgentTab(t) {
			continue
		}
		switch t.Status {
		case "blocked":
			blocked = append(blocked, t)
		case "working":
			working = append(working, t)
		case "done":
			done = append(done, t)
		default:
			idle = append(idle, t)
		}
	}
	byOrder := func(group []TabRow) {
		if len(order) == 0 {
			return
		}
		sort.SliceStable(group, func(i, j int) bool {
			oi, ok := order[group[i].TabID]
			if !ok {
				oi = 1 << 30
			}
			oj, ok := order[group[j].TabID]
			if !ok {
				oj = 1 << 30
			}
			return oi < oj
		})
	}
	byOrder(blocked)
	byOrder(working)
	byOrder(done)
	byOrder(idle)
	out := make([]TabRow, 0, len(blocked)+len(working)+len(done)+len(idle))
	out = append(out, blocked...)
	out = append(out, working...)
	out = append(out, done...)
	out = append(out, idle...)
	return out
}

func CountStatus(tabs []TabRow, status string) int {
	n := 0
	for _, t := range tabs {
		if t.Status == status {
			n++
		}
	}
	return n
}
