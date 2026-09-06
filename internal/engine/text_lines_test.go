package engine_test

import (
	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
	"strings"
	"testing"
	"time"
)

func TestSectionTextLineFunctions(t *testing.T) {
	source := "# Лог\n\nfirst\nsecond\n \t\n\n# Last\n\nlast1\nlast2\n\n \t\n"
	doc, err := parser.ParseFile("synthetic.md", source, int64(len(source)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	q, err := query.Parse("SELECT title, tail(raw, 2) AS tail, head(raw, 1) AS head, lines(raw) AS lines, lines(raw, true) AS exact, raw FROM sections WHERE title IN ('Лог', 'Last') ORDER BY start")
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Execute(q, []*parser.Document{doc}, engine.Options{Strict: true})
	if err != nil || len(result.Rows) != 2 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	for i, want := range []string{"first\nsecond", "last1\nlast2"} {
		row := result.Rows[i]
		if row["tail"].String() != want || row["lines"].Data != int64(4) || row["head"].String() != "# "+row["title"].String() || row["exact"].Data != int64(len(strings.Split(row["raw"].String(), "\n"))) || !strings.HasSuffix(row["raw"].String(), "\n") {
			t.Fatalf("row=%v", row)
		}
	}
}
