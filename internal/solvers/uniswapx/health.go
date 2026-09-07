package uniswapx

import (
	"net/http"
	"time"
)

func (s *Solver) exclusiveDeliveryHealthy() bool {
	if s.exclusiveStateUnknown.Load() {
		return false
	}
	observed := s.lastExclusivePoll.Load()
	if observed == 0 {
		return true
	}
	expiry := time.Unix(observed, 0).Add(max(3*s.cfg.OrderServer.PollInterval, 5*time.Second))
	return !time.Now().After(expiry)
}

func (s *Solver) markExclusiveStateUnknown() {
	s.exclusiveStateUnknown.Store(true)
	s.invalidateQuotes()
}

func (s *Solver) ready() bool {
	now := s.quoteClock()
	if s.lastExclusivePoll.Load() <= 0 || s.quoteBlocked(now.Unix()) {
		return false
	}
	snapshot := s.quotes.current()
	return s.currentQuoteSnapshot(snapshot, now) && len(snapshot.inventory) > 0
}

func (s *Solver) currentQuoteSnapshot(snapshot *quoteState, now time.Time) bool {
	return snapshot != nil && snapshot.expiresAt.After(now) && snapshot == s.quotes.current()
}

func (s *Solver) readyHandler(w http.ResponseWriter, _ *http.Request) {
	if !s.ready() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
