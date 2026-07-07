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
		MinYieldBps:   new(big.Int),
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
	a1 := testAdapter(1, 50) // capacity 50
	a2 := testAdapter(2, 80) // capacity 80
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1, a2},
		Auctions: []types.AuctionSnapshot{testAuction(10, 100)},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 2 {
		t.Fatalf("offers = %d, want 2", len(got.Offers))
	}
	if got.Offers[0].Maker != a2.Adapter || got.Offers[0].Principal.Int64() != 80 {
		t.Fatalf("offer0 = %+v, want adapter 2 / 80", got.Offers[0])
	}
	if got.Offers[1].Maker != a1.Adapter || got.Offers[1].Principal.Int64() != 20 {
		t.Fatalf("offer1 = %+v, want adapter 1 / 20", got.Offers[1])
	}
	if got.Offers[0].ExpectedReturn.String() != "1" || got.Offers[1].ExpectedReturn.Sign() != 0 {
		t.Fatalf("expected returns = %s/%s, want 1/0", got.Offers[0].ExpectedReturn, got.Offers[1].ExpectedReturn)
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
	a1 := testAdapter(1, 100)
	a1.MaxAssets = big.NewInt(80)
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1},
		Auctions: []types.AuctionSnapshot{
			testAuction(10, 70),
			testAuction(11, 70),
		},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 2 {
		t.Fatalf("offers = %d, want 2", len(got.Offers))
	}
	// Auction 10 takes 70 of the 80 ceiling; auction 11 sees only 100-70=30 of budget left.
	if got.Offers[0].Principal.Int64() != 70 || got.Offers[1].Principal.Int64() != 30 {
		t.Fatalf("principals = %s/%s, want 70/30", got.Offers[0].Principal, got.Offers[1].Principal)
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

func TestStrategyOwnsEligibility(t *testing.T) {
	a1 := testAdapter(1, 100) // filtered by an existing live offer
	a2 := testAdapter(2, 100)
	a2.Collateral = common.Address{0xbb} // collateral mismatch
	a3 := testAdapter(3, 100)
	a3.MinYieldBps = big.NewInt(300) // min-yield above the auction's max rate (200)
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
