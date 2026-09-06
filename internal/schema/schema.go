package schema

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Valiev/mdsql/internal/query"
	"github.com/Valiev/mdsql/internal/tables"
)

type Schema struct {
	Tables       map[string][]Column    `json:"tables"`
	Funcs        []string               `json:"functions"`
	FunctionDocs map[string]string      `json:"function_docs,omitempty"`
	Operators    []string               `json:"operators"`
	Examples     []string               `json:"examples"`
	WriteTargets map[string]WriteTarget `json:"write_targets,omitempty"`
}

// WriteTarget describes writable inputs separately from projected read columns.
type WriteTarget struct {
	Insert []Column `json:"insert,omitempty"`
	Update []Column `json:"update,omitempty"`
}

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Doc  string `json:"doc"`
}

func JSON(w io.Writer, tablesOnly bool, examplesOnly bool) error {
	full := collect()
	if tablesOnly {
		full.Funcs = nil
		full.FunctionDocs = nil
		full.Operators = nil
		full.Examples = nil
	}
	if examplesOnly {
		full.WriteTargets = nil
		full.Tables = nil
		full.Funcs = nil
		full.FunctionDocs = nil
		full.Operators = nil
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(full)
}

func Text(w io.Writer, tablesOnly bool, examplesOnly bool) {
	s := collect()
	fmt.Fprintln(w, "mdsql schema")
	fmt.Fprintln(w, "")
	if !examplesOnly {
		tableNames := make([]string, 0, len(s.Tables))
		for name := range s.Tables {
			tableNames = append(tableNames, name)
		}
		sort.Strings(tableNames)
		for _, name := range tableNames {
			cols := s.Tables[name]
			fmt.Fprintf(w, "%s\n", name)
			for _, c := range cols {
				fmt.Fprintf(w, "%s:%s %s\n", c.Name, c.Type, c.Doc)
			}
		}
	}
	if !tablesOnly && !examplesOnly {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Функции:")
		for _, f := range s.Funcs {
			fmt.Fprintf(w, "  - %s\n", f)
		}
		fmt.Fprintf(w, "has_tag: %s\n", s.FunctionDocs["has_tag"])
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Операторы:")
		for _, f := range s.Operators {
			fmt.Fprintf(w, "  - %s\n", f)
		}
	}
	if !examplesOnly {
		fmt.Fprintln(w, "\nWrite targets (one file; writes immediately; --dry-run previews):")
		for _, name := range []string{"lines", "tasks", "sections", "code", "frontmatter_values"} {
			target := s.WriteTargets[name]
			for _, operation := range []struct {
				name    string
				columns []Column
			}{{"INSERT", target.Insert}, {"UPDATE", target.Update}} {
				if len(operation.columns) == 0 {
					continue
				}
				fields := make([]string, 0, len(operation.columns))
				for _, column := range operation.columns {
					if column.Name == "line" || column.Name == "section" || column.Name == "section_path" || column.Name == "heading_line" {
						continue
					}
					fields = append(fields, column.Name+":"+column.Type)
				}
				fmt.Fprintf(w, "%s %s (%s)\n", operation.name, name, strings.Join(fields, ", "))
			}
		}
		fmt.Fprintln(w, "Body INSERT position: line:int, section:string, section_path:string, heading_line:int; mutually exclusive, including parent_line. Omitted: EOF. line=N inserts before N.")
		fmt.Fprintln(w, "UPDATE requires WHERE. state/status are alternatives. frontmatter_values has no position; scalar values only. Full field docs: schema --json.")
	}
	if !tablesOnly {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Примеры:")
		for _, e := range s.Examples {
			fmt.Fprintf(w, "  - %s\n", e)
		}
	}
}

func collect() Schema {
	result := Schema{Tables: map[string][]Column{}, Funcs: functions(), Operators: operators(), Examples: examples(), FunctionDocs: map[string]string{
		"has_tag": "Case-insensitive hierarchical membership: tasks use task tags; headings/lines/blocks use inline tags in their source range; files/frontmatter use YAML tags. Fenced code is excluded.",
	}}
	result.WriteTargets = writeTargets()
	for _, t := range tables.Registry {
		cols := make([]Column, 0, len(t.Columns))
		for _, c := range t.Columns {
			cols = append(cols, Column{Name: c.Name, Type: c.Type, Doc: c.Doc})
		}
		result.Tables[t.Name] = cols
	}
	return result
}

func functions() []string {
	return query.FunctionSignatures()
}

func operators() []string {
	return []string{"+", "-", "*", "/", "%", "=", "!=", "<", "<=", ">", ">=", "LIKE", "NOT LIKE", "~", "!~", "~*", "!~*", "IN", "BETWEEN", "HAS", "IS", "IS NOT"}
}

func examples() []string {
	return []string{
		"SELECT level, title, line FROM headings IN docs/**/*.md",
		"SELECT text, due, priority FROM tasks WHERE state = 'todo' AND due <= today + 7d",
		"SELECT lang, start, code FROM code WHERE lang IN ('go', 'python')",
		"SELECT type, target, text FROM links WHERE external = true",
		"SELECT kind, text FROM blocks WHERE text ~* 'deadline'",
		"SELECT sum(size), min(size), max(size), avg(size) FROM files",
		"SELECT state, count(due), min(due) FROM tasks GROUP BY state",
	}
}

func TextAsString() string {
	parts := []string{"mdsql schema"}
	for name, cols := range collect().Tables {
		names := make([]string, len(cols))
		for i, c := range cols {
			names[i] = c.Name
		}
		parts = append(parts, "TABLE "+name+": "+strings.Join(names, ", "))
	}
	return strings.Join(parts, "\n")
}
