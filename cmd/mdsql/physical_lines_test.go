package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPhysicalLineCountsInCLI(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		count int
	}{
		{"", 0},
		{"\n", 1},
		{"---\n", 1},
		{"foo", 1},
		{"foo\n", 1},
	} {
		path := filepath.Join(t.TempDir(), "lines.md")
		if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
			t.Fatal(err)
		}
		code, out, stderr := captureFeedbackRun(t, []string{"--strict", "--json", "SELECT lines FROM files", path})
		if code != 0 || out != fmt.Sprintf("[{\"lines\":%d}]\n", tc.count) || stderr != "" {
			t.Fatalf("files.lines for %q: %d %q %q", tc.raw, code, out, stderr)
		}
		code, out, stderr = captureFeedbackRun(t, []string{"--strict", "--json", "SELECT n FROM lines", path})
		expected := "[{\"n\":1}]\n"
		if tc.count == 0 {
			expected = "[]\n"
		}
		if code != 0 || out != expected || stderr != "" {
			t.Fatalf("line rows for %q: %d %q %q", tc.raw, code, out, stderr)
		}
	}
}
