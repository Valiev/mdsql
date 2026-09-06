package query

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseAdvancedGroupingClauses(t *testing.T) {
	q, err := Parse("SELECT file, section, count(*) AS n FROM tasks GROUP BY file, section HAVING n > 1 ORDER BY file LIMIT 2 PER file, section")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(q.GroupByColumns, []string{"file", "section"}) || q.GroupBy != "file" {
		t.Fatalf("group by=%v legacy=%q", q.GroupByColumns, q.GroupBy)
	}
	if q.Having == nil || q.Limit != 2 || !reflect.DeepEqual(q.LimitPer, []string{"file", "section"}) {
		t.Fatalf("query=%#v", q)
	}
}

func TestLimitPerRejectsOffset(t *testing.T) {
	_, err := Parse("SELECT file, n FROM lines ORDER BY file, n DESC LIMIT 2 PER file OFFSET 1")
	if err == nil || !strings.Contains(err.Error(), "OFFSET cannot be used with LIMIT PER") {
		t.Fatalf("error=%v", err)
	}
}
