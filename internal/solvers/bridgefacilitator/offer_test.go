package bridgefacilitator

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestOfferExpiration(t *testing.T) {
	buffer := 2 * time.Hour
	now := time.Unix(1_700_000_000, 0).UTC()

	withSolveStart := func(s string) auctionView {
		dto := testAuctionDto(1, common.Address{0xaa}, "100")
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
