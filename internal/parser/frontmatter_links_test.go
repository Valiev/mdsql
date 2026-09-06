package parser_test

import (
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/tables"
)

func TestFrontmatterLinks(t *testing.T) {
	raw := `---
categories:
  - "[[Agent Tasks]]"
  - "[[Projects]]" # [[Comment]]
project: "[[Project#Heading|Alias]]"
nested:
  refs: ["![[Image]]", "[[Note#^block]]"]
"[[Key is not a link]]": ordinary value
other: 42
# [[Another comment]]
---
[[Body]] [external](https://example.test)
`
	doc, err := parser.ParseFile("card.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		target string
		key    string
		line   int
	}{
		{"Agent Tasks", "categories", 3},
		{"Projects", "categories", 4},
		{"Project", "project", 5},
		{"Image", "nested.refs", 7},
		{"Note", "nested.refs", 7},
	}
	if len(doc.Links) != len(want)+2 {
		t.Fatalf("links = %#v", doc.Links)
	}
	for i, expected := range want {
		link := doc.Links[i]
		if link.Target != expected.target || link.Key != expected.key || link.Line != expected.line || link.Source != "frontmatter" || link.External {
			t.Errorf("link %d = %#v, want %#v", i, link, expected)
		}
	}
	if doc.Links[2].Anchor != "Heading" || doc.Links[2].Text != "Alias" || doc.Links[3].Type != "embed" || doc.Links[4].BlockRef != "block" {
		t.Fatalf("wikilink components = %#v", doc.Links)
	}
	for _, link := range doc.Links[len(want):] {
		if link.Source != "body" || link.Key != "" || link.Line != 12 {
			t.Errorf("body source = %#v", link)
		}
	}
	table, ok := tables.Table("links")
	if !ok {
		t.Fatal("missing links table")
	}
	rows := table.Rows(doc)
	if rows[0]["source"].String() != "frontmatter" || rows[0]["key"].String() != "categories" || rows[len(want)]["source"].String() != "body" {
		t.Fatalf("link source columns = %#v", rows)
	}
	for _, name := range []string{"source", "key"} {
		found := false
		for _, column := range table.Columns {
			if column.Name == name && column.Type == "string" {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %s column in links schema", name)
		}
	}
}

func TestMalformedFrontmatterDoesNotProduceLinks(t *testing.T) {
	raw := "---\ncategories: [\"[[Hidden]]\"\n---\n[[Body]]"
	doc, err := parser.ParseFile("card.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Warnings) != 1 || len(doc.Links) != 1 || doc.Links[0].Target != "Body" || doc.Links[0].Source != "body" {
		t.Fatalf("warnings = %#v, links = %#v", doc.Warnings, doc.Links)
	}
}
