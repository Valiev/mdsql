package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDivisionByZeroWarningIsDeduplicatedAndStrict(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# Note\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sql := "SELECT size / (size - size) AS value FROM files"
	code, out, errOut := captureFeedbackRun(t, []string{"--json", sql, dir})
	if code != 0 || out != "[{\"value\":null},{\"value\":null}]\n" || strings.Count(errOut, "division by zero") != 1 {
		t.Fatalf("code=%d output=%q stderr=%q", code, out, errOut)
	}
	code, out, errOut = captureFeedbackRun(t, []string{"--strict", "--json", sql, dir})
	if code != 1 || out != "" || !strings.Contains(errOut, "division by zero") {
		t.Fatalf("strict code=%d output=%q stderr=%q", code, out, errOut)
	}
}
