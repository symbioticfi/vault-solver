package defaultstrategy

import (
	"context"
	"maps"
	"math/big"
	"slices"
	"sync/atomic"
	"time"

	"github.com/go-errors/errors"

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
	quotes    map[common.Hash]AdapterQuote
	positions map[common.Hash]map[common.Address]morpho.PositionState

	block     uint64
	blockTime uint64
	updatedAt time.Time // wall clock of the last successful refresh store; zero until one succeeds
}

// marketMonitor is the sole publication boundary for both API and harness
// sources. Failed or empty observations never advance the retained timestamp.
type marketMonitor struct {
	log         logr.Logger
	loadAdapter func() (types.AdapterSnapshot, bool)
	read        func(context.Context, common.Address, []common.Address) (*snapshot, error)
	snap        atomic.Pointer[snapshot]
}

func (m *marketMonitor) snapshot() *snapshot { return m.snap.Load() }

func (m *marketMonitor) refresh(ctx context.Context) {
	adapter, available := m.loadAdapter()
	if !available {
		m.log.V(1).Info("market refresh skipped: adapter snapshot unavailable")
		return
	}
	loan, redeemable, complete := adapterMarketScope(adapter)
	if !complete {
		m.log.V(1).Info("market refresh skipped: adapter snapshot incomplete")
		return
	}
	next, err := m.read(ctx, loan, redeemable)
	if err != nil {
		m.log.Error(err, "market refresh failed; keeping cache")
		return
	}
	if next == nil {
		m.log.V(1).Info("market refresh returned no usable adapter markets")
		return
	}
	next.updatedAt = time.Now()
	m.snap.Store(next)
}

type apiMonitor struct {
	marketMonitor

	maxPositions int
	maxHF        float64
	chainID      int64
	api          *morphoClient
}

func newAPIMonitor(log logr.Logger, cfg Config, chainID int64, loadAdapter func() (types.AdapterSnapshot, bool)) *apiMonitor {
	m := &apiMonitor{
		marketMonitor: marketMonitor{log: log.WithName("monitor"), loadAdapter: loadAdapter},
		maxPositions:  cfg.MaxTrackedPositions, maxHF: cfg.DiscoveryMaxHealthFactor,
		chainID: chainID, api: newMorphoClient(cfg.MorphoAPIURL),
	}
	m.read = m.readAPI
	m.snap.Store(&snapshot{})
	return m
}

func (m *apiMonitor) readAPI(ctx context.Context, loan common.Address, redeemable []common.Address) (*snapshot, error) {
	records, err := m.api.DiscoverMarketData(ctx, m.chainID, []common.Address{loan}, redeemable)
	if err != nil {
		return nil, errors.Errorf("discover Morpho markets: %w", err)
	}
	view := m.apiMarketSnapshot(records, loan, redeemable)
	if len(view.markets) == 0 {
		return nil, nil
	}
	positions, err := m.api.PositionsByMarket(ctx, slices.Collect(maps.Keys(view.markets)), m.maxPositions, &m.maxHF)
	if err != nil {
		return nil, errors.Errorf("read Morpho positions: %w", err)
	}
	view.positions = apiPositionsSnapshot(positions, view.markets)
	return view, nil
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

func (m *apiMonitor) apiMarketSnapshot(records []morphoMarket, loan common.Address, redeemable []common.Address) *snapshot {
	out := &snapshot{markets: make(map[common.Hash]MarketInfo), prices: make(map[common.Hash]*big.Int)}
	// Advancing the chosen block resets both maps together. Older rows can never
	// leak into the final epoch, regardless of upstream result order.
	for _, record := range records {
		view, valid := marketInfoFromAPI(record)
		if !valid || view.info.Params.LoanToken != loan || !slices.Contains(redeemable, view.info.Params.CollateralToken) {
			continue
		}
		id, err := deriveMarketID(view.info.Params)
		if err != nil || id != view.id {
			m.log.V(1).Info("morpho API market id mismatch; dropping", "market", view.id.Hex())
			continue
		}
		if view.block < out.block {
			continue
		}
		if view.block > out.block {
			clear(out.markets)
			clear(out.prices)
			out.block, out.blockTime = view.block, view.blockTime
		}
		out.markets[view.id] = view.info
		delete(out.prices, view.id)
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

func marketInfoFromAPI(record morphoMarket) (apiMarketView, bool) {
	if record.MarketID == (common.Hash{}) || record.CollateralAsset == nil || record.State == nil {
		return apiMarketView{}, false
	}
	view := apiMarketView{id: record.MarketID}
	view.info.Params = MarketParams{LoanToken: record.LoanAsset.Address, CollateralToken: record.CollateralAsset.Address,
		Oracle: record.Oracle, Irm: record.IRM}
	if view.info.Params.LoanToken == (common.Address{}) || view.info.Params.CollateralToken == (common.Address{}) || record.Oracle == (common.Address{}) {
		return apiMarketView{}, false
	}
	state := &view.info.State
	for _, field := range []struct {
		text string
		to   **big.Int
	}{
		{record.LLTV, &view.info.Params.Lltv}, {record.State.SupplyAssets, &state.TotalSupplyAssets},
		{record.State.SupplyShares, &state.TotalSupplyShares}, {record.State.BorrowAssets, &state.TotalBorrowAssets},
		{record.State.BorrowShares, &state.TotalBorrowShares},
	} {
		value, ok := parseAPIBig(field.text)
		if !ok {
			return apiMarketView{}, false
		}
		*field.to = value
	}
	timestamp, validTime := parseAPIUint64(record.State.Timestamp)
	block, validBlock := parseAPIUint64(record.State.BlockNumber)
	if !validTime || !validBlock || block == 0 {
		return apiMarketView{}, false
	}
	view.block, view.blockTime = block, timestamp
	state.LastUpdate, state.Lltv = timestamp, view.info.Params.Lltv
	// The API publishes accrued totals; replay therefore starts with zero interest.
	state.Fee, state.BorrowRatePerSec = new(big.Int), new(big.Int)
	if record.State.Price != "" {
		price, valid := parseAPIBig(record.State.Price)
		if !valid {
			return apiMarketView{}, false
		}
		view.price = price
	}
	return view, true
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
