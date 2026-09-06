package parser_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/tables"
)

func TestSectionHierarchy(t *testing.T) {
	raw := "Preamble.\n# A\nOwn A.\n## B\nOwn B.\n### C\n- [ ] Task [[Link]] #tag\n## D\nOwn D.\n# E\nOwn E."
	doc, err := parser.ParseFile("sections.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		title, parent, path, own    string
		depth, children, start, end int
	}{
		{"", "", "", "Preamble.", 0, 2, 1, 11},
		{"A", "", "A", "Own A.", 1, 2, 2, 9},
		{"B", "A", "A > B", "Own B.", 2, 1, 4, 7},
		{"C", "B", "A > B > C", "- [ ] Task [[Link]] #tag", 3, 0, 6, 7},
		{"D", "A", "A > D", "Own D.", 2, 0, 8, 9},
		{"E", "", "E", "Own E.", 1, 0, 10, 11},
	}
	if len(doc.Sections) != len(want) {
		t.Fatalf("sections = %#v", doc.Sections)
	}
	for i, expected := range want {
		section := doc.Sections[i]
		if section.Title != expected.title || section.Parent != expected.parent || section.Path != expected.path || section.Own != expected.own || section.Depth != expected.depth || section.Children != expected.children || section.Start != expected.start || section.End != expected.end {
			t.Errorf("section %d = %#v, want %#v", i, section, expected)
		}
		wantRaw := strings.Join(doc.Lines[expected.start-1:expected.end], "\n")
		bodyStart := expected.start - 1
		if i > 0 {
			bodyStart++
			heading := doc.Headings[i-1]
			if heading.Depth != expected.depth || heading.Parent != expected.parent || heading.Path != expected.path || heading.SectionEnd != expected.end || heading.Section != expected.title {
				t.Errorf("heading %d = %#v", i-1, heading)
			}
		}
		wantBody := strings.TrimSpace(strings.Join(doc.Lines[bodyStart:expected.end], "\n"))
		if section.Raw != wantRaw || section.Body != wantBody || section.Lines != expected.end-expected.start+1 {
			t.Errorf("section text %d: raw=%q body=%q lines=%d", i, section.Raw, section.Body, section.Lines)
		}
	}
	if doc.Tasks[0].Section != "C" || doc.Tasks[0].SectionPath != "A > B > C" || doc.Links[0].Section != "C" || doc.Tags[0].Section != "C" || doc.LinesRows[6].Section != "C" {
		t.Fatal("descendant content assigned to ancestor section")
	}
}

func TestSkippedHeadingLevelsAndSetext(t *testing.T) {
	raw := "A\n=\nOwn A.\n### B\nB body.\n## C\nC body."
	doc, err := parser.ParseFile("sections.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Headings[1].Depth != 2 || doc.Headings[1].Parent != "A" || doc.Headings[2].Depth != 2 || doc.Headings[2].Parent != "A" || doc.Sections[1].Children != 2 {
		t.Fatalf("headings = %#v, sections = %#v", doc.Headings, doc.Sections)
	}
	if doc.Sections[1].Own != "Own A." || !strings.HasPrefix(doc.Sections[1].Body, "Own A.\n### B") || !strings.HasPrefix(doc.Sections[1].Raw, "A\n=\n") || doc.Sections[0].Own != "" {
		t.Fatalf("setext section = %#v", doc.Sections[1])
	}
}

func TestSectionRootWithoutHeadings(t *testing.T) {
	for _, tc := range []struct {
		raw, body string
		start     int
	}{
		{"  Preamble.\nSecond line.\n", "  Preamble.\nSecond line.\n", 1},
		{"---\ntitle: Note\n---\nPreamble.", "Preamble.", 4},
		{"---\ntitle: Note\n---", "", 4},
		{"", "", 1},
	} {
		doc, err := parser.ParseFile("sections.md", tc.raw, int64(len(tc.raw)), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if tc.body == "" {
			if len(doc.Sections) != 0 {
				t.Fatalf("empty body has sections: %#v", doc.Sections)
			}
			continue
		}
		if len(doc.Sections) != 1 {
			t.Fatalf("sections = %#v", doc.Sections)
		}
		root := doc.Sections[0]
		if root.Level != 0 || root.Depth != 0 || root.Children != 0 || root.Raw != tc.body || root.Body != strings.TrimSpace(tc.body) || root.Own != root.Body || root.Start != tc.start {
			t.Fatalf("root = %#v", root)
		}
	}
}

func TestSectionSchemaDocumentsHierarchy(t *testing.T) {
	for tableName, columns := range map[string][]string{
		"sections": {"depth", "children", "body", "own", "raw"},
		"headings": {"depth", "parent", "section_end"},
	} {
		table, _ := tables.Table(tableName)
		for _, name := range columns {
			found := false
			for _, column := range table.Columns {
				if column.Name == name && column.Doc != "" {
					found = true
				}
			}
			if !found {
				t.Errorf("%s.%s missing schema documentation", tableName, name)
			}
		}
	}
}
