package bridgefacilitator

import (
	"testing"

	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/threef"
)

// The tolerant client zero-values a required field the API drops instead of failing the decode, so
// the solver has to notice at the boundary rather than silently offer on nothing.
func TestValidAuctionsDropsIncompleteEntries(t *testing.T) {
	s := &Solver{log: logr.Discard()}
	good := threef.AuctionDto{Id: 7, Status: "open", RequestId: "0x00000000000000000000000000000000000000aa"}
	in := []threef.AuctionDto{
		good,
		{Id: 0, Status: "open", RequestId: good.RequestId},
		{Id: 8, Status: "", RequestId: good.RequestId},
		{Id: 9, Status: "open", RequestId: "not-an-address"},
	}
	kept := s.validAuctions(in)
	if len(kept) != 1 || kept[0].Id != good.Id {
		t.Fatalf("kept %+v, want only auction %v", kept, good.Id)
	}
}
