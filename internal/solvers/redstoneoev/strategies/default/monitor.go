package defaultstrategy

import (
	"context"
	"math/big"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/observability"
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

// apiMonitor owns the API-backed Morpho snapshot. Its run loop is the only writer.
type apiMonitor struct {
	log logr.Logger

	maxPositions int
	loadAdapter  func() (types.AdapterSnapshot, bool)

	maxHF   float64
	chainID int64

	api         *morphoClient
	monitorPoll time.Duration
	tracer      *observability.Tracer

	snap atomic.Pointer[snapshot]
}

func newAPIMonitor(
	log logr.Logger,
	cfg Config,
	chainID int64,
	loadAdapter func() (types.AdapterSnapshot, bool),
	tracer *observability.Tracer,
) *apiMonitor {
	m := &apiMonitor{
		log:          log.WithName("monitor"),
		maxPositions: cfg.MaxTrackedPositions,
		loadAdapter:  loadAdapter,
		maxHF:        cfg.DiscoveryMaxHealthFactor,
		chainID:      chainID,
		api:          newMorphoClient(cfg.MorphoAPIURL),
		monitorPoll:  cfg.MonitorPoll,
		tracer:       tracer,
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

// candidates evaluates our tracked at-risk set at the auction price. RedStone's pushed positions are ignored.
func (m *apiMonitor) candidates(
	auction types.AuctionSnapshot, nowTs uint64, adapter types.AdapterSnapshot, tally *rejectTally,
) []evalItem {
	return candidatesFromAuctionWithAdapter(m.log, m.snapshot(), auction, nowTs, adapter, tally)
}

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

// refresh is one monitor tick: it roots its own trace, so the Morpho GraphQL calls it makes nest
// under it (spec §9.4).
func (m *apiMonitor) refresh(ctx context.Context) {
	ctx, end := m.tracer.Start(ctx, "oev.monitor")
	var err error
	defer func() { end(err) }()
	// The monitor logger is narrower than the strategy's; carry it for every line below.
	ctx = observability.WithLogger(ctx, m.log)

	adapter, ok := m.loadAdapter()
	if !ok {
		observability.Decline(ctx, "skipped", "adapter_snapshot_unavailable")
		observability.Log(ctx).V(1).Info("API refresh skipped: solver has not read the adapter snapshot yet")
		return
	}
	loan, redeemable, ok := adapterMarketScope(adapter)
	if !ok {
		observability.Decline(ctx, "skipped", "adapter_snapshot_incomplete")
		observability.Log(ctx).V(1).Info("API refresh skipped: adapter snapshot needs a loan token and a non-zero redeemable asset",
			"loan", adapter.Loan.Hex(), "redeemable", len(adapter.Redeemable))
		return
	}

	apiMarkets, err := m.api.DiscoverMarketData(ctx, m.chainID, []common.Address{loan}, redeemable)
	if err != nil {
		observability.Log(ctx).Error(err, "morpho API market refresh failed; keeping cache")
		return
	}
	apiSnap := m.apiMarketSnapshot(apiMarkets, loan, redeemable)
	if len(apiSnap.markets) == 0 {
		observability.Decline(ctx, "skipped", "no_usable_markets")
		observability.Log(ctx).V(1).Info("morpho API market refresh returned no usable adapter markets",
			"discovered", len(apiMarkets), "loan", loan.Hex(), "redeemable", len(redeemable))
		return
	}

	ids := make([]common.Hash, 0, len(apiSnap.markets))
	for id := range apiSnap.markets {
		ids = append(ids, id)
	}
	apiPositions, err := m.api.PositionsByMarket(ctx, m.chainID, ids, m.maxPositions, &m.maxHF)
	if err != nil {
		observability.Log(ctx).Error(err, "morpho API position refresh failed; keeping cache")
		return
	}
	positions := apiPositionsSnapshot(observability.Log(ctx), apiPositions, apiSnap.markets)

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
		view, err := marketInfoFromAPI(apiMarket)
		if err != nil {
			m.log.V(1).Info("dropping malformed Morpho API market", "market", apiMarket.MarketID.Hex(), "reason", err.Error())
			continue
		}
		if view.info.Params.LoanToken != loan || !redeem[view.info.Params.CollateralToken] {
			continue // outside the adapter's loan/collateral scope
		}
		derived, err := deriveMarketID(view.info.Params)
		if err != nil {
			m.log.V(1).Info("morpho API market params do not hash; dropping", "market", view.id.Hex(), "reason", err.Error())
			continue
		}
		if derived != view.id {
			m.log.V(1).Info("morpho API market id mismatch; dropping", "market", view.id.Hex(), "derived", derived.Hex())
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

func marketInfoFromAPI(m morphoMarket) (apiMarketView, error) {
	switch {
	case m.MarketID == (common.Hash{}):
		return apiMarketView{}, errors.New("market id is empty")
	case m.CollateralAsset == nil:
		return apiMarketView{}, errors.New("collateral asset is missing")
	case m.State == nil:
		return apiMarketView{}, errors.New("market state is missing")
	}
	var bad []string
	num := func(field, v string) *big.Int {
		n, ok := parseAPIBig(v)
		if !ok {
			bad = append(bad, field+"="+strconv.Quote(v))
		}
		return n
	}
	lltv := num("lltv", m.LLTV)
	supplyAssets := num("supplyAssets", m.State.SupplyAssets)
	supplyShares := num("supplyShares", m.State.SupplyShares)
	borrowAssets := num("borrowAssets", m.State.BorrowAssets)
	borrowShares := num("borrowShares", m.State.BorrowShares)
	lastUpdate, ok := parseAPIUint64(m.State.Timestamp)
	if !ok {
		bad = append(bad, "timestamp="+strconv.Quote(m.State.Timestamp))
	}
	block, ok := parseAPIUint64(m.State.BlockNumber)
	if !ok || block == 0 {
		bad = append(bad, "blockNumber="+strconv.Quote(m.State.BlockNumber))
	}
	var price *big.Int
	if m.State.Price != "" {
		price = num("price", m.State.Price)
	}
	if len(bad) > 0 {
		return apiMarketView{}, errors.Errorf("fields are not non-negative integers: %s", strings.Join(bad, ", "))
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
		return apiMarketView{}, errors.Errorf("zero address in params: loan %s, collateral %s, oracle %s",
			params.LoanToken.Hex(), params.CollateralToken.Hex(), params.Oracle.Hex())
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
	}}, nil
}

func apiPositionsSnapshot(
	log logr.Logger, apiPositions []morphoPosition, markets map[common.Hash]MarketInfo,
) map[common.Hash]map[common.Address]morpho.PositionState {
	out := make(map[common.Hash]map[common.Address]morpho.PositionState)
	for _, p := range apiPositions {
		if _, ok := markets[p.MarketID]; !ok {
			continue // position in a market outside the adapter scope
		}
		pos, err := positionStateFromAPI(p)
		if err != nil {
			log.V(1).Info("dropping malformed Morpho API position", "market", p.MarketID.Hex(),
				"borrower", p.Borrower.Hex(), "reason", err.Error())
			continue
		}
		if out[p.MarketID] == nil {
			out[p.MarketID] = make(map[common.Address]morpho.PositionState)
		}
		out[p.MarketID][p.Borrower] = pos
	}
	return out
}

func positionStateFromAPI(p morphoPosition) (morpho.PositionState, error) {
	if p.MarketID == (common.Hash{}) || p.Borrower == (common.Address{}) {
		return morpho.PositionState{}, errors.Errorf("market id %s or borrower %s is empty", p.MarketID.Hex(), p.Borrower.Hex())
	}
	borrowShares, ok := parseAPIBig(p.BorrowShares)
	if !ok {
		return morpho.PositionState{}, errors.Errorf("borrowShares %q is not a non-negative integer", p.BorrowShares)
	}
	collateral, ok := parseAPIBig(p.Collateral)
	if !ok {
		return morpho.PositionState{}, errors.Errorf("collateral %q is not a non-negative integer", p.Collateral)
	}
	return morpho.PositionState{BorrowShares: borrowShares, Collateral: collateral}, nil
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

func snapshotFreshForAuction(snap *snapshot, auction types.AuctionSnapshot) bidSkip {
	if !snapshotHasPositions(snap) {
		return bidSkip{}
	}
	if snap.block == 0 || snap.blockTime == 0 {
		return skipf(skipStaleEpoch, "position snapshot has no block (block %d, blockTime %d)", snap.block, snap.blockTime)
	}
	auctionTs := auction.Timestamp / 1000
	if auctionTs <= 0 {
		return bidSkip{}
	}
	if uint64(auctionTs) > snap.blockTime+uint64(snapshotMaxAuctionLag/time.Second) {
		return skipf(skipStaleEpoch, "auction time %d is more than %s after snapshot block time %d",
			auctionTs, snapshotMaxAuctionLag, snap.blockTime)
	}
	return bidSkip{}
}
