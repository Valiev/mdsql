package cli

import (
	"strings"
	"testing"
)

func TestGroupedAndNoHeaderFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--format", "grouped", "SELECT file, text FROM tasks"},
		{"SELECT file, text FROM tasks", "-fgrouped"},
		{"--format=grouped", "SELECT file FROM files", "--no-file", "--with-file"},
		{"--format=grouped", "--no-file", "SELECT file FROM files"},
	} {
		cfg, err := ParseArgs(args)
		if err != nil || cfg.Format != "grouped" {
			t.Fatalf("args=%q config=%+v error=%v", args, cfg, err)
		}
	}
	for _, args := range [][]string{
		{"--no-header", "--csv", "SELECT text FROM tasks"},
		{"SELECT text FROM tasks", "--format=tsv", "--no-header"},
		{"--format", "json", "--no-header", "SELECT text FROM tasks", "--csv"},
	} {
		cfg, err := ParseArgs(args)
		if err != nil || !cfg.NoHeader || cfg.Format != "csv" && cfg.Format != "tsv" {
			t.Fatalf("args=%q config=%+v error=%v", args, cfg, err)
		}
	}
}

func TestRejectIncompatibleGroupedAndNoHeaderFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--no-header", "SELECT text FROM tasks"},
		{"--csv", "--no-header", "SELECT text FROM tasks", "--json"},
		{"--format", "grouped", "--no-header", "SELECT file FROM files"},
	} {
		if _, err := ParseArgs(args); err == nil {
			t.Fatalf("accepted incompatible flags: %q", args)
		}
	}
	if help := Help(); !strings.Contains(help, "grouped") || !strings.Contains(help, "--no-header") {
		t.Fatalf("missing output options in help: %s", help)
	}
}
