package parser

import (
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/model"
)

func TestStandaloneDashLineIsBodyThematicBreak(t *testing.T) {
	for _, source := range []string{"---\n", "Paragraph\n\n---\n"} {
		doc, err := ParseFile("note.md", source, int64(len(source)), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		last := doc.Blocks[len(doc.Blocks)-1]
		if last.Kind != model.KindHr || last.Raw != "---" {
			t.Fatalf("last block = %#v", last)
		}
	}
}
