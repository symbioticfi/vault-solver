package lifi

import (
	"context"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

// Observe each revalidated configured direction once per refresh, irrespective of adapter count.
// Zero capacity retains structural support; inventory cannot revive removed or expired proof.
// Missing inventory is distinct from a strategy declining an available pair.
func (s *Solver) observeQuoteBuildRejections(ctx context.Context, inventory []liquidlane.Inventory, quotes []types.Quote, reason string, unavailableDiscounts ...liquidlane.Inventory) {
	seen := make(map[[2]common.Address]bool)
	observe := func(r route, hasInventory bool) {
		if !s.cfg.TokenPolicy.Allows(r.TokenIn) {
			return
		}
		pair := [2]common.Address{r.TokenIn, r.TokenOut}
		if seen[pair] {
			return
		}
		seen[pair] = true
		pairReason := reason
		if pairReason == "" {
			for _, q := range quotes {
				if q.FromAsset == r.TokenIn && q.ToAsset == r.TokenOut {
					return
				}
			}
			pairReason = "no_inventory"
			if hasInventory {
				pairReason = "strategy_declined"
			} else if slices.ContainsFunc(unavailableDiscounts, func(base liquidlane.Inventory) bool {
				return base.TokenIn == r.TokenIn && base.TokenOut == r.TokenOut
			}) {
				pairReason = "discount_api_error"
			}
		}
		s.observeStandingQuoteRejection(ctx, r.TokenIn, r.TokenOut, pairReason, inventory)
	}
	for _, item := range inventory {
		observe(item.Route, true)
	}
	for _, r := range s.currentPairRoutes() {
		observe(r, false)
	}
}

// Failed submission is an observation of a local attempt; remote acceptance may be uncertain.
// Withdrawals are separate from publication, including when both occur in one batch.
func (s *Solver) observeQuoteSubmitError(ctx context.Context, quotes []types.Quote, inventory []liquidlane.Inventory) {
	for _, reason := range []string{"publish_error", "withdrawal_error"} {
		seen := make(map[[2]common.Address]bool)
		for _, q := range quotes {
			if (len(q.Ranges) == 0) != (reason == "withdrawal_error") {
				continue
			}
			pair := [2]common.Address{q.FromAsset, q.ToAsset}
			if !seen[pair] {
				seen[pair] = true
				s.observeStandingQuoteRejection(ctx, q.FromAsset, q.ToAsset, reason, inventory)
			}
		}
	}
}

func (s *Solver) observeStandingQuoteRejection(ctx context.Context, in, out common.Address, reason string, inventory []liquidlane.Inventory) {
	source := s.pairScopeSource(in, out, inventory)
	if source == "" {
		return
	}
	reason = boundedStandingQuoteReason(reason)
	if s.metrics != nil {
		s.metrics.pairs.Standing(s.chainID, in, out, "rejected", reason)
	}
	observability.Log(ctx).WithValues(s.tokenMetadata.PairLogFields(in, out)...).V(1).Info("standing quote rejected", "chainId", s.chainID, "reason", reason, "tokenIn", in.Hex(), "tokenOut", out.Hex(), "pairInScope", true, "pairScope", "directed_route", "pairScopeSource", source)
}

func boundedStandingQuoteReason(reason string) string {
	switch reason {
	case "lane_unavailable", "chain_time_error", "snapshot_error", "gas_price_error", "strategy_error",
		"pair_out_of_scope", "no_inventory", "discount_api_error", "strategy_declined", "publish_error", "withdrawal_error":
		return reason
	default:
		return "unknown"
	}
}

// Revalidated configured adapter direction survives zero capacity, but expires on discovery
// failure and is replaced on removal. Old inventory cannot resurrect withdrawn route proof.
func (s *Solver) pairScopeSource(in, out common.Address, inventory []liquidlane.Inventory) string {
	if s.cfg == nil || !s.cfg.TokenPolicy.Allows(in) {
		return ""
	}
	for _, route := range s.currentPairRoutes() {
		if route.TokenIn == in && route.TokenOut == out {
			for _, item := range inventory {
				if item.Route == route {
					return "current_inventory"
				}
			}
			return "current_configured_route"
		}
	}
	return ""
}

// Count successful changed records only, including explicit empty-range withdrawals. This is
// acknowledgement by the standing-quote server, not an observed response to a client quote request.
func (s *Solver) observeStandingQuoteSubmission(quotes []types.Quote, inventory []liquidlane.Inventory) {
	if s.metrics == nil {
		return
	}
	for _, q := range quotes {
		if s.pairScopeSource(q.FromAsset, q.ToAsset, inventory) == "" {
			continue
		}
		result := "published"
		if len(q.Ranges) == 0 {
			result = "withdrawn"
		}
		s.metrics.pairs.Standing(s.chainID, q.FromAsset, q.ToAsset, result, "")
	}
}

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
