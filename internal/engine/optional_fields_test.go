package engine

import (
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

func TestOptionalFieldDiagnosticsAcrossDocuments(t *testing.T) {
	for _, tc := range []struct {
		name           string
		docs           []*model.Document
		sql            string
		warnings, rows int
	}{
		{"some-present", []*model.Document{{FrontMatter: model.FrontMatter{"stage": "intake"}}, {}}, "SELECT file FROM files WHERE fm.stage = 'intake'", 0, 1},
		{"all-absent", []*model.Document{{}, {}}, "SELECT file FROM files WHERE fm.stage = 'intake'", 1, 0},
		{"explicit-null", []*model.Document{{FrontMatter: model.FrontMatter{"stage": nil}}, {}}, "SELECT file FROM files WHERE fm.stage = 'intake'", 0, 0},
		{"nested", []*model.Document{{FrontMatter: model.FrontMatter{"meta": map[string]interface{}{"stage": "intake"}}}, {}}, "SELECT file FROM files WHERE fm.meta.stage = 'intake'", 0, 1},
		{"no-selected-rows-in-present-document", []*model.Document{{FrontMatter: model.FrontMatter{"stage": "intake"}}, {Headings: []model.Heading{{Title: "Heading"}}}}, "SELECT title FROM headings WHERE fm.stage = 'intake'", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := query.Parse(tc.sql)
			if err != nil {
				t.Fatal(err)
			}
			res, err := Execute(q, tc.docs, Options{})
			if err != nil || len(res.Rows) != tc.rows || len(res.Warnings) != tc.warnings {
				t.Fatalf("result=%+v error=%v", res, err)
			}
			_, err = Execute(q, tc.docs, Options{Strict: true})
			if tc.warnings == 0 && err != nil {
				t.Fatalf("optional field rejected: %v", err)
			}
			if tc.warnings > 0 && (err == nil || !strings.Contains(err.Error(), "missing field")) {
				t.Fatalf("missing diagnostic: %v", err)
			}
		})
	}
}
