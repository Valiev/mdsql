package parser

import (
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/model"
)

func TestMalformedFrontmatter(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		line string
	}{
		{"unclosed bracket", "title: [unclosed", "2"},
		{"invalid mapping", ": :", "2"},
		{"duplicate key", "title: first\ntitle: second", "3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := "---\n" + tc.yaml + "\n---\n# Body\nParagraph.\n"
			doc, err := ParseFile("broken.md", raw, int64(len(raw)), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if len(doc.Warnings) != 1 || !strings.Contains(doc.Warnings[0], "broken.md:"+tc.line+": invalid frontmatter:") {
				t.Fatalf("warnings = %#v", doc.Warnings)
			}
			if doc.FrontMatter != nil || doc.FrontMatterRaw != tc.yaml || doc.FrontMatterFrom != 1 || doc.FrontMatterTo != strings.Count(tc.yaml, "\n")+3 {
				t.Fatalf("frontmatter = %#v, raw = %q, range = %d:%d", doc.FrontMatter, doc.FrontMatterRaw, doc.FrontMatterFrom, doc.FrontMatterTo)
			}
			if len(doc.Headings) != 1 || doc.Headings[0].Title != "Body" || doc.Headings[0].Start != doc.FrontMatterTo+1 {
				t.Fatalf("headings = %#v", doc.Headings)
			}
			if len(doc.Blocks) != 2 || doc.Blocks[1].Kind != model.KindParagraph || doc.Blocks[1].Text != "Paragraph." {
				t.Fatalf("blocks = %#v", doc.Blocks)
			}
			assertFrontmatterLines(t, doc)
		})
	}
}

func TestValidFrontmatter(t *testing.T) {
	raw := "---\ntitle: Valid\ntags: [one, two]\n---\n# Body\n"
	doc, err := ParseFile("valid.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Warnings) != 0 || doc.FrontMatter["title"] != "Valid" || doc.FrontMatterTo != 4 {
		t.Fatalf("frontmatter = %#v, warnings = %#v", doc.FrontMatter, doc.Warnings)
	}
	if len(doc.Headings) != 1 || doc.Headings[0].Title != "Body" || len(doc.Tags) != 2 {
		t.Fatalf("headings = %#v, tags = %#v", doc.Headings, doc.Tags)
	}
	assertFrontmatterLines(t, doc)
}

func TestWithoutClosedFrontmatter(t *testing.T) {
	for _, raw := range []string{"# Body\nParagraph.\n", "---\n# Body\nParagraph.\n"} {
		doc, err := ParseFile("body.md", raw, int64(len(raw)), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if len(doc.Warnings) != 0 || doc.FrontMatterFrom != 0 || doc.FrontMatterTo != 0 || doc.FrontMatter != nil {
			t.Fatalf("unexpected frontmatter: %#v", doc)
		}
		if len(doc.Headings) != 1 || doc.Headings[0].Title != "Body" {
			t.Fatalf("headings = %#v", doc.Headings)
		}
		for _, line := range doc.LinesRows {
			if line.Kind == model.KindFrontmatter {
				t.Fatalf("body line marked frontmatter: %#v", line)
			}
		}
	}
}

func assertFrontmatterLines(t *testing.T, doc *model.Document) {
	t.Helper()
	if len(doc.LinesRows) != len(doc.Lines) {
		t.Fatalf("source lines lost: got %d, want %d", len(doc.LinesRows), len(doc.Lines))
	}
	for i, line := range doc.LinesRows {
		if line.N != i+1 || line.Text != doc.Lines[i] {
			t.Fatalf("source line changed: %#v", line)
		}
		if line.N <= doc.FrontMatterTo && line.Kind != model.KindFrontmatter {
			t.Fatalf("frontmatter line is body content: %#v", line)
		}
		if line.N > doc.FrontMatterTo && line.Kind == model.KindFrontmatter {
			t.Fatalf("body line marked frontmatter: %#v", line)
		}
	}
}
