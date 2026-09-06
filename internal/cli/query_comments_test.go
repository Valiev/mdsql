package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestQueryCommentsPreserveSQLAndArgumentOrder(t *testing.T) {
	for _, sql := range []string{
		"-- comment\nSELECT file FROM files",
		"--comment\nSELECT file FROM files",
		"--\nSELECT file FROM files",
		"-- comment\r\nSELECT file FROM files",
		"--format=json\nSELECT file FROM files",
		"--version\nSELECT file FROM files",
		"-- comment",
		"--\tcomment",
	} {
		t.Run(sql, func(t *testing.T) {
			cfg, err := ParseArgs([]string{"--json", "QuErY", "--csv", sql, "first.md", "--strict", "-", "second.md"})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Mode != ModeQuery || cfg.Query != sql || cfg.Format != "csv" || !cfg.Strict {
				t.Fatalf("unexpected query config: %#v", cfg)
			}
			if want := []string{"first.md", "-", "second.md"}; !reflect.DeepEqual(cfg.Files, want) {
				t.Fatalf("files = %v, want %v", cfg.Files, want)
			}
		})
	}
}

func TestQueryCommentsKeepUnknownFlagErrors(t *testing.T) {
	sql := "-- comment\nSELECT file FROM files"
	for _, args := range [][]string{
		{"--unknown", "query", sql},
		{"query", "--unknown", sql},
		{"query", sql, "--unknown", "note.md"},
		{"query", sql, "note.md", "--unknown"},
		{"query", "--unknown"},
		{"query", "SELECT file FROM files", sql},
		{"tasks", sql},
	} {
		_, err := ParseArgs(args)
		if err == nil || !strings.Contains(err.Error(), "unknown flag") {
			t.Fatalf("%v: expected unknown flag, got %v", args, err)
		}
	}
	_, err := ParseArgs([]string{"not-an-alias", sql})
	if err == nil || !strings.Contains(err.Error(), "unknown alias") {
		t.Fatalf("expected unknown alias, got %v", err)
	}
}

func TestQueryCommentsKeepOptionValues(t *testing.T) {
	sql := "-- comment\nSELECT file FROM files"
	cfg, err := ParseArgs([]string{"query", "--root=path with spaces", "--include", "--help", "--stdin-name", "--version", sql, "--exclude", "--draft*", "note.md"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModeQuery || cfg.Query != sql || cfg.Root != "path with spaces" || cfg.StdinName != "--version" || !reflect.DeepEqual(cfg.Include, []string{"--help"}) || !reflect.DeepEqual(cfg.Exclude, []string{"--draft*"}) {
		t.Fatalf("option values were consumed as flags or SQL: %#v", cfg)
	}
	cfg, err = ParseArgs([]string{"tasks", "--tag", sql, "--json", "note.md"})
	if err != nil || !strings.Contains(cfg.Query, sql) || !cfg.Json || !reflect.DeepEqual(cfg.Files, []string{"note.md"}) {
		t.Fatalf("alias option value was consumed as SQL: %#v %v", cfg, err)
	}
}

func TestQueryCommentsKeepHelpAndVersionPrecedence(t *testing.T) {
	sql := "-- comment\nSELECT file FROM files"
	for _, tc := range []struct {
		args []string
		mode CommandMode
	}{
		{[]string{"--help", "query", sql}, ModeHelp},
		{[]string{"query", "--help", sql}, ModeHelp},
		{[]string{"query", sql, "note.md", "--help", "--version"}, ModeHelp},
		{[]string{"query", sql, "note.md", "--version", "--help"}, ModeVersion},
	} {
		cfg, err := ParseArgs(tc.args)
		if err != nil || cfg.Mode != tc.mode {
			t.Fatalf("%v: mode=%q, want %q; error=%v", tc.args, cfg.Mode, tc.mode, err)
		}
	}
}
