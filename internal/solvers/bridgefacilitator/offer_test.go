package bridgefacilitator

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/solver"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
)

func TestBuildSignedOffer(t *testing.T) {
	auction := auctionView{testAuctionDto(7, common.Address{0xaa})}
	request := auction.requestAddr()
	maker := common.HexToAddress("0x0000000000000000000000000000000000000042")
	s := &Solver{
		cfg:  &Config{OfferExpiryBuffer: time.Hour},
		deps: solver.Deps{Signer: fakeSigner{}},
	}
	s.nonceSeq.Store(41)

	dto, err := s.buildSignedOffer(auction, strategytypes.OfferExecution{
		AuctionID: 7, Request: request, Maker: maker,
		Principal: big.NewInt(100), ExpectedReturn: big.NewInt(2),
	})
	if err != nil {
		t.Fatalf("buildSignedOffer: %v", err)
	}
	if dto.AuctionId != 7 || dto.Maker != lowerAddr(maker) || dto.Amount != "100" ||
		dto.ExpectedReturn != "2" || dto.Nonce != "42" || !dto.UseCallback {
		t.Fatalf("unexpected offer DTO: %+v", dto)
	}
	if dto.GetChainId() != 11155111 || len(dto.GetSignature()) != 132 {
		t.Fatalf("chainId=%v signature=%q", dto.GetChainId(), dto.GetSignature())
	}
}

func TestOfferExpiration(t *testing.T) {
	buffer := 2 * time.Hour
	now := time.Unix(1_700_000_000, 0).UTC()

	withSolveStart := func(s string) auctionView {
		dto := testAuctionDto(1, common.Address{0xaa})
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
			name: "past solve start still anchors expiry to solveStart+buffer",
			av:   withSolveStart(now.Add(-time.Hour).Format(time.RFC3339)),
			want: now.Add(-time.Hour).Add(buffer).Unix(),
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
