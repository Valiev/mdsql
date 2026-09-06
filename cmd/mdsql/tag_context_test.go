package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIHasTagLoadsFrontmatter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.md")
	source := "---\ntags: [Project/Root]\n---\n# Heading #inline/child\n"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ sql, want string }{
		{"SELECT has_tag('project') AS tagged FROM files", `[{"tagged":true}]`},
		{"SELECT count(*) FROM files WHERE has_tag('project')", `[{"count":1}]`},
		{"SELECT has_tag('project') AS tagged FROM frontmatter", `[{"tagged":true}]`},
		{"SELECT count(*) FROM headings WHERE has_tag('INLINE')", `[{"count":1}]`},
		{"SELECT count(*) FROM files WHERE has_tag('inline')", `[{"count":0}]`},
	} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", "--strict", tc.sql, path})
		if code != 0 || out != tc.want+"\n" || errOut != "" {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", tc.sql, code, out, errOut)
		}
	}
}
