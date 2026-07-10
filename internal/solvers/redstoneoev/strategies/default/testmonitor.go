package defaultstrategy

import (
	"context"
	"math/big"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

const (
	envTestMarkets   = "OEV_TEST_MARKETS"
	envTestPositions = "OEV_TEST_POSITIONS"
)

// testMonitor is the Sepolia harness source. It enumerates nothing: markets/borrowers are supplied by the
// testbed manifest env, then market state and positions are read from the callback's Morpho contract.
type testMonitor struct {
	reader      Reader
	log         logr.Logger
	callback    common.Address
	loadAdapter func() (types.AdapterSnapshot, bool)
	markets     []common.Hash
	positions   []common.Address
	monitorPoll time.Duration

	snap atomic.Pointer[snapshot]
}

func newTestMonitor(
	r Reader,
	log logr.Logger,
	cfg Config,
	callback common.Address,
	loadAdapter func() (types.AdapterSnapshot, bool),
) (*testMonitor, error) {
	markets, err := parseHashListEnv(envTestMarkets)
	if err != nil {
		return nil, err
	}
	if len(markets) == 0 {
		return nil, errors.Errorf("test monitor: set at least one market id for %s", envTestMarkets)
	}
	positions, err := parseAddressListEnv(envTestPositions)
	if err != nil {
		return nil, err
	}
	if len(positions) == 0 {
		return nil, errors.Errorf("test monitor: set at least one borrower for %s", envTestPositions)
	}
	m := &testMonitor{
		reader:      r,
		log:         log.WithName("testMonitor"),
		callback:    callback,
		loadAdapter: loadAdapter,
		markets:     markets,
		positions:   positions,
		monitorPoll: cfg.MonitorPoll,
	}
	m.snap.Store(&snapshot{
		markets:   map[common.Hash]MarketInfo{},
		prices:    map[common.Hash]*big.Int{},
		quotes:    map[common.Hash]AdapterQuote{},
		positions: map[common.Hash]map[common.Address]morpho.PositionState{},
	})
	return m, nil
}

func (m *testMonitor) run(ctx context.Context) {
	tick := time.NewTicker(m.monitorPoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			m.refresh(ctx)
		}
	}
}

func (m *testMonitor) refresh(ctx context.Context) {
	adapter, ok := m.loadAdapter()
	if !ok {
		m.log.V(1).Info("test monitor adapter snapshot unavailable; keeping cache")
		return
	}
	loan, redeemable, ok := adapterMarketScope(adapter)
	if !ok {
		m.log.V(1).Info("test monitor adapter snapshot incomplete; keeping cache")
		return
	}
	startBlock, startTime, err := m.reader.ReadHead(ctx)
	if err != nil {
		m.log.Error(err, "test monitor header read failed; keeping cache")
		return
	}
	morphoAddr, err := m.reader.ReadCallbackMorpho(ctx, m.callback)
	if err != nil || morphoAddr == (common.Address{}) {
		m.log.Error(err, "test monitor MORPHO read failed; keeping cache")
		return
	}
	params, err := m.reader.ResolveParams(ctx, morphoAddr, m.markets)
	if err != nil {
		m.log.Error(err, "test monitor market params read failed; keeping cache")
		return
	}
	served := verifyAdapterPair(params, loan, redeemable)
	want := make(map[common.Hash]MarketParams, len(served))
	for _, id := range served {
		want[id] = params[id]
	}
	if len(want) == 0 {
		m.log.V(1).Info("test monitor found no adapter-served markets")
		return
	}
	markets, prices, err := m.reader.ReadTestMarketStates(
		ctx, morphoAddr, want, new(big.Int).SetUint64(startBlock),
	)
	if err != nil {
		m.log.Error(err, "test monitor market state read failed; keeping cache")
		return
	}
	positions, err := m.reader.ReadTestPositions(ctx, morphoAddr, markets, m.positions)
	if err != nil {
		m.log.Error(err, "test monitor positions read failed; keeping cache")
		return
	}
	endBlock, _, err := m.reader.ReadHead(ctx)
	if err != nil {
		m.log.Error(err, "test monitor end-header read failed; keeping cache")
		return
	}
	if endBlock != startBlock {
		m.log.V(1).Info("test monitor refresh crossed block boundary; keeping cache",
			"startBlock", startBlock, "endBlock", endBlock)
		return
	}
	m.snap.Store(&snapshot{
		markets: markets, prices: prices, positions: positions,
		block: startBlock, blockTime: startTime, updatedAt: time.Now(),
	})
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

func (m *testMonitor) candidates(auction types.AuctionSnapshot, nowTs uint64, adapter types.AdapterSnapshot) []evalItem {
	return candidatesFromAuctionWithAdapter(m.log, m.snapshot(), auction, nowTs, adapter)
}

func (m *testMonitor) snapshot() *snapshot {
	return m.snap.Load()
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
