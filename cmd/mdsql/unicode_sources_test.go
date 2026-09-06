package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestUnicodeSourcesDisplayNFCWithPhysicalIO(t *testing.T) {
	base := t.TempDir()
	nfcRoot := filepath.Join(base, "Мой архив")
	nfdRoot := norm.NFD.String(nfcRoot)
	if err := os.Mkdir(nfdRoot, 0700); err != nil {
		t.Fatal(err)
	}
	nfcFile := "Йога.md"
	nfdFile := norm.NFD.String(nfcFile)
	if err := os.WriteFile(filepath.Join(nfdRoot, nfdFile), []byte("# Yoga\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
	want := []map[string]string{{"file": nfcFile, "title": "Yoga"}}
	cacheFlags := []string{"--no-cache"}
	if runtime.GOOS != "windows" {
		cacheFlags = append(cacheFlags, "--cache", "--cache")
	}
	for _, root := range []string{nfcRoot, nfdRoot} {
		for _, tc := range []struct {
			query  string
			args   []string
			source string
		}{
			{query: "SELECT file, title FROM headings WHERE file = 'Йога.md'", source: "."},
			{query: "SELECT file, title FROM headings", source: nfcFile},
			{query: "SELECT file, title FROM headings", source: nfdFile},
			{query: "SELECT file, title FROM headings", source: "Йо*.md"},
			{query: "SELECT file, title FROM headings", source: ".", args: []string{"--include", "**/Йога.md"}},
			{query: "SELECT file, title FROM headings IN 'Йо*.md'"},
		} {
			for _, cacheFlag := range cacheFlags {
				args := append([]string{cacheFlag, "--root", root, "--json"}, tc.args...)
				args = append(args, tc.query)
				if tc.source != "" {
					args = append(args, tc.source)
				}
				code, out, stderr := captureFeedbackRun(t, args)
				var got []map[string]string
				if err := json.Unmarshal([]byte(out), &got); err != nil {
					t.Fatalf("args %q: code=%d out=%q stderr=%q: %v", args, code, out, stderr, err)
				}
				if code != 0 || stderr != "" || !reflect.DeepEqual(got, want) {
					t.Errorf("args %q: code=%d rows=%q stderr=%q; want %q", args, code, got, stderr, want)
				}
			}
		}
		code, out, stderr := captureFeedbackRun(t, []string{"--no-cache", "--root", root, "--json", "SELECT file FROM files WHERE file = 'Йога.md'", "."})
		var got []map[string]string
		if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 || stderr != "" || !reflect.DeepEqual(got, []map[string]string{{"file": nfcFile}}) {
			t.Errorf("metadata path root=%q: code=%d rows=%q stderr=%q err=%v", root, code, got, stderr, err)
		}
	}
}
