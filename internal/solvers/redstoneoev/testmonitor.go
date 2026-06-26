package redstoneoev

import (
	"context"
	"math/big"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

const (
	envTestMarkets   = "OEV_TEST_MARKETS"
	envTestPositions = "OEV_TEST_POSITIONS"
)

// testMonitor is the Sepolia harness source. It enumerates nothing: markets/borrowers are supplied by the
// testbed manifest env, then market state and positions are read from the callback's Morpho contract.
type testMonitor struct {
	reader      *reader
	log         logr.Logger
	callback    common.Address
	adapter     common.Address
	markets     []common.Hash
	positions   []common.Address
	monitorPoll time.Duration

	snap atomic.Pointer[snapshot]
}

func newTestMonitor(r *reader, log logr.Logger, cfg *Config) (*testMonitor, error) {
	markets, err := parseHashListEnv(envTestMarkets)
	if err != nil {
		return nil, err
	}
	if len(markets) == 0 {
		return nil, errors.Errorf("%s: set at least one market id for %s", envTestMonitor, envTestMarkets)
	}
	positions, err := parseAddressListEnv(envTestPositions)
	if err != nil {
		return nil, err
	}
	if len(positions) == 0 {
		return nil, errors.Errorf("%s: set at least one borrower for %s", envTestMonitor, envTestPositions)
	}
	m := &testMonitor{
		reader:      r,
		log:         log.WithName("testMonitor"),
		callback:    cfg.Callback,
		adapter:     cfg.Adapter,
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

func (m *testMonitor) name() string { return "test" }

func (m *testMonitor) refresh(ctx context.Context) {
	header, err := m.reader.chain.HeaderByNumber(ctx, nil)
	if err != nil || header == nil || header.Number == nil || !header.Number.IsUint64() {
		m.log.Error(err, "test monitor header read failed; keeping cache")
		return
	}
	morphoAddr, err := m.reader.callAddress(ctx, m.callback, callbackB.PackMORPHO(), callbackB.UnpackMORPHO)
	if err != nil || morphoAddr == (common.Address{}) {
		m.log.Error(err, "test monitor MORPHO read failed; keeping cache")
		return
	}
	adapter, err := m.reader.readAdapterSnapshot(ctx, m.callback, m.adapter)
	if err != nil {
		m.log.Error(err, "test monitor adapter state unreadable; keeping cache")
		return
	}
	params, err := m.reader.ResolveParams(ctx, morphoAddr, m.markets)
	if err != nil {
		m.log.Error(err, "test monitor market params read failed; keeping cache")
		return
	}
	served := verifyAdapterPair(params, adapter.loan, adapter.redeemable)
	want := make(map[common.Hash]abiMarketParams, len(served))
	serve := make(map[common.Hash]bool, len(served))
	for _, id := range served {
		want[id] = params[id]
		serve[id] = adapter.filler
	}
	if len(want) == 0 {
		m.log.V(1).Info("test monitor found no adapter-served markets")
		return
	}
	markets, prices, err := m.readMarkets(ctx, morphoAddr, want)
	if err != nil {
		m.log.Error(err, "test monitor market state read failed; keeping cache")
		return
	}
	quotes, err := m.reader.ReadAdapterQuotes(ctx, want, m.adapter, serve)
	if err != nil {
		m.log.Error(err, "test monitor adapter quote read failed; keeping cache")
		return
	}
	positions, err := m.readPositions(ctx, morphoAddr, markets)
	if err != nil {
		m.log.Error(err, "test monitor positions read failed; keeping cache")
		return
	}
	end, err := m.reader.chain.HeaderByNumber(ctx, nil)
	if err != nil || end == nil || end.Number == nil || !end.Number.IsUint64() {
		m.log.Error(err, "test monitor end-header read failed; keeping cache")
		return
	}
	if end.Number.Uint64() != header.Number.Uint64() {
		m.log.V(1).Info("test monitor refresh crossed block boundary; keeping cache",
			"startBlock", header.Number.Uint64(), "endBlock", end.Number.Uint64())
		return
	}
	m.snap.Store(&snapshot{
		markets: markets, prices: prices, quotes: compactQuotes(quotes), positions: positions,
		block: header.Number.Uint64(), blockTime: header.Time,
	})
}

func (m *testMonitor) readMarkets(ctx context.Context, morphoAddr common.Address, params map[common.Hash]abiMarketParams) (map[common.Hash]MarketInfo, map[common.Hash]*big.Int, error) {
	ids := sortedMarketIDs(params)
	calls := make([]chain.Call, 0, len(ids)*2)
	for _, id := range ids {
		p := params[id]
		calls = append(calls,
			chain.Call{Target: morphoAddr, AllowFailure: true, Data: morphoB.PackMarket(id)},
			chain.Call{Target: p.Oracle, AllowFailure: true, Data: oracleB.PackPrice()},
		)
	}
	res, err := m.reader.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, nil, err
	}
	if len(res) != len(calls) {
		return nil, nil, errors.Errorf("testMonitor markets: got %d results, want %d", len(res), len(calls))
	}
	markets := make(map[common.Hash]MarketInfo, len(ids))
	prices := make(map[common.Hash]*big.Int, len(ids))
	for i, id := range ids {
		marketRes := res[i*2]
		priceRes := res[i*2+1]
		if !marketRes.Success || !priceRes.Success {
			continue
		}
		state, ok := decodeTestMarketState(marketRes.ReturnData, params[id])
		if !ok {
			continue
		}
		price, err := oracleB.UnpackPrice(priceRes.ReturnData)
		if err != nil || price == nil || price.Sign() <= 0 {
			continue
		}
		markets[id] = MarketInfo{Params: params[id], State: state}
		prices[id] = price
	}
	return markets, prices, nil
}

func (m *testMonitor) readPositions(ctx context.Context, morphoAddr common.Address, markets map[common.Hash]MarketInfo) (map[common.Hash]map[common.Address]morpho.PositionState, error) {
	ids := sortedMarketIDsFromInfo(markets)
	calls := make([]chain.Call, 0, len(ids)*len(m.positions))
	type slot struct {
		id       common.Hash
		borrower common.Address
	}
	slots := make([]slot, 0, cap(calls))
	for _, id := range ids {
		for _, borrower := range m.positions {
			slots = append(slots, slot{id: id, borrower: borrower})
			calls = append(calls, chain.Call{Target: morphoAddr, AllowFailure: true, Data: morphoB.PackPosition(id, borrower)})
		}
	}
	res, err := m.reader.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("testMonitor positions: got %d results, want %d", len(res), len(calls))
	}
	out := make(map[common.Hash]map[common.Address]morpho.PositionState, len(ids))
	for i, s := range slots {
		if !res[i].Success {
			continue
		}
		p, err := morphoB.UnpackPosition(res[i].ReturnData)
		if err != nil || p.BorrowShares == nil || p.Collateral == nil {
			continue
		}
		if out[s.id] == nil {
			out[s.id] = make(map[common.Address]morpho.PositionState)
		}
		out[s.id][s.borrower] = morpho.PositionState{BorrowShares: p.BorrowShares, Collateral: p.Collateral}
	}
	return out, nil
}

func decodeTestMarketState(data []byte, params abiMarketParams) (morpho.MarketState, bool) {
	out, err := morphoB.UnpackMarket(data)
	if err != nil || out.TotalSupplyAssets == nil || out.TotalSupplyShares == nil ||
		out.TotalBorrowAssets == nil || out.TotalBorrowShares == nil || out.LastUpdate == nil ||
		out.Fee == nil || params.Lltv == nil || !out.LastUpdate.IsUint64() {
		return morpho.MarketState{}, false
	}
	return morpho.MarketState{
		TotalSupplyAssets: out.TotalSupplyAssets,
		TotalSupplyShares: out.TotalSupplyShares,
		TotalBorrowAssets: out.TotalBorrowAssets,
		TotalBorrowShares: out.TotalBorrowShares,
		LastUpdate:        out.LastUpdate.Uint64(),
		Fee:               out.Fee,
		Lltv:              params.Lltv,
		BorrowRatePerSec:  big.NewInt(0),
	}, true
}

func sortedMarketIDs(params map[common.Hash]abiMarketParams) []common.Hash {
	ids := make([]common.Hash, 0, len(params))
	for id := range params {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, common.Hash.Cmp)
	return ids
}

func sortedMarketIDsFromInfo(markets map[common.Hash]MarketInfo) []common.Hash {
	ids := make([]common.Hash, 0, len(markets))
	for id := range markets {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, common.Hash.Cmp)
	return ids
}

func (m *testMonitor) candidates(auction AuctionMessage, nowTs uint64) []evalItem {
	return candidatesFromCachedPrices(m.snapshot(), nowTs)
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
