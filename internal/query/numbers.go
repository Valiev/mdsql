package query

import (
	"math"
	"math/big"
)

func IsNumber(value Value) bool {
	return value.Kind == ValueInt || value.Kind == ValueFloat
}

func Number(value Value) float64 {
	if value.Kind == ValueInt {
		return float64(value.Data.(int64))
	}
	return value.Data.(float64)
}

// Compare compares compatible values, including mixed integers and floats.
func Compare(left, right Value) int { return compareStrict(left, right) }

func comparableKinds(left, right ValueKind) bool {
	return left == right || ((left == ValueInt || left == ValueFloat) && (right == ValueInt || right == ValueFloat))
}

func compareNumbers(left, right Value) int {
	if left.Kind == ValueFloat || right.Kind == ValueFloat {
		a, b := Number(left), Number(right)
		if math.IsNaN(a) || math.IsNaN(b) {
			if math.IsNaN(a) && math.IsNaN(b) {
				return 0
			}
			if math.IsNaN(a) {
				return -1
			}
			return 1
		}
		if math.IsInf(a, 0) || math.IsInf(b, 0) {
			if a < b {
				return -1
			}
			if a > b {
				return 1
			}
			return 0
		}
	}
	if left.Kind == ValueInt && right.Kind == ValueInt {
		a, b := left.Data.(int64), right.Data.(int64)
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}
	if left.Kind == ValueFloat && right.Kind == ValueFloat {
		a, b := Number(left), Number(right)
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}
	// Avoid rounding an int64 to float64 before comparing large integer values.
	asRat := func(value Value) *big.Rat {
		if value.Kind == ValueInt {
			return new(big.Rat).SetInt64(value.Data.(int64))
		}
		return new(big.Rat).SetFloat64(value.Data.(float64))
	}
	return asRat(left).Cmp(asRat(right))
}
