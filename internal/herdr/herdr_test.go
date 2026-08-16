package herdr

import "testing"

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
