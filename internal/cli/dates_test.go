package cli

import (
	"strings"
	"testing"
	"time"
)

func TestDateFlagValidation(t *testing.T) {
	for _, flag := range []string{"--due", "--scheduled-before", "--scheduled-after", "--today"} {
		for _, value := range []string{"notadate", "2026-13-45", "2026-02-29", "today+bad", "today+999999999999999999999d", "today OR true", "", "2026-09-03.."} {
			args := []string{"tasks", flag, value, "missing.md"}
			if _, err := ParseArgs(args); err == nil || !strings.Contains(err.Error(), "invalid value for "+flag) {
				t.Errorf("%v: %v", args, err)
			}
		}
		for _, value := range []string{"2024-02-29", "today", "today+7d", "today - 1w", "today()+2m"} {
			if _, err := ParseArgs([]string{"tasks", flag, value, "missing.md"}); err != nil {
				t.Errorf("%s %q: %v", flag, value, err)
			}
		}
	}
	cfg, err := ParseArgs([]string{"tasks", "--due", "today..today+7d", "missing.md"})
	if err != nil || !strings.Contains(cfg.Query, "due BETWEEN today AND today+7d") {
		t.Fatalf("range: %s %v", cfg.Query, err)
	}
}

func TestDateFlagCalendarAndRelativeToday(t *testing.T) {
	base := time.Date(2026, 9, 3, 15, 30, 0, 0, time.UTC)
	for _, tc := range []struct{ input, want string }{{"today", "2026-09-03"}, {"today+7d", "2026-09-10"}, {"today-1w", "2026-08-27"}, {"2024-02-29", "2024-02-29"}} {
		_, date, err := dateFlag("--today", tc.input, base)
		if err != nil || date.Format(time.DateOnly) != tc.want {
			t.Errorf("%s: %s %v", tc.input, date, err)
		}
	}
}
