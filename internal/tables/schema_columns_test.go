package tables

import (
	"testing"

	"github.com/Valiev/mdsql/internal/model"
)

func TestSchemaDeclaresMaterializedColumns(t *testing.T) {
	doc := &model.Document{
		FrontMatter: model.FrontMatter{"label": "demo"},
		Headings:    []model.Heading{{}},
		Sections:    []model.Section{{}},
		Tasks:       []model.Task{{}},
		Links:       []model.Link{{}},
		Tags:        []model.Tag{{}},
		Quotes:      []model.Quote{{}},
		LinesRows:   []model.Line{{}},
		Blocks: []model.Block{
			{Kind: model.KindParagraph}, {Kind: model.KindList},
			{Kind: model.KindTable, Raw: "| Header |\n| --- |\n| Value |"}, {Kind: model.KindCode},
		},
	}
	for _, table := range Registry {
		t.Run(table.Name, func(t *testing.T) {
			declared := map[string]bool{}
			for _, column := range table.Columns {
				if declared[column.Name] {
					t.Errorf("duplicate column %q", column.Name)
				}
				declared[column.Name] = true
			}
			materialized := map[string]bool{}
			for _, row := range table.Rows(doc) {
				for column := range row {
					materialized[column] = true
					if !declared[column] {
						t.Errorf("column %q is missing from schema", column)
					}
				}
			}
			for column := range declared {
				if !materialized[column] {
					t.Errorf("declared column %q is not materialized", column)
				}
			}
		})
	}
}
