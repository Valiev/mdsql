package cache

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/parser"
)

const synthetic = `---
title: Synthetic cache fixture
enabled: true
missing: null
empty: []
empty_map: {}
count: 42
huge: 18446744073709551615
ratio: 2.5
stamp: 2026-09-02T12:34:56+03:00
nested:
  entries: [one, true, null, 2, 3.5, []]
  mixed: {1: numeric-key, other: string-key}
tags: [example, cache]
---
# Synthetic heading

Text with [[Synthetic link]] and [web](https://example.test) #synthetic

- [ ] Synthetic task 📅 2026-09-03 ⏳ 2026-09-02 🛫 2026-09-01 ➕ 2026-08-31 [points:: 3] [review:: 2026-09-04]
- [x] Synthetic done ✅ 2026-09-02
- [-] Synthetic cancelled ❌ 2026-09-02

> [!note] Synthetic callout
> Body

~~~go
fmt.Println("synthetic")
~~~

| col |
| --- |
| val |
`

func testCache(t testing.TB) *Cache {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("private cache is unsupported on Windows")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	c, err := OpenDefault()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func testDocument(t testing.TB, path, raw string) (*model.Document, os.FileInfo) {
	t.Helper()
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parser.ParseFile(path, raw, fi.Size(), fi.ModTime())
	if err != nil {
		t.Fatal(err)
	}
	return doc, fi
}

func TestRoundTrip(t *testing.T) {
	c := testCache(t)
	for _, raw := range []string{synthetic, "", "---\nitems: []\n---\nplain\n", "---\n---\n"} {
		doc, fi := testDocument(t, filepath.Join(t.TempDir(), "note.md"), raw)
		if err := c.Store(doc); err != nil {
			t.Fatal(err)
		}
		got, ok := c.Load(doc.Path, fi)
		if !ok {
			t.Fatal("cache miss after Store")
		}
		if !reflect.DeepEqual(doc, got) {
			t.Fatalf("document changed after round trip:\nwant %#v\ngot  %#v", doc, got)
		}
	}
}

func TestMutationIsolation(t *testing.T) {
	c := testCache(t)
	doc, fi := testDocument(t, filepath.Join(t.TempDir(), "note.md"), synthetic)
	if err := c.Store(doc); err != nil {
		t.Fatal(err)
	}
	doc.FrontMatter["title"] = "caller mutation"
	doc.Headings[0].File = "relative.md"
	first, ok := c.Load(doc.Path, fi)
	if !ok {
		t.Fatal("cache miss")
	}
	if first.FrontMatter["title"] != "Synthetic cache fixture" {
		t.Fatal("Store kept caller state")
	}
	first.FrontMatter["nested"].(model.FrontMatter)["entries"].([]interface{})[0] = "changed"
	first.Tasks[0].Fields["points"] = 100
	first.Headings[0].File = "changed"
	second, ok := c.Load(doc.Path, fi)
	if !ok {
		t.Fatal("cache miss")
	}
	if second.FrontMatter["nested"].(model.FrontMatter)["entries"].([]interface{})[0] != "one" || second.Tasks[0].Fields["points"] != 3 || second.Headings[0].File != doc.Path {
		t.Fatal("Load results share mutable state")
	}
}

func TestInvalidation(t *testing.T) {
	c := testCache(t)
	doc, fi := testDocument(t, filepath.Join(t.TempDir(), "note.md"), synthetic)
	if err := c.Store(doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Load("note.md", fi); ok {
		t.Fatal("relative path hit")
	}
	if _, ok := c.Load(doc.Path, nil); ok {
		t.Fatal("missing source stat hit")
	}
	other := filepath.Join(filepath.Dir(doc.Path), "renamed.md")
	if err := os.Rename(doc.Path, other); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Load(other, fi); ok {
		t.Fatal("renamed path hit")
	}
	if err := c.Store(doc); err == nil {
		t.Fatal("stored missing source")
	}
	if err := os.Rename(other, doc.Path); err != nil {
		t.Fatal(err)
	}
	changed := fi.ModTime().Add(time.Second)
	if err := os.Chtimes(doc.Path, changed, changed); err != nil {
		t.Fatal(err)
	}
	fi2, err := os.Stat(doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Load(doc.Path, fi2); ok {
		t.Fatal("changed mtime hit")
	}
	if err := c.Store(doc); err == nil {
		t.Fatal("stored stale document")
	}
	if err := os.WriteFile(doc.Path, []byte(synthetic+"more"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(doc.Path, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	fi2, err = os.Stat(doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Load(doc.Path, fi2); ok {
		t.Fatal("changed size hit")
	}
}

func TestCorruption(t *testing.T) {
	c := testCache(t)
	doc, fi := testDocument(t, filepath.Join(t.TempDir(), "note.md"), synthetic)
	if err := c.Store(doc); err != nil {
		t.Fatal(err)
	}
	good, err := os.ReadFile(c.slot(doc.Path))
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func([]byte) []byte{
		"truncated":         func(b []byte) []byte { return b[:headerSize+1] },
		"magic":             func(b []byte) []byte { b[0] ^= 1; return b },
		"format":            func(b []byte) []byte { binary.LittleEndian.PutUint32(b[len(magic):], formatVersion+1); return b },
		"length":            func(b []byte) []byte { binary.LittleEndian.PutUint64(b[len(magic)+4:], maxPayload+1); return b },
		"decoded length":    func(b []byte) []byte { return alterEntry(t, b, func(e *entry) { e.ContentSize = maxPayload + 1 }) },
		"decoded too short": func(b []byte) []byte { return alterEntry(t, b, func(e *entry) { e.ContentSize = 1 }) },
		"decoded too long":  func(b []byte) []byte { return alterEntry(t, b, func(e *entry) { e.ContentSize++ }) },
		"checksum":          func(b []byte) []byte { b[len(b)-1] ^= 1; return b },
		"extra":             func(b []byte) []byte { return append(b, 0) },
		"bad gob": func(b []byte) []byte {
			for i := headerSize; i < len(b); i++ {
				b[i] = 0xff
			}
			checksum(b)
			return b
		},
		"parser version": func(b []byte) []byte { return alterEntry(t, b, func(e *entry) { e.ParserVersion = "old" }) },
		"metadata path":  func(b []byte) []byte { return alterEntry(t, b, func(e *entry) { e.Path = "/other.md" }) },
		"document path":  func(b []byte) []byte { return alterEntry(t, b, func(e *entry) { e.Document.Path = "/other.md" }) },
		"document size":  func(b []byte) []byte { return alterEntry(t, b, func(e *entry) { e.Document.Size++ }) },
		"invalid string reference": func(b []byte) []byte {
			return alterEntry(t, b, func(e *entry) { e.TextRefs[0] = uint32(len(e.Strings)) })
		},
		"missing string references": func(b []byte) []byte { return alterEntry(t, b, func(e *entry) { e.TextRefs = nil }) },
		"extra string reference":    func(b []byte) []byte { return alterEntry(t, b, func(e *entry) { e.TextRefs = append(e.TextRefs, 0) }) },
		"invalid source range": func(b []byte) []byte {
			return alterEntry(t, b, func(e *entry) { e.Strings[0].Start = 0; e.Strings[0].End = maxPayload + 1 })
		},
		"negative source range": func(b []byte) []byte { return alterEntry(t, b, func(e *entry) { e.Strings[0].Start = -2 }) },
		"wrong line number":     func(b []byte) []byte { return alterEntry(t, b, func(e *entry) { e.Document.LinesRows[0].N++ }) },
	}
	for name, corrupt := range tests {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(c.slot(doc.Path), corrupt(bytes.Clone(good)), 0600); err != nil {
				t.Fatal(err)
			}
			if _, ok := c.Load(doc.Path, fi); ok {
				t.Fatal("corrupt entry hit")
			}
		})
	}
	t.Run("oversized file", func(t *testing.T) {
		f, err := os.Create(c.slot(doc.Path))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(int64(headerSize + maxPayload + 1)); err != nil {
			t.Fatal(err)
		}
		f.Close()
		if _, ok := c.Load(doc.Path, fi); ok {
			t.Fatal("oversized entry hit")
		}
	})
}

func checksum(data []byte) {
	binary.LittleEndian.PutUint64(data[len(magic)+4:], uint64(len(data)-headerSize))
	sum := sha256.Sum256(data[headerSize:])
	copy(data[headerSize-sha256.Size:headerSize], sum[:])
}

func alterEntry(t *testing.T, data []byte, change func(*entry)) []byte {
	t.Helper()
	var e entry
	if err := gob.NewDecoder(bytes.NewReader(data[headerSize:])).Decode(&e); err != nil {
		t.Fatal(err)
	}
	change(&e)
	var out bytes.Buffer
	if err := gob.NewEncoder(&out).Encode(e); err != nil {
		t.Fatal(err)
	}
	data = append(data[:headerSize:headerSize], out.Bytes()...)
	checksum(data)
	return data
}

func TestPrivatePermissionsAndSymlinks(t *testing.T) {
	c := testCache(t)
	doc, fi := testDocument(t, filepath.Join(t.TempDir(), "note.md"), synthetic)
	if err := c.Store(doc); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]os.FileMode{c.dir: 0700, c.slot(doc.Path): 0600} {
		got, err := os.Lstat(name)
		if err != nil {
			t.Fatal(err)
		}
		if got.Mode().Perm() != want {
			t.Fatalf("mode %s: got %o, want %o", name, got.Mode().Perm(), want)
		}
	}
	if err := os.Chmod(c.slot(doc.Path), 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Load(doc.Path, fi); ok {
		t.Fatal("public entry hit")
	}
	if err := os.Chmod(c.dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDefault(); err == nil {
		t.Fatal("accepted public cache directory")
	}
	if _, ok := c.Load(doc.Path, fi); ok {
		t.Fatal("public directory hit")
	}
	if err := c.Store(doc); err == nil {
		t.Fatal("stored into public directory")
	}
	if err := ClearDefault(); err == nil {
		t.Fatal("cleared public directory")
	}
	if err := os.Chmod(c.dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(c.slot(doc.Path)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(doc.Path, c.slot(doc.Path)); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Load(doc.Path, fi); ok {
		t.Fatal("symlink entry hit")
	}
	if err := c.Store(doc); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(doc.Path)
	if err != nil || string(contents) != synthetic {
		t.Fatal("Store followed symlink")
	}
	oldDir := c.dir + "-old"
	if err := os.Rename(c.dir, oldDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(oldDir, c.dir); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDefault(); err == nil {
		t.Fatal("accepted symlink directory")
	}
	if err := ClearDefault(); err == nil {
		t.Fatal("cleared symlink directory")
	}
}

func TestDefaultLocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		if _, err := OpenDefault(); err == nil {
			t.Fatal("Windows cache enabled without private ACL support")
		}
		return
	}
	c := testCache(t)
	if c.dir != filepath.Join(os.Getenv("XDG_CACHE_HOME"), "mdsql") {
		t.Fatal("ignored XDG_CACHE_HOME")
	}
	t.Setenv("XDG_CACHE_HOME", "relative")
	if _, err := OpenDefault(); err == nil {
		t.Fatal("accepted relative XDG_CACHE_HOME")
	}
	t.Setenv("XDG_CACHE_HOME", "")
	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := defaultDir()
	if err != nil || got != filepath.Join(base, "mdsql") {
		t.Fatalf("fallback %q: %v", got, err)
	}
}

func TestClearOnlyCacheFiles(t *testing.T) {
	c := testCache(t)
	doc, _ := testDocument(t, filepath.Join(t.TempDir(), "note.md"), synthetic)
	if err := c.Store(doc); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{tempPrefix + "123456", "keep.txt", tempPrefix + "123-not-a-temp", "not-a-slot.gob"} {
		if err := os.WriteFile(filepath.Join(c.dir, name), []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	subdir := filepath.Join(c.dir, strings.Repeat("a", 64)+".gob")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "keep"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(c.dir, strings.Repeat("b", 64)+".gob")
	if err := os.Symlink(doc.Path, link); err != nil {
		t.Fatal(err)
	}
	if err := ClearDefault(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{c.slot(doc.Path), filepath.Join(c.dir, tempPrefix+"123456")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("cache file remains: %s", path)
		}
	}
	for _, path := range []string{filepath.Join(c.dir, "keep.txt"), filepath.Join(c.dir, tempPrefix+"123-not-a-temp"), filepath.Join(c.dir, "not-a-slot.gob"), filepath.Join(subdir, "keep"), doc.Path, link} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("removed unrelated file %s: %v", path, err)
		}
	}
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "missing"))
	if err := ClearDefault(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(os.Getenv("XDG_CACHE_HOME")); !os.IsNotExist(err) {
		t.Fatal("clear created missing directory")
	}
}

func TestConcurrentWriters(t *testing.T) {
	c := testCache(t)
	doc, fi := testDocument(t, filepath.Join(t.TempDir(), "note.md"), synthetic)
	if err := c.Store(doc); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 12; j++ {
				if err := c.Store(doc); err != nil {
					t.Error(err)
					return
				}
				got, ok := c.Load(doc.Path, fi)
				// An atomic replacement between lstat and open may safely miss.
				if ok && !reflect.DeepEqual(got, doc) {
					t.Error("partial document from concurrent Store")
					return
				}
			}
		}()
	}
	wg.Wait()
	got, ok := c.Load(doc.Path, fi)
	if !ok || !reflect.DeepEqual(got, doc) {
		t.Fatal("final concurrent entry invalid")
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files remain: %v", entries)
	}
}

func TestStoreRejectsQueryPathsAndEncodingFailures(t *testing.T) {
	c := testCache(t)
	doc, _ := testDocument(t, filepath.Join(t.TempDir(), "note.md"), synthetic)
	doc.RelPath = "note.md"
	if err := c.Store(doc); err == nil {
		t.Fatal("stored query-rewritten path")
	}
	doc.RelPath = ""
	doc.FrontMatter["unsupported"] = make(chan int)
	if err := c.Store(doc); err == nil {
		t.Fatal("stored unencodable document")
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed encoding left files")
	}
	var out limitedBuffer
	if _, err := out.Write(make([]byte, maxPayload+1)); err == nil {
		t.Fatal("accepted oversized payload")
	}
}

func BenchmarkSyntheticCorpus(b *testing.B) {
	c := testCache(b)
	dir := b.TempDir()
	const count = 2048
	paths := make([]string, count)
	var cacheBytes int64
	for i := range paths {
		paths[i] = filepath.Join(dir, fmt.Sprintf("note-%04d.md", i))
		doc, _ := testDocument(b, paths[i], synthetic)
		if err := c.Store(doc); err != nil {
			b.Fatal(err)
		}
		fi, err := os.Stat(c.slot(doc.Path))
		if err != nil {
			b.Fatal(err)
		}
		cacheBytes += fi.Size()
	}
	for _, mode := range []string{"uncached", "warm", "cold"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(count, "files/op")
			b.ReportMetric(float64(cacheBytes), "cache-bytes/corpus")
			for i := 0; i < b.N; i++ {
				if mode == "cold" {
					b.StopTimer()
					if err := ClearDefault(); err != nil {
						b.Fatal(err)
					}
					b.StartTimer()
				}
				for _, path := range paths {
					f, err := os.Open(path)
					if err != nil {
						b.Fatal(err)
					}
					fi, err := f.Stat()
					if err != nil {
						b.Fatal(err)
					}
					if mode != "uncached" {
						if _, ok := c.Load(path, fi); ok {
							f.Close()
							continue
						}
					}
					var raw bytes.Buffer
					if _, err := raw.ReadFrom(f); err != nil {
						b.Fatal(err)
					}
					if err := f.Close(); err != nil {
						b.Fatal(err)
					}
					doc, err := parser.ParseFile(path, raw.String(), fi.Size(), fi.ModTime())
					if err != nil {
						b.Fatal(err)
					}
					if mode == "cold" {
						if err := c.Store(doc); err != nil {
							b.Fatal(err)
						}
					}
				}
			}
		})
	}
}
