package defaultstrategy

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func TestCandidatePriceSource(t *testing.T) {
	id := common.HexToHash("0x01")
	oracle := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	onchain := mustBig("1000000000000000000000000000000000000")
	framePx := new(big.Int).Mul(onchain, big.NewInt(2))

	snap := &snapshot{
		markets: map[common.Hash]MarketInfo{
			id: {Params: abiMarketParams{Oracle: oracle}, State: goldenMarket()},
		},
		prices: map[common.Hash]*big.Int{id: onchain},
		quotes: map[common.Hash]AdapterQuote{
			id: newQuote("1780000000000000000000", mustBig("100000000000")),
		},
		positions: map[common.Hash]map[common.Address]morpho.PositionState{
			id: {common.Address{1}: goldenBorrower()},
		},
	}
	auction := types.AuctionSnapshot{
		Prices: []types.AuctionPrice{{Oracle: oracle, Price: framePx}},
	}

	apiCands := candidatesFromAuction(logr.Discard(), snap, auction, snap.markets[id].State.LastUpdate)
	if len(apiCands) != 1 || apiCands[0].price.Cmp(framePx) != 0 {
		t.Fatalf("auction path price = %+v, want %v", apiCands, framePx)
	}

	testCands := candidatesFromCachedPrices(snap, snap.markets[id].State.LastUpdate)
	if len(testCands) != 1 || testCands[0].price.Cmp(onchain) != 0 {
		t.Fatalf("cached-price path price = %+v, want %v", testCands, onchain)
	}
}

func TestCandidateRequiresAuctionPriceForMarketOracle(t *testing.T) {
	id := common.HexToHash("0x01")
	oracle := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	otherOracle := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	snap := &snapshot{
		markets: map[common.Hash]MarketInfo{
			id: {Params: abiMarketParams{Oracle: oracle}, State: goldenMarket()},
		},
		quotes: map[common.Hash]AdapterQuote{
			id: newQuote("1780000000000000000000", mustBig("100000000000")),
		},
		positions: map[common.Hash]map[common.Address]morpho.PositionState{
			id: {common.Address{1}: goldenBorrower()},
		},
	}
	auction := types.AuctionSnapshot{Prices: []types.AuctionPrice{
		{Oracle: otherOracle, Price: mustBig("1000000000000000000000000000")},
		{Oracle: oracle, Price: big.NewInt(0)},
	}}

	got := candidatesFromAuction(logr.Discard(), snap, auction, snap.markets[id].State.LastUpdate)
	if len(got) != 0 {
		t.Fatalf("market without positive auction price for its oracle must not produce candidates: %+v", got)
	}
}

func TestSnapshotFreshForAuction(t *testing.T) {
	auctionAt := int64(1_000_000)
	auction := types.AuctionSnapshot{Timestamp: auctionAt}
	positioned := func(s snapshot) *snapshot {
		market := common.Hash{1}
		borrower := common.Address{2}
		s.positions = map[common.Hash]map[common.Address]morpho.PositionState{
			market: {
				borrower: {BorrowShares: big.NewInt(1), Collateral: big.NewInt(1)},
			},
		}
		return &s
	}
	tests := []struct {
		name string
		snap *snapshot
		want string
	}{
		{"nil snapshot has no positions", nil, ""},
		{"empty snapshot needs no epoch", &snapshot{}, ""},
		{"positions need block", positioned(snapshot{}), skipStaleEpoch},
		{"positions need block time", positioned(snapshot{block: 1}), skipStaleEpoch},
		{"positions within auction lag are usable", positioned(snapshot{block: 1, blockTime: uint64(auctionAt / 1000)}), ""},
		{"positions older than auction lag are stale", positioned(snapshot{block: 1, blockTime: uint64(auctionAt/1000) - uint64(snapshotMaxAuctionLag/time.Second) - 1}), skipStaleEpoch},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := snapshotFreshForAuction(tc.snap, auction); got != tc.want {
				t.Fatalf("snapshotFreshForAuction = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMarketInfoFromAPI(t *testing.T) {
	loan := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	coll := common.HexToAddress("0x45804880De22913dAFE09f4980848ECE6EcbAf78")
	oracle := common.HexToAddress("0x1234567890123456789012345678901234567890")
	irm := common.HexToAddress("0x2222222222222222222222222222222222222222")
	lltv := mustBig("860000000000000000")
	id, err := deriveMarketID(abiMarketParams{LoanToken: loan, CollateralToken: coll, Oracle: oracle, Irm: irm, Lltv: lltv})
	if err != nil {
		t.Fatalf("deriveMarketID: %v", err)
	}

	view, ok := marketInfoFromAPI(morphoMarket{
		MarketID: id,
		Oracle:   oracle,
		IRM:      irm,
		LLTV:     lltv.String(),
		LoanAsset: morphoAsset{
			Address: loan,
		},
		CollateralAsset: &morphoAsset{
			Address: coll,
		},
		State: &morphoMarketState{
			BlockNumber:  "123",
			BorrowAssets: "1000",
			BorrowShares: "900",
			SupplyAssets: "5000",
			SupplyShares: "4500",
			Timestamp:    "456",
			Price:        "1000000000000000000000000000000000000",
		},
	})
	if !ok {
		t.Fatal("marketInfoFromAPI returned !ok")
	}
	if view.id != id || view.block != 123 || view.blockTime != 456 || view.price.String() != "1000000000000000000000000000000000000" {
		t.Fatalf("bad id/block/blockTime/price: id=%s block=%d blockTime=%d price=%v",
			view.id, view.block, view.blockTime, view.price)
	}
	info := view.info
	if info.Params.LoanToken != loan || info.Params.CollateralToken != coll || info.Params.Oracle != oracle || info.Params.Irm != irm {
		t.Fatalf("bad params: %+v", info.Params)
	}
	if info.State.TotalBorrowAssets.String() != "1000" || info.State.TotalBorrowShares.String() != "900" ||
		info.State.TotalSupplyAssets.String() != "5000" || info.State.TotalSupplyShares.String() != "4500" ||
		info.State.LastUpdate != 456 || info.State.BorrowRatePerSec.Sign() != 0 || info.State.Fee.Sign() != 0 {
		t.Fatalf("bad state: %+v", info.State)
	}
}

func TestAPIMarketSnapshotKeepsLatestBlockOnly(t *testing.T) {
	loan := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	coll := common.HexToAddress("0x45804880De22913dAFE09f4980848ECE6EcbAf78")
	lltv := mustBig("860000000000000000")
	mk := func(oracle common.Address, block, ts string) morphoMarket {
		params := abiMarketParams{LoanToken: loan, CollateralToken: coll, Oracle: oracle, Lltv: lltv}
		id, err := deriveMarketID(params)
		if err != nil {
			t.Fatalf("deriveMarketID: %v", err)
		}
		return morphoMarket{
			MarketID:        id,
			Oracle:          oracle,
			LLTV:            lltv.String(),
			LoanAsset:       morphoAsset{Address: loan},
			CollateralAsset: &morphoAsset{Address: coll},
			State: &morphoMarketState{
				BlockNumber: block, Timestamp: ts,
				BorrowAssets: "1000", BorrowShares: "900", SupplyAssets: "5000", SupplyShares: "4500",
			},
		}
	}
	old := mk(common.HexToAddress("0x1111111111111111111111111111111111111111"), "10", "120")
	latest := mk(common.HexToAddress("0x2222222222222222222222222222222222222222"), "11", "132")

	snap := (&apiMonitor{log: logr.Discard()}).apiMarketSnapshot([]morphoMarket{old, latest}, loan, []common.Address{coll})
	if snap.block != 11 || snap.blockTime != 132 {
		t.Fatalf("snapshot epoch = (%d,%d), want (11,132)", snap.block, snap.blockTime)
	}
	if _, ok := snap.markets[latest.MarketID]; !ok || len(snap.markets) != 1 {
		t.Fatalf("latest-only markets = %+v, want exactly %s", snap.markets, latest.MarketID.Hex())
	}
}

func TestAPIMarketAndPositionFailClosed(t *testing.T) {
	if _, ok := marketInfoFromAPI(morphoMarket{
		MarketID:        common.Hash{1},
		CollateralAsset: &morphoAsset{Address: common.Address{2}},
		State:           &morphoMarketState{BlockNumber: "bad"},
	}); ok {
		t.Fatal("bad market numbers must be rejected")
	}

	if _, ok := positionStateFromAPI(morphoPosition{
		MarketID:     common.Hash{1},
		Borrower:     common.Address{2},
		BorrowShares: "not-a-number",
		Collateral:   "10",
	}); ok {
		t.Fatal("bad position numbers must be rejected")
	}

	pos, ok := positionStateFromAPI(morphoPosition{
		MarketID:     common.Hash{1},
		Borrower:     common.Address{2},
		BorrowShares: "11",
		Collateral:   "22",
	})
	if !ok || pos.BorrowShares.Cmp(big.NewInt(11)) != 0 || pos.Collateral.Cmp(big.NewInt(22)) != 0 {
		t.Fatalf("bad parsed position: %+v ok=%v", pos, ok)
	}
}
