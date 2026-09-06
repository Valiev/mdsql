package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootRequiresAnExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	if err := os.WriteFile(file, []byte("# Note\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{file, filepath.Join(dir, "absent")} {
		for _, mode := range []string{"--strict", "--quiet", "--explain"} {
			code, out, stderr := captureFeedbackRun(t, []string{mode, "--root", root, "SELECT file FROM files", "."})
			if code != 1 || out != "" || !strings.Contains(stderr, "invalid value for --root") {
				t.Errorf("root=%s mode=%s code=%d out=%q stderr=%q", root, mode, code, out, stderr)
			}
			if root == file && !strings.Contains(stderr, "root is not a directory") {
				t.Errorf("error=%q", stderr)
			}
		}
	}
	code, out, stderr := captureFeedbackRun(t, []string{"--strict", "--json", "--root", dir, "SELECT file FROM files", "."})
	if code != 0 || strings.TrimSpace(out) != `[{"file":"note.md"}]` || stderr != "" {
		t.Fatalf("directory: %d %q %q", code, out, stderr)
	}
}
