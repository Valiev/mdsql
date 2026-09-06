package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeAliasCombinesLanguageAndSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	raw := "# Other\n```go\nwrong section\n```\n# Examples' notes\n```go\nwanted go\n```\n```python\nwanted python\n```\n```js\nwrong language\n```\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	for _, sectionFlag := range []string{"--section", "--under"} {
		for _, flags := range [][]string{
			{"--lang", "go", sectionFlag, "Examples' notes"},
			{sectionFlag, "Examples' notes", "--lang", "go"},
			{"--lang", "go", sectionFlag, "Examples' notes", "--lang", "python"},
		} {
			args := append(append([]string{"--json", "code"}, flags...), path)
			code, out, errOut := captureFeedbackRun(t, args)
			var rows []struct{ Lang, Code string }
			if err := json.Unmarshal([]byte(out), &rows); err != nil || code != 0 || errOut != "" {
				t.Fatalf("flags=%v code=%d out=%q err=%q decode=%v", flags, code, out, errOut, err)
			}
			wantCount := 1
			if len(flags) == 6 {
				wantCount = 2
			}
			if len(rows) != wantCount || rows[0].Lang != "go" || rows[0].Code != "wanted go" {
				t.Fatalf("flags=%v rows=%v", flags, rows)
			}
			if wantCount == 2 && (rows[1].Lang != "python" || rows[1].Code != "wanted python") {
				t.Fatalf("rows=%v", rows)
			}
			code, sql, errOut := captureFeedbackRun(t, append(args, "--explain"))
			if code != 0 || errOut != "" || strings.Count(sql, " WHERE ") != 1 || !strings.Contains(sql, " AND ") {
				t.Fatalf("explain=%d %q %q", code, sql, errOut)
			}
		}
	}
}
