package query

import (
	"strings"
	"time"
)

func coerceDateComparison(left, right Value) (Value, Value) {
	if left.Kind == ValueString {
		left = coerceDateString(left, right.Kind)
	}
	if right.Kind == ValueString {
		right = coerceDateString(right, left.Kind)
	}
	return left, right
}

func coerceDateString(value Value, kind ValueKind) Value {
	switch kind {
	case ValueDate:
		if date, err := parseDate(value.String()); err == nil {
			return NewDate(date)
		}
	case ValueDateTime:
		if stamp, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value.String())); err == nil {
			return NewDateTime(stamp)
		}
		// A date-only string denotes midnight UTC when compared to a timestamp.
		if date, err := parseDate(value.String()); err == nil {
			return NewDateTime(date)
		}
	}
	return value
}
