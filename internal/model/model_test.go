package model

import "testing"

func TestFeedTabs(t *testing.T) {
	tabs := []TabRow{
		{TabID: "a", Status: "idle", TabLabel: "A", Agent: "pi"},
		{TabID: "b", Status: "working", TabLabel: "B", Agent: "grok"},
		{TabID: "c", Status: "blocked", TabLabel: "C", Agent: "codex"},
		{TabID: "d", Status: "unknown", TabLabel: "neyham", Agent: "", Focused: true},
		{TabID: "e", Status: "done", TabLabel: "E", Agent: "agy"},
		{TabID: "self", Status: "working", TabLabel: "neyham", Agent: "grok"},
	}
	feed := FeedTabs(tabs, "self", "", nil)
	if len(feed) != 4 {
		t.Fatalf("want 4 agent cards, got %+v", feed)
	}
	if feed[0].TabID != "c" || feed[1].TabID != "b" || feed[2].TabID != "e" || feed[3].TabID != "a" {
		t.Fatalf("order should be blocked, working, done, idle: %+v", feed)
	}
}

func TestFeedTabsStableWithinGroup(t *testing.T) {
	tabs := []TabRow{
		{TabID: "x", Status: "working", TabLabel: "X", Agent: "pi"},
		{TabID: "y", Status: "working", TabLabel: "Y", Agent: "grok"},
	}
	// y was seen first: it must sort ahead of x even though the snapshot
	// listed x first.
	order := map[string]int{"y": 0, "x": 1}
	feed := FeedTabs(tabs, "", "", order)
	if len(feed) != 2 || feed[0].TabID != "y" || feed[1].TabID != "x" {
		t.Fatalf("first-seen order should win within a group: %+v", feed)
	}
}
