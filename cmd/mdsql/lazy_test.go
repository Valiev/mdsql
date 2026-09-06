package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/query"
)

func TestMetadataOnlyPlan(t *testing.T) {
	for _, tc := range []struct {
		sql  string
		want bool
	}{
		{"SELECT count(*) FROM files", true},
		{"SELECT file, size, mtime FROM files WHERE size BETWEEN 1 AND 9 ORDER BY file", true},
		{"SELECT name FROM files WHERE lower(name) = 'x' AND mtime IS NOT NULL", true},
		{"SELECT * FROM files", false},
		{"SELECT count(*) FROM files WHERE fm.status = 'active'", false},
		{"SELECT file FROM files ORDER BY words", false},
		{"SELECT words FROM files", false},
		{"SELECT count(*) FROM lines", false},
		{"SELECT dir, count(*) FROM files GROUP BY dir", false},
		{"SELECT file FROM files WHERE file IN (SELECT file FROM files WHERE size > 0)", true},
		{"SELECT file FROM files WHERE file IN (SELECT file FROM headings)", false},
	} {
		q, err := query.Parse(tc.sql)
		if err != nil {
			t.Fatal(err)
		}
		if got := metadataOnly(q); got != tc.want {
			t.Errorf("%s: got %v", tc.sql, got)
		}
	}
}

func TestFilesMetadataSkipsParserButMetricsDoNot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.md")
	data := "---\ninvalid: [\n---\n# Heading\n- [ ] Task\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("MDSQL_CACHE", "1")
	code, out, errOut := captureFeedbackRun(t, []string{"--strict", "--json", "SELECT count(*) FROM files", path})
	if code != 0 || out != "[{\"count\":1}]\n" || errOut != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	entries, err := os.ReadDir(cacheHome)
	if err != nil || len(entries) != 0 {
		t.Fatalf("metadata query created cache: %v %v", entries, err)
	}
	code, _, errOut = captureFeedbackRun(t, []string{"--strict", "--json", "SELECT tasks FROM files", path})
	if code != 2 || !strings.Contains(errOut, "frontmatter") {
		t.Fatalf("full parser not used: code=%d err=%q", code, errOut)
	}
	code, out, _ = captureFeedbackRun(t, []string{"--quiet", "--json", "SELECT headings, tasks FROM files", path})
	if code != 0 || out != "[{\"headings\":1,\"tasks\":1}]\n" {
		t.Fatalf("wrong file metrics: code=%d out=%q", code, out)
	}
	code, _, errOut = captureFeedbackRun(t, []string{"--strict", "--max-file-size=1", "SELECT count(*) FROM files", path})
	if code != 2 || !strings.Contains(errOut, "file too large") {
		t.Fatalf("size limit bypassed: code=%d err=%q", code, errOut)
	}
}
