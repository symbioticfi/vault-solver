package liquidlane

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/lens"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/vaultv2"
	"github.com/symbioticfi/vault-solver/internal/chain"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

const (
	DefaultMaxTokensPerAdapter = 64
	inventoryReadsPerRoute     = 4
	fillReadsPerRoute          = 4
)

var (
	llAdapter = adapter.NewLiquidLaneAdapter()
	erc4626b  = erc4626.NewIERC4626()
	vaultV2b  = vaultv2.NewIVaultV2()
	lensB     = lens.NewFrontendLiquidityLens()
)

type Reader struct {
	chain liquidLaneBackend
	log   logr.Logger
	dec   decimalsReader

	chainID             int64
	maxTokensPerAdapter int
	// lens is the FrontendLiquidityLens address. When non-zero, swappable headroom is read from the lens's
	// cross-adapter deallocation-cascade estimate instead of the adapter's own getMaxAssets(tokenToRedeem);
	// zero falls back to the adapter getter.
	lens common.Address
}

type gasAdapterState struct {
	owner       common.Address
	marketMaker common.Address
	state       *liquidlanegas.AdapterState
}

type liquidLaneBackend interface {
	ChainID() *big.Int
	Multicall(ctx context.Context, calls []chain.Call) ([]chain.CallResult, error)
}

type decimalsReader interface {
	Get(ctx context.Context, token common.Address) (int, error)
}

func NewReader(c *chain.Client, log logr.Logger, liquidityLens common.Address) *Reader {
	return &Reader{
		chain:               c,
		log:                 log,
		dec:                 chain.NewDecimals(c),
		chainID:             c.ChainID().Int64(),
		maxTokensPerAdapter: DefaultMaxTokensPerAdapter,
		lens:                liquidityLens,
	}
}

// maxAssetsCall builds the getMaxAssets sub-call for a route: via the lens when configured (which models
// the delegator's cross-adapter deallocation cascade the adapter's own getter overstates), else the
// adapter itself. Both return a single uint256, so the result unpacks identically via
// llAdapter.UnpackGetMaxAssets regardless of source.
func (r *Reader) maxAssetsCall(adapterAddr, tokenToRedeem common.Address) chain.Call {
	if r.lens != (common.Address{}) {
		return chain.Call{Target: r.lens, AllowFailure: true, Data: lensB.PackGetMaxAssets0(adapterAddr, tokenToRedeem)}
	}
	return chain.Call{Target: adapterAddr, AllowFailure: true, Data: llAdapter.PackGetMaxAssets(tokenToRedeem)}
}

// TokenDecimals returns the cached ERC-20 decimals used to build typed routes
// when an upstream protocol omits input-token metadata.
func (r *Reader) TokenDecimals(ctx context.Context, token common.Address) (int, error) {
	return r.dec.Get(ctx, token)
}
