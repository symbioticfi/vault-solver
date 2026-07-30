package uniswapx

import (
	"context"
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
)

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
	recoveredAtStart bool
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
	if s.metrics != nil {
		s.metrics.pendingFills.Set(float64(s.capacity.Len()))
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
	if s.metrics != nil {
		s.metrics.pendingFills.Set(float64(s.capacity.Len()))
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
		s.updateBlockUntilMetric()
		s.log.Info("local fade breaker opened", "until", s.localBlockUntil.Load())
	}
}

func (s *Solver) recordFillSuccess() {
	s.stateMu.Lock()
	hadFailures := len(s.failureTimes) > 0
	s.failureTimes = nil
	s.stateMu.Unlock()
	blockedUntil := s.localBlockUntil.Swap(0)
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
	s.stateMu.Lock()
	s.cleanupExclusiveLocked(now)
	updated := false
	if _, terminal := s.exclusiveTerminal[obligation.hash]; !terminal {
		current, exists := s.exclusiveUntil[obligation.hash]
		if !exists || obligation.deadline.Before(current.deadline) {
			current.deadline = obligation.deadline
			updated = true
		}
		if !exists {
			current.recoveredAtStart = obligation.recoveredAtStart
		} else if !obligation.recoveredAtStart {
			// A live observation or runtime recovery takes precedence over startup history.
			current.recoveredAtStart = false
		}
		s.exclusiveUntil[obligation.hash] = current
	}
	s.stateMu.Unlock()
	if updated {
		s.log.V(1).Info(
			"exclusive obligation tracked",
			"orderHash", obligation.hash.Hex(),
			"quoteId", quoteID,
			"exclusiveUntil", obligation.deadline.Unix(),
		)
	}
}

func (s *Solver) sweepExclusive(ctx context.Context, now time.Time) error {
	s.stateMu.Lock()
	s.cleanupExclusiveLocked(now)
	expired := make([]exclusiveObligation, 0, len(s.exclusiveUntil))
	for hash, tracked := range s.exclusiveUntil {
		if now.After(tracked.deadline) {
			expired = append(expired, exclusiveObligation{
				hash:             hash,
				deadline:         tracked.deadline,
				recoveredAtStart: tracked.recoveredAtStart,
			})
		}
	}
	s.stateMu.Unlock()
	if len(expired) == 0 {
		return nil
	}
	s.log.V(1).Info(
		"exclusive obligations reconciliation started",
		"obligations", len(expired),
		"chainTime", now.Unix(),
	)

	hashes := make([]common.Hash, len(expired))
	for i := range expired {
		hashes[i] = expired[i].hash
	}
	terminals, err := s.orders.ordersByHash(ctx, s.chainID, hashes)
	if err != nil {
		return errors.Errorf("lookup expired obligations: %w", err)
	}
	decisions := make([]exclusiveDecision, 0, len(expired))
	for _, obligation := range expired {
		terminal, ok := terminals[obligation.hash]
		if !ok {
			return errors.Errorf("lookup expired obligation %s: missing result", obligation.hash.Hex())
		}
		decision := exclusiveDecision{
			exclusiveObligation: obligation,
			txHash:              terminal.TxHash,
			status:              terminal.Status,
		}
		switch terminal.Status {
		case orderStatusFilled:
			if terminal.TxHash == (common.Hash{}) {
				return errors.Errorf("lookup expired obligation %s: filled order has no transaction", obligation.hash.Hex())
			}
			filledAt, readErr := s.reader.transactionBlockTimeConfirmed(
				ctx,
				terminal.TxHash,
				s.confirmations,
			)
			if readErr != nil {
				return errors.Errorf("lookup expired obligation %s fill time: %w", obligation.hash.Hex(), readErr)
			}
			decision.filledAt = filledAt
			// Uniswap counts the original quoter as faded whenever exclusivity expires
			// unfilled, even if our executor later wins the public Dutch auction.
			decision.settledInTime = !filledAt.After(obligation.deadline)
		case orderStatusOpen:
			return errors.Errorf(
				"lookup expired obligation %s: order is still open",
				obligation.hash.Hex(),
			)
		case orderStatusExpired, orderStatusError, orderStatusCancelled, orderStatusInsufficientFunds:
			// Only a successful on-chain fill before exclusivity ends discharges the obligation.
			// Every other known lifecycle state means the awarded fill was not delivered in time.
			if terminal.TxHash != (common.Hash{}) {
				return errors.Errorf(
					"lookup expired obligation %s: status %q unexpectedly has transaction %s",
					obligation.hash.Hex(),
					terminal.Status,
					terminal.TxHash.Hex(),
				)
			}
		default:
			return errors.Errorf(
				"lookup expired obligation %s: unknown status %q",
				obligation.hash.Hex(),
				terminal.Status,
			)
		}
		decisions = append(decisions, decision)
	}

	var missed, historicalMissed []exclusiveDecision
	var settled []exclusiveDecision
	s.stateMu.Lock()
	s.cleanupExclusiveLocked(now)
	for _, decision := range decisions {
		tracked, ok := s.exclusiveUntil[decision.hash]
		if !ok || !tracked.deadline.Equal(decision.deadline) {
			continue
		}
		decision.recoveredAtStart = tracked.recoveredAtStart
		delete(s.exclusiveUntil, decision.hash)
		s.exclusiveTerminal[decision.hash] = now
		if decision.settledInTime {
			settled = append(settled, decision)
		} else if decision.recoveredAtStart {
			historicalMissed = append(historicalMissed, decision)
		} else {
			missed = append(missed, decision)
		}
	}
	s.stateMu.Unlock()

	for _, decision := range settled {
		s.observeFill("exclusive-settled-in-time")
		s.log.Info(
			"exclusive order settled before exclusivity ended",
			"orderHash", decision.hash.Hex(),
			"tx", decision.txHash.Hex(),
			"filledAt", decision.filledAt.Unix(),
			"exclusiveUntil", decision.deadline.Unix(),
		)
	}
	for _, decision := range historicalMissed {
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
	s.openExclusiveBreaker(missed, now)
	return nil
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
	s.updateBlockUntilMetric()
	for _, decision := range missed {
		s.observeFill("missed-exclusive")
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

func (s *Solver) cleanupExclusiveLocked(now time.Time) {
	if s.exclusiveUntil == nil {
		s.exclusiveUntil = make(map[common.Hash]trackedExclusive)
	}
	if s.exclusiveTerminal == nil {
		s.exclusiveTerminal = make(map[common.Hash]time.Time)
	}
	for hash, terminalAt := range s.exclusiveTerminal {
		if now.Sub(terminalAt) > time.Hour {
			delete(s.exclusiveTerminal, hash)
		}
	}
}

func (s *Solver) invalidateQuotes() {
	s.quoteEpoch.Add(1)
	s.quoteState.Store(nil)
}

func (s *Solver) beginFillPlanning() {
	s.planningFills.Add(1)
	s.quoteEpoch.Add(1)
	s.quoteState.Store(nil)
}

func (s *Solver) endFillPlanning() {
	remaining := s.planningFills.Add(-1)
	s.quoteEpoch.Add(1)
	if remaining < 0 {
		panic("uniswapx: negative planning fill count")
	}
	s.requestQuoteRefresh()
}
