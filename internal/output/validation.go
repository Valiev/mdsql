package output

import (
	"fmt"

	"github.com/Valiev/mdsql/internal/query"
)

// ValidateProjection rejects incompatible explicit selections, including empty results.
// A wildcard with no rows has no observable schema and produces empty output.
func ValidateProjection(format Format, selected []string) error {
	if len(selected) == 0 {
		return nil
	}
	keys := map[string]bool{}
	for _, key := range selected {
		if key == "*" {
			return nil
		}
		keys[key] = true
	}
	switch format {
	case FormatGrouped:
		if !keys["file"] {
			return fmt.Errorf("format %q requires column file", format)
		}
	case Format("outline"):
		if !keys["level"] || !keys["title"] {
			return fmt.Errorf("format %q requires columns level and title", format)
		}
	case FormatMD:
		if !keys["raw"] && !keys["body"] && !keys["own"] && !keys["code"] {
			return fmt.Errorf("format %q requires a content column: raw, body, own, or code", format)
		}
	}
	return nil
}

func validateFormatRows(format Format, rows []query.Row) error {
	if format != FormatMD && format != Format("outline") && format != FormatGrouped {
		return nil
	}
	for i, row := range rows {
		switch format {
		case FormatGrouped:
			if row["file"].Kind != query.ValueString {
				return fmt.Errorf("format %q requires string file (row %d)", format, i+1)
			}
		case Format("outline"):
			level, title := row["level"], row["title"]
			if level.Kind != query.ValueInt || title.Kind != query.ValueString {
				return fmt.Errorf("format %q requires integer level and string title (row %d)", format, i+1)
			}
			if n := level.Data.(int64); n < 1 || n > 6 {
				return fmt.Errorf("format %q requires heading level between 1 and 6 (row %d)", format, i+1)
			}
		case FormatMD:
			hasContent := false
			for _, key := range []string{"raw", "body", "own", "code"} {
				if value, ok := row[key]; ok && value.Kind == query.ValueString {
					hasContent = true
				}
			}
			if !hasContent {
				return fmt.Errorf("format %q requires string content in raw, body, own, or code (row %d)", format, i+1)
			}
		}
	}
	return nil
}
