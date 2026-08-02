package chlog

import (
	"math"
	"strconv"
)

// convertValue converts a string value to the target ClickHouse type.
//
// Supported types:
//   - "String" — passthrough
//   - "Int64" — strconv.ParseInt
//   - "Float64" — strconv.ParseFloat
//   - "Bool" — strconv.ParseBool
//
// Unrecognized or "" defaults to String passthrough.
// Conversion errors silently return the original string value.
func convertValue(v string, typ string) any {
	switch typ {
	case "Int64":
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return v
		}
		return n
	case "Float64":
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return v
		}
		// Keep as float64; NaN/Inf are valid in ClickHouse.
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return v
		}
		return f
	case "Bool":
		b, err := strconv.ParseBool(v)
		if err != nil {
			return v
		}
		return b
	default:
		return v
	}
}
