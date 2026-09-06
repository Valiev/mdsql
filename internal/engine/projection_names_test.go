package engine

import (
	"reflect"
	"testing"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

func TestAutomaticNamesReserveExplicitAliases(t *testing.T) {
	q, err := query.Parse("SELECT len(title), len(file), line AS len_2 FROM headings ORDER BY len_3")
	if err != nil {
		t.Fatal(err)
	}
	original := append([]query.SelectItem(nil), q.Select...)
	doc := &model.Document{Path: "note.md", Headings: []model.Heading{{Title: "Text", Start: 1}}}
	res, err := Execute(q, []*model.Document{doc}, Options{})
	if err != nil || !reflect.DeepEqual(res.Columns, []string{"len", "len_3", "len_2"}) {
		t.Fatalf("result=%v err=%v", res, err)
	}
	if !reflect.DeepEqual(q.Select, original) {
		t.Fatal("query AST mutated")
	}
}

func TestRepeatedCountGetsAutomaticNames(t *testing.T) {
	q, err := query.Parse("SELECT count(*), count(*) FROM headings")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Execute(q, nil, Options{})
	if err != nil || !reflect.DeepEqual(res.Columns, []string{"count", "count_2"}) {
		t.Fatalf("result=%v err=%v", res, err)
	}
}
