package engine

import (
	"reflect"
	"sort"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

// Map identity keeps documents with the same displayed file name separate.
// Tags stay outside the row so SELECT * exposes only the table's columns.
type rowTagContext map[reflect.Value][]string

func (tags rowTagContext) forRow(row query.Row) []string {
	return tags[reflect.ValueOf(row)]
}

func (tags rowTagContext) bind(table string, doc *model.Document, rows []query.Row) {
	switch table {
	case "files", "frontmatter", "headings", "lines", "blocks":
	default:
		return
	}
	var frontmatter []string
	var inline []model.Tag
	for _, tag := range doc.Tags {
		if tag.Source == "frontmatter" {
			frontmatter = append(frontmatter, tag.Tag)
		} else if tag.Source == "body" {
			inline = append(inline, tag)
		}
	}
	sort.SliceStable(inline, func(i, j int) bool { return inline[i].Line < inline[j].Line })
	for _, row := range rows {
		var selected []string
		switch table {
		case "files", "frontmatter":
			selected = frontmatter
		case "headings", "lines", "blocks":
			start, end := row["start"], row["end"]
			if table == "lines" {
				start, end = row["n"], row["n"]
			}
			if start.Kind != query.ValueInt || end.Kind != query.ValueInt {
				continue
			}
			first, last := start.Data.(int64), end.Data.(int64)
			i := sort.Search(len(inline), func(i int) bool { return int64(inline[i].Line) >= first })
			for ; i < len(inline) && int64(inline[i].Line) <= last; i++ {
				selected = append(selected, inline[i].Tag)
			}
		}
		if len(selected) > 0 {
			tags[reflect.ValueOf(row)] = selected
		}
	}
}
