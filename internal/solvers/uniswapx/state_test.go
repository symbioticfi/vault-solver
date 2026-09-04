package uniswapx

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/capacity"
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

func TestLocalBreakerInvalidatesQuotes(t *testing.T) {
	now := time.Now()
	solver := &Solver{
		cfg: &Config{Breaker: BreakerConfig{MaxFailures: 2, Window: time.Minute}},
		log: logr.Discard(),
	}
	solver.quoteState.Store(&quoteState{expiresAt: now.Add(time.Minute)})
	solver.recordFillFailure(now)
	if solver.breaker.localUntil.Load() != 0 || solver.quoteState.Load() == nil {
		t.Fatal("breaker opened before threshold")
	}
	solver.recordFillFailure(now.Add(time.Second))
	if solver.breaker.localUntil.Load() <= now.Unix() || solver.quoteState.Load() != nil {
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

	if solver.breaker.exclusiveUntil.Load() != now.Add(15*time.Minute+2*time.Second).Unix() {
		t.Fatalf("exclusive block until = %d", solver.breaker.exclusiveUntil.Load())
	}
	if solver.quoteState.Load() != nil {
		t.Fatal("missed exclusive obligation did not invalidate quotes")
	}
	if testOrderLifecycle(solver, hash).exclusive.pending() {
		t.Fatal("missed obligation remained pending")
	}
	if !testOrderLifecycle(solver, hash).exclusive.terminal() {
		t.Fatal("missed obligation was not marked terminal")
	}

	// A later unrelated fill may reset the ordinary failure breaker, but never the fade breaker.
	solver.recordFillSuccess()
	if solver.breaker.exclusiveUntil.Load() == 0 {
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
	if tracked := testOrderLifecycle(solver, hash).exclusive; !tracked.recoveredAtStart {
		t.Fatal("startup recovery marker was lost after failed reconciliation")
	}

	poller.err = nil
	if err := solver.sweepExclusive(t.Context(), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if solver.breaker.exclusiveUntil.Load() != 0 {
		t.Fatal("retried startup history opened exclusive breaker")
	}
	if testOrderLifecycle(solver, hash).exclusive.pending() {
		t.Fatal("retried startup history remained pending")
	}
}

func TestClearPendingReservationsInvalidatesQuoteState(t *testing.T) {
	hash := common.HexToHash("0x1234")
	t.Run("before releasing existing capacity", func(t *testing.T) {
		solver := &Solver{capacity: testCapacityBook()}
		reservations := liquidlane.CapacityReservations{"capacity-1": big.NewInt(1)}
		lease, err := solver.capacity.Acquire(
			capacity.NewOwner(Name, hash.Hex()), reservations, reservations,
		)
		if err != nil {
			t.Fatal(err)
		}
		solver.onPendingReservationsAcquired(hash, reservations)
		solver.quoteState.Store(&quoteState{expiresAt: time.Now().Add(time.Minute)})

		solver.clearPendingReservations(hash, lease)

		if solver.quoteState.Load() != nil {
			t.Fatal("released capacity remained quotable through the old snapshot")
		}
		if solver.capacity.Len() != 0 {
			t.Fatal("reservation was not released")
		}
	})
	t.Run("even when reservation is already absent", func(t *testing.T) {
		solver := &Solver{capacity: testCapacityBook()}
		solver.quoteState.Store(&quoteState{expiresAt: time.Now().Add(time.Minute)})

		solver.clearPendingReservations(hash, nil)

		if solver.quoteState.Load() != nil {
			t.Fatal("clear attempted ledger deletion before invalidating quote state")
		}
	})
}

func TestClaimTracksInflightAndBackoff(t *testing.T) {
	now := time.Now()
	hash := common.HexToHash("0x1")
	solver := &Solver{
		cfg:    &Config{OrderServer: OrderServerConfig{PollInterval: time.Second}},
		ledger: testLifecycle(nil),
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
