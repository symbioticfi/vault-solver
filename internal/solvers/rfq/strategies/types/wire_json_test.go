package types

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func mustBig(t *testing.T, s string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad big.Int %q", s)
	}
	return n
}

func TestQuoteInputMarshalJSONWireShape(t *testing.T) {
	discountID := common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000ab")
	route := liquidlane.Route{
		ID: "internal-only", Adapter: common.HexToAddress("0x0000000000000000000000000000000000000003"),
		TokenOut: common.HexToAddress("0x0000000000000000000000000000000000000002"), TokenOutDecimals: 6,
	}
	input := QuoteInput{
		RequestID:          "request-1",
		QuoteID:            "quote-1",
		ChainID:            1,
		Executor:           common.HexToAddress("0x0000000000000000000000000000000000000010"),
		TokenIn:            common.HexToAddress("0x0000000000000000000000000000000000000001"),
		TokenOut:           common.HexToAddress("0x0000000000000000000000000000000000000002"),
		AmountIn:           mustBig(t, "1000000000000000000"),
		RequireSingleRoute: true,
		Candidates: []liquidlane.QuoteCandidate{{
			ID:           "candidate-1",
			Route:        route,
			MaxAmountOut: mustBig(t, "1000000"),
			Rate:         mustBig(t, "1000000000000000000"),
			DiscountID:   &discountID,
		}},
		Now: time.Unix(1, 0).UTC(),
	}

	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(body), "AmountIn") || !strings.Contains(string(body), `"amountIn":"1000000000000000000"`) {
		t.Fatalf("JSON does not use lower-camel decimal-string amountIn: %s", body)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	if _, ok := raw["requiredAmountOut"]; ok {
		t.Fatalf("requiredAmountOut should be omitted when nil: %s", body)
	}
	if _, ok := raw["mode"]; ok {
		t.Fatalf("mode should not be part of the RFQ strategy input: %s", body)
	}
	requireSingleRoute, ok := raw["requireSingleRoute"].(bool)
	if !ok || !requireSingleRoute {
		t.Fatalf("requireSingleRoute = %#v, want true", raw["requireSingleRoute"])
	}
	candidates, ok := raw["candidates"].([]any)
	if !ok || len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one candidate", raw["candidates"])
	}
	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		t.Fatalf("candidate = %#v, want object", candidates[0])
	}
	if candidate["maxAssets"] != "1000000" || candidate["maxRate"] != "1000000000000000000" {
		t.Fatalf("candidate amounts not decimal strings: %#v", candidate)
	}
	if _, routeExists := candidate["route"]; routeExists {
		t.Fatalf("internal route leaked into webhook JSON: %#v", candidate)
	}
}

func TestQuoteOutputUnmarshalJSONWireShape(t *testing.T) {
	var out QuoteOutput
	if err := json.Unmarshal([]byte(`{
		"decision": "quote",
		"reason": "selected",
		"quotedAmountOut": "1000000",
		"legs": [{"candidateId": "candidate-1", "amountIn": "1000000000000000000", "amountOut": "1000000"}]
	}`), &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Decision != DecisionQuote || out.Reason != "selected" {
		t.Fatalf("unexpected metadata: %+v", out)
	}
	if out.QuotedAmountOut.String() != "1000000" {
		t.Fatalf("quotedAmountOut = %s, want 1000000", out.QuotedAmountOut)
	}
	if len(out.Legs) != 1 ||
		out.Legs[0].CandidateID != "candidate-1" ||
		out.Legs[0].AmountIn.String() != "1000000000000000000" ||
		out.Legs[0].AmountOut.String() != "1000000" {
		t.Fatalf("unexpected legs: %+v", out.Legs)
	}
}

func TestQuoteOutputUnmarshalJSONRejectsUnknownFields(t *testing.T) {
	var out QuoteOutput
	err := json.Unmarshal([]byte(`{"decision":"decline","extra":1}`), &out)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Unmarshal error = %v, want unknown field rejection", err)
	}
}

func TestQuoteOutputUnmarshalJSONRejectsInvalidDecimal(t *testing.T) {
	var out QuoteOutput
	err := json.Unmarshal([]byte(`{"decision":"quote","quotedAmountOut":"not-a-number"}`), &out)
	if err == nil || !strings.Contains(err.Error(), "quotedAmountOut") {
		t.Fatalf("Unmarshal error = %v, want quotedAmountOut decimal rejection", err)
	}
}
