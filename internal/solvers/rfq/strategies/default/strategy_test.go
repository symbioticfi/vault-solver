package defaultstrategy

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

var (
	tIn  = common.HexToAddress("0x0000000000000000000000000000000000000001")
	tOut = common.HexToAddress("0x0000000000000000000000000000000000000002")
	vlt  = common.HexToAddress("0x0000000000000000000000000000000000000003")
)

func quoteCandidate(
	adapter common.Address,
	rate int64,
	maxInput int64,
	maxOutput int64,
	discountID *common.Hash,
) liquidlane.QuoteCandidate {
	route := liquidlane.NewRoute(1, adapter, common.HexToAddress("0x10"), tIn, tOut, 0, 0)
	route.CapacityID = liquidlane.CapacityID(route.ID)
	return liquidlane.QuoteCandidate{
		ID:           liquidlane.NewCandidateID(route, discountID),
		Route:        route,
		Rate:         new(big.Int).Mul(big.NewInt(rate), big.NewInt(1_000_000_000_000_000_000)),
		MaxAmountIn:  big.NewInt(maxInput),
		MaxAmountOut: big.NewInt(maxOutput),
		DiscountID:   liquidlane.CloneHash(discountID),
	}
}

func baseInput(candidates ...liquidlane.QuoteCandidate) types.QuoteInput {
	return types.QuoteInput{
		RequestID: "r", QuoteID: "q", ChainID: 1,
		Executor: common.HexToAddress("0x0000000000000000000000000000000000000010"),
		TokenIn:  tIn, TokenOut: tOut, AmountIn: big.NewInt(100),
		Candidates: candidates, Now: time.Unix(0, 0),
	}
}

func TestStrategyQuotesNormalizedLiquidLaneCandidates(t *testing.T) {
	candidate := quoteCandidate(vlt, 2, 100, 200, nil)
	got, err := New().DecideQuote(t.Context(), baseInput(candidate))
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if got.Decision != types.DecisionQuote || got.QuotedAmountOut.Cmp(big.NewInt(200)) != 0 ||
		len(got.Legs) != 1 || got.Legs[0].CandidateID != string(candidate.ID) {
		t.Fatalf("output = %+v, want one 200-output leg", got)
	}
}

func TestStrategyAggregatesRoutesButHonorsSingleRoute(t *testing.T) {
	first := quoteCandidate(vlt, 2, 60, 120, nil)
	second := quoteCandidate(common.HexToAddress("0x04"), 1, 100, 100, nil)

	got, err := New().DecideQuote(t.Context(), baseInput(first, second))
	if err != nil || got.Decision != types.DecisionQuote || len(got.Legs) != 2 ||
		got.QuotedAmountOut.Cmp(big.NewInt(160)) != 0 {
		t.Fatalf("aggregate quote = %+v, err %v", got, err)
	}

	input := baseInput(first, second)
	input.RequireSingleRoute = true
	got, err = New().DecideQuote(t.Context(), input)
	if err != nil || got.Decision != types.DecisionQuote || len(got.Legs) != 1 ||
		got.QuotedAmountOut.Cmp(big.NewInt(120)) != 0 ||
		got.Legs[0].CandidateID != string(first.ID) {
		t.Fatalf("single-route quote = %+v, err %v", got, err)
	}
}

func TestStrategySingleRouteQuotesCappedOutputWhenInputExceedsCapacity(t *testing.T) {
	candidate := quoteCandidate(vlt, 2, 60, 120, nil)
	input := baseInput(candidate)
	input.RequireSingleRoute = true

	got, err := New().DecideQuote(t.Context(), input)
	if err != nil || got.Decision != types.DecisionQuote ||
		got.QuotedAmountOut.Cmp(big.NewInt(120)) != 0 || len(got.Legs) != 1 ||
		got.Legs[0].AmountIn.Cmp(big.NewInt(100)) != 0 ||
		got.Legs[0].AmountOut.Cmp(big.NewInt(120)) != 0 {
		t.Fatalf("single-route capped quote = %+v, err %v", got, err)
	}
}

func TestStrategyTreatsDirectAndDiscountAsRouteAlternatives(t *testing.T) {
	discountID := common.HexToHash("0x01")
	private := quoteCandidate(vlt, 3, 60, 180, &discountID)
	direct := quoteCandidate(vlt, 2, 100, 200, nil)

	got, err := New().DecideQuote(t.Context(), baseInput(private, direct))
	if err != nil || got.Decision != types.DecisionQuote || len(got.Legs) != 1 ||
		got.Legs[0].CandidateID != string(direct.ID) {
		t.Fatalf("quote = %+v, err %v; want full direct alternative", got, err)
	}
}

func TestBuildFillPlanUsesTypedCandidateWithoutRepricing(t *testing.T) {
	discountID := common.HexToHash("0x01")
	candidate := quoteCandidate(vlt, 2, 50, 100, &discountID)
	candidate.ValidUntil = time.Unix(2_000_000_000, 0)
	input := baseInput(candidate)
	input.RequiredAmountOut = big.NewInt(100)

	plan, err := New().BuildFillPlan(t.Context(), input)
	if err != nil || plan == nil || len(plan.Legs) != 1 {
		t.Fatalf("plan = %+v, err %v", plan, err)
	}
	leg := plan.Legs[0]
	if leg.Adapter != vlt || leg.AmountIn.Cmp(big.NewInt(100)) != 0 ||
		leg.AmountOut.Cmp(big.NewInt(100)) != 0 ||
		leg.MaxRate.Cmp(big.NewInt(2_000_000_000_000_000_000)) != 0 ||
		leg.DiscountID == nil || *leg.DiscountID != discountID {
		t.Fatalf("leg = %+v", leg)
	}
	if leg.CandidateID != candidate.ID || leg.Route != candidate.Route ||
		!leg.ValidUntil.Equal(candidate.ValidUntil) {
		t.Fatalf("leg identity = %+v, want candidate %+v", leg, candidate)
	}
}

func TestBuildFillPlanSingleRouteKeepsCappedQuoteWhenInputExceedsCapacity(t *testing.T) {
	candidate := quoteCandidate(vlt, 2, 60, 120, nil)
	input := baseInput(candidate)
	input.RequireSingleRoute = true
	input.RequiredAmountOut = big.NewInt(120)

	plan, err := New().BuildFillPlan(t.Context(), input)
	if err != nil || plan == nil || plan.QuotedAmountOut.Cmp(big.NewInt(120)) != 0 ||
		len(plan.Legs) != 1 || plan.Legs[0].AmountIn.Cmp(big.NewInt(100)) != 0 ||
		plan.Legs[0].AmountOut.Cmp(big.NewInt(120)) != 0 {
		t.Fatalf("single-route capped plan = %+v, err %v", plan, err)
	}
}

func TestBuildFillPlanRejectsNonCanonicalCandidateID(t *testing.T) {
	candidate := quoteCandidate(vlt, 2, 100, 200, nil)
	candidate.ID = "wrong"
	plan, err := New().BuildFillPlan(t.Context(), baseInput(candidate))
	if err == nil || plan != nil {
		t.Fatalf("plan = %+v, err %v; want invalid identity rejection", plan, err)
	}
}
