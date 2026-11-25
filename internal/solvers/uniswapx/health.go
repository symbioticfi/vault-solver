package uniswapx

import (
	"fmt"
	"net/http"
	"time"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// exclusiveDeliveryProblem names why exclusive order delivery cannot be trusted, or "" when it can.
func (s *Solver) exclusiveDeliveryProblem(now time.Time) string {
	if s.exclusiveStateUnknown.Load() {
		return "exclusive order state is unknown until a poll reconciles it"
	}
	if s.exclusiveOpenInGrace.Load() {
		return "an exclusive order is still open after its exclusivity ended"
	}
	last := s.lastExclusivePoll.Load()
	if last == 0 {
		return ""
	}
	maxAge := max(3*s.cfg.OrderServer.PollInterval, 5*time.Second)
	if age := now.Sub(time.Unix(last, 0)); age > maxAge {
		return fmt.Sprintf("last exclusive order poll at %d is %s old, max %s", last, age.Truncate(time.Second), maxAge)
	}
	return ""
}

func (s *Solver) markExclusiveStateUnknown() {
	s.exclusiveStateUnknown.Store(true)
	s.invalidateQuotes()
}

func (s *Solver) ready() bool { return s.readyProblem() == "" }

// readyProblem names the first readiness check that fails, or "" when the solver can quote.
func (s *Solver) readyProblem() string {
	now := time.Now()
	epoch := s.quoteEpoch.Load()
	state := s.quoteState.Load()
	if s.lastExclusivePoll.Load() == 0 {
		return "no successful exclusive order poll yet"
	}
	if blocked := s.quoteBlockReason(now.Unix()); blocked != "" {
		return blocked
	}
	switch {
	case state == nil:
		return "no quote state published"
	case len(state.inventory) == 0:
		return "quote state has no inventory"
	case state.epoch != epoch:
		return fmt.Sprintf("quote state epoch %d differs from current epoch %d", state.epoch, epoch)
	case !state.expiresAt.After(now):
		return fmt.Sprintf("quote state expired at %d, now %d", state.expiresAt.Unix(), now.Unix())
	case s.quoteEpoch.Load() != epoch || s.quoteState.Load() != state:
		return "quote state changed during the readiness check"
	}
	return ""
}

func (s *Solver) readyHandler(w http.ResponseWriter, r *http.Request) {
	if problem := s.readyProblem(); problem != "" {
		// The quote port is published to the order provider, so the reason stays in the logs.
		observability.Log(r.Context()).V(1).Info("not ready", "reason", problem)
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
