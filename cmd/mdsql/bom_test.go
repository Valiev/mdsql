package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBOMFileAndStdin(t *testing.T) {
	const plain = "---\ntitle: Note\n---\n# Heading\nBody.\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "bom.md")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	for _, sql := range []string{
		"SELECT fm.title, raw, start, end FROM frontmatter",
		"SELECT title, start, raw FROM headings",
		"SELECT n, text FROM lines",
	} {
		t.Run(sql, func(t *testing.T) {
			args := []string{"--strict", "--json", sql}
			code, expected, stderr := stdinRun(t, plain, args)
			if code != 0 || stderr != "" {
				t.Fatalf("plain stdin: %d %q %q", code, expected, stderr)
			}
			code, out, stderr := stdinRun(t, "\uFEFF"+plain, args)
			if code != 0 || out != expected || stderr != "" {
				t.Fatalf("BOM stdin: %d %q %q, want %q", code, out, stderr, expected)
			}
			if err := os.WriteFile(path, []byte("\uFEFF"+plain), 0600); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				code, out, stderr = captureFeedbackRun(t, append(args, "--cache", path))
				if code != 0 || out != expected || stderr != "" || strings.Contains(out, "\uFEFF") {
					t.Fatalf("BOM file run %d: %d %q %q, want %q", i, code, out, stderr, expected)
				}
			}
		})
	}
}
