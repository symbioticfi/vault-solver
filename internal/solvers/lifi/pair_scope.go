package lifi

import (
	"context"
	"slices"
	"time"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

type pairRouteScope struct {
	routes    []route
	expiresAt time.Time
}

// Keep the trading route slice unchanged. Optional discovery only retires observational proof
// when a configured adapter no longer advertises the same direction/units. New trading routes
// still require the existing startup lifecycle; zero capacity never removes structural proof.
func (s *Solver) observePairRoutes(ctx context.Context, tradingRoutes []route) {
	// Wait and read each get one third of the existing proof TTL. This leaves renewal
	// headroom without tying full discovery to the much faster block polling cadence.
	interval := s.cfg.QuoteTTL / 3
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.refreshPairRoutes(ctx, tradingRoutes)
			// Space attempts after completion; a slow read must not cause catch-up bursts.
			timer.Reset(interval)
		}
	}
}

func (s *Solver) refreshPairRoutes(ctx context.Context, tradingRoutes []route) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.QuoteTTL/3)
	defer cancel()
	current, err := s.reader.resolveRoutes(ctx, s.cfg.Adapters)
	if err != nil {
		// A failed read is unknown, not proof of removal. Do not renew its old expiry.
		observability.Log(ctx).V(1).Info("optional pair scope unavailable", "error", err.Error())
		return
	}
	current = slices.DeleteFunc(slices.Clone(current), func(r route) bool { return !slices.Contains(tradingRoutes, r) })
	s.setPairRoutes(current)
}

func (s *Solver) setPairRoutes(routes []route) {
	s.pairRoutes.Store(&pairRouteScope{routes: slices.Clone(routes), expiresAt: time.Now().Add(s.cfg.QuoteTTL)})
}

func (s *Solver) currentPairRoutes() []route {
	scope := s.pairRoutes.Load()
	if scope == nil || !scope.expiresAt.After(time.Now()) {
		return nil
	}
	return scope.routes
}
