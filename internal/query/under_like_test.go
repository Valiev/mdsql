package query

import "testing"

func TestUnderLikePatternsAndSubstringFunction(t *testing.T) {
	for _, tc := range []struct {
		function string
		pattern  string
		section  string
		path     string
		want     bool
	}{
		{"under_like", "%proj%", "Other", "Root > Project > Child", true},
		{"under_like", "Proj_ct", "Project", "Other", true},
		{"under_like", "Project", "Project", "Root > Project", true},
		{"under_like", "proj", "Project", "Root > Project", false},
		{"under_like", "Root > %", "Child", "Root > Project", true},
		{"under_like", `100\%`, "100%", "", true},
		{"under_like", `100\%`, "100x", "", false},
		{"under_like", `Proj\_ct`, "Proj_ct", "", true},
		{"under_like", "П_ОЕКТ", "Проект", "", true},
		{"under_like", "%", "", "", true},
		{"under_contains", "proj", "Project", "Root > Project", true},
		{"under_contains", "%proj%", "Project", "Root > Project", false},
		{"under_contains", "%proj%", "literal %Proj% text", "", true},
		{"under_contains", "Proj_ct", "Project", "", false},
	} {
		t.Run(tc.function+":"+tc.pattern+":"+tc.section, func(t *testing.T) {
			q, err := Parse("SELECT title FROM headings WHERE " + tc.function + "('" + tc.pattern + "')")
			if err != nil {
				t.Fatal(err)
			}
			value, err := Eval(q.Where, Row{"section": NewString(tc.section), "path": NewString(tc.path)}, nil)
			if err != nil || value.Kind != ValueBool || truthy(value) != tc.want {
				t.Fatalf("value=%v want=%v error=%v", value, tc.want, err)
			}
		})
	}
}
