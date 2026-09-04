package uniswapx

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

const (
	orderStatusCancelled         = "cancelled"
	orderStatusError             = "error"
	orderStatusExpired           = "expired"
	orderStatusFilled            = "filled"
	orderStatusInsufficientFunds = "insufficient-funds"
)

// quoteRuntime owns publication and invalidation of immutable quote snapshots.
type quoteRuntime struct {
	refreshMu     sync.Mutex
	quoteState    atomic.Pointer[quoteState]
	quoteEpoch    atomic.Uint64
	planningFills atomic.Int64
	chainTime     atomic.Int64
	refreshCh     chan struct{}
}

type orderLifecycle struct {
	execution trackedOrder
	exclusive trackedExclusive
}

type orderTerminal struct {
	Status string
	TxHash common.Hash
}

type exclusiveObligation struct {
	hash             common.Hash
	deadline         time.Time
	recoveredAtStart bool
}

type trackedExclusive struct {
	deadline         time.Time
	terminalAt       time.Time
	recoveredAtStart bool
}

func (t trackedExclusive) pending() bool {
	return !t.deadline.IsZero() && t.terminalAt.IsZero()
}

func (t trackedExclusive) terminal() bool { return !t.terminalAt.IsZero() }

type exclusiveDecision struct {
	exclusiveObligation

	settledInTime bool
	txHash        common.Hash
	filledAt      time.Time
	status        string
}

type exclusiveReconciliation struct {
	settled          []exclusiveDecision
	historicalMissed []exclusiveDecision
	missed           []exclusiveDecision
}

func (q *quoteRuntime) requestQuoteRefresh() {
	select {
	case q.refreshCh <- struct{}{}:
	default:
	}
}

func (s *Solver) onPendingReservationsAcquired(
	hash common.Hash,
	reservations liquidlane.CapacityReservations,
) {
	s.metrics.setPendingFills(s.capacity.Len())
	s.log.V(1).Info(
		"fill capacity reserved",
		"orderHash", hash.Hex(),
		"capacityGroups", len(reservations),
		"pendingFills", s.capacity.Len(),
	)
	s.invalidateQuotes()
	s.requestQuoteRefresh()
}

func (s *Solver) clearPendingReservations(hash common.Hash, lease *capacity.Lease) {
	// Stop quotes before releasing capacity. The next snapshot must observe the fill outcome
	// before the released capacity can be advertised again.
	s.invalidateQuotes()
	if !lease.Release() {
		return
	}
	s.metrics.setPendingFills(s.capacity.Len())
	s.log.V(1).Info(
		"fill capacity released",
		"orderHash", hash.Hex(),
		"pendingFills", s.capacity.Len(),
	)
	s.requestQuoteRefresh()
}

func (s *Solver) recordFillFailure(now time.Time) {
	tripped, until := s.breaker.recordFailure(now, s.cfg.Breaker.Window, s.cfg.Breaker.MaxFailures)
	if tripped {
		s.invalidateQuotes()
		s.updateBlockUntilMetric()
		s.log.Info("local fade breaker opened", "until", until)
	}
}

func (s *Solver) recordFillSuccess() {
	hadFailures, blockedUntil := s.breaker.recordSuccess()
	s.updateBlockUntilMetric()
	if hadFailures || blockedUntil != 0 {
		s.log.V(1).Info(
			"local fill breaker cleared",
			"hadFailures", hadFailures,
			"previousBlockUntil", blockedUntil,
		)
	}
}

func (s *Solver) trackExclusive(order *resolvedOrder, now time.Time) {
	if order.Source != orderSourceExclusiveV2 || order.ExclusiveUntil == 0 {
		return
	}
	s.trackExclusiveObligation(
		exclusiveObligation{
			hash:     order.Hash,
			deadline: time.Unix(int64(order.ExclusiveUntil), 0),
		},
		order.QuoteID,
		now,
	)
}

func (s *Solver) trackExclusiveObligation(
	obligation exclusiveObligation,
	quoteID string,
	now time.Time,
) {
	updated := s.ledger.trackExclusive(obligation, now)
	if updated {
		s.metrics.observeExclusiveWon()
		s.log.V(1).Info(
			"exclusive obligation tracked",
			"orderHash", obligation.hash.Hex(),
			"quoteId", quoteID,
			"exclusiveUntil", obligation.deadline.Unix(),
		)
	}
	s.observeExclusiveState()
}

func (s *Solver) sweepExclusive(ctx context.Context, now time.Time) error {
	expired := s.ledger.expiredExclusive(now)
	if len(expired) == 0 {
		return nil
	}
	s.log.V(1).Info(
		"exclusive obligations reconciliation started",
		"obligations", len(expired),
		"chainTime", now.Unix(),
	)

	decisions, err := s.resolveExclusiveDecisions(ctx, expired)
	if err != nil {
		return err
	}
	outcome := s.ledger.commitExclusive(now, decisions)
	s.reportExclusiveReconciliation(outcome, now)
	s.observeExclusiveState()
	return nil
}

func (s *Solver) resolveExclusiveDecisions(
	ctx context.Context,
	expired []exclusiveObligation,
) ([]exclusiveDecision, error) {
	hashes := make([]common.Hash, len(expired))
	for i := range expired {
		hashes[i] = expired[i].hash
	}
	terminals, err := s.orders.ordersByHash(ctx, s.chainID, hashes)
	if err != nil {
		return nil, errors.Errorf("lookup expired obligations: %w", err)
	}
	decisions := make([]exclusiveDecision, 0, len(expired))
	for _, obligation := range expired {
		terminal, ok := terminals[obligation.hash]
		if !ok {
			return nil, errors.Errorf("lookup expired obligation %s: missing result", obligation.hash.Hex())
		}
		decision := exclusiveDecision{
			exclusiveObligation: obligation,
			txHash:              terminal.TxHash,
			status:              terminal.Status,
		}
		if err := s.resolveExclusiveDecision(ctx, &decision); err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

func (s *Solver) resolveExclusiveDecision(ctx context.Context, decision *exclusiveDecision) error {
	switch decision.status {
	case orderStatusFilled:
		if decision.txHash == (common.Hash{}) {
			return errors.Errorf("lookup expired obligation %s: filled order has no transaction", decision.hash.Hex())
		}
		filledAt, err := s.reader.transactionBlockTimeConfirmed(ctx, decision.txHash, s.confirmations)
		if err != nil {
			return errors.Errorf("lookup expired obligation %s fill time: %w", decision.hash.Hex(), err)
		}
		decision.filledAt = filledAt
		// Uniswap counts the original quoter as faded whenever exclusivity expires
		// unfilled, even if our executor later wins the public Dutch auction.
		decision.settledInTime = !filledAt.After(decision.deadline)
	case orderStatusOpen:
		return errors.Errorf("lookup expired obligation %s: order is still open", decision.hash.Hex())
	case orderStatusExpired, orderStatusError, orderStatusCancelled, orderStatusInsufficientFunds:
		// Only a successful on-chain fill before exclusivity ends discharges the obligation.
		// Every other known lifecycle state means the awarded fill was not delivered in time.
		if decision.txHash != (common.Hash{}) {
			return errors.Errorf(
				"lookup expired obligation %s: status %q unexpectedly has transaction %s",
				decision.hash.Hex(), decision.status, decision.txHash.Hex(),
			)
		}
	default:
		return errors.Errorf("lookup expired obligation %s: unknown status %q", decision.hash.Hex(), decision.status)
	}
	return nil
}

func (s *Solver) reportExclusiveReconciliation(outcome exclusiveReconciliation, now time.Time) {
	for _, decision := range outcome.settled {
		s.metrics.observeExclusive("settled_in_time")
		s.log.Info(
			"exclusive order settled before exclusivity ended",
			"orderHash", decision.hash.Hex(),
			"tx", decision.txHash.Hex(),
			"filledAt", decision.filledAt.Unix(),
			"exclusiveUntil", decision.deadline.Unix(),
		)
	}
	for _, decision := range outcome.historicalMissed {
		fields := []any{
			"orderHash", decision.hash.Hex(),
			"status", decision.status,
			"exclusiveUntil", decision.deadline.Unix(),
			"origin", "startup-recovery",
		}
		if decision.txHash != (common.Hash{}) {
			fields = append(fields, "tx", decision.txHash.Hex(), "filledAt", decision.filledAt.Unix())
		}
		s.log.Info("historical exclusive obligation missed", fields...)
	}
	s.openExclusiveBreaker(outcome.missed, now)
}

func (s *Solver) openExclusiveBreaker(missed []exclusiveDecision, now time.Time) {
	if len(missed) == 0 {
		return
	}
	blockedUntil := s.breaker.openExclusive(now.Add(s.cfg.Breaker.Window).Unix())
	s.invalidateQuotes()
	s.updateBlockUntilMetric()
	for _, decision := range missed {
		s.metrics.observeExclusive("missed")
		fields := []any{
			"orderHash", decision.hash.Hex(),
			"status", decision.status,
			"exclusiveUntil", decision.deadline.Unix(),
			"blockUntil", blockedUntil,
		}
		if decision.txHash != (common.Hash{}) {
			fields = append(fields, "tx", decision.txHash.Hex(), "filledAt", decision.filledAt.Unix())
		}
		s.log.Error(errors.New("exclusive fill missed decay start"), "exclusive obligation missed", fields...)
	}
}

func (q *quoteRuntime) invalidateQuotes() {
	q.quoteEpoch.Add(1)
	q.quoteState.Store(nil)
}

func (q *quoteRuntime) beginFillPlanning() {
	q.planningFills.Add(1)
	q.invalidateQuotes()
}

func (q *quoteRuntime) endFillPlanning() {
	remaining := q.planningFills.Add(-1)
	q.quoteEpoch.Add(1)
	if remaining < 0 {
		panic("uniswapx: negative planning fill count")
	}
	q.requestQuoteRefresh()
}
