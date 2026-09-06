package engine_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func TestCellsQueries(t *testing.T) {
	source := "# Routes\n\n| Name | Workflow |\n| --- | --- |\n| Code | build\\|test |\n| Ship | release |\n\n| Name | Workflow |\n| --- | --- |\n| Deploy | rollout |\n"
	doc, err := parser.ParseFile("synthetic.md", source, int64(len(source)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		sql    string
		values []string
	}{
		{"SELECT value FROM cells WHERE header = 'Workflow'", []string{"Workflow", "build|test", "release", "Workflow", "rollout"}},
		{"SELECT value FROM cells WHERE header = 'Workflow' AND row > 0 ORDER BY table, row", []string{"build|test", "release", "rollout"}},
		{"SELECT value FROM cells WHERE table = 2 AND row = 0 ORDER BY col", []string{"Name", "Workflow"}},
		{"SELECT value FROM cells WHERE table = 1 AND row = 1 AND col = 2", []string{"build|test"}},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			q, err := query.Parse(tc.sql)
			if err != nil {
				t.Fatal(err)
			}
			result, err := engine.Execute(q, []*parser.Document{doc}, engine.Options{})
			if err != nil {
				t.Fatal(err)
			}
			values := []string{}
			for _, row := range result.Rows {
				values = append(values, row["value"].String())
			}
			if !reflect.DeepEqual(values, tc.values) {
				t.Fatalf("values=%v want=%v", values, tc.values)
			}
		})
	}
}
