package rfq

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

// mGLOBAL (permissioned) and mF-ONE (permissionless) Hoodi addresses, used as token fixtures.
var (
	permissionedToken   = common.HexToAddress("0x2Ee6f1A395Bce7a7c5bF1D07bAaF9F8A0828A8d3")
	permissionlessToken = common.HexToAddress("0xA684911e92b8E4Dd27046331B849Bbd6dbca0fA2")
)

func TestQuotesTokenInScope(t *testing.T) {
	perm := map[common.Address]bool{permissionedToken: true}

	cases := []struct {
		scope string
		token common.Address
		want  bool
	}{
		{tokensToQuoteAll, permissionedToken, true},
		{tokensToQuoteAll, permissionlessToken, true},
		{tokensToQuotePermissioned, permissionedToken, true},
		{tokensToQuotePermissioned, permissionlessToken, false},
		{tokensToQuotePermissionless, permissionedToken, false},
		{tokensToQuotePermissionless, permissionlessToken, true},
		{"", permissionlessToken, true}, // unset scope behaves like "all"
	}
	for _, c := range cases {
		qs := &quoteService{tokensToQuote: c.scope, permissionedTokens: perm}
		if got := qs.quotesTokenIn(c.token); got != c.want {
			t.Errorf("scope=%q token=%s: got %v, want %v", c.scope, c.token.Hex(), got, c.want)
		}
	}
}

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
	if cfg.TokensToQuote != tokensToQuotePermissioned {
		t.Errorf("TokensToQuote = %q, want %q", cfg.TokensToQuote, tokensToQuotePermissioned)
	}
	if !cfg.PermissionedTokens[permissionedToken] {
		t.Errorf("expected mGLOBAL in PermissionedTokens")
	}

	def, err := parseCfg(t, base)
	if err != nil {
		t.Fatalf("parse default: %v", err)
	}
	if def.TokensToQuote != tokensToQuoteAll {
		t.Errorf("default TokensToQuote = %q, want %q", def.TokensToQuote, tokensToQuoteAll)
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

func TestQuoteMarksPermissionedTokenAsSingleRoute(t *testing.T) {
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
	srv.quotes.permissionedTokens = map[common.Address]bool{permissionedToken: true}
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
