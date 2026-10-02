package rfq

import (
	"bytes"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestExecutionBackendCompletionRollbackRebuildsFreshFill(t *testing.T) {
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, be, txm)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	e.now = st.now
	builds := 0
	e.strategy = fixedFillStrategy{plan: baseFillPlan(), onBuild: func() { builds++ }}
	syncCycle(t.Context(), e)
	first := append([]byte(nil), txm.lastReq.Data...)
	if st.order("o1").Status != statusFilled {
		t.Fatal("initial fill was not completed")
	}

	be.order.OrderStatus = "open"
	be.executable.ProtocolSignature = strPtr("0x1234")
	txm.onResult = func() { be.order.OrderStatus = "filled" }
	syncCycle(t.Context(), e)
	if rec := st.order("o1"); rec.Status != statusRetryWaiting || txm.calls != 1 {
		t.Fatalf("reopened completion skipped retry backoff: sends=%d order=%+v", txm.calls, rec)
	}
	now = now.Add(e.pollInterval)
	syncCycle(t.Context(), e)
	if txm.calls != 2 || builds != 2 || st.order("o1").Status != statusFilled {
		t.Fatalf("completion rollback did not rebuild: sends=%d plans=%d order=%+v", txm.calls, builds, st.order("o1"))
	}
	if bytes.Equal(first, txm.lastReq.Data) {
		t.Fatal("reorg recovery replayed old executable calldata")
	}
	if txm.lastReq.Obsolete == nil || txm.lastReq.CancelAt.IsZero() || txm.lastReq.GasLimit != 0 {
		t.Fatal("reorg retry omitted fresh deadline, obsolescence check, or gas estimation")
	}
	if st.order("o1").CancellationRetries != 1 {
		t.Fatal("completion rollback did not share the bounded nonce retry policy")
	}
	if st.order("o1").TxHash == (common.Hash{}) {
		t.Fatal("fresh lifecycle was not tracked")
	}
}

func completedFillResult(outcome txmanager.Outcome) txmanager.Result {
	result := confirmedTxResult()
	result.Outcome = outcome
	result.Receipt = &ethtypes.Receipt{
		TxHash: result.Hash, Status: ethtypes.ReceiptStatusSuccessful,
		BlockNumber: big.NewInt(100), BlockHash: common.HexToHash("0x1234"),
	}
	return result
}

func TestExecutionCompletedFillRequiresProvenReorgAndFreshOpenStatus(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{txmanager.OutcomeConfirmed, txmanager.OutcomeIncludedUnconfirmed} {
		t.Run(string(outcome), func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				mutate func(*fakeBackend, *fakeRecoveryReader)
			}{
				{name: "canonical fill", mutate: func(_ *fakeBackend, rdr *fakeRecoveryReader) { rdr.reorged = false }},
				{name: "rpc unavailable", mutate: func(_ *fakeBackend, rdr *fakeRecoveryReader) { rdr.reorgErr = errors.New("rpc unavailable") }},
				{name: "backend still filled", mutate: func(be *fakeBackend, _ *fakeRecoveryReader) { be.order.OrderStatus = "filled" }},
				{name: "backend unavailable", mutate: func(be *fakeBackend, _ *fakeRecoveryReader) { be.orderErr = errors.New("backend unavailable") }},
				{name: "unknown backend status", mutate: func(be *fakeBackend, _ *fakeRecoveryReader) { be.order.OrderStatus = "unknown" }},
				{name: "missing backend order", mutate: func(be *fakeBackend, _ *fakeRecoveryReader) { be.order = nil }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					st, be := fillFixtures(t)
					txm := &fakeTxm{result: completedFillResult(outcome)}
					e := newExec(t, st, be, txm)
					syncCycle(t.Context(), e)
					before := st.order("o1")
					if before.IncludedAt != 100 || before.IncludedHash != txm.result.Receipt.BlockHash {
						t.Fatalf("successful inclusion evidence was not retained: %+v", before)
					}
					be.order.OrderStatus = "open"
					rdr := e.reader.(*fakeRecoveryReader)
					rdr.reorged = true
					tc.mutate(be, rdr)
					for range 3 {
						syncCycle(t.Context(), e)
					}
					if rec := st.order("o1"); txm.calls != 1 || rec.Status != statusFilled || rec.CancellationRetries != 0 || rec.IncludedAt != 100 {
						t.Fatalf("uncertain completion was re-armed: sends=%d order=%+v", txm.calls, rec)
					}
				})
			}
		})
	}
}

func TestExecutionOrphanedFillDropsSpendBeforeFreshRetry(t *testing.T) {
	for _, backendStatus := range []string{"filled", "open"} {
		t.Run(backendStatus, func(t *testing.T) {
			st, be := fillFixtures(t)
			be.order.OrderStatus = backendStatus
			txm := &fakeTxm{result: completedFillResult(txmanager.OutcomeConfirmed)}
			e := newExec(t, st, be, txm)
			syncCycle(t.Context(), e)
			before := st.order("o1")
			if before.Status != statusFilled && before.Status != statusSubmitted {
				t.Fatalf("initial completion not tracked: %+v", before)
			}
			be.order.OrderStatus = "open"
			e.reader.(*fakeRecoveryReader).reorged = true
			e.syncOnce(t.Context())
			if rec := st.order("o1"); rec.Status != statusRetryWaiting || rec.IncludedAt != 0 || rec.IncludedHash != (common.Hash{}) || !st.reserved("o1") {
				t.Fatalf("orphaned spend survived reopening: %+v reserved=%v", rec, st.reserved("o1"))
			}
			if st.attempts["o1"] != 1 || st.order("o1").CancellationRetries != 1 || !st.order("o1").RetryDeadline.Equal(before.RetryDeadline) {
				t.Fatal("reopening lost the attempt count, deadline or retry budget")
			}
		})
	}
}

func TestExecutionReopenedCompletionKeepsBoundedRetryPolicy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		limit    int
		retries  int
		deadline time.Time
		want     orderStatus
	}{
		{name: "disabled", limit: 0, deadline: time.Unix(100, 0), want: statusFailed},
		{name: "exhausted", limit: 2, retries: 2, deadline: time.Unix(100, 0), want: statusFailed},
		{name: "expired", limit: 2, deadline: time.Unix(1, 0), want: statusExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			txm := &fakeTxm{result: completedFillResult(txmanager.OutcomeConfirmed)}
			e := newExec(t, st, be, txm)
			syncCycle(t.Context(), e)
			st.mu.Lock()
			st.orders["o1"].CancellationRetries = tc.retries
			st.orders["o1"].RetryDeadline = tc.deadline
			st.mu.Unlock()
			be.order.OrderStatus = "open"
			e.maxCancellationRetries = tc.limit
			e.reader.(*fakeRecoveryReader).reorged = true
			for range 3 {
				syncCycle(t.Context(), e)
			}
			if rec := st.order("o1"); txm.calls != 1 || rec.Status != tc.want || rec.CancellationRetries != tc.retries || rec.IncludedAt != 0 {
				t.Fatalf("reopened completion escaped limits: sends=%d order=%+v", txm.calls, rec)
			}
		})
	}
}

func TestExecutionOpenListingCannotReopenPendingOrSuppressedOrders(t *testing.T) {
	for _, status := range []orderStatus{statusSubmitting, statusSubmitted, statusFailed, statusExpired, statusObsolete} {
		t.Run(string(status), func(t *testing.T) {
			st, be := fillFixtures(t)
			st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
			st.markStatus("o1", status, common.HexToHash("0x1111"), "unresolved or terminal")
			be.order.OrderStatus = "open"
			e := newExec(t, st, be, &fakeTxm{result: confirmedTxResult()})
			e.reader.(*fakeRecoveryReader).reorged = true
			if err := e.pollOpenOrders(t.Context()); err != nil {
				t.Fatal(err)
			}
			if rec := st.order("o1"); rec.Status != status || rec.CancellationRetries != 0 {
				t.Fatalf("fresh listing disturbed protected lifecycle: %+v", rec)
			}
		})
	}
}

func TestExecutionReorgPollCannotDisturbOwnedLifecycle(t *testing.T) {
	st, be := fillFixtures(t)
	st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
	st.markStatus("o1", statusFilled, common.HexToHash("0x1111"), "")
	be.order.OrderStatus = "open"
	e := newExec(t, st, be, &fakeTxm{result: confirmedTxResult()})
	if !e.acquire("o1") {
		t.Fatal("acquire lifecycle")
	}
	defer e.release("o1")
	if err := e.pollOpenOrders(t.Context()); err != nil {
		t.Fatal(err)
	}
	if st.order("o1").Status != statusFilled {
		t.Fatal("reorg poll reopened an order another goroutine owns")
	}
}

func TestExecutionReopenedCompletionDoesNotCountAnotherWin(t *testing.T) {
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, be, txm)
	reg := prometheus.NewRegistry()
	metrics, err := newRFQMetrics(reg, st, "")
	if err != nil {
		t.Fatal(err)
	}
	e.metrics = metrics
	capacityID := liquidlane.RouteCapacityID(testInventory(vlt, tIn, tOut, big.NewInt(10), big.NewInt(1)).Route)
	syncCycle(t.Context(), e)
	if !st.reservations.Set("o1", liquidlane.CapacityReservations{capacityID: big.NewInt(10)}) {
		t.Fatal("seed old spend")
	}
	be.order.OrderStatus = "open"
	e.syncOnce(t.Context())
	if len(st.orders) != 1 || st.order("o1").CancellationRetries != 1 {
		t.Fatal("completion reopening duplicated an order")
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, "order", "won", 1)
	reserved := st.pendingReservations("", nil)[capacityID]
	if reserved == nil || reserved.Cmp(big.NewInt(10)) != 0 || st.order("o1").IncludedAt != 0 {
		t.Fatalf("reopened commitment was not reclassified from old spend: %v", reserved)
	}
}

func TestExecutionReopenedBackendCompletionDerivesMissingDeadline(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(map[bool]string{true: "fresh executable", false: "executable unavailable"}[available], func(t *testing.T) {
			st, be := fillFixtures(t)
			st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
			st.markStatus("o1", statusFilled, common.Hash{}, "")
			be.order.OrderStatus = "open"
			if !available {
				be.executable = nil
			}
			txm := &fakeTxm{result: confirmedTxResult()}
			e := newExec(t, st, be, txm)
			e.syncOnce(t.Context())
			rec := st.order("o1")
			if available && (rec.Status != statusRetryWaiting || rec.RetryDeadline.IsZero() || rec.CancellationRetries != 1) {
				t.Fatalf("missing retry deadline did not refresh from signed order: %+v", rec)
			}
			if !available && (rec.Status != statusFilled || !rec.RetryDeadline.IsZero() || rec.CancellationRetries != 0) {
				t.Fatalf("unavailable executable was prematurely reopened: %+v", rec)
			}
			if txm.calls != 0 {
				t.Fatal("reopened backend completion skipped its retry backoff")
			}
		})
	}
}

func TestExecutionReincludedFillAdoptsFreshCanonicalEvidence(t *testing.T) {
	st, be := fillFixtures(t)
	result := completedFillResult(txmanager.OutcomeConfirmed)
	be.order.TxHash = strPtr(common.HexToHash("0x9999").Hex())
	txm := &fakeTxm{result: result}
	e := newExec(t, st, be, txm)
	syncCycle(t.Context(), e)
	if rec := st.order("o1"); rec.TxHash == result.Hash || rec.IncludedTxHash != result.Hash {
		t.Fatalf("backend transaction reference overwrote own receipt identity: %+v", rec)
	}
	be.order.OrderStatus = "open"
	rdr := e.reader.(*fakeRecoveryReader)
	rdr.canonical = &ethtypes.Receipt{
		TxHash: result.Hash, Status: ethtypes.ReceiptStatusSuccessful,
		BlockNumber: big.NewInt(101), BlockHash: common.HexToHash("0x5678"),
	}
	syncCycle(t.Context(), e)
	if rec := st.order("o1"); txm.calls != 1 || rec.Status != statusFilled || rec.CancellationRetries != 0 ||
		rec.IncludedAt != 101 || rec.IncludedHash != rdr.canonical.BlockHash || rec.IncludedTxHash != result.Hash {
		t.Fatalf("re-included fill was reopened or evidence not refreshed: sends=%d order=%+v", txm.calls, rec)
	}
	if rdr.lastInclusion == nil || rdr.lastInclusion.TxHash != result.Hash {
		t.Fatal("canonical reconciliation looked up the backend transaction instead of the owned fill")
	}
}

func TestExecutionReorgRetryRequiresAnotherFreshOpenListing(t *testing.T) {
	st, be := fillFixtures(t)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	txm := &fakeTxm{result: completedFillResult(txmanager.OutcomeConfirmed)}
	e := newExec(t, st, be, txm)
	e.now = st.now
	syncCycle(t.Context(), e)
	be.order.OrderStatus = "open"
	e.reader.(*fakeRecoveryReader).reorged = true
	syncCycle(t.Context(), e)
	now = now.Add(e.pollInterval)
	be.open = nil
	syncCycle(t.Context(), e)
	if txm.calls != 1 || st.order("o1").Status != statusRetryWaiting {
		t.Fatalf("reorg retry bypassed fresh open listing: sends=%d order=%+v", txm.calls, st.order("o1"))
	}
}

func TestExecutionBackendCompletionRetryBudgetCannotBecomeUnsignedRetry(t *testing.T) {
	st, be := fillFixtures(t)
	st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
	st.markStatus("o1", statusFilled, common.Hash{}, "")
	be.order.OrderStatus = "open"
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, be, txm)
	e.maxCancellationRetries = 0
	for range 3 {
		syncCycle(t.Context(), e)
	}
	if rec := st.order("o1"); txm.calls != 0 || rec.Status != statusFailed || rec.CancellationRetries != 0 {
		t.Fatalf("disabled completion retry became an unsigned retry: sends=%d order=%+v", txm.calls, rec)
	}
}
