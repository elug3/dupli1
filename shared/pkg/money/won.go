package money

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// Won is an amount of Korean won. KRW has no minor unit, so a Won is always
// whole; the type makes a fractional amount unrepresentable instead of
// something each caller has to check for.
//
// It marshals as a plain JSON integer, so a whole amount reads the same as it
// did when it was a float64.
//
// It unmarshals from any JSON number whose value is whole: 1000, 1000.0 and
// 1e3 are the same amount, and some clients (Python's json.dumps, for one)
// write whole floats with a ".0". A fractional or out-of-range number, or a
// value that isn't a number, is refused with a *json.UnmarshalTypeError, which
// the decoder fills in with the field's path. JSON null leaves the value
// unchanged, as encoding/json does for other non-pointer types.
type Won int64

// maxExactFloat is the largest integer a float64 holds exactly (2^53). A
// number written with a fraction or an exponent is accepted only below it, so
// the amount accepted is the amount the client wrote. No KRW amount comes
// near it.
const maxExactFloat = 1 << 53

var wonType = reflect.TypeOf(Won(0))

// UnmarshalJSON implements json.Unmarshaler; see Won.
func (w *Won) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" {
		return nil
	}
	if s == "" || !(s[0] == '-' || (s[0] >= '0' && s[0] <= '9')) {
		return &json.UnmarshalTypeError{Value: jsonKind(s), Type: wonType}
	}
	if !strings.ContainsAny(s, ".eE") {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return &json.UnmarshalTypeError{Value: "number " + s, Type: wonType}
		}
		*w = Won(n)
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f != math.Trunc(f) || math.Abs(f) >= maxExactFloat {
		return &json.UnmarshalTypeError{Value: "number " + s, Type: wonType}
	}
	*w = Won(f)
	return nil
}

// IsWonNumberError reports whether err is a JSON number that Won refused
// because it was fractional or out of range, as opposed to a value that was
// not a number at all. The returned error's Field is the path of the refused
// field, e.g. "price" or "variants.priceOverride".
func IsWonNumberError(err error) (*json.UnmarshalTypeError, bool) {
	var ute *json.UnmarshalTypeError
	if !errors.As(err, &ute) || ute.Type != wonType || !strings.HasPrefix(ute.Value, "number") {
		return nil, false
	}
	return ute, true
}

// jsonKind names a non-number JSON value the way encoding/json does in its
// own UnmarshalTypeError.
func jsonKind(s string) string {
	switch {
	case strings.HasPrefix(s, `"`):
		return "string"
	case s == "true" || s == "false":
		return "bool"
	case strings.HasPrefix(s, "["):
		return "array"
	case strings.HasPrefix(s, "{"):
		return "object"
	default:
		return s
	}
}
