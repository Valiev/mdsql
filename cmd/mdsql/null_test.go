package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLINullFormatting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("---\nvalue: '-'\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		flags []string
		want  string
	}{
		{[]string{"--csv"}, "missing,value\n,-\n"},
		{[]string{"--csv", "--null=NULL"}, "missing,value\nNULL,-\n"},
		{[]string{"--tsv", "--null", "NONE"}, "missing\tvalue\nNONE\t-\n"},
		{[]string{"--json"}, "[{\"missing\":null,\"value\":\"-\"}]\n"},
	} {
		code, out, errOut := captureFeedbackRun(t, append(append([]string{}, tc.flags...), "SELECT fm.missing AS missing, fm.value AS value FROM frontmatter", path))
		if code != 0 || out != tc.want || errOut != "" {
			t.Fatalf("%v: %d %q %q", tc.flags, code, out, errOut)
		}
	}
}
