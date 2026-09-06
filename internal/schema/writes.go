package schema

func writeTargets() map[string]WriteTarget {
	positions := []Column{
		{Name: "line", Type: "int", Doc: "Insert before physical line N; line count + 1 appends."},
		{Name: "section", Type: "string", Doc: "Append to the own text of one exact heading title."},
		{Name: "section_path", Type: "string", Doc: "Append to one exact heading path; duplicates are errors."},
		{Name: "heading_line", Type: "int", Doc: "Select a section by its heading's first physical line."},
	}
	withPositions := func(columns ...Column) []Column {
		return append(columns, positions...)
	}
	state := Column{Name: "state", Type: "string", Doc: "todo, done, in_progress or cancelled; exclusive with status."}
	status := Column{Name: "status", Type: "string", Doc: "Checkbox character: space, x, X, / or -; exclusive with state."}
	text := Column{Name: "text", Type: "string", Doc: "One-line task content without list marker or checkbox."}
	due := Column{Name: "due", Type: "string|null", Doc: "ISO date serialized as 📅 YYYY-MM-DD; NULL clears it."}
	scheduled := Column{Name: "scheduled", Type: "string|null", Doc: "ISO date serialized as ⏳ YYYY-MM-DD; NULL clears it."}
	priority := Column{Name: "priority", Type: "int|null", Doc: "Rank 1..5 serialized as an Obsidian Tasks emoji; NULL clears it."}
	tags := Column{Name: "tags", Type: "string|null", Doc: "Comma/space-separated tags serialized with #; NULL clears them."}
	value := Column{Name: "value", Type: "scalar", Doc: "String, number, boolean or NULL; NULL means YAML null, not deletion."}
	return map[string]WriteTarget{
		"lines": {Insert: withPositions(Column{Name: "text", Type: "string", Doc: "Required nonempty text; may span multiple lines."})},
		"tasks": {
			Insert: withPositions(text, state, status, due, scheduled, priority, tags, Column{Name: "parent_line", Type: "int", Doc: "Append the last child of a task; exclusive with other positions."}, Column{Name: "position", Type: "string", Doc: "With a section selector, section_end creates a separate list at the section end."}),
			Update: []Column{state, status, text, due, scheduled, priority, tags},
		},
		"sections": {Insert: withPositions(
			Column{Name: "level", Type: "int", Doc: "Required heading level from 1 to 6."},
			Column{Name: "title", Type: "string", Doc: "Required nonempty single-line heading title."},
			Column{Name: "body", Type: "string", Doc: "Optional Markdown content following the heading."},
		)},
		"code": {Insert: withPositions(
			Column{Name: "lang", Type: "string", Doc: "Optional code language."},
			Column{Name: "code", Type: "string", Doc: "Required code content; fences are generated."},
		)},
		"frontmatter_values": {
			Insert: []Column{{Name: "key", Type: "string", Doc: "Required absent top-level YAML key, or existing list key for append."}, value},
			Update: []Column{value},
		},
	}
}
