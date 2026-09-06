package main

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/tables"
)

func TestCLIStarSchemaOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("---\nlabel: demo\n---\n# Title\n"), 0600); err != nil {
		t.Fatal(err)
	}
	table, _ := tables.Table("headings")
	var want []string
	for _, col := range table.Columns {
		if col.Name != "fm" {
			want = append(want, col.Name)
		}
	}
	for _, format := range []string{"csv", "tsv", "json", "jsonl", "text"} {
		code, out, errOut := captureFeedbackRun(t, []string{"--format", format, "SELECT * FROM headings", path})
		if code != 0 || errOut != "" {
			t.Fatalf("%s: %d %q", format, code, errOut)
		}
		var got []string
		switch format {
		case "csv", "tsv":
			r := csv.NewReader(strings.NewReader(out))
			if format == "tsv" {
				r.Comma = '\t'
			}
			var err error
			got, err = r.Read()
			if err != nil {
				t.Fatal(err)
			}
		case "json", "jsonl":
			d := json.NewDecoder(strings.NewReader(out))
			if format == "json" {
				if _, err := d.Token(); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := d.Token(); err != nil {
				t.Fatal(err)
			}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, key.(string))
				var value any
				if err := d.Decode(&value); err != nil {
					t.Fatal(err)
				}
			}
		case "text":
			if !strings.HasPrefix(out, "file=") || strings.Contains(out, "fm=") {
				t.Fatalf("text: %q", out)
			}
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s keys=%v want=%v", format, got, want)
		}
	}
	code, out, errOut := captureFeedbackRun(t, []string{"--json", "SELECT *, fm FROM headings", path})
	if code != 0 || !strings.Contains(out, `"fm":{"label":"demo"}`) || errOut != "" {
		t.Fatalf("explicit fm: %d %q %q", code, out, errOut)
	}
}
