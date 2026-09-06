package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLISectionTextLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.md")
	source := "# Лог\n\nfirst\nsecond\n\n\n# Last\n\nlast1\nlast2\n\n\n"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ sql, want string }{
		{"SELECT tail(raw, 2) AS tail, lines(raw) AS lines FROM sections WHERE title = 'Лог'", `[{"lines":4,"tail":"first\nsecond"}]`},
		{"SELECT tail(raw, 2) AS tail FROM sections WHERE title = 'Last'", `[{"tail":"last1\nlast2"}]`},
		{"SELECT tail(raw, 2, true) AS tail FROM sections WHERE title = 'Last'", `[{"tail":"\n"}]`},
	} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", "--strict", tc.sql, path})
		if code != 0 || out != tc.want+"\n" || errOut != "" {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", tc.sql, code, out, errOut)
		}
	}
}
