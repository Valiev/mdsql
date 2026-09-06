package engine

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Valiev/mdsql/internal/query"
)

// Length prefixes and type tags keep separators in user data unambiguous.
func writeDistinctPart(out *strings.Builder, part string) {
	out.WriteString(strconv.Itoa(len(part)))
	out.WriteByte(':')
	out.WriteString(part)
}

func writeDistinctObject(out *strings.Builder, values map[string]query.Value, omitInternal bool) {
	keys := make([]string, 0, len(values))
	for key := range values {
		if !omitInternal || !strings.HasPrefix(key, "__") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	writeDistinctPart(out, strconv.Itoa(len(keys)))
	for _, key := range keys {
		writeDistinctPart(out, key)
		writeDistinctValue(out, values[key])
	}
}

func writeDistinctValue(out *strings.Builder, value query.Value) {
	writeDistinctPart(out, strconv.Itoa(int(value.Kind)))
	switch value.Kind {
	case query.ValueObject:
		writeDistinctObject(out, value.Data.(map[string]query.Value), false)
	case query.ValueList:
		items := value.Data.([]query.Value)
		writeDistinctPart(out, strconv.Itoa(len(items)))
		for _, item := range items {
			writeDistinctValue(out, item)
		}
	case query.ValueDate, query.ValueDateTime:
		// Equality compares instants; keep nanoseconds and normalize offsets.
		writeDistinctPart(out, value.Data.(time.Time).UTC().Format(time.RFC3339Nano))
	case query.ValueDuration:
		duration := value.Data.(query.Duration)
		writeDistinctPart(out, strconv.Itoa(duration.Value))
		writeDistinctPart(out, strconv.Itoa(int(duration.Unit)))
	default:
		writeDistinctPart(out, value.String())
	}
}
