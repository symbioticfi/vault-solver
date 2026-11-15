package rfq

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

func TestQuoteDetailRequiresValidatedDirection(t *testing.T) {
	for _, mode := range []string{"wrong chain", "lane unavailable", "below minimum", "policy", "missing adapters", "reverse", "unknown token", "invalid snapshot", "supported decline"} {
		t.Run(mode, func(t *testing.T) {
			srv := testServer()
			body := validQuoteBody()
			log, events := rfqObservationLogger(t)
			srv.quotes.log = log
			switch mode {
			case "wrong chain":
				body.TokenOutChainID = 8453
			case "lane unavailable":
				srv.quotes.laneAvailable = func() bool { return false }
			case "below minimum":
				srv.quotes.minAmountsIn = map[common.Address]*big.Int{tIn: maxUint256()}
			case "policy":
				srv.quotes.tokenPolicy = testPermissionedPolicy(t, common.HexToAddress("0xffff"))
			case "missing adapters":
				body.Adapters = nil
			case "unknown token":
				body.TokenOut = common.HexToAddress("0x9999").Hex()
			case "reverse":
				body.TokenIn, body.TokenOut = body.TokenOut, body.TokenIn
			case "invalid snapshot":
				body.Adapters[0].Price = ""
			case "supported decline":
				srv.quotes.strategy = &inputRecordingStrategy{quoteOut: types.QuoteOutput{Decision: types.DecisionDecline}}
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequestWithContext(observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", bytes.NewReader(raw))
			r.Header.Set(sharedSecretHeader, testSecret)
			r.Header.Set(requestIDHeader, "scope-operation-id")
			w := httptest.NewRecorder()
			srv.handler().ServeHTTP(w, r)

			resultCount := 0
			for _, event := range *events {
				if mode != "supported decline" {
					for _, field := range []string{"tokenIn", "tokenOut", "amount", "amountIn", "amountOut", "min"} {
						if _, present := event[field]; present {
							t.Fatalf("unproven detail %s leaked: %v", field, event)
						}
					}
					encoded, _ := json.Marshal(event)
					if strings.Contains(string(encoded), "0x0000000000000000000000000000000000009999") {
						t.Fatalf("unvalidated dependency error leaked token: %v", event)
					}
				}
				if event["msg"] == "quote result" {
					resultCount++
					if event["requestId"] != "scope-operation-id" || event["pairInScope"] != (mode == "supported decline") {
						t.Fatalf("decision fields=%v", event)
					}
					if mode == "supported decline" && (event["tokenIn"] != body.TokenIn || event["pairScopeSource"] != "validated_candidates") {
						t.Fatalf("supported decline lost proof: %v", event)
					}
				}
			}
			if resultCount != 1 || (mode != "invalid snapshot" && w.Code != http.StatusNoContent) ||
				(mode == "invalid snapshot" && w.Code != http.StatusBadRequest) {
				t.Fatalf("status=%d decision events=%d", w.Code, resultCount)
			}
		})
	}
}
