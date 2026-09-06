package engine

import (
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/Valiev/mdsql/internal/query"
)

type aggregatePlan struct {
	items []*query.AggregateExpr
	keys  map[*query.AggregateExpr]string
}

func planAggregates(q *query.Query, order []query.OrderItem, groups []*query.ColumnExpr) (*aggregatePlan, error) {
	hasAggregate := false
	for _, item := range q.Select {
		hasAggregate = hasAggregate || query.HasAggregate(item.Expr)
	}
	for _, item := range order {
		hasAggregate = hasAggregate || query.HasAggregate(item.Expr)
	}
	hasAggregate = hasAggregate || query.HasAggregate(q.Having)
	if !hasAggregate {
		if len(groups) > 0 {
			return nil, fmt.Errorf("GROUP BY requires an aggregate function")
		}
		return nil, nil
	}
	plan := &aggregatePlan{keys: map[*query.AggregateExpr]string{}}
	groupNames := make(map[string]bool, len(groups))
	for _, group := range groups {
		groupNames[group.Name] = true
	}
	for i, item := range q.Select {
		expr, err := plan.rewrite(item.Expr, groupNames)
		if err != nil {
			return nil, err
		}
		q.Select[i].Expr = expr
	}
	for i, item := range order {
		expr, err := plan.rewrite(item.Expr, groupNames)
		if err != nil {
			return nil, err
		}
		order[i].Expr = expr
	}
	var err error
	q.Having, err = plan.rewrite(q.Having, groupNames)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func (p *aggregatePlan) rewrite(expr query.Expr, groups map[string]bool) (query.Expr, error) {
	if expr == nil {
		return nil, nil
	}
	rewrite := func(child query.Expr) (query.Expr, error) { return p.rewrite(child, groups) }
	switch value := expr.(type) {
	case *query.AggregateExpr:
		if query.HasAggregate(value.Arg) {
			return nil, fmt.Errorf("nested aggregates are not supported")
		}
		key, exists := p.keys[value]
		if !exists {
			key = fmt.Sprintf("__aggregate_%d", len(p.items))
			p.items = append(p.items, value)
			p.keys[value] = key
		}
		return &query.ColumnExpr{Name: key}, nil
	case *query.ColumnExpr:
		if !groups[value.Name] {
			return nil, fmt.Errorf("non-grouped column %q in aggregate result", value.Name)
		}
	case *query.BinaryExpr:
		left, err := rewrite(value.Left)
		if err != nil {
			return nil, err
		}
		right, err := rewrite(value.Right)
		return &query.BinaryExpr{Op: value.Op, Left: left, Right: right}, err
	case *query.UnaryExpr:
		child, err := rewrite(value.Expr)
		return &query.UnaryExpr{Op: value.Op, Expr: child}, err
	case *query.IsNullExpr:
		child, err := rewrite(value.Expr)
		return &query.IsNullExpr{Expr: child, Not: value.Not}, err
	case *query.BetweenExpr:
		left, err := rewrite(value.Left)
		if err != nil {
			return nil, err
		}
		low, err := rewrite(value.Low)
		if err != nil {
			return nil, err
		}
		high, err := rewrite(value.High)
		return &query.BetweenExpr{Left: left, Low: low, High: high, Not: value.Not}, err
	case *query.FuncCallExpr:
		args := make([]query.Expr, len(value.Args))
		for i, arg := range value.Args {
			var err error
			args[i], err = rewrite(arg)
			if err != nil {
				return nil, err
			}
		}
		return &query.FuncCallExpr{Name: value.Name, Args: args}, nil
	}
	return expr, nil
}

func (p *aggregatePlan) execute(groupColumns []*query.ColumnExpr, rows []query.Row, ctx *query.EvalContext) ([]query.Row, error) {
	type group struct {
		values []query.Value
		rows   []query.Row
	}
	groups := []group{{rows: rows}}
	if len(groupColumns) > 0 {
		groups = nil
		byKey := map[string]int{}
		for _, row := range rows {
			var key strings.Builder
			values := make([]query.Value, 0, len(groupColumns))
			for _, groupColumn := range groupColumns {
				value, err := query.Eval(groupColumn, row, ctx)
				if err != nil {
					return nil, err
				}
				if value.Kind == query.ValueList || value.Kind == query.ValueObject {
					return nil, fmt.Errorf("GROUP BY column %q requires scalar values, got %s", groupColumn.Name, value.Kind)
				}
				values = append(values, value)
				writeDistinctValue(&key, value)
			}
			index, exists := byKey[key.String()]
			if !exists {
				index = len(groups)
				byKey[key.String()] = index
				groups = append(groups, group{values: values})
			}
			groups[index].rows = append(groups[index].rows, row)
		}
	}
	result := make([]query.Row, 0, len(groups))
	for _, group := range groups {
		row := query.Row{}
		for i, groupColumn := range groupColumns {
			row[groupColumn.Name] = group.values[i]
		}
		for _, aggregate := range p.items {
			value, err := aggregateValue(aggregate, group.rows, ctx)
			if err != nil {
				return nil, err
			}
			row[p.keys[aggregate]] = value
		}
		result = append(result, row)
	}
	if len(groupColumns) > 0 {
		order := make([]query.OrderItem, len(groupColumns))
		for i, groupColumn := range groupColumns {
			order[i].Expr = groupColumn
		}
		if err := query.Order(result, order, ctx); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func aggregateValue(aggregate *query.AggregateExpr, rows []query.Row, ctx *query.EvalContext) (query.Value, error) {
	if aggregate.Star {
		return query.NewInt(int64(len(rows))), nil
	}
	report := func(problem string) (query.Value, error) {
		if ctx.Report != nil {
			return query.NewNull(), ctx.Report(fmt.Sprintf("%s: %s", aggregate.Name, problem))
		}
		return query.NewNull(), nil
	}
	result := query.NewNull()
	count := int64(0)
	var integerSum big.Int
	floatSum := 0.0
	hasFloat := false
	for _, row := range rows {
		value, err := query.Eval(aggregate.Arg, row, ctx)
		if err != nil {
			return query.NewNull(), err
		}
		if value.IsNull() {
			continue
		}
		count++
		if aggregate.Name == "count" {
			continue
		}
		if value.Kind == query.ValueFloat && (math.IsNaN(query.Number(value)) || math.IsInf(query.Number(value), 0)) {
			return report("requires finite numeric values")
		}
		switch aggregate.Name {
		case "sum", "avg":
			if !query.IsNumber(value) {
				return report(fmt.Sprintf("requires numeric values, got %s", value.Kind))
			}
			floatSum += query.Number(value)
			if math.IsInf(floatSum, 0) || math.IsNaN(floatSum) {
				return report("numeric overflow")
			}
			if value.Kind == query.ValueFloat {
				hasFloat = true
			} else {
				integerSum.Add(&integerSum, big.NewInt(value.Data.(int64)))
			}
		case "min", "max":
			if !query.IsNumber(value) && value.Kind != query.ValueString && value.Kind != query.ValueDate && value.Kind != query.ValueDateTime {
				return report(fmt.Sprintf("requires numeric, string, date or datetime values, got %s", value.Kind))
			}
			if result.IsNull() {
				result = value
				continue
			}
			if result.Kind != value.Kind && !(query.IsNumber(result) && query.IsNumber(value)) {
				return report(fmt.Sprintf("incompatible value types %s and %s", result.Kind, value.Kind))
			}
			comparison := query.Compare(value, result)
			if (aggregate.Name == "min" && comparison < 0) || (aggregate.Name == "max" && comparison > 0) {
				result = value
			}
		}
	}
	if aggregate.Name == "count" {
		return query.NewInt(count), nil
	}
	if aggregate.Name == "avg" && count > 0 {
		return query.NewFloat(floatSum / float64(count)), nil
	}
	if aggregate.Name == "sum" && count > 0 {
		if hasFloat {
			return query.NewFloat(floatSum), nil
		}
		if !integerSum.IsInt64() {
			return report("integer overflow")
		}
		return query.NewInt(integerSum.Int64()), nil
	}
	return result, nil
}
