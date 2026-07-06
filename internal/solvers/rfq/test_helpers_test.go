package rfq

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategytypes"
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
	queries  [][]strategytypes.QuoteCandidate
}

func (f *fakeStrategyPricing) TokenDecimals(context.Context, common.Address) (int, error) {
	return f.decimals, nil
}

func (f *fakeStrategyPricing) AmountsOut(
	_ context.Context,
	_ common.Address,
	candidates []strategytypes.QuoteCandidate,
	_ *big.Int,
) (map[common.Address]*big.Int, error) {
	f.queries = append(f.queries, candidates)
	return f.out, nil
}

func newDefaultTestStrategy(decimals int, out map[common.Address]*big.Int) strategytypes.Strategy {
	return defaultstrategy.New(&fakeStrategyPricing{decimals: decimals, out: out})
}
