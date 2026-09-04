package planning

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestFilterLiveInventoryRemovesExpiredAndDuplicateCandidates(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	direct := liquidlane.DirectInventory(
		liquidlane.Route{ID: "direct"}, big.NewInt(100), big.NewInt(1),
	)
	discountID := common.HexToHash("0x01")
	private := liquidlane.DiscountInventory(
		liquidlane.Route{ID: "private"}, big.NewInt(100), big.NewInt(1), discountID, now.Add(time.Minute),
	)
	expired := liquidlane.DirectInventory(
		liquidlane.Route{ID: "expired"}, big.NewInt(100), big.NewInt(1),
	)
	expired.ValidUntil = now

	got := FilterLiveInventory([]liquidlane.Inventory{direct, direct, private, expired}, now)
	if len(got) != 2 || got[0].ID != direct.ID || got[1].ID != private.ID {
		t.Fatalf("filtered inventory = %+v", got)
	}
}
