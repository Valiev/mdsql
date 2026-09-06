package schema

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaDocumentsAllFunctions(t *testing.T) {
	want := []string{
		"COUNT(*)", "after_heading(title[, level])", "at(n)", "avg(x)", "basename(p)", "ceil(x)", "coalesce(a, b, ...)",
		"contains(string, substring) or contains(list, scalar)", "count(x)", "date(s)", "dirname(p)",
		"floor(x)", "has_tag(t)", "head(s, n[, exact])", "icontains(s, sub)", "len(x)", "lines(s[, exact])",
		"lower(s)", "max(x)", "min(x)", "now()", "round(x[, digits])", "sum(x)", "tail(s, n[, exact])", "today()", "trim(s)", "under(title)", "under_contains(text)", "under_like(pattern)", "upper(s)",
	}
	var out bytes.Buffer
	if err := JSON(&out, false, false); err != nil {
		t.Fatal(err)
	}
	var schema Schema
	if err := json.Unmarshal(out.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema.Funcs, want) {
		t.Fatalf("functions=%v, want=%v", schema.Funcs, want)
	}
	out.Reset()
	Text(&out, false, false)
	for _, signature := range want {
		if !strings.Contains(out.String(), "  - "+signature+"\n") {
			t.Errorf("text schema missing %q", signature)
		}
	}
}

func TestHasTagScopeIsDocumented(t *testing.T) {
	schema := collect()
	for _, scope := range []string{"tasks", "headings/lines/blocks", "files/frontmatter", "hierarchical"} {
		if !strings.Contains(schema.FunctionDocs["has_tag"], scope) {
			t.Errorf("has_tag description missing %s", scope)
		}
	}
	var out bytes.Buffer
	Text(&out, false, false)
	if !strings.Contains(out.String(), schema.FunctionDocs["has_tag"]) {
		t.Fatal("text schema lost function scope")
	}
}
