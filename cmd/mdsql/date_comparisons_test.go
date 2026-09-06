package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIQuotedDateAndTimestampComparisons(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(path, []byte("---\ncreated: 2026-09-03\nstamp: 2026-09-03T03:00:00+03:00\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, predicate := range []string{
		"fm.created = '2026-09-03'", "fm.created < '2026-09-04'", "fm.created >= '2026-09-01'",
		"fm.created BETWEEN '2026-09-01' AND '2026-09-04'", "fm.created IN ('2026-09-02', '2026-09-03')",
		"fm.stamp = '2026-09-03T00:00:00Z'", "fm.stamp < '2026-09-04T00:00:00Z'", "fm.stamp >= '2026-09-01'",
		"fm.stamp BETWEEN '2026-09-01' AND '2026-09-04'", "fm.stamp IN ('2026-09-03T00:00:00Z')",
	} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", "--strict", "SELECT true AS matched FROM frontmatter WHERE " + predicate, path})
		if code != 0 || out != "[{\"matched\":true}]\n" || errOut != "" {
			t.Fatalf("predicate=%q code=%d stdout=%q stderr=%q", predicate, code, out, errOut)
		}
	}
	for _, predicate := range []string{"fm.created = 'invalid'", "fm.stamp IN ('invalid')", "fm.created IN ('2026-09-03', 'invalid')"} {
		sql := "SELECT true AS matched FROM frontmatter WHERE " + predicate
		code, out, errOut := captureFeedbackRun(t, []string{"--json", sql, path})
		if code != 0 || out == "" || !strings.Contains(errOut, "incompatible comparison") {
			t.Fatalf("predicate=%q code=%d stdout=%q stderr=%q", predicate, code, out, errOut)
		}
		code, out, errOut = captureFeedbackRun(t, []string{"--json", "--strict", sql, path})
		if code != 1 || out != "" || !strings.Contains(errOut, "incompatible comparison") {
			t.Fatalf("strict predicate=%q code=%d stdout=%q stderr=%q", predicate, code, out, errOut)
		}
	}
}
