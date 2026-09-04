package rfq

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestOrderStateMachineRejectsTerminalRegression(t *testing.T) {
	store := newStore(func() time.Time { return time.Unix(1, 0) })
	store.upsertQueued("order")
	for _, status := range []orderStatus{statusSubmitted, statusFilled} {
		if !store.markStatus("order", status, common.Hash{}, "") {
			t.Fatalf("transition to %s rejected", status)
		}
	}
	if store.markStatus("order", statusQueued, common.Hash{}, "") {
		t.Fatal("terminal order regressed to queued")
	}
	store.upsertQueued("order")
	if got := store.orders["order"].Status; got != statusFilled {
		t.Fatalf("poll regressed terminal state to %s", got)
	}
}
