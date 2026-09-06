package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIAfterHeadingWithPerFileLimit(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.md", "b.md"} {
		content := "# Before\n- old\n## Log\n- first\n## Later\n- second\n- third\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	code, out, errOut := captureFeedbackRun(t, []string{"--format", "plain", "SELECT text FROM lines WHERE after_heading('Log') AND text LIKE '- %' ORDER BY file, n DESC LIMIT 2 PER file", dir})
	if code != 0 || out != "- third\n- second\n- third\n- second\n" || errOut != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}
