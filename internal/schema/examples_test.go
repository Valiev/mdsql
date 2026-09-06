package schema

import (
	"testing"

	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/query"
)

func TestSchemaExamplesAreExecutable(t *testing.T) {
	for _, sql := range examples() {
		t.Run(sql, func(t *testing.T) {
			q, err := query.Parse(sql)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Execute(q, nil, engine.Options{Strict: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
