package query

import (
	"fmt"
	"strings"
)

// ResolveColumn preserves the distinction between a missing field and explicit NULL.
func ResolveColumn(row Row, name string) (Value, bool) {
	if value, ok := row[name]; ok {
		return value, true
	}
	parts := strings.Split(name, ".")
	value, ok := row[parts[0]]
	if !ok {
		return NewNull(), false
	}
	for _, part := range parts[1:] {
		if value.Kind != ValueObject {
			return NewNull(), false
		}
		value, ok = value.Data.(map[string]Value)[part]
		if !ok {
			return NewNull(), false
		}
	}
	return value, true
}

func (kind ValueKind) String() string {
	names := [...]string{"null", "string", "integer", "boolean", "date", "datetime", "list", "object", "duration", "float"}
	if int(kind) < 0 || int(kind) >= len(names) {
		return "unknown"
	}
	return names[kind]
}

func reportIssue(ctx *EvalContext, message string) error {
	if ctx != nil && ctx.Report != nil {
		return ctx.Report(message)
	}
	return nil
}

func withoutMissingFields(ctx *EvalContext) *EvalContext {
	if ctx == nil {
		return nil
	}
	copy := *ctx
	copy.MissingField = nil
	return &copy
}

func expressionLabel(expr Expr) string {
	if column, ok := expr.(*ColumnExpr); ok {
		return column.Name
	}
	if call, ok := expr.(*FuncCallExpr); ok {
		return call.Name + "(...)"
	}
	return "value"
}

func comparisonProblem(op string, leftExpr, rightExpr Expr, left, right Value) string {
	// NULL is handled by the operator itself. Missing fields are reported at lookup.
	if left.Kind == ValueNull || right.Kind == ValueNull {
		return ""
	}
	invalid := false
	switch op {
	case "=", "==", "!=", "<>", "<", "<=", ">", ">=":
		invalid = !comparableKinds(left.Kind, right.Kind) || left.Kind == ValueList || left.Kind == ValueObject
	case "LIKE", "NOT LIKE", "~", "~*", "!~", "!~*":
		invalid = left.Kind != ValueString || right.Kind != ValueString
	case "IN", "NOT IN":
		invalid = right.Kind != ValueList || left.Kind == ValueList || left.Kind == ValueObject
	case "HAS":
		invalid = left.Kind != ValueList || right.Kind != ValueString
	}
	if !invalid {
		return ""
	}
	message := fmt.Sprintf("incompatible comparison: %s (%s) %s %s (%s)", expressionLabel(leftExpr), left.Kind, op, expressionLabel(rightExpr), right.Kind)
	if left.Kind == ValueList || right.Kind == ValueList {
		message += "; use contains(list, scalar) for membership"
	}
	return message
}
