package cli

import "testing"

func TestParseArgsCSV(t *testing.T) {
	config, err := ParseArgs([]string{"--csv", "SELECT line, raw FROM headings", "note.md"})
	if err != nil {
		t.Fatal(err)
	}
	if config.Format != "csv" || config.Json {
		t.Fatalf("unexpected format config: format=%q json=%v", config.Format, config.Json)
	}
}

func TestParseArgsTablesAlias(t *testing.T) {
	for _, files := range [][]string{nil, {"note.md"}, {"first.md", "second.md"}} {
		args := append([]string{"tables", "--csv", "--explain"}, files...)
		config, err := ParseArgs(args)
		if err != nil {
			t.Fatal(err)
		}
		if config.Mode != ModeQuery || config.Query != "SELECT file, start, end, raw FROM tables" {
			t.Fatalf("unexpected tables alias: mode=%q query=%q", config.Mode, config.Query)
		}
		if config.Format != "csv" || !config.Explain {
			t.Fatalf("global flags lost: format=%q explain=%v", config.Format, config.Explain)
		}
		if len(config.Files) != len(files) {
			t.Fatalf("got files %v, want %v", config.Files, files)
		}
		for i, file := range files {
			if config.Files[i] != file {
				t.Fatalf("got files %v, want %v", config.Files, files)
			}
		}
	}
}
