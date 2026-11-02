package bridgefacilitator

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
)

func TestOfferExpiration(t *testing.T) {
	buffer := 2 * time.Hour
	now := time.Unix(1_700_000_000, 0).UTC()

	withSolveStart := func(s string) auctionView {
		dto := testAuctionDto(1, common.Address{0xaa}, "1000")
		if s != "" {
			dto.SetSolveStartTime(s)
		}
		return auctionView{dto}
	}

	tests := []struct {
		name string
		av   auctionView
		want int64
	}{
		{
			name: "future solve start anchors expiry to solveStart+buffer",
			av:   withSolveStart(now.Add(time.Hour).Format(time.RFC3339)),
			want: now.Add(time.Hour).Add(buffer).Unix(),
		},
		{
			name: "past solve start retains now+buffer floor",
			av:   withSolveStart(now.Add(-time.Hour).Format(time.RFC3339)),
			want: now.Add(buffer).Unix(),
		},
		{
			name: "long expired solve start retains now+buffer floor",
			av:   withSolveStart(now.Add(-3 * time.Hour).Format(time.RFC3339)),
			want: now.Add(buffer).Unix(),
		},
		{
			name: "missing solve start falls back to now+buffer",
			av:   withSolveStart(""),
			want: now.Add(buffer).Unix(),
		},
		{
			name: "unparseable solve start falls back to now+buffer",
			av:   withSolveStart("not-a-timestamp"),
			want: now.Add(buffer).Unix(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := offerExpiration(tt.av, buffer, now).Int64(); got != tt.want {
				t.Fatalf("offerExpiration = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestBuildSignedOfferReusesLiveNonce: a top-up of a live offer is signed with that offer's nonce so 3F
// updates it and the chain can consume only one of the two signatures; a fresh pair takes a new nonce.
func TestBuildSignedOfferReusesLiveNonce(t *testing.T) {
	adapter := common.Address{0xad}
	s := &Solver{
		cfg:    &Config{OfferExpiryBuffer: time.Hour},
		deps:   solver.Deps{Signer: fakeSigner{}},
		offers: newOfferTracker(),
	}
	s.offers.reconcileAdapter(adapter, map[int64]offerState{
		1: {expiry: time.Now().Add(time.Hour), principal: big.NewInt(60), nonce: big.NewInt(42), open: true},
	})
	dto := testAuctionDto(1, common.Address{0xaa}, "1000")
	name, version, chainID := "SuperstateRequest", "1", float32(1)
	dto.SetEip712Domain(*threef.NewAuctionEip712DomainDto(
		*threef.NewNullableString(&name), *threef.NewNullableString(&version), *threef.NewNullableFloat32(&chainID)))
	offer := types.OfferExecution{AuctionID: 1, Maker: adapter, Principal: big.NewInt(100), ExpectedReturn: big.NewInt(2)}

	got, err := s.buildSignedOffer(t.Context(), auctionView{dto}, offer)
	if err != nil {
		t.Fatal(err)
	}
	if got.Nonce != "42" {
		t.Fatalf("top-up nonce = %s, want the live offer's 42", got.Nonce)
	}
	offer.AuctionID = 2
	dto.Id = 2
	got, err = s.buildSignedOffer(t.Context(), auctionView{dto}, offer)
	if err != nil {
		t.Fatal(err)
	}
	if got.Nonce == "42" || got.Nonce == "0" {
		t.Fatalf("fresh offer nonce = %s, want a new one", got.Nonce)
	}
}
