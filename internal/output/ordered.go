package output

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Valiev/mdsql/internal/query"
)

type orderedRow struct {
	row  query.Row
	keys []string
}

func (row orderedRow) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	seen := make(map[string]bool, len(row.keys))
	for _, key := range row.keys {
		if seen[key] || strings.HasPrefix(key, "__") {
			continue
		}
		if len(seen) > 0 {
			b.WriteByte(',')
		}
		seen[key] = true
		name, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(toAny(row.row[key]))
		if err != nil {
			return nil, err
		}
		b.Write(name)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (r *Renderer) renderOrderedJSON(rows []query.Row, keys []string, lines bool) error {
	enc := json.NewEncoder(r.Out)
	ordered := make([]orderedRow, 0, len(rows))
	for _, row := range rows {
		item := orderedRow{row: row, keys: keys}
		if lines {
			if err := enc.Encode(item); err != nil {
				return err
			}
		} else {
			ordered = append(ordered, item)
		}
	}
	if lines {
		return nil
	}
	return enc.Encode(ordered)
}
