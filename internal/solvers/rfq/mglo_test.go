package rfq

import (
	"encoding/json"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestQuoteMGloMainnet(t *testing.T) {
	cfg, err := parseCfg(t, minimalConfig+oneAdapter+`
tokensToQuote: permissioned
permissionedTokens:
  - "0x1DD91a111606382B77A917633ED90feAf25E0F76"
minAmountsIn:
  "0x1DD91a111606382B77A917633ED90feAf25E0F76": "1000000000000000000"
`)
	if err != nil {
		t.Fatal(err)
	}
	mglo := common.HexToAddress("0x1DD91a111606382B77A917633ED90feAf25E0F76")
	usdc := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	if !cfg.TokenPolicy.RequiresSingleRoute(mglo) {
		t.Fatal("mGLO must use one LiquidLane route")
	}

	for _, tc := range []struct {
		name, amount, discount, wantOut string
		wantStatus                      int
	}{
		{"below issuer minimum", "999999999999999999", "0", "", http.StatusNoContent},
		{"at issuer minimum", "1000000000000000000", "0", "1000000", http.StatusOK},
		{"discounted payout", "1000000000000000000", "20000", "980000", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := testServer()
			srv.quotes.tokenPolicy = cfg.TokenPolicy
			srv.quotes.minAmountsIn = cfg.MinAmountsIn
			// Simulated adapter price of $1 per mGLO; token decimals remain 18 -> 6.
			reader := &fakeQuoteCandidateReader{out: map[common.Address]*big.Int{usdc: big.NewInt(1_000_000)}}
			srv.quotes.reader = reader
			body := validQuoteBody()
			body.TokenIn, body.TokenOut, body.Amount = mglo.Hex(), usdc.Hex(), tc.amount
			body.Adapters[0].Asset, body.Adapters[0].Discount = usdc.Hex(), tc.discount

			rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, body)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.wantStatus == http.StatusNoContent {
				if len(reader.queries) != 0 {
					t.Fatal("below-minimum request reached the chain reader")
				}
				return
			}
			var response quoteResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.AmountOut != tc.wantOut {
				t.Fatalf("USDC payout = %s, want %s", response.AmountOut, tc.wantOut)
			}
		})
	}
}
