package defaultstrategy

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

func mustBig(t *testing.T, s string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad big.Int %q", s)
	}
	return n
}

var (
	tIn  = common.HexToAddress("0x0000000000000000000000000000000000000001")
	tOut = common.HexToAddress("0x0000000000000000000000000000000000000002")
	vlt  = common.HexToAddress("0x0000000000000000000000000000000000000003")
)

type fakePricing struct {
	out     map[common.Address]*big.Int
	queries *[][]types.QuoteCandidate
}

func (f fakePricing) TokenDecimals(context.Context, common.Address) (int, error) {
	return 18, nil
}

func (f fakePricing) AmountsOut(
	_ context.Context,
	_ common.Address,
	candidates []types.QuoteCandidate,
	_ *big.Int,
) (map[common.Address]*big.Int, error) {
	if f.queries != nil {
		*f.queries = append(*f.queries, append([]types.QuoteCandidate(nil), candidates...))
	}
	return f.out, nil
}

func baseInput(t *testing.T, candidates []types.QuoteCandidate) types.QuoteInput {
	t.Helper()
	return types.QuoteInput{
		RequestID:  "r",
		QuoteID:    "q",
		ChainID:    1,
		Executor:   common.HexToAddress("0x0000000000000000000000000000000000000010"),
		TokenIn:    tIn,
		TokenOut:   tOut,
		AmountIn:   mustBig(t, "1000000000000000000"),
		Candidates: candidates,
		Now:        time.Unix(0, 0),
	}
}

func TestStrategyDirectFill(t *testing.T) {
	input := baseInput(t, []types.QuoteCandidate{{
		ID: "c0", Adapter: vlt, Asset: tOut, AssetDecimals: 6,
		MaxAssets: mustBig(t, "10000000"),
		MaxRate:   mustBig(t, "1000000000000000000"),
	}})
	got, err := New(fakePricing{out: map[common.Address]*big.Int{tOut: mustBig(t, "1000000")}}).DecideQuote(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if got.Decision != types.DecisionQuote || got.QuotedAmountOut.String() != "1000000" {
		t.Fatalf("output = %+v, want 1.000000 quote", got)
	}
	if len(got.Legs) != 1 {
		t.Fatalf("legs = %d, want 1", len(got.Legs))
	}
	if got.Legs[0].CandidateID != "c0" ||
		got.Legs[0].AmountIn.String() != "1000000000000000000" ||
		got.Legs[0].AmountOut.String() != "1000000" {
		t.Fatalf("leg = %+v, want c0 with full input and 1000000 output", got.Legs[0])
	}
}

func TestStrategyRejectsMaxRateBelowPrivateRate(t *testing.T) {
	input := baseInput(t, []types.QuoteCandidate{{
		ID: "c0", Adapter: vlt, Asset: tOut, AssetDecimals: 6,
		MaxAssets: mustBig(t, "10000000"),
		MaxRate:   mustBig(t, "800000000000000000"),
	}})
	got, err := New(fakePricing{out: map[common.Address]*big.Int{tOut: mustBig(t, "1000000")}}).DecideQuote(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if got.Decision != types.DecisionDecline {
		t.Fatalf("decision = %q, want decline", got.Decision)
	}
}

func TestStrategyAssetMustEqualTokenOut(t *testing.T) {
	other := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	input := baseInput(t, []types.QuoteCandidate{{
		ID: "c0", Adapter: vlt, Asset: other, AssetDecimals: 6,
		MaxAssets: mustBig(t, "10000000"), MaxRate: mustBig(t, "1000000000000000000"),
	}})
	got, err := New(fakePricing{out: map[common.Address]*big.Int{tOut: mustBig(t, "1000000")}}).DecideQuote(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if got.Decision != types.DecisionDecline {
		t.Fatalf("decision = %q, want decline", got.Decision)
	}
}

func TestStrategyPricesAndSelectsOnlyMatchingCandidates(t *testing.T) {
	other := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	input := baseInput(t, []types.QuoteCandidate{
		{
			ID: "wrong", Adapter: vlt, Asset: other, AssetDecimals: 6,
			MaxAssets: mustBig(t, "10000000"), MaxRate: mustBig(t, "1000000000000000000"),
		},
		{
			ID: "match", Adapter: vlt, Asset: tOut, AssetDecimals: 6,
			MaxAssets: mustBig(t, "10000000"), MaxRate: mustBig(t, "1000000000000000000"),
		},
	})
	var queries [][]types.QuoteCandidate
	got, err := New(fakePricing{
		out:     map[common.Address]*big.Int{tOut: mustBig(t, "1000000")},
		queries: &queries,
	}).DecideQuote(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if got.Decision != types.DecisionQuote || len(got.Legs) != 1 || got.Legs[0].CandidateID != "match" {
		t.Fatalf("output = %+v, want quote through matching candidate", got)
	}
	if len(queries) != 1 || len(queries[0]) != 1 || queries[0][0].ID != "match" {
		t.Fatalf("pricing candidates = %+v, want only matching candidate", queries)
	}
}

func TestStrategyNoOraclePriceSkips(t *testing.T) {
	input := baseInput(t, []types.QuoteCandidate{{
		ID: "c0", Adapter: vlt, Asset: tOut, AssetDecimals: 6,
		MaxAssets: mustBig(t, "10000000"), MaxRate: mustBig(t, "1000000000000000000"),
	}})
	got, err := New(fakePricing{out: map[common.Address]*big.Int{}}).DecideQuote(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if got.Decision != types.DecisionDecline {
		t.Fatalf("decision = %q, want decline", got.Decision)
	}
}

func TestStrategyDiscountLegUsesMaxRate(t *testing.T) {
	h := common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000ab")
	input := baseInput(t, []types.QuoteCandidate{{
		ID: "c0", Adapter: vlt, Asset: tOut, AssetDecimals: 6,
		MaxAssets:  mustBig(t, "10000000"),
		MaxRate:    mustBig(t, "1000000000000000000"),
		DiscountID: &h,
	}})
	got, err := New(fakePricing{out: map[common.Address]*big.Int{tOut: mustBig(t, "500000")}}).DecideQuote(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if got.Decision != types.DecisionQuote || got.QuotedAmountOut.String() != "1000000" {
		t.Fatalf("output = %+v, want discount quote at maxRate", got)
	}
}

func TestStrategySplitsAcrossDiscountsBestRateFirst(t *testing.T) {
	betterDiscountID := common.HexToHash("0x01")
	worseDiscountID := common.HexToHash("0x02")
	input := baseInput(t, []types.QuoteCandidate{
		{
			ID: "worse", Adapter: common.HexToAddress("0x04"), Asset: tOut, AssetDecimals: 18,
			MaxAssets:  mustBig(t, "2000000000000000000000"),
			MaxRate:    mustBig(t, "880000000000000000"),
			DiscountID: &worseDiscountID,
		},
		{
			ID: "better", Adapter: vlt, Asset: tOut, AssetDecimals: 18,
			MaxAssets:  mustBig(t, "1000000000000000000000"),
			MaxRate:    mustBig(t, "990000000000000000"),
			DiscountID: &betterDiscountID,
		},
	})
	input.AmountIn = mustBig(t, "2000000000000000000000")

	got, err := New(fakePricing{out: map[common.Address]*big.Int{
		tOut: mustBig(t, "2000000000000000000000"),
	}}).DecideQuote(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if got.Decision != types.DecisionQuote || got.QuotedAmountOut.String() != "1871111111111111111110" {
		t.Fatalf("output = %+v, want best-rate-first split totaling 1871111111111111111110", got)
	}
	if len(got.Legs) != 2 {
		t.Fatalf("legs = %d, want 2", len(got.Legs))
	}
	if got.Legs[0].CandidateID != "better" ||
		got.Legs[0].AmountIn.String() != "1010101010101010101011" ||
		got.Legs[0].AmountOut.String() != "1000000000000000000000" {
		t.Fatalf("first leg = %+v, want better discount saturated at maxAssets", got.Legs[0])
	}
	if got.Legs[1].CandidateID != "worse" ||
		got.Legs[1].AmountIn.String() != "989898989898989898989" ||
		got.Legs[1].AmountOut.String() != "871111111111111111110" {
		t.Fatalf("second leg = %+v, want worse discount to consume remaining input", got.Legs[1])
	}
}

func TestStrategyCapsQuoteAtAvailableAssetsWhenCapacityCannotCoverInput(t *testing.T) {
	discountID := common.HexToHash("0x01")
	input := baseInput(t, []types.QuoteCandidate{{
		ID: "c0", Adapter: vlt, Asset: tOut, AssetDecimals: 6,
		MaxAssets:  mustBig(t, "500000"),
		MaxRate:    mustBig(t, "1000000000000000000"),
		DiscountID: &discountID,
	}})
	got, err := New(fakePricing{out: map[common.Address]*big.Int{tOut: mustBig(t, "1000000")}}).DecideQuote(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if got.Decision != types.DecisionQuote || got.QuotedAmountOut.String() != "500000" {
		t.Fatalf("output = %+v, want quote capped at maxAssets 500000", got)
	}
	if len(got.Legs) != 1 ||
		got.Legs[0].AmountIn.Cmp(input.AmountIn) != 0 ||
		got.Legs[0].AmountOut.String() != "500000" {
		t.Fatalf("legs = %+v, want full input quoted for capped output", got.Legs)
	}
}
