package rfq

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
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

type fakeQuoteCandidateReader struct {
	out           map[common.Address]*big.Int
	inputDecimals int
	queries       [][]liquidlane.Route
}

func (f *fakeQuoteCandidateReader) readQuoteCandidates(
	_ context.Context,
	inventory []solverInventory,
	tokenIn common.Address,
	tokenOut common.Address,
	amount *big.Int,
) ([]liquidlane.QuoteCandidate, error) {
	matching := make([]liquidlane.Inventory, 0, len(inventory))
	inputDecimals := f.inputDecimals
	if inputDecimals == 0 {
		inputDecimals = 18
	}
	for _, item := range inventory {
		if item.TokenIn == tokenIn && item.TokenOut == tokenOut {
			item.TokenInDecimals = inputDecimals
			matching = append(matching, item)
		}
	}
	matching = liquidgreedy.AllocateInventoryCapacity(matching, nil, 0)
	routes := make([]liquidlane.Route, 0, len(matching))
	for _, item := range matching {
		routes = append(routes, item.Route)
	}
	f.queries = append(f.queries, append([]liquidlane.Route(nil), routes...))
	quotes := make([]liquidlane.FillQuote, 0, len(routes))
	for _, route := range routes {
		amountOut := f.out[route.TokenOut]
		if amountOut == nil {
			continue
		}
		quotes = append(quotes, liquidlane.FillQuote{
			Inventory: liquidlane.Inventory{Route: route, MaxAssets: maxUint256()},
			AmountIn:  liquidlane.CloneBig(amount), MaxAmountOut: liquidlane.CloneBig(amountOut),
		})
	}
	return liquidgreedy.NormalizeOracleInventory(amount, matching, quotes), nil
}

func newDefaultTestStrategy() types.Strategy { return defaultstrategy.New() }

func maxUint256() *big.Int {
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
}

func testInventory(adapter, tokenIn, tokenOut common.Address, maxAssets, maxRate *big.Int) solverInventory {
	route := liquidlane.NewRoute(1, adapter, common.Address{}, tokenIn, tokenOut, 18, 6)
	return liquidlane.DirectInventory(route, maxAssets, maxRate)
}
