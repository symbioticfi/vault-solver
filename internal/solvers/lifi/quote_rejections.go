package lifi

import (
	"context"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

// Count a pair once per failed build evaluation, irrespective of routes, discounts or ranges.
// A missing strategy output has no proven liquidity/capacity cause, so retain strategy_declined.
func (s *Solver) observeQuoteBuildRejections(ctx context.Context, routes []route, inventory []liquidlane.Inventory, quotes []types.Quote, reason string) {
	if s.metrics == nil {
		return
	}
	seen := make(map[[2]common.Address]bool)
	for _, r := range routes {
		pair := [2]common.Address{r.TokenIn, r.TokenOut}
		if seen[pair] {
			continue
		}
		seen[pair] = true
		pairReason := reason
		if pairReason == "" {
			quoted := false
			for _, q := range quotes {
				quoted = quoted || q.FromAsset == r.TokenIn && q.ToAsset == r.TokenOut
			}
			if quoted {
				continue
			}
			pairReason = "no_inventory"
			for _, item := range inventory {
				if item.TokenIn == r.TokenIn && item.TokenOut == r.TokenOut {
					pairReason = "strategy_declined"
					break
				}
			}
			if !s.cfg.TokenPolicy.Allows(r.TokenIn) {
				pairReason = "pair_out_of_scope"
			}
		}
		s.observeStandingQuoteRejection(ctx, r.TokenIn, r.TokenOut, pairReason)
	}
}

// Failed submission is an observation of a local attempt; remote acceptance may be uncertain.
// Withdrawals are separate from publication, including when both occur in one batch.
func (s *Solver) observeQuoteSubmitError(ctx context.Context, quotes []types.Quote) {
	for _, reason := range []string{"publish_error", "withdrawal_error"} {
		seen := make(map[[2]common.Address]bool)
		for _, q := range quotes {
			if (len(q.Ranges) == 0) != (reason == "withdrawal_error") {
				continue
			}
			pair := [2]common.Address{q.FromAsset, q.ToAsset}
			if !seen[pair] {
				seen[pair] = true
				s.observeStandingQuoteRejection(ctx, q.FromAsset, q.ToAsset, reason)
			}
		}
	}
}

func (s *Solver) observeStandingQuoteRejection(ctx context.Context, in, out common.Address, reason string) {
	if s.metrics == nil {
		return
	}
	switch reason {
	case "lane_unavailable", "chain_time_error", "snapshot_error", "gas_price_error", "strategy_error",
		"pair_out_of_scope", "no_inventory", "strategy_declined", "publish_error", "withdrawal_error":
	default:
		reason = "unknown"
	}
	// Only a pair from immutable startup routes can enter labels. No webhook/request/error data
	// grows this set. The bound is the configured route-pair count plus one unresolved bucket.
	inLabel, outLabel := "other", "other"
	for _, r := range s.metrics.quotes.routes {
		if r.TokenIn == in && r.TokenOut == out {
			inLabel, outLabel = strings.ToLower(in.Hex()), strings.ToLower(out.Hex())
			break
		}
	}
	s.metrics.quotes.rejections.WithLabelValues(inLabel, outLabel, reason).Inc()
	observability.Log(ctx).V(1).Info("standing quote rejected", "reason", reason, "tokenIn", in.Hex(), "tokenOut", out.Hex())
}
