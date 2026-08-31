package liquidlane

import (
	"math/big"
	"sync"
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

func TestCapacityLedgerConcurrentSnapshotsObserveOnlyLegalAggregates(t *testing.T) {
	const operations = 100
	var ledger CapacityLedger
	start := make(chan struct{})
	writerReady := make(chan struct{}, 4)
	var writers sync.WaitGroup
	for index, amount := range []int64{1, 2, 4, 8} {
		writers.Go(func() {
			writerReady <- struct{}{}
			<-start
			key := big.NewInt(int64(index)).String()
			for range operations {
				if !ledger.Set(key, CapacityReservations{"shared": big.NewInt(amount)}) {
					t.Errorf("Set(%q) failed", key)
					return
				}
				ledger.Snapshot()
				if !ledger.Delete(key) {
					t.Errorf("Delete(%q) failed", key)
					return
				}
			}
		})
	}
	for range 4 {
		<-writerReady
	}
	close(start)
	for range operations * 4 {
		snapshot := ledger.Snapshot()
		if len(snapshot) == 0 {
			continue
		}
		amount := snapshot["shared"]
		if len(snapshot) != 1 || amount == nil || amount.Sign() <= 0 || amount.Cmp(big.NewInt(15)) > 0 {
			t.Fatalf("illegal concurrent aggregate snapshot: %v", snapshot)
		}
		amount.SetInt64(1_000)
	}
	writers.Wait()
	if snapshot := ledger.Snapshot(); len(snapshot) != 0 || ledger.Len() != 0 {
		t.Fatalf("final snapshot = %v, len = %d; want empty", snapshot, ledger.Len())
	}
}
