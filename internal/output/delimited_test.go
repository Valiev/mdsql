package output

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/query"
)

func TestNoHeaderOnlyRemovesDelimitedHeader(t *testing.T) {
	rows := []query.Row{
		{"title": query.NewString("commas, \"quotes\"\ttab\nnewline"), "level": query.NewInt(2)},
		{"title": query.NewString("second"), "level": query.NewInt(1)},
	}
	for _, format := range []Format{FormatCSV, FormatTSV} {
		t.Run(string(format), func(t *testing.T) {
			var withHeader, withoutHeader bytes.Buffer
			for _, noHeader := range []bool{false, true} {
				out := &withHeader
				if noHeader {
					out = &withoutHeader
				}
				if err := Render(out, rows, format, 0, false, false, "", []string{"title", "level"}, Options{NoHeader: noHeader}); err != nil {
					t.Fatal(err)
				}
			}
			_, data, ok := strings.Cut(withHeader.String(), "\n")
			if !ok || data != withoutHeader.String() {
				t.Fatalf("--no-header changed data: with=%q without=%q", withHeader.String(), withoutHeader.String())
			}
			reader := csv.NewReader(&withoutHeader)
			wantTitle := rows[0]["title"].String()
			if format == FormatTSV {
				reader.Comma = '\t'
				wantTitle = "commas, \"quotes\"\\ttab\\nnewline"
			}
			records, err := reader.ReadAll()
			if err != nil || len(records) != 2 {
				t.Fatalf("records=%v error=%v", records, err)
			}
			if format == FormatCSV && (records[0][0] != wantTitle || records[0][1] != "2" || records[1][0] != "second") {
				t.Fatalf("CSV did not preserve selected order/data: %v", records)
			}
			if format == FormatTSV && (records[0][0] != wantTitle || records[0][1] != "2" || records[1][0] != "second") {
				t.Fatalf("TSV did not preserve selected order/escaped data: %v", records)
			}
		})
	}
}

func TestPlainPrintsEverySelectedColumnInOrder(t *testing.T) {
	rows := []query.Row{{
		"title": query.NewString("Title"),
		"line":  query.NewInt(7),
		"tags":  query.NewList([]query.Value{query.NewString("one"), query.NewString("two")}),
	}}
	var out bytes.Buffer
	if err := Render(&out, rows, Format("plain"), 0, false, false, "", []string{"title", "line", "tags"}); err != nil {
		t.Fatal(err)
	}
	if want := "Title\t7\t[\"one\",\"two\"]\n"; out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}

func TestNoHeaderValidationAndEmptyResults(t *testing.T) {
	for _, format := range []Format{FormatText, FormatJSON, FormatJSONL, FormatGrouped, FormatMD, Format("plain"), Format("outline")} {
		var out bytes.Buffer
		if err := Render(&out, nil, format, 0, false, false, "", []string{"*"}, Options{NoHeader: true}); err == nil || out.Len() != 0 {
			t.Fatalf("format=%q error=%v output=%q", format, err, out.String())
		}
	}
	for _, format := range []Format{FormatCSV, FormatTSV} {
		for _, noHeader := range []bool{false, true} {
			var out bytes.Buffer
			if err := Render(&out, nil, format, 0, false, false, "", []string{"title"}, Options{NoHeader: noHeader}); err != nil || out.Len() != 0 {
				t.Fatalf("format=%q noHeader=%v error=%v output=%q", format, noHeader, err, out.String())
			}
		}
	}
}

func TestDelimitedAndGroupedReturnWriterErrors(t *testing.T) {
	for _, format := range []Format{FormatCSV, FormatTSV, FormatGrouped, FormatText} {
		for _, size := range []int{10, 10000} {
			rows := []query.Row{{"file": query.NewString("note.md"), "text": query.NewString(strings.Repeat("x", size))}}
			if err := Render(failedFormatWriter{}, rows, format, 0, false, false, "", []string{"file", "text"}); err == nil {
				t.Fatalf("format=%q size=%d swallowed the writer error", format, size)
			}
		}
	}
}
