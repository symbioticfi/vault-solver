package planning

import (
	"math/big"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

func TestSolveQuoteRejectsOrAbsorbsUncoveredInput(t *testing.T) {
	candidates := []Candidate{candidate("only", "route", 100, 60)}
	var declineReason string
	strict, err := NewQuotePool(candidates).Solve(QuoteTask{
		ExactInput: big.NewInt(100), MaxRoutes: 1,
		Trace: func(_ string, fields ...any) {
			declineReason, _ = fields[1].(string)
		},
	})
	if err != nil || strict != nil {
		t.Fatalf("strict = %+v, err %v", strict, err)
	}
	if declineReason != insufficientCapacityReason {
		t.Fatalf("decline reason = %q", declineReason)
	}
	absorbed, err := NewQuotePool(candidates).Solve(QuoteTask{
		ExactInput: big.NewInt(100), MaxRoutes: 1,
		InputPolicy: AbsorbUncoveredInput,
	})
	if err != nil || absorbed == nil || absorbed.AmountOut.Int64() != 60 ||
		len(absorbed.Allocations) != 1 || absorbed.Allocations[0].AmountIn.Int64() != 100 {
		t.Fatalf("absorbed = %+v, err %v", absorbed, err)
	}
}

func TestSolveQuoteStrictUsesRouteThatCanCoverLastSlot(t *testing.T) {
	narrow := candidate("narrow", "route-1", 200, 60)
	wide := candidate("wide", "route-2", 100, 100)
	quote, err := NewQuotePool([]Candidate{narrow, wide}).Solve(QuoteTask{
		ExactInput: big.NewInt(100), MaxRoutes: 1,
		InputPolicy: RejectUncoveredInput,
	})
	if err != nil || quote == nil || len(quote.Allocations) != 1 ||
		quote.Allocations[0].Candidate.ID != "wide" {
		t.Fatalf("quote = %+v, err %v; want the route that covers the full input", quote, err)
	}
}

func TestSolveQuoteAppliesBufferAndFindsExactOutputInput(t *testing.T) {
	candidates := []Candidate{candidate("only", "route", 100, 1_000)}
	exactInput, err := NewQuotePool(candidates).Solve(QuoteTask{
		ExactInput: big.NewInt(1_000), MaxRoutes: 1,
		OutputBufferBps: 200,
	})
	if err != nil || exactInput == nil || exactInput.GrossAmountOut.Int64() != 1_000 ||
		exactInput.GasCost.Sign() != 0 || exactInput.AmountOut.Int64() != 980 {
		t.Fatalf("exact input = %+v, err %v", exactInput, err)
	}
	exactOutput, err := NewQuotePool(candidates).Solve(QuoteTask{
		ExactOutput: big.NewInt(980), MaxRoutes: 1,
		OutputBufferBps: 200,
	})
	if err != nil || exactOutput == nil || exactOutput.AmountIn.Int64() != 1_000 ||
		exactOutput.AmountOut.Int64() != 980 {
		t.Fatalf("exact output = %+v, err %v", exactOutput, err)
	}
}

func TestSolveQuoteExactOutputKeepsRoundingSurplus(t *testing.T) {
	discountID := common.HexToHash("0x01")
	private := candidate("private", "route", 120, 100)
	private.DiscountID = &discountID
	direct := candidate("direct", "route", 100, 1_000)

	quote, err := NewQuotePool([]Candidate{private, direct}).Solve(QuoteTask{
		ExactOutput: big.NewInt(119), MaxRoutes: 1,
	})
	if err != nil || quote == nil || quote.AmountIn.Int64() != 100 || quote.AmountOut.Int64() != 119 ||
		len(quote.Allocations) != 1 || quote.Allocations[0].AmountOut.Int64() != 120 {
		t.Fatalf("quote = %+v, err %v; want input 100, user output 119, gross output 120", quote, err)
	}
}

func TestSolveQuoteExactOutputDoesNotDeclineAtWiderWorseAlternative(t *testing.T) {
	discountID := common.HexToHash("0x01")
	private := candidate("private", "route", 200, 100)
	private.DiscountID = &discountID
	direct := candidate("direct", "route", 10, 1_000)

	quote, err := NewQuotePool([]Candidate{private, direct}).Solve(QuoteTask{
		ExactOutput: big.NewInt(150), MaxRoutes: 1,
	})
	if err != nil || quote == nil || quote.AmountIn.Int64() != 75 || quote.AmountOut.Int64() != 150 {
		t.Fatalf("quote = %+v, err %v; want the narrow private alternative", quote, err)
	}
}

func TestSolveQuoteExactOutputUsesWiderAlternativeWhenPrivateCannotCover(t *testing.T) {
	discountID := common.HexToHash("0x01")
	private := candidate("private", "route", 200, 100)
	private.DiscountID = &discountID
	direct := candidate("direct", "route", 100, 1_000)

	quote, err := NewQuotePool([]Candidate{private, direct}).Solve(QuoteTask{
		ExactOutput: big.NewInt(250), MaxRoutes: 1,
	})
	if err != nil || quote == nil || quote.AmountIn.Int64() != 250 ||
		len(quote.Allocations) != 1 || quote.Allocations[0].Candidate.ID != "direct" {
		t.Fatalf("quote = %+v, err %v; want wider direct alternative", quote, err)
	}
}

func TestSolveQuoteExactInputUsesWiderPrivateAlternative(t *testing.T) {
	narrowDiscountID := common.HexToHash("0x01")
	wideDiscountID := common.HexToHash("0x02")
	narrow := candidate("narrow-private", "route", 200, 40)
	narrow.DiscountID = &narrowDiscountID
	wide := candidate("wide-private", "route", 100, 100)
	wide.DiscountID = &wideDiscountID

	quote, err := NewQuotePool([]Candidate{narrow, wide}).Solve(QuoteTask{
		ExactInput: big.NewInt(100), MaxRoutes: 1,
		InputPolicy: RejectUncoveredInput,
	})
	if err != nil || quote == nil || len(quote.Allocations) != 1 ||
		quote.Allocations[0].Candidate.ID != "wide-private" ||
		quote.AmountOut.Int64() != 100 {
		t.Fatalf("quote = %+v, err %v; want the wider private alternative", quote, err)
	}
}

func TestSolveQuoteExactOutputUsesNarrowPrivateAfterAnotherRoute(t *testing.T) {
	discountID := common.HexToHash("0x01")
	private := candidate("private", "route-1", 200, 100)
	private.DiscountID = &discountID
	direct := candidate("direct", "route-1", 100, 1_000)
	second := candidate("second", "route-2", 150, 100)

	quote, err := NewQuotePool([]Candidate{private, direct, second}).Solve(QuoteTask{
		ExactOutput: big.NewInt(250), MaxRoutes: 2,
	})
	if err != nil || quote == nil || quote.AmountIn.Int64() != 150 || len(quote.Allocations) != 2 ||
		quote.Allocations[0].Candidate.ID != "second" || quote.Allocations[1].Candidate.ID != "private" {
		t.Fatalf("quote = %+v, err %v; want second route then narrow private alternative", quote, err)
	}
}

func TestSolveQuoteExactOutputCanUseMinimumInputAsSurplus(t *testing.T) {
	quote, err := NewQuotePool([]Candidate{candidate("only", "route", 200, 100)}).Solve(QuoteTask{
		ExactOutput: big.NewInt(100), MinInput: big.NewInt(80),
		MaxRoutes: 1,
	})
	if err != nil || quote == nil || quote.AmountIn.Int64() != 80 || quote.AmountOut.Int64() != 100 ||
		len(quote.Allocations) != 1 || quote.Allocations[0].AmountOut.Int64() != 160 {
		t.Fatalf("quote = %+v, err %v; want minimum input with gross surplus", quote, err)
	}
}

func TestSolveQuoteRejectsAmbiguousExactOutputPolicy(t *testing.T) {
	_, err := NewQuotePool([]Candidate{candidate("only", "route", 100, 1)}).Solve(QuoteTask{
		ExactOutput: big.NewInt(1),
		MaxRoutes:   1, InputPolicy: AbsorbUncoveredInput,
	})
	if err == nil {
		t.Fatal("expected exact-output policy error")
	}
}

func FuzzSolveQuoteExactOutputFindsNoMoreInputThanExactInput(f *testing.F) {
	f.Add(uint32(1_000), uint16(125), uint16(200))
	f.Fuzz(func(t *testing.T, rawAmount uint32, rawRate, rawBuffer uint16) {
		amount := int64(rawAmount%1_000_000 + 1)
		rate := int64(rawRate%200 + 1)
		buffer := int(rawBuffer % 1_000)
		candidates := []Candidate{candidate("only", "route", rate, amount)}
		exactInput, err := NewQuotePool(candidates).Solve(QuoteTask{
			ExactInput: big.NewInt(amount), MaxRoutes: 1,
			OutputBufferBps: buffer,
		})
		testcheck.NoError(t, err)
		if exactInput == nil {
			return
		}
		exactOutput, err := NewQuotePool(candidates).Solve(QuoteTask{
			ExactOutput: exactInput.AmountOut, MaxRoutes: 1,
			OutputBufferBps: buffer,
		})
		if err != nil || exactOutput == nil {
			t.Fatalf("exact output = %+v, err %v", exactOutput, err)
		}
		if exactOutput.AmountIn.Cmp(exactInput.AmountIn) > 0 ||
			exactOutput.AmountOut.Cmp(exactInput.AmountOut) != 0 {
			t.Fatalf("exact input = %+v, exact output = %+v", exactInput, exactOutput)
		}
	})
}

// Reusing a prepared pool must not retain a previous amount or expose mutable
// result amounts to another query. The race gate also exercises parallel readers.
func TestQuotePoolQueriesOwnTheirAmounts(t *testing.T) {
	t.Parallel()
	candidate := candidate("only", "route", 200, 100)
	pool := NewQuotePool([]Candidate{candidate})
	for amount := int64(1); amount <= 12; amount++ {
		t.Run(strconv.FormatInt(amount, 10), func(t *testing.T) {
			t.Parallel()
			input := big.NewInt(amount)
			task := QuoteTask{ExactInput: input, MaxRoutes: 1}
			for range 2 {
				result, err := pool.Solve(task)
				if err != nil || result == nil || result.AmountIn.Cmp(input) != 0 || result.AmountOut.Cmp(big.NewInt(2*amount)) != 0 {
					t.Fatalf("amount %d: %+v, %v", amount, result, err)
				}
				result.AmountIn.SetInt64(0)
				result.AmountOut.SetInt64(0)
				result.Allocations[0].AmountIn.SetInt64(0)
				result.Allocations[0].AmountOut.SetInt64(0)
			}
		})
	}
}
