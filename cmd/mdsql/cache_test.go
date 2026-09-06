package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/cache"
	"github.com/Valiev/mdsql/internal/tables"
)

func TestCacheMatchesEveryTableAcrossRoots(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cache is unsupported on Windows")
	}
	base := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
	root := filepath.Join(base, "notes")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "note.md")
	raw := "---\ncategories: [one, one, two]\nnumber: 7\nenabled: true\nempty: []\nmissing: null\nnested: {items: [2, {name: demo}]}\n---\n# Demo\n- [ ] A task #work 📅 2026-09-05\n\n[link](https://example.com)\n\n> quote\n\n| A | B |\n| - | - |\n| x | y |\n\n```go\nvar x = 1\n```\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	for _, queryRoot := range []string{root, base} {
		for _, table := range tables.TableNames() {
			args := []string{"--json", "--max-rows=0", "--root", queryRoot, "SELECT * FROM " + table, path}
			code, want, stderr := captureFeedbackRun(t, args)
			if code != 0 || stderr != "" {
				t.Fatalf("uncached %s: %d %s", table, code, stderr)
			}
			for attempt := 0; attempt < 2; attempt++ {
				cachedCode, got, cachedErr := captureFeedbackRun(t, append([]string{"--cache"}, args...))
				if cachedCode != code || got != want || cachedErr != stderr {
					t.Fatalf("root=%s table=%s attempt=%d cached=(%d,%q,%q) want=(%d,%q,%q)", queryRoot, table, attempt, cachedCode, got, cachedErr, code, want, stderr)
				}
			}
		}
	}
}

func TestCacheEditDeleteRenameAndClear(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cache is unsupported on Windows")
	}
	base := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
	path := filepath.Join(base, "note.md")
	if err := os.WriteFile(path, []byte("# Before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	queryFile := func(path, title string) {
		t.Helper()
		code, out, stderr := captureFeedbackRun(t, []string{"--cache", "--json", "SELECT title FROM headings", path})
		if code != 0 || !strings.Contains(out, title) || stderr != "" {
			t.Fatalf("code=%d out=%q stderr=%q", code, out, stderr)
		}
	}
	queryFile(path, "Before")
	if err := os.WriteFile(path, []byte("# After with changed size\n"), 0600); err != nil {
		t.Fatal(err)
	}
	queryFile(path, "After with changed size")
	newPath := filepath.Join(base, "renamed.md")
	if err := os.Rename(path, newPath); err != nil {
		t.Fatal(err)
	}
	queryFile(newPath, "After with changed size")
	if err := os.Remove(newPath); err != nil {
		t.Fatal(err)
	}
	code, out, _ := captureFeedbackRun(t, []string{"--strict", "--cache", "--json", "SELECT title FROM headings", newPath})
	if code != 2 || out != "" {
		t.Fatalf("deleted input returned cached data: %d %q", code, out)
	}
	code, out, stderr := captureFeedbackRun(t, []string{"--clear-cache"})
	if code != 0 || out != "" || stderr != "" {
		t.Fatalf("clear: %d %q %q", code, out, stderr)
	}
	entries, err := os.ReadDir(filepath.Join(base, "cache", "mdsql"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("cache not empty: %v %v", entries, err)
	}
}

func TestCacheFallbackAndExplicitDisable(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "note.md")
	if err := os.WriteFile(path, []byte("# Demo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", "relative-cache-is-invalid")
	code, out, stderr := captureFeedbackRun(t, []string{"--cache", "--strict", "--json", "SELECT title FROM headings", path})
	if code != 0 || !strings.Contains(out, "Demo") || !strings.Contains(stderr, "cache:") {
		t.Fatalf("fallback: %d %q %q", code, out, stderr)
	}
	code, out, stderr = captureFeedbackRun(t, []string{"--cache", "--no-cache", "--strict", "--json", "SELECT title FROM headings", path})
	if code != 0 || !strings.Contains(out, "Demo") || stderr != "" {
		t.Fatalf("disabled: %d %q %q", code, out, stderr)
	}
}

func TestCacheDoesNotStoreStdin(t *testing.T) {
	base := t.TempDir()
	cacheDir := filepath.Join(base, "cache")
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	input := filepath.Join(base, "stdin.md")
	if err := os.WriteFile(input, []byte("# Stdin\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()
	code, out, stderr := captureFeedbackRun(t, []string{"--cache", "--stdin-name", input, "--json", "SELECT title FROM headings", "-"})
	if code != 0 || !strings.Contains(out, "Stdin") || stderr != "" {
		t.Fatalf("stdin: %d %q %q", code, out, stderr)
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Fatalf("stdin created cache: %v", err)
	}
}

func TestCacheKnownMetadataCollisionCanBeBypassed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cache is unsupported on Windows")
	}
	base := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
	path := filepath.Join(base, "note.md")
	if err := os.WriteFile(path, []byte("# Old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	c, err := cache.OpenDefault()
	if err != nil {
		t.Fatal(err)
	}
	warn := func(err error) { t.Fatal(err) }
	if _, err := loadDocument(path, 1024, c, warn); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# New\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	cached, err := loadDocument(path, 1024, c, warn)
	if err != nil || cached.Headings[0].Title != "Old" {
		t.Fatalf("expected documented metadata-only hit: %+v %v", cached, err)
	}
	fresh, err := loadDocument(path, 1024, nil, warn)
	if err != nil || fresh.Headings[0].Title != "New" {
		t.Fatalf("uncached load not fresh: %+v %v", fresh, err)
	}
}
