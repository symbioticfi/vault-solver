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

func candidate(adapterID string, auctionID int64, capacity int64) types.OfferCandidate {
	return types.OfferCandidate{
		ID:        adapterID + ":" + big.NewInt(auctionID).String(),
		AdapterID: adapterID,
		AuctionID: auctionID,
		Capacity:  big.NewInt(capacity),
	}
}

func TestStrategyLargestFirstClampsLastOffer(t *testing.T) {
	a1 := testAdapter(1, 50)
	a2 := testAdapter(2, 80)
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1, a2},
		Auctions: []types.AuctionSnapshot{testAuction(10, 100)},
		Candidates: []types.OfferCandidate{
			candidate(a1.ID, 10, 50),
			candidate(a2.ID, 10, 80),
		},
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

func TestStrategyClampsOfferToCandidateCapacity(t *testing.T) {
	a1 := testAdapter(1, 1000)
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1},
		Auctions: []types.AuctionSnapshot{testAuction(10, 500)},
		Candidates: []types.OfferCandidate{
			candidate(a1.ID, 10, 100),
		},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 1 {
		t.Fatalf("offers = %d, want 1", len(got.Offers))
	}
	if got.Offers[0].Principal.Int64() != 100 {
		t.Fatalf("principal = %s, want candidate capacity 100", got.Offers[0].Principal)
	}
}

func TestStrategyRejectsZeroCandidateCapacity(t *testing.T) {
	a1 := testAdapter(1, 1000)
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1},
		Auctions: []types.AuctionSnapshot{testAuction(10, 500)},
		Candidates: []types.OfferCandidate{
			candidate(a1.ID, 10, 0),
		},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 0 {
		t.Fatalf("offers = %+v, want none because candidate capacity is zero", got.Offers)
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
		Candidates: []types.OfferCandidate{
			candidate(a1.ID, 10, 80),
			candidate(a1.ID, 11, 80),
		},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 2 {
		t.Fatalf("offers = %d, want 2", len(got.Offers))
	}
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
		Candidates: []types.OfferCandidate{
			candidate(a1.ID, 10, 50),
			candidate(a2.ID, 10, 80),
		},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 1 || got.Offers[0].Maker != a2.Adapter || got.Offers[0].Principal.Int64() != 80 {
		t.Fatalf("offers = %+v, want only adapter 2 / 80", got.Offers)
	}
}

func TestStrategyOwnsCandidateEligibility(t *testing.T) {
	a1 := testAdapter(1, 100)
	a2 := testAdapter(2, 100)
	a2.Collateral = common.Address{0xbb}
	a3 := testAdapter(3, 100)
	a3.MinYieldBps = big.NewInt(300)
	auction := testAuction(10, 100)
	input := types.OfferInput{
		Now:      time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{a1, a2, a3},
		Auctions: []types.AuctionSnapshot{auction},
		Candidates: []types.OfferCandidate{
			func() types.OfferCandidate {
				c := candidate(a1.ID, 10, 100)
				c.HasLiveOffer = true
				return c
			}(),
			candidate(a2.ID, 10, 100),
			candidate(a3.ID, 10, 100),
		},
	}

	got, err := New().DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(got.Offers) != 0 {
		t.Fatalf("offers = %+v, want none: live, collateral, and min-yield filters are strategy-owned", got.Offers)
	}
}
