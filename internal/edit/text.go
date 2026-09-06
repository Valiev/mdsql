package edit

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

var checkboxPattern = regexp.MustCompile(`^(?:[ \t]*>[ \t]?)*[ \t]*(?:[-*+][ \t]+|[0-9]+\.[ \t]+)\[(.)\]`)
var listPattern = regexp.MustCompile(`^([ ]*)(?:[-*+][ \t]+|[0-9]+\.[ \t]+)`)

func stringValue(values map[string]query.Value, key string, required bool) (string, error) {
	value, ok := values[key]
	if !ok {
		if required {
			return "", fmt.Errorf("%s is required", key)
		}
		return "", nil
	}
	text, ok := value.Data.(string)
	if value.Kind != query.ValueString || !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return text, nil
}
func integerValue(values map[string]query.Value, key string) (int, error) {
	value, ok := values[key]
	if !ok || value.Kind != query.ValueInt {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	n, ok := value.Data.(int64)
	if !ok || int64(int(n)) != n {
		return 0, fmt.Errorf("%s is out of range", key)
	}
	return int(n), nil
}
func taskStatus(values map[string]query.Value) (string, error) {
	if _, ok := values["state"]; ok {
		state, err := stringValue(values, "state", true)
		if err != nil {
			return "", err
		}
		status, ok := map[string]string{"todo": " ", "done": "x", "in_progress": "/", "cancelled": "-"}[state]
		if !ok {
			return "", fmt.Errorf("unsupported task state %q", state)
		}
		return status, nil
	}
	if _, ok := values["status"]; ok {
		status, err := stringValue(values, "status", true)
		if err != nil {
			return "", err
		}
		switch status {
		case " ", "x", "X", "/", "-":
		default:
			return "", fmt.Errorf("unsupported task status %q", status)
		}
		return status, nil
	}
	return " ", nil
}

func planText(original []byte, stmt *query.WriteStatement, doc *model.Document) ([]byte, int, int, error) {
	source := indexBytes(original)
	if stmt.Operation == "UPDATE" {
		return planTasksUpdate(source, stmt, doc)
	}
	if err := frontmatterHealthy(source, doc); err != nil {
		return nil, 0, 0, err
	}
	line, indent, err := resolvePosition(source, stmt, doc)
	if err != nil {
		return nil, 0, 0, err
	}
	continueTaskList := false
	if stmt.Table == "tasks" && hasSectionSelector(stmt) && stmt.Values["position"].String() != "section_end" {
		line, indent, continueTaskList = sectionTaskPosition(source, doc, stmt, line)
	}
	if doc.FrontMatterTo > 0 && line <= doc.FrontMatterTo {
		return nil, 0, 0, fmt.Errorf("position is inside frontmatter; insert after line %d", doc.FrontMatterTo)
	}
	structured := stmt.Table != "lines"
	if structured {
		if err := structuralBoundary(source, doc, line, indent != "" || continueTaskList); err != nil {
			return nil, 0, 0, err
		}
	}
	var text, taskText, title, code, lang, status string
	level := 0
	switch stmt.Table {
	case "lines":
		text, err = stringValue(stmt.Values, "text", true)
		if err == nil && text == "" {
			err = fmt.Errorf("text must not be empty")
		}
	case "tasks":
		taskText, err = stringValue(stmt.Values, "text", true)
		if err == nil && (strings.TrimSpace(taskText) == "" || strings.ContainsAny(taskText, "\r\n")) {
			err = fmt.Errorf("task text must be one nonempty line")
		}
		if err == nil && taskTextMarker.MatchString(taskText) {
			err = fmt.Errorf("task text must contain content without a list marker or checkbox")
		}
		if err == nil {
			status, err = taskStatus(stmt.Values)
		}
		if err == nil {
			err = taskMetadataConflict(taskText, stmt.Values)
		}
		metadata := ""
		if err == nil {
			metadata, err = taskInsertMetadata(stmt.Values)
		}
		text = indent + insertionTaskMarker(source, doc, stmt, line, indent) + "[" + status + "] " + taskText + metadata
	case "sections":
		level, err = integerValue(stmt.Values, "level")
		if err == nil && (level < 1 || level > 6) {
			err = fmt.Errorf("section level must be between 1 and 6")
		}
		if err == nil {
			title, err = stringValue(stmt.Values, "title", true)
		}
		if err == nil && (strings.TrimSpace(title) == "" || strings.ContainsAny(title, "\r\n")) {
			err = fmt.Errorf("section title must be one nonempty line")
		}
		if err == nil {
			text, err = stringValue(stmt.Values, "body", false)
		}
		if err == nil {
			if text != "" {
				text = "\n\n" + text
			}
			text = strings.Repeat("#", level) + " " + title + text
		}
	case "code":
		code, err = stringValue(stmt.Values, "code", true)
		if err == nil {
			lang, err = stringValue(stmt.Values, "lang", false)
		}
		if err == nil && (strings.ContainsAny(lang, " \t\r\n`~") || strings.ContainsRune(lang, 0)) {
			err = fmt.Errorf("lang must be a single fence language token")
		}
		code = normalizeNewlines(code)
		// Longer than every backtick run in the payload, including an indented closer.
		width, run := 3, 0
		for _, c := range code {
			if c == '`' {
				run++
				if run >= width {
					width = run + 1
				}
			} else {
				run = 0
			}
		}
		fence := strings.Repeat("`", width)
		text = fence + lang + "\n"
		if code != "" {
			text += code + "\n"
		}
		text += fence
	}
	if err != nil {
		return nil, 0, 0, err
	}
	if structured && stmt.Table == "sections" {
		fragment, parseErr := parser.ParseFile(doc.Path, text, int64(len(text)), doc.ModTime)
		if parseErr != nil {
			return nil, 0, 0, parseErr
		}
		if boundaryErr := structuralBoundary(indexBytes([]byte(text)), fragment, len(fragment.Lines)+1, false); boundaryErr != nil {
			return nil, 0, 0, fmt.Errorf("section body: %w", boundaryErr)
		}
	}
	// An explicit physical line is exact: formatting must not shift the payload.
	// Section/implicit EOF positions retain their default surrounding blank lines;
	// continuing an existing task list uses the list's own line spacing.
	_, exactLine := stmt.Values["line"]
	data, first, err := source.insertion(line, text, structured && !exactLine, indent != "" || continueTaskList)
	if err != nil {
		return nil, 0, 0, err
	}
	at, _ := source.offset(line)
	after, err := applyPatches(original, []bytePatch{{at, at, data}})
	if err != nil {
		return nil, 0, 0, err
	}
	next, err := parser.ParseFile(doc.Path, string(after), int64(len(after)), doc.ModTime)
	if err != nil {
		return nil, 0, 0, err
	}
	if err := frontmatterHealthy(indexBytes(after), next); err != nil {
		return nil, 0, 0, fmt.Errorf("insertion would produce invalid frontmatter: %w", err)
	}
	if structured {
		if err = preserveBlocks(doc, next, line, len(next.Lines)-len(doc.Lines)); err != nil {
			return nil, 0, 0, err
		}
		found := false
		switch stmt.Table {
		case "tasks":
			for _, task := range next.Tasks {
				if task.Line == first && task.Status == status && task.Container == "" && task.Raw == normalizeNewlines(text) && taskMatchesWrite(task, stmt.Values) {
					found = true
				}
			}
		case "sections":
			for _, heading := range next.Headings {
				if heading.Start == first && heading.Level == level && heading.Title == title && !heading.Nested {
					found = true
				}
			}
		case "code":
			for _, block := range next.Blocks {
				if block.Start == first && block.Kind == model.KindCode && block.Text == code && block.Lang == strings.ToLower(lang) {
					found = true
				}
			}
		}
		if !found {
			return nil, 0, 0, fmt.Errorf("inserted %s does not round-trip through the Markdown parser", stmt.Table)
		}
	}
	changed := 1
	if bytes.Equal(original, after) {
		changed = 0
	}
	return after, 1, changed, nil
}

func planTasksUpdate(source sourceBytes, stmt *query.WriteStatement, doc *model.Document) ([]byte, int, int, error) {
	if err := frontmatterHealthy(source, doc); err != nil {
		return nil, 0, 0, err
	}
	rows, err := selectWriteRows(stmt, doc)
	if err != nil {
		return nil, 0, 0, err
	}
	patches := []bytePatch{}
	selected := map[int]bool{}
	for _, row := range rows {
		n, err := integerValue(row, "line")
		if err != nil {
			return nil, 0, 0, err
		}
		if n < 1 || n > len(source.lines) || selected[n] {
			return nil, 0, 0, fmt.Errorf("invalid or duplicate task line %d", n)
		}
		selected[n] = true
		raw := source.text(n)
		updated, err := updateTaskLine(raw, stmt.Values)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("task line %d: %w", n, err)
		}
		if raw == updated {
			continue
		}
		line := source.lines[n-1]
		patches = append(patches, bytePatch{line.start, line.end, []byte(updated)})
	}
	after, err := applyPatches(source.raw, patches)
	if err != nil {
		return nil, 0, 0, err
	}
	next, err := parser.ParseFile(doc.Path, string(after), int64(len(after)), doc.ModTime)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(next.Tasks) != len(doc.Tasks) {
		return nil, 0, 0, fmt.Errorf("checkbox update changed task structure")
	}
	found := 0
	for _, task := range next.Tasks {
		if selected[task.Line] {
			found++
			if !taskMatchesWrite(task, stmt.Values) {
				return nil, 0, 0, fmt.Errorf("task update does not round-trip on line %d", task.Line)
			}
		}
	}
	if found != len(selected) {
		return nil, 0, 0, fmt.Errorf("task update changed selected task structure")
	}
	return after, len(rows), len(patches), nil
}

func resolvePosition(source sourceBytes, stmt *query.WriteStatement, doc *model.Document) (int, string, error) {
	if _, ok := stmt.Values["line"]; ok {
		line, err := integerValue(stmt.Values, "line")
		if err != nil {
			return 0, "", err
		}
		_, err = source.offset(line)
		return line, "", err
	}
	if _, ok := stmt.Values["parent_line"]; ok {
		line, err := integerValue(stmt.Values, "parent_line")
		if err != nil {
			return 0, "", err
		}
		return childPosition(source, doc, line)
	}
	selector := ""
	for _, key := range []string{"section", "section_path", "heading_line"} {
		if _, ok := stmt.Values[key]; ok {
			selector = key
		}
	}
	if selector == "" {
		return len(source.lines) + 1, "", nil
	}
	var value string
	headingLine := 0
	var err error
	if selector == "heading_line" {
		headingLine, err = integerValue(stmt.Values, selector)
	} else {
		value, err = stringValue(stmt.Values, selector, true)
	}
	if err != nil {
		return 0, "", err
	}
	matches := []model.Section{}
	for _, section := range doc.Sections {
		if section.Level == 0 {
			continue
		}
		if selector == "section" && section.Title == value || selector == "section_path" && section.Path == value || selector == "heading_line" && section.Start == headingLine {
			matches = append(matches, section)
		}
	}
	if len(matches) == 0 {
		return 0, "", fmt.Errorf("%s matched no section; check the section title or path", selector)
	}
	if len(matches) > 1 {
		return 0, "", fmt.Errorf("%s must identify exactly one section; matched %d (use heading_line to disambiguate)", selector, len(matches))
	}
	section := matches[0]
	line := section.End + 1
	// Own ends before the first descendant, not at the recursive section end.
	for _, heading := range doc.Headings {
		if !heading.Nested && heading.Start > section.Start && heading.Start < line {
			line = heading.Start
		}
	}
	minLine := section.Start + 1
	for _, heading := range doc.Headings {
		if heading.Start == section.Start {
			minLine = heading.End + 1
			break
		}
	}
	for line > minLine && strings.TrimSpace(source.text(line-1)) == "" {
		line--
	}
	return line, "", nil
}

func hasSectionSelector(stmt *query.WriteStatement) bool {
	for _, key := range []string{"section", "section_path", "heading_line"} {
		if _, ok := stmt.Values[key]; ok {
			return true
		}
	}
	return false
}

// sectionTaskPosition continues the last unquoted task list in the selected
// section. The supplied fallback is the section's normal own-text end.
func sectionTaskPosition(source sourceBytes, doc *model.Document, stmt *query.WriteStatement, fallback int) (int, string, bool) {
	sectionStart := 0
	for _, section := range doc.Sections {
		matches := false
		if value, ok := stmt.Values["section"]; ok {
			matches = section.Title == value.String()
		}
		if value, ok := stmt.Values["section_path"]; ok {
			matches = section.Path == value.String()
		}
		if value, ok := stmt.Values["heading_line"]; ok {
			matches = section.Start == int(value.Data.(int64))
		}
		if section.Level > 0 && matches {
			sectionStart = section.Start
			break
		}
	}
	lastLine := 0
	for _, task := range doc.Tasks {
		if task.Container == "" && task.Line > sectionStart && task.Line < fallback && task.Line > lastLine {
			lastLine = task.Line
		}
	}
	if lastLine == 0 {
		return fallback, "", false
	}
	match := listStylePattern.FindStringSubmatch(source.text(lastLine))
	if match == nil {
		return fallback, "", false
	}
	indent := match[1]
	end := lastLine
	for n := lastLine + 1; n < fallback; n++ {
		text := source.text(n)
		if strings.TrimSpace(text) == "" {
			break
		}
		if item := listStylePattern.FindStringSubmatch(text); item != nil {
			if len(item[1]) < len(indent) {
				break
			}
			end = n
			continue
		}
		leading := len(text) - len(strings.TrimLeft(text, " "))
		if leading <= len(indent) || strings.ContainsRune(text[:leading], '\t') {
			break
		}
		end = n
	}
	return end + 1, indent, true
}

func childPosition(source sourceBytes, doc *model.Document, parentLine int) (int, string, error) {
	found := false
	for _, task := range doc.Tasks {
		if task.Line == parentLine && task.Container == "" {
			found = true
		}
	}
	if !found {
		return 0, "", fmt.Errorf("parent_line must identify an unquoted task")
	}
	raw := source.text(parentLine)
	match := listPattern.FindStringSubmatchIndex(raw)
	if match == nil || strings.ContainsRune(raw[:match[1]], '\t') {
		return 0, "", fmt.Errorf("cannot safely infer parent indentation")
	}
	base := match[3] - match[2]
	childIndent := 0
	end := parentLine
	for n := parentLine + 1; n <= len(source.lines); n++ {
		text := source.text(n)
		if strings.TrimSpace(text) == "" {
			continue
		}
		leading := len(text) - len(strings.TrimLeft(text, " \t"))
		if strings.ContainsRune(text[:leading], '\t') {
			return 0, "", fmt.Errorf("cannot safely infer mixed tab indentation")
		}
		if leading <= base {
			break
		}
		end = n
		if listPattern.MatchString(text) && (childIndent == 0 || leading < childIndent) {
			childIndent = leading
		}
	}
	if childIndent == 0 {
		childIndent = base + 2
		if match[1] > childIndent {
			childIndent = match[1]
		}
	}
	if childIndent <= base {
		return 0, "", fmt.Errorf("cannot safely infer child indentation")
	}
	return end + 1, strings.Repeat(" ", childIndent), nil
}

func frontmatterHealthy(source sourceBytes, doc *model.Document) error {
	if len(doc.Warnings) > 0 {
		return fmt.Errorf("source diagnostics prevent structural write: %s", strings.Join(doc.Warnings, "; "))
	}
	if len(source.lines) > 0 && strings.TrimSpace(source.text(1)) == "---" && doc.FrontMatterTo == 0 {
		return fmt.Errorf("unterminated frontmatter prevents structural write")
	}
	return nil
}

func structuralBoundary(source sourceBytes, doc *model.Document, line int, child bool) error {
	if err := frontmatterHealthy(source, doc); err != nil {
		return err
	}
	if doc.FrontMatterTo > 0 && line <= doc.FrontMatterTo {
		return fmt.Errorf("position is inside frontmatter")
	}
	for _, block := range doc.Blocks {
		if block.Start < line && line <= block.End {
			return fmt.Errorf("position is inside %s", block.Kind)
		}
		if block.Kind == model.KindCode && block.Start < line && line == block.End+1 {
			last := strings.TrimSpace(source.text(block.End))
			run := 0
			if len(block.Fence) > 0 {
				for run < len(last) && last[run] == block.Fence[0] {
					run++
				}
			}
			if block.Start == block.End || run < len(block.Fence) || strings.TrimSpace(last[run:]) != "" {
				return fmt.Errorf("position is inside an unterminated code fence")
			}
		}
	}
	// The lightweight table parser indexes paragraphs and lists line by line.
	// Reject cuts through their continuous text or a list item's descendants.
	if !child && line > 1 && line <= len(source.lines) {
		previous, current := source.text(line-1), source.text(line)
		if strings.TrimSpace(previous) != "" && strings.TrimSpace(current) != "" {
			prevKind, curKind := "", ""
			for _, b := range doc.Blocks {
				if b.Start <= line-1 && line-1 <= b.End {
					prevKind = b.Kind
				}
				if b.Start <= line && line <= b.End {
					curKind = b.Kind
				}
			}
			if prevKind == model.KindParagraph && curKind == model.KindParagraph {
				return fmt.Errorf("position splits a paragraph")
			}
			if strings.HasPrefix(current, " ") || strings.HasPrefix(current, "\t") {
				return fmt.Errorf("position splits an indented construction")
			}
		}
	}
	// Unclosed comments and display math can swallow inserted Markdown even
	// though the query parser represents them as individual paragraph rows.
	comment, math := false, false
	var rawHTMLClose *regexp.Regexp
	for n := doc.FrontMatterTo + 1; n < line; n++ {
		inCode := false
		for _, b := range doc.Blocks {
			if b.Kind == model.KindCode && b.Start <= n && n <= b.End {
				inCode = true
				break
			}
		}
		if inCode {
			continue
		}
		text := source.text(n)
		if rawHTMLClose == nil && !comment {
			if match := rawHTMLOpen.FindStringSubmatch(text); match != nil {
				rawHTMLClose = regexp.MustCompile(`(?i)</` + match[1] + `[ \t]*>`)
			}
		}
		if rawHTMLClose != nil {
			if rawHTMLClose.MatchString(text) {
				rawHTMLClose = nil
			}
			continue
		}
		for rest := text; rest != ""; {
			if comment {
				at := strings.Index(rest, "-->")
				if at < 0 {
					break
				}
				comment = false
				rest = rest[at+3:]
			} else {
				at := strings.Index(rest, "<!--")
				if at < 0 {
					break
				}
				comment = true
				rest = rest[at+4:]
			}
		}
		if strings.TrimSpace(text) == "$$" {
			math = !math
		}
	}
	if comment || math || rawHTMLClose != nil {
		return fmt.Errorf("position is inside an unfinished Markdown construction")
	}
	return nil
}

func preserveBlocks(before, after *model.Document, line, shift int) error {
	byLine := map[int]model.Block{}
	for _, b := range after.Blocks {
		byLine[b.Start] = b
	}
	for _, old := range before.Blocks {
		start, end := old.Start, old.End
		if start >= line {
			start += shift
			end += shift
		}
		next, ok := byLine[start]
		if !ok || next.Kind != old.Kind || next.End != end || next.Raw != old.Raw {
			return fmt.Errorf("insertion would change the existing %s block at line %d", old.Kind, old.Start)
		}
	}
	return nil
}

var rawHTMLOpen = regexp.MustCompile(`(?i)^ {0,3}<(script|pre|style|textarea)(?:[ \t>]|$)`)

var taskTextMarker = regexp.MustCompile(`^[ \t]*(?:[-*+](?:[ \t]|$)|[0-9]+[.)](?:[ \t]|$)|\[.\](?:[ \t]|$))`)
var listStylePattern = regexp.MustCompile(`^( *)([-*+]|[0-9]+\.)([ \t]+)`)

// Prefer the preceding sibling's marker and spacing, or the following sibling
// when inserting before the first item. Never borrow style across a non-list
// block or a shallower list. Ordered markers are copied, not renumbered.
func insertionTaskMarker(source sourceBytes, doc *model.Document, stmt *query.WriteStatement, line int, indent string) string {
	lists := make(map[int]bool)
	for _, block := range doc.Blocks {
		if block.Kind == model.KindList {
			lists[block.Start] = true
		}
	}
	sibling := func(start, step int) string {
		for n := start; n >= 1 && n <= len(source.lines); n += step {
			text := source.text(n)
			if strings.TrimSpace(text) == "" {
				continue
			}
			depth := len(text) - len(strings.TrimLeft(text, " "))
			if depth < len(indent) {
				return ""
			}
			if depth > len(indent) {
				continue
			}
			match := listStylePattern.FindStringSubmatch(text)
			if !lists[n] || match == nil {
				return ""
			}
			return match[2] + match[3]
		}
		return ""
	}
	if marker := sibling(line-1, -1); marker != "" {
		return marker
	}
	if marker := sibling(line, 1); marker != "" {
		return marker
	}
	// A first child has no siblings yet; inherit the parent's marker style.
	if _, ok := stmt.Values["parent_line"]; ok {
		parent, err := integerValue(stmt.Values, "parent_line")
		if err == nil && parent >= 1 && parent <= len(source.lines) {
			if match := listStylePattern.FindStringSubmatch(source.text(parent)); match != nil {
				return match[2] + match[3]
			}
		}
	}
	return "- "
}
