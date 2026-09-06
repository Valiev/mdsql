package query

import (
	"fmt"
	"sort"
	"strings"
)

type functionSpec struct {
	signature string
	minArgs   int
	maxArgs   int // -1 means no upper bound.
	argTypes  []string
}

var functionRegistry = map[string]functionSpec{
	"at":             {"at(n)", 1, 1, []string{"integer"}},
	"after_heading":  {"after_heading(title[, level])", 1, 2, []string{"string", "integer"}},
	"under":          {"under(title)", 1, 1, []string{"string"}},
	"under_like":     {"under_like(pattern)", 1, 1, []string{"string"}},
	"under_contains": {"under_contains(text)", 1, 1, []string{"string"}},
	"has_tag":        {"has_tag(t)", 1, 1, []string{"string"}},
	"lower":          {"lower(s)", 1, 1, []string{"string"}},
	"upper":          {"upper(s)", 1, 1, []string{"string"}},
	"trim":           {"trim(s)", 1, 1, []string{"string"}},
	"len":            {"len(x)", 1, 1, []string{"string or list"}},
	"lines":          {"lines(s[, exact])", 1, 2, []string{"string", "boolean"}},
	"head":           {"head(s, n[, exact])", 2, 3, []string{"string", "integer", "boolean"}},
	"tail":           {"tail(s, n[, exact])", 2, 3, []string{"string", "integer", "boolean"}},
	"contains":       {"contains(string, substring) or contains(list, scalar)", 2, 2, []string{"string or list", "scalar"}},
	"icontains":      {"icontains(s, sub)", 2, 2, []string{"string", "string"}},
	"coalesce":       {"coalesce(a, b, ...)", 2, -1, nil},
	"round":          {"round(x[, digits])", 1, 2, []string{"number", "integer"}},
	"floor":          {"floor(x)", 1, 1, []string{"number"}},
	"ceil":           {"ceil(x)", 1, 1, []string{"number"}},
	"date":           {"date(s)", 1, 1, []string{"string"}},
	"basename":       {"basename(p)", 1, 1, []string{"string"}},
	"dirname":        {"dirname(p)", 1, 1, []string{"string"}},
	"today":          {"today()", 0, 0, nil},
	"now":            {"now()", 0, 0, nil},
}

// FunctionSignatures lists scalar functions, context aliases, and the supported aggregate.
func FunctionSignatures() []string {
	functions := []string{"COUNT(*)"}
	for _, signature := range aggregateFunctions {
		functions = append(functions, signature)
	}
	for _, spec := range functionRegistry {
		functions = append(functions, spec.signature)
	}
	sort.Strings(functions)
	return functions
}

func validateFunction(name string) error {
	if _, ok := functionRegistry[strings.ToLower(name)]; ok {
		return nil
	}
	return fmt.Errorf("unknown function %q; available functions: %s", name, strings.Join(FunctionSignatures(), ", "))
}

func validateFunctionArity(name string, count int) error {
	if err := validateFunction(name); err != nil {
		return err
	}
	spec := functionRegistry[strings.ToLower(name)]
	if count >= spec.minArgs && (spec.maxArgs < 0 || count <= spec.maxArgs) {
		return nil
	}
	expected := fmt.Sprintf("%d", spec.minArgs)
	if spec.maxArgs < 0 {
		expected = "at least " + expected
	} else if spec.maxArgs > spec.minArgs {
		expected = fmt.Sprintf("%d..%d", spec.minArgs, spec.maxArgs)
	}
	return fmt.Errorf("function %q expects %s arguments, got %d; signature: %s", name, expected, count, spec.signature)
}

func functionArgumentProblem(name string, values []Value) string {
	name = strings.ToLower(name)
	spec := functionRegistry[name]
	// contains(list, NULL) tests list membership; other NULL combinations propagate NULL.
	if name == "contains" && (values[0].IsNull() || values[1].IsNull()) {
		return ""
	}
	for i, expected := range spec.argTypes {
		if i >= len(values) {
			break
		}
		value := values[i]
		if value.IsNull() {
			continue
		}
		if name == "contains" && i == 1 && values[0].Kind == ValueString {
			expected = "string"
		}
		valid := value.Kind.String() == expected
		switch expected {
		case "number":
			valid = IsNumber(value)
		case "string or list":
			valid = value.Kind == ValueString || value.Kind == ValueList
		case "scalar":
			valid = value.Kind != ValueList && value.Kind != ValueObject
		}
		if !valid {
			return fmt.Sprintf("function %q argument %d: expected %s, got %s; signature: %s", name, i+1, expected, value.Kind, spec.signature)
		}
	}
	return ""
}
