package cache

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/parser"
)

func TestStoreDoesNotMutateDocument(t *testing.T) {
	c := testCache(t)
	for _, raw := range []string{synthetic, strings.ReplaceAll(synthetic, "\n", "\r\n"), "", "---\nlink: https://example.test\nnested:\n  page: https://example.test/page\n---\n"} {
		doc, fi := testDocument(t, filepath.Join(t.TempDir(), "note.md"), raw)
		want, err := parser.ParseFile(doc.Path, raw, fi.Size(), fi.ModTime())
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Store(doc); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(doc, want) {
			t.Fatal("Store mutated the original document")
		}
		got, ok := c.Load(doc.Path, fi)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatal("compacted document changed after round trip")
		}
	}
}

func TestDecompressionBounds(t *testing.T) {
	raw := bytes.Repeat([]byte("synthetic"), 1024)
	payload, err := compress(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []uint64{0, 1, uint64(len(raw) - 1), uint64(len(raw) + 1), maxPayload + 1, ^uint64(0)} {
		if _, err := decompress(payload, size); err == nil {
			t.Fatalf("accepted decoded size %d", size)
		}
	}
	if _, err := decompress(payload[:len(payload)-1], uint64(len(raw))); err == nil {
		t.Fatal("accepted truncated compressed payload")
	}
	if _, err := decompress(append(bytes.Clone(payload), 1), uint64(len(raw))); err == nil {
		t.Fatal("accepted trailing compressed payload")
	}
	got, err := decompress(payload, uint64(len(raw)))
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("decompression did not recover after invalid entries")
	}
}

func TestOldFormatIsMiss(t *testing.T) {
	c := testCache(t)
	doc, fi := testDocument(t, filepath.Join(t.TempDir(), "note.md"), synthetic)
	if err := c.Store(doc); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(c.slot(doc.Path))
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(data[len(magic):], 1)
	if err := os.WriteFile(c.slot(doc.Path), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Load(doc.Path, fi); ok {
		t.Fatal("accepted old cache format")
	}
}

func TestStatsDefault(t *testing.T) {
	c := testCache(t)
	doc, _ := testDocument(t, filepath.Join(t.TempDir(), "note.md"), synthetic)
	if err := c.Store(doc); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(c.slot(doc.Path))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{tempPrefix + "123", "unrelated.gob"} {
		if err := os.WriteFile(filepath.Join(c.dir, name), []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(c.slot(doc.Path), filepath.Join(c.dir, strings.Repeat("a", 64)+".gob")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(c.dir, strings.Repeat("b", 64)+".gob"), 0700); err != nil {
		t.Fatal(err)
	}
	stats, err := StatsDefault()
	if err != nil || stats.Entries != 1 || stats.Bytes != fi.Size() {
		t.Fatalf("stats=%+v, error=%v", stats, err)
	}
	if err := os.Chmod(c.slot(doc.Path), 0644); err != nil {
		t.Fatal(err)
	}
	stats, err = StatsDefault()
	if err != nil || stats != (Stats{}) {
		t.Fatalf("counted unsafe entry: %+v, %v", stats, err)
	}
	if err := os.Chmod(c.dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := StatsDefault(); err == nil {
		t.Fatal("accepted unsafe directory")
	}
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "missing"))
	stats, err = StatsDefault()
	if err != nil || stats != (Stats{}) {
		t.Fatalf("missing cache: %+v, %v", stats, err)
	}
	if _, err := os.Stat(os.Getenv("XDG_CACHE_HOME")); !os.IsNotExist(err) {
		t.Fatal("stats created cache directory")
	}
}
