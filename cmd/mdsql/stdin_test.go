package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stdinRun(t *testing.T, input string, args []string) (int, string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()
	return captureFeedbackRun(t, args)
}

func TestEmptyStdinDiagnostics(t *testing.T) {
	t.Setenv("MDSQL_STDIN_NAME", "synthetic-input.md")
	args := []string{"--json", "SELECT file, title FROM headings"}
	code, out, stderr := stdinRun(t, "", args)
	if code != 0 || out != "[]\n" || !strings.Contains(stderr, "warning: empty stdin (synthetic-input.md)") {
		t.Fatalf("empty: %d %q %q", code, out, stderr)
	}
	code, out, stderr = stdinRun(t, "", append([]string{"--strict"}, args...))
	if code != 2 || out != "" || !strings.Contains(stderr, "empty stdin") {
		t.Fatalf("strict: %d %q %q", code, out, stderr)
	}
	code, out, stderr = stdinRun(t, "# Hello\n", append([]string{"--strict"}, args...))
	if code != 0 || !strings.Contains(out, "synthetic-input.md") || !strings.Contains(out, "Hello") || stderr != "" {
		t.Fatalf("nonempty: %d %q %q", code, out, stderr)
	}
	code, out, stderr = stdinRun(t, "", append([]string{"--quiet"}, args...))
	if code != 0 || out != "[]\n" || stderr != "" {
		t.Fatalf("quiet: %d %q %q", code, out, stderr)
	}
}
