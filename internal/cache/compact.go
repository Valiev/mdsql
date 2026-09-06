package cache

import (
	"bytes"
	"compress/flate"
	"errors"
	"io"
	"sync"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/parser"
)

var compressors = sync.Pool{New: func() interface{} {
	w, _ := flate.NewWriter(io.Discard, flate.BestSpeed)
	return w
}}

var decompressors = sync.Pool{New: func() interface{} {
	return flate.NewReader(bytes.NewReader(nil))
}}

func compress(raw []byte) ([]byte, error) {
	var out limitedBuffer
	w := compressors.Get().(*flate.Writer)
	defer compressors.Put(w)
	w.Reset(&out)
	if _, err := w.Write(raw); err != nil {
		w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func decompress(payload []byte, size uint64) ([]byte, error) {
	if size > maxPayload {
		return nil, errors.New("decoded cache entry exceeds size limit")
	}
	r := decompressors.Get().(io.ReadCloser)
	defer decompressors.Put(r)
	defer r.Close()
	source := bytes.NewReader(payload)
	if err := r.(flate.Resetter).Reset(source, nil); err != nil {
		return nil, err
	}
	raw := make([]byte, int(size))
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, err
	}
	var extra [1]byte
	if n, err := r.Read(extra[:]); n != 0 || err != io.EOF {
		return nil, errors.New("decoded cache entry length mismatch")
	}
	if source.Len() != 0 {
		return nil, errors.New("trailing data after compressed content")
	}
	return raw, nil
}

// Copy every slice we change: Store must not rewrite the caller's document.
// These strings are reproduced exactly from the document path and content.
func compactDocument(doc *model.Document) model.Document {
	out := *doc
	out.Lines = nil
	out.Headings = append([]model.Heading(nil), doc.Headings...)
	out.Sections = append([]model.Section(nil), doc.Sections...)
	out.Blocks = append([]model.Block(nil), doc.Blocks...)
	out.Tasks = append([]model.Task(nil), doc.Tasks...)
	out.Links = append([]model.Link(nil), doc.Links...)
	out.Tags = append([]model.Tag(nil), doc.Tags...)
	out.Quotes = append([]model.Quote(nil), doc.Quotes...)
	out.LinesRows = append([]model.Line(nil), doc.LinesRows...)
	for i := range out.Headings {
		out.Headings[i].File = ""
	}
	for i := range out.Sections {
		out.Sections[i].File = ""
	}
	for i := range out.Blocks {
		out.Blocks[i].File = ""
	}
	for i := range out.Tasks {
		out.Tasks[i].File = ""
	}
	for i := range out.Links {
		out.Links[i].File = ""
	}
	for i := range out.Tags {
		out.Tags[i].File = ""
	}
	for i := range out.Quotes {
		out.Quotes[i].File = ""
	}
	for i := range out.LinesRows {
		out.LinesRows[i].File = ""
		out.LinesRows[i].Text = ""
	}
	return out
}

func restoreDocument(doc *model.Document) bool {
	doc.Lines = parser.SourceLines(doc.Content)
	if len(doc.LinesRows) != len(doc.Lines) {
		return false
	}
	for i := range doc.Headings {
		doc.Headings[i].File = doc.Path
	}
	for i := range doc.Sections {
		doc.Sections[i].File = doc.Path
	}
	for i := range doc.Blocks {
		doc.Blocks[i].File = doc.Path
	}
	for i := range doc.Tasks {
		doc.Tasks[i].File = doc.Path
	}
	for i := range doc.Links {
		doc.Links[i].File = doc.Path
	}
	for i := range doc.Tags {
		doc.Tags[i].File = doc.Path
	}
	for i := range doc.Quotes {
		doc.Quotes[i].File = doc.Path
	}
	for i := range doc.LinesRows {
		if doc.LinesRows[i].N != i+1 {
			return false
		}
		doc.LinesRows[i].File = doc.Path
		doc.LinesRows[i].Text = doc.Lines[i]
	}
	return true
}
