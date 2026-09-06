package edit

import (
	"testing"
)

func TestTaskSectionInsertContinuesLastList(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		sql  string
		want string
	}{
		{
			name: "W3 byte shape",
			raw:  "## Приёмка\n\n- [ ] first\n- [x] second\n\n## Лог\nunchanged\n",
			sql:  "INSERT INTO tasks (section, text) VALUES ('Приёмка', 'benchmark')",
			want: "## Приёмка\n\n- [ ] first\n- [x] second\n- [ ] benchmark\n\n## Лог\nunchanged\n",
		},
		{
			name: "before trailing paragraph",
			raw:  "# Tasks\n\n* [ ] first\nparagraph\n",
			sql:  "INSERT INTO tasks (section, text) VALUES ('Tasks', 'second')",
			want: "# Tasks\n\n* [ ] first\n* [ ] second\nparagraph\n",
		},
		{
			name: "after final plain item in task list",
			raw:  "# Tasks\n- [ ] first\n- supporting note\nparagraph",
			sql:  "INSERT INTO tasks (section, text) VALUES ('Tasks', 'second')",
			want: "# Tasks\n- [ ] first\n- supporting note\n- [ ] second\nparagraph",
		},
		{
			name: "nested marker and continuation",
			raw:  "# Tasks\n- [ ] parent\n  + [ ] child\n    child continuation\nparagraph",
			sql:  "INSERT INTO tasks (section, text) VALUES ('Tasks', 'sibling')",
			want: "# Tasks\n- [ ] parent\n  + [ ] child\n    child continuation\n  + [ ] sibling\nparagraph",
		},
		{
			name: "ordered marker and spacing",
			raw:  "# Tasks\r\n1.  [ ] first\r\n2.  [ ] second\r\n\r\ntext\r\n",
			sql:  "INSERT INTO tasks (section, text) VALUES ('Tasks', 'third')",
			want: "# Tasks\r\n1.  [ ] first\r\n2.  [ ] second\r\n2.  [ ] third\r\n\r\ntext\r\n",
		},
		{
			name: "quoted task is not continued",
			raw:  "# Tasks\n> - [ ] quoted\n",
			sql:  "INSERT INTO tasks (section, text) VALUES ('Tasks', 'plain')",
			want: "# Tasks\n> - [ ] quoted\n\n- [ ] plain\n",
		},
		{
			name: "tab-indented list falls back safely",
			raw:  "# Tasks\n\t- [ ] tabbed\n",
			sql:  "INSERT INTO tasks (section, text) VALUES ('Tasks', 'plain')",
			want: "# Tasks\n\t- [ ] tabbed\n\n- [ ] plain\n",
		},
		{
			name: "no task list falls back to section end",
			raw:  "# Tasks\nbody\n\n# Next",
			sql:  "INSERT INTO tasks (section, text) VALUES ('Tasks', 'first')",
			want: "# Tasks\nbody\n\n- [ ] first\n\n# Next",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, matched, changed := corePlan(t, tc.raw, tc.sql)
			if got != tc.want || matched != 1 || changed != 1 {
				t.Fatalf("got %q (%d,%d), want %q", got, matched, changed, tc.want)
			}
		})
	}
}

func TestTaskSectionEndPositionStartsSeparateList(t *testing.T) {
	raw := "# Tasks\n- [ ] first\n\ntrailing"
	after, matched, changed := corePlan(t, raw, "INSERT INTO tasks (section, text, position) VALUES ('Tasks', 'separate', 'section_end')")
	want := "# Tasks\n- [ ] first\n\ntrailing\n\n- [ ] separate"
	if after != want || matched != 1 || changed != 1 {
		t.Fatalf("got %q (%d,%d); want %q", after, matched, changed, want)
	}
}
