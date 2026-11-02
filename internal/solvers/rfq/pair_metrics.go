package rfq

import (
	"context"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// configuredPairInScope is a structural direction proof, not liquidity or quote availability. It
// never consults request-provided adapter/token lists or performs I/O. Candidate validation remains
// an independent proof for new trusted directions discovered through the existing quote pipeline.
func (qs *quoteService) configuredPairInScope(q *quoteRequest) bool {
	if q.TokenInChainID != qs.chainID || q.TokenOutChainID != qs.chainID {
		return false
	}
	in, out := common.HexToAddress(q.TokenIn), common.HexToAddress(q.TokenOut)
	if in == (common.Address{}) || out == (common.Address{}) || in == out || !qs.tokenPolicy.Allows(in) {
		return false
	}
	routes := qs.pairRoutes.Load()
	if routes == nil {
		return false
	}
	for _, route := range *routes {
		if route.TokenIn == in && route.TokenOut == out && route.ID == liquidlane.NewRouteID(qs.chainID, route.Adapter, in, out) {
			return true
		}
	}
	return false
}

type pairRouteResolver interface {
	resolvePairRoutes(ctx context.Context, adapters []common.Address) ([]liquidlane.Route, error)
}

func (r *reader) resolvePairRoutes(ctx context.Context, adapters []common.Address) ([]liquidlane.Route, error) {
	return r.ll.ResolveRoutes(ctx, adapters)
}

// warmPairRoutes publishes one immutable configured adapter route snapshot for metrics only. The
// existing Run context cancels its optional RPC; neither startup nor HTTP waits for it. Failure leaves
// candidate-validated metrics available and never aborts or changes trading.
func (s *Solver) warmPairRoutes(ctx context.Context) {
	if s.server.metrics == nil || len(s.cfg.Adapters) == 0 {
		return
	}
	resolver, ok := s.exec.reader.(pairRouteResolver)
	if !ok {
		return
	}
	adapters := make([]common.Address, len(s.cfg.Adapters))
	for i, adapter := range s.cfg.Adapters {
		adapters[i] = adapter.Adapter
	}
	go func() {
		routes, err := resolver.resolvePairRoutes(ctx, adapters)
		if err != nil {
			observability.Log(ctx).V(1).Info("optional metric pair routes unavailable", "error", err.Error())
			return
		}
		if ctx.Err() != nil {
			return
		}
		routes = slices.Clone(routes)
		s.server.quotes.pairRoutes.Store(&routes)
	}()
}
