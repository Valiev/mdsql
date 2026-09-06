package cli

import (
	"strings"
	"testing"
)

func TestParseArgsMaxRows(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{"default", nil, 500},
		{"separate", []string{"--max-rows", "12"}, 12},
		{"equals", []string{"--max-rows=12"}, 12},
		{"disabled", []string{"--max-rows", "0"}, 0},
		{"disabled-equals", []string{"--max-rows=0"}, 0},
		{"last-value", []string{"--max-rows=12", "--max-rows", "3"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, command := range []string{"headings", "SELECT title FROM headings"} {
				args := append([]string{command, "note.md"}, tc.args...)
				cfg, err := ParseArgs(args)
				if err != nil || cfg.MaxRows != tc.want || len(cfg.Files) != 1 || cfg.Files[0] != "note.md" {
					t.Fatalf("%v: config=%#v error=%v", args, cfg, err)
				}
			}
		})
	}
}

func TestParseArgsMaxRowsRejectsInvalidValues(t *testing.T) {
	for _, args := range [][]string{
		{"--max-rows"},
		{"--max-rows", "-1"},
		{"--max-rows", "abc"},
		{"--max-rows", "1.5"},
		{"--max-rows", "--json"},
		{"--max-rows="},
		{"--max-rows=-1"},
		{"--max-rows=abc"},
		{"--max-rows=1.5"},
		{"--max-rows=999999999999999999999999"},
	} {
		_, err := ParseArgs(args)
		if err == nil || !strings.Contains(err.Error(), "--max-rows requires") {
			t.Fatalf("%v: expected a clear max-rows error, got %v", args, err)
		}
	}
}
