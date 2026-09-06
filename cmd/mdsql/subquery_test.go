package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIMembershipSubquery(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"active.md": "---\nstatus: active\n---\n# Work\n- [ ] included\n",
		"paused.md": "---\nstatus: paused\n---\n# Work\n- [ ] excluded\n",
	}
	for name, markdown := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(markdown), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sql := "SELECT file, text FROM tasks WHERE file IN (SELECT file FROM frontmatter WHERE fm.status = 'active')"
	code, out, errOut := captureFeedbackRun(t, []string{"--strict", "--json", "--root", dir, sql, "."})
	if code != 0 || out != `[{"file":"active.md","text":"included"}]`+"\n" || errOut != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestExplainShowsSubqueryExecutionPlan(t *testing.T) {
	sql := "SELECT file FROM tasks WHERE file IN (SELECT file FROM frontmatter WHERE fm.status = 'active')"
	code, out, errOut := captureFeedbackRun(t, []string{"--explain", sql})
	if code != 0 || errOut != "" || out != sql+"\nsubquery 1: execute once: SELECT file FROM frontmatter WHERE fm.status = 'active'\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if !strings.Contains(out, "execute once") {
		t.Fatal("explain output does not describe one-time execution")
	}
}
