package rfq

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// TestUpsertQueued_RearmsFailedOrder: a still-open order that previously failed a fill is re-armed to
// queued on the next poll (mirrors the TS filler retrying failed-but-open orders), with LastError cleared.
func TestUpsertQueued_RearmsFailedOrder(t *testing.T) {
	t.Parallel()
	st := newStore(func() time.Time { return time.Unix(0, 0) })
	st.upsertQueued("o1")
	st.markStatus("o1", statusFailed, common.Hash{}, "fill reverted")

	st.upsertQueued("o1") // backend still lists it open

	rec := testOrder(st)
	if rec == nil || rec.Status != statusQueued {
		t.Fatalf("status = %v, want queued (re-armed)", rec)
	}
	if rec.LastError != "" {
		t.Fatalf("LastError = %q, want cleared on re-arm", rec.LastError)
	}
}

// TestUpsertQueued_DoesNotRegressInFlightOrTerminal: re-polling must never knock an in-flight or
// settled order back to queued — only `failed` is re-armed.
func TestUpsertQueued_DoesNotRegressInFlightOrTerminal(t *testing.T) {
	t.Parallel()
	for _, status := range []orderStatus{statusSubmitting, statusSubmitted, statusFilled, statusExpired} {
		st := newStore(func() time.Time { return time.Unix(0, 0) })
		st.upsertQueued("o1")
		st.markStatus("o1", status, common.Hash{}, "")

		st.upsertQueued("o1")

		if rec := testOrder(st); rec == nil || rec.Status != status {
			t.Fatalf("status = %v, want %v (unchanged)", rec, status)
		}
	}
}

func TestRecordAttempt_RearmAndSweepLifecycle(t *testing.T) {
	t.Parallel()
	now := time.Unix(0, 0)
	st := newStore(func() time.Time { return now })
	st.upsertQueued("o1")

	if got := st.recordAttempt("o1"); got != 1 {
		t.Fatalf("first attempt = %d, want 1", got)
	}
	st.markStatus("o1", statusFailed, common.Hash{}, "fill reverted")
	st.upsertQueued("o1")
	if got := st.recordAttempt("o1"); got != 2 {
		t.Fatalf("attempt after re-arm = %d, want 2", got)
	}

	st.markStatus("o1", statusFailed, common.Hash{}, "fill reverted again")
	now = now.Add(terminalOrderTTL)
	st.sweep()
	if rec := testOrder(st); rec == nil {
		t.Fatal("order swept at terminal TTL boundary")
	}
	if got := testOrder(st).attempts; got != 2 {
		t.Fatalf("attempts at terminal TTL boundary = %d, want 2", got)
	}

	now = now.Add(time.Nanosecond)
	st.sweep()
	if rec := testOrder(st); rec != nil {
		t.Fatalf("order after terminal TTL = %v, want swept", rec)
	}
	st.upsertQueued("o1")
	if got := st.recordAttempt("o1"); got != 1 {
		t.Fatalf("first attempt after recreation = %d, want 1", got)
	}
}
