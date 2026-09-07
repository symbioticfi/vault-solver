package defaultstrategy

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	morphobinding "github.com/symbioticfi/vault-solver/api/bindings/oev/morpho"
)

// Morpho hashes the five static market-parameter words. Take their layout from
// the vendored binding rather than maintaining a second tuple declaration.
var marketParamsArgs = func() abi.Arguments {
	contract, err := morphobinding.MorphoMetaData.ParseABI()
	if err != nil {
		panic("morpho market params ABI: " + err.Error())
	}
	method, exists := contract.Methods["idToMarketParams"]
	if !exists || len(method.Outputs) != 5 {
		panic("morpho market params ABI has changed")
	}
	return method.Outputs
}()

func deriveMarketID(params MarketParams) (common.Hash, error) {
	data, err := marketParamsArgs.Pack(params.LoanToken, params.CollateralToken, params.Oracle, params.Irm, params.Lltv)
	if err != nil {
		return common.Hash{}, errors.Errorf("encode market params: %w", err)
	}
	return crypto.Keccak256Hash(data), nil
}
