package source

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestIncludeExcludeUseSameGlobRules(t *testing.T) {
	root := t.TempDir()
	paths := []string{"root.md", "nested/note.md", "Templates/direct.md", "Templates/deep/note.md"}
	for _, name := range paths {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# Note\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		pattern string
		want    []string
	}{
		{"*.md", []string{"root.md"}},
		{"**/*.md", paths},
		{"Templates/*", []string{"Templates/direct.md"}},
		{"Templates/**", []string{"Templates/direct.md", "Templates/deep/note.md"}},
	} {
		included, _, err := ResolveSpecs([]string{"."}, root, []string{tc.pattern}, nil)
		if err != nil {
			t.Fatal(err)
		}
		excluded, _, err := ResolveSpecs([]string{"."}, root, nil, []string{tc.pattern})
		if err != nil {
			t.Fatal(err)
		}
		want, complement := []string{}, []string{}
		for _, name := range paths {
			match := false
			for _, w := range tc.want {
				if w == name {
					match = true
				}
			}
			if match {
				want = append(want, filepath.Join(root, filepath.FromSlash(name)))
			} else {
				complement = append(complement, filepath.Join(root, filepath.FromSlash(name)))
			}
		}
		sort.Strings(want)
		sort.Strings(complement)
		if !reflect.DeepEqual(included, want) || !reflect.DeepEqual(excluded, complement) {
			t.Fatalf("pattern %s included=%v excluded=%v", tc.pattern, included, excluded)
		}
	}
}

func TestMarkdownExtensionGlobsAreCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"lower.md", "upper.MD", "long.MarkDown", "skip.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("# Note\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	got, _, err := ResolveSpecs([]string{"*.md", "*.markdown"}, root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "long.MarkDown"), filepath.Join(root, "lower.md"), filepath.Join(root, "upper.MD")}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("glob files = %v, want %v", got, want)
	}

	got, _, err = ResolveSpecs([]string{"."}, root, []string{"**/*.md", "**/*.markdown"}, []string{"**/upper.md"})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{filepath.Join(root, "long.MarkDown"), filepath.Join(root, "lower.md")}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("include/exclude files = %v, want %v", got, want)
	}
}

func TestResolveExistingLiteralBeforeGlob(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"[draft].md", "d.md", "[unfinished.md", "literal*.md", "literal-other.md",
		"normal/a.md", "normal/b.md", "[notes]/keep.md", "[notes]/skip.md", "n/decoy.md",
		"cafe\u0301[draft].md", "cafe\u0301[notes]/nested.md",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# Synthetic\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		spec    string
		include []string
		exclude []string
		want    []string
	}{
		{spec: "[draft].md", want: []string{"[draft].md"}},
		{spec: filepath.Join(root, "[draft].md"), want: []string{"[draft].md"}},
		{spec: "[unfinished.md", want: []string{"[unfinished.md"}},
		{spec: "literal*.md", want: []string{"literal*.md"}},
		{spec: "[notes]", want: []string{"[notes]/keep.md", "[notes]/skip.md"}},
		{spec: "caf\u00e9[draft].md", want: []string{"cafe\u0301[draft].md"}},
		{spec: "caf\u00e9[notes]", want: []string{"cafe\u0301[notes]/nested.md"}},
		{spec: "normal/*.md", want: []string{"normal/a.md", "normal/b.md"}},
		{spec: "missing*.md"},
		{spec: "[draft].md", include: []string{"*draft*.md"}, want: []string{"[draft].md"}},
		{spec: "[draft].md", exclude: []string{"*draft*.md"}},
		{spec: "[notes]", include: []string{"**/keep.md"}, want: []string{"[notes]/keep.md"}},
		{spec: "[notes]", exclude: []string{"**/skip.md"}, want: []string{"[notes]/keep.md"}},
		{spec: "normal/*.md", exclude: []string{"**/a.md"}, want: []string{"normal/b.md"}},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			want := make([]string, 0, len(tc.want))
			for _, name := range tc.want {
				physical, err := ResolvePath(filepath.Join(root, name))
				if err != nil {
					t.Fatal(err)
				}
				want = append(want, physical)
			}
			sort.Strings(want)
			got, stdin, err := ResolveSpecs([]string{tc.spec}, root, tc.include, tc.exclude)
			if err != nil || stdin || !reflect.DeepEqual(got, want) {
				t.Fatalf("got=%q stdin=%v error=%v want=%q", got, stdin, err, want)
			}
		})
	}
	if _, _, err := ResolveSpecs([]string{"missing.md"}, root, nil, nil); err == nil {
		t.Fatal("missing literal must still report an error")
	}
	if _, _, err := ResolveSpecs([]string{"[missing"}, root, nil, nil); err == nil {
		t.Fatal("nonexistent invalid pattern must still report an error")
	}
}
