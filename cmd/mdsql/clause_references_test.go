package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIOrderPositionsAndWhereAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(path, []byte("# B\n## A\n# C\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ sql, want string }{
		{"SELECT level, title FROM headings ORDER BY 2 DESC", `[{"level":1,"title":"C"},{"level":1,"title":"B"},{"level":2,"title":"A"}]`},
		{"SELECT lower(title) AS t FROM headings WHERE t = 'a'", `[{"t":"a"}]`},
		{"SELECT lower(title) AS title FROM headings WHERE title = 'A'", `[{"title":"a"}]`},
		{"SELECT level AS category, count(*) AS total FROM headings WHERE category = 1 GROUP BY category ORDER BY 2", `[{"category":1,"total":2}]`},
	} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", "--strict", tc.sql, path})
		if code != 0 || out != tc.want+"\n" || errOut != "" {
			t.Fatalf("SQL=%s code=%d stdout=%q stderr=%q", tc.sql, code, out, errOut)
		}
	}
	for _, sql := range []string{
		"SELECT title FROM headings ORDER BY 99", "SELECT title FROM headings ORDER BY 0", "SELECT title FROM headings ORDER BY -1",
		"SELECT count(*) AS total FROM headings WHERE false AND total > 0",
	} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", sql, path})
		if code != 1 || out != "" || !(strings.Contains(errOut, "ORDER BY position") || strings.Contains(errOut, "aggregate functions are not allowed in WHERE")) {
			t.Fatalf("SQL=%s code=%d stdout=%q stderr=%q", sql, code, out, errOut)
		}
	}
}
