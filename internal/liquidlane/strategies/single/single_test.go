package single

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidgas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
)

func source(id string, rate, capacity int64) liquidlane.QuoteCandidate {
	route := liquidlane.Route{ID: liquidlane.RouteID(id), CapacityID: liquidlane.CapacityID(id), TokenOut: common.HexToAddress("0x02")}
	return *greedy.NewQuoteCandidate(liquidlane.DirectInventory(route, big.NewInt(capacity), big.NewInt(rate)), big.NewInt(capacity))
}

func TestSingleQuoteFullAmountAndBestPrice(t *testing.T) {
	const rate = int64(1_000_000_000_000_000_000)
	for _, test := range []struct {
		name       string
		candidates []liquidlane.QuoteCandidate
		output     bool
		wantID     string
	}{
		{"full input", []liquidlane.QuoteCandidate{source("small", 2*rate, 50), source("full", rate, 200)}, false, "full"},
		{"full output", []liquidlane.QuoteCandidate{source("small", 2*rate, 50), source("full", rate, 200)}, true, "full"},
		{"better input price but all capacities insufficient", []liquidlane.QuoteCandidate{source("better-small", 2*rate, 50), source("worse-small", rate, 90)}, false, ""},
		{"better output price but all capacities insufficient", []liquidlane.QuoteCandidate{source("better-small", 2*rate, 50), source("worse-small", rate, 90)}, true, ""},
		{"no aggregation", []liquidlane.QuoteCandidate{source("a", rate, 60), source("b", rate, 60)}, false, ""},
		{"best input price", []liquidlane.QuoteCandidate{source("a", rate, 200), source("b", 2*rate, 300)}, false, "b"},
		{"best output price", []liquidlane.QuoteCandidate{source("a", rate, 200), source("b", 2*rate, 300)}, true, "b"},
		{"stable tie", []liquidlane.QuoteCandidate{source("b", rate, 200), source("a", rate, 200)}, false, "a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			task := strategies.QuoteTask{Candidates: test.candidates, ExactInput: big.NewInt(100), OutputBufferBps: 100}
			if test.output {
				task.ExactInput = nil
				task.ExactOutput = big.NewInt(100)
			}
			result, err := SolveQuote(task)
			if err != nil {
				t.Fatal(err)
			}
			if test.wantID == "" {
				if result != nil {
					t.Fatalf("unexpected quote: %+v", result)
				}
				return
			}
			if result == nil || len(result.Allocations) != 1 || string(result.Allocations[0].Candidate.Route.ID) != test.wantID {
				t.Fatalf("quote = %+v, want source %s", result, test.wantID)
			}
		})
	}
}

func TestSingleQuoteComparesNetGasOutput(t *testing.T) {
	const rate = int64(1_000_000_000_000_000_000)
	direct := source("direct", rate, 2_000_000)
	private := source("private", rate+rate/100, 2_000_000)
	id := common.HexToHash("0x01")
	private.DiscountID = &id
	private.ID = liquidlane.NewCandidateID(private.Route, private.DiscountID)
	pricing, err := strategies.NewGasPricing(big.NewInt(1), direct.Route.TokenOut,
		liquidgas.NewPriceSnapshot(map[common.Address]*big.Int{direct.Route.TokenOut: big.NewInt(rate)}),
		nil, 0, strategies.GasEnvelope{PrivateRouteUnits: 75_000})
	if err != nil {
		t.Fatal(err)
	}
	result, err := SolveQuote(strategies.QuoteTask{Candidates: []liquidlane.QuoteCandidate{private, direct}, ExactInput: big.NewInt(1_000_000), GasPricing: &pricing})
	if err != nil || result == nil || result.Allocations[0].Candidate.ID != direct.ID {
		t.Fatalf("quote = %+v, err = %v; lower gross rate should win after gas", result, err)
	}
}

// The first source always lacks volume, even where its price is better.
func TestSingleFillRequiresFullInputAndPromisedOutput(t *testing.T) {
	for _, test := range []struct {
		name                                    string
		firstOutput, capacity, output, required int64
		wantSolution, wantFill                  bool
	}{
		{"cannot aggregate or absorb", 100, 60, 100, 100, false, false},
		{"exact output", 100, 100, 100, 100, true, true},
		{"better price lacks volume", 200, 100, 100, 90, true, true},
		{"neither source covers input", 200, 60, 100, 90, false, false},
		{"promised output unavailable", 200, 100, 89, 90, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			quotes := []liquidlane.FillQuote{
				{Inventory: liquidlane.Inventory{Route: liquidlane.Route{ID: "small", CapacityID: "small"}, MaxAssets: big.NewInt(60)},
					AmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(test.firstOutput)},
				{Inventory: liquidlane.Inventory{Route: liquidlane.Route{ID: "full", CapacityID: "full"}, MaxAssets: big.NewInt(test.capacity)},
					AmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(test.output)},
			}
			solution, err := SolveFill(strategies.FillTask{
				AmountIn: big.NewInt(100), Quotes: quotes, InputPolicy: strategies.AbsorbUncoveredInput,
			})
			if err != nil || (solution != nil) != test.wantSolution {
				t.Fatalf("solution=%v, err=%v, want solution=%t", solution, err, test.wantSolution)
			}
			routes := solution.Finalize(big.NewInt(test.required))
			if (len(routes) != 0) != test.wantFill {
				t.Fatalf("routes=%+v, want fill=%t", routes, test.wantFill)
			}
			if test.wantFill && (len(routes) != 1 || routes[0].RouteID != "full" || routes[0].AmountIn.Int64() != 100) {
				t.Fatalf("expected one full-volume source: %+v", routes)
			}
		})
	}
}
