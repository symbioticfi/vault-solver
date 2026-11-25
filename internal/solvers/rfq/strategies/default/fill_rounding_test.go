package defaultstrategy

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

// A single-route award quoted at the adapter's exact payout must still plan at fill time. Values are
// the mainnet mF-ONE -> USDC award on the kpk adapter that a floored rate priced one unit short.
func TestBuildFillPlanSettlesAwardQuotedAtExactPayout(t *testing.T) {
	route := liquidlane.NewRoute(1, common.HexToAddress("0x97c0baefcf688a8389108a0893144ae0a1408b3a"),
		common.HexToAddress("0x8bcd746976885b5832bad07b4921e3f2dd1d3703"), tIn, tOut, 18, 6)
	amountIn, _ := new(big.Int).SetString("147980002963297794849932", 10)
	price := big.NewInt(1_139_099_520_000_000_000)
	discount := big.NewInt(7_000)
	gross := liquidlane.AmountOutForRate(amountIn, price, 18, 6)
	required := liquidlane.AmountOutAfterDiscount(gross, discount)
	if gross.String() != "168563950345" || required.String() != "167384002692" {
		t.Fatalf("fixture drifted: gross %s, payout %s", gross, required)
	}
	capacity := big.NewInt(199_988_609_005)
	source := liquidlane.DiscountInventory(route, capacity, nil, common.HexToHash("0xd"), time.Unix(1<<40, 0))
	source.Discount = discount
	physical := liquidlane.FillQuote{
		Inventory: liquidlane.DirectInventory(route, capacity, nil), AmountIn: amountIn,
		GrossAmountOut: gross, MaxAmountOut: required, MinDiscount: discount,
	}
	candidates := liquidgreedy.NormalizeOracleInventory(amountIn, []liquidlane.Inventory{source}, []liquidlane.FillQuote{physical}, nil)
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d, want 1", len(candidates))
	}

	plan, err := New().BuildFillPlan(t.Context(), types.FillInput{
		RequestID: "r", QuoteID: "q", ChainID: 1, TokenIn: tIn, TokenOut: tOut, AmountIn: amountIn,
		Candidates: candidates, RequiredAmountOut: required, RequireSingleRoute: true, Now: time.Unix(0, 0),
	})
	if err != nil {
		t.Fatalf("BuildFillPlan: %v", err)
	}
	if plan == nil || len(plan.Legs) != 1 || plan.QuotedAmountOut.Cmp(required) != 0 ||
		plan.Legs[0].AmountIn.Cmp(amountIn) != 0 || plan.Legs[0].AmountOut.Cmp(required) != 0 {
		t.Fatalf("plan = %+v, want one leg paying %s for %s", plan, required, amountIn)
	}
}

func TestBuildFillPlanPricesSplitLegsAtAdapterPayout(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		amount, price, gross   int64
		cap, legIn, legOut     int64
		otherPrice, otherGross int64
		total                  int64
		private                bool
	}{
		{"direct", 7, 1_900_000_000_000_000_000, 13, 5, 3, 4, 1_000_000_000_000_000_000, 7, 8, false},
		{"discount", 4, 1_350_000_000_000_000_000, 5, 2, 2, 1, 750_000_000_000_000_000, 3, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			amountIn := big.NewInt(tc.amount)
			first := liquidlane.NewRoute(1, vlt, vlt, tIn, tOut, 6, 6)
			other := common.HexToAddress("0x04")
			second := liquidlane.NewRoute(1, other, other, tIn, tOut, 6, 6)
			left := liquidlane.DirectInventory(first, big.NewInt(tc.cap), nil)
			right := liquidlane.DirectInventory(second, big.NewInt(10), nil)
			left.Price, left.AdapterMinDiscount = big.NewInt(tc.price), big.NewInt(7_000)
			right.Price, right.AdapterMinDiscount = big.NewInt(tc.otherPrice), new(big.Int)
			source := left
			if tc.private {
				id := common.HexToHash("0xd")
				source.DiscountID, source.Discount = &id, left.AdapterMinDiscount
			}
			candidates := liquidgreedy.NormalizeOracleInventory(amountIn, []liquidlane.Inventory{source, right}, []liquidlane.FillQuote{
				{Inventory: left, AmountIn: amountIn, GrossAmountOut: big.NewInt(tc.gross),
					MaxAmountOut: big.NewInt(tc.gross - 1), MinDiscount: left.AdapterMinDiscount},
				{Inventory: right, AmountIn: amountIn, GrossAmountOut: big.NewInt(tc.otherGross),
					MaxAmountOut: big.NewInt(tc.otherGross), MinDiscount: right.AdapterMinDiscount},
			}, nil)
			input := baseInput(candidates...)
			input.AmountIn, input.RequiredAmountOut = amountIn, big.NewInt(tc.total)
			plan, err := New().BuildFillPlan(t.Context(), input)
			if err != nil || plan == nil || len(plan.Legs) != 2 {
				t.Fatalf("expected two-leg fill, got %+v, %v", plan, err)
			}
			leg := plan.Legs[0]
			if leg.Adapter != vlt || leg.AmountIn.Int64() != tc.legIn || leg.AmountOut.Int64() != tc.legOut ||
				plan.QuotedAmountOut.Int64() != tc.total {
				t.Fatalf("first leg = %+v, total = %s; want input %d, output %d, total %d",
					leg, plan.QuotedAmountOut, tc.legIn, tc.legOut, tc.total)
			}
		})
	}
}

// A fill that cannot be planned says which condition failed instead of returning no plan silently.
func TestBuildFillPlanNamesTheFailedCondition(t *testing.T) {
	candidate := quoteCandidate(vlt, 2, 100, 200, nil)
	for name, tc := range map[string]struct {
		input types.FillInput
		want  string
	}{
		"no candidates": {baseInput(), "no fill candidates"},
		"below required": {func() types.FillInput {
			in := baseInput(candidate)
			in.RequireSingleRoute, in.RequiredAmountOut = true, big.NewInt(201)
			return in
		}(), "fill output 200 is below required amount out 201"},
	} {
		t.Run(name, func(t *testing.T) {
			plan, err := New().BuildFillPlan(t.Context(), tc.input)
			if plan != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("plan %+v, err %v; want error containing %q", plan, err, tc.want)
			}
		})
	}
}
