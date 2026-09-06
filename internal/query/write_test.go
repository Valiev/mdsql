package query

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestParseWriteInsertLiterals(t *testing.T) {
	for _, tc := range []struct {
		sql   string
		table string
		want  map[string]Value
	}{
		{"iNsErT InTo TaSkS (TeXt) VALUES ('task');", "tasks", map[string]Value{"text": NewString("task")}},
		{"INSERT INTO lines (text, line) VALUES ('first\nsecond', 2)", "lines", map[string]Value{"text": NewString("first\nsecond"), "line": NewInt(2)}},
		{`INSERT INTO lines (text) VALUES ('it''s \'quoted\' \\ \n \t \r \z; -- /*')`, "lines", map[string]Value{"text": NewString("it's 'quoted' \\ \n \t \r \\z; -- /*")}},
		{`INSERT INTO lines (text) VALUES ("a""b\"c")`, "lines", map[string]Value{"text": NewString("a\"b\"c")}},
		{"/* leading */ -- second\n INSERT/*middle*/INTO code(lang,code) VALUES ('go','fmt.Println(1);');/*end*/", "code", map[string]Value{"lang": NewString("go"), "code": NewString("fmt.Println(1);")}},
		{"INSERT INTO sections(level,title,body,section_path) VALUES (+2,'Child','body','Root')", "sections", map[string]Value{"level": NewInt(2), "title": NewString("Child"), "body": NewString("body"), "section_path": NewString("Root")}},
		{"INSERT INTO tasks(text,status,parent_line) VALUES ('child','X',4)", "tasks", map[string]Value{"text": NewString("child"), "status": NewString("X"), "parent_line": NewInt(4)}},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			stmt, err := ParseWrite(tc.sql)
			if err != nil {
				t.Fatal(err)
			}
			if stmt.Operation != "INSERT" || stmt.Table != tc.table || stmt.Where != nil || !reflect.DeepEqual(stmt.Values, tc.want) {
				t.Fatalf("statement=%#v, want table=%s values=%#v", stmt, tc.table, tc.want)
			}
		})
	}
}

func TestParseWriteFrontmatterScalars(t *testing.T) {
	for _, tc := range []struct {
		literal string
		value   Value
	}{
		{"NULL", NewNull()}, {"true", NewBool(true)}, {"FALSE", NewBool(false)},
		{"'null'", NewString("null")}, {"0", NewInt(0)},
		{"-9223372036854775808", NewInt(math.MinInt64)}, {"9223372036854775807", NewInt(math.MaxInt64)},
		{"-1.25", NewFloat(-1.25)}, {"1e3", NewFloat(1000)}, {"+.5", NewFloat(0.5)},
		{"1.", NewFloat(1)}, {"1.2e-3", NewFloat(0.0012)},
	} {
		for _, sql := range []string{
			"INSERT INTO frontmatter_values(key,value) VALUES ('answer'," + tc.literal + ")",
			"UPDATE frontmatter_values SET value = " + tc.literal + " WHERE key = 'answer';",
		} {
			stmt, err := ParseWrite(sql)
			if err != nil || !reflect.DeepEqual(stmt.Values["value"], tc.value) {
				t.Fatalf("SQL=%s statement=%#v error=%v want=%#v", sql, stmt, err, tc.value)
			}
		}
	}
}

func TestParseWriteUpdateWhere(t *testing.T) {
	stmt, err := ParseWrite("-- lead\nUPDATE TaSkS SET STATUS = 'X' WHERE (text = 'WHERE it''s; SET' OR line IN (1, 2)) AND state != 'done'; -- end")
	if err != nil {
		t.Fatal(err)
	}
	if stmt.Operation != "UPDATE" || stmt.Table != "tasks" || stmt.Values["status"].String() != "X" || stmt.Where == nil {
		t.Fatalf("statement=%#v", stmt)
	}
	for _, tc := range []struct {
		row  Row
		want bool
	}{
		{Row{"text": NewString("WHERE it's; SET"), "line": NewInt(4), "state": NewString("todo")}, true},
		{Row{"text": NewString("other"), "line": NewInt(2), "state": NewString("todo")}, true},
		{Row{"text": NewString("other"), "line": NewInt(3), "state": NewString("todo")}, false},
		{Row{"text": NewString("other"), "line": NewInt(2), "state": NewString("done")}, false},
	} {
		got, err := Eval(stmt.Where, tc.row, &EvalContext{})
		if err != nil || got.Kind != ValueBool || got.Data != tc.want {
			t.Fatalf("row=%#v value=%#v error=%v want=%v", tc.row, got, err, tc.want)
		}
	}
	for _, where := range []string{"line BETWEEN 1 AND 3", "text IS NOT NULL", "due < 2026-09-05", "under('Root') AND has_tag('work')"} {
		if _, err := ParseWrite("UPDATE tasks SET state='done' WHERE " + where); err != nil {
			t.Fatalf("WHERE %s: %v", where, err)
		}
	}
}

func TestParseWriteRejectsUnsafeOrUnsupportedSQL(t *testing.T) {
	for _, sql := range []string{
		"", "SELECT * FROM tasks", "DELETE FROM tasks", "INSERT tasks(text) VALUES ('x')",
		"INSERT INTO tasks VALUES ('x')", "INSERT INTO tasks() VALUES ()",
		"INSERT INTO tasks(text, TEXT) VALUES ('a','b')", "INSERT INTO tasks(text) VALUES ()",
		"INSERT INTO tasks(text) VALUES ('a','b')", "INSERT INTO tasks(text,state) VALUES ('a')",
		"INSERT INTO tasks(text) VALUES ('a'),('b')", "INSERT INTO tasks(text) SELECT text FROM tasks",
		"INSERT INTO tasks(text) VALUES ('a'); UPDATE tasks SET state='done' WHERE line=1",
		"INSERT INTO tasks(text) VALUES ('a');;", "INSERT INTO tasks(text) VALUES ('a') trailing",
		"INSERT INTO lines(text) VALUES ('unclosed)", "/* unclosed INSERT INTO lines(text) VALUES ('x')",
		"INSERT INTO lines(text) VALUES ('x') /* unclosed", "INSERT INTO tasks(text) VALUES (NULL)",
		"INSERT INTO lines(text) VALUES (1)", "INSERT INTO tasks(text) VALUES (lower('x'))",
		"INSERT INTO tasks(text) VALUES ('a' + 'b')", "INSERT INTO tasks(text) VALUES (text)",
		"INSERT INTO tasks(text,state,status) VALUES ('a','done','x')",
		"INSERT INTO tasks(text,state) VALUES ('a','unknown')", "INSERT INTO tasks(text,status) VALUES ('a','!')",
		"INSERT INTO lines(text,line) VALUES ('a',0)", "INSERT INTO lines(text,line) VALUES ('a',1.5)",
		"INSERT INTO lines(text,line,section) VALUES ('a',1,'Root')",
		"INSERT INTO code(code,parent_line) VALUES ('a',1)", "INSERT INTO sections(level,title) VALUES (7,'a')",
		"INSERT INTO sections(level,title) VALUES (0,'a')", "INSERT INTO tasks(state) VALUES ('todo')",
		"INSERT INTO unknown(text) VALUES ('a')", "INSERT INTO frontmatter_values(key) VALUES ('a')",
		"INSERT INTO frontmatter_values(key,value) VALUES ('a',2026-09-05)",
		"INSERT INTO frontmatter_values(key,value) VALUES ('a',1e999)",
		"INSERT INTO frontmatter_values(key,value) VALUES ('a',1e)",
		"INSERT INTO frontmatter_values(key,value) VALUES ('a',9223372036854775808)",
		"INSERT INTO frontmatter_values(key,value) VALUES ('a',-9223372036854775809)",
		"INSERT INTO frontmatter_values(key,value) VALUES ('a',--1\n)",
		"INSERT INTO frontmatter_values(key,value) VALUES ('a',-'text')",
		"UPDATE tasks SET state='done'", "UPDATE tasks SET state='done' WHERE",
		"UPDATE tasks SET state='done' WHERE line=1 LIMIT 1", "UPDATE tasks SET state='done' WHERE line=1 ORDER BY line",
		"UPDATE tasks SET state='done' WHERE line=1; SELECT * FROM tasks",
		"UPDATE tasks SET state='done', STATE='todo' WHERE line=1",
		"UPDATE tasks SET state=state WHERE line=1", "UPDATE tasks SET state=upper('done') WHERE line=1",
		"UPDATE tasks SET state='done' + 'x' WHERE line=1", "UPDATE tasks SET state=NULL WHERE line=1",
		"UPDATE tasks SET state='done',status='x' WHERE line=1",
		"UPDATE lines SET text='x' WHERE n=1", "UPDATE frontmatter_values SET key='x' WHERE key='y'",
		"UPDATE tasks SET state == 'done' WHERE line=1", "UPDATE tasks SET state='done' WHERE COUNT(*)=1",
	} {
		t.Run(sql, func(t *testing.T) {
			stmt, err := ParseWrite(sql)
			if err == nil || stmt != nil {
				t.Fatalf("accepted SQL: statement=%#v error=%v", stmt, err)
			}
		})
	}
}

func TestWriteErrorsIncludeLocation(t *testing.T) {
	_, err := ParseWrite("UPDATE tasks SET state='done' WHERE")
	if err == nil || !strings.Contains(err.Error(), "byte offset") {
		t.Fatalf("error=%v", err)
	}
}

func TestWriteWhereRejectsSelectTrailingClauses(t *testing.T) {
	for _, tail := range []string{
		"LIMIT 1", "LIMIT 0", "OFFSET 1", "ORDER BY line", "GROUP BY state",
		"WITH max_rows=1", "; UPDATE tasks SET state='todo' WHERE line=2", ";;", "unexpected",
	} {
		stmt, err := ParseWrite("UPDATE tasks SET state='done' WHERE line=1 " + tail)
		if err == nil || stmt != nil {
			t.Errorf("accepted trailing clause %q: statement=%#v error=%v", tail, stmt, err)
		}
	}
}

func TestWriteDecimalWhere(t *testing.T) {
	for _, tc := range []struct {
		where string
		value Value
		want  bool
	}{
		{"value = 1.5", NewFloat(1.5), true},
		{"value = 1.5", NewFloat(2), false},
		{"value = -1.5", NewFloat(-1.5), true},
		{"value = +.5", NewFloat(0.5), true},
		{"value = 1.5e2", NewInt(150), true},
		{"value BETWEEN -1.5 AND 2.5", NewFloat(0.5), true},
		{"value IN (0.5, 1.5, -2.5)", NewFloat(1.5), true},
		{"value NOT IN (0.5, 1.5, -2.5)", NewFloat(-2.5), false},
		{"value + 0.5 = 2", NewFloat(1.5), true},
		{"value IN (1.0 + 0.5, -2.5)", NewFloat(1.5), true},
		{"coalesce(value, 1.5) = 1.5", NewNull(), true},
	} {
		stmt, err := ParseWrite("UPDATE frontmatter_values SET value=2.0 WHERE " + tc.where)
		if err != nil {
			t.Fatalf("WHERE %s: %v", tc.where, err)
		}
		if got := stmt.Values["value"]; got.Kind != ValueFloat || got.Data != float64(2) {
			t.Fatalf("SET value=%#v", got)
		}
		got, err := Eval(stmt.Where, Row{"value": tc.value}, &EvalContext{})
		if err != nil || got.Kind != ValueBool || got.Data != tc.want {
			t.Errorf("WHERE %s: value=%#v result=%#v error=%v want=%v", tc.where, tc.value, got, err, tc.want)
		}
	}
	for _, where := range []string{"value=1e999", "value=-1e999", "value=1.2e", "value IN (1.0, 1e999)", "value BETWEEN 0.0 AND 1e999", "value=1.5 LIMIT 1"} {
		if stmt, err := ParseWrite("UPDATE frontmatter_values SET value=2.0 WHERE " + where); err == nil || stmt != nil {
			t.Errorf("accepted invalid WHERE %s: %#v %v", where, stmt, err)
		}
	}
	if _, err := Parse("SELECT * FROM frontmatter_values WHERE value=1.5"); err == nil {
		t.Fatal("write-only float tokens unexpectedly extended SELECT grammar")
	}
}

func TestWriteWhereBackslashParityWithSelect(t *testing.T) {
	predicates := []string{
		`text = 'a\b'`, `text = 'a\\b'`, `text = 'a\\\\b'`,
		`text = 'a\n'`, `text = 'a\\n'`, `text = 'a\t'`, `text = 'a\\t'`,
		`text = 'a\r'`, `text = 'a\\r'`, `text = 'a\'b'`,
		`text = "a\"b"`, `text = 'a\\'`,
		`text LIKE 'a\\_%'`, `text LIKE 'a\_%'`,
		`text LIKE 'a\\%'`, `text LIKE 'a\%'`,
		`text NOT LIKE 'a\\_%'`, `text IN ('a\\b', 'a\\n')`,
		`contains(text, 'a\\b')`, `text ~ '^a\\\\b$'`,
		`(text = 'a\\b' OR text LIKE 'a\_%') AND text IS NOT NULL`,
		`NOT (text = 'a\\b')`, `len(text) BETWEEN 2 AND 4`,
	}
	values := []string{
		`a\b`, `a\\b`, `a\\\\b`, `a\`, `a\\`, `a\q`, `a\_q`, `a_q`, `a_`,
		`a%`, `a\%`, `a\n`, `a\\n`, "a\n", "a\\\n", "a\t", "a\\\t",
		`a\r`, `a\\r`, "a\r", "a'b", `a"b`, "other",
	}
	for _, predicate := range predicates {
		t.Run(predicate, func(t *testing.T) {
			read, err := Parse("SELECT * FROM tasks WHERE " + predicate)
			if err != nil {
				t.Fatal(err)
			}
			write, err := ParseWrite("UPDATE tasks SET status='x' WHERE " + predicate)
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range values {
				row := Row{"text": NewString(text)}
				want, readErr := Eval(read.Where, row, &EvalContext{})
				got, writeErr := Eval(write.Where, row, &EvalContext{})
				if readErr != nil || writeErr != nil || !reflect.DeepEqual(got, want) {
					t.Errorf("row=%q SELECT=%#v (%v) UPDATE=%#v (%v)", text, want, readErr, got, writeErr)
				}
			}
		})
	}
}
