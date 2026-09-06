package engine

import (
	"fmt"
	"strings"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
	"github.com/Valiev/mdsql/internal/tables"
)

type Options struct {
	Today      time.Time
	Now        time.Time
	TableLimit int
	Strict     bool
}

type ExecuteResult struct {
	Rows     []query.Row
	Columns  []string
	Warnings []string
}

func Execute(q *query.Query, docs []*model.Document, opts Options) (result *ExecuteResult, err error) {
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
	expanded.Select, err = nameProjections(expandStars(q.Select, tdef))
	if err != nil {
		return nil, err
	}
	expanded.OrderBy = append([]query.OrderItem(nil), q.OrderBy...)
	q = &expanded
	rows := make([]query.Row, 0)
	tags := rowTagContext{}
	var headings rowHeadingContext
	if queryUsesFunction(q, "after_heading") {
		headings = rowHeadingContext{}
	}
	for _, d := range docs {
		docRows := tdef.Rows(d)
		tags.bind(q.Table, d, docRows)
		headings.bind(q.Table, d, docRows)
		rows = append(rows, docRows...)
	}

	warnings := []string{}
	seenWarnings := map[string]bool{}
	report := func(message string) error {
		if opts.Strict {
			return fmt.Errorf("%s", message)
		}
		if !seenWarnings[message] {
			seenWarnings[message] = true
			warnings = append(warnings, message)
		}
		return nil
	}
	defer func() {
		if result != nil {
			result.Warnings = warnings
		}
	}()
	for i := range q.Select {
		q.Select[i].Expr, err = resolveSubqueries(q.Select[i].Expr, docs, opts, report)
		if err != nil {
			return nil, err
		}
	}
	q.Where, err = resolveSubqueries(q.Where, docs, opts, report)
	if err != nil {
		return nil, err
	}
	q.Having, err = resolveSubqueries(q.Having, docs, opts, report)
	if err != nil {
		return nil, err
	}
	for i := range q.OrderBy {
		q.OrderBy[i].Expr, err = resolveSubqueries(q.OrderBy[i].Expr, docs, opts, report)
		if err != nil {
			return nil, err
		}
	}

	allowed := allowedColumns(q, tdef, rows)
	aliases := map[string]query.Expr{}
	for _, item := range q.Select {
		if item.Alias != "" && item.Alias != "*" {
			aliases[item.Alias] = item.Expr
		}
	}
	orderBy, err := resolveOrderBy(q.OrderBy, q.Select, aliases)
	if err != nil {
		return nil, err
	}
	q.Where = resolveAliases(q.Where, whereAliases(aliases, allowed))
	groupColumns := make([]*query.ColumnExpr, 0, len(q.GroupByColumns))
	groupNames := q.GroupByColumns
	if len(groupNames) == 0 && q.GroupBy != "" {
		groupNames = []string{q.GroupBy}
	}
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
	if err := validateQueryColumns(q, tdef, allowed, orderBy, groupColumns, limitPer, report); err != nil {
		return nil, err
	}
	plan, err := planAggregates(q, orderBy, groupColumns)
	if err != nil {
		return nil, err
	}
	var optionalFields map[string]bool
	if q.Where != nil {
		optionalFields = presentFrontmatterFields(docs)
	}
	ctx := &query.EvalContext{
		Today:        dateOnly(opts.Today),
		Now:          opts.Now,
		Report:       report,
		Tags:         tags.forRow,
		AfterHeading: headings.after,
		MissingField: func(name string) error {
			if err := validateExprColumns(&query.ColumnExpr{Name: name}, allowed, tdef.Name); err != nil {
				return report(err.Error())
			}
			if optionalFields[name] {
				return nil
			}
			return report(fmt.Sprintf("missing field %q in WHERE; use IS NULL or coalesce for optional fields", name))
		},
	}
	filtered := make([]query.Row, 0, len(rows))
	for _, row := range rows {
		if q.Where != nil {
			value, err := query.Eval(q.Where, row, ctx)
			if err != nil {
				return nil, err
			}
			if !truthy(value) {
				continue
			}
		}
		filtered = append(filtered, row)
	}
	// Missing optional fields remain NULL in projections and grouping/sort keys.
	ctx.MissingField = nil
	if plan != nil {
		filtered, err = plan.execute(groupColumns, filtered, ctx)
		if err != nil {
			return nil, err
		}
	}
	if q.Having != nil {
		if plan == nil {
			return nil, fmt.Errorf("HAVING requires GROUP BY or an aggregate function")
		}
		havingRows := filtered[:0]
		for _, row := range filtered {
			value, evalErr := query.Eval(q.Having, row, ctx)
			if evalErr != nil {
				return nil, evalErr
			}
			if truthy(value) {
				havingRows = append(havingRows, row)
			}
		}
		filtered = havingRows
	}
	if len(orderBy) > 0 {
		if err := query.Order(filtered, orderBy, ctx); err != nil {
			return nil, err
		}
	} else if plan == nil {
		if err := query.Order(filtered, []query.OrderItem{
			{Expr: &query.ColumnExpr{Name: "file"}},
			{Expr: &query.ColumnExpr{Name: "start"}},
		}, ctx); err != nil {
			return nil, err
		}
	}
	if len(limitPer) > 0 && q.Limit > 0 {
		filtered, err = limitRowsPerGroup(filtered, limitPer, q.Limit, ctx)
		if err != nil {
			return nil, err
		}
	}
	projected, err := projectRows(q.Select, filtered, ctx)
	if err != nil {
		return nil, err
	}
	if q.Distinct {
		projected = uniqueRows(projected)
	}
	if q.Offset > 0 {
		if q.Offset >= len(projected) {
			projected = []query.Row{}
		} else {
			projected = projected[q.Offset:]
		}
	}
	limit := q.Limit
	if len(limitPer) > 0 {
		limit = 0
	}
	if opts.TableLimit > 0 && (limit == 0 || opts.TableLimit < limit) {
		limit = opts.TableLimit
	}
	if limit > 0 && len(projected) > limit {
		projected = projected[:limit]
	}
	return &ExecuteResult{Rows: projected, Columns: q.SelectKeys()}, nil
}

func resolveSubqueries(expr query.Expr, docs []*model.Document, opts Options, report func(string) error) (query.Expr, error) {
	if expr == nil {
		return nil, nil
	}
	if !hasSubquery(expr) {
		return expr, nil
	}
	switch value := expr.(type) {
	case *query.SubqueryExpr:
		if value.Query == nil {
			return nil, fmt.Errorf("invalid empty subquery")
		}
		subOpts := opts
		// A display safety limit must not change membership semantics.
		subOpts.TableLimit = 0
		result, err := Execute(value.Query, docs, subOpts)
		if err != nil {
			return nil, fmt.Errorf("subquery failed: %w", err)
		}
		if len(result.Columns) != 1 {
			return nil, fmt.Errorf("IN subquery must return exactly one column, got %d", len(result.Columns))
		}
		for _, warning := range result.Warnings {
			if err := report("subquery: " + warning); err != nil {
				return nil, err
			}
		}
		column := result.Columns[0]
		items := make([]query.Value, 0, len(result.Rows))
		for _, row := range result.Rows {
			items = append(items, row[column])
		}
		return &query.LiteralExpr{Value: query.NewList(items)}, nil
	case *query.BinaryExpr:
		left, err := resolveSubqueries(value.Left, docs, opts, report)
		if err != nil {
			return nil, err
		}
		right, err := resolveSubqueries(value.Right, docs, opts, report)
		if err != nil {
			return nil, err
		}
		return &query.BinaryExpr{Op: value.Op, Left: left, Right: right}, nil
	case *query.UnaryExpr:
		child, err := resolveSubqueries(value.Expr, docs, opts, report)
		if err != nil {
			return nil, err
		}
		return &query.UnaryExpr{Op: value.Op, Expr: child}, nil
	case *query.BetweenExpr:
		left, err := resolveSubqueries(value.Left, docs, opts, report)
		if err != nil {
			return nil, err
		}
		low, err := resolveSubqueries(value.Low, docs, opts, report)
		if err != nil {
			return nil, err
		}
		high, err := resolveSubqueries(value.High, docs, opts, report)
		if err != nil {
			return nil, err
		}
		return &query.BetweenExpr{Left: left, Low: low, High: high, Not: value.Not}, nil
	case *query.IsNullExpr:
		child, err := resolveSubqueries(value.Expr, docs, opts, report)
		if err != nil {
			return nil, err
		}
		return &query.IsNullExpr{Expr: child, Not: value.Not}, nil
	case *query.FuncCallExpr:
		args := make([]query.Expr, len(value.Args))
		for i, arg := range value.Args {
			var err error
			args[i], err = resolveSubqueries(arg, docs, opts, report)
			if err != nil {
				return nil, err
			}
		}
		return &query.FuncCallExpr{Name: value.Name, Args: args}, nil
	case *query.AggregateExpr:
		arg, err := resolveSubqueries(value.Arg, docs, opts, report)
		if err != nil {
			return nil, err
		}
		return &query.AggregateExpr{Name: value.Name, Arg: arg, Star: value.Star}, nil
	default:
		return expr, nil
	}
}

func hasSubquery(expr query.Expr) bool {
	switch value := expr.(type) {
	case *query.SubqueryExpr:
		return true
	case *query.BinaryExpr:
		return hasSubquery(value.Left) || hasSubquery(value.Right)
	case *query.UnaryExpr:
		return hasSubquery(value.Expr)
	case *query.BetweenExpr:
		return hasSubquery(value.Left) || hasSubquery(value.Low) || hasSubquery(value.High)
	case *query.IsNullExpr:
		return hasSubquery(value.Expr)
	case *query.FuncCallExpr:
		for _, arg := range value.Args {
			if hasSubquery(arg) {
				return true
			}
		}
	case *query.AggregateExpr:
		return hasSubquery(value.Arg)
	}
	return false
}

func queryUsesFunction(q *query.Query, name string) bool {
	for _, item := range q.Select {
		if exprUsesFunction(item.Expr, name) {
			return true
		}
	}
	if exprUsesFunction(q.Where, name) || exprUsesFunction(q.Having, name) {
		return true
	}
	for _, item := range q.OrderBy {
		if exprUsesFunction(item.Expr, name) {
			return true
		}
	}
	return false
}

func exprUsesFunction(expr query.Expr, name string) bool {
	switch value := expr.(type) {
	case *query.FuncCallExpr:
		if strings.EqualFold(value.Name, name) {
			return true
		}
		for _, arg := range value.Args {
			if exprUsesFunction(arg, name) {
				return true
			}
		}
	case *query.BinaryExpr:
		return exprUsesFunction(value.Left, name) || exprUsesFunction(value.Right, name)
	case *query.UnaryExpr:
		return exprUsesFunction(value.Expr, name)
	case *query.BetweenExpr:
		return exprUsesFunction(value.Left, name) || exprUsesFunction(value.Low, name) || exprUsesFunction(value.High, name)
	case *query.IsNullExpr:
		return exprUsesFunction(value.Expr, name)
	case *query.AggregateExpr:
		return exprUsesFunction(value.Arg, name)
	}
	return false
}

func expandStars(selected []query.SelectItem, table tables.TableDef) []query.SelectItem {
	expanded := make([]query.SelectItem, 0, len(selected))
	for _, item := range selected {
		if item.Alias != "*" && item.Expr != nil && !isStar(item.Expr) {
			expanded = append(expanded, item)
			continue
		}
		for _, column := range table.Columns {
			if column.Name == "fm" && table.Name != "frontmatter" {
				continue
			}
			expanded = append(expanded, query.SelectItem{Expr: &query.ColumnExpr{Name: column.Name}})
		}
	}
	return expanded
}

func projectRows(selected []query.SelectItem, rows []query.Row, ctx *query.EvalContext) ([]query.Row, error) {
	if len(selected) == 0 {
		return rows, nil
	}
	out := make([]query.Row, 0, len(rows))
	for _, row := range rows {
		projected := query.Row{}
		for _, item := range selected {
			if item.Alias == "*" || item.Expr == nil || isStar(item.Expr) {
				for key, value := range row {
					projected[key] = value
				}
				continue
			}
			value, err := query.Eval(item.Expr, row, ctx)
			if err != nil {
				return nil, err
			}
			name := strings.TrimSpace(item.Alias)
			if name == "" {
				switch expr := item.Expr.(type) {
				case *query.ColumnExpr:
					name = expr.Name
				case *query.FuncCallExpr:
					name = expr.Name
				default:
					name = "expr"
				}
			}
			projected[name] = value
		}
		out = append(out, projected)
	}
	return out, nil
}

func allowedColumns(q *query.Query, table tables.TableDef, rows []query.Row) map[string]struct{} {
	allowed := map[string]struct{}{"*": {}}
	for _, column := range table.Columns {
		allowed[column.Name] = struct{}{}
	}
	var addObject func(string, query.Value)
	addObject = func(prefix string, value query.Value) {
		if value.Kind != query.ValueObject {
			return
		}
		for key, child := range value.Data.(map[string]query.Value) {
			name := prefix + "." + key
			allowed[name] = struct{}{}
			addObject(name, child)
		}
	}
	for _, row := range rows {
		for name, value := range row {
			allowed[name] = struct{}{}
			addObject(name, value)
		}
	}
	return allowed
}

func validateQueryColumns(q *query.Query, table tables.TableDef, allowed map[string]struct{}, orderBy []query.OrderItem, groupColumns, limitPer []*query.ColumnExpr, report func(string) error) error {
	if query.HasAggregate(q.Where) {
		return fmt.Errorf("aggregate functions are not allowed in WHERE")
	}
	for _, item := range q.Select {
		if err := validateExprColumns(item.Expr, allowed, table.Name); err != nil {
			return err
		}
	}
	if err := validateExprColumns(q.Where, allowed, table.Name); err != nil {
		if err := report(err.Error()); err != nil {
			return err
		}
	}
	for _, item := range orderBy {
		if err := validateExprColumns(item.Expr, allowed, table.Name); err != nil {
			return err
		}
	}
	if err := validateExprColumns(q.Having, allowed, table.Name); err != nil {
		return err
	}
	for _, groupColumn := range append(groupColumns, limitPer...) {
		if err := validateExprColumns(groupColumn, allowed, table.Name); err != nil {
			return err
		}
	}

	return nil
}

func resolveGroupColumns(names []string, aliases map[string]query.Expr, clause string) ([]*query.ColumnExpr, error) {
	columns := make([]*query.ColumnExpr, 0, len(names))
	for _, name := range names {
		expr := resolveAliases(&query.ColumnExpr{Name: name}, aliases)
		column, ok := expr.(*query.ColumnExpr)
		if !ok {
			return nil, fmt.Errorf("%s must resolve to non-aggregate columns", clause)
		}
		columns = append(columns, column)
	}
	return columns, nil
}

func limitRowsPerGroup(rows []query.Row, columns []*query.ColumnExpr, limit int, ctx *query.EvalContext) ([]query.Row, error) {
	counts := map[string]int{}
	result := make([]query.Row, 0, len(rows))
	for _, row := range rows {
		var key strings.Builder
		for _, column := range columns {
			value, err := query.Eval(column, row, ctx)
			if err != nil {
				return nil, err
			}
			if value.Kind == query.ValueList || value.Kind == query.ValueObject {
				return nil, fmt.Errorf("LIMIT PER column %q requires scalar values, got %s", column.Name, value.Kind)
			}
			writeDistinctValue(&key, value)
		}
		encoded := key.String()
		if counts[encoded] >= limit {
			continue
		}
		counts[encoded]++
		result = append(result, row)
	}
	return result, nil
}

func validateExprColumns(expr query.Expr, allowed map[string]struct{}, table string) error {
	if expr == nil {
		return nil
	}
	switch value := expr.(type) {
	case *query.ColumnExpr:
		name := value.Name
		if _, ok := allowed[name]; ok {
			return nil
		}
		base := strings.SplitN(name, ".", 2)[0]
		if _, ok := allowed[base]; !ok {
			hint := ""
			if _, exists := allowed["fm."+name]; exists {
				hint = fmt.Sprintf("; did you mean %q?", "fm."+name)
			}
			return fmt.Errorf("unknown column %q in table %q%s", name, table, hint)
		}
	case *query.BinaryExpr:
		if err := validateExprColumns(value.Left, allowed, table); err != nil {
			return err
		}
		return validateExprColumns(value.Right, allowed, table)
	case *query.UnaryExpr:
		return validateExprColumns(value.Expr, allowed, table)
	case *query.BetweenExpr:
		for _, child := range []query.Expr{value.Left, value.Low, value.High} {
			if err := validateExprColumns(child, allowed, table); err != nil {
				return err
			}
		}
	case *query.IsNullExpr:
		return validateExprColumns(value.Expr, allowed, table)
	case *query.AggregateExpr:
		return validateExprColumns(value.Arg, allowed, table)
	case *query.FuncCallExpr:
		for _, argument := range value.Args {
			if err := validateExprColumns(argument, allowed, table); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveAliases(expr query.Expr, aliases map[string]query.Expr) query.Expr {
	switch value := expr.(type) {
	case *query.ColumnExpr:
		if source, ok := aliases[value.Name]; ok {
			// SELECT expressions resolve against source columns, not other aliases.
			return source
		}
	case *query.BinaryExpr:
		return &query.BinaryExpr{Op: value.Op, Left: resolveAliases(value.Left, aliases), Right: resolveAliases(value.Right, aliases)}
	case *query.UnaryExpr:
		return &query.UnaryExpr{Op: value.Op, Expr: resolveAliases(value.Expr, aliases)}
	case *query.BetweenExpr:
		return &query.BetweenExpr{Left: resolveAliases(value.Left, aliases), Low: resolveAliases(value.Low, aliases), High: resolveAliases(value.High, aliases), Not: value.Not}
	case *query.IsNullExpr:
		return &query.IsNullExpr{Expr: resolveAliases(value.Expr, aliases), Not: value.Not}
	case *query.FuncCallExpr:
		args := make([]query.Expr, len(value.Args))
		for i, arg := range value.Args {
			args[i] = resolveAliases(arg, aliases)
		}
		return &query.FuncCallExpr{Name: value.Name, Args: args}
	}
	return expr
}

func uniqueRows(in []query.Row) []query.Row {
	seen := map[string]struct{}{}
	res := make([]query.Row, 0, len(in))
	for _, row := range in {
		var encoded strings.Builder
		writeDistinctObject(&encoded, row, true)
		key := encoded.String()
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			res = append(res, row)
		}
	}
	return res
}

func truthy(value query.Value) bool {
	switch value.Kind {
	case query.ValueNull:
		return false
	case query.ValueBool:
		return value.Data.(bool)
	default:
		return value.String() != "" && value.String() != "0"
	}
}

func isStar(expr query.Expr) bool {
	column, ok := expr.(*query.ColumnExpr)
	return ok && column.Name == "*"
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
