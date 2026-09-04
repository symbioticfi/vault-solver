package policy

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
)

var marketParamsArgs = abi.Arguments{{Type: mustTupleType([]abi.ArgumentMarshaling{
	{Name: "loanToken", Type: "address"},
	{Name: "collateralToken", Type: "address"},
	{Name: "oracle", Type: "address"},
	{Name: "irm", Type: "address"},
	{Name: "lltv", Type: "uint256"},
})}}

func mustTupleType(components []abi.ArgumentMarshaling) abi.Type {
	t, err := abi.NewType("tuple", "", components)
	if err != nil {
		panic("redstoneoev/defaultstrategy: market params tuple type: " + err.Error())
	}
	return t
}

func deriveMarketID(p MarketParams) (common.Hash, error) {
	encoded, err := encodeMarketParams(p)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

func encodeMarketParams(params MarketParams) ([]byte, error) {
	encoded, err := marketParamsArgs.Pack(params)
	if err != nil {
		return nil, errors.Errorf("encode market params: %w", err)
	}
	return encoded, nil
}
