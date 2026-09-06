package cli

import (
	"strings"
	"testing"
)

func TestCacheFlags(t *testing.T) {
	t.Setenv("MDSQL_CACHE", "")
	for _, tc := range []struct {
		flags []string
		want  bool
	}{
		{nil, false},
		{[]string{"--cache"}, true},
		{[]string{"--cache", "--no-cache"}, false},
		{[]string{"--no-cache", "--cache"}, true},
	} {
		args := append(append([]string{}, tc.flags...), "query", "-- comment\nSELECT file FROM files", "note.md")
		cfg, err := ParseArgs(args)
		if err != nil || cfg.Cache != tc.want || cfg.Mode != ModeQuery || len(cfg.Files) != 1 {
			t.Fatalf("args=%v config=%+v error=%v", args, cfg, err)
		}
	}
	cfg, err := ParseArgs([]string{"--clear-cache"})
	if err != nil || cfg.Mode != ModeClearCache {
		t.Fatalf("clear config=%+v error=%v", cfg, err)
	}
	for _, args := range [][]string{{"--clear-cache", "headings"}, {"--clear-cache", "note.md"}} {
		if _, err := ParseArgs(args); err == nil || !strings.Contains(err.Error(), "--clear-cache") {
			t.Fatalf("expected clear-cache argument error for %v, got %v", args, err)
		}
	}
	if _, err := ParseArgs([]string{"--clear-cache", "--unknown"}); err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected unknown flag with clear-cache, got %v", err)
	}
}

func TestCacheEnvironment(t *testing.T) {
	for _, value := range []string{"", "0", "1", "true"} {
		t.Setenv("MDSQL_CACHE", value)
		for _, flags := range [][]string{nil, {"--cache"}, {"--no-cache"}} {
			cfg, err := ParseArgs(append(append([]string{}, flags...), "SELECT file FROM files", "note.md"))
			want := value == "1"
			if len(flags) > 0 {
				want = flags[0] == "--cache"
			}
			if err != nil || cfg.Cache != want {
				t.Fatalf("env=%q flags=%v cache=%v err=%v", value, flags, cfg.Cache, err)
			}
		}
	}
}
