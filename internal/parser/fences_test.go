package parser

import (
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/model"
)

func TestCodeFences(t *testing.T) {
	for _, tc := range []struct {
		name  string
		open  string
		close string
		inner string
		fence string
		info  string
		lang  string
	}{
		{"triple", "```Go", "```", "code", "```", "Go", "go"},
		{"nested shorter", "````markdown", "````", "```go\n## Fake nested\n```", "````", "markdown", "markdown"},
		{"tilde", "~~~text", "~~~", "```\n~~~not-a-close\n```", "~~~", "text", "text"},
		{"longer closing", "````", "`````", "```", "````", "", ""},
		{"different character", "```", "```", "~~~\ncode\n~~~", "```", "", ""},
		{"nonempty closing tail", "```", "```", "```text\ncode", "```", "", ""},
		{"info fields", "```Go linenos", "```", "code", "```", "Go linenos", "go"},
		{"tilde backtick info", "~~~lang `info`", "~~~", "code", "~~~", "lang `info`", "lang"},
		{"indent and closing whitespace", "   ```go", "  ``` \t", "code", "```", "go", "go"},
		{"setext inside code", "```", "```", "---\nFake\n---", "```", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inside := tc.inner + "\n## Fake heading\n- [ ] Fake task #inside [[Inside]] https://inside.test"
			code := tc.open + "\n" + inside + "\n" + tc.close
			raw := "# Before\n" + code + "\n# After\n- [ ] Real task #outside [[Outside]]"
			doc, err := ParseFile("fence.md", raw, int64(len(raw)), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			var blocks []model.Block
			for _, block := range doc.Blocks {
				if block.Kind == model.KindCode {
					blocks = append(blocks, block)
				}
			}
			if len(blocks) != 1 {
				t.Fatalf("code blocks = %#v", blocks)
			}
			block := blocks[0]
			if block.Start != 2 || block.End != strings.Count(code, "\n")+2 || block.Raw != code || block.Text != inside || block.Fence != tc.fence || block.Info != tc.info || block.Lang != tc.lang {
				t.Fatalf("code block = %#v", block)
			}
			if len(doc.Headings) != 2 || doc.Headings[0].Title != "Before" || doc.Headings[1].Title != "After" {
				t.Fatalf("headings = %#v", doc.Headings)
			}
			if len(doc.Tasks) != 1 || !strings.HasPrefix(doc.Tasks[0].Text, "Real task") {
				t.Fatalf("tasks = %#v", doc.Tasks)
			}
			if len(doc.Links) != 1 || doc.Links[0].Target != "Outside" {
				t.Fatalf("links = %#v", doc.Links)
			}
			if len(doc.Tags) != 1 || doc.Tags[0].Tag != "outside" {
				t.Fatalf("tags = %#v", doc.Tags)
			}
			for _, line := range doc.LinesRows {
				if line.N >= block.Start && line.N <= block.End && line.Kind != model.KindCode {
					t.Fatalf("code line kind = %#v", line)
				}
			}
		})
	}
}

func TestUnterminatedCodeFence(t *testing.T) {
	raw := "````go\n```\n## Hidden\n- [ ] Hidden [[Hidden]] #hidden"
	doc, err := ParseFile("fence.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Kind != model.KindCode || doc.Blocks[0].End != 4 || doc.Blocks[0].Raw != raw {
		t.Fatalf("blocks = %#v", doc.Blocks)
	}
	if len(doc.Headings)+len(doc.Tasks)+len(doc.Links)+len(doc.Tags) != 0 {
		t.Fatalf("code leaked into body: %#v", doc)
	}
}

func TestInvalidCodeFenceOpeners(t *testing.T) {
	for _, line := range []string{"``go", "~~text", "```go `invalid`", "    ```go", "\t```go"} {
		t.Run(line, func(t *testing.T) {
			if fence, _ := parseCodeFence(line); fence != "" {
				t.Fatalf("invalid opener recognized: %q", fence)
			}
			raw := line + "\n# Visible"
			doc, err := ParseFile("fence.md", raw, int64(len(raw)), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if len(doc.Headings) != 1 || doc.Headings[0].Title != "Visible" {
				t.Fatalf("invalid opener hid body: %#v", doc.Headings)
			}
		})
	}
}
