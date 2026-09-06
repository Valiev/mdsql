package cli

import (
	"fmt"
	"strings"
)

// Alias options are separate from global flags, and values may start with a dash.
var aliasOptions = map[string]map[string]int{
	"headings": {"--under": 1, "-l": 1, "--level": 1, "--min-level": 1, "--max-level": 1},
	"tasks": {
		"--open": 0, "--done": 0, "--in-progress": 0, "--state": 1, "--tag": 1,
		"--depth": 1, "--due": 1, "--overdue": 0, "--scheduled-before": 1,
		"--scheduled-after": 1, "--priority": 1, "--path": 1, "--section": 1,
		"--has-id": 0, "--raw": 0,
	},
	"code":        {"--lang": 1, "--section": 1, "--under": 1},
	"links":       {"--type": 1, "--external": 0, "--internal": 0, "--target": 1},
	"tags":        {"--count": 0, "--source": 1, "--explode-frontmatter": 0},
	"frontmatter": {}, "files": {}, "sections": {}, "blocks": {}, "tables": {},
	"at": {}, "toc": {}, "lines": {}, "grep": {}, "section": {},
}

func aliasOptionNeedsValue(flag string) bool {
	for _, options := range aliasOptions {
		if options[flag] == 1 {
			return true
		}
	}
	return false
}

func isQueryComment(arg string) bool {
	return strings.HasPrefix(arg, "--") && (strings.ContainsAny(arg, "\r\n") || strings.HasPrefix(arg, "-- ") || strings.HasPrefix(arg, "--\t"))
}

func validateFlags(command string, args []string) error {
	options := aliasOptions[command]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			continue
		}
		arity, ok := options[arg]
		if !ok {
			return fmt.Errorf("unknown flag %q", arg)
		}
		if arity == 1 {
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires value", arg)
			}
			i++
		}
	}
	return nil
}
