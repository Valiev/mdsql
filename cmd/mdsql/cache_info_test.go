package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Valiev/mdsql/internal/cache"
)

func TestCacheInfoAndEnvironment(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
	t.Setenv("MDSQL_CACHE", "1")
	code, out, errOut := captureFeedbackRun(t, []string{"--cache-info", "--json"})
	if code != 0 || out != "{\"entries\":0,\"bytes\":0}\n" || errOut != "" {
		t.Fatalf("empty: %d %q %q", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(base, "cache")); !os.IsNotExist(err) {
		t.Fatalf("cache-info created cache: %v", err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--cache-info", "--csv"}, "entries,bytes\n0,0\n"},
		{[]string{"--cache-info", "--tsv", "--no-header"}, "0\t0\n"},
		{[]string{"--cache-info", "--format", "plain"}, "0\t0\n"},
		{[]string{"--cache-info", "--format", "jsonl"}, "{\"entries\":0,\"bytes\":0}\n"},
	} {
		code, out, errOut = captureFeedbackRun(t, tc.args)
		if code != 0 || out != tc.want || errOut != "" {
			t.Fatalf("format args=%v: %d %q %q", tc.args, code, out, errOut)
		}
	}
	code, out, errOut = captureFeedbackRun(t, []string{"--cache-info", "--format", "grouped"})
	if code != 1 || out != "" || errOut == "" {
		t.Fatalf("incompatible format: %d %q %q", code, out, errOut)
	}
	path := filepath.Join(base, "note.md")
	if err := os.WriteFile(path, []byte("# Test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = captureFeedbackRun(t, []string{"SELECT title FROM headings", path})
	if code != 0 || errOut != "" {
		t.Fatalf("env cache: %d %q", code, errOut)
	}
	code, out, errOut = captureFeedbackRun(t, []string{"--cache-info", "--json"})
	var stats cache.Stats
	if err := json.Unmarshal([]byte(out), &stats); err != nil || code != 0 || errOut != "" || stats.Entries != 1 || stats.Bytes <= 0 {
		t.Fatalf("stats: %d %q %q %v", code, out, errOut, err)
	}
	for _, args := range [][]string{{"--cache-info", "--clear-cache"}, {"--cache-info", "SELECT count(*) FROM files"}} {
		code, _, _ := captureFeedbackRun(t, args)
		if code != 1 {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
}
