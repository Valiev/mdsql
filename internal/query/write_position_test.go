package query

import "testing"

func TestParseTaskSectionEndPosition(t *testing.T) {
	for _, selector := range []string{
		"section, text, position) VALUES ('Tasks', 'new', 'section_end')",
		"section_path, text, position) VALUES ('Project > Tasks', 'new', 'section_end')",
		"heading_line, text, position) VALUES (4, 'new', 'section_end')",
	} {
		stmt, err := ParseWrite("INSERT INTO tasks (" + selector)
		if err != nil || stmt.Values["position"].String() != "section_end" {
			t.Fatalf("selector %q: statement=%#v error=%v", selector, stmt, err)
		}
	}
}

func TestRejectInvalidTaskSectionEndPosition(t *testing.T) {
	for _, sql := range []string{
		"INSERT INTO tasks (text, position) VALUES ('new', 'section_end')",
		"INSERT INTO tasks (section, text, position) VALUES ('Tasks', 'new', 'end')",
		"INSERT INTO tasks (section, text, position) VALUES ('Tasks', 'new', 1)",
		"INSERT INTO tasks (line, text, position) VALUES (2, 'new', 'section_end')",
		"INSERT INTO lines (section, text, position) VALUES ('Tasks', 'new', 'section_end')",
	} {
		if stmt, err := ParseWrite(sql); err == nil || stmt != nil {
			t.Errorf("accepted %q: statement=%#v error=%v", sql, stmt, err)
		}
	}
}
