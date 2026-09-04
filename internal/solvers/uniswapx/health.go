package uniswapx

import (
	"net/http"
	"time"
)

func (s *Solver) exclusiveDeliveryHealthy() bool {
	if s.breaker.exclusiveUnknown.Load() {
		return false
	}
	last := s.breaker.lastExclusivePoll.Load()
	if last == 0 {
		return true
	}
	maxAge := max(3*s.cfg.OrderServer.PollInterval, 5*time.Second)
	return time.Since(time.Unix(last, 0)) <= maxAge
}

func (s *Solver) markExclusiveStateUnknown() {
	s.breaker.exclusiveUnknown.Store(true)
	s.invalidateQuotes()
}

func (s *Solver) ready() bool {
	now := time.Now()
	lastPoll := s.breaker.lastExclusivePoll.Load()
	epoch := s.quoteEpoch.Load()
	state := s.quoteState.Load()
	return lastPoll > 0 && !s.quoteBlocked(now.Unix()) &&
		state != nil && len(state.inventory) > 0 &&
		state.epoch == epoch && state.expiresAt.After(now) &&
		s.quoteEpoch.Load() == epoch && s.quoteState.Load() == state
}

func (s *Solver) readyHandler(w http.ResponseWriter, _ *http.Request) {
	if !s.ready() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
