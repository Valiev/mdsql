package edit

import (
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func fmTestDocument(t *testing.T, source string) *model.Document {
	t.Helper()
	doc, err := parser.ParseFile("synthetic.md", source, int64(len(source)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func fmTestStatement(t *testing.T, operation string, value query.Value, selector string) *query.WriteStatement {
	t.Helper()
	stmt := &query.WriteStatement{Operation: operation, Table: "frontmatter_values", Values: map[string]query.Value{"value": value}}
	if operation == "INSERT" {
		stmt.Values["key"] = query.NewString(selector)
	} else if selector != "" {
		q, err := query.Parse("SELECT * FROM frontmatter_values WHERE " + selector)
		if err != nil {
			t.Fatal(err)
		}
		stmt.Where = q.Where
	}
	return stmt
}

func TestPlanFrontmatterPreservesBytes(t *testing.T) {
	tests := []struct {
		name, source, operation, selector string
		value                             query.Value
		want                              string
		matched, changed                  int
	}{
		{"create empty", "", "INSERT", "key", query.NewString("true"), "---\nkey: \"true\"\n---\n", 1, 1},
		{"create BOM CRLF", "\ufeff# Body\r\nlast", "INSERT", "literal.key", query.NewNull(), "\ufeff---\r\nliteral.key: null\r\n---\r\n# Body\r\nlast", 1, 1},
		{"BOM only", "\ufeff", "INSERT", "k", query.NewBool(true), "\ufeff---\nk: true\n---\n", 1, 1},
		{"create CR", "first\rlast", "INSERT", "k", query.NewInt(2), "---\rk: 2\r---\rfirst\rlast", 1, 1},
		{"empty FM", "---\n---", "INSERT", "k", query.NewBool(false), "---\nk: false\n---", 1, 1},
		{"comment FM", "---\n# note\n\n---\nbody", "INSERT", "k", query.NewInt(7), "---\n# note\n\nk: 7\n---\nbody", 1, 1},
		{"insert preserves collections", "---\nitems: [one]\nempty: []\nobj: {a: b}\n---\nbody", "INSERT", "k", query.NewFloat(1), "---\nitems: [one]\nempty: []\nobj: {a: b}\nk: 1.0\n---\nbody", 1, 1},
		{"indented mapping", "---\n  a: 1\n---", "INSERT", "b", query.NewInt(2), "---\n  a: 1\n  b: 2\n---", 1, 1},
		{"comments mixed EOL BOM", "\ufeff---\r\n# comment\na: old  # keep\r\nb: 'untouched'\r---\nbody", "UPDATE", "key = 'a'", query.NewString("new"), "\ufeff---\r\n# comment\na: new  # keep\r\nb: 'untouched'\r---\nbody", 1, 1},
		{"unicode column", "---\n\u043a\u043b\u044e\u0447: old # keep\n---", "UPDATE", "key = '\u043a\u043b\u044e\u0447'", query.NewString("next"), "---\n\u043a\u043b\u044e\u0447: next # keep\n---", 1, 1},
		{"literal dotted key", "---\nliteral.key: old\nliteral: {key: keep}\n---", "UPDATE", "key = 'literal.key'", query.NewString("next"), "---\nliteral.key: next\nliteral: {key: keep}\n---", 1, 1},
		{"quoted key and value", "---\n'a''b' : 'it''s # old' # keep\n---", "UPDATE", "key LIKE 'a%b'", query.NewNull(), "---\n'a''b' : null # keep\n---", 1, 1},
		{"escaped double quote", "---\nk: \"say \\\"hi\\\" # x\" # keep\n---", "UPDATE", "key = 'k'", query.NewBool(true), "---\nk: true # keep\n---", 1, 1},
		{"implicit null comment", "---\nk:  # keep\n---", "UPDATE", "key = 'k'", query.NewString("next"), "---\nk: next  # keep\n---", 1, 1},
		{"implicit null no space", "---\nk:\n---", "UPDATE", "key = 'k'", query.NewInt(3), "---\nk: 3\n---", 1, 1},
		{"NULL remains field", "---\nk: old\n---", "UPDATE", "key = 'k'", query.NewNull(), "---\nk: null\n---", 1, 1},
		{"no-op scalar", "---\nk: 'same' # keep\n---", "UPDATE", "key = 'k'", query.NewString("same"), "---\nk: 'same' # keep\n---", 1, 0},
		{"no-op null", "---\nk: ~ # keep\n---", "UPDATE", "key = 'k'", query.NewNull(), "---\nk: ~ # keep\n---", 1, 0},
		{"no-op int", "---\nk: 0x10 # keep\n---", "UPDATE", "key = 'k'", query.NewInt(16), "---\nk: 0x10 # keep\n---", 1, 0},
		{"update all source matches", "---\na: old\nb: new # keep\nc: old\n---", "UPDATE", "key IN ('a', 'b', 'c')", query.NewString("new"), "---\na: new\nb: new # keep\nc: new\n---", 3, 2},
		{"value filter", "---\na: old\nb: other\n---", "UPDATE", "value = 'old'", query.NewString("new"), "---\na: new\nb: other\n---", 1, 1},
		{"no match", "---\na: old\n---", "UPDATE", "key = 'absent'", query.NewString("new"), "---\na: old\n---", 0, 0},
		{"no FM update", "\ufeffbody", "UPDATE", "key = 'absent'", query.NewString("new"), "\ufeffbody", 0, 0},
		{"unrelated list remains", "---\nk: old\nitems: [one]\nempty: []\nobj: {}\n---", "UPDATE", "key = 'k'", query.NewString("new"), "---\nk: new\nitems: [one]\nempty: []\nobj: {}\n---", 1, 1},
		{"date predicate", "---\nd: 2026-09-02\nkeep: 2026-09-03\n---", "UPDATE", "key = 'd' AND value = '2026-09-02'", query.NewString("2026-09-04"), "---\nd: \"2026-09-04\"\nkeep: 2026-09-03\n---", 1, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := []byte(tc.source)
			after, matched, changed, err := planFrontmatter(original, fmTestStatement(t, tc.operation, tc.value, tc.selector), fmTestDocument(t, tc.source))
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != tc.want || matched != tc.matched || changed != tc.changed {
				t.Fatalf("got %q (%d,%d), want %q (%d,%d)", after, matched, changed, tc.want, tc.matched, tc.changed)
			}
			if !bytes.Equal(original, []byte(tc.source)) {
				t.Fatal("modified original buffer")
			}
			if doc := fmTestDocument(t, string(after)); len(doc.Warnings) != 0 {
				t.Fatalf("edited document has parser warnings: %v", doc.Warnings)
			}
		})
	}
}

func TestPlanFrontmatterRejectsUnsafeYAML(t *testing.T) {
	tests := []struct{ name, source, operation, selector string }{
		{"duplicate", "---\nk: a\nk: b\n---", "INSERT", "new"},
		{"nested duplicate", "---\nobj: {a: 1, a: 2}\n---", "INSERT", "new"},
		{"unicode line break", "---\nk: a\u0085other: b\n---", "INSERT", "new"},
		{"broken", "---\nk: [bad\n---", "INSERT", "new"},
		{"unclosed", "---\nk: old", "INSERT", "new"},
		{"root sequence", "---\n- a\n---", "INSERT", "new"},
		{"root flow", "---\n{k: old}\n---", "INSERT", "new"},
		{"tagged empty root", "---\n!!null \"\"\n---", "INSERT", "new"},
		{"empty anchored root", "---\n&a\n---", "INSERT", "new"},
		{"anchor", "---\nk: &a old\n---", "INSERT", "new"},
		{"alias", "---\nk: &a old\nz: *a\n---", "UPDATE", "key = 'k'"},
		{"merge", "---\nk: {<<: {a: b}}\n---", "INSERT", "new"},
		{"tag", "---\nk: !!str old\n---", "UPDATE", "key = 'k'"},
		{"complex key", "---\n? [a, b]\n: old\n---", "INSERT", "new"},
		{"explicit key", "---\n? k\n: old\n---", "INSERT", "new"},
		{"nonstring key", "---\ntrue: old\n---", "INSERT", "new"},
		{"document end", "---\nk: old\n...\n---", "INSERT", "new"},
		{"multiline literal", "---\nk: |\n  a\n  b\n---", "UPDATE", "key = 'k'"},
		{"multiline folded", "---\nk: >-\n  a\n  b\n---", "UPDATE", "key = 'k'"},
		{"multiline plain", "---\nk: a\n  b\n---", "UPDATE", "key = 'k'"},
		{"multiline quoted", "---\nk: 'a\n  b'\n---", "UPDATE", "key = 'k'"},
		{"next line scalar", "---\nk:\n  old\n---", "UPDATE", "key = 'k'"},
		{"overflow uint", "---\nk: 9223372036854775808\n---", "UPDATE", "key = 'k'"},
		{"overflow huge int", "---\nk: 184467440737095516160\n---", "INSERT", "new"},
		{"nonfinite", "---\nk: .inf\n---", "INSERT", "new"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := planFrontmatter([]byte(tc.source), fmTestStatement(t, tc.operation, query.NewString("new"), tc.selector), fmTestDocument(t, tc.source))
			if err == nil {
				t.Fatal("expected explicit error")
			}
		})
	}
}

func TestPlanFrontmatterRejectsCollectionTargets(t *testing.T) {
	for _, collection := range []string{"{}", "{a: b}"} {
		for _, operation := range []string{"INSERT", "UPDATE"} {
			t.Run(operation+collection, func(t *testing.T) {
				source := "---\nitems: " + collection + "\n---"
				selector := "items"
				if operation == "UPDATE" {
					selector = "key = 'items'"
				}
				_, _, _, err := planFrontmatter([]byte(source), fmTestStatement(t, operation, query.NewString("new"), selector), fmTestDocument(t, source))
				if err == nil {
					t.Fatal("expected collection target error")
				}
			})
		}
	}
	source := "---\nitems: scalar\n---"
	_, _, _, err := planFrontmatter([]byte(source), fmTestStatement(t, "INSERT", query.NewString("new"), "items"), fmTestDocument(t, source))
	if err == nil || !strings.Contains(err.Error(), "use UPDATE for a scalar") {
		t.Fatalf("got %v, want scalar advice", err)
	}
}

func TestPlanFrontmatterLists(t *testing.T) {
	for _, tc := range []struct {
		name, source, operation, selector, value, want string
		matched, changed                               int
	}{
		{"append block", "---\r\nitems:\r\n  - one\r\n  - \"[[Old]]\" # keep\r\nnext: value\r\n---", "INSERT", "items", "[[New]]", "---\r\nitems:\r\n  - one\r\n  - \"[[Old]]\" # keep\r\n  - \"[[New]]\"\r\nnext: value\r\n---", 1, 1},
		{"append duplicate", "---\nitems:\n  - one\n---", "INSERT", "items", "one", "---\nitems:\n  - one\n---", 1, 0},
		{"append inline", "---\nitems: [one, two] # keep\n---", "INSERT", "items", "three", "---\nitems: [one, two, three] # keep\n---", 1, 1},
		{"append empty", "---\nitems: [] # keep\n---", "INSERT", "items", "first", "---\nitems: [first] # keep\n---", 1, 1},
		{"update block", "---\nitems:\n  - one\n  - 'old' # keep\n---", "UPDATE", "key = 'items' AND value = 'old'", "new", "---\nitems:\n  - one\n  - 'new' # keep\n---", 1, 1},
		{"update inline", "---\nitems: [one, \"[[Old]]\", three] # keep\n---", "UPDATE", "key = 'items' AND value = '[[Old]]'", "[[New]]", "---\nitems: [one, \"[[New]]\", three] # keep\n---", 1, 1},
		{"update all duplicates", "---\nitems: [old, old]\n---", "UPDATE", "key = 'items' AND value = 'old'", "new", "---\nitems: [new, new]\n---", 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after, matched, changed, err := planFrontmatter([]byte(tc.source), fmTestStatement(t, tc.operation, query.NewString(tc.value), tc.selector), fmTestDocument(t, tc.source))
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != tc.want || matched != tc.matched || changed != tc.changed {
				t.Fatalf("got %q (%d,%d), want %q (%d,%d)", after, matched, changed, tc.want, tc.matched, tc.changed)
			}
			if doc := fmTestDocument(t, string(after)); len(doc.Warnings) != 0 {
				t.Fatalf("edited document has parser warnings: %v", doc.Warnings)
			}
		})
	}
}

func TestPlanFrontmatterScalarTypes(t *testing.T) {
	values := []query.Value{
		query.NewNull(), query.NewBool(true), query.NewBool(false), query.NewInt(math.MinInt64), query.NewInt(math.MaxInt64),
		query.NewFloat(1.0), query.NewFloat(1.25), query.NewFloat(math.MaxFloat64), query.NewFloat(math.SmallestNonzeroFloat64),
		query.NewString("\u0085\u0080\u007f\ufffe\uffff"), query.NewString("true"), query.NewString("null"), query.NewString("123"), query.NewString("a\nb\r\t\x00\"'\\ # :\u2028"),
	}
	for _, value := range values {
		t.Run(value.Kind.String()+"/"+value.String(), func(t *testing.T) {
			after, _, _, err := planFrontmatter(nil, fmTestStatement(t, "INSERT", value, "literal.key"), fmTestDocument(t, ""))
			if err != nil {
				t.Fatal(err)
			}
			doc := fmTestDocument(t, string(after))
			stmt := fmTestStatement(t, "UPDATE", value, "key = 'literal.key'")
			rows, err := selectWriteRows(stmt, doc)
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows=%v err=%v", rows, err)
			}
			if !reflect.DeepEqual(rows[0]["value"], value) {
				t.Fatalf("got %#v, want %#v", rows[0]["value"], value)
			}
		})
	}
}

func TestPlanFrontmatterScalarStyle(t *testing.T) {
	for _, tc := range []struct {
		name, source, operation, selector, value, want string
	}{
		{"insert plain", "---\n---", "INSERT", "stage", "plan", "---\nstage: plan\n---"},
		{"update plain", "---\nstage: plan\n---", "UPDATE", "key = 'stage'", "done", "---\nstage: done\n---"},
		{"update single", "---\nq: 'single'\n---", "UPDATE", "key = 'q'", "it's", "---\nq: 'it''s'\n---"},
		{"update double wikilink", "---\nproject: \"[[Old]]\"\n---", "UPDATE", "key = 'project'", "[[New]]", "---\nproject: \"[[New]]\"\n---"},
		{"colon space", "---\n---", "INSERT", "value", "a: b", "---\nvalue: \"a: b\"\n---"},
		{"leading hash", "---\n---", "INSERT", "value", "#tag", "---\nvalue: \"#tag\"\n---"},
		{"leading bracket", "---\n---", "INSERT", "value", "[x]", "---\nvalue: \"[x]\"\n---"},
		{"empty", "---\n---", "INSERT", "value", "", "---\nvalue: \"\"\n---"},
		{"date string", "---\n---", "INSERT", "value", "2026-09-05", "---\nvalue: \"2026-09-05\"\n---"},
		{"number string", "---\n---", "INSERT", "value", "42", "---\nvalue: \"42\"\n---"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after, _, _, err := planFrontmatter([]byte(tc.source), fmTestStatement(t, tc.operation, query.NewString(tc.value), tc.selector), fmTestDocument(t, tc.source))
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != tc.want {
				t.Fatalf("got %q, want %q", after, tc.want)
			}
			if doc := fmTestDocument(t, string(after)); len(doc.Warnings) != 0 {
				t.Fatalf("edited document has parser warnings: %v", doc.Warnings)
			}
		})
	}

	for _, key := range []string{"", "true", "a: b", "#comment", "[key]"} {
		_, _, _, err := planFrontmatter([]byte("---\n---"), fmTestStatement(t, "INSERT", query.NewString("value"), key), fmTestDocument(t, "---\n---"))
		if err == nil || !strings.Contains(err.Error(), "plain YAML key") {
			t.Errorf("key %q: got %v", key, err)
		}
	}
}

func TestPlanFrontmatterRejectsInvalidStatements(t *testing.T) {
	source := "---\nk: old\n---"
	doc := fmTestDocument(t, source)
	for _, field := range []string{"line", "section", "section_path", "heading_line", "parent_line", "unknown", "key"} {
		stmt := fmTestStatement(t, "UPDATE", query.NewString("new"), "key = 'k'")
		stmt.Values[field] = query.NewString("x")
		_, _, _, err := planFrontmatter([]byte(source), stmt, doc)
		if err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("field %s: %v", field, err)
		}
		if field != "key" {
			stmt.Operation = "INSERT"
			stmt.Where = nil
			stmt.Values["key"] = query.NewString("new")
			if _, _, _, err := planFrontmatter([]byte(source), stmt, doc); err == nil {
				t.Errorf("INSERT accepted %s", field)
			}
		}
	}
	for _, value := range []query.Value{query.NewList(nil), query.NewObject(nil), query.NewFloat(math.Inf(1)), query.NewFloat(math.NaN()), {Kind: query.ValueInt, Data: uint64(math.MaxUint64)}} {
		if _, _, _, err := planFrontmatter([]byte(source), fmTestStatement(t, "INSERT", value, "new"), doc); err == nil {
			t.Errorf("accepted %#v", value)
		}
	}
	for _, stmt := range []*query.WriteStatement{
		nil,
		fmTestStatement(t, "UPDATE", query.NewString("new"), ""),
		fmTestStatement(t, "INSERT", query.NewString("new"), "k"),
		{Operation: "INSERT", Table: "frontmatter_values", Values: map[string]query.Value{"key": query.NewString("new")}},
		{Operation: "INSERT", Table: "frontmatter_values", Values: map[string]query.Value{"value": query.NewNull()}},
	} {
		if _, _, _, err := planFrontmatter([]byte(source), stmt, doc); err == nil {
			t.Errorf("accepted %#v", stmt)
		}
	}
}
