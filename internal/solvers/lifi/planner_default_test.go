package lifi

import (
	"context"
	"encoding/binary"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestConfigContract(t *testing.T) {
	tests := []struct {
		name    string
		cfg     defaultPlannerConfig
		wantErr bool
	}{
		{name: "defaults"},
		{name: "explicit", cfg: defaultPlannerConfig{PriceBufferBps: 100, MinAmount: "1000", RangeCount: 4, InventoryReserveBps: 250, ExecutionDeadlineBuffer: "30s"}},
		{name: "negative ranges", cfg: defaultPlannerConfig{RangeCount: -1}, wantErr: true},
		{name: "too many ranges", cfg: defaultPlannerConfig{RangeCount: MaxQuoteRanges + 1}, wantErr: true},
		{name: "invalid amount", cfg: defaultPlannerConfig{MinAmount: "nope"}, wantErr: true},
		{name: "invalid deadline", cfg: defaultPlannerConfig{ExecutionDeadlineBuffer: "soon"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy, err := newDefaultPlanner(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("newDefaultPlanner() error = %v, wantErr %t", err, tt.wantErr)
			}
			if err == nil && strategy.rangeCount < 1 {
				t.Fatalf("rangeCount = %d", strategy.rangeCount)
			}
		})
	}
}

func TestDecideQuotesPublishesBoundedCapacity(t *testing.T) {
	strategy := mustStrategy(t, defaultPlannerConfig{PriceBufferBps: 100, MinAmount: "100"})
	now := time.Unix(1_800_000_000, 0)
	laneRoute := testRoute("shared", 1)
	out, err := strategy.DecideQuotes(context.Background(), QuoteInput{
		Solver:         testSolver,
		ChainTime:      now,
		QuoteExpiresAt: now.Add(5 * time.Minute),
		MaxFeePerGas:   big.NewInt(0),
		Reservations:   liquidlane.CapacityReservations{"shared": big.NewInt(200)},
		Inventory: []liquidlane.Inventory{
			testInventory(laneRoute, 700),
			testInventory(testRoute("shared", 2), 700),
		},
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %d, want 1", len(out.Quotes))
	}
	quote := out.Quotes[0]
	if quote.Expiry != now.Add(5*time.Minute).Unix() {
		t.Fatalf("expiry = %d", quote.Expiry)
	}
	last := quote.Ranges[len(quote.Ranges)-1]
	if last.MaxAmount.String() != "500" {
		t.Fatalf("published shared capacity = %s, want 500", last.MaxAmount)
	}
	rate, ok := new(big.Rat).SetString(last.Quote)
	if !ok || rate.Sign() <= 0 || rate.Cmp(big.NewRat(99, 100)) > 0 {
		t.Fatalf("buffered quote = %q", last.Quote)
	}
}

func TestDecideQuotesRequiresExpiry(t *testing.T) {
	strategy := mustStrategy(t, defaultPlannerConfig{})
	_, err := strategy.DecideQuotes(context.Background(), QuoteInput{ChainTime: time.Unix(1_800_000_000, 0)})
	if err == nil {
		t.Fatal("expected missing quote expiry error")
	}
}

func TestDecideFillContract(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := FillInput{
		Solver:       testSolver,
		TokenIn:      testTokenIn,
		TokenOut:     testTokenOut,
		AmountIn:     big.NewInt(1_000),
		OutputAmount: big.NewInt(900),
		ChainTime:    now,
		FillDeadline: uint32(now.Add(time.Minute).Unix()),
		MaxFeePerGas: big.NewInt(0),
		Quotes: []liquidlane.FillQuote{
			testFillQuote(testRoute("capacity-1", 1), 500),
			testFillQuote(testRoute("capacity-2", 2), 500),
		},
	}
	tests := []struct {
		name          string
		mutate        func(*FillInput)
		wantRoutes    int
		wantPermanent bool
	}{
		{name: "profitable multi route", wantRoutes: 2},
		{name: "shared capacity cannot be double spent", mutate: func(input *FillInput) {
			input.Quotes[1].CapacityID = "capacity-1"
		}},
		{name: "pending reservation is subtracted", mutate: func(input *FillInput) {
			input.Reservations = liquidlane.CapacityReservations{"capacity-1": big.NewInt(200), "capacity-2": big.NewInt(200)}
		}},
		{name: "expired", mutate: func(input *FillInput) {
			input.Expires = uint32(now.Unix())
		}},
		{name: "malformed output context", mutate: func(input *FillInput) {
			input.OutputContext = []byte{0xff}
		}, wantPermanent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := cloneFillInput(base)
			if tt.mutate != nil {
				tt.mutate(&input)
			}
			decision, err := mustStrategy(t, defaultPlannerConfig{}).DecideFill(context.Background(), input)
			if tt.wantPermanent {
				if err == nil || !IsPermanentFillDecisionError(err) {
					t.Fatalf("error = %v, want permanent", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecideFill: %v", err)
			}
			if tt.wantRoutes == 0 {
				if decision.Plan != nil {
					t.Fatalf("plan = %+v, want nil", decision.Plan)
				}
				return
			}
			if decision.Plan == nil || len(decision.Plan.Routes) != tt.wantRoutes {
				t.Fatalf("plan = %+v, want %d routes", decision.Plan, tt.wantRoutes)
			}
		})
	}
}

func TestDecideFillHonorsExclusiveWindow(t *testing.T) {
	strategy := mustStrategy(t, defaultPlannerConfig{})
	now := time.Unix(1_800_000_000, 0)
	input := FillInput{
		Solver:        testSolver,
		TokenIn:       testTokenIn,
		TokenOut:      testTokenOut,
		AmountIn:      big.NewInt(1_000),
		OutputAmount:  big.NewInt(900),
		OutputContext: exclusiveLimitContext(common.HexToAddress("0x6666666666666666666666666666666666666666")),
		ChainTime:     now,
		MaxFeePerGas:  big.NewInt(0),
		Quotes:        []liquidlane.FillQuote{testFillQuote(testRoute("capacity-1", 1), 1_000)},
	}
	decision, err := strategy.DecideFill(context.Background(), input)
	if err != nil || decision.Plan != nil {
		t.Fatalf("exclusive fill = (%+v, %v), want skip", decision.Plan, err)
	}
	input.ChainTime = now.Add(11 * time.Second)
	decision, err = strategy.DecideFill(context.Background(), input)
	if err != nil || decision.Plan == nil {
		t.Fatalf("post-exclusive fill = (%+v, %v), want plan", decision.Plan, err)
	}
}

func TestFixedPointDecimal(t *testing.T) {
	tests := map[string]string{
		"0":                      "0",
		"990000000000000000":     "0.99",
		"1000000000000000000":    "1",
		"1234500000000000000":    "1.2345",
		"1000000000000000000000": "1000",
	}
	for raw, want := range tests {
		n, _ := new(big.Int).SetString(raw, 10)
		if got := fixedPointDecimal(n, 18); got != want {
			t.Fatalf("fixedPointDecimal(%s) = %q, want %q", raw, got, want)
		}
	}
}

var (
	testSolver   = common.HexToAddress("0x1111111111111111111111111111111111111111")
	testTokenIn  = common.HexToAddress("0x2222222222222222222222222222222222222222")
	testTokenOut = common.HexToAddress("0x3333333333333333333333333333333333333333")
)

func mustStrategy(t *testing.T, cfg defaultPlannerConfig) *defaultPlanner {
	t.Helper()
	strategy, err := newDefaultPlanner(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return strategy
}

func testRoute(capacityID liquidlane.CapacityID, number int) liquidlane.Route {
	return liquidlane.Route{
		ID:               liquidlane.RouteID("route-" + strconv.Itoa(number)),
		CapacityID:       capacityID,
		Adapter:          common.BytesToAddress([]byte{byte(number)}),
		TokenIn:          testTokenIn,
		TokenOut:         testTokenOut,
		TokenInDecimals:  6,
		TokenOutDecimals: 6,
	}
}

func testInventory(route liquidlane.Route, capacity int64) liquidlane.Inventory {
	return liquidlane.Inventory{
		Route:     route,
		MaxAssets: big.NewInt(capacity),
		MaxRate:   big.NewInt(1_000_000_000_000_000_000),
	}
}

func testFillQuote(route liquidlane.Route, capacity int64) liquidlane.FillQuote {
	return liquidlane.FillQuote{
		Inventory:    testInventory(route, capacity),
		AmountIn:     big.NewInt(1_000),
		MaxAmountOut: big.NewInt(1_000),
	}
}

func cloneFillInput(input FillInput) FillInput {
	input.AmountIn = new(big.Int).Set(input.AmountIn)
	input.OutputAmount = new(big.Int).Set(input.OutputAmount)
	input.Quotes = append([]liquidlane.FillQuote(nil), input.Quotes...)
	return input
}

func exclusiveLimitContext(exclusiveFor common.Address) []byte {
	out := make([]byte, 37)
	out[0] = exclusiveLimitOrderContextType
	solverID := solverIdentifier(exclusiveFor)
	copy(out[1:33], solverID[:])
	binary.BigEndian.PutUint32(out[33:37], 1_800_000_010)
	return out
}
