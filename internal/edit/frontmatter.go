package edit

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
	"gopkg.in/yaml.v3"
)

// planFrontmatter edits top-level scalar values without serializing the document.
// Updating multiline scalars, explicit tags/keys, and flow-style root mappings
// is deliberately unsupported. Untouched ordinary collections are preserved.
func planFrontmatter(original []byte, stmt *query.WriteStatement, doc *model.Document) ([]byte, int, int, error) {
	if stmt == nil || doc == nil || stmt.Table != "frontmatter_values" {
		return nil, 0, 0, fmt.Errorf("frontmatter_values: invalid statement or document")
	}
	if stmt.Operation != "INSERT" && stmt.Operation != "UPDATE" {
		return nil, 0, 0, fmt.Errorf("frontmatter_values: unsupported operation %q", stmt.Operation)
	}
	for field := range stmt.Values {
		if field != "value" && !(stmt.Operation == "INSERT" && field == "key") {
			return nil, 0, 0, fmt.Errorf("frontmatter_values: unknown target field %q", field)
		}
	}
	value, ok := stmt.Values["value"]
	if !ok {
		return nil, 0, 0, fmt.Errorf("frontmatter_values: value is required")
	}
	literal, err := fmLiteral(value)
	if err != nil {
		return nil, 0, 0, err
	}
	if stmt.Operation == "UPDATE" && stmt.Where == nil {
		return nil, 0, 0, fmt.Errorf("frontmatter_values: UPDATE requires WHERE")
	}
	if stmt.Operation == "INSERT" && stmt.Where != nil {
		return nil, 0, 0, fmt.Errorf("frontmatter_values: INSERT does not accept WHERE")
	}
	fm, err := fmRead(original)
	if err != nil {
		return nil, 0, 0, err
	}
	if stmt.Operation == "INSERT" {
		key, exists := stmt.Values["key"]
		if !exists || key.Kind != query.ValueString {
			return nil, 0, 0, fmt.Errorf("frontmatter_values: INSERT requires a string key")
		}
		keyLiteral, err := fmKeyLiteral(key.String())
		if err != nil {
			return nil, 0, 0, err
		}
		if node, exists := fm.values[key.String()]; exists {
			if node.Kind != yaml.SequenceNode {
				return nil, 0, 0, fmt.Errorf("frontmatter_values: key %q already exists and is not a list; use UPDATE for a scalar", key.String())
			}
			return fmAppendSequence(original, fm, key.String(), node, value, literal)
		}
		entry := keyLiteral + ": " + literal + fm.eol
		at := fm.close
		if !fm.present {
			at = fm.bom
			entry = "---" + fm.eol + entry + "---" + fm.eol
		} else if fm.root != nil {
			entry = strings.Repeat(" ", fm.root.Column-1) + entry
		}
		return fmSplice(original, at, at, entry), 1, 1, nil
	}

	rows, err := selectWriteRows(stmt, doc)
	if err != nil {
		return nil, 0, 0, err
	}
	type replacement struct {
		start, end int
		text       string
	}
	var edits []replacement
	seenScalars := map[string]bool{}
	seenItems := map[string]map[int]bool{}
	for _, row := range rows {
		key := row["key"]
		if key.Kind != query.ValueString || fm.values[key.String()] == nil {
			return nil, 0, 0, fmt.Errorf("frontmatter_values: selected row has no matching source key")
		}
		name := key.String()
		node := fm.values[name]
		if node.Kind == yaml.SequenceNode {
			rowLiteral, err := fmLiteral(row["value"])
			if err != nil {
				return nil, 0, 0, fmt.Errorf("frontmatter_values: selected list item for %q: %w", name, err)
			}
			if seenItems[name] == nil {
				seenItems[name] = map[int]bool{}
			}
			item := -1
			for i, child := range node.Content {
				if !seenItems[name][i] && child.Kind == yaml.ScalarNode && fmSameScalar(child, rowLiteral) {
					item = i
					break
				}
			}
			if item < 0 {
				return nil, 0, 0, fmt.Errorf("frontmatter_values: selected row has no matching source list item for %q", name)
			}
			seenItems[name][item] = true
			child := node.Content[item]
			if fmSameScalar(child, literal) {
				continue
			}
			start, end, err := fm.sequenceScalarSpan(original, node, item)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("frontmatter_values: list key %q: %w", name, err)
			}
			text, err := fmStyledLiteral(value, child.Style)
			if err != nil {
				return nil, 0, 0, err
			}
			edits = append(edits, replacement{start, end, text})
			continue
		}
		if node.Kind != yaml.ScalarNode {
			return nil, 0, 0, fmt.Errorf("frontmatter_values: cannot UPDATE object key %q", name)
		}
		if seenScalars[name] {
			return nil, 0, 0, fmt.Errorf("frontmatter_values: duplicate selected key %q", name)
		}
		seenScalars[name] = true
		start, end, implicit, err := fm.scalarSpan(original, fm.keys[name], node)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("frontmatter_values: key %q: %w", name, err)
		}
		if fmSameScalar(node, literal) {
			continue
		}
		text, err := fmStyledLiteral(value, node.Style)
		if err != nil {
			return nil, 0, 0, err
		}
		if implicit {
			text = " " + text
		}
		edits = append(edits, replacement{start, end, text})
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	after := original
	for _, edit := range edits {
		after = fmSplice(after, edit.start, edit.end, edit.text)
	}
	return after, len(rows), len(edits), nil
}

func fmAppendSequence(source []byte, fm *fmSource, key string, node *yaml.Node, value query.Value, literal string) ([]byte, int, int, error) {
	for _, child := range node.Content {
		if child.Kind != yaml.ScalarNode {
			return nil, 0, 0, fmt.Errorf("frontmatter_values: list key %q contains a non-scalar item", key)
		}
		if fmSameScalar(child, literal) {
			return source, 1, 0, nil
		}
	}
	style := yaml.Style(0)
	if len(node.Content) != 0 {
		style = node.Content[len(node.Content)-1].Style
	}
	item, err := fmStyledLiteral(value, style)
	if err != nil {
		return nil, 0, 0, err
	}
	if node.Style&yaml.FlowStyle != 0 {
		open, close, err := fm.flowSequenceBounds(source, node)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("frontmatter_values: list key %q: %w", key, err)
		}
		at := close
		for at > open+1 && (source[at-1] == ' ' || source[at-1] == '\t') {
			at--
		}
		if len(node.Content) != 0 {
			item = ", " + item
		}
		return fmSplice(source, at, at, item), 1, 1, nil
	}
	if len(node.Content) == 0 {
		return nil, 0, 0, fmt.Errorf("frontmatter_values: empty block list %q has no insertion style", key)
	}
	last := node.Content[len(node.Content)-1]
	if last.Line < 1 || last.Line >= len(fm.lines) || strings.ContainsAny(last.Value, "\r\n") {
		return nil, 0, 0, fmt.Errorf("frontmatter_values: multiline list items are unsupported")
	}
	line := fm.lines[last.Line]
	start, err := fmColumn(source, line, last.Column)
	if err != nil {
		return nil, 0, 0, err
	}
	dash := start
	for dash > line.start && (source[dash-1] == ' ' || source[dash-1] == '\t') {
		dash--
	}
	if dash == line.start || source[dash-1] != '-' || strings.TrimSpace(string(source[line.start:dash-1])) != "" {
		return nil, 0, 0, fmt.Errorf("frontmatter_values: unsupported block list indentation")
	}
	prefix := string(source[line.start:start])
	eol := string(source[line.end:line.next])
	if eol == "" {
		eol = fm.eol
	}
	return fmSplice(source, line.next, line.next, prefix+item+eol), 1, 1, nil
}

type fmLine struct{ start, end, next int }

type fmSource struct {
	present             bool
	bom, close, content int
	eol                 string
	lines               []fmLine
	root                *yaml.Node
	keys, values        map[string]*yaml.Node
}

func fmRead(source []byte) (*fmSource, error) {
	if !utf8.Valid(source) {
		return nil, fmt.Errorf("frontmatter_values: invalid UTF-8 source")
	}
	fm := &fmSource{eol: "\n", keys: map[string]*yaml.Node{}, values: map[string]*yaml.Node{}}
	if bytes.HasPrefix(source, []byte("\xef\xbb\xbf")) {
		fm.bom = 3
	}
	for at := fm.bom; at < len(source); {
		end := at
		for end < len(source) && source[end] != '\n' && source[end] != '\r' {
			end++
		}
		next := end
		if next < len(source) {
			next++
			if source[end] == '\r' && next < len(source) && source[next] == '\n' {
				next++
			}
		}
		fm.lines = append(fm.lines, fmLine{at, end, next})
		at = next
	}
	for _, line := range fm.lines {
		if line.next > line.end {
			fm.eol = string(source[line.end:line.next])
			break
		}
	}
	if len(fm.lines) == 0 || strings.TrimSpace(string(source[fm.lines[0].start:fm.lines[0].end])) != "---" {
		return fm, nil
	}
	fm.present = true
	fm.content = fm.lines[0].next
	for i := 1; i < len(fm.lines); i++ {
		line := fm.lines[i]
		if strings.TrimSpace(string(source[line.start:line.end])) == "---" {
			fm.close = line.start
			break
		}
	}
	if fm.close == 0 {
		return nil, fmt.Errorf("frontmatter_values: unclosed YAML frontmatter")
	}
	if strings.ContainsAny(string(source[fm.content:fm.close]), "\u0085\u2028\u2029") {
		return nil, fmt.Errorf("frontmatter_values: Unicode YAML line breaks are unsupported")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(source[fm.content:fm.close]))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil && err != io.EOF {
		return nil, fmt.Errorf("frontmatter_values: invalid YAML: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("frontmatter_values: multiple or invalid YAML documents are unsupported")
	}
	// Explicit document boundaries/directives would put an appended entry
	// outside the mapping even when the decoder accepts the original content.
	for _, line := range fm.lines[1:] {
		if line.start >= fm.close {
			break
		}
		text := string(source[line.start:line.end])
		if strings.HasPrefix(text, "%") || text == "..." || strings.HasPrefix(text, "... ") || strings.HasPrefix(text, "...\t") {
			return nil, fmt.Errorf("frontmatter_values: YAML directives/document markers are unsupported")
		}
	}
	if len(document.Content) == 0 {
		return fm, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode || root.Style&yaml.FlowStyle != 0 {
		return nil, fmt.Errorf("frontmatter_values: expected a block-style YAML mapping")
	}
	if err := fmValidateNode(root); err != nil {
		return nil, err
	}
	fm.root = root
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		// Only ordinary single-line string keys are supported at the top level.
		if key.Line < 1 || key.Line >= len(fm.lines) {
			return nil, fmt.Errorf("frontmatter_values: invalid YAML key position")
		}
		line := fm.lines[key.Line]
		if key.Column != root.Column || key.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 || strings.ContainsAny(key.Value, "\r\n") {
			return nil, fmt.Errorf("frontmatter_values: complex top-level keys are unsupported")
		}
		start, err := fmColumn(source, line, key.Column)
		if err != nil {
			return nil, err
		}
		end, err := fmTokenEnd(source, start, line.end, true)
		if err != nil || end >= line.end || source[end] != ':' {
			return nil, fmt.Errorf("frontmatter_values: explicit or multiline YAML keys are unsupported")
		}
		fm.keys[key.Value], fm.values[key.Value] = key, value
	}
	return fm, nil
}

func fmValidateNode(node *yaml.Node) error {
	if node.Anchor != "" || node.Kind == yaml.AliasNode || node.Tag == "!!merge" {
		return fmt.Errorf("frontmatter_values: YAML anchors, aliases and merge keys are unsupported")
	}
	if node.Style&yaml.TaggedStyle != 0 {
		return fmt.Errorf("frontmatter_values: explicit YAML tags are unsupported")
	}
	if node.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return fmt.Errorf("frontmatter_values: YAML mapping keys must be strings")
			}
			if seen[key.Value] {
				return fmt.Errorf("frontmatter_values: duplicate YAML key %q", key.Value)
			}
			seen[key.Value] = true
		}
	}
	if node.Kind == yaml.ScalarNode {
		switch node.Tag {
		case "!!str", "!!null", "!!bool", "!!timestamp":
		case "!!int":
			var value interface{}
			if err := node.Decode(&value); err != nil {
				return fmt.Errorf("frontmatter_values: invalid YAML integer: %w", err)
			}
			switch value.(type) {
			case int, int64:
			default:
				return fmt.Errorf("frontmatter_values: integer overflow")
			}
		case "!!float":
			var value float64
			if err := node.Decode(&value); err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
				return fmt.Errorf("frontmatter_values: invalid or overflowing YAML number")
			}
			// yaml.v3 resolves integers beyond uint64 as float64; do not round them.
			if !strings.ContainsAny(node.Value, ".eE") {
				return fmt.Errorf("frontmatter_values: integer overflow")
			}
		default:
			return fmt.Errorf("frontmatter_values: unsupported YAML scalar tag %q", node.Tag)
		}
	}
	for _, child := range node.Content {
		if err := fmValidateNode(child); err != nil {
			return err
		}
	}
	return nil
}

func (fm *fmSource) scalarSpan(source []byte, key, value *yaml.Node) (int, int, bool, error) {
	if value.Line != key.Line || value.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return 0, 0, false, fmt.Errorf("multiline YAML scalars are unsupported")
	}
	line := fm.lines[key.Line]
	keyStart, err := fmColumn(source, line, key.Column)
	if err != nil {
		return 0, 0, false, err
	}
	colon, err := fmTokenEnd(source, keyStart, line.end, true)
	if err != nil {
		return 0, 0, false, err
	}
	start := colon + 1
	for start < line.end && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	if start == line.end || source[start] == '#' {
		if value.Tag != "!!null" || value.Value != "" {
			return 0, 0, false, fmt.Errorf("unsupported YAML value placement")
		}
		return colon + 1, colon + 1, true, nil
	}
	end, err := fmTokenEnd(source, start, line.end, false)
	if err != nil {
		return 0, 0, false, err
	}
	var check yaml.Node
	if err := yaml.Unmarshal(source[start:end], &check); err != nil || len(check.Content) != 1 {
		return 0, 0, false, fmt.Errorf("unsupported YAML scalar syntax")
	}
	actual := check.Content[0]
	if actual.Kind != yaml.ScalarNode || actual.Tag != value.Tag || actual.Value != value.Value {
		return 0, 0, false, fmt.Errorf("multiline YAML scalars are unsupported")
	}
	return start, end, false, nil
}

func (fm *fmSource) sequenceScalarSpan(source []byte, sequence *yaml.Node, item int) (int, int, error) {
	if item < 0 || item >= len(sequence.Content) {
		return 0, 0, fmt.Errorf("invalid list item position")
	}
	node := sequence.Content[item]
	if node.Kind != yaml.ScalarNode || node.Line < 1 || node.Line >= len(fm.lines) || strings.ContainsAny(node.Value, "\r\n") {
		return 0, 0, fmt.Errorf("only one-line scalar list items are writable")
	}
	line := fm.lines[node.Line]
	start, err := fmColumn(source, line, node.Column)
	if err != nil {
		return 0, 0, err
	}
	end := 0
	if sequence.Style&yaml.FlowStyle != 0 {
		_, close, err := fm.flowSequenceBounds(source, sequence)
		if err != nil {
			return 0, 0, err
		}
		end = close
		if item+1 < len(sequence.Content) {
			next := sequence.Content[item+1]
			if next.Line != node.Line {
				return 0, 0, fmt.Errorf("multiline inline lists are unsupported")
			}
			nextStart, err := fmColumn(source, line, next.Column)
			if err != nil {
				return 0, 0, err
			}
			comma := nextStart
			for comma > start && (source[comma-1] == ' ' || source[comma-1] == '\t') {
				comma--
			}
			if comma == start || source[comma-1] != ',' {
				return 0, 0, fmt.Errorf("cannot locate inline list separator")
			}
			end = comma - 1
		}
		for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
			end--
		}
	} else {
		end, err = fmTokenEnd(source, start, line.end, false)
		if err != nil {
			return 0, 0, err
		}
	}
	if end <= start || !fmSameScalar(node, string(source[start:end])) {
		return 0, 0, fmt.Errorf("cannot locate scalar list item")
	}
	return start, end, nil
}

func (fm *fmSource) flowSequenceBounds(source []byte, sequence *yaml.Node) (int, int, error) {
	if sequence.Line < 1 || sequence.Line >= len(fm.lines) {
		return 0, 0, fmt.Errorf("invalid inline list position")
	}
	line := fm.lines[sequence.Line]
	open, err := fmColumn(source, line, sequence.Column)
	if err != nil || source[open] != '[' {
		return 0, 0, fmt.Errorf("cannot locate inline list")
	}
	quote := byte(0)
	depth := 0
	for at := open; at < line.end; at++ {
		c := source[at]
		if quote != 0 {
			if quote == '"' && c == '\\' {
				at++
				continue
			}
			if c == quote {
				if quote == '\'' && at+1 < line.end && source[at+1] == '\'' {
					at++
					continue
				}
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		switch c {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return open, at, nil
			}
		}
	}
	return 0, 0, fmt.Errorf("unterminated inline list")
}

func fmColumn(source []byte, line fmLine, column int) (int, error) {
	at := line.start
	for n := 1; n < column && at < line.end; n++ {
		_, size := utf8.DecodeRune(source[at:line.end])
		at += size
	}
	if column < 1 || at >= line.end {
		return 0, fmt.Errorf("frontmatter_values: invalid YAML source position")
	}
	return at, nil
}

func fmTokenEnd(source []byte, start, limit int, key bool) (int, error) {
	if start >= limit {
		return 0, fmt.Errorf("unsupported empty YAML token")
	}
	quote := source[start]
	if quote == '\'' || quote == '"' {
		for at := start + 1; at < limit; at++ {
			if quote == '"' && source[at] == '\\' {
				at++
				continue
			}
			if source[at] != quote {
				continue
			}
			if quote == '\'' && at+1 < limit && source[at+1] == '\'' {
				at++
				continue
			}
			end := at + 1
			if key {
				for end < limit && (source[end] == ' ' || source[end] == '\t') {
					end++
				}
			}
			return end, nil
		}
		return 0, fmt.Errorf("multiline quoted YAML scalars are unsupported")
	}
	end := limit
	for at := start; at < limit; at++ {
		if key && source[at] == ':' && (at+1 == limit || source[at+1] == ' ' || source[at+1] == '\t') {
			return at, nil
		}
		if source[at] == '#' && (at == start || source[at-1] == ' ' || source[at-1] == '\t') {
			end = at
			break
		}
	}
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	return end, nil
}

func fmSameScalar(node *yaml.Node, literal string) bool {
	var other yaml.Node
	if yaml.Unmarshal([]byte(literal), &other) != nil || len(other.Content) != 1 {
		return false
	}
	if node.Tag != other.Content[0].Tag {
		return false
	}
	var before, after interface{}
	return node.Decode(&before) == nil && other.Content[0].Decode(&after) == nil && reflect.DeepEqual(before, after)
}

func fmLiteral(value query.Value) (string, error) {
	return fmStyledLiteral(value, 0)
}

func fmStyledLiteral(value query.Value, style yaml.Style) (string, error) {
	switch value.Kind {
	case query.ValueNull:
		return "null", nil
	case query.ValueString:
		if text, ok := value.Data.(string); ok && utf8.ValidString(text) {
			if style == yaml.SingleQuotedStyle && !strings.ContainsAny(text, "\r\n") {
				literal := "'" + strings.ReplaceAll(text, "'", "''") + "'"
				if parsed, valid := fmLiteralString(literal); valid && parsed == text {
					return literal, nil
				}
			}
			if parsed, valid := fmLiteralString(text); style == 0 && valid && parsed == text {
				return text, nil
			}
			return strconv.QuoteToASCII(text), nil
		}
	case query.ValueInt:
		if number, ok := value.Data.(int64); ok {
			return strconv.FormatInt(number, 10), nil
		}
	case query.ValueFloat:
		if number, ok := value.Data.(float64); ok && !math.IsNaN(number) && !math.IsInf(number, 0) {
			text := strconv.FormatFloat(number, 'g', -1, 64)
			if !strings.ContainsAny(text, ".eE") {
				text += ".0"
			}
			return text, nil
		}
	case query.ValueBool:
		if boolean, ok := value.Data.(bool); ok {
			return strconv.FormatBool(boolean), nil
		}
	case query.ValueDate, query.ValueDateTime:
		if date, ok := value.Data.(time.Time); ok && date.Year() >= 0 && date.Year() <= 9999 {
			if value.Kind == query.ValueDate {
				return date.Format(time.DateOnly), nil
			}
			return date.Format(time.RFC3339Nano), nil
		}
	}
	return "", fmt.Errorf("frontmatter_values: value must be a valid scalar literal (no numeric overflow)")
}

func fmLiteralString(literal string) (string, bool) {
	if literal == "" || strings.ContainsAny(literal, "\r\n") {
		return "", false
	}
	var document yaml.Node
	if yaml.Unmarshal([]byte(literal), &document) != nil || len(document.Content) != 1 {
		return "", false
	}
	node := document.Content[0]
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", false
	}
	return node.Value, true
}

func fmKeyLiteral(key string) (string, error) {
	if key == "" || !utf8.ValidString(key) || strings.ContainsAny(key, "\r\n") {
		return "", fmt.Errorf("frontmatter_values: key must be a nonempty plain YAML key")
	}
	var document yaml.Node
	if yaml.Unmarshal([]byte(key+": value"), &document) == nil && len(document.Content) == 1 {
		root := document.Content[0]
		if root.Kind == yaml.MappingNode && len(root.Content) == 2 {
			node := root.Content[0]
			if node.Kind == yaml.ScalarNode && node.Tag == "!!str" && node.Style == 0 && node.Value == key {
				return key, nil
			}
		}
	}
	return "", fmt.Errorf("frontmatter_values: key %q cannot be written as a plain YAML key", key)
}

func fmSplice(source []byte, start, end int, text string) []byte {
	after := make([]byte, 0, len(source)-(end-start)+len(text))
	after = append(after, source[:start]...)
	after = append(after, text...)
	return append(after, source[end:]...)
}
