package parser_test

import (
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/tables"
)

func TestListAndTaskNestingAgree(t *testing.T) {
	raw := "- plain\n- [ ] task\n  - child\n  - [ ] child task\n    - grandchild\n    - [x] grandchild task\n> - [ ] quote task\n>   - [ ] quote child\n"
	doc, err := parser.ParseFile("lists.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		block := doc.Blocks[i]
		if block.Nested != (i >= 2) || block.Level != i/2 {
			t.Errorf("list block %d nesting = %#v", i, block)
		}
	}
	for _, task := range doc.Tasks {
		if task.Nested != (task.Depth > 0) || task.Level != task.Depth {
			t.Errorf("task nesting = %#v", task)
		}
	}
	for _, name := range []string{"lists", "items"} {
		table, _ := tables.Table(name)
		rows := table.Rows(doc)
		if len(rows) != 6 {
			t.Fatalf("%s rows = %#v", name, rows)
		}
		for i, row := range rows {
			if row["nested"].Data.(bool) != (i >= 2) {
				t.Errorf("%s row %d nesting = %#v", name, i, row)
			}
		}
	}
}
