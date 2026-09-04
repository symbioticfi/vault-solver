package uniswapx

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// orderLedger is the sole owner of execution and exclusivity state for every order hash.
// All transitions are atomic; callers own transport, logging, and readiness side effects.
type orderLedger struct {
	mu      sync.Mutex
	records map[common.Hash]orderLifecycle
}

func (l *orderLedger) claim(hash common.Hash, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupExecutionLocked(now)
	record := l.records[hash]
	order := record.execution
	if !order.completedAt.IsZero() || order.inFlight || order.retryAt.After(now) {
		return false
	}
	order.retryAt = time.Time{}
	order.inFlight = true
	record.execution = order
	l.records[hash] = record
	return true
}

func (l *orderLedger) retry(hash common.Hash, now time.Time, base time.Duration, failed bool) trackedOrder {
	l.mu.Lock()
	defer l.mu.Unlock()
	record := l.records[hash]
	order := record.execution
	order.inFlight = false
	backoff := base
	if failed {
		order.attempts++
		backoff *= time.Duration(1 << min(order.attempts-1, 5))
		backoff = min(backoff, 30*time.Second)
	}
	order.retryAt = now.Add(backoff)
	record.execution = order
	l.records[hash] = record
	return order
}

func (l *orderLedger) complete(hash common.Hash, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ensure()
	record := l.records[hash]
	record.execution = trackedOrder{completedAt: now}
	l.records[hash] = record
}

func (l *orderLedger) trackExclusive(obligation exclusiveObligation, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupExclusiveLocked(now)
	record := l.records[obligation.hash]
	current := record.exclusive
	updated := false
	if current == (trackedExclusive{}) {
		current = trackedExclusive{deadline: obligation.deadline, recoveredAtStart: obligation.recoveredAtStart}
		updated = true
	} else if current.pending() {
		if obligation.deadline.Before(current.deadline) {
			current.deadline = obligation.deadline
			updated = true
		}
		current.recoveredAtStart = current.recoveredAtStart && obligation.recoveredAtStart
	}
	if current.pending() {
		record.exclusive = current
		l.records[obligation.hash] = record
	}
	return updated
}

func (l *orderLedger) expiredExclusive(now time.Time) []exclusiveObligation {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupExclusiveLocked(now)
	expired := make([]exclusiveObligation, 0, len(l.records))
	for hash, record := range l.records {
		tracked := record.exclusive
		if tracked.pending() && now.After(tracked.deadline) {
			expired = append(expired, exclusiveObligation{
				hash: hash, deadline: tracked.deadline, recoveredAtStart: tracked.recoveredAtStart,
			})
		}
	}
	return expired
}

func (l *orderLedger) exclusiveMetrics() (int, int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	count, nearest := 0, int64(0)
	for _, record := range l.records {
		if !record.exclusive.pending() {
			continue
		}
		count++
		deadline := record.exclusive.deadline.Unix()
		if nearest == 0 || deadline < nearest {
			nearest = deadline
		}
	}
	return count, nearest
}

func (l *orderLedger) commitExclusive(now time.Time, decisions []exclusiveDecision) exclusiveReconciliation {
	var outcome exclusiveReconciliation
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupExclusiveLocked(now)
	for _, decision := range decisions {
		record, ok := l.records[decision.hash]
		tracked := record.exclusive
		if !ok || !tracked.pending() || !tracked.deadline.Equal(decision.deadline) {
			continue
		}
		decision.recoveredAtStart = tracked.recoveredAtStart
		tracked.terminalAt = now
		record.exclusive = tracked
		l.records[decision.hash] = record
		if decision.settledInTime {
			outcome.settled = append(outcome.settled, decision)
		} else if decision.recoveredAtStart {
			outcome.historicalMissed = append(outcome.historicalMissed, decision)
		} else {
			outcome.missed = append(outcome.missed, decision)
		}
	}
	return outcome
}

func (l *orderLedger) ensure() {
	if l.records == nil {
		l.records = make(map[common.Hash]orderLifecycle)
	}
}

func (l *orderLedger) cleanupExecutionLocked(now time.Time) {
	l.ensure()
	for hash, record := range l.records {
		execution := record.execution
		expired := (!execution.completedAt.IsZero() && now.Sub(execution.completedAt) > time.Hour) ||
			(!execution.retryAt.IsZero() && now.Sub(execution.retryAt) > time.Hour)
		if !expired {
			continue
		}
		record.execution = trackedOrder{}
		if record.exclusive == (trackedExclusive{}) {
			delete(l.records, hash)
		} else {
			l.records[hash] = record
		}
	}
}

func (l *orderLedger) cleanupExclusiveLocked(now time.Time) {
	l.ensure()
	for hash, record := range l.records {
		if !record.exclusive.terminal() || now.Sub(record.exclusive.terminalAt) <= time.Hour {
			continue
		}
		record.exclusive = trackedExclusive{}
		if record.execution == (trackedOrder{}) {
			delete(l.records, hash)
		} else {
			l.records[hash] = record
		}
	}
}
