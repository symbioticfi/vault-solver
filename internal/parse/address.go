// Package parse contains protocol-neutral boundary parsers.
package parse

import (
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
)

func Address(value, field string) (common.Address, error) {
	if !common.IsHexAddress(value) {
		return common.Address{}, errors.Errorf("%s: invalid address %q", field, value)
	}
	return common.HexToAddress(value), nil
}

func NonZeroAddress(value, field string) (common.Address, error) {
	address, err := Address(value, field)
	if err != nil {
		return common.Address{}, err
	}
	if address == (common.Address{}) {
		return common.Address{}, errors.Errorf("%s: zero address (placeholder not replaced?)", field)
	}
	return address, nil
}

func OptionalNonZeroAddress(value, field string) (common.Address, error) {
	if value == "" {
		return common.Address{}, nil
	}
	return NonZeroAddress(value, field)
}

func NonZeroAddresses(values []string, field string) ([]common.Address, error) {
	addresses := make([]common.Address, 0, len(values))
	seen := make(map[common.Address]struct{}, len(values))
	for index, value := range values {
		address, err := NonZeroAddress(value, field+"["+strconv.Itoa(index)+"]")
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[address]; duplicate {
			return nil, errors.Errorf("%s[%d]: duplicate address %s", field, index, address.Hex())
		}
		seen[address] = struct{}{}
		addresses = append(addresses, address)
	}
	return addresses, nil
}

func Hash(value, field string) (common.Hash, error) {
	decoded, err := hexutil.Decode(value)
	if err != nil || len(decoded) != common.HashLength {
		return common.Hash{}, errors.Errorf("%s: invalid 32-byte hex %q", field, value)
	}
	return common.BytesToHash(decoded), nil
}
