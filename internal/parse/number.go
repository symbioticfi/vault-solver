package parse

import (
	"math/big"
	"strings"

	"github.com/go-errors/errors"
)

func Big(value, field string) (*big.Int, error) {
	integer, valid := new(big.Int).SetString(value, 10)
	if !valid {
		return nil, errors.Errorf("%s: invalid integer %q", field, value)
	}
	return integer, nil
}

func EthToWei(value, field string) (*big.Int, error) {
	integer, fraction, _ := strings.Cut(value, ".")
	if integer == "" {
		integer = "0"
	}
	if len(fraction) > 18 {
		return nil, errors.Errorf("%s: more than 18 decimals: %q", field, value)
	}
	fraction += strings.Repeat("0", 18-len(fraction))
	wei, valid := new(big.Int).SetString(integer+fraction, 10)
	if !valid {
		return nil, errors.Errorf("%s: invalid decimal %q", field, value)
	}
	if wei.Sign() < 0 {
		return nil, errors.Errorf("%s: must be >= 0, got %q", field, value)
	}
	return wei, nil
}

func OrDefault[T comparable](value, fallback T) T {
	var zero T
	if value == zero {
		return fallback
	}
	return value
}
