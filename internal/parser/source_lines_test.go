package parser_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/cache"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
	"github.com/Valiev/mdsql/internal/tables"
)

func TestPhysicalLinesAndEmptyBodyRanges(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store, err := cache.OpenDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, raw string
		lines     int
		root      bool
	}{
		{"empty", "", 0, false},
		{"newline", "\n", 1, true},
		{"terminated", "foo\n", 1, true},
		{"unterminated", "foo", 1, true},
		{"blank last line", "foo\n\n", 2, true},
		{"delimiter", "---\n", 1, true},
		{"frontmatter only", "---\nowner: bob\n---", 3, false},
		{"frontmatter with newline", "---\nowner: bob\n---\n", 3, false},
		{"frontmatter and blank body", "---\nowner: bob\n---\n\n", 4, true},
		{"bom crlf", "\uFEFF# Heading\r\n", 1, true},
		{"open code", "```\nbody\n", 2, true},
		{"empty code", "```\n```\n", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.md")
			if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
				t.Fatal(err)
			}
			fi, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := parser.ParseFile(path, tc.raw, fi.Size(), fi.ModTime())
			if err != nil {
				t.Fatal(err)
			}
			if len(doc.Lines) != tc.lines || len(doc.LinesRows) != tc.lines {
				t.Fatalf("lines = %#v, rows = %#v; want %d physical lines", doc.Lines, doc.LinesRows, tc.lines)
			}
			if (len(doc.Sections) > 0) != tc.root {
				t.Fatalf("root presence = %t, want %t", len(doc.Sections) > 0, tc.root)
			}
			normalized := strings.ReplaceAll(strings.TrimPrefix(tc.raw, "\uFEFF"), "\r\n", "\n")
			if doc.Content != normalized || doc.Size != fi.Size() {
				t.Fatalf("source changed: content=%q size=%d", doc.Content, doc.Size)
			}
			for _, table := range tables.Registry {
				for _, row := range table.Rows(doc) {
					start, hasStart := row["start"]
					end, hasEnd := row["end"]
					if hasStart && hasEnd && start.Kind == query.ValueInt && end.Kind == query.ValueInt {
						if start.Data.(int64) > end.Data.(int64) || start.Data.(int64) < 1 || end.Data.(int64) > int64(tc.lines) {
							t.Errorf("%s invalid range: %#v", table.Name, row)
						}
					}
					if table.Name == "files" && row["lines"].Data.(int64) != int64(tc.lines) {
						t.Errorf("files.lines = %s, want %d", row["lines"].String(), tc.lines)
					}
				}
			}
			if err := store.Store(doc); err != nil {
				t.Fatal(err)
			}
			warm, ok := store.Load(path, fi)
			if !ok || !reflect.DeepEqual(doc, warm) {
				t.Fatalf("cache changed physical line/range data: hit=%t cold=%#v warm=%#v", ok, doc, warm)
			}
		})
	}
}

func TestSourceFragmentsKeepFinalNewline(t *testing.T) {
	for _, raw := range []string{"# Heading\nParagraph.\n", "```\ncode\n", "```\n"} {
		doc, err := parser.ParseFile("source.md", raw, int64(len(raw)), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if doc.Sections[0].Raw != raw {
			t.Errorf("root raw = %q, want %q", doc.Sections[0].Raw, raw)
		}
		if strings.HasPrefix(raw, "```") && doc.Blocks[0].Raw != raw {
			t.Errorf("unterminated code raw = %q, want %q", doc.Blocks[0].Raw, raw)
		}
	}
}
