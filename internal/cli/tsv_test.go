package cli

import "testing"

func TestTSVShortcut(t *testing.T) {
	for _, tc := range []struct {
		flags  []string
		format string
	}{
		{[]string{"--tsv", "--no-header"}, "tsv"},
		{[]string{"--json", "--tsv"}, "tsv"},
		{[]string{"--tsv", "--json"}, "json"},
		{[]string{"--tsv", "--csv"}, "csv"},
	} {
		cfg, err := ParseArgs(append(tc.flags, "SELECT title FROM headings", "note.md"))
		if err != nil || cfg.Format != tc.format || cfg.Json != (tc.format == "json") {
			t.Fatalf("%v: config=%+v error=%v", tc.flags, cfg, err)
		}
	}
}
