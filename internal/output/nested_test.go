package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/query"
)

func TestNestedValuesRoundTripInTextAndDelimitedFormats(t *testing.T) {
	values := map[string]query.Value{
		"list":   query.NewList([]query.Value{query.NewString("a,b"), query.NewString("c"), query.NewNull()}),
		"object": query.NewObject(map[string]query.Value{"nested": query.NewList([]query.Value{query.NewInt(2), query.NewBool(true), query.NewString("line\nbreak\tquote\"")})}),
	}
	row := query.Row{"file": query.NewString("sample.md"), "list": values["list"], "object": values["object"]}
	for _, format := range []Format{FormatCSV, FormatTSV, FormatGrouped, FormatText} {
		t.Run(string(format), func(t *testing.T) {
			var out bytes.Buffer
			if err := Render(&out, []query.Row{row}, format, 1, false, false, "", []string{"file", "list", "object"}); err != nil {
				t.Fatal(err)
			}
			cells := map[string]string{}
			if format == FormatText {
				for _, pair := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\t") {
					key, value, ok := strings.Cut(pair, "=")
					if !ok {
						t.Fatalf("unlabelled text cell: %q", pair)
					}
					cells[key] = value
				}
			} else {
				text := out.String()
				if format == FormatGrouped {
					text = strings.Replace(text, "@\"sample.md\"\n", "", 1)
				}
				reader := csv.NewReader(strings.NewReader(text))
				if format != FormatCSV {
					reader.Comma = '\t'
				}
				records, err := reader.ReadAll()
				if err != nil || len(records) != 2 {
					t.Fatalf("output %q: %v", text, err)
				}
				for i, key := range records[0] {
					cells[key] = records[1][i]
				}
			}
			for key, original := range values {
				var decoded any
				if err := json.Unmarshal([]byte(cells[key]), &decoded); err != nil {
					t.Fatalf("%s cell %q: %v", key, cells[key], err)
				}
				got, _ := json.Marshal(decoded)
				want, _ := json.Marshal(toAny(original))
				if !bytes.Equal(got, want) {
					t.Errorf("%s: got %s, want %s", key, got, want)
				}
			}
		})
	}
}

func TestTextCountKeepsItsColumnName(t *testing.T) {
	var out bytes.Buffer
	if err := Render(&out, []query.Row{{"count": query.NewInt(2)}}, FormatText, 0, false, false, "", []string{"count"}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "count=2\n" {
		t.Fatalf("output=%q", out.String())
	}
}
