package bridgefacilitator

import (
	"encoding/json"
	"testing"

	"github.com/symbioticfi/vault-solver/api/threef"
)

func TestAuctionViewRateBasisSeconds(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		want   int64
		wantOk bool
	}{
		{"absent (pre-switch)", `{"id":1,"maxRate":2}`, 0, true},
		{"annual", `{"id":1,"maxRate":730,"rateBasisSeconds":31536000}`, 31536000, true},
		{"legacy settlement delay", `{"id":1,"maxRate":100,"rateBasisSeconds":2678400}`, 2678400, true},
		{"null fails closed", `{"id":1,"rateBasisSeconds":null}`, 0, false},
		{"string fails closed", `{"id":1,"rateBasisSeconds":"31536000"}`, 0, false},
		{"zero fails closed", `{"id":1,"rateBasisSeconds":0}`, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var dto threef.AuctionDto
			if err := json.Unmarshal([]byte(c.body), &dto); err != nil {
				t.Fatal(err)
			}
			got, ok := (auctionView{dto: dto}).rateBasisSeconds()
			if got != c.want || ok != c.wantOk {
				t.Fatalf("rateBasisSeconds() = %d, %v; want %d, %v", got, ok, c.want, c.wantOk)
			}
		})
	}
}
