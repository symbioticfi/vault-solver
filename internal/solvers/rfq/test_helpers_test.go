package rfq

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"

	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/default"
)

func mustBig(t *testing.T, s string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad big.Int %q", s)
	}
	return n
}

var (
	tIn  = common.HexToAddress("0x0000000000000000000000000000000000000001")
	tOut = common.HexToAddress("0x0000000000000000000000000000000000000002")
	vlt  = common.HexToAddress("0x0000000000000000000000000000000000000003")
)

type fakeStrategyPricing struct {
	decimals int
	out      map[common.Address]*big.Int
	queries  [][]types.QuoteCandidate
}

func (f *fakeStrategyPricing) TokenDecimals(context.Context, common.Address) (int, error) {
	return f.decimals, nil
}

func (f *fakeStrategyPricing) AmountsOut(
	_ context.Context,
	_ common.Address,
	candidates []types.QuoteCandidate,
	_ *big.Int,
) (map[common.Address]*big.Int, error) {
	f.queries = append(f.queries, candidates)
	return f.out, nil
}

func newDefaultTestStrategy(decimals int, out map[common.Address]*big.Int) types.Strategy {
	return defaultstrategy.New(&fakeStrategyPricing{decimals: decimals, out: out})
}

func testInventory(adapter, tokenIn, tokenOut common.Address, maxAssets, maxRate *big.Int) solverInventory {
	route := liquidlane.NewRoute(1, adapter, common.Address{}, tokenIn, tokenOut, 18, 6)
	return liquidlane.DirectInventory(route, maxAssets, maxRate)
}
