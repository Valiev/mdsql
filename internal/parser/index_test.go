package parser

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/model"
)

func TestIndexedSectionsMatchSourceRanges(t *testing.T) {
	raw := "---\ntags: [metadata]\nproject: '[[Project]]'\n---\nPreamble.\n"
	for i := 0; i < 30; i++ {
		raw += fmt.Sprintf("%s Heading %d\n\nParagraph [[Note]] #tag\n- [ ] Open\n- [x] Done\n> Quote\n\n```go\ncode\n```\n", strings.Repeat("#", 1+i%3), i)
	}
	doc, err := ParseFile("index.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range doc.Sections {
		code, tasks, open := 0, 0, 0
		for _, block := range doc.Blocks {
			if block.Kind == model.KindCode && block.Start >= section.Start && block.End <= section.End {
				code++
			}
		}
		for _, task := range doc.Tasks {
			if task.Line >= section.Start && task.Line <= section.End {
				tasks++
				if !task.Done {
					open++
				}
			}
		}
		if section.CodeBlocks != code || section.Tasks != tasks || section.OpenTasks != open {
			t.Errorf("section %q counts = %d/%d/%d, want %d/%d/%d", section.Path, section.CodeBlocks, section.Tasks, section.OpenTasks, code, tasks, open)
		}
	}
	lookup := func(line int) *model.Section {
		for i := len(doc.Sections) - 1; i >= 0; i-- {
			if line >= doc.Sections[i].Start && line <= doc.Sections[i].End {
				return &doc.Sections[i]
			}
		}
		return nil
	}
	for line := 0; line <= len(doc.Lines)+1; line++ {
		if got, want := sectionByLine(doc.Sections, line), lookup(line); got != want {
			t.Errorf("line %d section = %#v, want %#v", line, got, want)
		}
	}
	check := func(line int, section, path string) {
		t.Helper()
		wantSection, wantPath := "", ""
		if expected := lookup(line); expected != nil {
			wantSection, wantPath = expected.Title, expected.Path
		}
		if section != wantSection || path != wantPath {
			t.Errorf("line %d section/path = %q/%q, want %q/%q", line, section, path, wantSection, wantPath)
		}
	}
	for _, heading := range doc.Headings {
		check(heading.Start, heading.Section, heading.Path)
	}
	for _, block := range doc.Blocks {
		check(block.Start, block.Section, block.Path)
	}
	for _, task := range doc.Tasks {
		check(task.Line, task.Section, task.SectionPath)
	}
	for _, quote := range doc.Quotes {
		check(quote.Start, quote.Section, quote.Path)
	}
	for _, link := range doc.Links {
		check(link.Line, link.Section, link.Path)
	}
	for _, tag := range doc.Tags {
		check(tag.Line, tag.Section, tag.Path)
	}
	for _, line := range doc.LinesRows {
		check(line.N, line.Section, line.Path)
		kind := "blank"
		if line.N >= doc.FrontMatterFrom && line.N <= doc.FrontMatterTo {
			kind = model.KindFrontmatter
		}
		for _, block := range doc.Blocks {
			if line.N >= block.Start && line.N <= block.End {
				kind = block.Kind
				break
			}
		}
		if line.Kind != kind {
			t.Errorf("line %d kind = %q, want %q", line.N, line.Kind, kind)
		}
	}
}

func BenchmarkParseHeadings(b *testing.B) {
	for _, count := range []int{8000, 16000, 60000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			var source strings.Builder
			for i := 0; i < count; i++ {
				fmt.Fprintf(&source, "## Heading %d\nParagraph %d.\n", i, i)
			}
			raw := source.String()
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				doc, err := ParseFile("headings.md", raw, int64(len(raw)), time.Time{})
				if err != nil {
					b.Fatal(err)
				}
				if len(doc.Headings) != count {
					b.Fatalf("headings = %d, want %d", len(doc.Headings), count)
				}
			}
		})
	}
}
