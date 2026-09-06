package engine

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
	"github.com/Valiev/mdsql/internal/tables"
)

// Executor consumes parsed documents one at a time. Values retained for final
// ordering or aggregation are copied so they do not keep source documents live.
type Executor struct {
	q             *query.Query
	table         tables.TableDef
	opts          Options
	orderBy       []query.OrderItem
	groupColumns  []*query.ColumnExpr
	limitPer      []*query.ColumnExpr
	plan          *aggregatePlan
	rows          []streamRow
	aggregateRows []query.Row
	tags          rowTagContext
	headings      rowHeadingContext
	present       map[string]bool
	missing       map[string]bool
	warnings      []string
	seenWarnings  map[string]bool
}

type streamRow struct {
	projected   query.Row
	orderValues []query.Value
	groupValues []query.Value
}

func NewExecutor(q *query.Query, opts Options) (*Executor, error) {
	if q == nil {
		return nil, fmt.Errorf("query is required")
	}
	if len(q.Subqueries()) > 0 {
		return nil, fmt.Errorf("streaming execution requires resolved subqueries")
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.Today.IsZero() {
		now := time.Now()
		opts.Today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}
	tdef, ok := tables.Table(q.Table)
	if !ok {
		return nil, fmt.Errorf("unknown table %q", q.Table)
	}
	expanded := *q
	var err error
	expanded.Select, err = nameProjections(expandStars(q.Select, tdef))
	if err != nil {
		return nil, err
	}
	expanded.OrderBy = append([]query.OrderItem(nil), q.OrderBy...)
	q = &expanded

	aliases := map[string]query.Expr{}
	for _, item := range q.Select {
		if item.Alias != "" && item.Alias != "*" {
			aliases[item.Alias] = item.Expr
		}
	}
	allowed := allowedColumns(q, tdef, nil)
	orderBy, err := resolveOrderBy(q.OrderBy, q.Select, aliases)
	if err != nil {
		return nil, err
	}
	q.Where = resolveAliases(q.Where, whereAliases(aliases, allowed))
	groupNames := q.GroupByColumns
	if len(groupNames) == 0 && q.GroupBy != "" {
		groupNames = []string{q.GroupBy}
	}
	groupColumns := make([]*query.ColumnExpr, 0, len(groupNames))
	for _, name := range groupNames {
		expr := resolveAliases(&query.ColumnExpr{Name: name}, aliases)
		column, ok := expr.(*query.ColumnExpr)
		if !ok {
			return nil, fmt.Errorf("GROUP BY must resolve to non-aggregate columns")
		}
		groupColumns = append(groupColumns, column)
	}
	q.Having = resolveAliases(q.Having, aliases)
	limitPer, err := resolveGroupColumns(q.LimitPer, aliases, "LIMIT PER")
	if err != nil {
		return nil, err
	}
	e := &Executor{
		q: q, table: tdef, opts: opts, orderBy: orderBy,
		groupColumns: groupColumns, limitPer: limitPer,
		present: map[string]bool{}, missing: map[string]bool{},
		warnings: []string{}, seenWarnings: map[string]bool{},
	}
	if err := validateQueryColumns(q, tdef, allowed, orderBy, groupColumns, limitPer, e.report); err != nil {
		return nil, err
	}
	e.plan, err = planAggregates(q, orderBy, groupColumns)
	if err != nil {
		return nil, err
	}
	if e.plan != nil {
		e.tags = rowTagContext{}
		if queryUsesFunction(q, "after_heading") {
			e.headings = rowHeadingContext{}
		}
	}
	return e, nil
}

func (e *Executor) ConsumeDocument(doc *model.Document) error {
	if doc == nil {
		return nil
	}
	for name := range presentFrontmatterFields([]*model.Document{doc}) {
		e.present[name] = true
	}
	rows := e.table.Rows(doc)
	tags := rowTagContext{}
	tags.bind(e.q.Table, doc, rows)
	var headings rowHeadingContext
	if queryUsesFunction(e.q, "after_heading") {
		headings = rowHeadingContext{}
		headings.bind(e.q.Table, doc, rows)
	}
	whereContext := e.evalContext(tags, headings, true)
	rowContext := e.evalContext(tags, headings, false)
	for _, row := range rows {
		if e.q.Where != nil {
			value, err := query.Eval(e.q.Where, row, whereContext)
			if err != nil {
				return err
			}
			if !truthy(value) {
				continue
			}
		}
		if e.plan != nil {
			cloned := cloneRow(row)
			e.aggregateRows = append(e.aggregateRows, cloned)
			if values := tags.forRow(row); len(values) > 0 {
				e.tags[reflect.ValueOf(cloned)] = cloneStrings(values)
			}
			if headings != nil {
				e.headings[reflect.ValueOf(cloned)] = cloneHeadingIndex(headings[reflect.ValueOf(row)])
			}
			continue
		}
		projected, err := projectRows(e.q.Select, []query.Row{row}, rowContext)
		if err != nil {
			return err
		}
		item := streamRow{projected: cloneRow(projected[0])}
		order := e.orderBy
		if len(order) == 0 {
			order = []query.OrderItem{{Expr: &query.ColumnExpr{Name: "file"}}, {Expr: &query.ColumnExpr{Name: "start"}}}
		}
		item.orderValues, err = evalValues(orderExpressions(order), row, rowContext)
		if err != nil {
			return fmt.Errorf("ORDER BY: %w", err)
		}
		item.groupValues, err = evalValues(columnExpressions(e.limitPer), row, rowContext)
		if err != nil {
			return err
		}
		item.orderValues = cloneValues(item.orderValues)
		item.groupValues = cloneValues(item.groupValues)
		e.rows = append(e.rows, item)
	}
	return nil
}

func (e *Executor) Finish() (*ExecuteResult, error) {
	for name := range e.missing {
		if e.present[name] {
			continue
		}
		if err := e.report(fmt.Sprintf("missing field %q in WHERE; use IS NULL or coalesce for optional fields", name)); err != nil {
			return nil, err
		}
	}
	ctx := e.evalContext(e.tags, e.headings, false)
	var projected []query.Row
	if e.plan != nil {
		filtered, err := e.plan.execute(e.groupColumns, e.aggregateRows, ctx)
		if err != nil {
			return nil, err
		}
		if e.q.Having != nil {
			havingRows := filtered[:0]
			for _, row := range filtered {
				value, evalErr := query.Eval(e.q.Having, row, ctx)
				if evalErr != nil {
					return nil, evalErr
				}
				if truthy(value) {
					havingRows = append(havingRows, row)
				}
			}
			filtered = havingRows
		}
		if len(e.orderBy) > 0 {
			if err := query.Order(filtered, e.orderBy, ctx); err != nil {
				return nil, err
			}
		}
		projected, err = projectRows(e.q.Select, filtered, ctx)
		if err != nil {
			return nil, err
		}
	} else {
		e.sortRows()
		if len(e.limitPer) > 0 && e.q.Limit > 0 {
			if err := e.limitRowsPerGroup(); err != nil {
				return nil, err
			}
		}
		projected = make([]query.Row, len(e.rows))
		for i := range e.rows {
			projected[i] = e.rows[i].projected
		}
	}
	if e.q.Distinct {
		projected = uniqueRows(projected)
	}
	if e.q.Offset > 0 {
		if e.q.Offset >= len(projected) {
			projected = []query.Row{}
		} else {
			projected = projected[e.q.Offset:]
		}
	}
	limit := e.q.Limit
	if len(e.limitPer) > 0 {
		limit = 0
	}
	if e.opts.TableLimit > 0 && (limit == 0 || e.opts.TableLimit < limit) {
		limit = e.opts.TableLimit
	}
	if limit > 0 && len(projected) > limit {
		projected = projected[:limit]
	}
	return &ExecuteResult{Rows: projected, Columns: e.q.SelectKeys(), Warnings: e.warnings}, nil
}

func (e *Executor) evalContext(tags rowTagContext, headings rowHeadingContext, collectMissing bool) *query.EvalContext {
	ctx := &query.EvalContext{
		Today: dateOnly(e.opts.Today), Now: e.opts.Now, Report: e.report,
		Tags: tags.forRow, AfterHeading: headings.after,
	}
	if collectMissing {
		ctx.MissingField = func(name string) error {
			e.missing[name] = true
			return nil
		}
	}
	return ctx
}

func (e *Executor) report(message string) error {
	if e.opts.Strict {
		return fmt.Errorf("%s", message)
	}
	if !e.seenWarnings[message] {
		e.seenWarnings[message] = true
		e.warnings = append(e.warnings, message)
	}
	return nil
}

func (e *Executor) sortRows() {
	order := e.orderBy
	if len(order) == 0 {
		order = []query.OrderItem{{}, {}}
	}
	sort.SliceStable(e.rows, func(i, j int) bool {
		for k := range order {
			comparison := compareStreamValue(e.rows[i].orderValues[k], e.rows[j].orderValues[k])
			if comparison == 0 {
				continue
			}
			if order[k].Desc {
				return comparison > 0
			}
			return comparison < 0
		}
		return false
	})
}

func (e *Executor) limitRowsPerGroup() error {
	counts := map[string]int{}
	kept := e.rows[:0]
	for _, row := range e.rows {
		var key strings.Builder
		for i, value := range row.groupValues {
			if value.Kind == query.ValueList || value.Kind == query.ValueObject {
				return fmt.Errorf("LIMIT PER column %q requires scalar values, got %s", e.limitPer[i].Name, value.Kind)
			}
			if i > 0 {
				key.WriteByte('|')
			}
			writeDistinctValue(&key, value)
		}
		encoded := key.String()
		if counts[encoded] >= e.q.Limit {
			continue
		}
		counts[encoded]++
		kept = append(kept, row)
	}
	e.rows = kept
	return nil
}

func orderExpressions(items []query.OrderItem) []query.Expr {
	result := make([]query.Expr, len(items))
	for i := range items {
		result[i] = items[i].Expr
	}
	return result
}

func columnExpressions(items []*query.ColumnExpr) []query.Expr {
	result := make([]query.Expr, len(items))
	for i := range items {
		result[i] = items[i]
	}
	return result
}

func evalValues(expressions []query.Expr, row query.Row, ctx *query.EvalContext) ([]query.Value, error) {
	values := make([]query.Value, len(expressions))
	for i, expression := range expressions {
		value, err := query.Eval(expression, row, ctx)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return values, nil
}

func compareStreamValue(left, right query.Value) int {
	if left.Kind == query.ValueNull {
		if right.Kind == query.ValueNull {
			return 0
		}
		return 1
	}
	if right.Kind == query.ValueNull {
		return -1
	}
	if query.IsNumber(left) && query.IsNumber(right) {
		return query.Compare(left, right)
	}
	if left.Kind != right.Kind {
		if left.Kind < right.Kind {
			return -1
		}
		return 1
	}
	return query.Compare(left, right)
}

func cloneRow(row query.Row) query.Row {
	cloned := make(query.Row, len(row))
	for key, value := range row {
		cloned[strings.Clone(key)] = cloneValue(value)
	}
	return cloned
}

func cloneValues(values []query.Value) []query.Value {
	cloned := make([]query.Value, len(values))
	for i, value := range values {
		cloned[i] = cloneValue(value)
	}
	return cloned
}

func cloneValue(value query.Value) query.Value {
	switch value.Kind {
	case query.ValueString:
		value.Data = strings.Clone(value.Data.(string))
	case query.ValueList:
		value.Data = cloneValues(value.Data.([]query.Value))
	case query.ValueObject:
		object := value.Data.(map[string]query.Value)
		cloned := make(map[string]query.Value, len(object))
		for key, child := range object {
			cloned[strings.Clone(key)] = cloneValue(child)
		}
		value.Data = cloned
	}
	return value
}

func cloneStrings(values []string) []string {
	cloned := make([]string, len(values))
	for i, value := range values {
		cloned[i] = strings.Clone(value)
	}
	return cloned
}

func cloneHeadingIndex(index headingIndex) headingIndex {
	if index == nil {
		return nil
	}
	cloned := make(headingIndex, len(index))
	for key, value := range index {
		cloned[strings.Clone(key)] = value
	}
	return cloned
}
