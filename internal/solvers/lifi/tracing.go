package lifi

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// tracer names every LI.FI span and stamps solver=lifi-samechain on it (spec §9).
var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/lifi", Name)

// Retry stages of one order. Each pop off a retry queue re-enters planning under a fresh stage span
// of the order's processing span, numbered with tx.attempt (spec §9.4).
const (
	orderReserveStage = "lifi.order.reserve"
	orderDepositStage = "lifi.order.deposit"
)

// orderMessageSpanName keeps the message span name bounded (spec §9.3). The feed dispatches only the
// submit event, so anything else is named generically instead of echoing the wire value.
func orderMessageSpanName(event string) string {
	if event != orderSubmitEvent {
		return "lifi.order.other"
	}
	return "lifi.order." + orderSubmitEvent
}

// orderTrace is one order's open lifi.order.process span plus its per-stage attempt counters. The
// span is held rather than a context: the worker's context outlives no single attempt, so each call
// site derives its own context from the one it already has.
type orderTrace struct {
	order    *submittedOrder
	span     trace.Span
	end      observability.EndFunc
	attempts map[string]int
}

// context returns ctx carrying the order's processing span.
func (t *orderTrace) context(ctx context.Context) context.Context {
	return trace.ContextWithSpan(ctx, t.span)
}

// attempt starts a retry stage span under the processing span and numbers the attempt. The order's
// first pass through the worker has no retry stage and runs directly under the processing span.
func (t *orderTrace) attempt(ctx context.Context, stage string) (context.Context, observability.EndFunc) {
	if stage == "" {
		return ctx, func(error) {}
	}
	t.attempts[stage]++
	return tracer.Start(ctx, stage, observability.AttrTxAttempt.Int(t.attempts[stage]))
}

// orderTraces keeps one lifi.order.process span open per order for as long as the worker is working
// on it — across deposit propagation and capacity retries — and ends it exactly once when the order
// reaches a terminal state. Owned by the single order-worker goroutine.
type orderTraces struct {
	byKey map[string]*orderTrace
	// held names orders re-queued through the inbox for a recovery retry, with the recovery epoch
	// they joined. They are still referenced even though no worker queue holds them, so their span
	// stays open until the redelivery that begins the next attempt, or until that recovery ends
	// without one.
	held map[string]uint64
}

// orderAttrs identifies one order on a span: the same three keys wherever an order is described.
func orderAttrs(order *submittedOrder) []attribute.KeyValue {
	return []attribute.KeyValue{
		observability.AttrOrderID.String(order.OrderID),
		observability.AttrOrderOnchainID.String(order.OnChainOrderID),
		observability.AttrQuoteID.String(order.QuoteID),
	}
}

func newOrderTraces() *orderTraces {
	return &orderTraces{byKey: make(map[string]*orderTrace), held: make(map[string]uint64)}
}

// hold keeps the order's processing span open across an inbox re-queue, so the retry that comes
// back on the next recovery sweep continues this trace instead of opening a second one.
func (t *orderTraces) hold(order *submittedOrder, epoch uint64) {
	if key := orderInboxKey(order); key != "" && t.byKey[key] != nil {
		t.held[key] = epoch
	}
}

// isHeld reports whether an inbox re-queue still references the order.
func (t *orderTraces) isHeld(order *submittedOrder) bool {
	_, held := t.held[orderInboxKey(order)]
	return held
}

// dropHolds forgets every hold whose inbox re-queue dropped reports gone.
func (t *orderTraces) dropHolds(dropped func(key string, epoch uint64) bool) {
	for key, epoch := range t.held {
		if dropped(key, epoch) {
			delete(t.held, key)
		}
	}
}

// begin returns the order's processing span, starting it as a child of the feed message span the
// order carries when this is the order's first pass through the worker.
func (t *orderTraces) begin(ctx context.Context, order *submittedOrder) *orderTrace {
	key := orderInboxKey(order)
	delete(t.held, key)
	if existing := t.byKey[key]; existing != nil {
		return existing
	}
	spanCtx, end := tracer.Start(
		trace.ContextWithSpanContext(ctx, order.span), "lifi.order.process", orderAttrs(order)...,
	)
	tracked := &orderTrace{
		order: order, span: trace.SpanFromContext(spanCtx), end: end, attempts: make(map[string]int),
	}
	// A no-op span has nothing to keep open across attempts.
	if observability.TracingEnabled() {
		t.byKey[key] = tracked
	}
	return tracked
}

// context returns the order's processing span context, or ctx when the order is not being tracked.
func (t *orderTraces) context(ctx context.Context, order *submittedOrder) context.Context {
	if tracked := t.byKey[orderInboxKey(order)]; tracked != nil {
		return tracked.context(ctx)
	}
	return ctx
}

// finish ends the order's processing span. Ending an order that is not tracked is a no-op, so every
// terminal path may call it.
func (t *orderTraces) finish(order *submittedOrder, err error) {
	key := orderInboxKey(order)
	tracked := t.byKey[key]
	if tracked == nil {
		return
	}
	delete(t.byKey, key)
	delete(t.held, key)
	tracked.end(err)
}

// abandon ends the processing span of every order the worker dropped without finishing it, which is
// every tracked order referenced no longer claims. The drop is not the order's failure: it records an
// abandoned decline naming reason, and ends with cause, the cancellation when a shutdown dropped it.
func (t *orderTraces) abandon(
	ctx context.Context, referenced func(*submittedOrder) bool, reason string, cause error,
) {
	for key, tracked := range t.byKey {
		if referenced(tracked.order) {
			continue
		}
		delete(t.byKey, key)
		delete(t.held, key)
		observability.Decline(tracked.context(ctx), "abandoned", reason)
		tracked.end(cause)
	}
}

// finishAll ends every still-open processing span, so a shutdown that drops queued retries leaves
// no span unexported.
func (t *orderTraces) finishAll(err error) {
	for key, tracked := range t.byKey {
		delete(t.byKey, key)
		delete(t.held, key)
		tracked.end(err)
	}
}
