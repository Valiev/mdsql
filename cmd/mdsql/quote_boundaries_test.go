package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtBlankBetweenQuotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quotes.md")
	if err := os.WriteFile(path, []byte("> first\n\n> second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := captureFeedbackRun(t, []string{"--json", "at", path + ":2"})
	if code != 0 || out != "[]\n" || stderr != "" {
		t.Fatalf("blank line lookup: %d %q %q", code, out, stderr)
	}
}
