package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExplicitLimitOverridesDefaultCap(t *testing.T) {
	path := maxRowsInput(t, 620)
	for _, tc := range []struct {
		sql         string
		flags       []string
		count, code int
	}{
		{"SELECT title FROM headings LIMIT 600", nil, 600, 0},
		{"SELECT title FROM headings LIMIT 0", nil, 620, 0},
		{"SELECT title FROM headings LIMIT 600", []string{"--max-rows=500"}, 500, 0},
		{"SELECT title FROM headings LIMIT 600", []string{"--strict", "--quiet", "--max-rows=500"}, 500, 4},
		{"SELECT title FROM headings LIMIT 600", []string{"--strict"}, 600, 0},
		{"SELECT title FROM headings", nil, 500, 0},
	} {
		code, out, stderr := captureFeedbackRun(t, append([]string{"--json", tc.sql, path}, tc.flags...))
		var rows []map[string]any
		if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != tc.count || code != tc.code {
			t.Fatalf("%s %v: %d rows=%d err=%v stderr=%q", tc.sql, tc.flags, code, len(rows), err, stderr)
		}
		if tc.code == 4 && !strings.Contains(stderr, "truncated") {
			t.Fatalf("strict truncation diagnostic missing: %q", stderr)
		}
		if tc.count >= 600 && stderr != "" {
			t.Fatalf("explicit limit warns: %q", stderr)
		}
	}
}
