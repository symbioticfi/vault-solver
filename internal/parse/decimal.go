package parse

import (
	"math/big"

	"github.com/go-errors/errors"
)

// DecimalText preserves the caller's wire representation of a missing integer.
func DecimalText(value *big.Int, missing string) string {
	if value != nil {
		return value.String()
	}
	return missing
}

// NonnegativeDecimal accepts decimal strings used by strategy webhooks. Missing
// values may be represented by an empty string only when the wire field allows it.
func NonnegativeDecimal(raw, field string, allowEmpty bool) (*big.Int, error) {
	if allowEmpty && raw == "" {
		return nil, nil
	}
	value, err := Big(raw, field)
	if err != nil || value.Sign() < 0 {
		return nil, errors.Errorf("%s: invalid decimal string %q", field, raw)
	}
	return value, nil
}
