package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrokenFrontmatterDiagnostics(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
	path := filepath.Join(base, "broken.md")
	for _, yaml := range []string{"title: [unclosed", "bad: : x"} {
		if err := os.WriteFile(path, []byte("---\n"+yaml+"\n---\n# Body\n"), 0600); err != nil {
			t.Fatal(err)
		}
		for _, flags := range [][]string{nil, {"--cache"}, {"--cache"}, {"--strict"}, {"--cache", "--strict"}, {"--quiet"}} {
			code, out, stderr := captureFeedbackRun(t, append(flags, "--json", "SELECT title FROM headings", path))
			joined := strings.Join(flags, " ")
			if strings.Contains(joined, "--strict") {
				if code != 2 || out != "" || !strings.Contains(stderr, "invalid frontmatter") {
					t.Fatalf("strict %v: %d %q %q", flags, code, out, stderr)
				}
			} else {
				if code != 0 || !strings.Contains(out, "Body") || strings.Contains(out, yaml) {
					t.Fatalf("body %v: %d %q %q", flags, code, out, stderr)
				}
				if !strings.Contains(joined, "--quiet") && (!strings.Contains(stderr, "broken.md:2:") || !strings.Contains(stderr, "warning:")) {
					t.Fatalf("missing warning: %q", stderr)
				}
			}
		}
		code, out, stderr := captureFeedbackRun(t, []string{"--quiet", "--json", "SELECT text FROM lines WHERE kind != 'frontmatter'", path})
		if code != 0 || strings.Contains(out, "unclosed") || strings.Contains(out, "bad:") || stderr != "" {
			t.Fatalf("body lines: %d %q %q", code, out, stderr)
		}
	}
}
