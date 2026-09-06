package engine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

func clauseDocument() *model.Document {
	return &model.Document{Path: "synthetic.md", Headings: []model.Heading{
		{File: "synthetic.md", Title: "B", Level: 2, Start: 1},
		{File: "synthetic.md", Title: "A", Level: 1, Start: 2},
		{File: "synthetic.md", Title: "C", Level: 2, Start: 3},
	}}
}

func TestOrderBySelectPosition(t *testing.T) {
	for _, tc := range []struct {
		sql    string
		column string
		want   []string
	}{
		{"SELECT level, title FROM headings ORDER BY 2 DESC", "title", []string{"C", "B", "A"}},
		{"SELECT level, title AS label FROM headings ORDER BY +2", "label", []string{"A", "B", "C"}},
		{"SELECT title FROM headings ORDER BY (1)", "title", []string{"A", "B", "C"}},
		{"SELECT *, lower(title) AS label FROM headings ORDER BY 6 DESC", "title", []string{"C", "B", "A"}},
		{"SELECT title AS level, level AS title FROM headings ORDER BY 1", "level", []string{"A", "B", "C"}},
		{"SELECT level AS category, count(*) AS total FROM headings GROUP BY category ORDER BY 2 DESC", "category", []string{"2", "1"}},
		{"SELECT title FROM headings ORDER BY 1 + 0", "title", []string{"B", "A", "C"}},
	} {
		q, err := query.Parse(tc.sql)
		if err != nil {
			t.Fatal(err)
		}
		before := q.OrderBy[0].Expr
		result, err := Execute(q, []*model.Document{clauseDocument()}, Options{Strict: true})
		if err != nil {
			t.Fatal(err)
		}
		var values []string
		for _, row := range result.Rows {
			values = append(values, row[tc.column].String())
		}
		if !reflect.DeepEqual(values, tc.want) || q.OrderBy[0].Expr != before {
			t.Fatalf("%s: values=%v want=%v", tc.sql, values, tc.want)
		}
	}
}

func TestOrderByPositionRejectsInvalidNumbersWithoutRows(t *testing.T) {
	for _, position := range []string{"0", "-1", "-(1)", "99", "9223372036854775807", "-(-9223372036854775808)"} {
		q, err := query.Parse("SELECT title FROM headings ORDER BY " + position)
		if err != nil {
			t.Fatal(err)
		}
		for _, docs := range [][]*model.Document{nil, {clauseDocument()}} {
			_, err = Execute(q, docs, Options{})
			if err == nil || !strings.Contains(err.Error(), "ORDER BY position") || !strings.Contains(err.Error(), "out of range") {
				t.Errorf("position=%s error=%v", position, err)
			}
		}
	}
}

func TestWhereAliasesRespectSourceColumns(t *testing.T) {
	for _, tc := range []struct {
		sql    string
		column string
		want   []string
	}{
		{"SELECT title AS t FROM headings WHERE t = 'A'", "t", []string{"A"}},
		{"SELECT lower(title) AS normalized FROM headings WHERE normalized = 'a'", "normalized", []string{"a"}},
		{"SELECT title AS t FROM headings WHERE lower(t) = 'a'", "t", []string{"A"}},
		{"SELECT title AS t FROM headings WHERE t IN ('A', 'C') ORDER BY t", "t", []string{"A", "C"}},
		{"SELECT lower(title) AS title FROM headings WHERE title = 'A'", "title", []string{"a"}},
		{"SELECT title AS level FROM headings WHERE level = 1", "level", []string{"A"}},
		{"SELECT level AS category, count(*) AS total FROM headings WHERE category = 2 GROUP BY category ORDER BY total DESC", "total", []string{"2"}},
	} {
		q, err := query.Parse(tc.sql)
		if err != nil {
			t.Fatal(err)
		}
		before := q.Where
		result, err := Execute(q, []*model.Document{clauseDocument()}, Options{Strict: true})
		if err != nil {
			t.Fatal(err)
		}
		var values []string
		for _, row := range result.Rows {
			values = append(values, row[tc.column].String())
		}
		if !reflect.DeepEqual(values, tc.want) || len(result.Warnings) != 0 || q.Where != before {
			t.Fatalf("%s: result=%#v want=%v", tc.sql, result, tc.want)
		}
		if _, err := Execute(q, nil, Options{Strict: true}); err != nil {
			t.Fatalf("alias failed without rows: %s: %v", tc.sql, err)
		}
	}
}

func TestAggregateAliasesInWhereFailBeforeFiltering(t *testing.T) {
	for _, sql := range []string{
		"SELECT count(*) AS total FROM headings WHERE total > 1",
		"SELECT sum(level) AS total FROM headings WHERE false AND total > 1",
		"SELECT avg(level) + 1 AS mean FROM headings WHERE mean > 1",
	} {
		q, err := query.Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		for _, docs := range [][]*model.Document{nil, {clauseDocument()}} {
			_, err := Execute(q, docs, Options{})
			if err == nil || !strings.Contains(err.Error(), "aggregate functions are not allowed in WHERE") {
				t.Errorf("%s: error=%v", sql, err)
			}
		}
	}
}
