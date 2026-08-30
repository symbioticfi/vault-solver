package strategies

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

func TestFillPlanFromQuote(t *testing.T) {
	tokenIn := common.HexToAddress("0x01")
	tokenOut := common.HexToAddress("0x02")
	adapter := common.HexToAddress("0x03")
	route := liquidlane.NewRoute(1, adapter, common.HexToAddress("0x04"), tokenIn, tokenOut, 18, 18)
	candidateID := liquidlane.NewCandidateID(route, nil)
	input := types.QuoteInput{
		RequestID: "request", QuoteID: "quote", TokenIn: tokenIn, TokenOut: tokenOut,
		AmountIn: big.NewInt(100), RequiredAmountOut: big.NewInt(90),
		Candidates: []liquidlane.QuoteCandidate{{
			ID: candidateID, Route: route, Rate: big.NewInt(1_000_000_000_000_000_000),
			MaxAmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		}},
	}
	out := types.QuoteOutput{
		Decision: types.DecisionQuote, QuotedAmountOut: big.NewInt(95),
		Legs: []types.QuoteLeg{{CandidateID: string(candidateID), AmountIn: big.NewInt(100), AmountOut: big.NewInt(95)}},
	}

	plan, err := FillPlanFromQuote(input, out)
	if err != nil {
		t.Fatalf("FillPlanFromQuote: %v", err)
	}
	if plan == nil || plan.TokenIn != tokenIn || len(plan.Legs) != 1 ||
		plan.Legs[0].Adapter != adapter || plan.Legs[0].AmountIn.Cmp(input.AmountIn) != 0 ||
		plan.Legs[0].AmountOut.Cmp(out.QuotedAmountOut) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestFillPlanFromQuoteRejectsInconsistentDecision(t *testing.T) {
	tokenIn := common.HexToAddress("0x01")
	tokenOut := common.HexToAddress("0x02")
	adapter := common.HexToAddress("0x03")
	route := liquidlane.NewRoute(1, adapter, common.HexToAddress("0x04"), tokenIn, tokenOut, 18, 18)
	candidateID := liquidlane.NewCandidateID(route, nil)
	input := types.QuoteInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100),
		Candidates: []liquidlane.QuoteCandidate{{
			ID: candidateID, Route: route, Rate: big.NewInt(1_000_000_000_000_000_000),
			MaxAmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		}},
	}
	tests := []struct {
		name string
		out  types.QuoteOutput
	}{
		{
			name: "unknown candidate",
			out: types.QuoteOutput{
				Decision: types.DecisionQuote, QuotedAmountOut: big.NewInt(90),
				Legs: []types.QuoteLeg{{CandidateID: "unknown", AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}},
			},
		},
		{
			name: "input sum",
			out: types.QuoteOutput{
				Decision: types.DecisionQuote, QuotedAmountOut: big.NewInt(90),
				Legs: []types.QuoteLeg{{CandidateID: string(candidateID), AmountIn: big.NewInt(99), AmountOut: big.NewInt(90)}},
			},
		},
		{
			name: "output capacity",
			out: types.QuoteOutput{
				Decision: types.DecisionQuote, QuotedAmountOut: big.NewInt(101),
				Legs: []types.QuoteLeg{{CandidateID: string(candidateID), AmountIn: big.NewInt(100), AmountOut: big.NewInt(101)}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := FillPlanFromQuote(input, test.out); err == nil {
				t.Fatal("FillPlanFromQuote returned nil error")
			}
		})
	}
}

func TestFillPlanFromQuoteRejectsOutputAboveCandidateRate(t *testing.T) {
	tokenIn := common.HexToAddress("0x01")
	tokenOut := common.HexToAddress("0x02")
	route := liquidlane.NewRoute(
		1, common.HexToAddress("0x03"), common.HexToAddress("0x04"), tokenIn, tokenOut, 18, 18,
	)
	candidateID := liquidlane.NewCandidateID(route, nil)
	input := types.QuoteInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100),
		Candidates: []liquidlane.QuoteCandidate{{
			ID: candidateID, Route: route, Rate: big.NewInt(500_000_000_000_000_000),
			MaxAmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		}},
	}
	out := types.QuoteOutput{
		Decision: types.DecisionQuote, QuotedAmountOut: big.NewInt(90),
		Legs: []types.QuoteLeg{{
			CandidateID: string(candidateID), AmountIn: big.NewInt(100), AmountOut: big.NewInt(90),
		}},
	}

	if _, err := FillPlanFromQuote(input, out); err == nil {
		t.Fatal("FillPlanFromQuote returned nil error")
	}
}

func TestFillPlanFromQuoteRejectsRepeatedRoute(t *testing.T) {
	tokenIn := common.HexToAddress("0x01")
	tokenOut := common.HexToAddress("0x02")
	vault := common.HexToAddress("0x04")
	first := liquidlane.NewRoute(1, common.HexToAddress("0x11"), vault, tokenIn, tokenOut, 18, 18)
	discountID := common.HexToHash("0x01")
	directID := liquidlane.NewCandidateID(first, nil)
	privateID := liquidlane.NewCandidateID(first, &discountID)
	input := types.QuoteInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100),
		Candidates: []liquidlane.QuoteCandidate{
			{ID: directID, Route: first, Rate: big.NewInt(1_000_000_000_000_000_000), MaxAmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(100)},
			{ID: privateID, Route: first, Rate: big.NewInt(1_000_000_000_000_000_000), MaxAmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(100), DiscountID: &discountID},
		},
	}
	legs := []types.QuoteLeg{
		{CandidateID: string(directID), AmountIn: big.NewInt(50), AmountOut: big.NewInt(50)},
		{CandidateID: string(privateID), AmountIn: big.NewInt(50), AmountOut: big.NewInt(50)},
	}
	out := types.QuoteOutput{Decision: types.DecisionQuote, Legs: legs, QuotedAmountOut: big.NewInt(100)}
	if _, err := FillPlanFromQuote(input, out); err == nil {
		t.Fatal("FillPlanFromQuote returned nil error")
	}
}
