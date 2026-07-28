package defaultstrategy

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
)

func testAdapter(id byte, fundable int64) types.AdapterSnapshot {
	addr := common.Address{id}
	return types.AdapterSnapshot{
		ID:            addr.Hex(),
		Adapter:       addr,
		Vault:         common.Address{0x99, id},
		Collateral:    common.Address{0xaa},
		Fundable:      big.NewInt(fundable),
		MaxAssets:     big.NewInt(fundable),
		MinAssets:     new(big.Int),
		MaxConcurrent: 50,
	}
}

func testAuction(id int64, remaining int64) types.AuctionSnapshot {
	return types.AuctionSnapshot{
		ID:              big.NewInt(id).String(),
		AuctionID:       id,
		OriginalIndex:   int(id),
		Request:         common.Address{0xbb, byte(id)},
		Status:          "open",
		DepositAsset:    common.Address{0xaa},
		AmountRequested: big.NewInt(remaining),
		RemainingAmount: big.NewInt(remaining),
		MaxRateBps:      200,
	}
}

func TestStrategyLargestFirstClampsLastOffer(t *testing.T) {
	a1 := testAdapter(1, 50_000_000) // capacity 50M
	a2 := testAdapter(2, 80_000_000) // capacity 80M
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1, a2},
		Auctions: []types.AuctionSnapshot{testAuction(10, 100_000_000)},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 2 {
		t.Fatalf("offers = %d, want 2", len(got.Offers))
	}
	// Largest capacity first (a2: 80M), then a1 clamped to the remaining 20M.
	if got.Offers[0].Maker != a2.Adapter || got.Offers[0].Principal.Int64() != 80_000_000 {
		t.Fatalf("offer0 = %+v, want adapter 2 / 80M", got.Offers[0])
	}
	if got.Offers[1].Maker != a1.Adapter || got.Offers[1].Principal.Int64() != 20_000_000 {
		t.Fatalf("offer1 = %+v, want adapter 1 / 20M", got.Offers[1])
	}
	// 200 bps of each principal, both positive (a 0-return clamped offer would be skipped, not posted).
	if got.Offers[0].ExpectedReturn.Int64() != 1_600_000 || got.Offers[1].ExpectedReturn.Int64() != 400_000 {
		t.Fatalf("expected returns = %s/%s, want 1600000/400000", got.Offers[0].ExpectedReturn, got.Offers[1].ExpectedReturn)
	}
}

func TestStrategyClampsOfferToAdapterCapacity(t *testing.T) {
	a1 := testAdapter(1, 1000)
	a1.MaxAssets = big.NewInt(100) // per-request ceiling below fundable ⇒ capacity 100
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1},
		Auctions: []types.AuctionSnapshot{testAuction(10, 500)},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 1 {
		t.Fatalf("offers = %d, want 1", len(got.Offers))
	}
	if got.Offers[0].Principal.Int64() != 100 {
		t.Fatalf("principal = %s, want adapter capacity 100", got.Offers[0].Principal)
	}
}

func TestStrategyRejectsZeroAdapterCapacity(t *testing.T) {
	a1 := testAdapter(1, 1000)
	a1.MaxAssets = new(big.Int) // maxAssets 0 ⇒ reject-all
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1},
		Auctions: []types.AuctionSnapshot{testAuction(10, 500)},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 0 {
		t.Fatalf("offers = %+v, want none because adapter capacity is zero", got.Offers)
	}
}

func TestStrategyReplaysAdapterCapacityAcrossAuctions(t *testing.T) {
	a1 := testAdapter(1, 100_000_000)
	a1.MaxAssets = big.NewInt(80_000_000)
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1},
		Auctions: []types.AuctionSnapshot{
			testAuction(10, 70_000_000),
			testAuction(11, 70_000_000),
		},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 2 {
		t.Fatalf("offers = %d, want 2", len(got.Offers))
	}
	// Auction 10 takes 70M of the 80M ceiling; auction 11 sees only 100M-70M=30M of budget left.
	if got.Offers[0].Principal.Int64() != 70_000_000 || got.Offers[1].Principal.Int64() != 30_000_000 {
		t.Fatalf("principals = %s/%s, want 70M/30M", got.Offers[0].Principal, got.Offers[1].Principal)
	}
}

func TestStrategySkipsClampedOfferBelowMinAssets(t *testing.T) {
	a1 := testAdapter(1, 50)
	a1.MinAssets = big.NewInt(20)
	a2 := testAdapter(2, 80)
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1, a2},
		Auctions: []types.AuctionSnapshot{testAuction(10, 90)},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	// a2 (80) fills first, leaving 10 for a1 — below a1's min-request size of 20, so a1 is skipped.
	if len(got.Offers) != 1 || got.Offers[0].Maker != a2.Adapter || got.Offers[0].Principal.Int64() != 80 {
		t.Fatalf("offers = %+v, want only adapter 2 / 80", got.Offers)
	}
}

// TestStrategyDropsOfferBelowMinYieldFloor covers the exact ppm floor guard: an offer priced at the
// auction max rate and truncated down must still clear the adapter's minYieldPerRequest, or it is
// dropped (posting it would revert on-chain as FAILED).
func TestStrategyDropsOfferBelowMinYieldFloor(t *testing.T) {
	const principal = 600518648976
	a := testAdapter(1, principal)
	a.MinYieldPpm = big.NewInt(190) // 1.9 bps floor

	// maxRate exactly at the floor (1.9 bps): floor(principal*1.9/1e4) yields 189.9999… ppm < 190.
	auction := testAuction(10, principal)
	auction.MaxRateBps = 1.9
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a},
		Auctions: []types.AuctionSnapshot{auction},
	}
	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 0 {
		t.Fatalf("offers = %+v, want none: truncated expectedReturn is below the 190 ppm floor", got.Offers)
	}

	// A hair more headroom (1.91 bps) leaves room, so the offer is made — and priced AT the floor
	// (ceil(principal*190/1e6) = 114098544), not at the max rate.
	auction.MaxRateBps = 1.91
	input.Auctions = []types.AuctionSnapshot{auction}
	got, err = New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 1 {
		t.Fatalf("offers = %+v, want 1: 1.91 bps leaves room above the 190 ppm floor", got.Offers)
	}
	if want := big.NewInt(114098544); got.Offers[0].ExpectedReturn.Cmp(want) != 0 {
		t.Fatalf("expectedReturn = %s, want %s (priced at the minYieldPerRequest floor)", got.Offers[0].ExpectedReturn, want)
	}
}

// TestStrategySkipsZeroRatePair covers the degenerate case: no adapter floor and a zero auction max rate
// would price the offer at 0 return — the pair must be skipped, not offered at 0 yield.
func TestStrategySkipsZeroRatePair(t *testing.T) {
	a := testAdapter(1, 1000) // MinYieldPpm nil (0)
	auction := testAuction(10, 500)
	auction.MaxRateBps = 0 // both floor and max rate are 0
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a},
		Auctions: []types.AuctionSnapshot{auction},
	}
	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 0 {
		t.Fatalf("offers = %+v, want none: a 0-floor / 0-maxRate pair must not be offered at 0 return", got.Offers)
	}
}

func TestStrategyOwnsEligibility(t *testing.T) {
	a1 := testAdapter(1, 100) // filtered by an existing live offer
	a2 := testAdapter(2, 100)
	a2.Collateral = common.Address{0xbb} // collateral mismatch
	a3 := testAdapter(3, 100)
	a3.MinYieldPpm = big.NewInt(30_000) // 300 bps floor, above the auction's 200-bps (20_000 ppm) max rate
	auction := testAuction(10, 100)
	input := types.OfferInput{
		Now:        time.Unix(0, 0),
		Adapters:   []types.AdapterSnapshot{a1, a2, a3},
		Auctions:   []types.AuctionSnapshot{auction},
		LiveOffers: []types.LiveOffer{{AdapterID: a1.ID, AuctionID: 10}},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 0 {
		t.Fatalf("offers = %+v, want none: live, collateral, and min-yield filters are strategy-owned", got.Offers)
	}
}
