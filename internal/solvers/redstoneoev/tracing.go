package redstoneoev

import (
	"context"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// tracer names every RedStone span and stamps solver=redstone-oev on it (spec §9).
var tracer = observability.NewTracer(
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev", Name,
)

// attrWon records whether the auction's winning liquidator was our callback.
const attrWon = attribute.Key("oev.won")

// strategyLabel is the configured strategy's registry key, for the bid span's strategy.name.
func (s *Solver) strategyLabel() string {
	if s.strategyName == "" {
		return defaultStrategyName
	}
	return s.strategyName
}

// auctionLogger narrows the solver logger to one auction, the key every line on an auction's path
// carries.
func (s *Solver) auctionLogger(auctionID string) logr.Logger {
	return s.log.WithValues("auctionId", auctionID)
}

// startResultSpan begins the short span one result frame gets, linked to the auction span that bid on
// it, and stores the auction id (and the linked trace) on the returned context's logger. A link miss is
// recorded as an event naming the key and is otherwise inert.
func (s *Solver) startResultSpan(
	ctx context.Context, name, auctionID string, attrs ...attribute.KeyValue,
) (context.Context, observability.EndFunc) {
	ctx = observability.WithLogger(ctx, s.auctionLogger(auctionID))
	ctx, end, _ := tracer.StartLinkedKey(ctx, s.links, auctionID, name,
		append([]attribute.KeyValue{observability.AttrAuctionID.String(auctionID)}, attrs...)...)
	return ctx, end
}
