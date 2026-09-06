package cli

import (
	"strings"
	"testing"
)

func TestVersionFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--version"}, {"-V"}, {"version"}, {"--json", "-V"},
		{"SELECT file FROM files", "--version"}, {"--version", "--help"},
	} {
		cfg, err := ParseArgs(args)
		if err != nil || cfg.Mode != ModeVersion {
			t.Fatalf("%v: mode=%q error=%v", args, cfg.Mode, err)
		}
	}
	cfg, err := ParseArgs([]string{"--help", "--version"})
	if err != nil || cfg.Mode != ModeHelp {
		t.Fatalf("first terminal flag should win: mode=%q error=%v", cfg.Mode, err)
	}
}

func TestUnknownFlagsAreNotAliases(t *testing.T) {
	for _, args := range [][]string{
		{"--verison"}, {"-nonsense"}, {"tasks", "--opne"}, {"tables", "--wat=1"},
		{"SELECT file FROM files", "--unknown"}, {"schema", "--unknown"},
		{"query", "SELECT file FROM files", "--unknown"},
	} {
		_, err := ParseArgs(args)
		if err == nil || !strings.Contains(err.Error(), "unknown flag") {
			t.Fatalf("%v: expected unknown flag, got %v", args, err)
		}
	}
	_, err := ParseArgs([]string{"not-an-alias"})
	if err == nil || !strings.Contains(err.Error(), "unknown alias") {
		t.Fatalf("expected unknown alias, got %v", err)
	}
}

func TestAliasValuesAndStdinSurviveFlagParsing(t *testing.T) {
	cfg, err := ParseArgs([]string{"tasks", "--tag", "--json", "--csv", "note.md"})
	if err != nil || cfg.Json || !strings.Contains(cfg.Query, "--json") {
		t.Fatalf("alias value was consumed as a global flag: %#v %v", cfg, err)
	}
	cfg, err = ParseArgs([]string{"headings", "--level", "2", "--json", "-"})
	if err != nil || !cfg.Json || len(cfg.Files) != 1 || cfg.Files[0] != "-" {
		t.Fatalf("known alias options or stdin failed: %#v %v", cfg, err)
	}
}
