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
	ticker := time.NewTicker(s.cfg.QuoteInterval)
	defer ticker.Stop()
	for {
		s.refreshPairRoutes(ctx, tradingRoutes)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Solver) refreshPairRoutes(ctx context.Context, tradingRoutes []route) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.QuoteInterval)
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
