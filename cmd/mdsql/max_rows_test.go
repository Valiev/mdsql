package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func maxRowsInput(t *testing.T, count int) string {
	t.Helper()
	var text strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&text, "%s Row%03d\n", strings.Repeat("#", i%2+1), i+1)
	}
	path := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(path, []byte(text.String()), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMaxRowsOutputBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		count    int
		flags    []string
		wantRows int
		wantCode int
	}{
		{name: "empty", count: 0, wantRows: 0},
		{name: "one", count: 1, wantRows: 1},
		{name: "at-default", count: 500, wantRows: 500},
		{name: "above-default", count: 501, wantRows: 500},
		{name: "explicit", count: 501, flags: []string{"--max-rows", "1"}, wantRows: 1},
		{name: "disabled", count: 501, flags: []string{"--max-rows=0"}, wantRows: 501},
		{name: "quiet", count: 501, flags: []string{"--quiet", "--max-rows=1"}, wantRows: 1},
		{name: "strict", count: 501, flags: []string{"--strict", "--max-rows=1"}, wantRows: 1, wantCode: 4},
		{name: "fail-empty", count: 0, flags: []string{"--fail-empty"}, wantRows: 0, wantCode: 3},
		{name: "fail-empty-truncated", count: 501, flags: []string{"--fail-empty", "--max-rows=1"}, wantRows: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := maxRowsInput(t, tc.count)
			for _, format := range []string{"json", "csv", "tsv"} {
				t.Run(format, func(t *testing.T) {
					args := append([]string{"--format", format, "SELECT title FROM headings", path}, tc.flags...)
					code, out, errOut := captureFeedbackRun(t, args)
					wantErr := ""
					if tc.wantRows < tc.count {
						wantErr = fmt.Sprintf("mdsql: truncated, %d of %d rows\n", tc.wantRows, tc.count)
					}
					if code != tc.wantCode || errOut != wantErr {
						t.Fatalf("code=%d want=%d stderr=%q want=%q", code, tc.wantCode, errOut, wantErr)
					}
					var titles []string
					if format == "json" {
						var rows []struct{ Title string }
						if err := json.Unmarshal([]byte(out), &rows); err != nil {
							t.Fatalf("invalid JSON: %v", err)
						}
						for _, row := range rows {
							titles = append(titles, row.Title)
						}
					} else {
						reader := csv.NewReader(strings.NewReader(out))
						if format == "tsv" {
							reader.Comma = '\t'
						}
						records, err := reader.ReadAll()
						if err != nil {
							t.Fatalf("invalid %s: %v", format, err)
						}
						if len(records) > 0 {
							if len(records[0]) != 1 || records[0][0] != "title" {
								t.Fatalf("unexpected header: %v", records[0])
							}
							for _, record := range records[1:] {
								titles = append(titles, record[0])
							}
						}
					}
					if len(titles) != tc.wantRows {
						t.Fatalf("got %d rows, want %d", len(titles), tc.wantRows)
					}
					for i, title := range titles {
						if want := fmt.Sprintf("Row%03d", i+1); title != want {
							t.Fatalf("row %d: title=%q want=%q", i, title, want)
						}
					}
				})
			}
		})
	}
}

func TestMaxRowsPreservesQuerySemantics(t *testing.T) {
	path := maxRowsInput(t, 501)
	for _, tc := range []struct {
		name string
		sql  string
		args []string
		want string
		warn string
	}{
		{"limit-offset", "SELECT title FROM headings LIMIT 4 OFFSET 3", []string{"--max-rows=2"}, `[{"title":"Row004"},{"title":"Row005"}]`, "mdsql: truncated, 2 of 4 rows\n"},
		{"smaller-limit", "SELECT title FROM headings LIMIT 1", []string{"--max-rows=2"}, `[{"title":"Row001"}]`, ""},
		{"offset", "SELECT title FROM headings OFFSET 500", []string{"--max-rows=2"}, `[{"title":"Row501"}]`, ""},
		{"table-limit", "SELECT title FROM headings", []string{"--max-rows=2", "--table-limit", "3"}, `[{"title":"Row001"},{"title":"Row002"}]`, "mdsql: truncated, 2 of 3 rows\n"},
		{"count", "SELECT COUNT(*) FROM headings", []string{"--max-rows=1"}, `[{"count":501}]`, ""},
		{"grouped-count", "SELECT level, COUNT(*) FROM headings GROUP BY level ORDER BY level", []string{"--max-rows=1"}, `[{"count":251,"level":1}]`, "mdsql: truncated, 1 of 2 rows\n"},
		{"distinct", "SELECT DISTINCT level FROM headings ORDER BY level", []string{"--max-rows=1"}, `[{"level":1}]`, "mdsql: truncated, 1 of 2 rows\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--json", tc.sql, path}, tc.args...)
			code, out, errOut := captureFeedbackRun(t, args)
			if code != 0 || out != tc.want+"\n" || errOut != tc.warn {
				t.Fatalf("code=%d stdout=%q want=%q stderr=%q want=%q", code, out, tc.want, errOut, tc.warn)
			}
		})
	}
}

func TestMaxRowsPreservesNoHeader(t *testing.T) {
	path := maxRowsInput(t, 2)
	for _, format := range []string{"csv", "tsv"} {
		code, out, errOut := captureFeedbackRun(t, []string{"--format", format, "--no-header", "--max-rows=1", "SELECT title FROM headings", path})
		if code != 0 || out != "Row001\n" || errOut != "mdsql: truncated, 1 of 2 rows\n" {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", format, code, out, errOut)
		}
	}
}

func TestMaxRowsDoesNotReportTruncationAfterRenderError(t *testing.T) {
	path := maxRowsInput(t, 2)
	stdout, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	oldArgs, oldOut, oldErr := os.Args, os.Stdout, os.Stderr
	defer func() { os.Args, os.Stdout, os.Stderr = oldArgs, oldOut, oldErr }()
	os.Args = []string{"mdsql", "--json", "--max-rows=1", "SELECT title FROM headings", path}
	os.Stdout, os.Stderr = stdout, stderr
	code := run()
	errOut, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(errOut), "render error") || strings.Contains(string(errOut), "truncated") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}
