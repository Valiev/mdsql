package engine

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

type headingIndex map[string]int

// Row identity keeps documents with the same displayed file name separate.
type rowHeadingContext map[reflect.Value]headingIndex

func (headings rowHeadingContext) bind(table string, doc *model.Document, rows []query.Row) {
	if headings == nil || table != "lines" {
		return
	}
	index := headingIndex{}
	for _, heading := range doc.Headings {
		title := strings.ToLower(heading.Title)
		for _, level := range []int{0, heading.Level} {
			key := title + "\x00" + strconv.Itoa(level)
			if _, exists := index[key]; !exists {
				index[key] = heading.Start
			}
		}
	}
	for _, row := range rows {
		headings[reflect.ValueOf(row)] = index
	}
}

func (headings rowHeadingContext) after(row query.Row, title string, level int) bool {
	line := row["n"]
	if line.Kind != query.ValueInt {
		return false
	}
	start, exists := headings[reflect.ValueOf(row)][strings.ToLower(title)+"\x00"+strconv.Itoa(level)]
	return exists && line.Data.(int64) > int64(start)
}
