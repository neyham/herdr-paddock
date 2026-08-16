package layout

import (
	"strings"
	"testing"
)

func TestBandOf(t *testing.T) {
	if BandOf(40) != Phone {
		t.Fatalf("40 -> %s", BandOf(40))
	}
	if BandOf(52) != Phone {
		t.Fatalf("52 -> %s", BandOf(52))
	}
	if BandOf(53) != Compact {
		t.Fatalf("53 -> %s", BandOf(53))
	}
	if BandOf(80) != Full {
		t.Fatalf("80 -> %s", BandOf(80))
	}
}

func TestTruncateCJK(t *testing.T) {
	s := Truncate("会计视频脚本很长很长", 8)
	if Width(s) > 8 {
		t.Fatalf("width %d > 8: %q", Width(s), s)
	}
	if !containsEllipsis(s) {
		t.Fatalf("expected ellipsis: %q", s)
	}
	if Width(Truncate("abc", 10)) != 3 {
		t.Fatalf("short string changed")
	}
}

func TestSplitCells(t *testing.T) {
	left, right := SplitCells("abcdef", 3)
	if left != "abc" || right != "def" {
		t.Fatalf("%q %q", left, right)
	}
}

func TestClipHasNoEllipsis(t *testing.T) {
	s := Clip("waiting on the provider docs wording", 10)
	if Width(s) > 10 {
		t.Fatalf("width %d: %q", Width(s), s)
	}
	if containsEllipsis(s) {
		t.Fatalf("clip should not add ellipsis: %q", s)
	}
}

func TestWrapKeepsTail(t *testing.T) {
	lines := Wrap("snapshot has workspaces tabs and agents", 16)
	if len(lines) < 2 {
		t.Fatalf("expected wrap, got %#v", lines)
	}
	joined := strings.Join(lines, "")
	for _, ln := range lines {
		if Width(ln) > 16 {
			t.Fatalf("line too wide %q", ln)
		}
		if containsEllipsis(ln) {
			t.Fatalf("wrap should not add ellipsis: %q", ln)
		}
	}
	if !strings.Contains(joined, "agents") {
		t.Fatalf("lost tail: %#v", lines)
	}
	for _, ln := range lines {
		if strings.HasPrefix(ln, "ts ") || strings.HasSuffix(ln, " agen") {
			t.Fatalf("split a word: %#v", lines)
		}
	}
}

func containsEllipsis(s string) bool {
	for _, r := range s {
		if r == '…' {
			return true
		}
	}
	return false
}
