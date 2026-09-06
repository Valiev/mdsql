package engine

import (
	"fmt"
	"strconv"

	"github.com/Valiev/mdsql/internal/query"
)

func nameProjections(selected []query.SelectItem) ([]query.SelectItem, error) {
	explicit := make(map[string]bool)
	for _, item := range selected {
		if item.Alias == "" {
			continue
		}
		if explicit[item.Alias] {
			return nil, fmt.Errorf("duplicate output alias %q", item.Alias)
		}
		explicit[item.Alias] = true
	}
	result := append([]query.SelectItem(nil), selected...)
	used := make(map[string]bool)
	for i, item := range result {
		name := item.Alias
		if name == "" {
			base := (&query.Query{Select: []query.SelectItem{item}}).SelectKeys()[0]
			name = base
			for suffix := 2; used[name] || explicit[name]; suffix++ {
				name = base + "_" + strconv.Itoa(suffix)
			}
		}
		result[i].Alias = name
		used[name] = true
	}
	return result, nil
}
