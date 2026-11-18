package rfq

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

type unvalidatedErrorReader struct{}

func (unvalidatedErrorReader) readQuoteCandidates(context.Context, []solverInventory, common.Address, common.Address, *big.Int, pendingReservations) ([]liquidlane.QuoteCandidate, error) {
	return nil, errors.New("unsupported 0x0000000000000000000000000000000000009999 amount=123456789")
}

func TestQuoteDetailRequiresValidatedDirection(t *testing.T) {
	for _, mode := range []string{"wrong chain", "lane unavailable", "below minimum", "policy", "missing adapters", "reverse", "unknown token", "candidate error", "supported decline"} {
		t.Run(mode, func(t *testing.T) {
			srv := testServer()
			body := validQuoteBody()
			initialReader := srv.quotes.reader.(*fakeQuoteCandidateReader)
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
			case "candidate error":
				srv.quotes.reader = unvalidatedErrorReader{}
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
			switch mode {
			case "wrong chain", "lane unavailable", "below minimum", "policy", "missing adapters":
				if len(initialReader.queries) != 0 {
					t.Fatalf("early refusal added candidate I/O: %v", initialReader.queries)
				}
			}
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
			if resultCount != 1 || (mode != "candidate error" && w.Code != http.StatusNoContent) ||
				(mode == "candidate error" && w.Code != http.StatusBadGateway) {
				t.Fatalf("status=%d decision events=%d", w.Code, resultCount)
			}
		})
	}
}
