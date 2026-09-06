package cache

import (
	"strings"

	"github.com/Valiev/mdsql/internal/model"
)

// A string is stored once, using a source range whenever it occurs in Content.
// Start == -1 identifies a derived string that needs its own Value.
type cachedString struct {
	Start int
	End   int
	Value string
}

func compactStrings(doc *model.Document) ([]cachedString, []uint32) {
	var values []cachedString
	var refs []uint32
	seen := make(map[string]uint32)
	ranges := sourceRanges(doc)
	visitStrings(doc, func(value *string) {
		index, ok := seen[*value]
		if !ok {
			index = uint32(len(values))
			seen[*value] = index
			cached := cachedString{Start: -1, Value: *value}
			if source, ok := ranges[*value]; ok {
				cached = source
			}
			values = append(values, cached)
		}
		refs = append(refs, index)
		*value = ""
	})
	return values, refs
}

// Look only inside known line/block/section spans, avoiding a full-document
// substring scan for every heading, task, and derived string.
func sourceRanges(doc *model.Document) map[string]cachedString {
	ranges := make(map[string]cachedString)
	lines := strings.Split(doc.Content, "\n")
	offsets := make([]int, len(lines)+1)
	for i, line := range lines {
		offsets[i+1] = offsets[i] + len(line) + 1
	}
	add := func(value string, from, to int) {
		if value == "" || from < 1 || to < from || to > len(lines) {
			return
		}
		if _, ok := ranges[value]; ok {
			return
		}
		start, end := offsets[from-1], offsets[to]-1
		if found := strings.Index(doc.Content[start:end], value); found >= 0 {
			start += found
			ranges[value] = cachedString{Start: start, End: start + len(value)}
		}
	}
	for i, line := range lines {
		add(line, i+1, i+1)
		add(strings.TrimSpace(line), i+1, i+1)
	}
	add(doc.FrontMatterRaw, doc.FrontMatterFrom+1, doc.FrontMatterTo-1)
	for _, h := range doc.Headings {
		for _, value := range []string{h.Raw, h.Title, h.Text} {
			add(value, h.Start, h.End)
		}
	}
	for _, s := range doc.Sections {
		for _, value := range []string{s.Raw, s.Body, s.Own} {
			add(value, s.Start, s.End)
		}
	}
	for _, b := range doc.Blocks {
		add(b.Raw, b.Start, b.End)
		add(b.Text, b.Start, b.End)
	}
	for _, task := range doc.Tasks {
		add(task.Raw, task.Start, task.End)
		add(task.Text, task.Start, task.End)
		add(task.Block.Text, task.Start, task.End)
	}
	return ranges
}

func restoreStrings(doc *model.Document, cached []cachedString, refs []uint32) bool {
	values := make([]string, len(cached))
	for i, value := range cached {
		if value.Start == -1 {
			values[i] = value.Value
		} else {
			if value.Start < 0 || value.End < value.Start || value.End > len(doc.Content) {
				return false
			}
			values[i] = doc.Content[value.Start:value.End]
		}
	}
	index := 0
	valid := true
	visitStrings(doc, func(value *string) {
		if index >= len(refs) || uint64(refs[index]) >= uint64(len(values)) {
			valid = false
			return
		}
		*value = values[refs[index]]
		index++
	})
	return valid && index == len(refs)
}

// The order is part of formatVersion. Only copied scalar fields are visited;
// YAML values and nested task slices retain their original typed gob encoding.
func visitStrings(doc *model.Document, visit func(*string)) {
	visit(&doc.FrontMatterRaw)
	for i := range doc.Headings {
		h := &doc.Headings[i]
		for _, value := range []*string{&h.Title, &h.Text, &h.Slug, &h.Section, &h.Path, &h.Parent, &h.Raw} {
			visit(value)
		}
	}
	for i := range doc.Sections {
		s := &doc.Sections[i]
		for _, value := range []*string{&s.Title, &s.Text, &s.Slug, &s.Path, &s.Parent, &s.Raw, &s.Body, &s.Own} {
			visit(value)
		}
	}
	for i := range doc.Blocks {
		visitBlockStrings(&doc.Blocks[i], visit)
	}
	for i := range doc.Tasks {
		t := &doc.Tasks[i]
		visitBlockStrings(&t.Block, visit)
		for _, value := range []*string{&t.Text, &t.Status, &t.State, &t.Recurrence, &t.ID, &t.PriorityName, &t.Section, &t.SectionPath} {
			visit(value)
		}
	}
	for i := range doc.Links {
		l := &doc.Links[i]
		for _, value := range []*string{&l.Type, &l.Target, &l.Anchor, &l.BlockRef, &l.Text, &l.Raw, &l.Section, &l.Path} {
			visit(value)
		}
	}
	for i := range doc.Tags {
		t := &doc.Tags[i]
		for _, value := range []*string{&t.Tag, &t.Source, &t.Section, &t.Path, &t.BlockKind} {
			visit(value)
		}
	}
	for i := range doc.Quotes {
		q := &doc.Quotes[i]
		for _, value := range []*string{&q.Type, &q.Title, &q.Fold, &q.Body, &q.Text, &q.Path, &q.Section} {
			visit(value)
		}
	}
	for i := range doc.LinesRows {
		l := &doc.LinesRows[i]
		for _, value := range []*string{&l.Kind, &l.Section, &l.Path} {
			visit(value)
		}
	}
}

func visitBlockStrings(b *model.Block, visit func(*string)) {
	for _, value := range []*string{&b.Kind, &b.Section, &b.Path, &b.Text, &b.Raw, &b.Lang, &b.Info, &b.Fence} {
		visit(value)
	}
}
