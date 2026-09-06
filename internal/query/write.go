package query

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WriteStatement describes one literal-only INSERT or guarded UPDATE.
type WriteStatement struct {
	Operation string
	Table     string
	Values    map[string]Value
	Where     Expr
}

const tWriteFloat tokenType = tInvalid + 1

// ParseWrite parses exactly one write statement. WHERE uses the same expression
// parser as SELECT, without accepting SELECT's trailing clauses.
func ParseWrite(sql string) (*WriteStatement, error) {
	lx := &lexer{src: sql}
	var tokens []token
	predicate := false
	for {
		t := nextWriteToken(lx, predicate)
		// WHERE is a reserved identifier, never a string or comment token.
		if t.typ == tIdent && strings.EqualFold(t.lit, "WHERE") {
			predicate = true
		}
		tokens = append(tokens, t)
		if lx.err != nil {
			return nil, lx.err
		}
		if t.typ == tEOF {
			break
		}
	}
	if len(tokens) > 1 && tokens[len(tokens)-2].typ == tInvalid && tokens[len(tokens)-2].lit == ";" {
		tokens = append(tokens[:len(tokens)-2], tokens[len(tokens)-1])
	}
	p := &parser{tokens: tokens}
	stmt := &WriteStatement{Values: make(map[string]Value)}
	if p.acceptValue("INSERT") {
		stmt.Operation = "INSERT"
		if !p.acceptValue("INTO") {
			return nil, p.expected("INTO after INSERT")
		}
	} else if p.acceptValue("UPDATE") {
		stmt.Operation = "UPDATE"
	} else {
		return nil, p.expected("INSERT or UPDATE")
	}
	if !isIdentifier(p.peek()) {
		return nil, p.expected("table name")
	}
	stmt.Table = strings.ToLower(p.next().lit)
	if stmt.Operation == "INSERT" {
		if !p.match(tLParen) {
			return nil, p.expected("( before INSERT columns")
		}
		p.next()
		var columns []string
		seen := make(map[string]bool)
		for {
			if !isIdentifier(p.peek()) {
				return nil, p.expected("column name")
			}
			column := strings.ToLower(p.next().lit)
			if seen[column] {
				return nil, fmt.Errorf("duplicate column %q", column)
			}
			seen[column] = true
			columns = append(columns, column)
			if !p.match(tComma) {
				break
			}
			p.next()
		}
		if !p.match(tRParen) {
			return nil, p.expected(") after INSERT columns")
		}
		p.next()
		if !p.acceptValue("VALUES") || !p.match(tLParen) {
			return nil, p.expected("VALUES ( after INSERT columns")
		}
		p.next()
		for i, column := range columns {
			if i > 0 {
				if !p.match(tComma) {
					return nil, p.expected(", between INSERT values")
				}
				p.next()
			}
			value, err := parseWriteLiteral(p)
			if err != nil {
				return nil, err
			}
			stmt.Values[column] = value
		}
		if !p.match(tRParen) {
			return nil, p.expected(") after INSERT values (one value per column)")
		}
		p.next()
	} else {
		if !p.acceptValue("SET") {
			return nil, p.expected("SET after UPDATE table")
		}
		for {
			if !isIdentifier(p.peek()) {
				return nil, p.expected("assignment column")
			}
			column := strings.ToLower(p.next().lit)
			if _, exists := stmt.Values[column]; exists {
				return nil, fmt.Errorf("duplicate assignment %q", column)
			}
			if !p.match(tEq) || p.peek().lit != "=" {
				return nil, p.expected("= after assignment column")
			}
			p.next()
			value, err := parseWriteLiteral(p)
			if err != nil {
				return nil, err
			}
			stmt.Values[column] = value
			if !p.match(tComma) {
				break
			}
			p.next()
		}
		if !p.acceptValue("WHERE") {
			return nil, p.expected("WHERE (required for UPDATE; SET accepts literals only)")
		}
		var err error
		stmt.Where, err = p.parseOr()
		if err != nil {
			return nil, err
		}
		if HasAggregate(stmt.Where) {
			return nil, fmt.Errorf("aggregate functions are not allowed in WHERE")
		}
	}
	if !p.match(tEOF) {
		return nil, p.expected("end of write statement")
	}
	if err := validateWriteValues(stmt); err != nil {
		return nil, err
	}
	return stmt, nil
}

func parseWriteLiteral(p *parser) (Value, error) {
	sign := ""
	if p.match(tPlus) || p.match(tMinus) {
		sign = p.next().lit
	}
	t := p.next()
	if sign != "" && t.typ != tInt && t.typ != tWriteFloat {
		return Value{}, expectedToken(t, "numeric literal after sign")
	}
	switch t.typ {
	case tString:
		return NewString(t.lit), nil
	case tInt:
		t.lit = sign + t.lit
		v, err := t.literalInt(64)
		return NewInt(v), err
	case tWriteFloat:
		v, err := strconv.ParseFloat(sign+t.lit, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return Value{}, expectedToken(t, "finite numeric literal")
		}
		return NewFloat(v), nil
	case tTrue:
		return NewBool(true), nil
	case tFalse:
		return NewBool(false), nil
	case tNull:
		return NewNull(), nil
	default:
		return Value{}, expectedToken(t, "scalar literal")
	}
}

// Keep SELECT lexing unchanged; writes additionally support SQL doubled quotes,
// block comments, decimal literals and an optional final semicolon.
func nextWriteToken(l *lexer, predicate bool) token {
	for l.pos < len(l.src) {
		if isSpace(l.src[l.pos]) {
			l.pos++
			continue
		}
		if strings.HasPrefix(l.src[l.pos:], "--") {
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
			continue
		}
		if strings.HasPrefix(l.src[l.pos:], "/*") {
			end := strings.Index(l.src[l.pos+2:], "*/")
			if end < 0 {
				l.err = fmt.Errorf("at byte offset %d: unclosed SQL comment", l.pos)
				return token{typ: tEOF, pos: l.pos}
			}
			l.pos += end + 4
			continue
		}
		break
	}
	start := l.pos
	if start == len(l.src) {
		return token{typ: tEOF, pos: start}
	}
	quote := l.src[start]
	if quote == '\'' || quote == '"' {
		l.pos++
		var value strings.Builder
		for l.pos < len(l.src) {
			c := l.src[l.pos]
			l.pos++
			if c == quote {
				if l.pos < len(l.src) && l.src[l.pos] == quote {
					value.WriteByte(quote)
					l.pos++
					continue
				}
				lit := value.String()
				if predicate {
					// Match SELECT's replacement order, including doubled
					// backslashes and its lack of a carriage-return escape.
					lit = strings.ReplaceAll(lit, "\\t", "\t")
					lit = strings.ReplaceAll(lit, "\\n", "\n")
					lit = strings.ReplaceAll(lit, "\\\"", "\"")
					lit = strings.ReplaceAll(lit, "\\'", "'")
				}
				return token{typ: tString, lit: lit, pos: start}
			}
			if c == '\\' && l.pos < len(l.src) {
				next := l.src[l.pos]
				if predicate {
					value.WriteByte(c)
					value.WriteByte(next)
					l.pos++
					continue
				}
				switch next {
				case 'n':
					c = '\n'
				case 'r':
					c = '\r'
				case 't':
					c = '\t'
				case '\\', '\'', '"':
					c = next
				default:
					value.WriteByte(c)
					c = next
				}
				l.pos++
			}
			value.WriteByte(c)
		}
		l.err = fmt.Errorf("at byte offset %d: unclosed string literal", start)
		return token{typ: tEOF, pos: l.pos}
	}
	if isDigit(quote) || quote == '.' && start+1 < len(l.src) && isDigit(l.src[start+1]) {
		// Preserve SELECT's unquoted dates in WHERE.
		if start+10 <= len(l.src) && isDateLiteral(l.src[start:start+10]) {
			return l.nextToken()
		}
		end := start
		for end < len(l.src) && isDigit(l.src[end]) {
			end++
		}
		floating := false
		if end < len(l.src) && l.src[end] == '.' {
			floating = true
			end++
			for end < len(l.src) && isDigit(l.src[end]) {
				end++
			}
		}
		if end < len(l.src) && (l.src[end] == 'e' || l.src[end] == 'E') {
			floating = true
			end++
			if end < len(l.src) && (l.src[end] == '+' || l.src[end] == '-') {
				end++
			}
			for end < len(l.src) && isDigit(l.src[end]) {
				end++
			}
		}
		if floating {
			l.pos = end
			return token{typ: tWriteFloat, lit: l.src[start:end], pos: start}
		}
	}
	return l.nextToken()
}

func validateWriteValues(stmt *WriteStatement) error {
	allowed := map[string]ValueKind{}
	var required []string
	if stmt.Operation == "UPDATE" {
		switch stmt.Table {
		case "tasks":
			allowed["state"], allowed["status"], allowed["text"] = ValueString, ValueString, ValueString
			for _, key := range []string{"due", "scheduled", "priority", "tags"} {
				allowed[key] = ValueNull
			}
		case "frontmatter_values":
			allowed["value"] = ValueNull // Any scalar.
		default:
			return fmt.Errorf("UPDATE is not supported for table %q", stmt.Table)
		}
	} else {
		switch stmt.Table {
		case "lines":
			allowed["text"], required = ValueString, []string{"text"}
		case "tasks":
			allowed["text"], allowed["state"], allowed["status"] = ValueString, ValueString, ValueString
			allowed["parent_line"], allowed["position"], required = ValueInt, ValueString, []string{"text"}
			for _, key := range []string{"due", "scheduled", "priority", "tags"} {
				allowed[key] = ValueNull
			}
		case "sections":
			allowed["level"], allowed["title"], allowed["body"] = ValueInt, ValueString, ValueString
			required = []string{"level", "title"}
		case "code":
			allowed["lang"], allowed["code"], required = ValueString, ValueString, []string{"code"}
		case "frontmatter_values":
			allowed["key"], allowed["value"], required = ValueString, ValueNull, []string{"key", "value"}
		default:
			return fmt.Errorf("INSERT is not supported for table %q", stmt.Table)
		}
		if stmt.Table != "frontmatter_values" {
			allowed["line"], allowed["heading_line"] = ValueInt, ValueInt
			allowed["section"], allowed["section_path"] = ValueString, ValueString
		}
	}
	for _, key := range required {
		if _, ok := stmt.Values[key]; !ok {
			return fmt.Errorf("%s into %s requires %s", stmt.Operation, stmt.Table, key)
		}
	}
	positions := 0
	for key, value := range stmt.Values {
		kind, ok := allowed[key]
		if !ok {
			return fmt.Errorf("%s %s does not support column %q", stmt.Operation, stmt.Table, key)
		}
		if kind != ValueNull && value.Kind != kind {
			return fmt.Errorf("invalid literal type for %s.%s", stmt.Table, key)
		}
		switch key {
		case "line", "heading_line", "parent_line":
			positions++
			if value.Data.(int64) < 1 {
				return fmt.Errorf("%s must be a positive integer", key)
			}
		case "section", "section_path":
			positions++
		case "level":
			if n := value.Data.(int64); n < 1 || n > 6 {
				return fmt.Errorf("section level must be between 1 and 6")
			}
		case "state":
			switch value.String() {
			case "todo", "done", "in_progress", "cancelled":
			default:
				return fmt.Errorf("unsupported task state %q", value.String())
			}
		case "status":
			switch value.String() {
			case " ", "x", "X", "/", "-":
			default:
				return fmt.Errorf("unsupported task status %q", value.String())
			}
		case "position":
			if value.String() != "section_end" {
				return fmt.Errorf("unsupported task position %q", value.String())
			}
		case "due", "scheduled":
			if value.Kind != ValueNull {
				if value.Kind != ValueString {
					return fmt.Errorf("invalid literal type for tasks.%s", key)
				}
				if _, err := time.Parse(time.DateOnly, value.String()); err != nil {
					return fmt.Errorf("tasks.%s must be an ISO date", key)
				}
			}
		case "priority":
			if value.Kind != ValueNull {
				if value.Kind != ValueInt {
					return fmt.Errorf("invalid literal type for tasks.priority")
				}
				if n := value.Data.(int64); n < 1 || n > 5 {
					return fmt.Errorf("tasks.priority must be between 1 and 5")
				}
			}
		case "tags":
			if err := validateTaskTagsLiteral(value); err != nil {
				return err
			}
		}
	}
	if positions > 1 {
		return fmt.Errorf("only one insertion position is allowed")
	}
	if _, state := stmt.Values["state"]; state {
		if _, status := stmt.Values["status"]; status {
			return fmt.Errorf("specify either state or status, not both")
		}
	}
	if _, ok := stmt.Values["position"]; ok {
		_, section := stmt.Values["section"]
		_, sectionPath := stmt.Values["section_path"]
		_, headingLine := stmt.Values["heading_line"]
		if !section && !sectionPath && !headingLine {
			return fmt.Errorf("task position requires a section selector")
		}
	}
	return nil
}

var taskTagLiteral = regexp.MustCompile(`^[\p{L}\p{N}_/-]+$`)

func validateTaskTagsLiteral(value Value) error {
	if value.Kind == ValueNull {
		return nil
	}
	if value.Kind != ValueString {
		return fmt.Errorf("invalid literal type for tasks.tags")
	}
	parts := strings.Fields(strings.ReplaceAll(value.String(), ",", " "))
	if len(parts) == 0 {
		return fmt.Errorf("tasks.tags must contain at least one tag")
	}
	for _, part := range parts {
		part = strings.TrimPrefix(part, "#")
		if !taskTagLiteral.MatchString(part) {
			return fmt.Errorf("invalid task tag %q", part)
		}
	}
	return nil
}
