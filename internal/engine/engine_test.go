package engine

import (
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

func TestExecuteProjectsOnlySelectedColumns(t *testing.T) {
	q, err := query.Parse("SELECT line, raw FROM headings WHERE level = 2")
	if err != nil {
		t.Fatal(err)
	}
	doc := &model.Document{Headings: []model.Heading{{Start: 7, End: 7, Level: 2, Raw: "## Title"}}}
	result, err := Execute(q, []*model.Document{doc}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(result.Rows))
	}
	row := result.Rows[0]
	if len(row) != 2 || row["line"].String() != "7" || row["raw"].String() != "## Title" {
		t.Fatalf("unexpected projection: %#v", row)
	}
}

func TestExecuteRejectsUnknownColumn(t *testing.T) {
	q, err := query.Parse("SELECT does_not_exist FROM headings")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Execute(q, []*model.Document{{}}, Options{})
	if err == nil || !strings.Contains(err.Error(), `unknown column "does_not_exist"`) {
		t.Fatalf("got error %v, want unknown-column error", err)
	}
}
