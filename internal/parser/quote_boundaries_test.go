package parser_test

import (
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/parser"
)

func TestQuotesSplitAtUnquotedBlankLines(t *testing.T) {
	raw := "> first\n\n> [!warning]- Warning\n> - [ ] Warned\n \t\n> normal\n>\n> continued\n\n> [!note]+ Note\n> - [ ] Noted\n"
	doc, err := parser.ParseFile("quotes.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		start, end        int
		kind, title, fold string
	}{
		{1, 1, "", "", ""},
		{3, 4, "warning", "Warning", "-"},
		{6, 8, "", "", ""},
		{10, 11, "note", "Note", "+"},
	}
	if len(doc.Quotes) != len(want) || len(doc.Blocks) != len(want) {
		t.Fatalf("quotes=%#v blocks=%#v", doc.Quotes, doc.Blocks)
	}
	for i, expected := range want {
		quote := doc.Quotes[i]
		if quote.Start != expected.start || quote.End != expected.end || quote.Type != expected.kind || quote.Title != expected.title || quote.Fold != expected.fold {
			t.Errorf("quote %d = %#v, want %#v", i, quote, expected)
		}
		if doc.Blocks[i].Start != expected.start || doc.Blocks[i].End != expected.end {
			t.Errorf("quote block range = %#v", doc.Blocks[i])
		}
	}
	for _, line := range []int{2, 5, 9} {
		if doc.LinesRows[line-1].Kind != "blank" {
			t.Errorf("unquoted blank %d belongs to quote", line)
		}
	}
	if doc.Quotes[2].Body != "normal\n\ncontinued" || len(doc.Tasks) != 2 || doc.Tasks[0].Container != "callout" || doc.Tasks[1].Container != "callout" {
		t.Fatalf("quoted blank or callout tasks changed: %#v %#v", doc.Quotes, doc.Tasks)
	}
}
