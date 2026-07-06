package defaultstrategy

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategytypes"
)

var (
	llAdapter = adapter.NewLiquidLaneAdapter()
)

// ChainReader is the on-chain pricing surface used by the RFQ default strategy.
type ChainReader struct {
	chain *chain.Client
	log   logr.Logger
	dec   *chain.Decimals
}

func NewChainReader(c *chain.Client, log logr.Logger) *ChainReader {
	return &ChainReader{chain: c, log: log, dec: chain.NewDecimals(c)}
}

func (r *ChainReader) TokenDecimals(ctx context.Context, token common.Address) (int, error) {
	return r.dec.Get(ctx, token)
}

func (r *ChainReader) AmountsOut(
	ctx context.Context,
	tokenIn common.Address,
	candidates []strategytypes.QuoteCandidate,
	amount *big.Int,
) (map[common.Address]*big.Int, error) {
	type group struct {
		asset   common.Address
		adapter common.Address
	}
	var groups []group
	seen := make(map[common.Address]bool, len(candidates))
	for _, c := range candidates {
		if seen[c.Asset] {
			continue
		}
		seen[c.Asset] = true
		groups = append(groups, group{asset: c.Asset, adapter: c.Adapter})
	}
	if len(groups) == 0 {
		return map[common.Address]*big.Int{}, nil
	}

	calls := make([]chain.Call, len(groups))
	for i, g := range groups {
		calls[i] = chain.Call{
			Target:       g.adapter,
			AllowFailure: true,
			Data:         llAdapter.PackGetAmountOut(tokenIn, amount),
		}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("amountsOut: got %d results for %d calls", len(res), len(calls))
	}
	out := make(map[common.Address]*big.Int, len(groups))
	for i, rr := range res {
		if !rr.Success {
			r.log.V(1).Info("getAmountOut reverted; asset left unpriced", "asset", groups[i].asset.Hex())
			continue
		}
		amt, derr := llAdapter.UnpackGetAmountOut(rr.ReturnData)
		if derr != nil {
			r.log.V(1).Error(derr, "getAmountOut decode failed; asset left unpriced", "asset", groups[i].asset.Hex())
			continue
		}
		out[groups[i].asset] = amt
	}
	return out, nil
}
