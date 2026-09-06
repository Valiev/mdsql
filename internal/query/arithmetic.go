package query

import "math"

func addIntOverflows(left, right int64, add bool) bool {
	if add {
		return (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right)
	}
	return (right > 0 && left < math.MinInt64+right) || (right < 0 && left > math.MaxInt64+right)
}

func multiplyOrNull(left, right Value, op string) (Value, string) {
	if !IsNumber(left) || !IsNumber(right) {
		return NewNull(), ""
	}
	if (op == "/" || op == "%") && Number(right) == 0 {
		if op == "/" {
			return NewNull(), "division by zero"
		}
		return NewNull(), "modulo by zero"
	}

	if left.Kind == ValueInt && right.Kind == ValueInt {
		a := left.Data.(int64)
		b := right.Data.(int64)
		switch op {
		case "*":
			value, overflow := multiplyInt(a, b)
			if overflow {
				return NewNull(), "numeric overflow in *"
			}
			return NewInt(value), ""
		case "%":
			return NewInt(a % b), ""
		}
	}

	var result float64
	switch op {
	case "*":
		result = Number(left) * Number(right)
	case "/":
		result = Number(left) / Number(right)
	case "%":
		result = math.Mod(Number(left), Number(right))
	}
	if math.IsInf(result, 0) || math.IsNaN(result) {
		return NewNull(), "numeric overflow in " + op
	}
	return NewFloat(result), ""
}

func multiplyInt(left, right int64) (int64, bool) {
	if left == 0 || right == 0 {
		return 0, false
	}
	if (left == math.MinInt64 && right == -1) || (right == math.MinInt64 && left == -1) {
		return 0, true
	}
	result := left * right
	return result, result/right != left
}
