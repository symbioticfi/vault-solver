package planning

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

type fillCandidate = pricedFill

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
	allocation, err := SolveFill(FillTask{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(2), Quotes: quotes, MaxRoutes: 3,
	})
	if err != nil || allocation == nil {
		t.Fatalf("SolveFill = %v, %v", allocation, err)
	}
	routes := allocation.Finalize(allocation.MaxAmountOut())
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
	allocation, err := SolveFill(FillTask{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100),
		Quotes: []liquidlane.FillQuote{private, direct}, MaxRoutes: 1,
	})
	if err != nil || allocation == nil {
		t.Fatalf("SolveFill = %v, %v", allocation, err)
	}
	routes := allocation.Finalize(big.NewInt(100))
	if len(routes) != 1 || routes[0].DiscountID != nil {
		t.Fatalf("routes = %+v, want direct fallback", routes)
	}
}

func TestSolveFillUsesWiderPrivateAlternative(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	narrowDiscountID := common.HexToHash("0x01")
	wideDiscountID := common.HexToHash("0x02")
	narrow := testFillQuote("route", "capacity", tokenIn, tokenOut, 100, 200, 50, &narrowDiscountID)
	wide := testFillQuote("route", "capacity", tokenIn, tokenOut, 100, 100, 100, &wideDiscountID)

	solution, err := SolveFill(FillTask{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100),
		Quotes: []liquidlane.FillQuote{narrow, wide}, MaxRoutes: 1,
	})
	if err != nil || solution == nil {
		t.Fatalf("SolveFill = %v, %v; want the wider private alternative", solution, err)
	}
	routes := solution.Finalize(big.NewInt(100))
	if len(routes) != 1 || routes[0].DiscountID == nil ||
		*routes[0].DiscountID != wideDiscountID {
		t.Fatalf("routes = %+v, want wider private discount %s", routes, wideDiscountID)
	}
}

func TestSolveFillDoesNotOverbookSharedCapacity(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	high := testFillQuote("high", "shared", tokenIn, tokenOut, 75, 150, 100, nil)
	wide := testFillQuote("wide", "shared", tokenIn, tokenOut, 75, 75, 60, nil)
	var declineReason string
	allocation, err := SolveFill(FillTask{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(75),
		Quotes: []liquidlane.FillQuote{high, wide}, MaxRoutes: 2,
		Trace: func(_ string, fields ...any) {
			declineReason, _ = fields[1].(string)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocation != nil {
		t.Fatalf("allocation = %+v, want shared-capacity rejection", allocation)
	}
	if declineReason != insufficientCapacityReason {
		t.Fatalf("decline reason = %q", declineReason)
	}
}

func TestSolveFillAbsorbsUncoveredInputAsPriceImpact(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	quote := testFillQuote("route", "capacity", tokenIn, tokenOut, 100, 100, 60, nil)
	solution, err := SolveFill(FillTask{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100),
		Quotes: []liquidlane.FillQuote{quote}, MaxRoutes: 1,
		InputPolicy: AbsorbUncoveredInput,
	})
	if err != nil || solution == nil || solution.MaxAmountOut().Int64() != 60 {
		t.Fatalf("solution = %+v, err %v", solution, err)
	}
	routes := solution.Finalize(solution.MaxAmountOut())
	if len(routes) != 1 || routes[0].AmountIn.Int64() != 100 ||
		routes[0].ExpectedAmountOut.Int64() != 60 {
		t.Fatalf("routes = %+v", routes)
	}
}

func TestSolveFillGasPricingIsOptional(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	amount := big.NewInt(10_000_000)
	quote := testFillQuote("route", "capacity", tokenIn, tokenOut, amount.Int64(), amount.Int64(), amount.Int64(), nil)
	task := FillTask{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: amount,
		Quotes: []liquidlane.FillQuote{quote}, MaxRoutes: 1,
	}

	withoutGas, err := SolveFill(task)
	if err != nil || withoutGas == nil || withoutGas.MaxAmountOut().Cmp(amount) != 0 {
		t.Fatalf("SolveFill without gas = %v, %v", withoutGas, err)
	}
	gasPricing, err := NewGasPricing(
		big.NewInt(1),
		tokenOut,
		liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{
			tokenOut: big.NewInt(1_000_000_000_000_000_000),
		}),
		nil,
		0,
		GasEnvelope{SettlementUnits: 250_000, PrivateRouteUnits: 75_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	task.GasPricing = &gasPricing
	withGas, err := SolveFill(task)
	if err != nil || withGas == nil {
		t.Fatalf("SolveFill with gas = %v, %v", withGas, err)
	}
	if withGas.MaxAmountOut().Cmp(withoutGas.MaxAmountOut()) >= 0 {
		t.Fatalf(
			"gas-aware output = %s, without gas = %s",
			withGas.MaxAmountOut(),
			withoutGas.MaxAmountOut(),
		)
	}
}

func TestSolveFillRejectsInvalidBps(t *testing.T) {
	_, err := SolveFill(FillTask{AmountIn: big.NewInt(1), Quotes: []liquidlane.FillQuote{{}},
		MaxRoutes: 1, PriceBufferBps: bpsDenominator})
	if err == nil {
		t.Fatal("expected invalid bps error")
	}
}

func TestDistributeMinimumsPreservesTotalAndBounds(t *testing.T) {
	targets := []*big.Int{big.NewInt(200), big.NewInt(800)}
	minimums := distributeMinimums(targets, big.NewInt(503))
	if len(minimums) != 2 || minimums[0].String() != "100" || minimums[1].String() != "403" {
		t.Fatalf("minimums = %v", minimums)
	}
}

func TestMaxInputWithinCapacityIsExact(t *testing.T) {
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	for _, discount := range []*common.Hash{nil, &discountID} {
		candidate := fillCandidate{quote: liquidlane.FillQuote{
			Inventory: liquidlane.Inventory{DiscountID: discount},
			AmountIn:  big.NewInt(137), MaxAmountOut: big.NewInt(233),
		}}
		for capacity := int64(1); capacity <= 233; capacity++ {
			limit := big.NewInt(137)
			got := maxInputWithinCapacity(candidate, limit, big.NewInt(capacity), 1234)
			if reservedCapacityOutput(candidate, got, 1234).Cmp(big.NewInt(capacity)) > 0 {
				t.Fatalf("discount=%v capacity=%d input=%s exceeds capacity", discount != nil, capacity, got)
			}
			if got.Cmp(limit) < 0 {
				next := new(big.Int).Add(got, big.NewInt(1))
				if reservedCapacityOutput(candidate, next, 1234).Cmp(big.NewInt(capacity)) <= 0 {
					t.Fatalf("discount=%v capacity=%d input=%s is not maximal", discount != nil, capacity, got)
				}
			}
		}
	}
}

func FuzzSolveFillPreservesExactInputAndOutputFloor(f *testing.F) {
	f.Add(uint16(100), uint16(60), uint16(40), uint16(30), uint16(120), uint16(110), uint16(90))
	f.Fuzz(func(
		t *testing.T,
		rawAmount, rawCapA, rawCapB, rawCapC, rawOutA, rawOutB, rawOutC uint16,
	) {
		amount := int64(rawAmount%1_000 + 1)
		caps := []int64{
			int64(rawCapA%2_000 + 1), int64(rawCapB%2_000 + 1), int64(rawCapC%2_000 + 1),
		}
		outputs := []int64{
			int64(rawOutA%2_000 + 1), int64(rawOutB%2_000 + 1), int64(rawOutC%2_000 + 1),
		}
		tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
		tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
		quotes := make([]liquidlane.FillQuote, 3)
		for index := range quotes {
			quotes[index] = testFillQuote(
				liquidlane.RouteID(string(rune('a'+index))),
				liquidlane.CapacityID(string(rune('A'+index))),
				tokenIn,
				tokenOut,
				amount,
				outputs[index],
				caps[index],
				nil,
			)
		}
		allocation, err := SolveFill(FillTask{
			TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(amount),
			Quotes: quotes, MaxRoutes: 3,
		})
		if err != nil {
			t.Fatal(err)
		}
		if allocation == nil {
			return
		}
		required := allocation.MaxAmountOut()
		routes := allocation.Finalize(required)
		if len(routes) == 0 {
			t.Fatal("complete allocation did not finalize")
		}
		totalInput := new(big.Int)
		totalMinimum := new(big.Int)
		for _, route := range routes {
			if route.AmountIn.Sign() <= 0 || route.ExpectedAmountOut.Sign() <= 0 ||
				route.MinAmountOut.Sign() <= 0 || route.MinAmountOut.Cmp(route.ExpectedAmountOut) > 0 {
				t.Fatalf("invalid route amounts: %+v", route)
			}
			totalInput.Add(totalInput, route.AmountIn)
			totalMinimum.Add(totalMinimum, route.MinAmountOut)
		}
		if totalInput.Cmp(big.NewInt(amount)) != 0 {
			t.Fatalf("input sum = %s, want %d", totalInput, amount)
		}
		if totalMinimum.Cmp(required) != 0 {
			t.Fatalf("minimum sum = %s, want %s", totalMinimum, required)
		}
	})
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
