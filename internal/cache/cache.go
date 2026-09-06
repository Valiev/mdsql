// Package cache stores private, disposable parsed Markdown documents.
package cache

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/parser"
)

const (
	magic         = "MDSQL\x00\x00\n"
	formatVersion = 2
	headerSize    = len(magic) + 4 + 8 + sha256.Size
	maxPayload    = 64 << 20
	tempPrefix    = ".mdsql-tmp-"
)

func init() {
	gob.Register(model.FrontMatter{})
	gob.Register(map[string]interface{}{})
	gob.Register(map[interface{}]interface{}{})
	gob.Register([]interface{}{})
	gob.Register(time.Time{})
}

// Cache is safe for concurrent Load and Store calls.
type Cache struct {
	dir string
}

type entry struct {
	Path          string
	Size          int64
	ModTime       int64
	ParserVersion string
	Document      model.Document
	Strings       []cachedString
	TextRefs      []uint32
	Content       []byte
	ContentSize   uint64
}

// OpenDefault opens the user's private cache. Errors should disable caching,
// never prevent an ordinary query from reading its source files.
func OpenDefault() (*Cache, error) {
	dir, err := defaultDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	c := &Cache{dir: dir}
	if err := c.checkDir(); err != nil {
		return nil, err
	}
	return c, nil
}

func defaultDir() (string, error) {
	if runtime.GOOS == "windows" {
		return "", errors.New("private Markdown cache is unsupported on Windows")
	}
	base := os.Getenv("XDG_CACHE_HOME")
	if base != "" && !filepath.IsAbs(base) {
		return "", errors.New("XDG_CACHE_HOME must be absolute")
	}
	if base == "" {
		var err error
		base, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
	}
	if !filepath.IsAbs(base) {
		return "", errors.New("cache directory must be absolute")
	}
	return filepath.Join(base, "mdsql"), nil
}

func (c *Cache) checkDir() error {
	fi, err := os.Lstat(c.dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode().Perm() != 0700 || !owned(fi) {
		return fmt.Errorf("cache directory must be owned by the current user with mode 0700: %s", c.dir)
	}
	return nil
}

func (c *Cache) slot(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])+".gob")
}

// Load returns a fresh document, or a miss for any stale, unsafe, or corrupt
// entry. The caller must first open and stat the source, including on a hit.
func (c *Cache) Load(path string, fi os.FileInfo) (*model.Document, bool) {
	if !filepath.IsAbs(path) || fi == nil || !fi.Mode().IsRegular() || c.checkDir() != nil {
		return nil, false
	}
	path = filepath.Clean(path)
	name := c.slot(path)
	cachedInfo, err := os.Lstat(name)
	if err != nil || !privateFile(cachedInfo) || cachedInfo.Size() < int64(headerSize) || cachedInfo.Size() > int64(headerSize+maxPayload) {
		return nil, false
	}
	f, err := openEntry(name)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	openedInfo, err := f.Stat()
	if err != nil || !privateFile(openedInfo) || !os.SameFile(cachedInfo, openedInfo) {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(headerSize+maxPayload+1)))
	if err != nil || len(data) < headerSize || len(data) > headerSize+maxPayload {
		return nil, false
	}
	if string(data[:len(magic)]) != magic || binary.LittleEndian.Uint32(data[len(magic):]) != formatVersion {
		return nil, false
	}
	if binary.LittleEndian.Uint64(data[len(magic)+4:]) != uint64(len(data)-headerSize) {
		return nil, false
	}
	sum := sha256.Sum256(data[headerSize:])
	if !bytes.Equal(sum[:], data[headerSize-sha256.Size:headerSize]) {
		return nil, false
	}
	var e entry
	if err := gob.NewDecoder(bytes.NewReader(data[headerSize:])).Decode(&e); err != nil {
		return nil, false
	}
	if e.Path != path || e.ParserVersion != parser.Version || e.Size != fi.Size() || e.ModTime != fi.ModTime().UnixNano() {
		return nil, false
	}
	doc := &e.Document
	if doc.Path != path || doc.RelPath != "" || doc.Size != e.Size || doc.ModTime.UnixNano() != e.ModTime {
		return nil, false
	}
	content, err := decompress(e.Content, e.ContentSize)
	if err != nil {
		return nil, false
	}
	doc.Content = string(content)
	if !restoreStrings(doc, e.Strings, e.TextRefs) || !restoreDocument(doc) {
		return nil, false
	}
	restoreCollections(doc)
	return doc, true
}

// Store saves an absolute parser result before query-specific path rewriting.
// The source's metadata must still match the parsed document.
func (c *Cache) Store(doc *model.Document) error {
	if doc == nil || !filepath.IsAbs(doc.Path) || doc.Path != filepath.Clean(doc.Path) || doc.RelPath != "" {
		return errors.New("cache requires an absolute, unmodified parser document")
	}
	if len(doc.Content) > maxPayload {
		return errors.New("source content exceeds cache entry size limit")
	}
	fi, err := os.Stat(doc.Path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() || fi.Size() != doc.Size || fi.ModTime().UnixNano() != doc.ModTime.UnixNano() {
		return errors.New("source changed before caching")
	}
	if err := c.checkDir(); err != nil {
		return err
	}
	var raw limitedBuffer
	e := entry{Path: doc.Path, Size: doc.Size, ModTime: doc.ModTime.UnixNano(), ParserVersion: parser.Version, Document: compactDocument(doc)}
	e.Strings, e.TextRefs = compactStrings(&e.Document)
	e.ContentSize = uint64(len(doc.Content))
	e.Content, err = compress([]byte(doc.Content))
	if err != nil {
		return err
	}
	e.Document.Content = ""
	if err := gob.NewEncoder(&raw).Encode(e); err != nil {
		return err
	}
	payload := raw.Bytes()
	header := make([]byte, headerSize)
	copy(header, magic)
	binary.LittleEndian.PutUint32(header[len(magic):], formatVersion)
	binary.LittleEndian.PutUint64(header[len(magic)+4:], uint64(len(payload)))
	sum := sha256.Sum256(payload)
	copy(header[headerSize-sha256.Size:], sum[:])
	f, err := os.CreateTemp(c.dir, tempPrefix)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(header); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(payload); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := c.checkDir(); err != nil {
		return err
	}
	// Rename replaces the directory entry itself, without following a symlink.
	return os.Rename(f.Name(), c.slot(doc.Path))
}

type limitedBuffer struct {
	bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > maxPayload-b.Len() {
		return 0, errors.New("parsed document exceeds cache entry size limit")
	}
	return b.Buffer.Write(p)
}

func privateFile(fi os.FileInfo) bool {
	return fi.Mode().IsRegular() && fi.Mode().Perm() == 0600 && owned(fi)
}

// ClearDefault removes only this cache's entry and temporary files. It neither
// creates a missing cache nor traverses directories or symlinks.
func ClearDefault() error {
	dir, err := defaultDir()
	if err != nil {
		return err
	}
	c := &Cache{dir: dir}
	if err := c.checkDir(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !cacheName(e.Name()) || !e.Type().IsRegular() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if !owned(fi) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func cacheName(name string) bool {
	if strings.HasPrefix(name, tempPrefix) {
		suffix := strings.TrimPrefix(name, tempPrefix)
		if suffix == "" {
			return false
		}
		for _, r := range suffix {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	if len(name) != sha256.Size*2+len(".gob") || !strings.HasSuffix(name, ".gob") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimSuffix(name, ".gob"))
	return err == nil
}

// Gob collapses empty slices to nil. The parser initializes these collections;
// YAML null is an untyped nil, while an empty YAML list must stay a list.
func restoreCollections(doc *model.Document) {
	if doc.Headings == nil {
		doc.Headings = []model.Heading{}
	}
	if doc.Sections == nil {
		doc.Sections = []model.Section{}
	}
	if doc.Blocks == nil {
		doc.Blocks = []model.Block{}
	}
	if doc.Tasks == nil {
		doc.Tasks = []model.Task{}
	}
	if doc.Links == nil {
		doc.Links = []model.Link{}
	}
	if doc.Tags == nil {
		doc.Tags = []model.Tag{}
	}
	if doc.Quotes == nil {
		doc.Quotes = []model.Quote{}
	}
	if doc.LinesRows == nil {
		doc.LinesRows = []model.Line{}
	}
	for k, v := range doc.FrontMatter {
		doc.FrontMatter[k] = restoreValue(v)
	}
	for i := range doc.Tasks {
		for k, v := range doc.Tasks[i].Fields {
			doc.Tasks[i].Fields[k] = restoreValue(v)
		}
	}
}

func restoreValue(v interface{}) interface{} {
	switch x := v.(type) {
	case []interface{}:
		if x == nil {
			x = []interface{}{}
		}
		for i := range x {
			x[i] = restoreValue(x[i])
		}
		return x
	case model.FrontMatter:
		for k, v := range x {
			x[k] = restoreValue(v)
		}
	case map[string]interface{}:
		for k, v := range x {
			x[k] = restoreValue(v)
		}
	case map[interface{}]interface{}:
		for k, v := range x {
			x[k] = restoreValue(v)
		}
	}
	return v
}
