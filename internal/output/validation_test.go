package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/query"
)

func TestStructuredTextFormatsRejectBadProjections(t *testing.T) {
	for _, format := range []Format{FormatMD, Format("outline")} {
		for _, rows := range [][]query.Row{nil, {{"text": query.NewString("task")}}} {
			var out bytes.Buffer
			err := Render(&out, rows, format, 0, false, false, "tasks", []string{"text"})
			if err == nil || !strings.Contains(err.Error(), "requires") || out.Len() != 0 {
				t.Fatalf("format=%q error=%v output=%q", format, err, out.String())
			}
		}
	}
}

func TestFormatValidationPreventsPartialOutput(t *testing.T) {
	tests := []struct {
		format Format
		keys   []string
		rows   []query.Row
	}{
		{FormatMD, []string{"raw"}, []query.Row{{"raw": query.NewString("valid")}, {"raw": query.NewNull()}}},
		{Format("outline"), []string{"level", "title"}, []query.Row{
			{"level": query.NewInt(2), "title": query.NewString("valid")},
			{"level": query.NewInt(1000000000), "title": query.NewString("bad")},
		}},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		if err := Render(&out, tt.rows, tt.format, 0, false, false, "", tt.keys); err == nil || out.Len() != 0 {
			t.Fatalf("format=%q error=%v output=%q", tt.format, err, out.String())
		}
	}
}

func TestValidAndEmptyContentFormats(t *testing.T) {
	var out bytes.Buffer
	if err := Render(&out, []query.Row{{"level": query.NewInt(2), "title": query.NewString("Title")}}, Format("outline"), 0, false, false, "", []string{"level", "title"}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "  - Title\n" {
		t.Fatalf("unexpected outline: %q", out.String())
	}
	out.Reset()
	if err := Render(&out, []query.Row{{"code": query.NewString("hello")}}, FormatMD, 0, false, false, "", []string{"code"}); err != nil || out.String() != "hello\n\n" {
		t.Fatalf("valid code content: error=%v output=%q", err, out.String())
	}
	out.Reset()
	for _, format := range []Format{FormatMD, Format("outline")} {
		if err := Render(&out, nil, format, 0, false, false, "", []string{"*"}); err != nil || out.Len() != 0 {
			t.Fatalf("empty wildcard output: error=%v output=%q", err, out.String())
		}
	}
}

type failedFormatWriter struct{}

func (failedFormatWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestContentFormatsReturnWriterErrors(t *testing.T) {
	for _, format := range []Format{FormatMD, Format("outline")} {
		rows := []query.Row{{"raw": query.NewString("text"), "level": query.NewInt(1), "title": query.NewString("title")}}
		if err := Render(failedFormatWriter{}, rows, format, 0, false, false, "", []string{"*"}); err == nil {
			t.Fatalf("format %q swallowed the writer error", format)
		}
	}
}
