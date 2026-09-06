package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func symlinkFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "notes"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "note.md"), []byte("# Linked\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for alias, target := range map[string]string{"linked": "notes", "alias.md": "notes/note.md"} {
		if err := os.Symlink(target, filepath.Join(root, alias)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	return root
}

func TestSymlinkPositionalINAndCachedLogicalPaths(t *testing.T) {
	root := symlinkFixture(t)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	for _, tc := range []struct {
		query string
		files []string
		want  []map[string]string
	}{
		{"SELECT file, title FROM headings", []string{"alias.md"}, []map[string]string{{"file": "alias.md", "title": "Linked"}}},
		{"SELECT file, title FROM headings", []string{"linked"}, []map[string]string{{"file": "linked/note.md", "title": "Linked"}}},
		{"SELECT file, title FROM headings IN 'linked'", nil, []map[string]string{{"file": "linked/note.md", "title": "Linked"}}},
		{"SELECT file, title FROM headings IN 'li*/**/*.md'", nil, []map[string]string{{"file": "linked/note.md", "title": "Linked"}}},
		{"SELECT file, title FROM headings ORDER BY file", []string{"notes/note.md", "alias.md", "alias.md"}, []map[string]string{{"file": "alias.md", "title": "Linked"}, {"file": "notes/note.md", "title": "Linked"}}},
	} {
		for _, cacheFlag := range []string{"--no-cache", "--cache", "--cache"} {
			args := append([]string{cacheFlag, "--root", root, "--json", tc.query}, tc.files...)
			code, out, stderr := captureFeedbackRun(t, args)
			var got []map[string]string
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("%v: code=%d out=%q stderr=%q: %v", args, code, out, stderr, err)
			}
			if code != 0 || stderr != "" || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%v: code=%d out=%q stderr=%q; want %v", args, code, out, stderr, tc.want)
			}
		}
	}
}

func TestSymlinkCycleWarningStrictAndQuiet(t *testing.T) {
	root := symlinkFixture(t)
	if err := os.Symlink(".", filepath.Join(root, "notes", "cycle")); err != nil {
		t.Fatal(err)
	}
	args := []string{"--no-cache", "--root", root, "--json", "SELECT file FROM files", "linked"}
	code, out, stderr := captureFeedbackRun(t, args)
	if code != 0 || !strings.Contains(out, "linked/note.md") || strings.Contains(out, "cycle/") || !strings.Contains(stderr, "warning: symlink cycle skipped:") {
		t.Fatalf("default: code=%d out=%q stderr=%q", code, out, stderr)
	}
	code, out, stderr = captureFeedbackRun(t, append([]string{"--strict"}, args...))
	if code != 2 || out != "" || !strings.Contains(stderr, "symlink cycle skipped:") {
		t.Fatalf("strict: code=%d out=%q stderr=%q", code, out, stderr)
	}
	code, out, stderr = captureFeedbackRun(t, append([]string{"--quiet"}, args...))
	if code != 0 || !strings.Contains(out, "linked/note.md") || stderr != "" {
		t.Fatalf("quiet: code=%d out=%q stderr=%q", code, out, stderr)
	}
}
