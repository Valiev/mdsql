package query

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type ValueKind int

const (
	ValueNull ValueKind = iota
	ValueString
	ValueInt
	ValueBool
	ValueDate
	ValueDateTime
	ValueList
	ValueObject
	ValueDuration
	ValueFloat
)

type Duration struct {
	Value int
	Unit  rune
}

type Value struct {
	Kind ValueKind
	Data interface{}
}

func NewNull() Value                     { return Value{Kind: ValueNull} }
func NewString(s string) Value           { return Value{Kind: ValueString, Data: s} }
func NewFloat(f float64) Value           { return Value{Kind: ValueFloat, Data: f} }
func NewInt(i int64) Value               { return Value{Kind: ValueInt, Data: i} }
func NewBool(v bool) Value               { return Value{Kind: ValueBool, Data: v} }
func NewDate(t time.Time) Value          { return Value{Kind: ValueDate, Data: t} }
func NewDateTime(t time.Time) Value      { return Value{Kind: ValueDateTime, Data: t} }
func NewList(v []Value) Value            { return Value{Kind: ValueList, Data: v} }
func NewObject(v map[string]Value) Value { return Value{Kind: ValueObject, Data: v} }
func NewDuration(v int, unit rune) Value {
	return Value{Kind: ValueDuration, Data: Duration{Value: v, Unit: unit}}
}

func (v Value) IsNull() bool { return v.Kind == ValueNull }

func (v Value) String() string {
	switch v.Kind {
	case ValueNull:
		return ""
	case ValueString:
		return v.Data.(string)
	case ValueInt:
		return strconv.FormatInt(v.Data.(int64), 10)
	case ValueFloat:
		return strconv.FormatFloat(v.Data.(float64), 'g', -1, 64)
	case ValueBool:
		return strconv.FormatBool(v.Data.(bool))
	case ValueDate:
		return v.Data.(time.Time).Format(time.DateOnly)
	case ValueDateTime:
		return v.Data.(time.Time).Format(time.RFC3339)
	case ValueList:
		parts := make([]string, 0, len(v.Data.([]Value)))
		for _, item := range v.Data.([]Value) {
			parts = append(parts, item.String())
		}
		return strings.Join(parts, ",")
	case ValueObject:
		return "{...}"
	case ValueDuration:
		d := v.Data.(Duration)
		if d.Unit == 0 {
			return ""
		}
		return fmt.Sprintf("%d%c", d.Value, d.Unit)
	default:
		return fmt.Sprintf("%v", v.Data)
	}
}

type Row map[string]Value

type Column struct {
	Name string
	Type string
	Doc  string
}

type SelectItem struct {
	Expr  Expr
	Alias string
}

type Query struct {
	Distinct       bool
	Select         []SelectItem
	Table          string
	In             []string
	Where          Expr
	OrderBy        []OrderItem
	Limit          int
	LimitSet       bool
	Offset         int
	WithOpt        map[string]string
	GroupBy        string
	GroupByColumns []string
	Having         Expr
	LimitPer       []string
	CountAll       bool
	Raw            string
}

func (q *Query) String() string {
	if q == nil {
		return ""
	}
	return q.Raw
}

// Subqueries returns uncorrelated membership subqueries in execution order.
func (q *Query) Subqueries() []*Query {
	if q == nil {
		return nil
	}
	var result []*Query
	var visit func(Expr)
	visit = func(expr Expr) {
		switch value := expr.(type) {
		case *SubqueryExpr:
			result = append(result, value.Query)
			for _, nested := range value.Query.Subqueries() {
				result = append(result, nested)
			}
		case *BinaryExpr:
			visit(value.Left)
			visit(value.Right)
		case *UnaryExpr:
			visit(value.Expr)
		case *BetweenExpr:
			visit(value.Left)
			visit(value.Low)
			visit(value.High)
		case *IsNullExpr:
			visit(value.Expr)
		case *FuncCallExpr:
			for _, arg := range value.Args {
				visit(arg)
			}
		case *AggregateExpr:
			visit(value.Arg)
		}
	}
	for _, item := range q.Select {
		visit(item.Expr)
	}
	visit(q.Where)
	visit(q.Having)
	for _, item := range q.OrderBy {
		visit(item.Expr)
	}
	return result
}

type OrderItem struct {
	Expr Expr
	Desc bool
}

func (q *Query) SelectKeys() []string {
	keys := make([]string, 0, len(q.Select))
	for _, item := range q.Select {
		if item.Alias != "" {
			keys = append(keys, item.Alias)
			continue
		}
		switch it := item.Expr.(type) {
		case *ColumnExpr:
			keys = append(keys, it.Name)
		case *FuncCallExpr:
			keys = append(keys, it.Name)
		case *AggregateExpr:
			keys = append(keys, it.Name)
		default:
			keys = append(keys, "expr")
		}
	}
	return keys
}

func Parse(source string) (*Query, error) {
	lx := &lexer{src: source}
	p := &parser{tokens: lx.scan(), source: source}
	if lx.err != nil {
		return nil, lx.err
	}
	q, err := p.parseQuery()
	if err != nil {
		return nil, err
	}
	if HasAggregate(q.Where) {
		return nil, fmt.Errorf("aggregate functions are not allowed in WHERE")
	}
	if p.fileListEnd > 0 {
		source = source[:p.fileListStart] + quotedFileSpecs(q.In) + source[p.fileListEnd:]
	}
	q.Raw = strings.TrimSpace(source)
	return q, nil
}

type Expr interface {
	eval(row Row, ctx *EvalContext) (Value, error)
}

// Eval evaluates an expression while keeping concrete expression methods
// private to the query package.
func Eval(expr Expr, row Row, ctx *EvalContext) (Value, error) {
	if expr == nil {
		return NewNull(), nil
	}
	return expr.eval(row, ctx)
}

type BinaryExpr struct {
	Op          string
	Left, Right Expr
}

func (b *BinaryExpr) eval(row Row, ctx *EvalContext) (Value, error) {
	lv, err := b.Left.eval(row, ctx)
	if err != nil {
		return Value{}, err
	}

	if b.Op == "AND" || b.Op == "OR" {
		if b.Op == "AND" && !truthy(lv) {
			return NewBool(false), nil
		}
		if b.Op == "OR" && truthy(lv) {
			return NewBool(true), nil
		}
		rv, err := b.Right.eval(row, ctx)
		if err != nil {
			return Value{}, err
		}
		if b.Op == "AND" {
			return NewBool(truthy(lv) && truthy(rv)), nil
		}
		return NewBool(truthy(lv) || truthy(rv)), nil
	}

	rv, err := b.Right.eval(row, ctx)
	if err != nil {
		return Value{}, err
	}

	switch b.Op {
	case "=", "==", "!=", "<>", "<", "<=", ">", ">=":
		lv, rv = coerceDateComparison(lv, rv)
	}
	if message := comparisonProblem(b.Op, b.Left, b.Right, lv, rv); message != "" {
		return NewBool(false), reportIssue(ctx, message)
	}
	switch b.Op {
	case "=", "==":
		if lv.Kind == ValueNull || rv.Kind == ValueNull {
			return NewBool(false), nil
		}
		return NewBool(compareStrict(lv, rv) == 0), nil
	case "!=", "<>":
		if lv.Kind == ValueNull || rv.Kind == ValueNull {
			return NewBool(false), nil
		}
		return NewBool(compareStrict(lv, rv) != 0), nil
	case "<":
		if lv.Kind == ValueNull || rv.Kind == ValueNull || !comparableKinds(lv.Kind, rv.Kind) {
			return NewBool(false), nil
		}
		return NewBool(compareStrict(lv, rv) < 0), nil
	case "<=":
		if lv.Kind == ValueNull || rv.Kind == ValueNull || !comparableKinds(lv.Kind, rv.Kind) {
			return NewBool(false), nil
		}
		return NewBool(compareStrict(lv, rv) <= 0), nil
	case ">":
		if lv.Kind == ValueNull || rv.Kind == ValueNull || !comparableKinds(lv.Kind, rv.Kind) {
			return NewBool(false), nil
		}
		return NewBool(compareStrict(lv, rv) > 0), nil
	case ">=":
		if lv.Kind == ValueNull || rv.Kind == ValueNull || !comparableKinds(lv.Kind, rv.Kind) {
			return NewBool(false), nil
		}
		return NewBool(compareStrict(lv, rv) >= 0), nil
	case "LIKE":
		if lv.Kind != ValueString || rv.Kind != ValueString {
			return NewBool(false), nil
		}
		return NewBool(matchLike(lv.String(), rv.String(), true)), nil
	case "NOT LIKE":
		if lv.Kind != ValueString || rv.Kind != ValueString {
			return NewBool(false), nil
		}
		return NewBool(!matchLike(lv.String(), rv.String(), true)), nil
	case "~":
		if lv.Kind != ValueString || rv.Kind != ValueString {
			return NewBool(false), nil
		}
		return NewBool(regexMatch(lv.String(), rv.String(), false)), nil
	case "~*":
		if lv.Kind != ValueString || rv.Kind != ValueString {
			return NewBool(false), nil
		}
		return NewBool(regexMatch(lv.String(), rv.String(), true)), nil
	case "!~":
		if lv.Kind != ValueString || rv.Kind != ValueString {
			return NewBool(false), nil
		}
		return NewBool(!regexMatch(lv.String(), rv.String(), false)), nil
	case "!~*":
		if lv.Kind != ValueString || rv.Kind != ValueString {
			return NewBool(false), nil
		}
		return NewBool(!regexMatch(lv.String(), rv.String(), true)), nil
	case "HAS":
		if lv.Kind != ValueList {
			return NewBool(false), nil
		}
		n := strings.ToLower(rv.String())
		for _, it := range lv.Data.([]Value) {
			v := strings.ToLower(it.String())
			if v == n || strings.HasPrefix(v, n+"/") {
				return NewBool(true), nil
			}
		}
		return NewBool(false), nil
	case "IN", "NOT IN":
		if rv.Kind != ValueList {
			return NewBool(b.Op == "NOT IN"), nil
		}
		found := false
		for i, item := range rv.Data.([]Value) {
			left, right := coerceDateComparison(lv, item)
			if message := comparisonProblem("=", b.Left, b.Right, left, right); message != "" {
				if err := reportIssue(ctx, fmt.Sprintf("%s item %d: %s", b.Op, i+1, message)); err != nil {
					return NewBool(false), err
				}
				continue
			}
			if compareStrict(left, right) == 0 {
				found = true
			}
		}
		if b.Op == "NOT IN" {
			found = !found
		}
		return NewBool(found), nil
	case "+", "-":
		if lv.Kind == ValueInt && rv.Kind == ValueInt && addIntOverflows(lv.Data.(int64), rv.Data.(int64), b.Op == "+") {
			return NewNull(), reportIssue(ctx, "numeric overflow in "+b.Op)
		}
		value := addOrNull(lv, rv, b.Op == "+")
		if value.Kind == ValueFloat && (math.IsInf(Number(value), 0) || math.IsNaN(Number(value))) {
			return NewNull(), reportIssue(ctx, "numeric overflow in "+b.Op)
		}
		return value, nil
	case "*", "/", "%":
		value, problem := multiplyOrNull(lv, rv, b.Op)
		if problem != "" {
			return NewNull(), reportIssue(ctx, problem)
		}
		return value, nil
	default:
		return NewNull(), nil
	}
}

type UnaryExpr struct {
	Op   string
	Expr Expr
}

func (u *UnaryExpr) eval(row Row, ctx *EvalContext) (Value, error) {
	v, err := u.Expr.eval(row, ctx)
	if err != nil {
		return Value{}, err
	}
	switch u.Op {
	case "NOT":
		return NewBool(!truthy(v)), nil
	case "PLUS":
		return v, nil
	case "MINUS":
		if v.Kind == ValueInt {
			if v.Data.(int64) == math.MinInt64 {
				return NewNull(), reportIssue(ctx, "numeric overflow in unary -")
			}
			return NewInt(-v.Data.(int64)), nil
		}
		if v.Kind == ValueFloat {
			return NewFloat(-v.Data.(float64)), nil
		}
		if v.Kind == ValueDuration {
			d := v.Data.(Duration)
			d.Value = -d.Value
			return NewDuration(d.Value, d.Unit), nil
		}
	}
	return v, nil
}

type ColumnExpr struct {
	Name string
}

func (c *ColumnExpr) eval(row Row, ctx *EvalContext) (Value, error) {
	value, exists := ResolveColumn(row, c.Name)
	if !exists && ctx != nil && ctx.MissingField != nil {
		if err := ctx.MissingField(c.Name); err != nil {
			return NewNull(), err
		}
	}
	return value, nil
}

type LiteralExpr struct {
	Value Value
}

func (l *LiteralExpr) eval(_ Row, _ *EvalContext) (Value, error) { return l.Value, nil }

// SubqueryExpr is resolved by the engine before row evaluation.
type SubqueryExpr struct {
	Query *Query
}

func (s *SubqueryExpr) eval(_ Row, _ *EvalContext) (Value, error) {
	return NewNull(), fmt.Errorf("subquery must be evaluated by the query engine")
}

type FuncCallExpr struct {
	Name string
	Args []Expr
}

type ContextValueExpr struct {
	Name string
}

func (c *ContextValueExpr) eval(_ Row, ctx *EvalContext) (Value, error) {
	switch strings.ToLower(c.Name) {
	case "today":
		return NewDate(ctx.Today), nil
	case "now":
		return NewDateTime(ctx.Now), nil
	default:
		return NewNull(), nil
	}
}

func (f *FuncCallExpr) eval(row Row, ctx *EvalContext) (Value, error) {
	if err := validateFunctionArity(f.Name, len(f.Args)); err != nil {
		return Value{}, err
	}
	argContext := ctx
	if strings.EqualFold(f.Name, "coalesce") {
		argContext = withoutMissingFields(ctx)
	}
	vals := make([]Value, 0, len(f.Args))
	for _, a := range f.Args {
		v, err := a.eval(row, argContext)
		if err != nil {
			return Value{}, err
		}
		vals = append(vals, v)
	}
	if message := functionArgumentProblem(f.Name, vals); message != "" {
		return NewNull(), reportIssue(ctx, message)
	}
	switch strings.ToLower(f.Name) {
	case "today", "now":
		return (&ContextValueExpr{Name: f.Name}).eval(row, ctx)
	case "lower":
		if len(vals) == 1 && vals[0].Kind == ValueString {
			return NewString(strings.ToLower(vals[0].String())), nil
		}
	case "upper":
		if len(vals) == 1 && vals[0].Kind == ValueString {
			return NewString(strings.ToUpper(vals[0].String())), nil
		}
	case "trim":
		if len(vals) == 1 && vals[0].Kind == ValueString {
			return NewString(strings.TrimSpace(vals[0].String())), nil
		}
	case "len":
		if len(vals) == 1 {
			if vals[0].Kind == ValueString {
				return NewInt(int64(len(vals[0].String()))), nil
			}
			if vals[0].Kind == ValueList {
				return NewInt(int64(len(vals[0].Data.([]Value)))), nil
			}
		}
	case "lines", "head", "tail":
		return evalTextLines(strings.ToLower(f.Name), vals), nil
	case "contains":
		if len(vals) == 2 && vals[0].Kind == ValueString && vals[1].Kind == ValueString {
			return NewBool(strings.Contains(vals[0].String(), vals[1].String())), nil
		}
		if len(vals) == 2 && vals[0].Kind == ValueList && vals[1].Kind != ValueList && vals[1].Kind != ValueObject {
			for _, item := range vals[0].Data.([]Value) {
				if item.Kind == vals[1].Kind && compareStrict(item, vals[1]) == 0 {
					return NewBool(true), nil
				}
			}
			return NewBool(false), nil
		}
		if len(vals) == 2 && (vals[0].Kind == ValueNull || vals[1].Kind == ValueNull) {
			return NewNull(), nil
		}
		return NewNull(), reportIssue(ctx, "contains requires (string, string) or (list, scalar)")
	case "icontains":
		if len(vals) == 2 && vals[0].Kind == ValueString && vals[1].Kind == ValueString {
			return NewBool(strings.Contains(strings.ToLower(vals[0].String()), strings.ToLower(vals[1].String()))), nil
		}
	case "coalesce":
		for _, v := range vals {
			if !v.IsNull() {
				return v, nil
			}
		}
		return NewNull(), nil
	case "round":
		if len(vals) >= 1 && IsNumber(vals[0]) {
			var digits int64
			if len(vals) == 2 {
				if vals[1].IsNull() {
					return NewNull(), nil
				}
				digits = vals[1].Data.(int64)
			}
			return NewFloat(roundTo(Number(vals[0]), digits)), nil
		}
	case "floor":
		if len(vals) == 1 && IsNumber(vals[0]) {
			return NewFloat(math.Floor(Number(vals[0]))), nil
		}
	case "ceil":
		if len(vals) == 1 && IsNumber(vals[0]) {
			return NewFloat(math.Ceil(Number(vals[0]))), nil
		}
	case "date":
		if len(vals) == 1 && vals[0].Kind == ValueString {
			t, err := parseDate(vals[0].String())
			if err == nil {
				return NewDate(t), nil
			}
		}
	case "at":
		if len(vals) == 1 && vals[0].Kind == ValueInt {
			n := int(vals[0].Data.(int64))
			start := rowInt(row, "start")
			if start == 0 {
				start = rowInt(row, "line")
			}
			end := rowInt(row, "end")
			if end == 0 {
				end = start
			}
			return NewBool(start <= n && n <= end), nil
		}
	case "after_heading":
		if len(vals) >= 1 && vals[0].Kind == ValueString {
			level := 0
			if len(vals) == 2 {
				level = int(vals[1].Data.(int64))
				if level < 1 || level > 6 {
					return NewNull(), reportIssue(ctx, "after_heading level must be between 1 and 6")
				}
			}
			if ctx != nil && ctx.AfterHeading != nil {
				return NewBool(ctx.AfterHeading(row, vals[0].String(), level)), nil
			}
			return NewBool(false), nil
		}
	case "under":
		if len(vals) == 1 && vals[0].Kind == ValueString {
			target := strings.ToLower(vals[0].String())
			path := strings.ToLower(rowStr(row, "path"))
			section := strings.ToLower(rowStr(row, "section"))
			if section == target {
				return NewBool(true), nil
			}
			if path == target {
				return NewBool(true), nil
			}
			needle := " > " + target + " > "
			return NewBool(strings.Contains(" > "+path+" > ", needle)), nil
		}
	case "under_like":
		if len(vals) == 1 && vals[0].Kind == ValueString {
			pattern := vals[0].String()
			return NewBool(matchLike(rowStr(row, "path"), pattern, true) || matchLike(rowStr(row, "section"), pattern, true)), nil
		}
	case "under_contains":
		if len(vals) == 1 && vals[0].Kind == ValueString {
			target := strings.ToLower(vals[0].String())
			path := strings.ToLower(rowStr(row, "path"))
			section := strings.ToLower(rowStr(row, "section"))
			return NewBool(strings.Contains(path, target) || strings.Contains(section, target)), nil
		}
	case "has_tag":
		if len(vals) == 1 && vals[0].Kind == ValueString {
			target := strings.ToLower(vals[0].String())
			if ctx != nil && ctx.Tags != nil {
				for _, tag := range ctx.Tags(row) {
					if matchesTag(tag, target) {
						return NewBool(true), nil
					}
				}
			}
			if tags := row["tags"]; tags.Kind == ValueList {
				for _, tag := range tags.Data.([]Value) {
					if matchesTag(tag.String(), target) {
						return NewBool(true), nil
					}
				}
			}
			return NewBool(false), nil
		}
	case "basename":
		if len(vals) == 1 && vals[0].Kind == ValueString {
			return NewString(filepath.Base(vals[0].String())), nil
		}
	case "dirname":
		if len(vals) == 1 && vals[0].Kind == ValueString {
			return NewString(filepath.ToSlash(filepath.Dir(vals[0].String()))), nil
		}
	}
	return NewNull(), nil
}

func roundTo(value float64, digits int64) float64 {
	if digits > 308 {
		return value
	}
	if digits < -308 {
		return math.Copysign(0, value)
	}
	if digits >= 0 {
		scale := math.Pow10(int(digits))
		scaled := value * scale
		if math.IsInf(scaled, 0) {
			return value
		}
		return math.Round(scaled) / scale
	}
	scale := math.Pow10(int(-digits))
	return math.Round(value/scale) * scale
}

type BetweenExpr struct {
	Left      Expr
	Low, High Expr
	Not       bool
}

type IsNullExpr struct {
	Expr Expr
	Not  bool
}

func (i *IsNullExpr) eval(row Row, ctx *EvalContext) (Value, error) {
	v, err := i.Expr.eval(row, withoutMissingFields(ctx))
	if err != nil {
		return Value{}, err
	}
	ok := v.Kind == ValueNull
	if i.Not {
		ok = !ok
	}
	return NewBool(ok), nil
}

func (b *BetweenExpr) eval(row Row, ctx *EvalContext) (Value, error) {
	left, err := b.Left.eval(row, ctx)
	if err != nil {
		return Value{}, err
	}
	lo, err := b.Low.eval(row, ctx)
	if err != nil {
		return Value{}, err
	}
	hi, err := b.High.eval(row, ctx)
	if err != nil {
		return Value{}, err
	}
	if left.Kind == ValueNull || lo.Kind == ValueNull || hi.Kind == ValueNull {
		return NewBool(false), nil
	}
	leftLow, lo := coerceDateComparison(left, lo)
	leftHigh, hi := coerceDateComparison(left, hi)
	if message := comparisonProblem(">=", b.Left, b.Low, leftLow, lo); message != "" {
		return NewBool(false), reportIssue(ctx, message)
	}
	if message := comparisonProblem("<=", b.Left, b.High, leftHigh, hi); message != "" {
		return NewBool(false), reportIssue(ctx, message)
	}
	ok := compareStrict(leftLow, lo) >= 0 && compareStrict(leftHigh, hi) <= 0
	if b.Not {
		ok = !ok
	}
	return NewBool(ok), nil
}

type parser struct {
	tokens        []token
	source        string
	pos           int
	fileListStart int
	fileListEnd   int
	countAll      bool
}

func (p *parser) parseQuery() (*Query, error) {
	return p.parseQueryUntil(tEOF)
}

func (p *parser) parseQueryUntil(end tokenType) (*Query, error) {
	parentCountAll := p.countAll
	p.countAll = false
	defer func() { p.countAll = parentCountAll }()
	q := &Query{WithOpt: map[string]string{}}
	if !p.acceptValue("SELECT") {
		return nil, p.expected("SELECT")
	}
	if p.acceptValue("DISTINCT") {
		q.Distinct = true
	}

	for {
		if p.match(tStar) {
			p.next()
			q.Select = append(q.Select, SelectItem{Expr: &ColumnExpr{Name: "*"}, Alias: "*"})
		} else {
			expr, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			alias := ""
			if p.acceptValue("AS") {
				if !isIdentifier(p.peek()) {
					return nil, p.expected("alias after AS")
				}
				alias = p.next().lit
			}
			q.Select = append(q.Select, SelectItem{Expr: expr, Alias: alias})
		}
		if p.match(tComma) {
			p.next()
			continue
		}
		break
	}

	if !p.acceptValue("FROM") {
		return nil, p.expected("FROM")
	}
	if !isIdentifier(p.peek()) {
		return nil, p.expected("table name")
	}
	q.Table = p.next().lit

	if p.match(tIn) {
		p.next()
		var err error
		q.In, err = p.parseFileSpecs()
		if err != nil {
			return nil, err
		}
	}

	if p.acceptValue("WHERE") {
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		q.Where = e
	}

	if p.acceptValue("GROUP") {
		if !p.acceptValue("BY") {
			return nil, p.expected("BY after GROUP")
		}
		if !isIdentifier(p.peek()) {
			return nil, p.expected("column after GROUP BY")
		}
		for {
			if !isIdentifier(p.peek()) {
				return nil, p.expected("column after GROUP BY")
			}
			q.GroupByColumns = append(q.GroupByColumns, p.next().lit)
			if !p.match(tComma) {
				break
			}
			p.next()
		}
		q.GroupBy = q.GroupByColumns[0]
	}

	if p.acceptValue("HAVING") {
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		q.Having = e
	}

	if p.acceptValue("ORDER") {
		if !p.acceptValue("BY") {
			return nil, p.expected("BY after ORDER")
		}
		for {
			expr, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			desc := false
			if p.acceptValue("DESC") {
				desc = true
			} else {
				p.acceptValue("ASC")
			}
			q.OrderBy = append(q.OrderBy, OrderItem{Expr: expr, Desc: desc})
			if p.match(tComma) {
				p.next()
				continue
			}
			break
		}
	}

	if p.acceptValue("LIMIT") {
		if p.peek().typ != tInt {
			return nil, p.expected("integer after LIMIT")
		}
		limit, err := p.next().literalInt(strconv.IntSize)
		if err != nil {
			return nil, err
		}
		q.Limit = int(limit)
		q.LimitSet = true
		if p.acceptValue("PER") {
			for {
				if !isIdentifier(p.peek()) {
					return nil, p.expected("column after PER")
				}
				q.LimitPer = append(q.LimitPer, p.next().lit)
				if !p.match(tComma) {
					break
				}
				p.next()
			}
		}
	}
	if p.acceptValue("OFFSET") {
		if len(q.LimitPer) > 0 {
			return nil, fmt.Errorf("at byte offset %d: OFFSET cannot be used with LIMIT PER", p.peek().pos)
		}
		if p.peek().typ != tInt {
			return nil, p.expected("integer after OFFSET")
		}
		offset, err := p.next().literalInt(strconv.IntSize)
		if err != nil {
			return nil, err
		}
		q.Offset = int(offset)
	}

	if p.acceptValue("WITH") {
		for {
			if p.peek().typ != tIdent {
				return nil, p.expected("option name after WITH")
			}
			key := p.next().lit
			if !p.match(tEq) {
				return nil, p.expected("= after option name")
			}
			p.next()
			if p.peek().typ != tString && p.peek().typ != tInt && p.peek().typ != tDate && p.peek().typ != tIdent {
				return nil, p.expected("option value")
			}
			q.WithOpt[key] = p.next().lit
			if !p.match(tComma) {
				break
			}
			p.next()
		}
	}

	if p.peek().typ != end {
		expected := "end of query"
		if end == tRParen {
			expected = ") after subquery"
		}
		return nil, p.expected(expected)
	}
	q.CountAll = p.countAll
	return q, nil
}

func (p *parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.acceptValue("OR") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: "OR", Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parseCmp()
	if err != nil {
		return nil, err
	}
	for p.acceptValue("AND") {
		right, err := p.parseCmp()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: "AND", Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseCmp() (Expr, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}

	not := false
	if p.acceptValue("NOT") {
		not = true
	}

	if p.match(tIs) {
		p.next()
		notEq := p.acceptValue("NOT")
		if !p.match(tNull) {
			return nil, p.expected("NULL after IS")
		}
		p.next()
		return &IsNullExpr{Expr: left, Not: notEq}, nil
	}

	if p.match(tBetween) {
		p.next()
		lo, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		if !p.acceptValue("AND") {
			return nil, p.expected("AND after BETWEEN lower bound")
		}
		hi, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		return &BetweenExpr{Left: left, Low: lo, High: hi, Not: not}, nil
	}

	if p.match(tIn) {
		p.next()
		if !p.match(tLParen) {
			return nil, p.expected("( after IN")
		}
		p.next()
		if p.acceptValue("SELECT") {
			p.pos--
			start := p.peek().pos
			subquery, err := p.parseQueryUntil(tRParen)
			if err != nil {
				return nil, err
			}
			if len(subquery.In) > 0 {
				return nil, fmt.Errorf("at byte offset %d: file specifications are not supported inside a subquery; provide files to the outer query", start)
			}
			if p.source != "" {
				subquery.Raw = strings.TrimSpace(p.source[start:p.peek().pos])
			}
			p.next()
			op := "IN"
			if not {
				op = "NOT IN"
			}
			return &BinaryExpr{Op: op, Left: left, Right: &SubqueryExpr{Query: subquery}}, nil
		}
		vals := []Expr{}
		for {
			e, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			vals = append(vals, e)
			if p.match(tComma) {
				p.next()
				continue
			}
			break
		}
		if !p.match(tRParen) {
			return nil, p.expected(") after IN values")
		}
		p.next()
		items := make([]Value, 0, len(vals))
		for _, v := range vals {
			e, err := v.eval(Row{}, &EvalContext{})
			if err != nil {
				return nil, err
			}
			items = append(items, e)
		}
		op := "IN"
		if not {
			op = "NOT IN"
		}
		return &BinaryExpr{Op: op, Left: left, Right: &LiteralExpr{Value: NewList(items)}}, nil
	}

	if p.match(tIdent) && strings.EqualFold(p.peek().lit, "HAS") {
		p.next()
		rv, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		if not {
			return &UnaryExpr{Op: "NOT", Expr: &BinaryExpr{Op: "HAS", Left: left, Right: rv}}, nil
		}
		return &BinaryExpr{Op: "HAS", Left: left, Right: rv}, nil
	}

	if p.match(tLike) {
		op := "LIKE"
		if not {
			op = "NOT LIKE"
		}
		p.next()
		rv, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{Op: op, Left: left, Right: rv}, nil
	}
	if p.match(tRegex) {
		op := p.next().lit
		if not {
			if op == "~" {
				op = "!~"
			} else {
				op = "~"
			}
		}
		rv, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{Op: op, Left: left, Right: rv}, nil
	}
	if p.match(tRegexI) {
		op := p.next().lit
		if not {
			if op == "~*" {
				op = "!~*"
			} else {
				op = "~*"
			}
		}
		rv, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{Op: op, Left: left, Right: rv}, nil
	}

	if p.match(tEq) || p.match(tNeq) || p.match(tLt) || p.match(tLte) || p.match(tGt) || p.match(tGte) {
		op := p.next().lit
		rv, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		if not {
			if op == "=" {
				op = "!="
			} else if op == "!=" {
				op = "="
			} else if op == "<" {
				op = ">="
			} else if op == "<=" {
				op = ">"
			} else if op == ">" {
				op = "<="
			} else if op == ">=" {
				op = "<"
			}
		}
		return &BinaryExpr{Op: op, Left: left, Right: rv}, nil
	}
	if not {
		// leading NOT must apply to expression; keep as unary
		return &UnaryExpr{Op: "NOT", Expr: left}, nil
	}
	return left, nil
}

func (p *parser) parseAdd() (Expr, error) {
	left, err := p.parseMultiply()
	if err != nil {
		return nil, err
	}
	for p.match(tPlus) || p.match(tMinus) {
		op := p.next().lit
		right, err := p.parseMultiply()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseMultiply() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.match(tStar) || p.match(tSlash) || p.match(tPercent) {
		op := p.next().lit
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (Expr, error) {
	if p.acceptValue("NOT") {
		e, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: "NOT", Expr: e}, nil
	}
	if p.match(tPlus) {
		p.next()
		e, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: "PLUS", Expr: e}, nil
	}
	if p.match(tMinus) {
		sign := p.next()
		if p.match(tInt) {
			t := p.next()
			t.lit = "-" + t.lit
			t.pos = sign.pos
			value, err := t.literalInt(64)
			if err != nil {
				return nil, err
			}
			return &LiteralExpr{Value: NewInt(value)}, nil
		}
		e, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: "MINUS", Expr: e}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Expr, error) {
	t := p.next()
	switch t.typ {
	case tWriteFloat: // Only the write lexer emits this token.
		p.pos--
		value, err := parseWriteLiteral(p)
		if err != nil {
			return nil, err
		}
		return &LiteralExpr{Value: value}, nil
	case tIdent:
		if !isIdentifier(t) {
			return nil, expectedToken(t, "expression")
		}
		name := strings.ToLower(t.lit)
		contextValue := name == "today" || name == "now"
		if contextValue && !p.match(tLParen) {
			return &ContextValueExpr{Name: name}, nil
		}
		if p.match(tLParen) {
			if _, ok := aggregateFunctions[name]; ok {
				return p.parseAggregate(t)
			}
			if err := validateFunction(t.lit); err != nil {
				return nil, fmt.Errorf("at byte offset %d: %w; expected a supported function", t.pos, err)
			}
			p.next()
			args := []Expr{}
			if !p.match(tRParen) {
				for {
					a, err := p.parseOr()
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if p.match(tComma) {
						p.next()
						continue
					}
					break
				}
			}
			if !p.match(tRParen) {
				return nil, p.expected(")")
			}
			p.next()
			if err := validateFunctionArity(t.lit, len(args)); err != nil {
				return nil, fmt.Errorf("at byte offset %d: %w", t.pos, err)
			}
			if contextValue {
				return &ContextValueExpr{Name: name}, nil
			}
			return &FuncCallExpr{Name: t.lit, Args: args}, nil
		}
		return &ColumnExpr{Name: t.lit}, nil
	case tString:
		return &LiteralExpr{Value: NewString(t.lit)}, nil
	case tInt:
		value, err := t.literalInt(64)
		if err != nil {
			return nil, err
		}
		return &LiteralExpr{Value: NewInt(value)}, nil
	case tDuration:
		v, unit := parseDuration(t.lit)
		if unit == 0 {
			return nil, expectedToken(t, "duration")
		}
		return &LiteralExpr{Value: NewDuration(v, unit)}, nil
	case tDate:
		d, err := parseDate(t.lit)
		if err != nil {
			return nil, expectedToken(t, "date in YYYY-MM-DD format")
		}
		return &LiteralExpr{Value: NewDate(d)}, nil
	case tLParen:
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.match(tRParen) {
			return nil, p.expected(")")
		}
		p.next()
		return e, nil
	case tTrue:
		return &LiteralExpr{Value: NewBool(true)}, nil
	case tFalse:
		return &LiteralExpr{Value: NewBool(false)}, nil
	case tNull:
		return &LiteralExpr{Value: NewNull()}, nil
	default:
		return nil, expectedToken(t, "expression")
	}
}

func (p *parser) expected(expected string) error {
	return expectedToken(p.peek(), expected)
}

func expectedToken(t token, expected string) error {
	got := fmt.Sprintf("%q", t.lit)
	if t.typ == tEOF {
		got = "end of query"
	}
	return fmt.Errorf("at byte offset %d: expected %s, got %s", t.pos, expected, got)
}

func (p *parser) peek() token {
	if p.pos >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.pos]
}

func (p *parser) next() token {
	t := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

func (p *parser) match(t tokenType) bool {
	return p.peek().typ == t
}

func (p *parser) acceptValue(v string) bool {
	if p.peek().typ == tIdent && strings.EqualFold(p.peek().lit, v) {
		p.next()
		return true
	}
	return false
}

func isIdentifier(t token) bool {
	if t.typ != tIdent || isQueryClause(t) {
		return false
	}
	switch strings.ToUpper(t.lit) {
	case "SELECT", "FROM", "AS", "BY", "AND", "OR", "NOT", "ASC", "DESC":
		return false
	default:
		return true
	}
}

func isQueryClause(t token) bool {
	if t.typ != tIdent {
		return false
	}
	switch strings.ToUpper(t.lit) {
	case "WHERE", "GROUP", "HAVING", "ORDER", "LIMIT", "OFFSET", "WITH":
		return true
	default:
		return false
	}
}

type tokenType int

const (
	tEOF tokenType = iota
	tIdent
	tString
	tInt
	tDate
	tDuration
	tComma
	tLParen
	tRParen
	tStar
	tSlash
	tPercent
	tPlus
	tMinus
	tEq
	tNeq
	tLt
	tLte
	tGt
	tGte
	tIn
	tLike
	tRegex
	tRegexI
	tIs
	tBetween
	tTrue
	tFalse
	tNull
	tInvalid
)

type token struct {
	typ tokenType
	lit string
	pos int // Zero-based byte offset in the original UTF-8 query.
}

func (t token) literalInt(bits int) (int64, error) {
	i, err := strconv.ParseInt(t.lit, 10, bits)
	if err != nil {
		return 0, fmt.Errorf("at byte offset %d: integer literal %q is out of range for a signed %d-bit integer", t.pos, t.lit, bits)
	}
	return i, nil
}

type lexer struct {
	src string
	pos int
	err error
}

func (l *lexer) scan() []token {
	res := make([]token, 0)
	for {
		t := l.nextToken()
		res = append(res, t)
		if t.typ == tEOF {
			break
		}
	}
	return res
}

func (l *lexer) nextToken() (result token) {
	for l.pos < len(l.src) {
		if isSpace(l.src[l.pos]) {
			l.pos++
			continue
		}
		if l.src[l.pos] == '-' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '-' {
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
			continue
		}
		break
	}
	start := l.pos
	defer func() { result.pos = start }()
	if l.pos >= len(l.src) {
		return token{typ: tEOF}
	}

	c := l.src[l.pos]
	r, _ := utf8.DecodeRuneInString(l.src[l.pos:])
	if isLetter(r) {
		j := l.pos
		for j < len(l.src) {
			r, size := utf8.DecodeRuneInString(l.src[j:])
			if !isLetter(r) && !unicode.IsDigit(r) && r != '.' {
				break
			}
			j += size
		}
		lit := l.src[l.pos:j]
		l.pos = j
		normalized := strings.ToLower(lit)
		switch normalized {
		case "and", "or", "not", "in", "between", "like", "is", "null", "true", "false", "select", "from", "where", "group", "having", "order", "by", "asc", "desc", "limit", "per", "offset", "with", "has", "count", "as", "today", "now", "under", "under_like", "at", "has_tag":
			if normalized == "in" {
				return token{typ: tIn, lit: normalized}
			}
			if normalized == "between" {
				return token{typ: tBetween, lit: normalized}
			}
			if normalized == "like" {
				return token{typ: tLike, lit: normalized}
			}
			if normalized == "null" {
				return token{typ: tNull, lit: normalized}
			}
			if normalized == "true" {
				return token{typ: tTrue, lit: normalized}
			}
			if normalized == "false" {
				return token{typ: tFalse, lit: normalized}
			}
			if isDateLiteral(lit) {
				return token{typ: tDate, lit: lit}
			}
			if normalized == "is" {
				return token{typ: tIs, lit: normalized}
			}
			return token{typ: tIdent, lit: lit}
		default:
			return token{typ: tIdent, lit: lit}
		}
	}

	if isDigit(c) {
		if l.pos+10 <= len(l.src) && isDateLiteral(l.src[l.pos:l.pos+10]) {
			lit := l.src[l.pos : l.pos+10]
			l.pos += 10
			return token{typ: tDate, lit: lit}
		}
		j := l.pos
		for j < len(l.src) && isDigit(l.src[j]) {
			j++
		}
		if j < len(l.src) && (l.src[j] == 'd' || l.src[j] == 'w' || l.src[j] == 'm' || l.src[j] == 'y') {
			lit := l.src[l.pos : j+1]
			l.pos = j + 1
			return token{typ: tDuration, lit: lit}
		}
		lit := l.src[l.pos:j]
		l.pos = j
		return token{typ: tInt, lit: lit}
	}

	if c == '\'' || c == '"' {
		q := c
		j := l.pos + 1
		for j < len(l.src) {
			if l.src[j] == '\\' && j+1 < len(l.src) {
				j += 2
				continue
			}
			if l.src[j] == q {
				break
			}
			j++
		}
		if j >= len(l.src) {
			l.err = expectedToken(token{typ: tEOF, pos: len(l.src)}, fmt.Sprintf("closing %q for string at byte offset %d", string(q), start))
			l.pos = len(l.src)
			return token{typ: tEOF}
		}
		lit := l.src[l.pos+1 : j]
		l.pos = j + 1
		lit = strings.ReplaceAll(lit, "\\t", "\t")
		lit = strings.ReplaceAll(lit, "\\n", "\n")
		lit = strings.ReplaceAll(lit, "\\\"", "\"")
		lit = strings.ReplaceAll(lit, "\\'", "'")
		return token{typ: tString, lit: lit}
	}

	switch c {
	case ',':
		l.pos++
		return token{typ: tComma, lit: ","}
	case '(':
		l.pos++
		return token{typ: tLParen, lit: "("}
	case ')':
		l.pos++
		return token{typ: tRParen, lit: ")"}
	case '*':
		l.pos++
		return token{typ: tStar, lit: "*"}
	case '/':
		l.pos++
		return token{typ: tSlash, lit: "/"}
	case '%':
		l.pos++
		return token{typ: tPercent, lit: "%"}
	case '+':
		l.pos++
		return token{typ: tPlus, lit: "+"}
	case '-':
		l.pos++
		return token{typ: tMinus, lit: "-"}
	case '=':
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '=' {
			l.pos += 2
			return token{typ: tEq, lit: "=="}
		}
		l.pos++
		return token{typ: tEq, lit: "="}
	case '!':
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '=' {
			l.pos += 2
			return token{typ: tNeq, lit: "!="}
		}
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '~' {
			if l.pos+2 < len(l.src) && l.src[l.pos+2] == '*' {
				l.pos += 3
				return token{typ: tRegexI, lit: "!~*"}
			}
			l.pos += 2
			return token{typ: tRegex, lit: "!~"}
		}
		l.pos++
		return token{typ: tInvalid, lit: "!"}
	case '<':
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '=' {
			l.pos += 2
			return token{typ: tLte, lit: "<="}
		}
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '>' {
			l.pos += 2
			return token{typ: tNeq, lit: "<>"}
		}
		l.pos++
		return token{typ: tLt, lit: "<"}
	case '>':
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '=' {
			l.pos += 2
			return token{typ: tGte, lit: ">="}
		}
		l.pos++
		return token{typ: tGt, lit: ">"}
	case '~':
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '~' {
			if l.pos+2 < len(l.src) && l.src[l.pos+2] == '*' {
				l.pos += 3
				return token{typ: tRegexI, lit: "~*"}
			}
			l.pos += 2
			return token{typ: tRegex, lit: "~"}
		}
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '*' {
			l.pos += 2
			return token{typ: tRegexI, lit: "~*"}
		}
		l.pos++
		return token{typ: tRegex, lit: "~"}
	}

	r, size := utf8.DecodeRuneInString(l.src[l.pos:])
	l.pos += size
	return token{typ: tInvalid, lit: string(r)}
}

func isLetter(c rune) bool {
	return unicode.IsLetter(c) || c == '_' || c == '$'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\t' || c == '\r'
}

func isDateLiteral(s string) bool {
	_, err := parseDate(s)
	return err == nil
}

func parseDate(s string) (time.Time, error) {
	return time.Parse(time.DateOnly, strings.TrimSpace(s))
}

func parseDuration(raw string) (int, rune) {
	if raw == "" {
		return 0, 0
	}
	unit := rune(raw[len(raw)-1])
	val, err := strconv.Atoi(raw[:len(raw)-1])
	if err != nil {
		return 0, 0
	}
	return val, unit
}

func addOrNull(left, right Value, add bool) Value {
	if IsNumber(left) && IsNumber(right) && (left.Kind == ValueFloat || right.Kind == ValueFloat) {
		if add {
			return NewFloat(Number(left) + Number(right))
		}
		return NewFloat(Number(left) - Number(right))
	}
	if left.Kind == ValueInt && right.Kind == ValueInt {
		if add {
			return NewInt(left.Data.(int64) + right.Data.(int64))
		}
		return NewInt(left.Data.(int64) - right.Data.(int64))
	}
	if left.Kind == ValueDate {
		t := left.Data.(time.Time)
		if right.Kind == ValueDuration {
			dur := right.Data.(Duration)
			if !add {
				dur.Value = -dur.Value
			}
			return NewDate(addDuration(t, dur))
		}
		if right.Kind == ValueInt {
			v := int(right.Data.(int64))
			if !add {
				v = -v
			}
			return NewDate(t.AddDate(0, 0, v))
		}
	}
	if left.Kind == ValueDuration && right.Kind == ValueDate {
		d := left.Data.(Duration)
		t := right.Data.(time.Time)
		if !add {
			d.Value = -d.Value
		}
		return NewDate(addDuration(t, d))
	}
	return NewNull()
}

func addDuration(t time.Time, d Duration) time.Time {
	days := 0
	months := 0
	switch d.Unit {
	case 'd':
		days = d.Value
	case 'w':
		days = d.Value * 7
	case 'm':
		months = d.Value
	case 'y':
		months = d.Value * 12
	}
	if months != 0 {
		return t.AddDate(0, months, days)
	}
	return t.AddDate(0, 0, days)
}

func compareStrict(a, b Value) int {
	if IsNumber(a) && IsNumber(b) {
		return compareNumbers(a, b)
	}
	if a.Kind != b.Kind {
		return -1
	}
	if a.Kind == ValueNull {
		if b.Kind == ValueNull {
			return 0
		}
		return 0
	}
	switch a.Kind {
	case ValueInt:
		av := a.Data.(int64)
		bv := b.Data.(int64)
		if av == bv {
			return 0
		}
		if av < bv {
			return -1
		}
		return 1
	case ValueBool:
		av := a.Data.(bool)
		bv := b.Data.(bool)
		if av == bv {
			return 0
		}
		if !av && bv {
			return -1
		}
		return 1
	case ValueDate:
		ad := a.Data.(time.Time)
		bd := b.Data.(time.Time)
		if ad.Equal(bd) {
			return 0
		}
		if ad.Before(bd) {
			return -1
		}
		return 1
	case ValueDateTime:
		ad := a.Data.(time.Time)
		bd := b.Data.(time.Time)
		if ad.Equal(bd) {
			return 0
		}
		if ad.Before(bd) {
			return -1
		}
		return 1
	case ValueString:
		as := a.Data.(string)
		bs := b.Data.(string)
		if as == bs {
			return 0
		}
		if as < bs {
			return -1
		}
		return 1
	case ValueList:
		la := len(a.Data.([]Value))
		lb := len(b.Data.([]Value))
		if la == lb {
			return 0
		}
		if la < lb {
			return -1
		}
		return 1
	case ValueDuration:
		ad := a.Data.(Duration)
		bd := b.Data.(Duration)
		if ad.Unit != bd.Unit {
			return strings.Compare(a.String(), b.String())
		}
		if ad.Value == bd.Value {
			return 0
		}
		if ad.Value < bd.Value {
			return -1
		}
		return 1
	case ValueObject:
		return strings.Compare(a.String(), b.String())
	default:
		return strings.Compare(a.String(), b.String())
	}
}

func truthy(v Value) bool {
	switch v.Kind {
	case ValueNull:
		return false
	case ValueBool:
		return v.Data.(bool)
	case ValueInt:
		return v.Data.(int64) != 0
	case ValueFloat:
		return v.Data.(float64) != 0
	case ValueString:
		return v.Data.(string) != "" && v.Data.(string) != "-"
	case ValueList:
		return len(v.Data.([]Value)) > 0
	case ValueObject:
		return len(v.Data.(map[string]Value)) > 0
	default:
		return true
	}
}

func rowStr(row Row, key string) string {
	v, ok := row[key]
	if !ok {
		return ""
	}
	return v.String()
}

func rowInt(row Row, key string) int {
	v, ok := row[key]
	if !ok {
		return 0
	}
	if v.Kind != ValueInt {
		return 0
	}
	return int(v.Data.(int64))
}

func matchLike(value, pattern string, ignoreCase bool) bool {
	var r strings.Builder
	r.WriteString("(?s)")
	if ignoreCase {
		r.WriteString("(?i)")
	}
	r.WriteByte('^')
	chars := []rune(pattern)
	for i := 0; i < len(chars); i++ {
		c := chars[i]
		if c == '\\' && i+1 < len(chars) {
			next := chars[i+1]
			if next == '%' || next == '_' || next == '\\' {
				r.WriteString(regexp.QuoteMeta(string(next)))
				i++
				continue
			}
		}
		switch c {
		case '%':
			r.WriteString(".*")
		case '_':
			r.WriteByte('.')
		default:
			r.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	r.WriteByte('$')
	rex, err := regexp.Compile(r.String())
	if err != nil {
		return false
	}
	return rex.MatchString(value)
}

func regexMatch(value, pattern string, caseInsensitive bool) bool {
	prefix := ""
	if caseInsensitive {
		prefix = "(?i)"
	}
	rex, err := regexp.Compile(prefix + pattern)
	if err != nil {
		return false
	}
	return rex.MatchString(value)
}

func Order(rows []Row, by []OrderItem, ctx *EvalContext) error {
	type entry struct {
		row    Row
		values []Value
	}
	entries := make([]entry, len(rows))
	for i, row := range rows {
		entries[i] = entry{row: row, values: make([]Value, len(by))}
		for j, item := range by {
			value, err := Eval(item.Expr, row, ctx)
			if err != nil {
				return fmt.Errorf("ORDER BY: %w", err)
			}
			entries[i].values[j] = value
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		for k, oi := range by {
			c := compareSortValue(entries[i].values[k], entries[j].values[k])
			if c == 0 {
				continue
			}
			if oi.Desc {
				return c > 0
			}
			return c < 0
		}
		return false
	})
	for i, item := range entries {
		rows[i] = item.row
	}
	return nil
}

func compareSortValue(a, b Value) int {
	if a.Kind == ValueNull {
		if b.Kind == ValueNull {
			return 0
		}
		return 1
	}
	if b.Kind == ValueNull {
		return -1
	}
	if IsNumber(a) && IsNumber(b) {
		return compareNumbers(a, b)
	}
	if a.Kind != b.Kind {
		if a.Kind < b.Kind {
			return -1
		}
		return 1
	}
	return compareStrict(a, b)
}

// EvalContext carries runtime values and diagnostics for expression evaluation.
type EvalContext struct {
	Today        time.Time
	Now          time.Time
	Report       func(string) error
	MissingField func(string) error
	Tags         func(Row) []string
	AfterHeading func(Row, string, int) bool
}
