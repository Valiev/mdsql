package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIUnderLikeAndUnderContains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(path, []byte("# Project\nText\n# Other\nOther text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ predicate, want string }{
		{"under_like('%proj%')", `[{"title":"Project"}]`},
		{"under_like('Proj_ct')", `[{"title":"Project"}]`},
		{"under_like('proj')", `[]`},
		{"under_contains('proj')", `[{"title":"Project"}]`},
		{"under_contains('%proj%')", `[]`},
	} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", "--strict", "SELECT title FROM sections WHERE " + tc.predicate, path})
		if code != 0 || out != tc.want+"\n" || errOut != "" {
			t.Fatalf("predicate=%q code=%d stdout=%q stderr=%q", tc.predicate, code, out, errOut)
		}
	}
}
