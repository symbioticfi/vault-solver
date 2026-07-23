package rfq

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
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
	strategy := &inputRecordingStrategy{quoteOut: types.QuoteOutput{
		Decision:        types.DecisionQuote,
		QuotedAmountOut: big.NewInt(1_000000),
		Legs: []types.QuoteLeg{{
			CandidateID: "candidate-0",
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
}

func testPermissionedPolicy(t *testing.T, tokens ...common.Address) tokenpolicy.Policy {
	t.Helper()
	policy, err := tokenpolicy.New(tokenpolicy.Permissioned, tokens)
	if err != nil {
		t.Fatalf("tokenpolicy.New: %v", err)
	}
	return policy
}
