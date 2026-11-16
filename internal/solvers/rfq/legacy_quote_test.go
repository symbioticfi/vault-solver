package rfq

import (
	"encoding/json"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// Main's backend sends none of the three snapshot-pricing fields. Keep the
// authenticated legacy envelope usable throughout a backend/solver rollout.
func TestLegacyBackendQuote(t *testing.T) {
	for _, tc := range []struct {
		name       string
		discounted bool
		reserved   int64
		wantStatus int
		wantOut    string
	}{
		{"direct", false, 0, http.StatusOK, "1000000"},
		{"signed discount", true, 0, http.StatusOK, "990000"},
		{"reserved", false, 10000000, http.StatusNoContent, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer()
			s.quotes.discountsEnabled = true
			s.quotes.legacyReader = &fakeQuoteCandidateReader{out: map[common.Address]*big.Int{tOut: big.NewInt(1000000)}}
			s.quotes.reservations = func(_ string, _ []solverInventory) liquidlane.CapacityReservations {
				return liquidlane.CapacityReservations{liquidlane.NewCapacityID(1, vlt, tOut): big.NewInt(tc.reserved)}
			}
			body := validQuoteBody()
			body.Adapters[0].Price = ""
			body.Adapters[0].TokenInDecimals = nil
			if tc.discounted {
				id := common.HexToHash("0x1234").Hex()
				body.Adapters[0].DiscountID = &id
				body.Adapters[0].Discount = "10000"
			}
			response := do(t, s.handler(), http.MethodPost, "/quote", testSecret, body)
			if response.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d: %s", response.Code, tc.wantStatus, response.Body.String())
			}
			if response.Code == http.StatusOK {
				var quote quoteResponse
				if err := json.Unmarshal(response.Body.Bytes(), &quote); err != nil {
					t.Fatal(err)
				}
				if quote.AmountOut != tc.wantOut {
					t.Fatalf("output=%s want=%s", quote.AmountOut, tc.wantOut)
				}
			}
		})
	}
}
