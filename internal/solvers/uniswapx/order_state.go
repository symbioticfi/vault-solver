package uniswapx

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type executionPhase uint8

const (
	executionRetry executionPhase = iota
	executionClaimed
	executionFilled
)

// One record owns deduplication, retry timing and attempt history. stateMu guards
// every transition because polling claims work while the fill loop completes it.
type executionState struct {
	phase    executionPhase
	until    time.Time
	attempts int
}

func (s *Solver) claim(hash common.Hash, now time.Time) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.executions == nil {
		s.executions = make(map[common.Hash]executionState)
	}
	state, exists := s.executions[hash]
	if exists && (state.phase != executionRetry || state.until.After(now)) {
		return false
	}
	state.phase, state.until = executionClaimed, time.Time{}
	s.executions[hash] = state
	s.beginFillPlanning()
	return true
}

func (s *Solver) retry(hash common.Hash, now time.Time, failed bool) {
	s.stateMu.Lock()
	if s.executions == nil {
		s.executions = make(map[common.Hash]executionState)
	}
	state := s.executions[hash]
	delay := s.cfg.OrderServer.PollInterval
	if failed {
		state.attempts++
		delay = min(delay*time.Duration(1<<min(state.attempts-1, 5)), 30*time.Second)
	}
	state.phase, state.until = executionRetry, now.Add(delay)
	s.executions[hash] = state
	s.stateMu.Unlock()
	s.log.V(1).Info("order retry scheduled", "orderHash", hash.Hex(), "failed", failed,
		"attempt", state.attempts, "backoff", delay, "retryAt", state.until.Unix())
}

func (s *Solver) complete(hash common.Hash, now time.Time) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.executions == nil {
		s.executions = make(map[common.Hash]executionState)
	}
	s.executions[hash] = executionState{phase: executionFilled, until: now}
}

// Cleanup runs once per source snapshot, before its orders are claimed. Active
// fills and unresolved exclusive obligations survive any age. Completed history
// retains its existing execution TTL or exclusive recovery lookback.
func (s *Solver) cleanupOrderHistory(now time.Time) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	for key, state := range s.executions {
		if state.phase != executionClaimed && now.Sub(state.until) > time.Hour {
			delete(s.executions, key)
		}
	}
	for hash, obligation := range s.obligations {
		if !obligation.resolvedAt.IsZero() && now.Sub(obligation.resolvedAt) > s.exclusiveRecoveryLookback() {
			delete(s.obligations, hash)
		}
	}
}
