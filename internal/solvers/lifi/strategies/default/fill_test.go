package defaultstrategy

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

func TestSolveFillChoosesBestCompleteRoutes(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	quotes := make([]liquidlane.FillQuote, 3)
	for index := range quotes {
		quotes[index] = testFillQuote(
			liquidlane.RouteID(string(rune('a'+index))),
			liquidlane.CapacityID(string(rune('A'+index))),
			tokenIn,
			tokenOut,
			2,
			int64(6+2*index),
			int64(3+index),
			nil,
		)
	}
	solution := solveTestFill(t, tokenIn, tokenOut, big.NewInt(2), quotes, nil, 3)
	if solution == nil {
		t.Fatal("expected fill solution")
	}
	routes := solution.buildRoutes(solution.maxAmountOut)
	if len(routes) != 2 || routes[0].RouteID != "c" || routes[1].RouteID != "b" {
		t.Fatalf("routes = %+v, want c,b", routes)
	}
}

func TestSolveFillUsesDirectWhenPrivateCannotCoverLeg(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	direct := testFillQuote("route", "capacity", tokenIn, tokenOut, 100, 100, 100, nil)
	private := testFillQuote("route", "capacity", tokenIn, tokenOut, 100, 200, 100, &discountID)
	solution := solveTestFill(
		t, tokenIn, tokenOut, big.NewInt(100), []liquidlane.FillQuote{private, direct}, nil, 1,
	)
	if solution == nil {
		t.Fatal("expected fill solution")
	}
	routes := solution.buildRoutes(big.NewInt(100))
	if len(routes) != 1 || routes[0].DiscountID != nil {
		t.Fatalf("routes = %+v, want direct fallback", routes)
	}
}

func TestSolveFillDoesNotOverbookSharedCapacity(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	high := testFillQuote("high", "shared", tokenIn, tokenOut, 75, 150, 100, nil)
	wide := testFillQuote("wide", "shared", tokenIn, tokenOut, 75, 75, 60, nil)
	solution := solveTestFill(
		t, tokenIn, tokenOut, big.NewInt(75), []liquidlane.FillQuote{high, wide}, nil, 2,
	)
	if solution != nil {
		t.Fatalf("solution = %+v, want shared-capacity rejection", solution)
	}
}

func TestMaxInputWithinCapacityIsExact(t *testing.T) {
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	for _, discount := range []*common.Hash{nil, &discountID} {
		strategy := &Strategy{cfg: Config{PriceBufferBps: 1234}}
		candidate := fillCandidate{quote: liquidlane.FillQuote{
			Inventory: liquidlane.Inventory{DiscountID: discount},
			AmountIn:  big.NewInt(137), MaxAmountOut: big.NewInt(233),
		}}
		for capacity := int64(1); capacity <= 233; capacity++ {
			limit := big.NewInt(137)
			got := strategy.maxInputWithinCapacity(candidate, limit, big.NewInt(capacity))
			if strategy.reservedCapacityOutput(candidate, got).Cmp(big.NewInt(capacity)) > 0 {
				t.Fatalf("discount=%v capacity=%d input=%s exceeds capacity", discount != nil, capacity, got)
			}
			if got.Cmp(limit) < 0 {
				next := new(big.Int).Add(got, big.NewInt(1))
				if strategy.reservedCapacityOutput(candidate, next).Cmp(big.NewInt(capacity)) <= 0 {
					t.Fatalf("discount=%v capacity=%d input=%s is not maximal", discount != nil, capacity, got)
				}
			}
		}
	}
}

func solveTestFill(
	t *testing.T,
	tokenIn, tokenOut common.Address,
	amountIn *big.Int,
	quotes []liquidlane.FillQuote,
	reservations map[liquidlane.CapacityID]*big.Int,
	maxRoutes int,
) *fillSolution {
	t.Helper()
	strategy := &Strategy{}
	solution, err := strategy.solveGreedyFill(types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: amountIn,
		Quotes: quotes, Reservations: reservations, MaxFeePerGas: new(big.Int),
	}, time.Time{}, maxRoutes)
	if err != nil {
		t.Fatalf("solveGreedyFill: %v", err)
	}
	return solution
}

func testFillQuote(
	routeID liquidlane.RouteID,
	capacityID liquidlane.CapacityID,
	tokenIn, tokenOut common.Address,
	amountIn, amountOut, maxAssets int64,
	discountID *common.Hash,
) liquidlane.FillQuote {
	return liquidlane.FillQuote{
		Inventory: liquidlane.Inventory{
			Route: liquidlane.Route{
				ID: routeID, CapacityID: capacityID, TokenIn: tokenIn, TokenOut: tokenOut,
			},
			MaxAssets: big.NewInt(maxAssets), DiscountID: discountID,
		},
		AmountIn: big.NewInt(amountIn), MaxAmountOut: big.NewInt(amountOut),
	}
}
