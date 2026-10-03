package uniswapx

import (
	"maps"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestLateFillReceiptRecordsMetricsWithoutReplayingCompletion(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	metrics, reg := newUniswapXTestMetricsWithRegistry(t, fixture.solver)
	fixture.solver.metrics = metrics
	pending, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route},
		fixture.order, fixture.now, fixture.now)
	if err != nil || pending == nil {
		t.Fatalf("start fill: pending=%v err=%v", pending, err)
	}
	fixture.solver.completePendingFill(t.Context(), pending, txmanager.Result{
		Outcome: txmanager.OutcomeAbandoned, Err: txmanager.ErrAbandoned,
	})
	// Later work may have opened the breaker; an old receipt must not clear it or retire a retry.
	fixture.solver.failureTimes = []time.Time{fixture.now}
	blockedUntil := fixture.now.Add(time.Minute).Unix()
	fixture.solver.localBlockUntil.Store(blockedUntil)
	retryAt := fixture.solver.retryAt[fixture.order.Hash]
	exclusive := fixture.solver.exclusiveUntil[fixture.order.Hash]
	if fixture.txm.reqs[0].ObserveReceipt == nil {
		t.Fatal("abandoned fill has no receipt metrics observer")
	}
	fixture.order.AmountIn.SetInt64(1)
	fixture.order.AmountOut.SetInt64(1)
	fixture.order.TokenIn, fixture.order.TokenOut = fixture.order.TokenOut, fixture.order.TokenIn
	fixture.strategy.plan.Routes[0].ExpectedAmountOut.SetInt64(1)
	fixture.txm.reqs[0].ObserveReceipt(t.Context(), txmanager.Result{
		Outcome: txmanager.OutcomeConfirmed, Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful},
	})
	if fixture.solver.capacity.Len() != 0 || len(fixture.solver.filled) != 0 ||
		!fixture.solver.retryAt[fixture.order.Hash].Equal(retryAt) || fixture.solver.attempts[fixture.order.Hash] != 0 ||
		fixture.solver.localBlockUntil.Load() != blockedUntil || len(fixture.solver.failureTimes) != 1 ||
		!reflect.DeepEqual(exclusive, fixture.solver.exclusiveUntil[fixture.order.Hash]) {
		t.Fatal("late receipt replayed business completion, retry, or breaker state")
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 1)
	metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenIn.Hex(), liquidlane.FillAmountInput, 100)
	metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenOut.Hex(), liquidlane.FillAmountOutput, 90)
	metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenOut.Hex(), liquidlane.FillAmountPlannedSurplus, 10)
	metricstest.RequireWorkflowEventCount(t, reg, Name, "exclusive_obligation", exclusiveOutcomeSettledInTime, 0)
}

func TestOrdinaryFillReceiptRecordsSuccessOnce(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{txmanager.OutcomeConfirmed, txmanager.OutcomeIncludedUnconfirmed} {
		t.Run(string(outcome), func(t *testing.T) {
			fixture := newDirectExecutionFixture(t)
			metrics, reg := newUniswapXTestMetricsWithRegistry(t, fixture.solver)
			fixture.solver.metrics = metrics
			pending, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route},
				fixture.order, fixture.now, fixture.now)
			if err != nil || pending == nil {
				t.Fatalf("start fill: pending=%v err=%v", pending, err)
			}
			fixture.txm.complete(txmanager.Result{
				Outcome: outcome, Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful},
			})
			fixture.solver.completePendingFill(t.Context(), pending, <-pending.result)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 1)
			metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenIn.Hex(), liquidlane.FillAmountInput, 100)
		})
	}
}

func TestObsoleteOwnedFillKeepsLateReceiptMetricsExclusive(t *testing.T) {
	for _, successful := range []bool{true, false} {
		for _, attemptsOnly := range []bool{false, true} {
			name := map[bool]string{true: "success", false: "revert"}[successful]
			name += map[bool]string{true: "/attempts", false: "/hash"}[attemptsOnly]
			t.Run(name, func(t *testing.T) {
				fixture := newDirectExecutionFixture(t)
				metrics, reg := newUniswapXTestMetricsWithRegistry(t, fixture.solver)
				fixture.solver.metrics = metrics
				pending, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route},
					fixture.order, fixture.now, fixture.now)
				if err != nil || pending == nil {
					t.Fatalf("start fill: pending=%v err=%v", pending, err)
				}
				fixture.solver.inFlight[fixture.order.Hash] = true
				fixture.solver.quoteState.Store(&quoteState{expiresAt: fixture.now.Add(time.Minute)})
				fixture.solver.failureTimes = []time.Time{fixture.now}
				blockedUntil := fixture.now.Add(time.Minute).Unix()
				fixture.solver.localBlockUntil.Store(blockedUntil)
				hash := common.HexToHash("0x1234")
				result := txmanager.Result{
					Hash: hash, Outcome: txmanager.OutcomeAbandoned,
					Err: errors.Errorf("pending transaction abandoned: %w", txmanager.ErrRequestObsolete),
				}
				if attemptsOnly {
					result.Hash, result.Attempts = common.Hash{}, []common.Hash{hash}
				}
				fixture.solver.completePendingFill(t.Context(), pending, result)
				if fixture.solver.capacity.Len() != 0 || fixture.solver.inFlight[fixture.order.Hash] ||
					fixture.solver.quoteState.Load() != nil || fixture.solver.quoteEpoch.Load() == 0 {
					t.Fatal("obsolete fill retained its reservation, in-flight state, or spent quote cache")
				}
				if _, retired := fixture.solver.filled[fixture.order.Hash]; !retired {
					t.Fatal("obsolete fill was not retired")
				}
				if _, retry := fixture.solver.retryAt[fixture.order.Hash]; retry ||
					fixture.solver.localBlockUntil.Load() != blockedUntil || len(fixture.solver.failureTimes) != 1 {
					t.Fatal("obsolete fill retried or changed the failure breaker")
				}
				metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeObsolete, 0)
				metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 0)
				metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
				// A refreshed quote must survive the old receipt callback too.
				fixture.solver.quoteState.Store(&quoteState{expiresAt: fixture.now.Add(time.Minute)})
				before := receiptBusinessSnapshot(fixture.solver)
				if fixture.txm.reqs[0].ObserveReceipt == nil {
					t.Fatal("obsolete owned fill lost its receipt metrics observer")
				}
				receipt := &types.Receipt{Status: types.ReceiptStatusFailed}
				outcome := txmanager.OutcomeReverted
				want := float64(0)
				if successful {
					receipt.Status, outcome, want = types.ReceiptStatusSuccessful, txmanager.OutcomeConfirmed, 1
				}
				fixture.txm.reqs[0].ObserveReceipt(t.Context(), txmanager.Result{Hash: hash, Outcome: outcome, Receipt: receipt})
				if !reflect.DeepEqual(before, receiptBusinessSnapshot(fixture.solver)) {
					t.Fatal("late receipt changed retired business state, cache, reservations, or breaker")
				}
				metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeObsolete, 0)
				metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, want)
				metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
				if !successful {
					return
				}
				metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenIn.Hex(), liquidlane.FillAmountInput, want*100)
				metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenOut.Hex(), liquidlane.FillAmountOutput, want*90)
				metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenOut.Hex(), liquidlane.FillAmountPlannedSurplus, want*10)
			})
		}
	}
}

func TestUnsignedObsoleteRetryDoesNotDoubleCountEarlierOwnedFill(t *testing.T) {
	for _, receiptBeforeRetry := range []bool{true, false} {
		t.Run(map[bool]string{true: "receipt before retry", false: "receipt after retry"}[receiptBeforeRetry], func(t *testing.T) {
			fixture := newDirectExecutionFixture(t)
			metrics, reg := newUniswapXTestMetricsWithRegistry(t, fixture.solver)
			fixture.solver.metrics = metrics
			pending, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route},
				fixture.order, fixture.now, fixture.now)
			if err != nil || pending == nil {
				t.Fatalf("start original fill: pending=%v err=%v", pending, err)
			}
			hash := common.HexToHash("0x1234")
			fixture.solver.completePendingFill(t.Context(), pending, txmanager.Result{
				Hash: hash, Outcome: txmanager.OutcomeAbandoned, Err: txmanager.ErrAbandoned,
			})
			fixture.solver.failureTimes = []time.Time{fixture.now}
			blockedUntil := fixture.now.Add(time.Minute).Unix()
			fixture.solver.localBlockUntil.Store(blockedUntil)
			observe := fixture.txm.reqs[0].ObserveReceipt
			if observe == nil {
				t.Fatal("abandoned original fill has no receipt metrics observer")
			}
			lateSuccess := func() {
				fixture.solver.quoteState.Store(&quoteState{expiresAt: fixture.now.Add(time.Minute)})
				before := receiptBusinessSnapshot(fixture.solver)
				observe(t.Context(), txmanager.Result{
					Hash: hash, Outcome: txmanager.OutcomeConfirmed,
					Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful},
				})
				if !reflect.DeepEqual(before, receiptBusinessSnapshot(fixture.solver)) {
					t.Fatal("late receipt changed retry/completion, cache, reservations, or breaker state")
				}
			}
			if receiptBeforeRetry {
				lateSuccess()
			}
			// Claiming a fresh retry must retain the earlier attempt's metric ownership.
			if !fixture.solver.claim(fixture.order.Hash, fixture.solver.retryAt[fixture.order.Hash]) {
				t.Fatal("open abandoned order could not be claimed for retry")
			}
			retry, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route},
				fixture.order, fixture.now, fixture.now)
			fixture.solver.endFillPlanning()
			if err != nil || retry == nil {
				t.Fatalf("start retry: pending=%v err=%v", retry, err)
			}
			fixture.solver.completePendingFill(t.Context(), retry, txmanager.Result{
				Outcome: txmanager.OutcomeSubmissionError,
				Err:     errors.Errorf("send before signing: %w", txmanager.ErrRequestObsolete),
			})
			if _, retired := fixture.solver.filled[fixture.order.Hash]; !retired {
				t.Fatal("obsolete unsigned retry was not retired")
			}
			if fixture.solver.capacity.Len() != 0 || fixture.solver.inFlight[fixture.order.Hash] ||
				len(fixture.solver.retryAt) != 0 || fixture.solver.quoteState.Load() != nil ||
				fixture.solver.localBlockUntil.Load() != blockedUntil || len(fixture.solver.failureTimes) != 1 {
				t.Fatal("obsolete retry retained work, cache, or reservations, or changed the breaker")
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeObsolete, 0)
			if !receiptBeforeRetry {
				lateSuccess()
			}
			if len(fixture.txm.reqs) != 2 {
				t.Fatalf("submissions = %d, want original and one unsigned retry", len(fixture.txm.reqs))
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 1)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeObsolete, 0)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
			metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenIn.Hex(), liquidlane.FillAmountInput, 100)
			metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenOut.Hex(), liquidlane.FillAmountOutput, 90)
			metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenOut.Hex(), liquidlane.FillAmountPlannedSurplus, 10)
		})
	}
}

// receiptBusinessSnapshot excludes metrics: callbacks must leave the entire fill lifecycle intact.
func receiptBusinessSnapshot(s *Solver) map[string]any {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return map[string]any{
		"filled": maps.Clone(s.filled), "retryAt": maps.Clone(s.retryAt),
		"inFlight": maps.Clone(s.inFlight), "attempts": maps.Clone(s.attempts),
		"ownedFillAttempts": maps.Clone(s.ownedFillAttempts),
		"exclusiveUntil":    maps.Clone(s.exclusiveUntil), "exclusiveTerminal": maps.Clone(s.exclusiveTerminal),
		"failureTimes":    append([]time.Time(nil), s.failureTimes...),
		"localBlockUntil": s.localBlockUntil.Load(), "exclusiveBlockUntil": s.exclusiveBlockUntil.Load(),
		"quoteState": s.quoteState.Load(), "quoteEpoch": s.quoteEpoch.Load(),
		"planningFills": s.planningFills.Load(), "capacity": s.capacity.SnapshotExcluding(""),
		"capacityRevision": s.capacity.Revision(),
	}
}

func TestOwnedFillAttemptHistoryExpiresWithOrderState(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{true: "terminal completion", false: "stale retry"}[terminal], func(t *testing.T) {
			fixture := newDirectExecutionFixture(t)
			metrics, reg := newUniswapXTestMetricsWithRegistry(t, fixture.solver)
			fixture.solver.metrics = metrics
			fixture.solver.completePendingFill(t.Context(), testPendingFill(t, fixture.order), txmanager.Result{
				Hash: common.HexToHash("0x1234"), Outcome: txmanager.OutcomeAbandoned, Err: txmanager.ErrAbandoned,
			})
			if !fixture.solver.ownedFillAttempts[fixture.order.Hash] {
				t.Fatal("abandoned fill did not retain owned attempt history for retry")
			}
			evictAt := fixture.solver.retryAt[fixture.order.Hash].Add(time.Hour + time.Second)
			obsolete := txmanager.Result{
				Outcome: txmanager.OutcomeSubmissionError,
				Err:     errors.Errorf("send before signing: %w", txmanager.ErrRequestObsolete),
			}
			if terminal {
				fixture.solver.completePendingFill(t.Context(), testPendingFill(t, fixture.order), obsolete)
				if len(fixture.solver.ownedFillAttempts) != 0 {
					t.Fatal("terminal order retained owned attempt history")
				}
				evictAt = fixture.solver.filled[fixture.order.Hash].Add(time.Hour + time.Second)
			}
			// After the existing order-state TTL, no owned transaction is associated with a
			// new unsigned lifecycle, so an obsolete skip must be observable again.
			if !fixture.solver.claim(fixture.order.Hash, evictAt) {
				t.Fatal("expired order-state history prevented a new lifecycle")
			}
			fixture.solver.endFillPlanning()
			if len(fixture.solver.ownedFillAttempts) != 0 {
				t.Fatal("eviction retained owned attempt history")
			}
			fixture.solver.completePendingFill(t.Context(), testPendingFill(t, fixture.order), obsolete)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeObsolete, 1)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 0)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
		})
	}
}
