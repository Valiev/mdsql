package query

import "strings"

func evalTextLines(name string, values []Value) Value {
	for _, value := range values {
		if value.IsNull() {
			return NewNull()
		}
	}
	required := 2
	if name == "lines" {
		required = 1
	}
	exact := len(values) > required && values[required].Data.(bool)
	lines := strings.Split(values[0].String(), "\n")
	if !exact {
		for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
			lines = lines[1:]
		}
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
	}
	if name == "lines" {
		return NewInt(int64(len(lines)))
	}
	n := values[1].Data.(int64)
	if n < 0 {
		n = 0
	}
	if n > int64(len(lines)) {
		n = int64(len(lines))
	}
	if name == "head" {
		return NewString(strings.Join(lines[:n], "\n"))
	}
	return NewString(strings.Join(lines[int64(len(lines))-n:], "\n"))
}
