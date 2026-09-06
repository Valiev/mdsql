package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/query"
)

func TestCLIAggregatesAndFloatFormats(t *testing.T) {
	dir := t.TempDir()
	for i, fm := range []string{"value: 1\ngroup: a", "value: 2.5\ngroup: a", "value: null\ngroup: b"} {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.md", i)), []byte("---\n"+fm+"\n---\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ sql, format, want string }{
		{"SELECT count(*), count(fm.value) AS present, sum(fm.value), min(fm.value), max(fm.value), avg(fm.value) FROM frontmatter", "json", `[{"avg":1.75,"count":3,"max":2.5,"min":1,"present":2,"sum":3.5}]` + "\n"},
		{"SELECT sum(fm.value) AS total, avg(fm.value) AS mean FROM frontmatter", "csv", "total,mean\n3.5,1.75\n"},
		{"SELECT sum(fm.value) AS total, avg(fm.value) AS mean FROM frontmatter", "tsv", "total\tmean\n3.5\t1.75\n"},
		{"SELECT sum(fm.value) AS total, avg(fm.value) AS mean FROM frontmatter", "text", "total=3.5\tmean=1.75\n"},
		{"SELECT count(*), count(fm.value) AS present, avg(fm.value) FROM frontmatter WHERE false", "json", `[{"avg":null,"count":0,"present":0}]` + "\n"},
		{"SELECT fm.group AS category, avg(fm.value) AS mean, count(fm.value) AS present FROM frontmatter GROUP BY category ORDER BY category", "json", `[{"category":"a","mean":1.75,"present":2},{"category":"b","mean":null,"present":0}]` + "\n"},
		{"SELECT avg(fm.value) + 1 AS value FROM frontmatter", "json", `[{"value":2.75}]` + "\n"},
		{"SELECT round(avg(fm.value), 1) AS value FROM frontmatter", "json", `[{"value":1.8}]` + "\n"},
	} {
		code, out, errOut := captureFeedbackRun(t, []string{"--strict", "--format", tc.format, tc.sql, dir})
		if code != 0 || out != tc.want || errOut != "" {
			t.Fatalf("SQL=%s format=%s code=%d output=%q want=%q stderr=%q", tc.sql, tc.format, code, out, tc.want, errOut)
		}
	}
}

func TestMetadataOnlyAggregates(t *testing.T) {
	for _, tc := range []struct {
		sql  string
		want bool
	}{
		{"SELECT sum(size), avg(size), min(mtime), max(name), count(name), count(*) FROM files", true},
		{"SELECT max(size) FROM files ORDER BY min(size)", true},
		{"SELECT sum(words) FROM files", false},
		{"SELECT count(fm.value) FROM files", false},
		{"SELECT sum(size) FROM files WHERE fm.value = 1", false},
	} {
		q, err := query.Parse(tc.sql)
		if err != nil {
			t.Fatal(err)
		}
		if got := metadataOnly(q); got != tc.want {
			t.Errorf("%s: metadataOnly=%v want=%v", tc.sql, got, tc.want)
		}
	}
	path := filepath.Join(t.TempDir(), "malformed.md")
	data := "---\ninvalid: [\n---\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := captureFeedbackRun(t, []string{"--strict", "--json", "SELECT sum(size) AS total, avg(size) AS mean, count(name) AS present FROM files", path})
	var rows []map[string]float64
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(rows) != 1 || rows[0]["total"] != float64(len(data)) || rows[0]["mean"] != float64(len(data)) || rows[0]["present"] != 1 || errOut != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestCLIAggregateErrorsAndFloatOverflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(path, []byte("---\nvalue: wrong\nlarge: 1.0e308\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"SELECT sum(fm.value) AS value FROM frontmatter",
		"SELECT fm.large + fm.large AS value FROM frontmatter",
		"SELECT max(fm.large) + max(fm.large) AS value FROM frontmatter",
	} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", sql, path})
		if code != 0 || out != "[{\"value\":null}]\n" || !strings.Contains(errOut, "mdsql: warning:") {
			t.Fatalf("SQL=%s code=%d out=%q err=%q", sql, code, out, errOut)
		}
		code, out, errOut = captureFeedbackRun(t, []string{"--strict", "--json", sql, path})
		if code != 1 || out != "" || errOut == "" {
			t.Fatalf("strict SQL=%s code=%d out=%q err=%q", sql, code, out, errOut)
		}
	}
	code, out, errOut := captureFeedbackRun(t, []string{"--json", "SELECT fm.value FROM frontmatter WHERE fm.large + fm.large > 0", path})
	if code != 0 || out != "[]\n" || !strings.Contains(errOut, "numeric overflow") {
		t.Fatalf("overflow comparison code=%d out=%q err=%q", code, out, errOut)
	}
}
