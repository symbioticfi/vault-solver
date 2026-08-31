package defaultstrategy

import (
	"context"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

const snapshotMaxAuctionLag = 3 * 12 * time.Second

// snapshot is immutable once stored and read lock-free by the WS goroutine.
type snapshot struct {
	markets   map[common.Hash]MarketInfo
	prices    map[common.Hash]*big.Int
	positions map[common.Hash]map[common.Address]morpho.PositionState

	block     uint64
	blockTime uint64
	updatedAt time.Time // wall clock of the last successful refresh store; zero until one succeeds
}

// apiMonitor owns the API-backed Morpho snapshot. Its run loop is the only writer.
type apiMonitor struct {
	log logr.Logger

	maxPositions int
	loadAdapter  func() (types.AdapterSnapshot, bool)

	maxHF   float64
	chainID int64

	api         *morphoClient
	monitorPoll time.Duration

	snap atomic.Pointer[snapshot]
}

func newAPIMonitor(
	log logr.Logger,
	cfg Config,
	chainID int64,
	loadAdapter func() (types.AdapterSnapshot, bool),
) *apiMonitor {
	m := &apiMonitor{
		log:          log.WithName("monitor"),
		maxPositions: cfg.MaxTrackedPositions,
		loadAdapter:  loadAdapter,
		maxHF:        cfg.DiscoveryMaxHealthFactor,
		chainID:      chainID,
		api:          newMorphoClient(cfg.MorphoAPIURL),
		monitorPoll:  cfg.MonitorPoll,
	}
	m.snap.Store(&snapshot{
		markets:   map[common.Hash]MarketInfo{},
		prices:    map[common.Hash]*big.Int{},
		positions: map[common.Hash]map[common.Address]morpho.PositionState{},
	})
	return m
}

func (m *apiMonitor) snapshot() *snapshot {
	return m.snap.Load()
}

// candidates evaluates our tracked at-risk set at the auction price. RedStone's pushed positions are ignored.
func (m *apiMonitor) candidates(auction types.AuctionSnapshot, nowTs uint64, adapter types.AdapterSnapshot) []evalItem {
	return candidatesFromAuctionWithAdapter(m.log, m.snapshot(), auction, nowTs, adapter)
}

func (m *apiMonitor) run(ctx context.Context) {
	runMonitor(ctx, m.monitorPoll, m.refresh)
}

func runMonitor(ctx context.Context, poll time.Duration, refresh func(context.Context)) {
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			refresh(ctx)
		}
	}
}

func (m *apiMonitor) refresh(ctx context.Context) {
	adapter, ok := m.loadAdapter()
	if !ok {
		m.log.V(1).Info("API refresh skipped: adapter snapshot unavailable")
		return
	}
	loan, redeemable, ok := adapterMarketScope(adapter)
	if !ok {
		m.log.V(1).Info("API refresh skipped: adapter snapshot incomplete")
		return
	}

	apiMarkets, err := m.api.DiscoverMarketData(ctx, m.chainID, []common.Address{loan}, redeemable)
	if err != nil {
		m.log.Error(err, "morpho API market refresh failed; keeping cache")
		return
	}
	apiSnap := m.apiMarketSnapshot(apiMarkets, loan, redeemable)
	if len(apiSnap.markets) == 0 {
		m.log.V(1).Info("morpho API market refresh returned no usable adapter markets")
		return
	}

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
		markets: apiSnap.markets, prices: apiSnap.prices, positions: positions,
		block: apiSnap.block, blockTime: apiSnap.blockTime, updatedAt: time.Now(),
	})
}

func adapterMarketScope(adapter types.AdapterSnapshot) (common.Address, []common.Address, bool) {
	if adapter.Loan == (common.Address{}) || len(adapter.Redeemable) == 0 {
		return common.Address{}, nil, false
	}
	redeemable := make([]common.Address, 0, len(adapter.Redeemable))
	for _, asset := range adapter.Redeemable {
		if asset.Asset != (common.Address{}) {
			redeemable = append(redeemable, asset.Asset)
		}
	}
	if len(redeemable) == 0 {
		return common.Address{}, nil, false
	}
	return adapter.Loan, redeemable, true
}

type apiMarketSnapshot struct {
	markets   map[common.Hash]MarketInfo
	prices    map[common.Hash]*big.Int
	block     uint64
	blockTime uint64
}

func (m *apiMonitor) apiMarketSnapshot(apiMarkets []morphoMarket, loan common.Address, redeemable []common.Address) apiMarketSnapshot {
	redeem := make(map[common.Address]bool, len(redeemable))
	for _, a := range redeemable {
		redeem[a] = true
	}
	out := apiMarketSnapshot{
		markets: make(map[common.Hash]MarketInfo, len(apiMarkets)),
		prices:  make(map[common.Hash]*big.Int, len(apiMarkets)),
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
	params := MarketParams{
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

func snapshotFreshForAuction(snap *snapshot, auction types.AuctionSnapshot) string {
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
