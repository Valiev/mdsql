package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var dateFlagPattern = regexp.MustCompile(`^(today(?:\(\))?|[0-9]{4}-[0-9]{2}-[0-9]{2})(?:\s*([+-])\s*([0-9]+)([dwmy]))?$`)

// Date flags accept date expressions, not arbitrary SQL identifiers or predicates.
func dateFlag(flag, raw string, today time.Time) (string, time.Time, error) {
	fail := func() (string, time.Time, error) {
		return "", time.Time{}, fmt.Errorf("invalid value for %s: %q; expected an ISO date, today, or today+7d", flag, raw)
	}
	parts := dateFlagPattern.FindStringSubmatch(strings.ToLower(strings.TrimSpace(raw)))
	if parts == nil {
		return fail()
	}
	base := parts[1]
	date := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	if strings.HasPrefix(base, "today") {
		base = "today"
	} else {
		var err error
		date, err = time.Parse(time.DateOnly, base)
		if err != nil {
			return fail()
		}
	}
	if parts[2] != "" {
		amount, err := strconv.Atoi(parts[3])
		// Even in days this exceeds the entire four-digit ISO year range.
		if err != nil || amount > 366*10000 {
			return fail()
		}
		if parts[2] == "-" {
			amount = -amount
		}
		switch parts[4] {
		case "d":
			date = date.AddDate(0, 0, amount)
		case "w":
			date = date.AddDate(0, 0, amount*7)
		case "m":
			date = date.AddDate(0, amount, 0)
		case "y":
			date = date.AddDate(amount, 0, 0)
		}
		base += parts[2] + parts[3] + parts[4]
	}
	if date.Year() < 1 || date.Year() > 9999 {
		return fail()
	}
	return base, date, nil
}
