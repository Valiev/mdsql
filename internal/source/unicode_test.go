package source

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	"golang.org/x/text/unicode/norm"
)

func TestResolveUnicodePathOnByteExactFilesystem(t *testing.T) {
	nfcDir, nfcFile := "Йога", "Café.md"
	nfdDir, nfdFile := norm.NFD.String(nfcDir), norm.NFD.String(nfcFile)
	physical := nfdDir + "/" + nfdFile
	fsys := fstest.MapFS{physical: &fstest.MapFile{Data: []byte("# Unicode\n")}}
	for _, input := range []string{nfcDir + "/" + nfcFile, physical, nfcDir + "/" + nfdFile, nfdDir + "/" + nfcFile} {
		got, err := resolveUnicodePath(fsys, input)
		if err != nil || got != physical {
			t.Errorf("%q: physical=%q err=%v; want %q", input, got, err, physical)
		}
	}
	fsys[nfcDir+"/"+nfcFile] = &fstest.MapFile{Data: []byte("# Separate file\n")}
	for _, input := range []string{nfcDir + "/" + nfcFile, physical} {
		got, err := resolveUnicodePath(fsys, input)
		if err != nil || got != input {
			t.Errorf("exact spelling %q: physical=%q err=%v", input, got, err)
		}
	}
	if _, err := resolveUnicodePath(fsys, nfcDir+"/missing.md"); err == nil {
		t.Fatal("missing source should return an error")
	}
}

func TestResolveUnicodeSpecsKeepsPhysicalNames(t *testing.T) {
	root := t.TempDir()
	nfcDir, nfcFile := "Мой архив", "Йога.md"
	nfdDir, nfdFile := norm.NFD.String(nfcDir), norm.NFD.String(nfcFile)
	sourceNote(t, root, nfdDir+"/"+nfdFile)
	physical, err := ResolvePath(filepath.Join(root, nfdDir, nfdFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		spec    string
		include []string
		exclude []string
		want    []string
	}{
		{spec: nfcDir + "/" + nfcFile, want: []string{physical}},
		{spec: nfdDir + "/" + nfdFile, want: []string{physical}},
		{spec: nfcDir, want: []string{physical}},
		{spec: nfcDir + "/Йо*.md", want: []string{physical}},
		{spec: "**/Йо*.md", want: []string{physical}},
		{spec: "**/" + norm.NFD.String("Йо") + "*.md", want: []string{physical}},
		{spec: ".", include: []string{"**/Йога.md"}, want: []string{physical}},
		{spec: ".", include: []string{"**/" + nfdFile}, want: []string{physical}},
		{spec: ".", exclude: []string{"**/Йога.md"}, want: []string{}},
		{spec: ".", exclude: []string{"**/" + nfdFile}, want: []string{}},
	} {
		got, stdin, err := ResolveSpecs([]string{tc.spec}, root, tc.include, tc.exclude)
		if err != nil || stdin || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%+v: physical=%q stdin=%v err=%v", tc, got, stdin, err)
		}
		for _, file := range got {
			if data, err := os.ReadFile(file); err != nil || string(data) != "# Note\n" {
				t.Fatalf("physical path unreadable: %q %q %v", file, data, err)
			}
		}
	}
	got, _, err := ResolveSpecs([]string{".", nfcFile, nfdFile}, filepath.Join(root, nfcDir), nil, nil)
	if err != nil || !reflect.DeepEqual(got, []string{physical}) {
		t.Errorf("Unicode root and equivalent specs dedup: %q %v", got, err)
	}
}

func TestUnicodeSymlinkAliasAndCycle(t *testing.T) {
	root := t.TempDir()
	nfcAlias := "Связь-Йога"
	nfdAlias := norm.NFD.String(nfcAlias)
	sourceNote(t, root, "notes/"+norm.NFD.String("Йога.md"))
	sourceLink(t, "notes", filepath.Join(root, nfdAlias))
	sourceLink(t, ".", filepath.Join(root, "notes", "cycle"))
	physical, err := ResolvePath(filepath.Join(root, nfdAlias, norm.NFD.String("Йога.md")))
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := ResolveSpecs([]string{nfcAlias + "/**/*.md"}, root, nil, nil)
	if err == nil || !reflect.DeepEqual(got, []string{physical}) {
		t.Fatalf("Unicode symlink with cycle: %q %v", got, err)
	}
	if NormalizePath(got[0]) != filepath.Join(root, nfcAlias, "Йога.md") {
		t.Fatalf("alias changed: %q", got[0])
	}
}

func TestDistinctCanonicalNamesAreNotMerged(t *testing.T) {
	root := t.TempDir()
	names := []string{"Йога.md", norm.NFD.String("Йога.md")}
	for _, name := range names {
		file, err := os.OpenFile(filepath.Join(root, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if os.IsExist(err) {
			t.Skip("filesystem treats canonical spellings as the same filename")
		}
		if err != nil {
			t.Fatal(err)
		}
		file.Close()
	}
	for _, spec := range []string{".", "Йо*.md"} {
		got, _, err := ResolveSpecs([]string{spec}, root, nil, nil)
		if err != nil || len(got) != 2 || got[0] == got[1] {
			t.Fatalf("spec %q merged distinct physical files: %q %v", spec, got, err)
		}
	}
}
