package liquidlane

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind/v2"

	"github.com/symbioticfi/vault-solver/api/bindings/adapterfactory"
	"github.com/symbioticfi/vault-solver/api/bindings/erc20"
	"github.com/symbioticfi/vault-solver/api/bindings/lens"
	adapterbinding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/vaultv2"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

// RevertDecoder names reverts from the LiquidLane core contracts (adapter, vault, liquidity lens and
// adapter factory), ERC-20 token errors that SafeERC20 bubbles up, plus the calling integration's own
// contracts, e.g. its executor or settler.
func RevertDecoder(integration ...*bind.MetaData) *chain.RevertDecoder {
	return chain.RevertDecoderFor(append([]*bind.MetaData{
		&adapterbinding.LiquidLaneAdapterMetaData, &vaultv2.IVaultV2MetaData,
		&lens.FrontendLiquidityLensMetaData, &adapterfactory.IAdapterFactoryMetaData, &erc20.ERC20MetaData,
	}, integration...)...)
}
