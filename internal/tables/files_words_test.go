package tables

import (
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/parser"
)

func TestFilesWordsExcludeFrontmatter(t *testing.T) {
	source := "---\ntitle: several metadata words\ntags: [one, two]\n---\ntwo body\nwords\n"
	doc, err := parser.ParseFile("note.md", source, int64(len(source)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	row := rowsFiles(doc)[0]
	if got := row["words"].Data; got != int64(3) {
		t.Fatalf("words = %v, want 3", got)
	}
}
