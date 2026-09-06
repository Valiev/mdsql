package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Valiev/mdsql/internal/cache"
	"github.com/Valiev/mdsql/internal/parser"
)

func loadDocument(path string, maxSize int64, parsedCache *cache.Cache, warn func(error)) (*parser.Document, error) {
	for attempt := 0; ; attempt++ {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		if fi.Size() > maxSize {
			f.Close()
			return nil, fmt.Errorf("file too large: %s (%d bytes)", path, fi.Size())
		}
		if parsedCache != nil {
			if doc, hit := parsedCache.Load(path, fi); hit && sourceUnchanged(path, f, fi) {
				f.Close()
				return doc, nil
			}
		}
		data, err := io.ReadAll(f)
		stable := parsedCache != nil && int64(len(data)) == fi.Size() && sourceUnchanged(path, f, fi)
		f.Close()
		if err != nil {
			return nil, err
		}
		if parsedCache != nil && !stable && attempt == 0 {
			continue
		}
		doc, err := parser.ParseFile(path, string(data), fi.Size(), fi.ModTime())
		if err != nil {
			return nil, err
		}
		if stable {
			if err := parsedCache.Store(doc); err != nil {
				warn(err)
			}
		}
		return doc, nil
	}
}

func sourceUnchanged(path string, file *os.File, before os.FileInfo) bool {
	after, err := file.Stat()
	if err != nil {
		return false
	}
	current, err := os.Stat(path)
	return err == nil && os.SameFile(before, current) &&
		before.Size() == after.Size() && before.Size() == current.Size() &&
		before.ModTime().Equal(after.ModTime()) && before.ModTime().Equal(current.ModTime())
}
