package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTasksHasIDRequiresAnActualID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.md")
	if err := os.WriteFile(path, []byte("- [ ] identified 🆔 abc-123\n- [ ] ordinary\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := captureFeedbackRun(t, []string{"--json", "tasks", "--has-id", path})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["text"] != "identified" {
		t.Fatalf("rows = %#v", rows)
	}
}
