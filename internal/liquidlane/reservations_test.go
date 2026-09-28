package liquidlane

import (
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCapacityLedgerAggregatesAndReleasesClonedReservations(t *testing.T) {
	var ledger CapacityLedger
	first := CapacityReservations{"shared": big.NewInt(30)}
	if !ledger.Set("first", first) || !ledger.Set("second", CapacityReservations{"shared": big.NewInt(20)}) {
		t.Fatal("expected reservations to change ledger")
	}
	first["shared"].SetInt64(1)
	if got := ledger.Snapshot()["shared"]; got == nil || got.Int64() != 50 || ledger.Len() != 2 {
		t.Fatalf("snapshot = %v, len = %d", ledger.Snapshot(), ledger.Len())
	}
	if got := ledger.SnapshotExcluding("first")["shared"]; got == nil || got.Int64() != 20 {
		t.Fatalf("snapshot excluding first = %v, want shared=20", ledger.SnapshotExcluding("first"))
	}
	if !ledger.Delete("first") || ledger.Delete("missing") {
		t.Fatal("unexpected delete result")
	}
	if got := ledger.Snapshot()["shared"]; got == nil || got.Int64() != 20 || ledger.Len() != 1 {
		t.Fatalf("snapshot after release = %v, len = %d", ledger.Snapshot(), ledger.Len())
	}
}

func TestCapacityLedgerRejectsConcurrentPlansAndReplacesOwnedCapacity(t *testing.T) {
	var ledger CapacityLedger
	var accepted atomic.Int32
	var workers sync.WaitGroup
	for _, key := range []string{"first", "second"} {
		workers.Go(func() {
			if ledger.SetAt(key, CapacityReservations{"shared": big.NewInt(30)}, 0) {
				accepted.Add(1)
			}
		})
	}
	workers.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d plans based on the same free capacity", accepted.Load())
	}
	key := "first"
	if !ledger.Has(key) {
		key = "second"
	}
	if len(ledger.SnapshotExcluding(key)) != 0 || !ledger.SetAt(key, CapacityReservations{"replacement": big.NewInt(40)}, ledger.Revision()) {
		t.Fatal("owner could not replace its reservation")
	}
	if got := ledger.Snapshot(); len(got) != 1 || got["replacement"].Int64() != 40 {
		t.Fatalf("replacement retained old capacity: %v", got)
	}
}
