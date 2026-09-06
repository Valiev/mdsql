package tables

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

var Registry = []TableDef{}

type TableDef struct {
	Name    string
	Doc     string
	Columns []query.Column
	Rows    func(doc *model.Document) []query.Row
}

func init() {
	Registry = []TableDef{
		{
			Name: "files",
			Doc:  "Файл и его метрики",
			Columns: []query.Column{
				{Name: "file", Type: "string"},
				{Name: "name", Type: "string"},
				{Name: "dir", Type: "string"},
				{Name: "size", Type: "int"},
				{Name: "lines", Type: "int"},
				{Name: "words", Type: "int"},
				{Name: "headings", Type: "int"},
				{Name: "tasks", Type: "int"},
				{Name: "open_tasks", Type: "int"},
				{Name: "code_blocks", Type: "int"},
				{Name: "mtime", Type: "datetime"},
				{Name: "fm", Type: "object"},
			},
			Rows: rowsFiles,
		},
		{
			Name: "frontmatter",
			Doc:  "Frontmatter файла",
			Columns: []query.Column{
				{Name: "file", Type: "string"},
				{Name: "fm", Type: "object"},
				{Name: "raw", Type: "string"},
				{Name: "start", Type: "int"},
				{Name: "end", Type: "int"},
			},
			Rows: rowsFrontmatter,
		},
		{
			Name: "frontmatter_values",
			Doc:  "Значения полей frontmatter",
			Columns: []query.Column{
				{Name: "file", Type: "string"},
				{Name: "key", Type: "string"},
				{Name: "value", Type: "any"},
			},
			Rows: rowsFrontmatterValues,
		},
		{
			Name:    "headings",
			Doc:     "Все заголовки",
			Columns: []query.Column{{Name: "file", Type: "string"}, {Name: "start", Type: "int"}, {Name: "end", Type: "int"}, {Name: "line", Type: "int"}, {Name: "level", Type: "int"}, {Name: "title", Type: "string"}, {Name: "slug", Type: "string"}, {Name: "text", Type: "string"}, {Name: "section", Type: "string"}, {Name: "path", Type: "string"}, {Name: "nested", Type: "bool"}, {Name: "section_end", Type: "int", Doc: "Последняя строка секции вместе с подсекциями"}, {Name: "parent", Type: "string", Doc: "Название ближайшего родительского заголовка; пусто у корневых"}, {Name: "depth", Type: "int", Doc: "Глубина в дереве: корень 0, первый заголовок 1; пропуски уровней не увеличивают глубину"}, {Name: "raw", Type: "string", Doc: "Исходный текст с собственным заголовком"}, {Name: "fm", Type: "object"}},
			Rows:    rowsHeadings,
		},
		{
			Name:    "sections",
			Doc:     "Иерархия секций; level=0 охватывает всё тело без frontmatter, own содержит преамбулу",
			Columns: []query.Column{{Name: "file", Type: "string"}, {Name: "level", Type: "int"}, {Name: "title", Type: "string"}, {Name: "slug", Type: "string"}, {Name: "parent", Type: "string", Doc: "Название ближайшего родительского заголовка; пусто у корневых"}, {Name: "depth", Type: "int", Doc: "Глубина в дереве: корень 0, первый заголовок 1; пропуски уровней не увеличивают глубину"}, {Name: "start", Type: "int"}, {Name: "end", Type: "int"}, {Name: "children", Type: "int", Doc: "Количество прямых дочерних секций"}, {Name: "body", Type: "string", Doc: "Текст без собственного заголовка, включая подсекции; края обрезаны"}, {Name: "own", Type: "string", Doc: "Текст без собственного заголовка и подсекций; края обрезаны"}, {Name: "raw", Type: "string", Doc: "Исходный текст секции с собственным заголовком и всеми подсекциями"}, {Name: "path", Type: "string"}, {Name: "tasks", Type: "int"}, {Name: "open_tasks", Type: "int"}, {Name: "code_blocks", Type: "int"}, {Name: "lines", Type: "int"}, {Name: "fm", Type: "object"}},
			Rows:    rowsSections,
		},
		{
			Name:    "blocks",
			Doc:     "Сегменты markdown",
			Columns: []query.Column{{Name: "file", Type: "string"}, {Name: "start", Type: "int"}, {Name: "end", Type: "int"}, {Name: "kind", Type: "string"}, {Name: "text", Type: "string"}, {Name: "line", Type: "int"}, {Name: "section", Type: "string"}, {Name: "path", Type: "string"}, {Name: "raw", Type: "string"}, {Name: "lang", Type: "string"}, {Name: "info", Type: "string"}, {Name: "fence", Type: "string"}, {Name: "lines", Type: "int"}, {Name: "fm", Type: "object"}},
			Rows:    rowsBlocks,
		},
		{
			Name:    "tasks",
			Doc:     "Задачи (task list)",
			Columns: []query.Column{{Name: "file", Type: "string"}, {Name: "line", Type: "int"}, {Name: "status", Type: "string"}, {Name: "state", Type: "string"}, {Name: "container", Type: "string", Doc: "Пусто для обычной задачи; blockquote или callout для задачи в цитате"}, {Name: "done", Type: "bool"}, {Name: "text", Type: "string"}, {Name: "due", Type: "date"}, {Name: "scheduled", Type: "date"}, {Name: "start_date", Type: "date"}, {Name: "done_at", Type: "date"}, {Name: "cancelled_at", Type: "date"}, {Name: "created", Type: "date"}, {Name: "recurrence", Type: "string"}, {Name: "id", Type: "string"}, {Name: "depends_on", Type: "list"}, {Name: "priority", Type: "int"}, {Name: "priority_name", Type: "string"}, {Name: "depth", Type: "int"}, {Name: "parent", Type: "int"}, {Name: "root", Type: "int"}, {Name: "tags", Type: "list"}, {Name: "section", Type: "string"}, {Name: "path", Type: "string"}, {Name: "start", Type: "int"}, {Name: "end", Type: "int"}, {Name: "raw", Type: "string"}, {Name: "fm", Type: "object"}},
			Rows:    rowsTasks,
		},
		{
			Name:    "code",
			Doc:     "Блоки кода",
			Columns: []query.Column{{Name: "file", Type: "string"}, {Name: "start", Type: "int"}, {Name: "end", Type: "int"}, {Name: "line", Type: "int"}, {Name: "lang", Type: "string"}, {Name: "info", Type: "string"}, {Name: "code", Type: "string"}, {Name: "fence", Type: "string"}, {Name: "lines", Type: "int"}, {Name: "section", Type: "string"}, {Name: "path", Type: "string"}, {Name: "raw", Type: "string"}, {Name: "text", Type: "string"}, {Name: "fm", Type: "object"}},
			Rows:    rowsCode,
		},
		{
			Name:    "links",
			Doc:     "Ссылки",
			Columns: []query.Column{{Name: "file", Type: "string"}, {Name: "type", Type: "string"}, {Name: "source", Type: "string"}, {Name: "key", Type: "string"}, {Name: "target", Type: "string"}, {Name: "anchor", Type: "string"}, {Name: "block_ref", Type: "string"}, {Name: "text", Type: "string"}, {Name: "raw", Type: "string"}, {Name: "line", Type: "int"}, {Name: "section", Type: "string"}, {Name: "path", Type: "string"}, {Name: "external", Type: "bool"}, {Name: "fm", Type: "object"}},
			Rows:    rowsLinks,
		},
		{
			Name:    "tags",
			Doc:     "Теги",
			Columns: []query.Column{{Name: "file", Type: "string"}, {Name: "tag", Type: "string"}, {Name: "source", Type: "string"}, {Name: "line", Type: "int"}, {Name: "section", Type: "string"}, {Name: "path", Type: "string"}, {Name: "block_kind", Type: "string"}, {Name: "fm", Type: "object"}},
			Rows:    rowsTags,
		},
		{
			Name:    "lines",
			Doc:     "Строки файла",
			Columns: []query.Column{{Name: "file", Type: "string"}, {Name: "n", Type: "int"}, {Name: "text", Type: "string"}, {Name: "kind", Type: "string"}, {Name: "section", Type: "string"}, {Name: "path", Type: "string"}, {Name: "fm", Type: "object"}},
			Rows:    rowsLines,
		},
	}
}

func Table(name string) (TableDef, bool) {
	for _, t := range Registry {
		if t.Name == name {
			return t, true
		}
	}
	return TableDef{}, false
}

func rowsFiles(doc *model.Document) []query.Row {
	tasksOpen := 0
	for _, t := range doc.Tasks {
		if !t.Done {
			tasksOpen++
		}
	}
	codeBlocks := 0
	for _, b := range doc.Blocks {
		if b.Kind == model.KindCode {
			codeBlocks++
		}
	}
	words := 0
	bodyStart := doc.FrontMatterTo
	if bodyStart < 0 || bodyStart > len(doc.Lines) {
		bodyStart = 0
	}
	for _, l := range doc.Lines[bodyStart:] {
		words += len(strings.Fields(l))
	}
	return fileRow(doc, map[string]interface{}{
		"size":        int64(doc.Size),
		"lines":       len(doc.Lines),
		"words":       words,
		"headings":    len(doc.Headings),
		"tasks":       len(doc.Tasks),
		"open_tasks":  tasksOpen,
		"code_blocks": codeBlocks,
		"mtime":       doc.ModTime,
	})
}

func fileRow(doc *model.Document, extras map[string]interface{}) []query.Row {
	row := query.Row{
		"file": query.NewString(doc.Path),
		"name": query.NewString(filepath.Base(doc.Path)),
		"dir":  query.NewString(filepath.Dir(doc.Path)),
		"fm":   frontMatterObject(doc.FrontMatter),
	}
	for k, v := range extras {
		switch vv := v.(type) {
		case int64:
			row[k] = query.NewInt(vv)
		case int:
			row[k] = query.NewInt(int64(vv))
		case time.Time:
			row[k] = query.NewDateTime(vv)
		default:
		}
	}
	return []query.Row{row}
}

func rowsFrontmatter(doc *model.Document) []query.Row {
	if doc.FrontMatter == nil {
		return []query.Row{}
	}
	return []query.Row{{
		"file":  query.NewString(doc.Path),
		"fm":    frontMatterObject(doc.FrontMatter),
		"raw":   query.NewString(doc.FrontMatterRaw),
		"start": query.NewInt(int64(doc.FrontMatterFrom)),
		"end":   query.NewInt(int64(doc.FrontMatterTo)),
	}}
}

func rowsFrontmatterValues(doc *model.Document) []query.Row {
	keys := make([]string, 0, len(doc.FrontMatter))
	for key := range doc.FrontMatter {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := []query.Row{}
	for _, key := range keys {
		value := normalizeFieldValue(doc.FrontMatter[key])
		values := []query.Value{value}
		if value.Kind == query.ValueList {
			values = value.Data.([]query.Value)
		}
		for _, value := range values {
			rows = append(rows, query.Row{
				"file":  query.NewString(doc.Path),
				"key":   query.NewString(key),
				"value": value,
			})
		}
	}
	return rows
}

func rowsHeadings(doc *model.Document) []query.Row {
	res := make([]query.Row, 0, len(doc.Headings))
	for _, h := range doc.Headings {
		res = append(res, query.Row{
			"file":        query.NewString(h.File),
			"start":       query.NewInt(int64(h.Start)),
			"end":         query.NewInt(int64(h.End)),
			"line":        query.NewInt(int64(h.Start)),
			"level":       query.NewInt(int64(h.Level)),
			"title":       query.NewString(h.Title),
			"slug":        query.NewString(h.Slug),
			"text":        query.NewString(h.Text),
			"section":     query.NewString(h.Section),
			"path":        query.NewString(h.Path),
			"nested":      query.NewBool(h.Nested),
			"section_end": query.NewInt(int64(h.SectionEnd)),
			"parent":      query.NewString(h.Parent),
			"depth":       query.NewInt(int64(h.Depth)),
			"fm":          frontMatterObject(doc.FrontMatter),
			"raw":         query.NewString(h.Raw),
		})
	}
	return res
}

func rowsSections(doc *model.Document) []query.Row {
	res := make([]query.Row, 0, len(doc.Sections))
	for _, s := range doc.Sections {
		res = append(res, query.Row{
			"file":        query.NewString(s.File),
			"level":       query.NewInt(int64(s.Level)),
			"title":       query.NewString(s.Title),
			"slug":        query.NewString(s.Slug),
			"parent":      query.NewString(s.Parent),
			"depth":       query.NewInt(int64(s.Depth)),
			"start":       query.NewInt(int64(s.Start)),
			"end":         query.NewInt(int64(s.End)),
			"children":    query.NewInt(int64(s.Children)),
			"body":        query.NewString(s.Body),
			"own":         query.NewString(s.Own),
			"raw":         query.NewString(s.Raw),
			"path":        query.NewString(s.Path),
			"tasks":       query.NewInt(int64(s.Tasks)),
			"open_tasks":  query.NewInt(int64(s.OpenTasks)),
			"code_blocks": query.NewInt(int64(s.CodeBlocks)),
			"lines":       query.NewInt(int64(s.End - s.Start + 1)),
			"fm":          frontMatterObject(doc.FrontMatter),
		})
	}
	return res
}

func rowsBlocks(doc *model.Document) []query.Row {
	res := make([]query.Row, 0, len(doc.Blocks))
	for _, b := range doc.Blocks {
		row := query.Row{
			"file":    query.NewString(b.File),
			"start":   query.NewInt(int64(b.Start)),
			"end":     query.NewInt(int64(b.End)),
			"kind":    query.NewString(b.Kind),
			"text":    query.NewString(b.Text),
			"line":    query.NewInt(int64(b.Line)),
			"section": query.NewString(b.Section),
			"path":    query.NewString(b.Path),
			"raw":     query.NewString(b.Raw),
			"fm":      frontMatterObject(doc.FrontMatter),
		}
		if b.Kind == model.KindCode {
			row["lang"] = query.NewString(b.Lang)
			row["info"] = query.NewString(b.Info)
			row["fence"] = query.NewString(b.Fence)
			row["lines"] = query.NewInt(int64(strings.Count(b.Text, "\n") + 1))
		}
		res = append(res, row)
	}
	return res
}

func rowsCode(doc *model.Document) []query.Row {
	res := make([]query.Row, 0, len(doc.Blocks))
	for _, b := range doc.Blocks {
		if b.Kind != model.KindCode {
			continue
		}
		code := b.Text
		row := query.Row{
			"file":    query.NewString(b.File),
			"start":   query.NewInt(int64(b.Start)),
			"end":     query.NewInt(int64(b.End)),
			"line":    query.NewInt(int64(b.Line)),
			"text":    query.NewString(code),
			"lang":    query.NewString(strings.ToLower(b.Lang)),
			"info":    query.NewString(b.Info),
			"code":    query.NewString(code),
			"fence":   query.NewString(b.Fence),
			"lines":   query.NewInt(int64(len(strings.Split(code, "\n")))),
			"section": query.NewString(b.Section),
			"path":    query.NewString(b.Path),
			"raw":     query.NewString(b.Raw),
			"fm":      frontMatterObject(doc.FrontMatter),
		}
		res = append(res, row)
	}
	return res
}

func rowsTasks(doc *model.Document) []query.Row {
	res := make([]query.Row, 0, len(doc.Tasks))
	for _, t := range doc.Tasks {
		depends := []query.Value{}
		for _, d := range t.DependsOn {
			depends = append(depends, query.NewString(d))
		}
		tags := []query.Value{}
		for _, tg := range t.Tags {
			tags = append(tags, query.NewString(tg))
		}
		row := query.Row{
			"file":          query.NewString(t.File),
			"start":         query.NewInt(int64(t.Start)),
			"end":           query.NewInt(int64(t.End)),
			"raw":           query.NewString(t.Raw),
			"line":          query.NewInt(int64(t.Line)),
			"status":        query.NewString(t.Status),
			"state":         query.NewString(t.State),
			"container":     query.NewString(t.Container),
			"done":          query.NewBool(t.Done),
			"text":          query.NewString(t.Text),
			"due":           datePtr(t.Due),
			"scheduled":     datePtr(t.Scheduled),
			"start_date":    datePtr(t.StartDate),
			"done_at":       datePtr(t.DoneAt),
			"cancelled_at":  datePtr(t.CancelledAt),
			"created":       datePtr(t.Created),
			"recurrence":    query.NewString(t.Recurrence),
			"id":            query.NewString(t.ID),
			"depends_on":    query.NewList(depends),
			"priority":      query.NewInt(int64(t.Priority)),
			"priority_name": query.NewString(t.PriorityName),
			"depth":         query.NewInt(int64(t.Depth)),
			"parent":        query.NewInt(int64(t.Parent)),
			"root":          query.NewInt(int64(t.Root)),
			"tags":          query.NewList(tags),
			"section":       query.NewString(t.Section),
			"path":          query.NewString(t.SectionPath),
			"fm":            frontMatterObject(doc.FrontMatter),
		}
		for k, v := range t.Fields {
			row["fields."+k] = normalizeFieldValue(v)
		}
		res = append(res, row)
	}
	return res
}

func rowsLinks(doc *model.Document) []query.Row {
	res := make([]query.Row, 0, len(doc.Links))
	for _, l := range doc.Links {
		res = append(res, query.Row{
			"file":      query.NewString(l.File),
			"type":      query.NewString(l.Type),
			"source":    query.NewString(l.Source),
			"key":       query.NewString(l.Key),
			"target":    query.NewString(l.Target),
			"anchor":    query.NewString(l.Anchor),
			"block_ref": query.NewString(l.BlockRef),
			"text":      query.NewString(l.Text),
			"raw":       query.NewString(l.Raw),
			"line":      query.NewInt(int64(l.Line)),
			"section":   query.NewString(l.Section),
			"path":      query.NewString(l.Path),
			"external":  query.NewBool(l.External),
			"fm":        frontMatterObject(doc.FrontMatter),
		})
	}
	return res
}

func rowsTags(doc *model.Document) []query.Row {
	res := make([]query.Row, 0, len(doc.Tags))
	for _, t := range doc.Tags {
		res = append(res, query.Row{
			"file":       query.NewString(t.File),
			"tag":        query.NewString(t.Tag),
			"source":     query.NewString(t.Source),
			"line":       query.NewInt(int64(t.Line)),
			"section":    query.NewString(t.Section),
			"path":       query.NewString(t.Path),
			"block_kind": query.NewString(t.BlockKind),
			"fm":         frontMatterObject(doc.FrontMatter),
		})
	}
	return res
}

func rowsLines(doc *model.Document) []query.Row {
	res := make([]query.Row, 0, len(doc.LinesRows))
	for _, l := range doc.LinesRows {
		res = append(res, query.Row{
			"file":    query.NewString(l.File),
			"n":       query.NewInt(int64(l.N)),
			"text":    query.NewString(l.Text),
			"kind":    query.NewString(l.Kind),
			"section": query.NewString(l.Section),
			"path":    query.NewString(l.Path),
			"fm":      frontMatterObject(doc.FrontMatter),
		})
	}
	return res
}

func rowToLineNumber(row query.Row, key string) int {
	v := row[key]
	if v.Kind == query.ValueNull {
		return 0
	}
	return int(v.Data.(int64))
}

func findSectionTitle(doc *model.Document, line int) query.Value {
	for _, s := range doc.Sections {
		if line >= s.Start && line <= s.End {
			return query.NewString(s.Title)
		}
	}
	return query.NewString("")
}

func frontMatterObject(fm model.FrontMatter) query.Value {
	if fm == nil {
		return query.NewObject(map[string]query.Value{})
	}
	m := map[string]query.Value{}
	for k, v := range fm {
		m[k] = normalizeFieldValue(v)
	}
	return query.NewObject(m)
}

func normalizeFieldValue(v interface{}) query.Value {
	switch t := v.(type) {
	case nil:
		return query.NewNull()
	case model.FrontMatter:
		return frontMatterObject(t)
	case time.Time:
		return query.NewDateTime(t)
	case string:
		if parseDateOrEmpty(t) != (time.Time{}) {
			return query.NewDate(parseDateOrEmpty(t))
		}
		return query.NewString(t)
	case int:
		return query.NewInt(int64(t))
	case int64:
		return query.NewInt(t)
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return query.NewString(fmt.Sprint(t))
		}
		return query.NewFloat(t)
	case float32:
		if math.IsNaN(float64(t)) || math.IsInf(float64(t), 0) {
			return query.NewString(fmt.Sprint(t))
		}
		return query.NewFloat(float64(t))
	case bool:
		return query.NewBool(t)
	case []interface{}:
		arr := make([]query.Value, 0, len(t))
		for _, x := range t {
			arr = append(arr, normalizeFieldValue(x))
		}
		return query.NewList(arr)
	case map[string]interface{}:
		obj := map[string]query.Value{}
		for k, sub := range t {
			obj[k] = normalizeFieldValue(sub)
		}
		return query.NewObject(obj)
	default:
		return query.NewString(fmt.Sprintf("%v", v))
	}
}

func datePtr(t *time.Time) query.Value {
	if t == nil {
		return query.NewNull()
	}
	return query.NewDate(*t)
}

func parseDateOrEmpty(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func TableNames() []string {
	n := make([]string, 0, len(Registry))
	for _, t := range Registry {
		n = append(n, t.Name)
	}
	sort.Strings(n)
	return n
}

func init() {
	appendBlockView("paragraphs", model.KindParagraph)
	appendBlockView("lists", model.KindList)
	appendBlockView("items", model.KindList)
	appendBlockView("tables", model.KindTable)

	quotes := Registry[0]
	quotes.Name = "quotes"
	quotes.Columns = []query.Column{
		{Name: "file", Type: "string"}, {Name: "start", Type: "int"},
		{Name: "end", Type: "int"}, {Name: "type", Type: "string"},
		{Name: "title", Type: "string"}, {Name: "fold", Type: "string"},
		{Name: "body", Type: "string"}, {Name: "text", Type: "string"},
		{Name: "section", Type: "string"}, {Name: "path", Type: "string"},
		{Name: "fm", Type: "object"},
	}
	quotes.Rows = func(doc *model.Document) []query.Row {
		rows := make([]query.Row, 0, len(doc.Quotes))
		for _, quote := range doc.Quotes {
			rows = append(rows, query.Row{
				"file": query.NewString(quote.File), "start": query.NewInt(int64(quote.Start)),
				"end": query.NewInt(int64(quote.End)), "type": query.NewString(quote.Type),
				"title": query.NewString(quote.Title), "fold": query.NewString(quote.Fold),
				"body": query.NewString(quote.Body), "text": query.NewString(quote.Text),
				"section": query.NewString(quote.Section), "path": query.NewString(quote.Path),
				"fm": frontMatterObject(doc.FrontMatter),
			})
		}
		return rows
	}
	Registry = append(Registry, quotes)
	Registry = append(Registry, cellsTable)
	populateColumnDocs()
}

func appendBlockView(name, kind string) {
	table := Registry[0]
	table.Name = name
	table.Columns = []query.Column{
		{Name: "file", Type: "string"}, {Name: "start", Type: "int"},
		{Name: "end", Type: "int"}, {Name: "line", Type: "int"},
		{Name: "text", Type: "string"}, {Name: "raw", Type: "string"},
		{Name: "section", Type: "string"}, {Name: "path", Type: "string"},
		{Name: "nested", Type: "bool"}, {Name: "fm", Type: "object"},
	}
	if name == "tables" {
		table.Columns = append(table.Columns, query.Column{Name: "rows", Type: "int"}, query.Column{Name: "columns", Type: "int"})
	}
	table.Rows = func(doc *model.Document) []query.Row {
		rows := []query.Row{}
		for _, block := range doc.Blocks {
			if block.Kind != kind {
				continue
			}
			row := query.Row{
				"file": query.NewString(block.File), "start": query.NewInt(int64(block.Start)),
				"end": query.NewInt(int64(block.End)), "line": query.NewInt(int64(block.Line)),
				"text": query.NewString(block.Text), "raw": query.NewString(block.Raw),
				"section": query.NewString(block.Section), "path": query.NewString(block.Path),
				"nested": query.NewBool(block.Nested), "fm": frontMatterObject(doc.FrontMatter),
			}
			if name == "tables" {
				headers, lines := markdownTable(block.Raw)
				dataRows := 0
				if headers != nil {
					dataRows = len(lines) - 2
				}
				row["rows"] = query.NewInt(int64(dataRows))
				row["columns"] = query.NewInt(int64(len(headers)))
			}
			rows = append(rows, row)
		}
		return rows
	}
	Registry = append(Registry, table)
}
