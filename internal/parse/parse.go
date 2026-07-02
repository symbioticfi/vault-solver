// Package parse holds the pure parse/coerce primitives shared by the solvers' config parsing.
// It is protocol-agnostic framework code: it must not import any solver or protocol package.
package parse

import (
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
)

func Address(s, field string) (common.Address, error) {
	if !common.IsHexAddress(s) {
		return common.Address{}, errors.Errorf("%s: invalid address %q", field, s)
	}
	return common.HexToAddress(s), nil
}

func NonZeroAddress(s, field string) (common.Address, error) {
	addr, err := Address(s, field)
	if err != nil {
		return common.Address{}, err
	}
	if addr == (common.Address{}) {
		return common.Address{}, errors.Errorf("%s: zero address (placeholder not replaced?)", field)
	}
	return addr, nil
}

func Hash(s, field string) (common.Hash, error) {
	// Decode (not just length-check) so a non-hex body fails closed instead of HexToHash silently
	// zero-filling a typo'd id into the zero hash.
	b, err := hexutil.Decode(s)
	if err != nil || len(b) != 32 {
		return common.Hash{}, errors.Errorf("%s: invalid 32-byte hex %q", field, s)
	}
	return common.BytesToHash(b), nil
}

func Big(s, field string) (*big.Int, error) {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, errors.Errorf("%s: invalid integer %q", field, s)
	}
	return n, nil
}

// EthToWei converts a decimal ether string (e.g. "0.0005") to wei exactly (no float rounding):
// split on the point, right-pad the fraction to 18 digits, and combine.
func EthToWei(s, field string) (*big.Int, error) {
	intPart, fracPart, hasDot := strings.Cut(s, ".")
	if intPart == "" {
		intPart = "0"
	}
	if len(fracPart) > 18 {
		return nil, errors.Errorf("%s: more than 18 decimals: %q", field, s)
	}
	for len(fracPart) < 18 {
		fracPart += "0"
	}
	combined := intPart + fracPart
	if hasDot && fracPart == "" {
		combined = intPart // "5." form
	}
	wei, ok := new(big.Int).SetString(combined, 10)
	if !ok {
		return nil, errors.Errorf("%s: invalid decimal %q", field, s)
	}
	if wei.Sign() < 0 { // an ETH amount is never negative; a "-…" would silently disable a floor/trigger
		return nil, errors.Errorf("%s: must be >= 0, got %q", field, s)
	}
	return wei, nil
}

// OrDefault returns v unless it is the zero value, in which case it returns fallback.
func OrDefault[T comparable](v, fallback T) T {
	var zero T
	if v == zero {
		return fallback
	}
	return v
}

// MsDuration converts a millisecond config field to a Duration: a nil pointer (field omitted) yields
// fallback, while a present value must be strictly positive — a set-but-non-positive interval is a
// misconfiguration and is rejected here rather than silently defaulted (mirrors the fail-closed
// duration handling in Duration).
func MsDuration(ms *int, fallback time.Duration, field string) (time.Duration, error) {
	if ms == nil {
		return fallback, nil
	}
	if *ms <= 0 {
		return 0, errors.Errorf("%s: must be a positive duration in ms, got %d", field, *ms)
	}
	return time.Duration(*ms) * time.Millisecond, nil
}

// Duration returns fallback when s is empty, but a present-but-invalid or non-positive value is
// an error rather than a silent fall back to the default — a typo'd interval should fail, not run at
// some surprising cadence.
func Duration(s string, fallback time.Duration, field string) (time.Duration, error) {
	if s == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, errors.Errorf("%s: invalid duration %q: %w", field, s, err)
	}
	if d <= 0 {
		return 0, errors.Errorf("%s: duration must be positive, got %q", field, s)
	}
	return d, nil
}
