package uniswapx

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type stateTestOrderPoller struct {
	terminals map[common.Hash]orderTerminal
	recent    []orderEntry
	err       error
}

func (p *stateTestOrderPoller) openOrders(
	context.Context,
	int64,
	*common.Address,
) ([]orderEntry, error) {
	return nil, nil
}

func (p *stateTestOrderPoller) recentOrders(
	context.Context,
	int64,
	common.Address,
	time.Time,
) ([]orderEntry, error) {
	return p.recent, p.err
}

func (p *stateTestOrderPoller) ordersByHash(
	_ context.Context,
	_ int64,
	_ []common.Hash,
) (map[common.Hash]orderTerminal, error) {
	return p.terminals, p.err
}

type stateTestChainReader struct {
	chainReader

	transactionTimes map[common.Hash]time.Time
	confirmations    uint64
	err              error
}

func (r *stateTestChainReader) transactionBlockTimeConfirmed(
	_ context.Context,
	hash common.Hash,
	confirmations uint64,
) (time.Time, error) {
	r.confirmations = confirmations
	return r.transactionTimes[hash], r.err
}

func TestLocalBreakerInvalidatesQuotes(t *testing.T) {
	now := time.Now()
	solver := &Solver{
		cfg: &Config{Breaker: BreakerConfig{MaxFailures: 2, Window: time.Minute}},
		log: logr.Discard(),
	}
	solver.quoteState.Store(&quoteState{expiresAt: now.Add(time.Minute)})
	solver.recordFillFailure(now)
	if solver.localBlockUntil.Load() != 0 || solver.quoteState.Load() == nil {
		t.Fatal("breaker opened before threshold")
	}
	solver.recordFillFailure(now.Add(time.Second))
	if solver.localBlockUntil.Load() <= now.Unix() || solver.quoteState.Load() != nil {
		t.Fatal("breaker did not open and invalidate quotes")
	}
}

func TestMissedExclusiveObligationOpensIndependentBreaker(t *testing.T) {
	now := time.Unix(1_000, 0)
	hash := common.HexToHash("0x1234")
	solver := &Solver{
		cfg: &Config{Breaker: BreakerConfig{Window: 15 * time.Minute}}, log: logr.Discard(),
		orders: &stateTestOrderPoller{terminals: map[common.Hash]orderTerminal{
			hash: {Status: orderStatusExpired},
		}},
	}
	solver.quoteState.Store(&quoteState{expiresAt: now.Add(time.Minute)})
	solver.trackExclusive(&resolvedOrder{
		Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(now.Add(time.Second).Unix()),
	}, now)
	if err := solver.sweepExclusive(t.Context(), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}

	if solver.exclusiveBlockUntil.Load() != now.Add(15*time.Minute+2*time.Second).Unix() {
		t.Fatalf("exclusive block until = %d", solver.exclusiveBlockUntil.Load())
	}
	if solver.quoteState.Load() != nil {
		t.Fatal("missed exclusive obligation did not invalidate quotes")
	}
	if solver.exclusiveState[hash].pending() {
		t.Fatal("missed obligation remained pending")
	}
	if !solver.exclusiveState[hash].terminal() {
		t.Fatal("missed obligation was not marked terminal")
	}

	// A later unrelated fill may reset the ordinary failure breaker, but never the fade breaker.
	solver.recordFillSuccess()
	if solver.exclusiveBlockUntil.Load() == 0 {
		t.Fatal("ordinary fill success cleared the exclusive fade breaker")
	}
}

func TestStartupRecoveredMissRemainsHistoricalAfterRetry(t *testing.T) {
	now := time.Unix(1_000, 0)
	hash := common.HexToHash("0x1234")
	poller := &stateTestOrderPoller{
		terminals: map[common.Hash]orderTerminal{hash: {Status: orderStatusExpired}},
		err:       errors.New("temporary order API failure"),
	}
	solver := &Solver{
		cfg: &Config{Breaker: BreakerConfig{Window: time.Minute}}, log: logr.Discard(),
		orders: poller,
	}
	solver.trackExclusiveObligation(exclusiveObligation{
		hash: hash, deadline: now.Add(time.Second), recoveredAtStart: true,
	}, "", now)

	if err := solver.sweepExclusive(t.Context(), now.Add(2*time.Second)); err == nil {
		t.Fatal("temporary terminal lookup failure was accepted")
	}
	if tracked := solver.exclusiveState[hash]; !tracked.recoveredAtStart {
		t.Fatal("startup recovery marker was lost after failed reconciliation")
	}

	poller.err = nil
	if err := solver.sweepExclusive(t.Context(), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if solver.exclusiveBlockUntil.Load() != 0 {
		t.Fatal("retried startup history opened exclusive breaker")
	}
	if solver.exclusiveState[hash].pending() {
		t.Fatal("retried startup history remained pending")
	}
}

func TestExclusiveSettlementAtDeadlineDoesNotTripBreaker(t *testing.T) {
	now := time.Unix(1_000, 0)
	deadline := now.Add(time.Second)
	hash := common.HexToHash("0x1234")
	txHash := common.HexToHash("0xabcd")
	solver := &Solver{
		cfg:           &Config{Breaker: BreakerConfig{Window: time.Minute}},
		log:           logr.Discard(),
		confirmations: 2,
		orders: &stateTestOrderPoller{terminals: map[common.Hash]orderTerminal{
			hash: {Status: orderStatusFilled, TxHash: txHash},
		}},
		reader: &stateTestChainReader{transactionTimes: map[common.Hash]time.Time{
			txHash: deadline,
		}},
	}
	solver.trackExclusive(&resolvedOrder{
		Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(deadline.Unix()),
	}, now)

	if err := solver.sweepExclusive(t.Context(), deadline.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	if solver.exclusiveBlockUntil.Load() != 0 {
		t.Fatal("in-time settlement tripped the exclusive breaker")
	}
	if solver.exclusiveState[hash].pending() {
		t.Fatal("settled obligation remained pending")
	}
	reader := solver.reader.(*stateTestChainReader)
	if reader.confirmations != 2 {
		t.Fatalf("confirmation depth = %d, want 2", reader.confirmations)
	}
}

func TestAnyLateFillTripsExclusiveBreaker(t *testing.T) {
	now := time.Unix(1_000, 0)
	deadline := now.Add(time.Second)
	hash := common.HexToHash("0x1234")
	txHash := common.HexToHash("0xabcd")
	solver := &Solver{
		cfg: &Config{Breaker: BreakerConfig{Window: time.Minute}},
		log: logr.Discard(),
		orders: &stateTestOrderPoller{terminals: map[common.Hash]orderTerminal{
			hash: {Status: orderStatusFilled, TxHash: txHash},
		}},
		reader: &stateTestChainReader{transactionTimes: map[common.Hash]time.Time{
			txHash: deadline.Add(time.Second),
		}},
	}
	solver.trackExclusive(&resolvedOrder{
		Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(deadline.Unix()),
	}, now)

	if err := solver.sweepExclusive(t.Context(), deadline.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}

	if solver.exclusiveBlockUntil.Load() == 0 {
		t.Fatal("late fill did not trip the exclusive breaker")
	}
}

func TestKnownUnfilledTerminalStatusesTripExclusiveBreaker(t *testing.T) {
	now := time.Unix(1_000, 0)
	for _, status := range []string{
		orderStatusExpired,
		orderStatusError,
		orderStatusCancelled,
		orderStatusInsufficientFunds,
	} {
		t.Run(status, func(t *testing.T) {
			hash := common.HexToHash("0x1234")
			solver := &Solver{
				cfg: &Config{Breaker: BreakerConfig{Window: time.Minute}},
				log: logr.Discard(),
				orders: &stateTestOrderPoller{terminals: map[common.Hash]orderTerminal{
					hash: {Status: status},
				}},
			}
			solver.trackExclusive(&resolvedOrder{
				Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(now.Add(time.Second).Unix()),
			}, now)

			if err := solver.sweepExclusive(t.Context(), now.Add(2*time.Second)); err != nil {
				t.Fatal(err)
			}
			if solver.exclusiveBlockUntil.Load() == 0 {
				t.Fatalf("terminal status %q did not trip the exclusive breaker", status)
			}
		})
	}
}

func TestUnresolvedExclusiveStateKeepsObligationPending(t *testing.T) {
	now := time.Unix(1_000, 0)
	deadline := now.Add(time.Second)
	hash := common.HexToHash("0x1234")
	for _, tc := range []struct {
		name      string
		terminals map[common.Hash]orderTerminal
	}{
		{name: "missing", terminals: map[common.Hash]orderTerminal{}},
		{name: "still open", terminals: map[common.Hash]orderTerminal{
			hash: {Status: orderStatusOpen},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			solver := &Solver{
				cfg:    &Config{Breaker: BreakerConfig{Window: time.Minute}},
				log:    logr.Discard(),
				orders: &stateTestOrderPoller{terminals: tc.terminals},
			}
			solver.trackExclusive(&resolvedOrder{
				Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(deadline.Unix()),
			}, now)

			if err := solver.sweepExclusive(t.Context(), deadline.Add(time.Second)); err == nil {
				t.Fatal("unresolved terminal result was accepted")
			}
			if solver.exclusiveBlockUntil.Load() != 0 {
				t.Fatal("unresolved terminal result tripped the exclusive breaker")
			}
			if !solver.exclusiveState[hash].pending() {
				t.Fatal("unresolved obligation was removed instead of retried")
			}
		})
	}
}

func TestClearPendingReservationsInvalidatesQuoteState(t *testing.T) {
	hash := common.HexToHash("0x1234")
	t.Run("before releasing existing capacity", func(t *testing.T) {
		solver := &Solver{}
		if !solver.capacity.Set(hash.Hex(), liquidlane.CapacityReservations{"capacity-1": big.NewInt(1)}) {
			t.Fatal("set reservation")
		}
		solver.quoteState.Store(&quoteState{expiresAt: time.Now().Add(time.Minute)})

		solver.clearPendingReservations(hash)

		if solver.quoteState.Load() != nil {
			t.Fatal("released capacity remained quotable through the old snapshot")
		}
		if solver.capacity.Len() != 0 {
			t.Fatal("reservation was not released")
		}
	})
	t.Run("even when reservation is already absent", func(t *testing.T) {
		solver := &Solver{}
		solver.quoteState.Store(&quoteState{expiresAt: time.Now().Add(time.Minute)})

		solver.clearPendingReservations(hash)

		if solver.quoteState.Load() != nil {
			t.Fatal("clear attempted ledger deletion before invalidating quote state")
		}
	})
}

func TestClaimTracksInflightAndBackoff(t *testing.T) {
	now := time.Now()
	hash := common.HexToHash("0x1")
	solver := &Solver{
		cfg:        &Config{OrderServer: OrderServerConfig{PollInterval: time.Second}},
		orderState: make(map[common.Hash]trackedOrder),
	}
	solver.quoteState.Store(&quoteState{expiresAt: now.Add(time.Minute)})
	if !solver.claim(hash, now) || solver.claim(hash, now) {
		t.Fatal("claim did not enforce in-flight deduplication")
	}
	if solver.planningFills.Load() != 1 || solver.quoteState.Load() != nil {
		t.Fatal("claimed order did not block quotes before fill planning")
	}
	solver.endFillPlanning()
	solver.retry(hash, now, true)
	if solver.claim(hash, now.Add(500*time.Millisecond)) || !solver.claim(hash, now.Add(time.Second)) {
		t.Fatal("retry backoff was not enforced")
	}
	solver.endFillPlanning()
}
