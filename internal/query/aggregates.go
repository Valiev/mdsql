package query

import (
	"fmt"
	"strings"
)

var aggregateFunctions = map[string]string{
	"count": "count(x)",
	"sum":   "sum(x)",
	"min":   "min(x)",
	"max":   "max(x)",
	"avg":   "avg(x)",
}

type AggregateExpr struct {
	Name string
	Arg  Expr
	Star bool
}

func (a *AggregateExpr) eval(_ Row, _ *EvalContext) (Value, error) {
	return NewNull(), fmt.Errorf("aggregate %s must be evaluated over a group", a.Name)
}

func (p *parser) parseAggregate(name token) (Expr, error) {
	p.next() // Opening parenthesis.
	aggregate := &AggregateExpr{Name: strings.ToLower(name.lit)}
	signature := aggregateFunctions[aggregate.Name]
	if aggregate.Name == "count" {
		signature += " or COUNT(*)"
	}
	if p.match(tStar) && aggregate.Name == "count" {
		p.next()
		aggregate.Star = true
		p.countAll = true
	} else {
		if p.match(tRParen) || p.match(tStar) {
			return nil, p.expected("one expression in " + signature)
		}
		arg, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if HasAggregate(arg) {
			return nil, fmt.Errorf("at byte offset %d: nested aggregates are not supported", name.pos)
		}
		aggregate.Arg = arg
	}
	if !p.match(tRParen) {
		return nil, p.expected(") after " + signature)
	}
	p.next()
	return aggregate, nil
}

func HasAggregate(expr Expr) bool {
	switch value := expr.(type) {
	case *AggregateExpr:
		return true
	case *BinaryExpr:
		return HasAggregate(value.Left) || HasAggregate(value.Right)
	case *UnaryExpr:
		return HasAggregate(value.Expr)
	case *BetweenExpr:
		return HasAggregate(value.Left) || HasAggregate(value.Low) || HasAggregate(value.High)
	case *IsNullExpr:
		return HasAggregate(value.Expr)
	case *FuncCallExpr:
		for _, arg := range value.Args {
			if HasAggregate(arg) {
				return true
			}
		}
	}
	return false
}
