package webhook

import (
	"math/big"

	"github.com/go-errors/errors"
)

func FormatDecimal(value *big.Int) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func FormatDecimalOrZero(value *big.Int) string {
	if value == nil {
		return "0"
	}
	return value.String()
}

func ParseOptionalDecimal(value, field string) (*big.Int, error) {
	if value == "" {
		return nil, nil
	}
	return ParseDecimal(value, field)
}

func ParseOptionalDecimalPointer(value *string, field string) (*big.Int, error) {
	if value == nil {
		return nil, nil
	}
	return ParseDecimal(*value, field)
}

func ParseDecimal(value, field string) (*big.Int, error) {
	number, ok := new(big.Int).SetString(value, 10)
	if !ok || number.Sign() < 0 {
		return nil, errors.Errorf("%s: invalid decimal string %q", field, value)
	}
	return number, nil
}
