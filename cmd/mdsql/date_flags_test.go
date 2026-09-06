package main

import (
	"strings"
	"testing"
)

func TestInvalidAliasDatesFailBeforeLoadingInput(t *testing.T) {
	for _, flag := range []string{"--due", "--scheduled-before", "--scheduled-after", "--today"} {
		for _, value := range []string{"notadate", "2026-13-45"} {
			code, out, stderr := captureFeedbackRun(t, []string{"--quiet", "tasks", flag, value, "/missing-date-test.md"})
			if code != 1 || out != "" || !strings.Contains(stderr, "invalid value for "+flag) || strings.Contains(stderr, "no files") {
				t.Errorf("%s=%s: %d %q %q", flag, value, code, out, stderr)
			}
		}
	}
	for _, flag := range []string{"--due", "--scheduled-before", "--scheduled-after"} {
		code, out, stderr := captureFeedbackRun(t, []string{"--explain", "tasks", flag, "today+7d"})
		if code != 0 || !strings.Contains(out, "today+7d") || stderr != "" {
			t.Errorf("%s: %d %q %q", flag, code, out, stderr)
		}
	}
}
