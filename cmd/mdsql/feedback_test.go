package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureFeedbackRun(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	oldArgs, oldOut, oldErr := os.Args, os.Stdout, os.Stderr
	defer func() {
		os.Args, os.Stdout, os.Stderr = oldArgs, oldOut, oldErr
		stdout.Close()
		stderr.Close()
	}()
	os.Args, os.Stdout, os.Stderr = append([]string{"mdsql"}, args...), stdout, stderr
	code := run()
	if _, err := stdout.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := stderr.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(stdout)
	if err != nil {
		t.Fatal(err)
	}
	errOut, err := io.ReadAll(stderr)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(out), string(errOut)
}

func TestQueryDiagnosticsUseStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(path, []byte("---\ncategories: [demo]\n---\n# Demo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sql := "SELECT file FROM frontmatter WHERE fm.categories = 'demo'"
	code, out, errOut := captureFeedbackRun(t, []string{"--json", sql, path})
	if code != 0 || out != "[]\n" || strings.Count(errOut, "mdsql: warning:") != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	code, out, errOut = captureFeedbackRun(t, []string{"--strict", "--json", sql, path})
	if code != 1 || out != "" || !strings.Contains(errOut, "incompatible comparison") {
		t.Fatalf("strict: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	code, out, errOut = captureFeedbackRun(t, []string{"--quiet", "--json", sql, path})
	if code != 0 || out != "[]\n" || errOut != "" {
		t.Fatalf("quiet: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestCLIRejectsFormatBeforeLoadingInput(t *testing.T) {
	for _, format := range []string{"md", "outline"} {
		code, out, errOut := captureFeedbackRun(t, []string{"--format", format, "SELECT text FROM tasks", "does-not-exist.md"})
		if code != 1 || out != "" || !strings.Contains(errOut, "requires") {
			t.Fatalf("format=%s code=%d stdout=%q stderr=%q", format, code, out, errOut)
		}
	}
}

func TestCLIVersionAliases(t *testing.T) {
	for _, flag := range []string{"version", "--version", "-V"} {
		code, out, errOut := captureFeedbackRun(t, []string{flag})
		if code != 0 || out != "mdsql "+appVersion+"\n" || errOut != "" {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", flag, code, out, errOut)
		}
	}
}
