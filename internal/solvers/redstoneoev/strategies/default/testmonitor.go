package defaultstrategy

import (
	"context"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

const (
	envTestMarkets   = "OEV_TEST_MARKETS"
	envTestPositions = "OEV_TEST_POSITIONS"
)

// testMonitor is the Sepolia harness source. It enumerates nothing: markets/borrowers are supplied by the
// testbed manifest env, then market state and positions are read from the callback's Morpho contract.
type testMonitor struct {
	marketMonitor

	reader    Reader
	callback  common.Address
	markets   []common.Hash
	positions []common.Address
}

func newTestMonitor(reader Reader, log logr.Logger, callback common.Address, loadAdapter func() (types.AdapterSnapshot, bool)) (*testMonitor, error) {
	markets, marketErr := parseHashListEnv(envTestMarkets)
	borrowers, borrowerErr := parseAddressListEnv(envTestPositions)
	if err := errors.Join(marketErr, borrowerErr); err != nil {
		return nil, err
	}
	if len(markets) == 0 {
		return nil, errors.Errorf("test monitor: set at least one market id for %s", envTestMarkets)
	}
	if len(borrowers) == 0 {
		return nil, errors.Errorf("test monitor: set at least one borrower for %s", envTestPositions)
	}
	m := &testMonitor{marketMonitor: marketMonitor{log: log.WithName("testMonitor"), loadAdapter: loadAdapter},
		reader: reader, callback: callback, markets: markets, positions: borrowers}
	m.read = m.readChain
	m.snap.Store(&snapshot{})
	return m, nil
}

// Harness reads have no historical block argument. Bracket the whole observation
// with head reads and publish it only when every read belongs to one block.
func (m *testMonitor) readChain(ctx context.Context, loan common.Address, redeemable []common.Address) (*snapshot, error) {
	block, timestamp, err := m.reader.ReadHead(ctx)
	if err != nil {
		return nil, errors.Errorf("read start header: %w", err)
	}
	address, err := m.reader.ReadCallbackMorpho(ctx, m.callback)
	if err != nil {
		return nil, errors.Errorf("read callback Morpho: %w", err)
	}
	if address == (common.Address{}) {
		return nil, errors.New("callback Morpho address is zero")
	}
	params, err := m.reader.ResolveParams(ctx, address, m.markets)
	if err != nil {
		return nil, errors.Errorf("resolve market parameters: %w", err)
	}
	selected := make(map[common.Hash]MarketParams)
	for _, id := range verifyAdapterPair(params, loan, redeemable) {
		selected[id] = params[id]
	}
	if len(selected) == 0 {
		return nil, nil
	}
	next := &snapshot{block: block, blockTime: timestamp}
	next.markets, next.prices, err = m.reader.ReadTestMarketStates(ctx, address, selected)
	if err != nil {
		return nil, errors.Errorf("read market state: %w", err)
	}
	next.positions, err = m.reader.ReadTestPositions(ctx, address, next.markets, m.positions)
	if err != nil {
		return nil, errors.Errorf("read market positions: %w", err)
	}
	end, _, err := m.reader.ReadHead(ctx)
	if err != nil {
		return nil, errors.Errorf("read end header: %w", err)
	}
	if end != block {
		m.log.V(1).Info("test monitor refresh crossed block boundary; keeping cache", "startBlock", block, "endBlock", end)
		return nil, nil
	}
	return next, nil
}

func verifyAdapterPair(params map[common.Hash]MarketParams, adapterLoan common.Address, redeemable []common.Address) []common.Hash {
	redeem := make(map[common.Address]bool, len(redeemable))
	for _, t := range redeemable {
		redeem[t] = true
	}
	out := make([]common.Hash, 0, len(params))
	for id, p := range params {
		if p.LoanToken == adapterLoan && redeem[p.CollateralToken] {
			out = append(out, id)
		}
	}
	return out
}

func parseHashListEnv(key string) ([]common.Hash, error) {
	parts := splitEnvList(os.Getenv(key))
	out := make([]common.Hash, 0, len(parts))
	for _, p := range parts {
		if !common.IsHexHash(p) {
			return nil, errors.Errorf("%s: invalid hash %q", key, p)
		}
		out = append(out, common.HexToHash(p))
	}
	return out, nil
}

func parseAddressListEnv(key string) ([]common.Address, error) {
	parts := splitEnvList(os.Getenv(key))
	out := make([]common.Address, 0, len(parts))
	for _, p := range parts {
		if !common.IsHexAddress(p) {
			return nil, errors.Errorf("%s: invalid address %q", key, p)
		}
		out = append(out, common.HexToAddress(p))
	}
	return out, nil
}

func splitEnvList(v string) []string {
	fields := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
