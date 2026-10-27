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

func testAuction(id int64, requested int64) types.AuctionSnapshot {
	return types.AuctionSnapshot{
		ID:              big.NewInt(id).String(),
		AuctionID:       id,
		OriginalIndex:   int(id),
		Request:         common.Address{0xbb, byte(id)},
		Status:          "open",
		DepositAsset:    common.Address{0xaa},
		AmountRequested: big.NewInt(requested),
		RemainingAmount: big.NewInt(requested),
		MaxRateBps:      200,
	}
}

func TestStrategyEveryAdapterOffersFullRequest(t *testing.T) {
	a1 := testAdapter(1, 50_000_000)  // capacity 50M, below the request
	a2 := testAdapter(2, 180_000_000) // capacity 180M, above the request
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
	// Each adapter bids the full request within its own capacity; a2's bid does not shrink a1's.
	if got.Offers[0].Maker != a2.Adapter || got.Offers[0].Principal.Int64() != 100_000_000 {
		t.Fatalf("offer0 = %+v, want adapter 2 / 100M", got.Offers[0])
	}
	if got.Offers[1].Maker != a1.Adapter || got.Offers[1].Principal.Int64() != 50_000_000 {
		t.Fatalf("offer1 = %+v, want adapter 1 / 50M", got.Offers[1])
	}
	if got.Offers[0].ExpectedReturn.Int64() != 2_000_000 || got.Offers[1].ExpectedReturn.Int64() != 1_000_000 {
		t.Fatalf("expected returns = %s/%s, want 2000000/1000000", got.Offers[0].ExpectedReturn, got.Offers[1].ExpectedReturn)
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
	a1 := testAdapter(1, 5_000)
	a1.MinAssets = big.NewInt(2_000)
	a2 := testAdapter(2, 8_000)
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1, a2},
		Auctions: []types.AuctionSnapshot{testAuction(10, 1_000)},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	// The 1,000-unit request is below a1's min-request size of 2,000, so a1 is skipped; a2 offers it in full.
	if len(got.Offers) != 1 || got.Offers[0].Maker != a2.Adapter || got.Offers[0].Principal.Int64() != 1_000 {
		t.Fatalf("offers = %+v, want only adapter 2 / 1000", got.Offers)
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

	// A hair more headroom (1.91 bps) leaves room, so the offer is made. The floor plus the full
	// partial-consumption margin (ceil(principal*190/1e6) + ceil(principal/1e6) = 114699063) would break
	// the 1.91 bps cap, so the price clamps to the cap (floor(principal*191/1e6) = 114699061), keeping
	// the margin that fits above the floor.
	auction.MaxRateBps = 1.91
	input.Auctions = []types.AuctionSnapshot{auction}
	got, err = New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 1 {
		t.Fatalf("offers = %+v, want 1: 1.91 bps leaves room above the 190 ppm floor", got.Offers)
	}
	if want := big.NewInt(114699061); got.Offers[0].ExpectedReturn.Cmp(want) != 0 {
		t.Fatalf("expectedReturn = %s, want %s (margined, clamped to the auction max rate)", got.Offers[0].ExpectedReturn, want)
	}
}

// TestStrategyPricesPartialConsumeMarginAboveFloor covers the normal case: when the auction max rate
// leaves room, the offer is priced one ppm of principal above the floor so a partial consume() cannot
// pro-rate below it (mainnet tx 0xc637…8386 failed TooLowYield on a floor-exact offer).
func TestStrategyPricesPartialConsumeMarginAboveFloor(t *testing.T) {
	const principal = 30_000_035_000
	a := testAdapter(1, principal)
	a.MinYieldPpm = big.NewInt(190)
	auction := testAuction(10, principal)
	auction.MaxRateBps = 200 // plenty of headroom above the 1.9 bps floor
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a},
		Auctions: []types.AuctionSnapshot{auction},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 1 {
		t.Fatalf("offers = %+v, want 1", got.Offers)
	}
	// ceil(principal*190/1e6) + ceil(principal/1e6) = 5700007 + 30001.
	if want := big.NewInt(5_730_008); got.Offers[0].ExpectedReturn.Cmp(want) != 0 {
		t.Fatalf("expectedReturn = %s, want %s (floor plus the partial-consumption margin)", got.Offers[0].ExpectedReturn, want)
	}
}

// TestStrategyCapsReturnByRateBasis covers APR auctions: maxRate is annual and the offer's return covers
// rateBasisSeconds of it, so a 1-day basis at 7.3% caps at the old 2 bps; the floor stays per request.
func TestStrategyCapsReturnByRateBasis(t *testing.T) {
	const principal = 1_000_000_000_000
	cases := []struct {
		name     string
		floorPpm int64
		want     int64
	}{
		{"no floor prices at the cap", 0, 200_000_000},
		{"floor plus margin under the cap", 190, 190_000_000 + 1_000_000},
		{"floor above the cap is skipped", 300, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := testAdapter(1, principal)
			if c.floorPpm > 0 {
				a.MinYieldPpm = big.NewInt(c.floorPpm)
			}
			auction := testAuction(10, principal)
			auction.MaxRateBps = 730
			auction.RateBasisSeconds = 86_400
			got, err := New().DecideOffers(t.Context(), types.OfferInput{
				Now: time.Unix(0, 0), Adapters: []types.AdapterSnapshot{a}, Auctions: []types.AuctionSnapshot{auction},
			})
			if err != nil {
				t.Fatalf("DecideOffers: %v", err)
			}
			if c.want == 0 {
				if len(got.Offers) != 0 {
					t.Fatalf("offers = %+v, want none", got.Offers)
				}
				return
			}
			if len(got.Offers) != 1 || got.Offers[0].ExpectedReturn.Int64() != c.want {
				t.Fatalf("offers = %+v, want one with expectedReturn %d", got.Offers, c.want)
			}
		})
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

func TestStrategyIgnoresOtherAdaptersLiveOffers(t *testing.T) {
	a1, a2 := testAdapter(1, 100_000_000), testAdapter(2, 100_000_000)
	auction := testAuction(10, 100_000_000)
	auction.RemainingAmount = big.NewInt(40_000_000) // a1 already holds 60M on this auction
	out, err := New().DecideOffers(t.Context(), types.OfferInput{
		Adapters:   []types.AdapterSnapshot{a1, a2},
		Auctions:   []types.AuctionSnapshot{auction},
		LiveOffers: []types.LiveOffer{{AdapterID: a1.ID, AuctionID: 10, Principal: big.NewInt(60_000_000)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// a1 is deduped on this auction; a2 offers the full request, not the 40M left after a1's live offer.
	if len(out.Offers) != 1 || out.Offers[0].Maker != a2.Adapter || out.Offers[0].Principal.Int64() != 100_000_000 {
		t.Fatalf("offers = %+v, want only adapter 2 / 100M", out.Offers)
	}
}

func TestStrategyReservesLiveOffersAcrossDiscoveryPasses(t *testing.T) {
	for _, tc := range []struct {
		name          string
		principal     *big.Int
		maxConcurrent int
		want          int64
	}{
		{"remaining principal", big.NewInt(60_000_000), 50, 40_000_000},
		{"fully committed", big.NewInt(100_000_000), 50, 0},
		{"last slot committed", big.NewInt(10_000_000), 1, 0},
		{"unknown principal", nil, 50, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := testAdapter(1, 100_000_000)
			adapter.MaxConcurrent = tc.maxConcurrent
			out, err := New().DecideOffers(t.Context(), types.OfferInput{
				Adapters: []types.AdapterSnapshot{adapter}, Auctions: []types.AuctionSnapshot{testAuction(2, 100_000_000)},
				LiveOffers: []types.LiveOffer{{AdapterID: adapter.ID, AuctionID: 1, Principal: tc.principal}},
			})
			if err != nil {
				t.Fatal(err)
			}
			total := new(big.Int)
			for _, offer := range out.Offers {
				total.Add(total, offer.Principal)
			}
			if total.Int64() != tc.want {
				t.Fatalf("principal = %s, want %d", total, tc.want)
			}
		})
	}
}
