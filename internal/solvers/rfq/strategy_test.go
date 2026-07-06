package rfq

import (
	"context"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/default"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/webhook"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategytypes"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

func baseQuoteInput(t *testing.T) strategytypes.QuoteInput {
	t.Helper()
	return strategytypes.QuoteInput{
		RequestID: "r", QuoteID: "q", ChainID: 1,
		Executor: common.HexToAddress("0x0000000000000000000000000000000000000010"),
		TokenIn:  tIn, TokenOut: tOut, AmountIn: mustBig(t, "1000000000000000000"),
		Candidates: []strategytypes.QuoteCandidate{{
			ID: "c0", Adapter: vlt, Asset: tOut, AssetDecimals: 6,
			MaxAssets: mustBig(t, "10000000"),
			MaxRate:   mustBig(t, "1000000000000000000"),
		}},
		Now: time.Unix(0, 0),
	}
}

func TestDefaultStrategyDecideQuote(t *testing.T) {
	pricing := &fakeStrategyPricing{
		decimals: 18,
		out:      map[common.Address]*big.Int{tOut: mustBig(t, "1000000")},
	}
	out, err := defaultstrategy.New(pricing).DecideQuote(t.Context(), baseQuoteInput(t))
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if out.Decision != strategytypes.DecisionQuote || out.QuotedAmountOut.String() != "1000000" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if len(out.Legs) != 1 || out.Legs[0].CandidateID != "c0" {
		t.Fatalf("legs = %+v, want candidate c0", out.Legs)
	}
	if len(pricing.queries) != 1 || len(pricing.queries[0]) != 1 || pricing.queries[0][0].Adapter != vlt {
		t.Fatalf("pricing queries = %+v, want one batched query for %s", pricing.queries, vlt.Hex())
	}
}

func TestValidateQuoteOutputRejectsUnsafeOutputs(t *testing.T) {
	input := baseQuoteInput(t)
	cases := map[string]strategytypes.QuoteOutput{
		"unknown candidate": {
			Decision:        strategytypes.DecisionQuote,
			QuotedAmountOut: mustBig(t, "1000000"),
			Legs: []strategytypes.QuoteLeg{{
				CandidateID: "missing",
				AmountIn:    mustBig(t, "1000000000000000000"),
				AmountOut:   mustBig(t, "1000000"),
			}},
		},
		"exceeds max assets": {
			Decision:        strategytypes.DecisionQuote,
			QuotedAmountOut: mustBig(t, "10000001"),
			Legs: []strategytypes.QuoteLeg{{
				CandidateID: "c0",
				AmountIn:    mustBig(t, "1000000000000000000"),
				AmountOut:   mustBig(t, "10000001"),
			}},
		},
		"wrong input sum": {
			Decision:        strategytypes.DecisionQuote,
			QuotedAmountOut: mustBig(t, "1000000"),
			Legs: []strategytypes.QuoteLeg{{
				CandidateID: "c0",
				AmountIn:    mustBig(t, "1"),
				AmountOut:   mustBig(t, "1000000"),
			}},
		},
		"exceeds max rate": {
			Decision:        strategytypes.DecisionQuote,
			QuotedAmountOut: mustBig(t, "1000001"),
			Legs: []strategytypes.QuoteLeg{{
				CandidateID: "c0",
				AmountIn:    mustBig(t, "1000000000000000000"),
				AmountOut:   mustBig(t, "1000001"),
			}},
		},
		"zero max assets": {
			Decision:        strategytypes.DecisionQuote,
			QuotedAmountOut: mustBig(t, "1000000"),
			Legs: []strategytypes.QuoteLeg{{
				CandidateID: "zero-capacity",
				AmountIn:    mustBig(t, "1000000000000000000"),
				AmountOut:   mustBig(t, "1000000"),
			}},
		},
	}
	input.Candidates = append(input.Candidates, strategytypes.QuoteCandidate{
		ID: "zero-capacity", Adapter: vlt, Asset: tOut, AssetDecimals: 6,
		MaxAssets: big.NewInt(0), MaxRate: mustBig(t, "1000000000000000000"),
	})
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			if rec, err := validateQuoteOutput(t.Context(), input, out, 18, fakeReader{decimals: 18}); err == nil || rec != nil {
				t.Fatalf("validateQuoteOutput = (%+v, %v), want rejection", rec, err)
			}
		})
	}
}

func TestValidateQuoteOutputRejectsRecoveryBelowRequired(t *testing.T) {
	input := baseQuoteInput(t)
	input.RequiredAmountOut = mustBig(t, "1000001")
	out := strategytypes.QuoteOutput{
		Decision:        strategytypes.DecisionQuote,
		QuotedAmountOut: mustBig(t, "1000000"),
		Legs: []strategytypes.QuoteLeg{{
			CandidateID: "c0",
			AmountIn:    mustBig(t, "1000000000000000000"),
			AmountOut:   mustBig(t, "1000000"),
		}},
	}
	if rec, err := validateQuoteOutput(t.Context(), input, out, 18, fakeReader{decimals: 18}); err == nil || rec != nil {
		t.Fatalf("validateQuoteOutput = (%+v, %v), want below-required rejection", rec, err)
	}
}

func TestDecideQuoteRejectsStrategyOutputAboveMaxRate(t *testing.T) {
	input := baseQuoteInput(t)
	out := strategytypes.QuoteOutput{
		Decision:        strategytypes.DecisionQuote,
		QuotedAmountOut: mustBig(t, "1000001"),
		Legs: []strategytypes.QuoteLeg{{
			CandidateID: "c0",
			AmountIn:    mustBig(t, "1000000000000000000"),
			AmountOut:   mustBig(t, "1000001"),
		}},
	}
	rec, err := decideQuote(t.Context(), input, fixedStrategy{out: out}, fakeReader{decimals: 18})
	if err == nil || rec != nil || !strings.Contains(err.Error(), "exceeds candidate maxRate") {
		t.Fatalf("decideQuote = (%+v, %v), want maxRate rejection", rec, err)
	}
}

func TestDecideQuoteRejectsStrategyOutputAboveLiveAmountOut(t *testing.T) {
	input := baseQuoteInput(t)
	input.Candidates[0].MaxRate = mustBig(t, "2000000000000000000")
	out := strategytypes.QuoteOutput{
		Decision:        strategytypes.DecisionQuote,
		QuotedAmountOut: mustBig(t, "1500000"),
		Legs: []strategytypes.QuoteLeg{{
			CandidateID: "c0",
			AmountIn:    mustBig(t, "1000000000000000000"),
			AmountOut:   mustBig(t, "1500000"),
		}},
	}
	rec, err := decideQuote(t.Context(), input, fixedStrategy{out: out}, fakeReader{
		decimals: 18,
		liveOut:  map[common.Address]*big.Int{vlt: mustBig(t, "1000000")},
	})
	if err == nil || rec != nil || !strings.Contains(err.Error(), "exceeds live amountOut") {
		t.Fatalf("decideQuote = (%+v, %v), want live amountOut rejection", rec, err)
	}
}

func TestWebhookStrategyDecodesLowerCamelResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		if !strings.Contains(string(body), `"amountIn":"1000000000000000000"`) ||
			strings.Contains(string(body), `"AmountIn"`) {
			t.Fatalf("request body does not use decimal-string lower-camel JSON: %s", string(body))
		}
		_, _ = w.Write([]byte(`{
			"decision": "quote",
			"quotedAmountOut": "1000000",
			"legs": [{"candidateId": "c0", "amountIn": "1000000000000000000", "amountOut": "1000000"}]
		}`))
	}))
	defer srv.Close()
	client, err := webhook.NewClient(webhook.Config{URL: srv.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	out, err := webhookstrategy.New(client).DecideQuote(t.Context(), baseQuoteInput(t))
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if out.Decision != strategytypes.DecisionQuote || out.QuotedAmountOut.String() != "1000000" ||
		len(out.Legs) != 1 || out.Legs[0].CandidateID != "c0" {
		t.Fatalf("unexpected webhook output: %+v", out)
	}
}

type fixedStrategy struct {
	out strategytypes.QuoteOutput
}

func (s fixedStrategy) DecideQuote(_ context.Context, _ strategytypes.QuoteInput) (strategytypes.QuoteOutput, error) {
	return s.out, nil
}
