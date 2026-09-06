package engine_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/cache"
	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/output"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func TestFrontmatterDatesColdAndCached(t *testing.T) {
	const raw = `---
start: 2026-09-02
quoted: "2026-09-02"
stamp: 2026-09-02T03:04:05+03:00
midnight: 2026-09-02T00:00:00Z
spaced: 2026-09-02 01:02:03
short: 2026-9-2
similar: 2026-99-02
dates: [2026-09-02]
nested:
  start: 2026-09-02
---
Body.
`
	path := filepath.Join(t.TempDir(), "dates.md")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parser.ParseFile(path, raw, fi.Size(), fi.ModTime())
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("warnings = %#v", doc.Warnings)
	}
	if doc.FrontMatter["start"] != "2026-09-02" {
		t.Fatalf("date lost its lexical form: %#v", doc.FrontMatter["start"])
	}
	if doc.FrontMatter["short"] != "2026-09-02" {
		t.Fatalf("short YAML date not normalized: %#v", doc.FrontMatter["short"])
	}
	if _, ok := doc.FrontMatter["midnight"].(time.Time); !ok {
		t.Fatalf("midnight timestamp became a date: %#v", doc.FrontMatter["midnight"])
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store, err := cache.OpenDefault()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Store(doc); err != nil {
		t.Fatal(err)
	}
	warm, ok := store.Load(path, fi)
	if !ok {
		t.Fatal("cache miss")
	}
	for name, parsed := range map[string]*parser.Document{"cold": doc, "warm": warm} {
		t.Run(name, func(t *testing.T) {
			q, err := query.Parse("SELECT fm.start AS start, fm.quoted AS quoted, fm.stamp AS stamp, fm.midnight AS midnight, fm.spaced AS spaced, fm.similar AS similar FROM frontmatter WHERE fm.start BETWEEN 2026-09-01 AND today AND fm.quoted < today AND fm.nested.start = 2026-09-02 AND contains(fm.dates, 2026-09-02)")
			if err != nil {
				t.Fatal(err)
			}
			result, err := engine.Execute(q, []*parser.Document{parsed}, engine.Options{Today: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), Strict: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Rows) != 1 || len(result.Warnings) != 0 {
				t.Fatalf("result = %#v", result)
			}
			row := result.Rows[0]
			for column, kind := range map[string]query.ValueKind{"start": query.ValueDate, "quoted": query.ValueDate, "stamp": query.ValueDateTime, "midnight": query.ValueDateTime, "spaced": query.ValueDateTime, "similar": query.ValueString} {
				if row[column].Kind != kind {
					t.Errorf("%s type = %v, want %v", column, row[column].Kind, kind)
				}
			}
			for _, format := range []output.Format{output.FormatJSON, output.FormatText, output.FormatCSV} {
				var buf bytes.Buffer
				if err := output.Render(&buf, result.Rows, format, 0, false, false, "frontmatter", result.Columns); err != nil {
					t.Fatal(err)
				}
				for _, expected := range []string{"2026-09-02", "2026-09-02T03:04:05+03:00", "2026-09-02T00:00:00Z", "2026-09-02T01:02:03Z", "2026-99-02"} {
					if !strings.Contains(buf.String(), expected) {
						t.Errorf("%s output lacks %q: %s", format, expected, buf.String())
					}
				}
				if strings.Contains(buf.String(), " UTC") || strings.Contains(buf.String(), " +0300") {
					t.Errorf("%s output contains Go timestamp: %s", format, buf.String())
				}
			}
		})
	}
}
