package query

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestIntegerLiteralBoundaries(t *testing.T) {
	for _, tc := range []struct {
		literal string
		want    int64
	}{
		{"9223372036854775807", math.MaxInt64},
		{"-9223372036854775808", math.MinInt64},
		{"+9223372036854775807", math.MaxInt64},
		{"- 9223372036854775808", math.MinInt64},
		{"-0009223372036854775808", math.MinInt64},
		{"-42", -42},
		{"-0", 0},
	} {
		t.Run(tc.literal, func(t *testing.T) {
			q, err := Parse("SELECT " + tc.literal + " FROM headings WHERE " + tc.literal + " = " + tc.literal)
			if err != nil {
				t.Fatal(err)
			}
			value, err := Eval(q.Select[0].Expr, Row{}, nil)
			if err != nil || value.Kind != ValueInt || value.Data != tc.want {
				t.Fatalf("value=%v want=%d error=%v", value, tc.want, err)
			}
			value, err = Eval(q.Where, Row{}, nil)
			if err != nil || !truthy(value) {
				t.Fatalf("WHERE value=%v error=%v", value, err)
			}
		})
	}
}

func TestIntegerLiteralOverflowHasPosition(t *testing.T) {
	for _, tc := range []struct{ sql, literal string }{
		{"SELECT 9223372036854775808 FROM headings", "9223372036854775808"},
		{"SELECT -9223372036854775809 FROM headings", "-9223372036854775809"},
		{"SELECT 999999999999999999999999999999999999 FROM headings", "999999999999999999999999999999999999"},
		{"SELECT title FROM headings WHERE 9223372036854775807 = 9223372036854775808", "9223372036854775808"},
		{"SELECT title FROM headings WHERE false AND line = -9223372036854775809", "-9223372036854775809"},
		{"SELECT title FROM headings WHERE true OR line = 9223372036854775808", "9223372036854775808"},
		{"SELECT title FROM headings LIMIT 9223372036854775808", "9223372036854775808"},
		{"SELECT title FROM headings OFFSET 9223372036854775808", "9223372036854775808"},
		{"SELECT title AS Заголовок FROM headings WHERE line = 9223372036854775808", "9223372036854775808"},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			_, err := Parse(tc.sql)
			prefix := fmt.Sprintf("at byte offset %d: integer literal %q is out of range", strings.Index(tc.sql, tc.literal), tc.literal)
			if err == nil || !strings.HasPrefix(err.Error(), prefix) {
				t.Fatalf("error=%v want prefix=%q", err, prefix)
			}
		})
	}
}

func TestLimitAndOffsetRespectNativeIntegerRange(t *testing.T) {
	max := int64(math.MaxInt64)
	if strconv.IntSize == 32 {
		max = math.MaxInt32
	}
	q, err := Parse(fmt.Sprintf("SELECT title FROM headings LIMIT %d OFFSET %d", max, max))
	if err != nil || q.Limit != int(max) || q.Offset != int(max) || !q.LimitSet {
		t.Fatalf("query=%#v error=%v", q, err)
	}
	for _, clause := range []string{"LIMIT", "OFFSET"} {
		_, err := Parse(fmt.Sprintf("SELECT title FROM headings %s %d", clause, uint64(max)+1))
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("signed %d-bit integer", strconv.IntSize)) {
			t.Fatalf("%s error=%v", clause, err)
		}
	}
}

func TestDurationOverflowStillRejected(t *testing.T) {
	for _, literal := range []string{"9223372036854775808d", "-9223372036854775809d"} {
		if _, err := Parse("SELECT " + literal + " FROM headings"); err == nil {
			t.Errorf("accepted overflowing duration %s", literal)
		}
	}
	q, err := Parse("SELECT -7d FROM headings")
	if err != nil {
		t.Fatal(err)
	}
	value, err := Eval(q.Select[0].Expr, Row{}, nil)
	if err != nil || value.Kind != ValueDuration || value.Data != (Duration{Value: -7, Unit: 'd'}) {
		t.Fatalf("duration=%v error=%v", value, err)
	}
}
