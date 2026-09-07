// Package parse holds the pure parse/coerce primitives shared by the solvers' config parsing.
// It is protocol-agnostic framework code: it must not import any solver or protocol package.
package parse

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"
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
	whole, fraction, _ := strings.Cut(s, ".")
	if len(fraction) > 18 {
		return nil, errors.Errorf("%s: more than 18 decimals: %q", field, s)
	}
	digits := whole + fraction
	if digits == "" {
		return nil, errors.Errorf("%s: invalid decimal %q", field, s)
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return nil, errors.Errorf("%s: invalid non-negative decimal %q", field, s)
		}
	}
	n, ok := new(big.Int).SetString(digits+strings.Repeat("0", 18-len(fraction)), 10)
	if !ok {
		return nil, errors.Errorf("%s: invalid decimal %q", field, s)
	}
	return n, nil
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
	if *ms <= 0 || uint64(*ms) > uint64(math.MaxInt64/int64(time.Millisecond)) {
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

// DecodeStrict decodes a deferred YAML node into out, rejecting unknown keys.
func DecodeStrict(node yaml.Node, out any) error {
	if node.Kind == 0 {
		node = yaml.Node{Kind: yaml.MappingNode}
	}
	b, err := yaml.Marshal(&node)
	if err != nil {
		return errors.Errorf("re-encode config: %w", err)
	}
	return YAML(bytes.NewReader(b), out)
}

// YAML rejects unknown fields and trailing documents. A config file describes one process.
func YAML(reader io.Reader, out any) error {
	dec := yaml.NewDecoder(reader)
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return errors.Errorf("decode config: %w", err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return errors.Errorf("decode trailing config: %w", err)
		}
		return errors.New("config must contain exactly one YAML document")
	}
	return nil
}

// JSON decodes one strict document. The caller owns transport and size limits.
func JSON(reader io.Reader, out any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("multiple JSON values")
	}
	return nil
}

// Uint accepts a decimal unsigned integer that fits in bits. No sign, exponent,
// radix prefix or whitespace is accepted at a signed/wire value boundary.
func Uint(raw, field string, bits int) (*big.Int, error) {
	if raw == "" || bits <= 0 {
		return nil, errors.Errorf("%s: invalid uint%d decimal %q", field, bits, raw)
	}
	for _, digit := range raw {
		if digit < '0' || digit > '9' {
			return nil, errors.Errorf("%s: invalid uint%d decimal %q", field, bits, raw)
		}
	}
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok || value.BitLen() > bits {
		return nil, errors.Errorf("%s: invalid uint%d decimal %q", field, bits, raw)
	}
	return value, nil
}

// OptionalAddress accepts omission, but rejects an explicitly configured zero address.
func OptionalAddress(raw, field string) (common.Address, error) {
	if raw == "" {
		return common.Address{}, nil
	}
	return NonZeroAddress(raw, field)
}

// Addresses preserves configured order and rejects aliases of an earlier address.
func Addresses(raw []string, field string) ([]common.Address, error) {
	out := make([]common.Address, 0, len(raw))
	seen := make(map[common.Address]bool, len(raw))
	for i, value := range raw {
		label := field + "[" + strconv.Itoa(i) + "]"
		address, err := NonZeroAddress(value, label)
		if err != nil {
			return nil, err
		}
		if seen[address] {
			return nil, errors.Errorf("%s: duplicate address %s", label, address.Hex())
		}
		seen[address] = true
		out = append(out, address)
	}
	return out, nil
}

// Bool accepts the deliberately narrow spellings used by explicit harness flags.
// Callers read the environment at the point of use; this function has no process state.
func Bool(raw, field string) (bool, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "true" || value == "1" {
		return true, nil
	}
	if value == "" || value == "false" || value == "0" {
		return false, nil
	}
	return false, errors.Errorf("%s: invalid bool %q (want true/1 or false/0)", field, raw)
}
