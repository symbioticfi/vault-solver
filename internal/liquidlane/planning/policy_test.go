package planning

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestNewPolicyDefaultsAndValidation(t *testing.T) {
	policy, err := NewPolicy(PolicyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if policy.MinAmount.Cmp(big.NewInt(1)) != 0 || policy.ExecutionBuffer != 12*time.Second {
		t.Fatalf("defaults = %+v", policy)
	}

	invalid := []PolicyConfig{
		{PriceBufferBps: -1},
		{PriceBufferBps: 5_000},
		{InventoryReserveBps: -1},
		{InventoryReserveBps: 10_000},
		{MinAmount: "0"},
		{MinAmount: "invalid"},
		{ExecutionDeadlineBuffer: "0s"},
	}
	for _, cfg := range invalid {
		if _, err := NewPolicy(cfg); err == nil {
			t.Fatalf("NewPolicy(%+v): error = nil", cfg)
		}
	}
}

func TestPolicyQuoteCandidatesAppliesCapacityAndPriceBuffers(t *testing.T) {
	discountID := common.HexToHash("0x1")
	route := liquidlane.NewRoute(1, common.HexToAddress("0x1"), common.HexToAddress("0x2"),
		common.HexToAddress("0x3"), common.HexToAddress("0x4"), 18, 18)
	inventory := []liquidlane.Inventory{
		liquidlane.DirectInventory(route, big.NewInt(1_000), big.NewInt(1_000_000_000_000_000_000)),
		liquidlane.DiscountInventory(route, big.NewInt(1_000), big.NewInt(1_000_000_000_000_000_000),
			discountID, time.Time{}),
	}
	policy, err := NewPolicy(PolicyConfig{PriceBufferBps: 100, InventoryReserveBps: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	allocated, candidates := policy.QuoteCandidates(inventory, nil)
	if len(allocated) != 2 || len(candidates) != 2 {
		t.Fatalf("allocated = %d, candidates = %d", len(allocated), len(candidates))
	}
	if allocated[0].MaxAssets.String() != "900" || candidates[0].MaxAmountOut.String() != "900" ||
		candidates[1].MaxAmountOut.String() != "891" {
		t.Fatalf("allocated = %+v, candidates = %+v", allocated, candidates)
	}
}

func TestPolicySolveFillBuildsSharedGasAndAllocationTask(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	policy, err := NewPolicy(PolicyConfig{PriceBufferBps: 100})
	if err != nil {
		t.Fatal(err)
	}
	solution, err := policy.SolveFill(PolicyFillTask{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(2),
		Quotes: []liquidlane.FillQuote{
			testFillQuote("route", "capacity", tokenIn, tokenOut, 2, 4, 4, nil),
		},
		MaxRoutes: 2, RequireSingleRoute: true, MaxFeePerGas: new(big.Int),
	})
	if err != nil || solution == nil || solution.MaxAmountOut().Cmp(big.NewInt(3)) != 0 {
		t.Fatalf("SolveFill = %v, %v", solution, err)
	}
}
