package redstoneoev

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

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

// auctionLink returns the span of the bid we sent for this auction. A miss is an ordinary result:
// linking is best effort and never changes what the solver does (spec §12).
func (s *Solver) auctionLink(auctionID string) (trace.Link, bool) {
	if s.links == nil {
		return trace.Link{}, false
	}
	return s.links.Lookup(auctionID)
}

// rememberAuction keeps the auction span linkable for as long as the reservation it created lives, so
// the result frames that arrive minutes later in their own trace can point back at the bid (spec §12.2).
func (s *Solver) rememberAuction(ctx context.Context, auctionID string) {
	if s.links == nil {
		return
	}
	s.links.Remember(ctx, auctionID, reservationTTL)
}

// startResultSpan begins the short span one result frame gets, linked to the auction span that bid on
// it, and stores the auction id (and the linked trace) on the returned context's logger. A link miss is
// recorded as an event naming the key and is otherwise inert.
func (s *Solver) startResultSpan(
	ctx context.Context, name, auctionID string, attrs ...attribute.KeyValue,
) (context.Context, observability.EndFunc) {
	spanAttrs := append([]attribute.KeyValue{observability.AttrAuctionID.String(auctionID)}, attrs...)
	link, linked := s.auctionLink(auctionID)
	var links []trace.Link
	if linked {
		links = []trace.Link{link}
		spanAttrs = append(spanAttrs, observability.AttrQuoteTraceID.String(link.SpanContext.TraceID().String()))
	}
	ctx, end := tracer.StartLinked(ctx, name, links, spanAttrs...)
	if !linked {
		trace.SpanFromContext(ctx).AddEvent("link_miss", trace.WithAttributes(
			attribute.String("key", auctionID),
		))
	}
	log := s.log.WithValues("auctionId", auctionID)
	if linked {
		log = log.WithValues("quoteTraceId", link.SpanContext.TraceID().String())
	}
	return observability.WithLogger(ctx, log), end
}
