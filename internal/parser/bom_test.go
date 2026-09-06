package parser

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLeadingBOM(t *testing.T) {
	plain := "---\r\ntitle: Note\r\n---\r\n# Heading\r\nBody."
	doc, err := ParseFile("bom.md", plain, int64(len(plain)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	bom := "\uFEFF" + plain
	withBOM, err := ParseFile("bom.md", bom, int64(len(bom)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if withBOM.Size != int64(len(bom)) {
		t.Fatalf("source size = %d, want %d", withBOM.Size, len(bom))
	}
	withBOM.Size = doc.Size
	if !reflect.DeepEqual(withBOM, doc) {
		t.Fatalf("BOM changed parsed document: %#v", withBOM)
	}
	if withBOM.Headings[0].Start != 4 || strings.HasPrefix(withBOM.Content, "\uFEFF") {
		t.Fatalf("BOM changed line positions or content: %#v", withBOM)
	}
}

func TestOnlyOneLeadingBOMIsRemoved(t *testing.T) {
	for _, tc := range []struct{ raw, content string }{
		{"\uFEFF\uFEFF# Heading", "\uFEFF# Heading"},
		{"Text \uFEFF inside", "Text \uFEFF inside"},
		{"\n\uFEFF# Heading", "\n\uFEFF# Heading"},
	} {
		doc, err := ParseFile("bom.md", tc.raw, int64(len(tc.raw)), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if doc.Content != tc.content {
			t.Errorf("content = %q, want %q", doc.Content, tc.content)
		}
	}
}
