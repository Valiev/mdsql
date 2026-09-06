package output

import (
	"bytes"
	"testing"

	"github.com/Valiev/mdsql/internal/query"
)

func TestRenderCSVUsesSelectOrderAndEscapesValues(t *testing.T) {
	rows := []query.Row{{"line": query.NewInt(7), "raw": query.NewString("hello, world")}}
	var output bytes.Buffer
	if err := Render(&output, rows, FormatCSV, 0, false, false, "", []string{"line", "raw"}); err != nil {
		t.Fatal(err)
	}
	want := "line,raw\n7,\"hello, world\"\n"
	if output.String() != want {
		t.Fatalf("got %q, want %q", output.String(), want)
	}
}

func TestTextCompactScalarRows(t *testing.T) {
	rows := []query.Row{
		{"line": query.NewInt(7), "text": query.NewString("first")},
		{"line": query.NewInt(9), "text": query.NewString("second")},
	}
	var out bytes.Buffer
	if err := Render(&out, rows, FormatText, 0, false, false, "", []string{"text", "line"}); err != nil {
		t.Fatal(err)
	}
	want := "text=first\tline=7\ntext=second\tline=9\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}

func TestTextMultilineRowsWithoutExtraSeparators(t *testing.T) {
	rows := []query.Row{
		{"line": query.NewInt(7), "raw": query.NewString("first\tline\nsecond\nthird"), "body": query.NewString("body1\nbody2")},
		{"line": query.NewInt(9), "raw": query.NewString("next\nlast"), "body": query.NewString("end")},
	}
	for _, tc := range []struct {
		name     string
		selected []string
		maxLines int
		want     string
	}{
		{"mixed", []string{"line", "raw", "body"}, 0, "line=7\nraw:\nfirst\\tline\nsecond\nthird\nbody:\nbody1\nbody2\nline=9\tbody=end\nraw:\nnext\nlast\n"},
		{"multiline only", []string{"raw"}, 0, "raw:\nfirst\\tline\nsecond\nthird\nraw:\nnext\nlast\n"},
		{"truncated", []string{"line", "raw", "body"}, 1, "line=7\nraw:\nfirst\\tline\n… (+2 lines)\nbody:\nbody1\n… (+1 lines)\nline=9\tbody=end\nraw:\nnext\n… (+1 lines)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := Render(&out, rows, FormatText, tc.maxLines, false, false, "", tc.selected); err != nil {
				t.Fatal(err)
			}
			if out.String() != tc.want {
				t.Fatalf("got %q, want %q", out.String(), tc.want)
			}
		})
	}
}

func TestTextPreservesContentBlankLines(t *testing.T) {
	var out bytes.Buffer
	rows := []query.Row{{"raw": query.NewString("first\n\nlast\n")}}
	if err := Render(&out, rows, FormatText, 0, false, false, "", []string{"raw"}); err != nil {
		t.Fatal(err)
	}
	if want := "raw:\nfirst\n\nlast\n\n"; out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
