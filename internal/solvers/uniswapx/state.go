package uniswapx

import (
	"context"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

const (
	orderStatusCancelled         = "cancelled"
	orderStatusError             = "error"
	orderStatusExpired           = "expired"
	orderStatusFilled            = "filled"
	orderStatusInsufficientFunds = "insufficient-funds"

	exclusiveOutcomeSettledInTime = "settled_in_time"
	exclusiveOutcomeMissed        = "missed"
)

type orderTerminal struct {
	Status string
	TxHash common.Hash
}

type exclusiveObligation struct {
	hash             common.Hash
	deadline         time.Time
	recoveredAtStart bool
	liveObserved     bool
}

type trackedExclusive struct {
	resolvedAt       time.Time
	deadline         time.Time
	recoveredAtStart bool
	liveObserved     bool
}

type exclusiveDecision struct {
	exclusiveObligation

	settledInTime bool
	txHash        common.Hash
	filledAt      time.Time
	status        string
}

func (s *Solver) requestQuoteRefresh() {
	if s.refreshCh == nil {
		return
	}
	select {
	case s.refreshCh <- struct{}{}:
	default:
	}
}

func (s *Solver) setPendingReservations(hash common.Hash, reservations liquidlane.CapacityReservations) {
	if !s.capacity.Set(hash.Hex(), reservations) {
		return
	}
	s.log.V(1).Info(
		"fill capacity reserved",
		"orderHash", hash.Hex(),
		"capacityGroups", len(reservations),
		"pendingFills", s.capacity.Len(),
	)
	s.invalidateQuotes()
	s.requestQuoteRefresh()
}

func (s *Solver) clearPendingReservations(hash common.Hash) {
	// Stop quotes before releasing capacity. The next snapshot must observe the fill outcome
	// before the released capacity can be advertised again.
	s.invalidateQuotes()
	if !s.capacity.Delete(hash.Hex()) {
		return
	}
	s.log.V(1).Info(
		"fill capacity released",
		"orderHash", hash.Hex(),
		"pendingFills", s.capacity.Len(),
	)
	s.requestQuoteRefresh()
}

func (s *Solver) recordFillFailure(now time.Time) {
	s.stateMu.Lock()
	cutoff := now.Add(-s.cfg.Breaker.Window)
	kept := s.failureTimes[:0]
	for _, failure := range s.failureTimes {
		if failure.After(cutoff) {
			kept = append(kept, failure)
		}
	}
	s.failureTimes = append(kept, now)
	tripped := len(s.failureTimes) >= s.cfg.Breaker.MaxFailures
	if tripped {
		s.failureTimes = nil
		s.localBlockUntil.Store(now.Add(s.cfg.Breaker.Window).Unix())
	}
	s.stateMu.Unlock()
	if tripped {
		s.invalidateQuotes()
		s.log.Info("local fade breaker opened", "until", s.localBlockUntil.Load())
	}
}

func (s *Solver) recordFillSuccess() {
	s.stateMu.Lock()
	hadFailures := len(s.failureTimes) > 0
	s.failureTimes = nil
	s.stateMu.Unlock()
	blockedUntil := s.localBlockUntil.Swap(0)
	if hadFailures || blockedUntil != 0 {
		s.log.V(1).Info(
			"local fill breaker cleared",
			"hadFailures", hadFailures,
			"previousBlockUntil", blockedUntil,
		)
	}
}

func (s *Solver) trackExclusive(order *resolvedOrder) bool {
	if order.Source != orderSourceExclusiveV2 || order.ExclusiveUntil == 0 {
		return false
	}
	return s.trackExclusiveObligation(
		exclusiveObligation{
			hash:         order.Hash,
			deadline:     time.Unix(int64(order.ExclusiveUntil), 0),
			liveObserved: true,
		},
		order.QuoteID,
	)
}

func (s *Solver) trackExclusiveObligation(obligation exclusiveObligation, quoteID string) bool {
	s.stateMu.Lock()
	if s.obligations == nil {
		s.obligations = make(map[common.Hash]trackedExclusive)
	}
	if !s.obligations[obligation.hash].resolvedAt.IsZero() {
		s.stateMu.Unlock()
		return false
	}
	current, exists := s.obligations[obligation.hash]
	newLive := obligation.liveObserved && (!exists || !current.liveObserved)
	updated := !exists || obligation.deadline.Before(current.deadline)
	if updated {
		current.deadline = obligation.deadline
	}
	if !exists {
		current.recoveredAtStart = obligation.recoveredAtStart
	} else if !obligation.recoveredAtStart {
		// A live observation or runtime recovery takes precedence over startup history.
		current.recoveredAtStart = false
	}
	current.liveObserved = current.liveObserved || obligation.liveObserved
	s.obligations[obligation.hash] = current
	s.stateMu.Unlock()
	if updated {
		s.log.V(1).Info(
			"exclusive obligation tracked",
			"orderHash", obligation.hash.Hex(),
			"quoteId", quoteID,
			"exclusiveUntil", obligation.deadline.Unix(),
		)
	}
	return newLive
}

// sweepExclusive commits an observation only after every expired obligation has a
// conclusive status. A partial backend response cannot clear quote-admission uncertainty.
func (s *Solver) sweepExclusive(ctx context.Context, now time.Time) error {
	s.stateMu.Lock()
	var expired []exclusiveObligation
	for hash, tracked := range s.obligations {
		if tracked.resolvedAt.IsZero() && now.After(tracked.deadline) {
			expired = append(expired, exclusiveObligation{hash: hash, deadline: tracked.deadline,
				recoveredAtStart: tracked.recoveredAtStart, liveObserved: tracked.liveObserved})
		}
	}
	s.stateMu.Unlock()
	if len(expired) == 0 {
		return nil
	}
	slices.SortFunc(expired, func(a, b exclusiveObligation) int { return a.hash.Cmp(b.hash) })
	hashes := make([]common.Hash, len(expired))
	for i := range expired {
		hashes[i] = expired[i].hash
	}
	terminals, err := s.orders.ordersByHash(ctx, s.chainID, hashes)
	if err != nil {
		return errors.Errorf("lookup expired obligations: %w", err)
	}
	decisions := make([]exclusiveDecision, len(expired))
	for index, obligation := range expired {
		terminal, exists := terminals[obligation.hash]
		if !exists {
			return errors.Errorf("lookup expired obligation %s: missing result", obligation.hash.Hex())
		}
		decision, err := s.resolveExclusive(ctx, obligation, terminal)
		if err != nil {
			return errors.Errorf("lookup expired obligation %s: %w", obligation.hash.Hex(), err)
		}
		decisions[index] = decision
	}
	accepted := decisions[:0]
	s.stateMu.Lock()
	for _, decision := range decisions {
		tracked, exists := s.obligations[decision.hash]
		if !exists || !tracked.resolvedAt.IsZero() || !tracked.deadline.Equal(decision.deadline) {
			continue
		}
		decision.recoveredAtStart, decision.liveObserved = tracked.recoveredAtStart, tracked.liveObserved
		tracked.resolvedAt = now
		s.obligations[decision.hash] = tracked
		accepted = append(accepted, decision)
	}
	s.stateMu.Unlock()
	var missed []exclusiveDecision
	for _, decision := range accepted {
		switch {
		case decision.settledInTime:
			if !decision.recoveredAtStart {
				s.observeExclusiveOutcome(exclusiveOutcomeSettledInTime)
			}
			s.log.Info("exclusive order settled before exclusivity ended", "orderHash", decision.hash.Hex(),
				"tx", decision.txHash.Hex(), "filledAt", decision.filledAt.Unix(), "exclusiveUntil", decision.deadline.Unix())
		case decision.recoveredAtStart:
			s.log.Info("historical exclusive obligation missed", "orderHash", decision.hash.Hex(),
				"status", decision.status, "exclusiveUntil", decision.deadline.Unix(), "origin", "startup-recovery",
				"tx", decision.txHash.Hex(), "filledAt", decision.filledAt.Unix())
		default:
			missed = append(missed, decision)
		}
	}
	s.openExclusiveBreaker(missed, now)
	return nil
}

func (s *Solver) resolveExclusive(ctx context.Context, obligation exclusiveObligation, terminal orderTerminal) (exclusiveDecision, error) {
	decision := exclusiveDecision{exclusiveObligation: obligation, txHash: terminal.TxHash, status: terminal.Status}
	switch terminal.Status {
	case orderStatusFilled:
		if terminal.TxHash == (common.Hash{}) {
			return decision, errors.New("filled order has no transaction")
		}
		when, err := s.reader.transactionBlockTimeConfirmed(ctx, terminal.TxHash, s.confirmations)
		if err != nil {
			return decision, errors.Errorf("fill time: %w", err)
		}
		decision.filledAt, decision.settledInTime = when, !when.After(obligation.deadline)
	case orderStatusOpen:
		return decision, errors.New("order is still open")
	case orderStatusExpired, orderStatusError, orderStatusCancelled, orderStatusInsufficientFunds:
		if terminal.TxHash != (common.Hash{}) {
			return decision, errors.Errorf("status %q unexpectedly has transaction %s", terminal.Status, terminal.TxHash.Hex())
		}
	default:
		return decision, errors.Errorf("unknown status %q", terminal.Status)
	}
	// A later public Dutch fill does not discharge a missed exclusive commitment.
	return decision, nil
}

func (s *Solver) openExclusiveBreaker(missed []exclusiveDecision, now time.Time) {
	if len(missed) == 0 {
		return
	}
	blockedUntil := now.Add(s.cfg.Breaker.Window).Unix()
	if blockedUntil > s.exclusiveBlockUntil.Load() {
		s.exclusiveBlockUntil.Store(blockedUntil)
	}
	s.invalidateQuotes()
	for _, decision := range missed {
		if !decision.recoveredAtStart {
			s.observeExclusiveOutcome(exclusiveOutcomeMissed)
		}
		fields := []any{
			"orderHash", decision.hash.Hex(),
			"status", decision.status,
			"exclusiveUntil", decision.deadline.Unix(),
			"blockUntil", s.exclusiveBlockUntil.Load(),
		}
		if decision.txHash != (common.Hash{}) {
			fields = append(fields, "tx", decision.txHash.Hex(), "filledAt", decision.filledAt.Unix())
		}
		s.log.Error(errors.New("exclusive fill missed decay start"), "exclusive obligation missed", fields...)
	}
}

func (s *Solver) exclusiveObligationMetrics() (int, int64) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	outstanding := 0
	var nearest time.Time
	for _, tracked := range s.obligations {
		if !tracked.resolvedAt.IsZero() {
			continue
		}
		outstanding++
		if nearest.IsZero() || tracked.deadline.Before(nearest) {
			nearest = tracked.deadline
		}
	}
	if nearest.IsZero() {
		return outstanding, 0
	}
	return outstanding, nearest.Unix()
}

func (s *Solver) invalidateQuotes() { s.quotes.changePlanning(0) }

func (s *Solver) beginFillPlanning() { s.quotes.changePlanning(1) }

func (s *Solver) endFillPlanning() {
	s.quotes.changePlanning(-1)
	s.requestQuoteRefresh()
}
