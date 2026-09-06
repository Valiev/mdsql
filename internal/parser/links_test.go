package parser

import (
	"testing"
	"time"
)

func TestLinkOccurrencesDoNotOverlap(t *testing.T) {
	raw := `[md](https://example.test/md) ![image](https://example.test/image) <https://example.test/angle> https://example.test/bare.`
	doc, err := ParseFile("links.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ kind, target, raw string }{
		{"md", "https://example.test/md", "[md](https://example.test/md)"},
		{"image", "https://example.test/image", "![image](https://example.test/image)"},
		{"autolink", "https://example.test/angle", "<https://example.test/angle>"},
		{"autolink", "https://example.test/bare", "https://example.test/bare"},
	}
	if len(doc.Links) != len(want) {
		t.Fatalf("links = %#v", doc.Links)
	}
	for i, link := range doc.Links {
		if link.Type != want[i].kind || link.Target != want[i].target || link.Raw != want[i].raw || !link.External || link.Line != 1 {
			t.Errorf("link %d = %#v, want %#v", i, link, want[i])
		}
	}
}

func TestRepeatedLinkOccurrencesArePreserved(t *testing.T) {
	for _, raw := range []string{
		"https://example.test https://example.test",
		"[one](https://example.test) [two](https://example.test)",
		"<https://example.test> <https://example.test>",
		"[one](https://example.test) https://example.test",
	} {
		t.Run(raw, func(t *testing.T) {
			doc, err := ParseFile("links.md", raw, int64(len(raw)), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if len(doc.Links) != 2 || doc.Links[0].Target != "https://example.test" || doc.Links[1].Target != "https://example.test" {
				t.Fatalf("links = %#v", doc.Links)
			}
		})
	}
}

func TestBareURLPunctuation(t *testing.T) {
	for _, tc := range []struct{ text, target string }{
		{"http://example.test/y.", "http://example.test/y"},
		{"https://example.test/y,", "https://example.test/y"},
		{"https://example.test/y;", "https://example.test/y"},
		{"(https://example.test/y)", "https://example.test/y"},
		{"(https://example.test/y).,;", "https://example.test/y"},
		{"https://example.test/a_(b)", "https://example.test/a_(b)"},
		{"(https://example.test/a_(b)).", "https://example.test/a_(b)"},
		{"https://example.test/a_(b(c))).", "https://example.test/a_(b(c))"},
		{"https://example.test/a.b,c;d?x=1", "https://example.test/a.b,c;d?x=1"},
		{"https://example.test/a%29.", "https://example.test/a%29"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			doc, err := ParseFile("links.md", tc.text, int64(len(tc.text)), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if len(doc.Links) != 1 || doc.Links[0].Target != tc.target || doc.Links[0].Raw != tc.target {
				t.Fatalf("links = %#v, want target %q", doc.Links, tc.target)
			}
		})
	}
}

func TestURLsInsideLinkLabelsAreNotSeparateOccurrences(t *testing.T) {
	raw := `[https://label.test](https://target.test "https://title.test") [[Note|https://wiki-label.test]]`
	doc, err := ParseFile("links.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Links) != 2 || doc.Links[0].Type != "wiki" || doc.Links[1].Type != "md" || doc.Links[1].Target != "https://target.test" {
		t.Fatalf("links = %#v", doc.Links)
	}
}
