package engine

import (
	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
	"github.com/Valiev/mdsql/internal/tables"
)

func presentFrontmatterFields(docs []*model.Document) map[string]bool {
	present := make(map[string]bool)
	var collect func(string, query.Value)
	collect = func(path string, value query.Value) {
		present[path] = true
		if value.Kind == query.ValueObject {
			for key, child := range value.Data.(map[string]query.Value) {
				collect(path+"."+key, child)
			}
		}
	}
	frontmatter, _ := tables.Table("frontmatter")
	for _, doc := range docs {
		for _, row := range frontmatter.Rows(doc) {
			collect("fm", row["fm"])
		}
	}
	return present
}
