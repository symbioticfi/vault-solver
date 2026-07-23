package liquidlane

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestIDsAreStableLowercase(t *testing.T) {
	adapter := common.HexToAddress("0x00000000000000000000000000000000000000AA")
	tokenIn := common.HexToAddress("0x00000000000000000000000000000000000000BB")
	tokenOut := common.HexToAddress("0x00000000000000000000000000000000000000CC")
	discount := common.HexToHash("0xABCDEF0000000000000000000000000000000000000000000000000000000000")

	route := NewRoute(11155111, adapter, common.Address{}, tokenIn, tokenOut, 18, 6)
	if got, want := string(route.ID), "route:11155111:0x00000000000000000000000000000000000000aa:0x00000000000000000000000000000000000000bb:0x00000000000000000000000000000000000000cc"; got != want {
		t.Fatalf("route id = %q, want %q", got, want)
	}
	if got, want := string(NewCandidateID(route, &discount)), "candidate:"+string(route.ID)+":discount:"+discount.Hex(); got != want {
		t.Fatalf("candidate id = %q, want %q", got, want)
	}
}

func TestInventoryConstructorsCloneMutableValues(t *testing.T) {
	route := NewRoute(1, common.HexToAddress("0x1"), common.Address{}, common.HexToAddress("0x2"), common.HexToAddress("0x3"), 18, 6)
	maxAssets := big.NewInt(100)
	maxRate := big.NewInt(200)
	discount := common.HexToHash("0x42")

	validUntil := time.Unix(2, 0)
	inv := DiscountInventory(route, maxAssets, maxRate, discount, validUntil)
	maxAssets.SetInt64(1)
	maxRate.SetInt64(2)

	if inv.MaxAssets.String() != "100" || inv.MaxRate.String() != "200" {
		t.Fatalf("inventory did not clone big.Int values: maxAssets=%s maxRate=%s", inv.MaxAssets, inv.MaxRate)
	}
	if inv.DiscountID == nil || *inv.DiscountID == (common.Hash{}) {
		t.Fatalf("inventory did not clone discount id: %v", inv.DiscountID)
	}
	if !inv.ValidUntil.Equal(validUntil) {
		t.Fatalf("valid until = %s", inv.ValidUntil)
	}
}
