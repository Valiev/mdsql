package query

import "testing"

func TestMatchLikeRegression(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		pattern    string
		ignoreCase bool
		want       bool
	}{
		{name: "empty", value: "", pattern: "", want: true},
		{name: "multiline", value: "categories:\n  - [[Agent Tasks]]\nstage: implement", pattern: "%Agent Tasks%", ignoreCase: true, want: true},
		{name: "underscore newline", value: "a\nb", pattern: "a_b", want: true},
		{name: "underscore unicode", value: "猫", pattern: "_", want: true},
		{name: "literal percent", value: "100%", pattern: `100\%`, want: true},
		{name: "escaped percent not wildcard", value: "100abc", pattern: `100\%`, want: false},
		{name: "literal underscore", value: "a_b", pattern: `a\_b`, want: true},
		{name: "escaped underscore not wildcard", value: "axb", pattern: `a\_b`, want: false},
		{name: "literal backslash", value: `path\file`, pattern: `path\\file`, want: true},
		{name: "trailing backslash", value: `path\`, pattern: `path\`, want: true},
		{name: "unknown escape preserved", value: `a\qb`, pattern: `a\qb`, want: true},
		{name: "regex metacharacters", value: "[a.b]+(x)?", pattern: "[a.b]+(x)?", want: true},
		{name: "anchored", value: "before Agent Tasks after", pattern: "Agent Tasks", want: false},
		{name: "case sensitive", value: "Agent Tasks", pattern: "%tasks%", want: false},
		{name: "ignore case", value: "Agent Tasks", pattern: "%tasks%", ignoreCase: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchLike(tt.value, tt.pattern, tt.ignoreCase); got != tt.want {
				t.Fatalf("matchLike(%q, %q, %v) = %v, want %v", tt.value, tt.pattern, tt.ignoreCase, got, tt.want)
			}
		})
	}
}

func TestSQLLikeRegression(t *testing.T) {
	tests := []struct {
		name      string
		predicate string
		raw       string
		want      bool
	}{
		{name: "multiline LIKE", predicate: "raw LIKE '%Agent Tasks%'", raw: "categories:\n  - [[Agent Tasks]]\nstage: implement", want: true},
		{name: "multiline NOT LIKE", predicate: "raw NOT LIKE '%Agent Tasks%'", raw: "categories:\n  - [[Agent Tasks]]\nstage: implement", want: false},
		{name: "NOT LIKE nonmatch", predicate: "raw NOT LIKE '%Agent Tasks%'", raw: "categories:\n  - [[Projects]]\n", want: true},
		{name: "escaped percent", predicate: `raw LIKE '100\%'`, raw: "100%", want: true},
		{name: "escaped percent nonmatch", predicate: `raw LIKE '100\%'`, raw: "100abc", want: false},
		{name: "escaped underscore", predicate: `raw LIKE 'a\_b'`, raw: "a_b", want: true},
		{name: "escaped underscore nonmatch", predicate: `raw LIKE 'a\_b'`, raw: "axb", want: false},
		{name: "escaped backslash", predicate: `raw LIKE 'path\\file'`, raw: `path\file`, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse("SELECT file FROM frontmatter WHERE " + tt.predicate)
			if err != nil {
				t.Fatal(err)
			}
			got, err := q.Where.eval(Row{"raw": NewString(tt.raw)}, &EvalContext{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != ValueBool || truthy(got) != tt.want {
				t.Fatalf("predicate result = %#v, want boolean %v", got, tt.want)
			}
		})
	}
}

func TestSQLContainsMembershipRegression(t *testing.T) {
	list := func(items ...Value) Value {
		return Value{Kind: ValueList, Data: items}
	}
	tests := []struct {
		name    string
		input   Value
		needle  string
		missing bool
		want    bool
	}{
		{name: "exact member", input: list(NewString("[[Agent Tasks]]")), needle: "'[[Agent Tasks]]'", want: true},
		{name: "later member", input: list(NewString("[[Projects]]"), NewString("[[Agent Tasks]]")), needle: "'[[Agent Tasks]]'", want: true},
		{name: "no member", input: list(NewString("[[Projects]]")), needle: "'[[Agent Tasks]]'", want: false},
		{name: "no substring membership", input: list(NewString("[[Agent Tasks]]")), needle: "'Agent Tasks'", want: false},
		{name: "case sensitive membership", input: list(NewString("[[Agent Tasks]]")), needle: "'[[agent tasks]]'", want: false},
		{name: "empty list", input: list(), needle: "'[[Agent Tasks]]'", want: false},
		{name: "integer member", input: list(NewInt(7)), needle: "7", want: true},
		{name: "no type coercion", input: list(NewInt(7)), needle: "'7'", want: false},
		{name: "boolean member", input: list(NewBool(true)), needle: "true", want: true},
		{name: "null member", input: list(NewNull()), needle: "NULL", want: true},
		{name: "mixed list", input: list(NewNull(), NewInt(7), NewString("[[Agent Tasks]]")), needle: "'[[Agent Tasks]]'", want: true},
		{name: "duplicate members", input: list(NewString("[[Agent Tasks]]"), NewString("[[Agent Tasks]]")), needle: "'[[Agent Tasks]]'", want: true},
		{name: "string substring unchanged", input: NewString("before [[Agent Tasks]] after"), needle: "'[[Agent Tasks]]'", want: true},
		{name: "string case sensitivity unchanged", input: NewString("Agent Tasks"), needle: "'agent tasks'", want: false},
		{name: "missing field filtered out", needle: "'[[Agent Tasks]]'", missing: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := map[string]Value{}
			if !tt.missing {
				fields["categories"] = tt.input
			}
			row := Row{"fm": Value{Kind: ValueObject, Data: fields}}
			q, err := Parse("SELECT file FROM frontmatter WHERE contains(fm.categories, " + tt.needle + ")")
			if err != nil {
				t.Fatal(err)
			}
			got, err := q.Where.eval(row, &EvalContext{})
			if err != nil {
				t.Fatal(err)
			}
			if truthy(got) != tt.want {
				t.Fatalf("contains result = %#v, want truthiness %v", got, tt.want)
			}
			if !tt.missing && got.Kind != ValueBool {
				t.Fatalf("contains result kind = %v, want boolean", got.Kind)
			}
		})
	}
}
