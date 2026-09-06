package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTSVAndPlainFollowSelectOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("# Title\n"), 0600); err != nil {
		t.Fatal(err)
	}

	query := "SELECT title, line, level FROM headings"
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"tsv", []string{"--format", "tsv", query, path}, "title\tline\tlevel\nTitle\t1\t1\n"},
		{"tsv no header", []string{"--format", "tsv", "--no-header", query, path}, "Title\t1\t1\n"},
		{"plain", []string{"--format", "plain", query, path}, "Title\t1\t1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := captureFeedbackRun(t, tc.args)
			if code != 0 || out != tc.want || errOut != "" {
				t.Fatalf("code=%d stdout=%q want=%q stderr=%q", code, out, tc.want, errOut)
			}
		})
	}
}
