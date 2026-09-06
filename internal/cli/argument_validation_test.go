package cli

import (
	"strings"
	"testing"
)

func TestHelpListsGlobalFlags(t *testing.T) {
	help := Help()
	for _, flag := range []string{"--quiet", "--fail-empty", "--max-lines", "--table-limit", "--stdin-name", "--no-file", "--max-file-size"} {
		if !strings.Contains(help, flag) {
			t.Errorf("help does not list %s", flag)
		}
	}
}

func TestTablesFlagPointsToSchemaCommand(t *testing.T) {
	for _, args := range [][]string{{"--tables"}, {"--tables", "SELECT file FROM files"}} {
		_, err := ParseArgs(args)
		if err == nil || !strings.Contains(err.Error(), "mdsql schema --tables") {
			t.Fatalf("ParseArgs(%v) error = %v", args, err)
		}
	}
	cfg, err := ParseArgs([]string{"schema", "--tables"})
	if err != nil || cfg.Mode != ModeSchema || !cfg.SchemaTablesOnly {
		t.Fatalf("schema --tables = %#v, %v", cfg, err)
	}
}

func TestAliasArgumentsAreValidated(t *testing.T) {
	cfg, err := ParseArgs([]string{"tasks", "--has-id", "note.md"})
	if err != nil || !strings.Contains(cfg.Query, "id != ''") {
		t.Fatalf("tasks --has-id query = %q, %v", cfg.Query, err)
	}
	for _, args := range [][]string{{"at", "note.md:0"}, {"at", "note.md:nope"}, {"-n", "-1", "headings", "note.md"}, {"-n-1", "headings", "note.md"}} {
		if _, err := ParseArgs(args); err == nil {
			t.Fatalf("ParseArgs(%v) accepted", args)
		}
	}
}
