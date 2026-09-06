package query

import (
	"reflect"
	"testing"
)

func TestTextLineEdgesAndExactMode(t *testing.T) {
	for _, tc := range []struct {
		text, expression string
		want             Value
	}{
		{"\n \t\n first \n\nlast\n \t\n", "lines(s)", NewInt(3)},
		{"\n \t\n first \n\nlast\n \t\n", "head(s, 2)", NewString(" first \n")},
		{"\n \t\n first \n\nlast\n \t\n", "tail(s, 2)", NewString("\nlast")},
		{"\nfirst\nlast\n\n", "lines(s, true)", NewInt(5)},
		{"\nfirst\nlast\n\n", "head(s, 2, true)", NewString("\nfirst")},
		{"\nfirst\nlast\n\n", "tail(s, 2, true)", NewString("\n")},
		{"\nfirst\nlast\n\n", "tail(s, 2, false)", NewString("first\nlast")},
		{"\nfirst\nlast\n\n", "head(s, 2, false)", NewString("first\nlast")},
		{"\nfirst\nlast\n\n", "lines(s, false)", NewInt(2)},
		{"", "lines(s)", NewInt(0)}, {"", "lines(s, true)", NewInt(1)},
		{"\n\t\n ", "lines(s)", NewInt(0)}, {"\n\t\n ", "tail(s, 3)", NewString("")},
		{"\n\t\n ", "head(s, 3)", NewString("")},
		{"first\nlast\n", "head(s, 0)", NewString("")},
		{"first\nlast\n", "tail(s, -1)", NewString("")},
		{"first\nlast\n", "tail(s, 9223372036854775807)", NewString("first\nlast")},
		{"x", "head(s, 1, NULL)", NewNull()}, {"x", "tail(s, NULL)", NewNull()},
		{"x", "lines(s, NULL)", NewNull()}, {"x", "lines(NULL)", NewNull()},
	} {
		q, err := Parse("SELECT " + tc.expression + " FROM sections")
		if err != nil {
			t.Fatal(err)
		}
		value, err := Eval(q.Select[0].Expr, Row{"s": NewString(tc.text)}, nil)
		if err != nil || !reflect.DeepEqual(value, tc.want) {
			t.Fatalf("%s(%q): value=%v want=%v error=%v", tc.expression, tc.text, value, tc.want, err)
		}
	}
}
