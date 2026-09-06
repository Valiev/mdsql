package output

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/query"
)

func TestDelimitedNullCells(t *testing.T) {
	rows := []query.Row{{
		"empty": query.NewString(""), "literal": query.NewString("-"), "null": query.NewNull(),
	}}
	for _, format := range []Format{FormatCSV, FormatTSV} {
		for _, noHeader := range []bool{false, true} {
			for _, marker := range []*string{nil, stringPointer("NULL"), stringPointer(""), stringPointer("missing, \"value\"\ttab\nnewline")} {
				var out bytes.Buffer
				if err := Render(&out, rows, format, 0, false, false, "", []string{"empty", "literal", "null"}, Options{NoHeader: noHeader, Null: marker}); err != nil {
					t.Fatal(err)
				}
				reader := csv.NewReader(&out)
				wantNull := ""
				if marker != nil {
					wantNull = *marker
				}
				if format == FormatTSV {
					reader.Comma = '\t'
					wantNull = strings.ReplaceAll(strings.ReplaceAll(wantNull, "\n", "\\n"), "\t", "\\t")
				}
				records, err := reader.ReadAll()
				if err != nil {
					t.Fatal(err)
				}
				wantRows := 2
				if noHeader {
					wantRows = 1
				}
				if len(records) != wantRows {
					t.Fatalf("format=%s noHeader=%v records=%v", format, noHeader, records)
				}
				row := records[len(records)-1]
				if len(row) != 3 || row[0] != "" || row[1] != "-" || row[2] != wantNull {
					t.Fatalf("format=%s noHeader=%v got=%q want null=%q", format, noHeader, row, wantNull)
				}
			}
		}
	}
	if rows[0]["null"].Kind != query.ValueNull {
		t.Fatal("render changed an input NULL")
	}
}

func TestGroupedNullCells(t *testing.T) {
	rows := []query.Row{{
		"file": query.NewString("note.md"), "literal": query.NewString("-"), "null": query.NewNull(),
	}}
	for _, tc := range []struct {
		marker *string
		want   string
	}{
		{nil, "literal\tnull\n@\"note.md\"\n-\t\n"},
		{stringPointer(""), "literal\tnull\n@\"note.md\"\n-\t\n"},
		{stringPointer("NULL"), "literal\tnull\n@\"note.md\"\n-\tNULL\n"},
		{stringPointer("N\tA\nB"), "literal\tnull\n@\"note.md\"\n-\tN\\tA\\nB\n"},
	} {
		var out bytes.Buffer
		if err := Render(&out, rows, FormatGrouped, 0, false, false, "", []string{"file", "literal", "null"}, Options{Null: tc.marker}); err != nil {
			t.Fatal(err)
		}
		if out.String() != tc.want {
			t.Fatalf("got=%q want=%q", out.String(), tc.want)
		}
	}
}

func TestTextNullDisplay(t *testing.T) {
	rows := []query.Row{{"literal": query.NewString("-"), "null": query.NewNull()}}
	for _, tc := range []struct {
		marker   *string
		maxLines int
		want     string
	}{
		{nil, 0, "literal=-\tnull=-\n"},
		{stringPointer(""), 0, "literal=-\tnull=\n"},
		{stringPointer("NULL"), 0, "literal=-\tnull=NULL\n"},
		{stringPointer("N\tA\nB"), 0, "literal=-\nnull:\nN\\tA\nB\n"},
		{stringPointer("N\tA\nB"), 1, "literal=-\nnull:\nN\\tA\n… (+1 lines)\n"},
	} {
		var out bytes.Buffer
		if err := Render(&out, rows, FormatText, tc.maxLines, false, false, "", []string{"literal", "null"}, Options{Null: tc.marker}); err != nil {
			t.Fatal(err)
		}
		if out.String() != tc.want {
			t.Fatalf("got=%q want=%q", out.String(), tc.want)
		}
	}
}

func TestJSONKeepsTypedNull(t *testing.T) {
	rows := []query.Row{{"literal": query.NewString("-"), "null": query.NewNull()}}
	for _, format := range []Format{FormatJSON, FormatJSONL} {
		var out bytes.Buffer
		if err := Render(&out, rows, format, 0, false, false, "", []string{"literal", "null"}, Options{Null: stringPointer("NULL")}); err != nil {
			t.Fatal(err)
		}
		want := "{\"literal\":\"-\",\"null\":null}"
		if format == FormatJSON {
			want = "[" + want + "]"
		}
		if out.String() != want+"\n" {
			t.Fatalf("format=%s got=%q want=%q", format, out.String(), want)
		}
	}
}

func stringPointer(value string) *string {
	return &value
}
