package engine

import (
	"fmt"
	"math"
	"strings"

	"github.com/Valiev/mdsql/internal/query"
)

func resolveOrderBy(items []query.OrderItem, selected []query.SelectItem, aliases map[string]query.Expr) ([]query.OrderItem, error) {
	result := make([]query.OrderItem, len(items))
	for i, item := range items {
		position, ordinal, err := orderPosition(item.Expr)
		if err != nil {
			return nil, err
		}
		expr := item.Expr
		if ordinal {
			if position <= 0 || position > int64(len(selected)) {
				return nil, fmt.Errorf("ORDER BY position %d is out of range; expected 1..%d", position, len(selected))
			}
			expr = selected[position-1].Expr
		} else {
			expr = resolveAliases(expr, aliases)
		}
		result[i] = query.OrderItem{Expr: expr, Desc: item.Desc}
	}
	return result, nil
}

func orderPosition(expr query.Expr) (int64, bool, error) {
	switch value := expr.(type) {
	case *query.LiteralExpr:
		if value.Value.Kind == query.ValueInt {
			return value.Value.Data.(int64), true, nil
		}
	case *query.UnaryExpr:
		if value.Op == "PLUS" || value.Op == "MINUS" {
			position, ordinal, err := orderPosition(value.Expr)
			if err != nil || !ordinal || value.Op == "PLUS" {
				return position, ordinal, err
			}
			if position == math.MinInt64 {
				return 0, true, fmt.Errorf("ORDER BY position is out of range")
			}
			return -position, true, nil
		}
	}
	return 0, false, nil
}

func whereAliases(aliases map[string]query.Expr, sourceColumns map[string]struct{}) map[string]query.Expr {
	visible := make(map[string]query.Expr, len(aliases))
	for name, expr := range aliases {
		base := strings.SplitN(name, ".", 2)[0]
		if _, sourceColumn := sourceColumns[base]; sourceColumn {
			continue
		}
		if _, sourceColumn := sourceColumns[name]; !sourceColumn {
			visible[name] = expr
		}
	}
	return visible
}
