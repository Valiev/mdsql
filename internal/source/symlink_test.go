package source

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func sourceLink(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func sourceNote(t *testing.T, root, name string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("# Note\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveSymlinkSourcesPreservesLogicalPaths(t *testing.T) {
	root := t.TempDir()
	sourceNote(t, root, "target/note.md")
	sourceNote(t, root, "target/deep/other.markdown")
	sourceNote(t, root, "target/.hidden.md")
	sourceNote(t, root, "target/.private/hidden.md")
	sourceLink(t, "target/note.md", filepath.Join(root, "alias.md"))
	sourceLink(t, "target", filepath.Join(root, "linked"))
	for _, tc := range []struct {
		specs   []string
		include []string
		exclude []string
		want    []string
	}{
		{specs: []string{"alias.md"}, want: []string{"alias.md"}},
		{specs: []string{"linked"}, want: []string{"linked/deep/other.markdown", "linked/note.md"}},
		{specs: []string{"linked/**/*.md"}, want: []string{"linked/note.md"}},
		{specs: []string{"lin*"}, want: []string{"linked/deep/other.markdown", "linked/note.md"}},
		{specs: []string{"li*/**/*.md"}, want: []string{"linked/note.md"}},
		{specs: []string{"alias.md", "target/note.md", "alias.md"}, want: []string{"alias.md", "target/note.md"}},
		{specs: []string{"."}, include: []string{"linked/**"}, exclude: []string{"linked/deep/**"}, want: []string{"linked/note.md"}},
	} {
		got, stdin, err := ResolveSpecs(tc.specs, root, tc.include, tc.exclude)
		want := make([]string, len(tc.want))
		for i, name := range tc.want {
			want[i] = filepath.Join(root, filepath.FromSlash(name))
		}
		sort.Strings(want)
		if err != nil || stdin || !reflect.DeepEqual(got, want) {
			t.Errorf("specs %v: got %v stdin=%v err=%v; want %v", tc.specs, got, stdin, err, want)
		}
	}
}

func TestResolveExplicitHiddenParentAndHiddenFile(t *testing.T) {
	root := t.TempDir()
	sourceNote(t, root, "checkout/note.md")
	sourceNote(t, root, "checkout/.hidden.md")
	if err := os.MkdirAll(filepath.Join(root, ".claude", "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	sourceLink(t, filepath.Join(root, "checkout"), filepath.Join(root, ".claude", "skills", "mdsql"))
	for _, tc := range []struct {
		spec string
		want string
	}{
		{".claude/skills/mdsql", ".claude/skills/mdsql/note.md"},
		{".claude/skills/mdsql/*.md", ".claude/skills/mdsql/note.md"},
		{"checkout/.hidden.md", "checkout/.hidden.md"},
	} {
		got, _, err := ResolveSpecs([]string{tc.spec}, root, nil, nil)
		want := []string{filepath.Join(root, filepath.FromSlash(tc.want))}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("spec %s: got %v %v; want %v", tc.spec, got, err, want)
		}
	}
}

func TestResolveSymlinkCyclesAndDanglingLinksReturnPartialResults(t *testing.T) {
	root := t.TempDir()
	sourceNote(t, root, "notes/good.md")
	sourceNote(t, root, "notes/nested/also.md")
	sourceLink(t, ".", filepath.Join(root, "notes", "self"))
	sourceLink(t, "..", filepath.Join(root, "notes", "nested", "up"))
	sourceLink(t, "missing.md", filepath.Join(root, "notes", "dangling.md"))
	sourceLink(t, "loop.md", filepath.Join(root, "notes", "loop.md"))
	for _, spec := range []string{"notes", "notes/**", "notes/**/*.md"} {
		got, _, err := ResolveSpecs([]string{spec}, root, nil, nil)
		want := []string{filepath.Join(root, "notes", "good.md"), filepath.Join(root, "notes", "nested", "also.md")}
		if !reflect.DeepEqual(got, want) || err == nil || !strings.Contains(err.Error(), "symlink cycle skipped") || !strings.Contains(err.Error(), "dangling.md") {
			t.Errorf("spec %s: got %v %v; want %v and warnings", spec, got, err, want)
		}
	}
	got, stdin, err := ResolveSpecs([]string{"notes/dangling.md", "notes/good.md", "-"}, root, nil, nil)
	if len(got) != 1 || !stdin || err == nil || !strings.Contains(err.Error(), "dangling.md") {
		t.Fatalf("partial explicit sources: %v %v %v", got, stdin, err)
	}
}
