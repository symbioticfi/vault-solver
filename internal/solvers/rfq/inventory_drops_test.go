package rfq

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestLiquidLaneDropsCountUnderOneReasonAndKeepTheDetail(t *testing.T) {
	t.Parallel()
	drops := newInventoryDrops(t.Context())
	record := liquidLaneDrops(drops.trace)
	adapter := common.HexToAddress("0xa")
	record(liquidlane.Drop{Adapter: adapter, Reason: "paused reverted: InvalidInitialization()"})
	record(liquidlane.Drop{Adapter: adapter, TokenIn: common.HexToAddress("0xb"), Reason: "authorization: filler 0xE is not marketMaker 0x0, owner 0xO, or a delegated filler"})
	got := drops.String()
	if !strings.HasPrefix(got, "liquidlane read dropped x2; last: ") ||
		!strings.Contains(got, "detail=authorization: filler 0xE is not marketMaker") {
		t.Fatalf("drops = %q", got)
	}
}
