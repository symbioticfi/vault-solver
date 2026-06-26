package redstoneoev

import (
	"context"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
)

const snapshotMaxAuctionLag = 3 * 12 * time.Second

// snapshot is immutable once stored and read lock-free by the WS goroutine.
type snapshot struct {
	markets   map[common.Hash]MarketInfo
	prices    map[common.Hash]*big.Int
	quotes    map[common.Hash]AdapterQuote
	positions map[common.Hash]map[common.Address]morpho.PositionState

	block     uint64
	blockTime uint64
}

type monitorSource interface {
	name() string
	run(context.Context)
	refresh(context.Context)
	snapshot() *snapshot
	candidates(auction AuctionMessage, nowTs uint64) []evalItem
}

// apiMonitor owns the API-backed Morpho snapshot. The run loop is the only snapshot writer.
type apiMonitor struct {
	reader *reader
	log    logr.Logger

	maxPositions int
	adapter      common.Address

	maxHF    float64
	callback common.Address
	chainID  int64

	api         *morphoClient
	monitorPoll time.Duration

	snap atomic.Pointer[snapshot]
}

func newAPIMonitor(r *reader, log logr.Logger, cfg *Config, chainID int64) *apiMonitor {
	m := &apiMonitor{
		reader:       r,
		log:          log.WithName("monitor"),
		maxPositions: cfg.MaxTrackedPositions,
		adapter:      cfg.Adapter,
		maxHF:        cfg.DiscoveryMaxHealthFactor,
		callback:     cfg.Callback,
		chainID:      chainID,
		monitorPoll:  cfg.MonitorPoll,
		api:          newMorphoClient(cfg.MorphoAPIURL),
	}
	m.snap.Store(&snapshot{
		markets:   map[common.Hash]MarketInfo{},
		prices:    map[common.Hash]*big.Int{},
		quotes:    map[common.Hash]AdapterQuote{},
		positions: map[common.Hash]map[common.Address]morpho.PositionState{},
	})
	return m
}

func (m *apiMonitor) snapshot() *snapshot {
	return m.snap.Load()
}

func (m *apiMonitor) name() string { return "api" }

// candidates evaluates our tracked at-risk set at the auction price. RedStone's pushed positions are ignored.
func (m *apiMonitor) candidates(auction AuctionMessage, nowTs uint64) []evalItem {
	return candidatesFromAuction(m.log, m.snapshot(), auction, nowTs)
}

func quoteCollateralsFromSnapshot(snap *snapshot) []common.Address {
	if snap == nil {
		return nil
	}
	seen := make(map[common.Address]bool, len(snap.quotes))
	out := make([]common.Address, 0, len(snap.quotes))
	for id := range snap.quotes {
		info, ok := snap.markets[id]
		if !ok {
			continue
		}
		coll := info.Params.CollateralToken
		if coll == (common.Address{}) || seen[coll] {
			continue
		}
		seen[coll] = true
		out = append(out, coll)
	}
	return out
}

func compactQuotes(in map[common.Hash]*AdapterQuote) map[common.Hash]AdapterQuote {
	out := make(map[common.Hash]AdapterQuote, len(in))
	for id, q := range in {
		if q != nil {
			out[id] = *q
		}
	}
	return out
}

// run drives API snapshot refreshes until ctx is cancelled.
func (m *apiMonitor) run(ctx context.Context) {
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

func (m *apiMonitor) refresh(ctx context.Context) {
	adapter, err := m.reader.readAdapterSnapshot(ctx, m.callback, m.adapter)
	if err != nil {
		m.log.Error(err, "API refresh skipped: adapter state unreadable")
		return
	}

	apiMarkets, err := m.api.DiscoverMarketData(ctx, m.chainID, []common.Address{adapter.loan}, adapter.redeemable)
	if err != nil {
		m.log.Error(err, "morpho API market refresh failed; keeping cache")
		return
	}
	apiSnap := m.apiMarketSnapshot(apiMarkets, adapter.loan, adapter.redeemable, adapter.filler)
	if len(apiSnap.markets) == 0 {
		m.log.V(1).Info("morpho API market refresh returned no usable adapter markets")
		return
	}

	apiQuotes, err := m.reader.ReadAdapterQuotes(ctx, apiSnap.params, m.adapter, apiSnap.serve)
	if err != nil {
		m.log.Error(err, "adapter quote refresh failed; keeping cache")
		return
	}
	quotes := compactQuotes(apiQuotes)

	ids := make([]common.Hash, 0, len(apiSnap.markets))
	for id := range apiSnap.markets {
		ids = append(ids, id)
	}
	apiPositions, err := m.api.PositionsByMarket(ctx, ids, m.maxPositions, &m.maxHF)
	if err != nil {
		m.log.Error(err, "morpho API position refresh failed; keeping cache")
		return
	}
	positions := apiPositionsSnapshot(apiPositions, apiSnap.markets)

	m.snap.Store(&snapshot{
		markets: apiSnap.markets, prices: apiSnap.prices, quotes: quotes, positions: positions,
		block: apiSnap.block, blockTime: apiSnap.blockTime,
	})
}

type apiMarketSnapshot struct {
	markets   map[common.Hash]MarketInfo
	prices    map[common.Hash]*big.Int
	params    map[common.Hash]abiMarketParams
	serve     map[common.Hash]bool
	block     uint64
	blockTime uint64
}

func (m *apiMonitor) apiMarketSnapshot(apiMarkets []morphoMarket, loan common.Address, redeemable []common.Address, filler bool) apiMarketSnapshot {
	redeem := make(map[common.Address]bool, len(redeemable))
	for _, a := range redeemable {
		redeem[a] = true
	}
	out := apiMarketSnapshot{
		markets: make(map[common.Hash]MarketInfo, len(apiMarkets)),
		prices:  make(map[common.Hash]*big.Int, len(apiMarkets)),
		params:  make(map[common.Hash]abiMarketParams, len(apiMarkets)),
		serve:   make(map[common.Hash]bool, len(apiMarkets)),
	}
	views := make([]apiMarketView, 0, len(apiMarkets))
	for _, apiMarket := range apiMarkets {
		view, ok := marketInfoFromAPI(apiMarket)
		if !ok || view.info.Params.LoanToken != loan || !redeem[view.info.Params.CollateralToken] {
			continue
		}
		derived, err := deriveMarketID(view.info.Params)
		if err != nil || derived != view.id {
			m.log.V(1).Info("morpho API market id mismatch; dropping", "market", view.id.Hex())
			continue
		}
		views = append(views, view)
		if view.block > out.block {
			out.block = view.block
			out.blockTime = view.blockTime
		}
	}
	for _, view := range views {
		if view.block != out.block {
			m.log.V(1).Info("morpho API market block mismatch; dropping",
				"market", view.id.Hex(), "wantBlock", out.block, "gotBlock", view.block)
			continue
		}
		out.markets[view.id] = view.info
		out.params[view.id] = view.info.Params
		out.serve[view.id] = filler
		if view.price != nil {
			out.prices[view.id] = view.price
		}
	}
	return out
}

type apiMarketView struct {
	id        common.Hash
	info      MarketInfo
	price     *big.Int
	block     uint64
	blockTime uint64
}

func marketInfoFromAPI(m morphoMarket) (apiMarketView, bool) {
	if m.MarketID == (common.Hash{}) || m.CollateralAsset == nil || m.State == nil {
		return apiMarketView{}, false
	}
	lltv, ok := parseAPIBig(m.LLTV)
	if !ok {
		return apiMarketView{}, false
	}
	supplyAssets, ok := parseAPIBig(m.State.SupplyAssets)
	if !ok {
		return apiMarketView{}, false
	}
	supplyShares, ok := parseAPIBig(m.State.SupplyShares)
	if !ok {
		return apiMarketView{}, false
	}
	borrowAssets, ok := parseAPIBig(m.State.BorrowAssets)
	if !ok {
		return apiMarketView{}, false
	}
	borrowShares, ok := parseAPIBig(m.State.BorrowShares)
	if !ok {
		return apiMarketView{}, false
	}
	lastUpdate, ok := parseAPIUint64(m.State.Timestamp)
	if !ok {
		return apiMarketView{}, false
	}
	block, ok := parseAPIUint64(m.State.BlockNumber)
	if !ok || block == 0 {
		return apiMarketView{}, false
	}
	var price *big.Int
	if m.State.Price != "" {
		if price, ok = parseAPIBig(m.State.Price); !ok {
			return apiMarketView{}, false
		}
	}
	params := abiMarketParams{
		LoanToken:       m.LoanAsset.Address,
		CollateralToken: m.CollateralAsset.Address,
		Oracle:          m.Oracle,
		Irm:             m.IRM,
		Lltv:            lltv,
	}
	if params.LoanToken == (common.Address{}) || params.CollateralToken == (common.Address{}) ||
		params.Oracle == (common.Address{}) {
		return apiMarketView{}, false
	}
	return apiMarketView{id: m.MarketID, price: price, block: block, blockTime: lastUpdate, info: MarketInfo{
		Params: params,
		State: morpho.MarketState{
			TotalSupplyAssets: supplyAssets,
			TotalSupplyShares: supplyShares,
			TotalBorrowAssets: borrowAssets,
			TotalBorrowShares: borrowShares,
			LastUpdate:        lastUpdate,
			Fee:               big.NewInt(0),
			Lltv:              lltv,
			BorrowRatePerSec:  big.NewInt(0),
		},
	}}, true
}

func apiPositionsSnapshot(apiPositions []morphoPosition, markets map[common.Hash]MarketInfo) map[common.Hash]map[common.Address]morpho.PositionState {
	out := make(map[common.Hash]map[common.Address]morpho.PositionState)
	for _, p := range apiPositions {
		if _, ok := markets[p.MarketID]; !ok {
			continue
		}
		pos, ok := positionStateFromAPI(p)
		if !ok {
			continue
		}
		if out[p.MarketID] == nil {
			out[p.MarketID] = make(map[common.Address]morpho.PositionState)
		}
		out[p.MarketID][p.Borrower] = pos
	}
	return out
}

func positionStateFromAPI(p morphoPosition) (morpho.PositionState, bool) {
	if p.MarketID == (common.Hash{}) || p.Borrower == (common.Address{}) {
		return morpho.PositionState{}, false
	}
	borrowShares, ok := parseAPIBig(p.BorrowShares)
	if !ok {
		return morpho.PositionState{}, false
	}
	collateral, ok := parseAPIBig(p.Collateral)
	if !ok {
		return morpho.PositionState{}, false
	}
	return morpho.PositionState{BorrowShares: borrowShares, Collateral: collateral}, true
}

func parseAPIBig(s string) (*big.Int, bool) {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 {
		return nil, false
	}
	return n, true
}

func parseAPIUint64(s string) (uint64, bool) {
	n, ok := parseAPIBig(s)
	if !ok || !n.IsUint64() {
		return 0, false
	}
	return n.Uint64(), true
}

func snapshotHasPositions(snap *snapshot) bool {
	if snap == nil {
		return false
	}
	for _, byBorrower := range snap.positions {
		if len(byBorrower) > 0 {
			return true
		}
	}
	return false
}

func (s *Solver) fresh(a AuctionMessage) (bool, string) {
	skip := snapshotFreshForAuction(s.mon.snapshot(), a)
	return skip == "", skip
}

func snapshotFreshForAuction(snap *snapshot, auction AuctionMessage) string {
	if !snapshotHasPositions(snap) {
		return ""
	}
	if snap.block == 0 || snap.blockTime == 0 {
		return skipStaleEpoch
	}
	auctionTs := auction.Timestamp / 1000
	if auctionTs <= 0 {
		return ""
	}
	if uint64(auctionTs) > snap.blockTime+uint64(snapshotMaxAuctionLag/time.Second) {
		return skipStaleEpoch
	}
	return ""
}
