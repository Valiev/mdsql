package parser

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	extension "github.com/yuin/goldmark/extension"
	gparser "github.com/yuin/goldmark/parser"
	yaml "gopkg.in/yaml.v3"

	"github.com/Valiev/mdsql/internal/model"
)

// Document is the public parser result type.
type Document = model.Document

var (
	reAtxHeading = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*(#+\s*)?$`)
	reSetextLine = regexp.MustCompile(`^[=-]+$`)
	reTask       = regexp.MustCompile(`^(\s*)(?:-\s+|\*\s+|\+\s+|\d+\.\s+)\[(.)\]\s*(.*)$`)
	reList       = regexp.MustCompile(`^(\s*)(?:[-*+]\s+|\d+\.\s+)(.*)$`)
	reLinkRef    = regexp.MustCompile(`!?\[\[([^\]]+)\]\]`)
	reMdLink     = regexp.MustCompile(`!?\[([^\]]+)\]\(([^\s\)]+)(?:\s+"[^"]+")?\)`)
	reTag        = regexp.MustCompile(`(?:^|[\s(])#([\p{L}\p{N}_/-]+)`)
	reAutolink   = regexp.MustCompile(`<https?://[^>]+>`)
	reBareURL    = regexp.MustCompile(`https?://[^\s<>\[\]]+`)
	reYAMLLine   = regexp.MustCompile(`\bline (\d+)\b`)
)

// ParseFile parses markdown and returns an indexed document.
func ParseFile(path, raw string, size int64, mtime time.Time) (*model.Document, error) {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithParserOptions(gparser.WithAttribute()))
	_ = md

	raw = strings.TrimPrefix(raw, "\uFEFF")
	normalized := strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
	lines := SourceLines(normalized)
	doc := &model.Document{
		Path:      filepath.Clean(path),
		Content:   normalized,
		Lines:     lines,
		Size:      size,
		ModTime:   mtime,
		Headings:  []model.Heading{},
		Sections:  []model.Section{},
		Blocks:    []model.Block{},
		Tasks:     []model.Task{},
		Links:     []model.Link{},
		Tags:      []model.Tag{},
		Quotes:    []model.Quote{},
		LinesRows: []model.Line{},
	}

	parseFrontMatter(doc, lines)
	quotedCode := parseBody(doc, lines)
	buildSections(doc)
	attachSections(doc)
	extractTagsAndLinks(doc, lines, quotedCode)
	collectLineRows(doc)
	return doc, nil
}

// SetDocumentPath replaces the logical path exposed by every document row.
func SetDocumentPath(doc *Document, path string) {
	doc.Path = path
	for i := range doc.Headings {
		doc.Headings[i].File = path
	}
	for i := range doc.Sections {
		doc.Sections[i].File = path
	}
	for i := range doc.Blocks {
		doc.Blocks[i].File = path
	}
	for i := range doc.Tasks {
		doc.Tasks[i].File = path
	}
	for i := range doc.Links {
		doc.Links[i].File = path
	}
	for i := range doc.Tags {
		doc.Tags[i].File = path
	}
	for i := range doc.Quotes {
		doc.Quotes[i].File = path
	}
	for i := range doc.LinesRows {
		doc.LinesRows[i].File = path
	}
}

// SourceLines splits normalized source into physical lines without a trailing sentinel.
func SourceLines(content string) []string {
	lines := strings.Split(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func parseFrontMatter(doc *model.Document, lines []string) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "---" {
			continue
		}
		blob := strings.Join(lines[1:i], "\n")
		doc.FrontMatterRaw = blob
		doc.FrontMatterFrom = 1
		doc.FrontMatterTo = i + 1
		var fm model.FrontMatter
		var node yaml.Node
		err := yaml.Unmarshal([]byte(blob), &node)
		if err == nil {
			normalizeFrontmatterDates(&node)
			err = node.Decode(&fm)
		}
		if err != nil {
			line := 2
			if m := reYAMLLine.FindStringSubmatch(err.Error()); m != nil {
				line, _ = strconv.Atoi(m[1])
				line++
			}
			message := reYAMLLine.ReplaceAllStringFunc(err.Error(), func(match string) string {
				n, _ := strconv.Atoi(strings.TrimPrefix(match, "line "))
				return fmt.Sprintf("line %d", n+1)
			})
			doc.Warnings = append(doc.Warnings, fmt.Sprintf("%s:%d: invalid frontmatter: %s", doc.Path, line, message))
			return
		}
		doc.FrontMatter = fm
		extractFrontmatterLinks(doc, &node, "")
		return
	}
}

func parseBody(doc *model.Document, lines []string) [][2]int {
	var quotedCode [][2]int
	start := doc.FrontMatterTo
	if start < 1 {
		start = 0
	}
	for i := start; i < len(lines); {
		line := lines[i]
		trim := strings.TrimSpace(line)
		i1 := i + 1

		if fence, info := parseCodeFence(line); fence != "" {
			j := i + 1
			for ; j < len(lines); j++ {
				closing, tail := parseCodeFence(lines[j])
				if len(closing) >= len(fence) && closing[0] == fence[0] && tail == "" {
					break
				}
			}
			end := j
			if j < len(lines) {
				end++
			}
			lang := ""
			if fields := strings.Fields(info); len(fields) > 0 {
				lang = strings.ToLower(fields[0])
			}
			code := strings.Join(lines[i+1:j], "\n")
			raw := strings.Join(lines[i:end], "\n")
			if j == len(lines) && strings.HasSuffix(doc.Content, "\n") {
				raw += "\n"
				if j > i+1 {
					code += "\n"
				}
			}
			doc.Blocks = append(doc.Blocks, model.Block{
				File:  doc.Path,
				Kind:  model.KindCode,
				Start: i1,
				End:   end,
				Line:  i1,
				Text:  code,
				Raw:   raw,
				Lang:  lang,
				Info:  info,
				Fence: fence,
			})
			i = end
			continue
		}

		if m := reAtxHeading.FindStringSubmatch(line); m != nil {
			lvl := len(m[1])
			title := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(stripFormatting(m[2])), "#"), "#"))
			doc.Headings = append(doc.Headings, model.Heading{
				File:   doc.Path,
				Start:  i1,
				End:    i1,
				Level:  lvl,
				Title:  title,
				Text:   title,
				Nested: leadingWhitespace(line),
				Raw:    line,
			})
			doc.Blocks = append(doc.Blocks, model.Block{File: doc.Path, Kind: model.KindHeading, Start: i1, End: i1, Line: i1, Text: title, Raw: line})
			i++
			continue
		}

		if isSetextHeading(lines, i) {
			title := strings.TrimSpace(stripFormatting(lines[i]))
			lvl := 1
			if strings.HasPrefix(strings.TrimSpace(lines[i+1]), "-") {
				lvl = 2
			}
			doc.Headings = append(doc.Headings, model.Heading{
				File:   doc.Path,
				Start:  i1,
				End:    i + 2,
				Level:  lvl,
				Title:  title,
				Text:   title,
				Nested: false,
				Raw:    strings.Join(lines[i:i+2], "\n"),
			})
			doc.Blocks = append(doc.Blocks, model.Block{File: doc.Path, Kind: model.KindHeading, Start: i1, End: i + 2, Line: i1, Text: title, Raw: strings.Join(lines[i:i+2], "\n")})
			i += 2
			continue
		}

		if t, ok := parseTask(doc.Path, line, line, i1, ""); ok {
			doc.Tasks = append(doc.Tasks, t)
			doc.Blocks = append(doc.Blocks, model.Block{File: doc.Path, Kind: model.KindList, Start: i1, End: i1, Line: i1, Text: t.Text, Raw: line, Nested: t.Nested, Level: t.Depth})
			i++
			continue
		}

		if m := reList.FindStringSubmatch(line); m != nil {
			text := strings.TrimSpace(stripFormatting(m[2]))
			doc.Blocks = append(doc.Blocks, model.Block{
				File: doc.Path, Kind: model.KindList, Start: i1, End: i1, Line: i1,
				Text: text, Raw: line, Nested: len(m[1]) > 0, Level: len(m[1]) / 2,
			})
			i++
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(line), ">") {
			j := i
			buf := []string{}
			for ; j < len(lines); j++ {
				if !strings.HasPrefix(strings.TrimSpace(lines[j]), ">") {
					break
				}
				buf = append(buf, strings.TrimSpace(strings.TrimPrefix(strings.TrimLeft(lines[j], "\t "), ">")))
			}
			quotedCode = append(quotedCode, extractQuoteTasks(doc, lines, i, j)...)
			qType, qTitle, qFold := parseCalloutFirstLine(buf)
			raw := strings.Join(lines[i:j], "\n")
			doc.Quotes = append(doc.Quotes, model.Quote{File: doc.Path, Start: i1, End: j, Type: qType, Title: qTitle, Fold: qFold, Body: strings.Join(buf, "\n"), Text: strings.Join(buf, "\n")})
			doc.Blocks = append(doc.Blocks, model.Block{File: doc.Path, Kind: model.KindQuote, Start: i1, End: j, Line: i1, Text: strings.Join(buf, "\n"), Raw: raw})
			i = j
			continue
		}

		if strings.HasPrefix(trim, "|") {
			j := i + 1
			for ; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == "" || !strings.HasPrefix(strings.TrimSpace(lines[j]), "|") {
					break
				}
			}
			doc.Blocks = append(doc.Blocks, model.Block{File: doc.Path, Kind: model.KindTable, Start: i1, End: j, Line: i1, Text: strings.TrimSpace(strings.Join(lines[i:j], "\n")), Raw: strings.Join(lines[i:j], "\n")})
			i = j
			continue
		}

		if trim == "" {
			i++
			continue
		}

		if isThematicBreak(trim) {
			doc.Blocks = append(doc.Blocks, model.Block{File: doc.Path, Kind: model.KindHr, Start: i1, End: i1, Line: i1, Raw: line})
			i++
			continue
		}

		doc.Blocks = append(doc.Blocks, model.Block{File: doc.Path, Kind: model.KindParagraph, Start: i1, End: i1, Line: i1, Text: stripFormatting(trim), Raw: line})
		i++
	}
	return quotedCode
}

func isThematicBreak(line string) bool {
	compact := strings.ReplaceAll(line, " ", "")
	if len(compact) < 3 || !strings.ContainsRune("-*_", rune(compact[0])) {
		return false
	}
	for i := 1; i < len(compact); i++ {
		if compact[i] != compact[0] {
			return false
		}
	}
	return true
}

func parseTaskMeta(task *model.Task) {
	text := task.Text
	addDateField := func(prefix string, target **time.Time, field string) {
		re := regexp.MustCompile(fmt.Sprintf(`%s\s*(\d{4}-\d{2}-\d{2})`, regexp.QuoteMeta(prefix)))
		m := re.FindStringSubmatch(text)
		if len(m) == 2 {
			if t, err := time.Parse(time.DateOnly, m[1]); err == nil {
				*target = &t
				text = strings.Replace(text, m[0], "", 1)
				_ = field
			}
		}
	}
	addDateField("📅", &task.Due, "Due")
	addDateField("⏳", &task.Scheduled, "Scheduled")
	addDateField("🛫", &task.StartDate, "StartDate")
	addDateField("✅", &task.DoneAt, "DoneAt")
	addDateField("❌", &task.CancelledAt, "CancelledAt")
	addDateField("➕", &task.Created, "Created")

	task.Tags = append(task.Tags, extractTagsFromText(text)...)
	if m := regexp.MustCompile(`(🔺|⏫|🔼|🔽|⏬)`).FindStringSubmatch(text); len(m) > 0 {
		switch m[1] {
		case "🔺":
			task.Priority, task.PriorityName = 5, "highest"
		case "⏫":
			task.Priority, task.PriorityName = 4, "high"
		case "🔼":
			task.Priority, task.PriorityName = 3, "medium"
		case "🔽":
			task.Priority, task.PriorityName = 2, "low"
		case "⏬":
			task.Priority, task.PriorityName = 1, "lowest"
		}
	}
	if m := regexp.MustCompile(`🆔\s*(\S+)`).FindStringSubmatch(text); len(m) > 1 {
		task.ID = m[1]
	}
	if m := regexp.MustCompile(`⛔\s*([^\s,]+(?:\s*,\s*[^\s,]+)*)`).FindStringSubmatch(text); len(m) > 1 {
		for _, p := range strings.Split(m[1], ",") {
			if x := strings.TrimSpace(p); x != "" {
				task.DependsOn = append(task.DependsOn, x)
			}
		}
	}
	if m := regexp.MustCompile(`🔁\s*([^\n]+)`).FindStringSubmatch(text); len(m) > 1 {
		task.Recurrence = strings.TrimSpace(m[1])
	}

	for _, m := range reKV.FindAllStringSubmatch(text, -1) {
		if len(m) < 3 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(m[1]))
		v := strings.TrimSpace(m[2])
		if d, ok := parseDate(v); ok {
			task.Fields[k] = d
		} else if n, err := strconv.Atoi(v); err == nil {
			task.Fields[k] = n
		} else {
			task.Fields[k] = v
		}
	}

	for _, m := range reKVParen.FindAllStringSubmatch(text, -1) {
		if len(m) < 3 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(m[1]))
		v := strings.TrimSpace(m[2])
		if d, ok := parseDate(v); ok {
			task.Fields[k] = d
		} else if n, err := strconv.Atoi(v); err == nil {
			task.Fields[k] = n
		} else {
			task.Fields[k] = v
		}
	}

	if task.Due == nil {
		if d, ok := task.Fields["due"].(time.Time); ok {
			task.Due = &d
		}
	}
	if task.Scheduled == nil {
		if d, ok := task.Fields["scheduled"].(time.Time); ok {
			task.Scheduled = &d
		}
	}
	if task.StartDate == nil {
		if d, ok := task.Fields["start"].(time.Time); ok {
			task.StartDate = &d
		}
	}
	if task.DoneAt == nil {
		if d, ok := task.Fields["completion"].(time.Time); ok {
			task.DoneAt = &d
		}
	}
	if task.Created == nil {
		if d, ok := task.Fields["created"].(time.Time); ok {
			task.Created = &d
		}
	}
	if p, ok := task.Fields["priority"]; ok {
		if n, err := strconv.Atoi(fmt.Sprintf("%v", p)); err == nil {
			task.Priority = n
		}
	}
	task.Text = strings.TrimSpace(stripMetadata(text))
}

var (
	reTaskLineDate = regexp.MustCompile(`([📅⏳🛫✅❌➕])\s*(\d{4}-\d{2}-\d{2})`)
	reKV           = regexp.MustCompile(`\[\s*([^:]+?)\s*::\s*([^\]]+)\]`)
	reKVParen      = regexp.MustCompile(`\(\s*([^:]+?)\s*::\s*([^\)]+)\)`)
)

func buildSections(doc *model.Document) {
	if doc.FrontMatterTo >= len(doc.Lines) {
		return
	}
	top := []int{}
	for i, h := range doc.Headings {
		if !h.Nested {
			top = append(top, i)
		}
	}

	doc.Sections = append(doc.Sections, model.Section{
		File: doc.Path, Start: maxInt(doc.FrontMatterTo+1, 1), End: len(doc.Lines),
	})
	slugUsed := map[string]int{}
	stack := []int{0}
	for _, idx := range top {
		h := &doc.Headings[idx]
		for len(stack) > 1 && doc.Sections[stack[len(stack)-1]].Level >= h.Level {
			doc.Sections[stack[len(stack)-1]].End = h.Start - 1
			stack = stack[:len(stack)-1]
		}
		parent := &doc.Sections[stack[len(stack)-1]]
		parent.Children++
		path := h.Title
		if parent.Path != "" {
			path = parent.Path + " > " + path
		}
		doc.Sections = append(doc.Sections, model.Section{
			File: doc.Path, Level: h.Level, Title: h.Title, Text: h.Title,
			Slug: uniqueSlug(slugUsed, slugify(h.Title)), Path: path,
			Start: h.Start, End: len(doc.Lines), Parent: parent.Title, Depth: parent.Depth + 1,
		})
		stack = append(stack, len(doc.Sections)-1)
	}

	for si := range doc.Sections {
		section := &doc.Sections[si]
		bodyStart := section.Start
		if si > 0 {
			h := &doc.Headings[top[si-1]]
			bodyStart = h.End + 1
			h.Slug, h.Path, h.Parent, h.Depth = section.Slug, section.Path, section.Parent, section.Depth
			h.Section, h.SectionEnd = section.Title, section.End
		}
		ownEnd := section.End
		if section.Children > 0 {
			ownEnd = doc.Sections[si+1].Start - 1
		}
		section.Raw = strings.Join(lineSlice(doc.Lines, section.Start, section.End), "\n")
		if section.End == len(doc.Lines) && strings.HasSuffix(doc.Content, "\n") {
			section.Raw += "\n"
		}
		section.Body = strings.TrimSpace(strings.Join(lineSlice(doc.Lines, bodyStart, section.End), "\n"))
		section.Own = strings.TrimSpace(strings.Join(lineSlice(doc.Lines, bodyStart, ownEnd), "\n"))
		section.Lines = maxInt(section.End-section.Start+1, 0)
	}

	for i := range doc.Headings {
		if doc.Headings[i].Nested {
			if section := sectionByLine(doc.Sections, doc.Headings[i].Start); section != nil {
				doc.Headings[i].Section = section.Title
				doc.Headings[i].Path = section.Path
				doc.Headings[i].SectionEnd = section.End
			}
		}
	}

	type counts struct{ code, tasks, open int }
	prefix := make([]counts, len(doc.Lines)+1)
	for _, block := range doc.Blocks {
		if block.Kind == model.KindCode {
			// Fenced code cannot cross a section boundary.
			prefix[block.Start].code++
		}
	}
	for _, task := range doc.Tasks {
		prefix[task.Line].tasks++
		if !task.Done {
			prefix[task.Line].open++
		}
	}
	for i := 1; i < len(prefix); i++ {
		prefix[i].code += prefix[i-1].code
		prefix[i].tasks += prefix[i-1].tasks
		prefix[i].open += prefix[i-1].open
	}
	for i := range doc.Sections {
		section := &doc.Sections[i]
		before, end := prefix[section.Start-1], prefix[section.End]
		section.CodeBlocks = end.code - before.code
		section.Tasks = end.tasks - before.tasks
		section.OpenTasks = end.open - before.open
	}
}

func attachSections(doc *model.Document) {
	for i := range doc.Blocks {
		s := sectionByLine(doc.Sections, doc.Blocks[i].Start)
		if s != nil {
			doc.Blocks[i].Section = s.Title
			doc.Blocks[i].Path = s.Path
		}
	}
	for i := range doc.Tasks {
		s := sectionByLine(doc.Sections, doc.Tasks[i].Line)
		if s != nil {
			doc.Tasks[i].Section = s.Title
			doc.Tasks[i].SectionPath = s.Path
		}
	}
	for i := range doc.Quotes {
		s := sectionByLine(doc.Sections, doc.Quotes[i].Start)
		if s != nil {
			doc.Quotes[i].Section = s.Title
			doc.Quotes[i].Path = s.Path
		}
	}
}

func collectLineRows(doc *model.Document) {
	blockIndex := 0
	for i, line := range doc.Lines {
		ln := i + 1
		kind := "blank"
		if ln >= doc.FrontMatterFrom && ln <= doc.FrontMatterTo {
			kind = model.KindFrontmatter
		}
		for blockIndex < len(doc.Blocks) && doc.Blocks[blockIndex].End < ln {
			blockIndex++
		}
		if blockIndex < len(doc.Blocks) && doc.Blocks[blockIndex].Start <= ln {
			kind = doc.Blocks[blockIndex].Kind
		}
		doc.LinesRows = append(doc.LinesRows, model.Line{File: doc.Path, N: ln, Text: line, Kind: kind})
	}
	for i := range doc.LinesRows {
		s := sectionByLine(doc.Sections, doc.LinesRows[i].N)
		if s != nil {
			doc.LinesRows[i].Section = s.Title
			doc.LinesRows[i].Path = s.Path
		}
	}
}

func extractTagsAndLinks(doc *model.Document, lines []string, quotedCode [][2]int) {
	blockIndex, codeIndex := 0, 0
	for i, line := range lines {
		ln := i + 1
		if ln >= doc.FrontMatterFrom && ln <= doc.FrontMatterTo {
			continue
		}
		for codeIndex < len(quotedCode) && quotedCode[codeIndex][1] < ln {
			codeIndex++
		}
		if codeIndex < len(quotedCode) && quotedCode[codeIndex][0] <= ln {
			continue
		}
		for blockIndex < len(doc.Blocks) && doc.Blocks[blockIndex].End < ln {
			blockIndex++
		}
		if blockIndex < len(doc.Blocks) && doc.Blocks[blockIndex].Kind == model.KindCode && doc.Blocks[blockIndex].Start <= ln {
			continue
		}
		for _, m := range reTag.FindAllStringSubmatch(line, -1) {
			if len(m) > 1 {
				doc.Tags = append(doc.Tags, model.Tag{File: doc.Path, Tag: m[1], Source: "body", Line: ln})
			}
		}
		var occupied [][2]int
		overlaps := func(start, end int) bool {
			for _, span := range occupied {
				if start < span[1] && end > span[0] {
					return true
				}
			}
			return false
		}
		for _, m := range reLinkRef.FindAllStringSubmatchIndex(line, -1) {
			raw := line[m[0]:m[1]]
			occupied = append(occupied, [2]int{m[0], m[1]})
			target, anchor, blockRef, alias := parseWikilink(line[m[2]:m[3]])
			kind := "wiki"
			if strings.HasPrefix(raw, "![[") {
				kind = "embed"
			}
			doc.Links = append(doc.Links, model.Link{File: doc.Path, Type: kind, Target: target, Anchor: anchor, BlockRef: blockRef, Text: alias, Raw: raw, Line: ln, External: false})
		}
		for _, m := range reMdLink.FindAllStringSubmatchIndex(line, -1) {
			if overlaps(m[0], m[1]) {
				continue
			}
			raw := line[m[0]:m[1]]
			occupied = append(occupied, [2]int{m[0], m[1]})
			target, anchor := splitAnchor(line[m[4]:m[5]])
			typeName := "md"
			if strings.HasPrefix(raw, "![") {
				typeName = "image"
			}
			doc.Links = append(doc.Links, model.Link{File: doc.Path, Type: typeName, Target: target, Anchor: anchor, Text: line[m[2]:m[3]], Raw: raw, Line: ln, External: isExternal(target)})
		}
		for _, m := range reAutolink.FindAllStringIndex(line, -1) {
			if overlaps(m[0], m[1]) {
				continue
			}
			u := line[m[0]:m[1]]
			occupied = append(occupied, [2]int{m[0], m[1]})
			t := strings.Trim(u, "<>")
			doc.Links = append(doc.Links, model.Link{File: doc.Path, Type: "autolink", Target: t, Text: t, Raw: u, Line: ln, External: true})
		}
		for _, m := range reBareURL.FindAllStringIndex(line, -1) {
			if overlaps(m[0], m[1]) {
				continue
			}
			u := trimURLPunctuation(line[m[0]:m[1]])
			doc.Links = append(doc.Links, model.Link{File: doc.Path, Type: "autolink", Target: u, Text: u, Raw: u, Line: ln, External: true})
		}
	}
	for _, tag := range parseFrontmatterTags(doc.FrontMatter) {
		doc.Tags = append(doc.Tags, model.Tag{File: doc.Path, Tag: tag, Source: "frontmatter", Line: maxInt(doc.FrontMatterTo, 1)})
	}

	for i := range doc.Tags {
		s := sectionByLine(doc.Sections, doc.Tags[i].Line)
		if s != nil {
			doc.Tags[i].Section = s.Title
			doc.Tags[i].Path = s.Path
		}
	}
	for i := range doc.Links {
		if doc.Links[i].Source == "" {
			doc.Links[i].Source = "body"
		}
		s := sectionByLine(doc.Sections, doc.Links[i].Line)
		if s != nil {
			doc.Links[i].Section = s.Title
			doc.Links[i].Path = s.Path
		}
		if strings.TrimSpace(doc.Links[i].BlockRef) != "" {
			doc.Links[i].BlockRef = strings.TrimSpace(doc.Links[i].BlockRef)
		}
	}
}

func normalizeFrontmatterDates(node *yaml.Node) {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!timestamp" {
		if date, err := time.Parse("2006-1-2", node.Value); err == nil {
			node.Tag = "!!str"
			node.Value = date.Format(time.DateOnly)
		}
	}
	for _, child := range node.Content {
		normalizeFrontmatterDates(child)
	}
}

func extractFrontmatterLinks(doc *model.Document, node *yaml.Node, key string) {
	switch node.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range node.Content {
			extractFrontmatterLinks(doc, child, key)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			childKey := node.Content[i].Value
			if key != "" {
				childKey = key + "." + childKey
			}
			extractFrontmatterLinks(doc, node.Content[i+1], childKey)
		}
	case yaml.ScalarNode:
		if node.Tag != "!!str" {
			return
		}
		for _, match := range reLinkRef.FindAllStringSubmatch(node.Value, -1) {
			target, anchor, blockRef, alias := parseWikilink(match[1])
			kind := "wiki"
			if strings.HasPrefix(match[0], "![[") {
				kind = "embed"
			}
			doc.Links = append(doc.Links, model.Link{
				File: doc.Path, Type: kind, Source: "frontmatter", Key: key,
				Target: target, Anchor: anchor, BlockRef: blockRef, Text: alias,
				Raw: match[0], Line: node.Line + 1,
			})
		}
	}
}

func trimURLPunctuation(url string) string {
	unmatched := strings.Count(url, ")") - strings.Count(url, "(")
	for len(url) > 0 {
		switch url[len(url)-1] {
		case '.', ',', ';':
			url = url[:len(url)-1]
		case ')':
			if unmatched <= 0 {
				return url
			}
			unmatched--
			url = url[:len(url)-1]
		default:
			return url
		}
	}
	return url
}

func parseFrontmatterTags(fm model.FrontMatter) []string {
	if fm == nil {
		return nil
	}
	vals := []string{}
	add := func(v interface{}) {
		switch x := v.(type) {
		case string:
			if strings.Contains(x, ",") {
				for _, p := range strings.Split(x, ",") {
					vals = append(vals, strings.TrimSpace(p))
				}
			} else {
				vals = append(vals, strings.TrimSpace(x))
			}
		case []interface{}:
			for _, p := range x {
				if s, ok := p.(string); ok {
					vals = append(vals, strings.TrimSpace(s))
				}
			}
		case []string:
			for _, p := range x {
				vals = append(vals, strings.TrimSpace(p))
			}
		}
	}
	if v, ok := fm["tags"]; ok {
		add(v)
	}
	if v, ok := fm["tag"]; ok {
		add(v)
	}
	res := []string{}
	for _, v := range vals {
		if v != "" {
			res = append(res, v)
		}
	}
	return res
}

func extractTagsFromText(text string) []string {
	t := []string{}
	for _, m := range reTag.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			t = append(t, m[1])
		}
	}
	return t
}

func parseWikilink(raw string) (target, anchor, blockRef, alias string) {
	target = raw
	if i := strings.Index(raw, "|"); i >= 0 {
		alias = raw[i+1:]
		target = raw[:i]
	}
	if i := strings.Index(target, "^"); i >= 0 {
		blockRef = target[i+1:]
		target = target[:i]
	}
	if i := strings.Index(target, "#"); i >= 0 {
		anchor = target[i+1:]
		target = target[:i]
	}
	if alias == "" {
		alias = target
	}
	return
}

func parseCalloutFirstLine(lines []string) (calloutType, title, fold string) {
	if len(lines) == 0 {
		return "", "", ""
	}
	match := regexp.MustCompile(`^\s*\[!([^\]]+)\]([+-])?\s*(.*)$`).FindStringSubmatch(lines[0])
	if len(match) == 0 {
		return "", "", ""
	}
	calloutType = strings.ToLower(strings.TrimSpace(match[1]))
	fold = match[2]
	title = strings.TrimSpace(match[3])
	return calloutType, title, fold
}

func sectionByLine(sections []model.Section, line int) *model.Section {
	i := sort.Search(len(sections), func(i int) bool { return sections[i].Start > line }) - 1
	if i >= 0 && line <= sections[i].End {
		return &sections[i]
	}
	return nil
}

func isSetextHeading(lines []string, i int) bool {
	if i+1 >= len(lines) {
		return false
	}
	if strings.TrimSpace(lines[i]) == "" {
		return false
	}
	return reSetextLine.MatchString(strings.TrimSpace(lines[i+1]))
}

func parseCodeFence(line string) (fence, info string) {
	trim := strings.TrimLeft(line, " ")
	if len(line)-len(trim) > 3 || len(trim) < 3 || (trim[0] != '`' && trim[0] != '~') {
		return "", ""
	}
	n := 1
	for n < len(trim) && trim[n] == trim[0] {
		n++
	}
	if n < 3 {
		return "", ""
	}
	info = strings.TrimSpace(trim[n:])
	if trim[0] == '`' && strings.ContainsRune(info, '`') {
		return "", ""
	}
	return trim[:n], info
}

func leadingWhitespace(line string) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, s)
	return t, err == nil
}

func stripMetadata(s string) string {
	s = reTaskLineDate.ReplaceAllString(s, "")
	s = regexp.MustCompile(`[\[\(][^\]\)]+::[^\]\)]+[\]\)]`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`🔁\s*[^\n]+`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`🆔\s*\S+`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`⛔\s*[^\s,]+(?:\s*,\s*[^\s,]+)*`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(🔺|⏫|🔼|🔽|⏬)`).ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func splitAnchor(raw string) (string, string) {
	if i := strings.Index(raw, "#"); i >= 0 {
		return raw[:i], raw[i+1:]
	}
	return raw, ""
}

func isExternal(raw string) bool {
	s := strings.ToLower(raw)
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "mailto:")
}

func lineSlice(lines []string, start, end int) []string {
	if len(lines) == 0 {
		return []string{}
	}
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return []string{}
	}
	return lines[start-1 : end]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func uniqueSlug(used map[string]int, base string) string {
	if base == "" {
		base = "section"
	}
	cnt := used[base]
	used[base] = cnt + 1
	if cnt == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, cnt)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	b := strings.Builder{}
	dash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		case r == ' ' || r == '-':
			if !dash && b.Len() > 0 {
				b.WriteRune('-')
				dash = true
			}
		}
	}
	res := strings.Trim(b.String(), "-")
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	if res == "" {
		res = "section"
	}
	return res
}

func stripFormatting(s string) string {
	s = regexp.MustCompile(`\*\*(.*?)\*\*`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile("`([^`]*)`").ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`\[([^\]]+)\]\([^\)]*\)`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`\[\[(.*?)\]\]`).ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}
