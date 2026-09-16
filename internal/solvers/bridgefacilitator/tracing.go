package bridgefacilitator

import (
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// tracer names every 3F span and stamps solver=3f-bridge-facilitator on it (spec §9).
var tracer = observability.NewTracer(
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator", Name,
)

// attrLinkedOffers counts the offer spans a redeem submission managed to link back to (spec §12).
const attrLinkedOffers = attribute.Key("offer.linked_count")

// offerLinkTTLSlack keeps a submitted offer's span linkable for a while past its expiration, so the
// settlement that follows the last solve still finds it.
const offerLinkTTLSlack = time.Hour

// requestLinkKey keys a submitted offer by the auction's Request contract address — the only
// identifier present both at offer time and on the settlement path, which reads Requests on-chain
// (the 3F API discards the created offer id). Used by redeemReady.
func requestLinkKey(request common.Address) string {
	return "req:" + strings.ToLower(request.Hex())
}

// auctionLinkKey keys the same offer by (adapter, auction), which is what the API's offer listing
// reports. Used by reconcileOffers to stamp quoteTraceId on a listed offer's log lines.
func auctionLinkKey(adapter common.Address, auctionID int64) string {
	return "auction:" + strings.ToLower(adapter.Hex()) + ":" + strconv.FormatInt(auctionID, 10)
}

// offerLink returns the span of the offer submission remembered under key. A miss is an ordinary
// result: linking is best effort and never changes what the solver does (spec §12).
func (s *Solver) offerLink(key string) (trace.Link, bool) {
	if s.links == nil {
		return trace.Link{}, false
	}
	return s.links.Lookup(key)
}

// offerLinks resolves one link per ready Request and reports the keys that missed.
func (s *Solver) offerLinks(ready []common.Address) (links []trace.Link, missed []string) {
	for _, request := range ready {
		key := requestLinkKey(request)
		if link, ok := s.offerLink(key); ok {
			links = append(links, link)
			continue
		}
		missed = append(missed, key)
	}
	return links, missed
}

// listedOfferLogger stamps the trace of the submission this listed offer came from, so an operator
// can join the API's view of an offer back to the pass that created it without the trace backend.
func (s *Solver) listedOfferLogger(log logr.Logger, adapter common.Address, auctionID int64) logr.Logger {
	if auctionID <= 0 {
		return log
	}
	link, ok := s.offerLink(auctionLinkKey(adapter, auctionID))
	if !ok {
		return log
	}
	return log.WithValues("quoteTraceId", link.SpanContext.TraceID().String())
}

// strategyName is the configured strategy's registry key, for the decide stage's strategy.name.
func (s *Solver) strategyName() string {
	if s.cfg == nil || s.cfg.Strategy.Name == "" {
		return defaultStrategyName
	}
	return s.cfg.Strategy.Name
}
