package rfq

import (
	"context"
	"math/big"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

// mGLOBAL Hoodi address, used as a permissioned-token fixture.
var permissionedToken = common.HexToAddress("0x2Ee6f1A395Bce7a7c5bF1D07bAaF9F8A0828A8d3")

func TestParseConfigTokenScope(t *testing.T) {
	const base = `
backendUrl: https://rfq-backend.example
backendSharedSecretEnv: RFQ_BACKEND_SHARED_SECRET
executor: "0x0000000000000000000000000000000000000010"
solverMode: internal
`

	cfg, err := parseCfg(t, base+`
tokensToQuote: permissioned
permissionedTokens:
  - "0x2Ee6f1A395Bce7a7c5bF1D07bAaF9F8A0828A8d3"
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.TokenPolicy.Scope() != tokenpolicy.Permissioned {
		t.Errorf("TokenPolicy.Scope() = %q, want %q", cfg.TokenPolicy.Scope(), tokenpolicy.Permissioned)
	}
	if !cfg.TokenPolicy.RequiresSingleRoute(permissionedToken) {
		t.Errorf("expected mGLOBAL to require one route")
	}

	def, err := parseCfg(t, base)
	if err != nil {
		t.Fatalf("parse default: %v", err)
	}
	if def.TokenPolicy.Scope() != tokenpolicy.All {
		t.Errorf("default token scope = %q, want %q", def.TokenPolicy.Scope(), tokenpolicy.All)
	}

	if _, err := parseCfg(t, base+"tokensToQuote: bogus\n"); err == nil {
		t.Errorf("expected error for invalid tokensToQuote")
	}
}

func TestParseConfigMinAmountsIn(t *testing.T) {
	cfg, err := parseCfg(t, minimalConfig+oneAdapter+`
minAmountsIn:
  "0x1204371AC0e5176f4B8c5B2F16C2Bec551b6FC1a": "100000000000000000000"
  "0xaaa0008c8cf3a7dca931adaf04336a5d808c82cc": "1000000000000000000000"
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.MinAmountsIn) != 2 {
		t.Fatalf("minAmountsIn = %d entries, want 2", len(cfg.MinAmountsIn))
	}
	// Keys are addresses, so the configured checksum casing does not matter at lookup time.
	got := cfg.MinAmountsIn[common.HexToAddress("0x1204371ac0e5176f4b8c5b2f16c2bec551b6fc1a")]
	if got == nil || got.Cmp(mustBig(t, "100000000000000000000")) != 0 {
		t.Fatalf("HYBOND minimum = %v, want 100e18", got)
	}
	got = cfg.MinAmountsIn[common.HexToAddress("0xAAA0008C8CF3A7Dca931adaF04336A5D808C82Cc")]
	if got == nil || got.Cmp(mustBig(t, "1000000000000000000000")) != 0 {
		t.Fatalf("deJAAA minimum = %v, want 1000e18", got)
	}

	def, err := parseCfg(t, minimalConfig+oneAdapter)
	if err != nil {
		t.Fatalf("parse default: %v", err)
	}
	if def.MinAmountsIn != nil {
		t.Fatalf("default minAmountsIn = %v, want nil (no minimums)", def.MinAmountsIn)
	}
}

func TestParseConfigMinAmountsInErrors(t *testing.T) {
	cases := map[string]string{ //nolint:gosec // G101 false positive: YAML test fixtures, not credentials.
		"zero address key": `
minAmountsIn:
  "0x0000000000000000000000000000000000000000": "1"
`,
		"invalid address key": `
minAmountsIn:
  "not-an-address": "1"
`,
		"non-numeric value": `
minAmountsIn:
  "0x1204371AC0e5176f4B8c5B2F16C2Bec551b6FC1a": "lots"
`,
		"zero value": `
minAmountsIn:
  "0x1204371AC0e5176f4B8c5B2F16C2Bec551b6FC1a": "0"
`,
		"negative value": `
minAmountsIn:
  "0x1204371AC0e5176f4B8c5B2F16C2Bec551b6FC1a": "-1"
`,
		"same token twice in different casing": `
minAmountsIn:
  "0x1204371AC0e5176f4B8c5B2F16C2Bec551b6FC1a": "1"
  "0x1204371ac0e5176f4b8c5b2f16c2bec551b6fc1a": "2"
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCfg(t, minimalConfig+oneAdapter+body); err == nil {
				t.Fatalf("expected an error for %q", name)
			}
		})
	}
}

// countingStrategy delegates to the in-process default strategy while counting quote decisions, so a
// gated request can assert the strategy was never consulted.
type countingStrategy struct {
	types.Strategy

	quoteCalls int
}

type countingQuoteReader struct {
	quoteCandidateReader

	calls int
}

func (r *countingQuoteReader) readQuoteCandidates(
	ctx context.Context,
	inventory []solverInventory,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
) ([]liquidlane.QuoteCandidate, error) {
	r.calls++
	return r.quoteCandidateReader.readQuoteCandidates(ctx, inventory, tokenIn, tokenOut, amountIn)
}

type laneFlippingStrategy struct {
	types.Strategy

	ready *atomic.Bool
}

func (s *laneFlippingStrategy) DecideQuote(
	ctx context.Context,
	input types.QuoteInput,
) (types.QuoteOutput, error) {
	out, err := s.Strategy.DecideQuote(ctx, input)
	s.ready.Store(false)
	return out, err
}

func TestQuoteDeclinesBeforePlanningWhenLaneNotReady(t *testing.T) {
	var ready atomic.Bool
	ready.Store(false)

	srv := testServer()
	reader := &countingQuoteReader{quoteCandidateReader: srv.quotes.reader}
	strategy := &countingStrategy{Strategy: srv.quotes.strategy}
	srv.quotes.laneReady = ready.Load
	srv.quotes.reader = reader
	srv.quotes.strategy = strategy

	rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, validQuoteBody())
	if rr.Code != http.StatusNoContent {
		t.Fatalf("quote status = %d, want 204 (body %s)", rr.Code, rr.Body.String())
	}
	if reader.calls != 0 || strategy.quoteCalls != 0 {
		t.Fatalf("non-ready lane performed reader=%d strategy=%d calls, want none", reader.calls, strategy.quoteCalls)
	}
}

func TestQuoteDeclinesWhenLaneBecomesBusyDuringPlanning(t *testing.T) {
	var ready atomic.Bool
	ready.Store(true)

	srv := testServer()
	reader := &countingQuoteReader{quoteCandidateReader: srv.quotes.reader}
	srv.quotes.laneReady = ready.Load
	srv.quotes.reader = reader
	srv.quotes.strategy = &laneFlippingStrategy{
		Strategy: srv.quotes.strategy,
		ready:    &ready,
	}

	rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, validQuoteBody())
	if rr.Code != http.StatusNoContent {
		t.Fatalf("quote status = %d, want 204 after lane pause (body %s)", rr.Code, rr.Body.String())
	}
	if reader.calls != 1 {
		t.Fatalf("candidate reader calls = %d, want one completed planning read", reader.calls)
	}
}

func (s *countingStrategy) DecideQuote(
	ctx context.Context,
	input types.QuoteInput,
) (types.QuoteOutput, error) {
	s.quoteCalls++
	return s.Strategy.DecideQuote(ctx, input)
}

func TestQuoteMinAmountIn(t *testing.T) {
	// validQuoteBody quotes 1e18 of tIn; the gate is evaluated against that amount.
	const amountIn = "1000000000000000000"
	cases := map[string]struct {
		minAmountsIn map[common.Address]*big.Int
		wantQuote    bool
	}{
		"below minimum declines":    {minAmountsIn: minAmountsFor(t, tIn, "2000000000000000000")},
		"equal to minimum quotes":   {minAmountsIn: minAmountsFor(t, tIn, amountIn), wantQuote: true},
		"above minimum quotes":      {minAmountsIn: minAmountsFor(t, tIn, "500000000000000000"), wantQuote: true},
		"minimum for another token": {minAmountsIn: minAmountsFor(t, tOut, "2000000000000000000"), wantQuote: true},
		"no minimum configured":     {wantQuote: true},
		"one wei below the minimum": {minAmountsIn: minAmountsFor(t, tIn, "1000000000000000001")},
		"one wei above the minimum": {minAmountsIn: minAmountsFor(t, tIn, "999999999999999999"), wantQuote: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := testServer()
			strategy := &countingStrategy{Strategy: newDefaultTestStrategy()}
			srv.quotes.strategy = strategy
			srv.quotes.minAmountsIn = tc.minAmountsIn
			request := validQuoteBody()
			request.Amount = amountIn

			response, err := srv.quotes.quote(t.Context(), &request)
			if err != nil {
				t.Fatalf("quote: %v", err)
			}
			if !tc.wantQuote {
				if response != nil {
					t.Fatalf("quote = %+v, want no quote (204)", response)
				}
				if strategy.quoteCalls != 0 {
					t.Fatalf("strategy consulted %d times for a below-minimum request", strategy.quoteCalls)
				}
				return
			}
			if response == nil {
				t.Fatal("quote declined, want a quote")
			}
			if strategy.quoteCalls != 1 {
				t.Fatalf("strategy quote calls = %d, want 1", strategy.quoteCalls)
			}
		})
	}
}

func minAmountsFor(t *testing.T, token common.Address, amount string) map[common.Address]*big.Int {
	t.Helper()
	return map[common.Address]*big.Int{token: mustBig(t, amount)}
}

type inputRecordingStrategy struct {
	quoteInput types.QuoteInput
	fillInput  types.FillInput
	quoteOut   types.QuoteOutput
	fillPlan   *types.FillPlan
}

func (s *inputRecordingStrategy) DecideQuote(
	_ context.Context,
	input types.QuoteInput,
) (types.QuoteOutput, error) {
	s.quoteInput = input
	return s.quoteOut, nil
}

func (s *inputRecordingStrategy) BuildFillPlan(
	_ context.Context,
	input types.FillInput,
) (*types.FillPlan, error) {
	s.fillInput = input
	return s.fillPlan, nil
}

func TestQuoteMarksPermissionedScopeAsSingleRoute(t *testing.T) {
	route := liquidlane.NewRoute(1, vlt, common.Address{}, permissionedToken, tOut, 18, 6)
	strategy := &inputRecordingStrategy{quoteOut: types.QuoteOutput{
		Decision:        types.DecisionQuote,
		QuotedAmountOut: big.NewInt(1_000000),
		Legs: []types.QuoteLeg{{
			CandidateID: string(liquidlane.NewCandidateID(route, nil)),
			AmountIn:    big.NewInt(1_000000000000000000),
			AmountOut:   big.NewInt(1_000000),
		}},
	}}
	srv := testServer()
	srv.quotes.tokenPolicy = testPermissionedPolicy(t, permissionedToken)
	srv.quotes.strategy = strategy
	request := validQuoteBody()
	request.TokenIn = permissionedToken.Hex()

	response, err := srv.quotes.quote(t.Context(), &request)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if response == nil {
		t.Fatal("quote declined, want response")
	}
	if !strategy.quoteInput.RequireSingleRoute {
		t.Fatal("permissioned quote input did not require a single route")
	}
	if len(strategy.quoteInput.Candidates) != 1 {
		t.Fatalf("candidates = %d, want one normalized LiquidLane candidate", len(strategy.quoteInput.Candidates))
	}
	candidate := strategy.quoteInput.Candidates[0]
	if candidate.Route.TokenIn != permissionedToken || candidate.Route.TokenOut != tOut ||
		candidate.Route.TokenInDecimals != 18 ||
		candidate.Rate.Cmp(big.NewInt(1_000_000_000_000_000_000)) != 0 ||
		candidate.MaxAmountOut.Cmp(big.NewInt(10_000_000)) != 0 {
		t.Fatalf("candidate = %+v, want typed current LiquidLane facts", candidate)
	}
}

func TestQuoteNormalizesDiscountRateWithInputDecimals(t *testing.T) {
	strategy := &inputRecordingStrategy{quoteOut: types.QuoteOutput{Decision: types.DecisionDecline}}
	srv := testServer()
	srv.quotes.strategy = strategy
	request := validQuoteBody()
	discountID := "0x00000000000000000000000000000000000000000000000000000000000000ab"
	request.Adapters[0].DiscountID = &discountID

	response, err := srv.quotes.quote(t.Context(), &request)
	if err != nil || response != nil {
		t.Fatalf("quote = %+v, err %v; want strategy decline", response, err)
	}
	if len(strategy.quoteInput.Candidates) != 1 {
		t.Fatalf("candidates = %d, want one", len(strategy.quoteInput.Candidates))
	}
	candidate := strategy.quoteInput.Candidates[0]
	// The advertised 1:1 rate is normalized against the on-chain tokenIn decimals (18, not the
	// backend's 0) and then shaved by one output unit — 1e12 rate units at 18→6 — so the candidate can
	// never price above what the adapter pays for the same amountIn.
	if candidate.Route.TokenInDecimals != 18 ||
		candidate.Rate.Cmp(mustBig(t, "999999000000000000")) != 0 ||
		candidate.MaxAmountIn.Cmp(mustBig(t, "10000010000010000010")) != 0 {
		t.Fatalf("candidate = %+v, want the conservative 18-decimal discount rate", candidate)
	}
}

func testPermissionedPolicy(t *testing.T, tokens ...common.Address) tokenpolicy.Policy {
	t.Helper()
	policy, err := tokenpolicy.New(tokenpolicy.Permissioned, tokens)
	if err != nil {
		t.Fatalf("tokenpolicy.New: %v", err)
	}
	return policy
}
