package output

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/query"
)

func TestGroupedPreservesGroupAndRowOrder(t *testing.T) {
	rows := []query.Row{
		{"file": query.NewString("b.md"), "line": query.NewInt(3), "text": query.NewString("first")},
		{"file": query.NewString("a.md"), "line": query.NewInt(1), "text": query.NewString("second")},
		{"file": query.NewString("b.md"), "line": query.NewInt(2), "text": query.NewString("third")},
	}
	var out bytes.Buffer
	if err := Render(&out, rows, FormatGrouped, 0, false, false, "", []string{"text", "file", "line"}); err != nil {
		t.Fatal(err)
	}
	want := "text\tline\n@\"b.md\"\nfirst\t3\nthird\t2\n@\"a.md\"\nsecond\t1\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
	if rows[0]["file"].String() != "b.md" {
		t.Fatal("render changed an input row")
	}
}

func TestGroupedQuotesFileOnlyHeaders(t *testing.T) {
	path := "a\"b\\c\n\t\r.md"
	rows := []query.Row{{"file": query.NewString(path)}, {"file": query.NewString(path)}}
	for _, selected := range [][]string{{"file"}, {"*"}, nil} {
		var out bytes.Buffer
		if err := Render(&out, rows, FormatGrouped, 0, false, false, "", selected); err != nil {
			t.Fatal(err)
		}
		if want := "@\"a\\\"b\\\\c\\n\\t\\r.md\"\n"; out.String() != want {
			t.Fatalf("got %q, want %q", out.String(), want)
		}
	}
}

func TestGroupedEscapesMultilineValuesAndMaxLines(t *testing.T) {
	rows := []query.Row{{
		"file": query.NewString("note.md"), "line": query.NewInt(7),
		"raw": query.NewString("raw\t1\nraw2\nraw3"), "body": query.NewString("body1\nbody2"),
	}}
	var out bytes.Buffer
	if err := Render(&out, rows, FormatGrouped, 1, false, false, "", []string{"file", "line", "raw", "body"}); err != nil {
		t.Fatal(err)
	}
	want := "line\traw\tbody\n@\"note.md\"\n7\traw\\t1\\n… (+2 lines)\tbody1\\n… (+1 lines)\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}

func TestGroupedTSVQuotesAndEscapes(t *testing.T) {
	rows := []query.Row{{
		"file": query.NewString("note.md"), "text": query.NewString("commas, \"quotes\"\ttab\nnewline"),
		"line": query.NewInt(7), "__private": query.NewString("hidden"),
	}}
	for _, selected := range [][]string{{"file", "line", "text"}, {"*"}, nil} {
		var out bytes.Buffer
		if err := Render(&out, rows, FormatGrouped, 0, false, false, "", selected); err != nil {
			t.Fatal(err)
		}
		columns, body, _ := strings.Cut(out.String(), "\n")
		header, body, _ := strings.Cut(body, "\n")
		if header != "@\"note.md\"" {
			t.Fatalf("file header=%q", header)
		}
		reader := csv.NewReader(strings.NewReader(columns + "\n" + body))
		reader.Comma = '\t'
		records, err := reader.ReadAll()
		if err != nil || len(records) != 2 {
			t.Fatalf("records=%v error=%v", records, err)
		}
		if len(records[0]) != 2 || records[0][0] != "line" || records[0][1] != "text" {
			t.Fatalf("columns=%v", records[0])
		}
		if records[1][0] != "7" || records[1][1] != "commas, \"quotes\"\\ttab\\nnewline" {
			t.Fatalf("values=%v", records[1])
		}
	}
}

func TestGroupedRootPreservesFilePaths(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		root  string
	}{
		{"absolute", []string{"/tmp/long-project/notes/a.md", "/tmp/long-project/notes/b.md"}, "/tmp/long-project/notes/"},
		{"relative", []string{"shared-project/notes/a.md", "shared-project/notes/b.md"}, "shared-project/notes/"},
		{"unicode", []string{"заметки/проект/первая.md", "заметки/проект/вторая.md"}, "заметки/проект/"},
		{"escaped", []string{"notes/long\n\t\"folder/a.md", "notes/long\n\t\"folder/b\n.md"}, "notes/long\n\t\"folder/"},
		{"different directories", []string{"/alpha/a.md", "/beta/b.md"}, ""},
		{"mixed paths", []string{"/notes/a.md", "notes/b.md"}, ""},
		{"single file", []string{"/long-project/notes/a.md"}, ""},
		{"single repeated file", []string{"/long-project/notes/a.md", "/long-project/notes/a.md"}, ""},
		{"small prefix", []string{"a/one.md", "a/two.md"}, ""},
		{"filename prefix", []string{"same-long-filename-one.md", "same-long-filename-two.md"}, ""},
		{"preserve dot segments", []string{"notes/../long-project/a.md", "notes/../long-project/b.md"}, "notes/../long-project/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := make([]query.Row, 0, len(tc.files))
			var baseline strings.Builder
			seen := map[string]bool{}
			files := []string{}
			for _, file := range tc.files {
				rows = append(rows, query.Row{"file": query.NewString(file)})
				if !seen[file] {
					seen[file] = true
					files = append(files, file)
					fmt.Fprintf(&baseline, "@%q\n", file)
				}
			}
			var out bytes.Buffer
			if err := Render(&out, rows, FormatGrouped, 0, false, false, "", []string{"file"}); err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
			root := ""
			if strings.HasPrefix(lines[0], "root=") {
				var err error
				root, err = strconv.Unquote(strings.TrimPrefix(lines[0], "root="))
				if err != nil {
					t.Fatal(err)
				}
				lines = lines[1:]
			}
			if root != tc.root || len(lines) != len(files) {
				t.Fatalf("root=%q files=%d output=%q", root, len(lines), out.String())
			}
			for i, line := range lines {
				file, err := strconv.Unquote(strings.TrimPrefix(line, "@"))
				if err != nil || root+file != files[i] {
					t.Fatalf("root=%q file=%q want=%q error=%v", root, file, files[i], err)
				}
			}
			if root != "" && out.Len() >= baseline.Len() {
				t.Fatalf("root increased output: before=%d after=%d", baseline.Len(), out.Len())
			}
			if root == "" && out.String() != baseline.String() {
				t.Fatalf("unexpected header changes: got=%q want=%q", out.String(), baseline.String())
			}
		})
	}
}

func TestGroupedRootPreservesTSVBody(t *testing.T) {
	rows := []query.Row{
		{"file": query.NewString("long-project/notes/a.md"), "text": query.NewString("a\tline\nnext"), "line": query.NewInt(3)},
		{"file": query.NewString("long-project/notes/b.md"), "text": query.NewNull(), "line": query.NewInt(9)},
		{"file": query.NewString("long-project/notes/a.md"), "text": query.NewString("last"), "line": query.NewInt(4)},
	}
	var out bytes.Buffer
	if err := Render(&out, rows, FormatGrouped, 0, false, false, "", []string{"text", "file", "line"}, Options{Null: stringPointer("NULL")}); err != nil {
		t.Fatal(err)
	}
	want := "root=\"long-project/notes/\"\ntext\tline\n@\"a.md\"\na\\tline\\nnext\t3\nlast\t4\n@\"b.md\"\nNULL\t9\n"
	if out.String() != want {
		t.Fatalf("got=%q want=%q", out.String(), want)
	}
	if err := Render(failedFormatWriter{}, rows, FormatGrouped, 0, false, false, "", []string{"file", "text"}); err == nil {
		t.Fatal("grouped root swallowed the writer error")
	}
}

func TestGroupedDirectoriesPreservePathsAndReset(t *testing.T) {
	files := []string{
		"/vault/root-note.md",
		"/vault/archive-long-name/a.md",
		"/vault/archive-long-name/b.md",
		"/vault/another-root-note.md",
		"/vault/заметки\n\t\"проекта/c.md",
		"/vault/заметки\n\t\"проекта/d.md",
		"/vault/final-root-note.md",
	}
	rows := make([]query.Row, 0, len(files))
	var uncompressed strings.Builder
	for _, file := range files {
		rows = append(rows, query.Row{"file": query.NewString(file)})
		fmt.Fprintf(&uncompressed, "@%q\n", file)
	}
	var out bytes.Buffer
	if err := Render(&out, rows, FormatGrouped, 0, false, false, "", []string{"file"}); err != nil {
		t.Fatal(err)
	}
	root, directory := "", ""
	recovered := []string{}
	directories := []string{}
	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		var value string
		var err error
		switch {
		case strings.HasPrefix(line, "root="):
			value, err = strconv.Unquote(strings.TrimPrefix(line, "root="))
			root = value
		case strings.HasPrefix(line, "dir="):
			value, err = strconv.Unquote(strings.TrimPrefix(line, "dir="))
			directory = value
			directories = append(directories, value)
		case strings.HasPrefix(line, "@\""):
			value, err = strconv.Unquote(strings.TrimPrefix(line, "@"))
			recovered = append(recovered, root+directory+value)
		default:
			t.Fatalf("unexpected header %q", line)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(recovered, files) {
		t.Fatalf("recovered=%q want=%q", recovered, files)
	}
	wantDirectories := []string{"archive-long-name/", "", "заметки\n\t\"проекта/", ""}
	if !reflect.DeepEqual(directories, wantDirectories) {
		t.Fatalf("directories=%q want=%q", directories, wantDirectories)
	}
	if out.Len() >= uncompressed.Len() {
		t.Fatalf("prefixes increased output: before=%d after=%d", uncompressed.Len(), out.Len())
	}
}

func TestGroupedDirectoriesOnlyWhenBeneficial(t *testing.T) {
	for _, tc := range []struct {
		files []string
		want  []string
	}{
		{[]string{"/vault/a/x.md", "/vault/a/y.md", "/vault/z.md"}, []string{"", "", ""}},
		{[]string{"/vault/long-directory/a.md", "/vault/z.md"}, []string{"", ""}},
		{[]string{"/vault/long-directory/a.md", "/vault/long-directory/b.md", "/vault/z.md"}, []string{"long-directory/", "long-directory/", ""}},
		{[]string{"/vault/long-directory/a.md", "/vault/z.md", "/vault/long-directory/b.md"}, []string{"", "", ""}},
	} {
		if got := groupedDirectories(tc.files, "/vault/"); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("files=%q directories=%q want=%q", tc.files, got, tc.want)
		}
	}
}

func TestGroupedDataCannotImpersonateHeaders(t *testing.T) {
	values := []string{"@\"other.md\"", "root=\"other/\"", "dir=\"other/\"", "file=\"other.md\"", "@plain", "back\\slash\twith\nnewlines", ""}
	rows := make([]query.Row, 0, len(values))
	for _, value := range values {
		rows = append(rows, query.Row{"file": query.NewString("note.md"), "text": query.NewString(value)})
	}
	var out bytes.Buffer
	if err := Render(&out, rows, FormatGrouped, 0, false, false, "", []string{"file", "text"}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if lines[0] != "text" || lines[1] != "@\"note.md\"" || len(lines) != len(values)+2 {
		t.Fatalf("unexpected grouped layout: %q", out.String())
	}
	for i, line := range lines[2:] {
		if strings.HasPrefix(line, "@\"") || strings.HasPrefix(line, "root=") || strings.HasPrefix(line, "dir=") {
			t.Fatalf("data impersonates a header: %q", line)
		}
		if line == "" && values[i] == "" {
			continue
		}
		reader := csv.NewReader(strings.NewReader(line + "\n"))
		reader.Comma = '\t'
		record, err := reader.Read()
		want := strings.ReplaceAll(strings.ReplaceAll(values[i], "\n", "\\n"), "\t", "\\t")
		if err != nil || len(record) != 1 || record[0] != want {
			t.Fatalf("record=%q want=%q error=%v", record, want, err)
		}
	}
}

func TestGroupedValidatesBeforeWriting(t *testing.T) {
	valid := query.Row{"file": query.NewString("valid.md")}
	for _, bad := range []query.Row{nil, {"path": query.NewString("alias.md")}, {"file": query.NewNull()}, {"file": query.NewInt(1)}} {
		var out bytes.Buffer
		if err := Render(&out, []query.Row{valid, bad}, FormatGrouped, 0, false, false, "", []string{"*"}); err == nil || out.Len() != 0 {
			t.Fatalf("bad row=%v: error=%v output=%q", bad, err, out.String())
		}
	}
	for _, selected := range [][]string{{"text"}, {"path"}} {
		var out bytes.Buffer
		if err := Render(&out, nil, FormatGrouped, 0, false, false, "", selected); err == nil || out.Len() != 0 {
			t.Fatalf("bad projection=%v: error=%v output=%q", selected, err, out.String())
		}
	}
	var out bytes.Buffer
	if err := Render(&out, nil, FormatGrouped, 0, false, true, "", []string{"file"}); err != nil || out.Len() != 0 {
		t.Fatalf("--no-file: error=%v output=%q", err, out.String())
	}
	for _, selected := range [][]string{{"file"}, {"*"}} {
		if err := Render(&out, nil, FormatGrouped, 0, false, false, "", selected); err != nil || out.Len() != 0 {
			t.Fatalf("empty result: error=%v output=%q", err, out.String())
		}
	}
}

func TestGroupedSyntheticOutputSize(t *testing.T) {
	rows := make([]query.Row, 0, 1000)
	for file := 0; file < 20; file++ {
		for line := 1; line <= 50; line++ {
			rows = append(rows, query.Row{
				"file": query.NewString(fmt.Sprintf("synthetic/project/notes/long-directory-name/document-%02d.md", file)),
				"line": query.NewInt(int64(line)), "text": query.NewString("example line"),
			})
		}
	}
	var text, grouped bytes.Buffer
	for format, out := range map[Format]*bytes.Buffer{FormatText: &text, FormatGrouped: &grouped} {
		if err := Render(out, rows, format, 0, false, false, "", []string{"file", "line", "text"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Count(grouped.String(), "@"); got != 20 {
		t.Fatalf("got %d file headers, want 20", got)
	}
	if got := strings.Count(grouped.String(), "line\ttext\n"); got != 1 {
		t.Fatalf("got %d column headers, want 1", got)
	}
	t.Logf("synthetic corpus: 20 files × 50 rows; text=%d bytes, grouped=%d bytes, saved=%d bytes (%.1f%%)", text.Len(), grouped.Len(), text.Len()-grouped.Len(), 100*float64(text.Len()-grouped.Len())/float64(text.Len()))
}
