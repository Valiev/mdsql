package output

import (
	"bytes"
	"testing"

	"github.com/Valiev/mdsql/internal/query"
)

func TestFloatOutputRemainsNumeric(t *testing.T) {
	rows := []query.Row{{"average": query.NewFloat(1.5)}}
	for _, tc := range []struct {
		format Format
		want   string
	}{
		{FormatJSON, "[{\"average\":1.5}]\n"},
		{FormatJSONL, "{\"average\":1.5}\n"},
		{FormatCSV, "average\n1.5\n"},
		{FormatTSV, "average\n1.5\n"},
		{FormatText, "average=1.5\n"},
	} {
		var out bytes.Buffer
		if err := Render(&out, rows, tc.format, 0, false, false, "", []string{"average"}); err != nil {
			t.Fatal(err)
		}
		if out.String() != tc.want {
			t.Errorf("format=%s output=%q want=%q", tc.format, out.String(), tc.want)
		}
	}
}

func TestHumanFormatsCompactFloatsOnly(t *testing.T) {
	rows := []query.Row{{"file": query.NewString("note.md"), "average": query.NewFloat(1.234567)}}
	for _, tc := range []struct {
		format Format
		keys   []string
		want   string
	}{
		{FormatText, []string{"average"}, "average=1.23\n"},
		{Format("plain"), []string{"average"}, "1.23\n"},
		{FormatJSON, []string{"average"}, "[{\"average\":1.234567,\"file\":\"note.md\"}]\n"},
		{FormatCSV, []string{"average"}, "average\n1.234567\n"},
		{FormatTSV, []string{"average"}, "average\n1.234567\n"},
		{FormatGrouped, []string{"file", "average"}, "average\n@\"note.md\"\n1.234567\n"},
	} {
		var out bytes.Buffer
		if err := Render(&out, rows, tc.format, 0, false, false, "", tc.keys); err != nil {
			t.Fatal(err)
		}
		if out.String() != tc.want {
			t.Errorf("format=%s output=%q want=%q", tc.format, out.String(), tc.want)
		}
	}
	for _, tc := range []struct {
		value float64
		want  string
	}{{1.2, "1.2"}, {1, "1"}, {-0.001, "0"}} {
		if got := scalarToHumanText(query.NewFloat(tc.value)); got != tc.want {
			t.Errorf("value=%v got=%q want=%q", tc.value, got, tc.want)
		}
	}
}
