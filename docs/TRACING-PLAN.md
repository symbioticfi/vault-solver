# Tracing

This is the source of truth for distributed tracing across the whole process: the framework helpers in
`internal/observability`, the boundaries that start spans, the naming and attribute contract every
solver follows, and the best-effort quote-to-fill links. Per-solver plans describe their own pipelines
and link here; the [README](../README.md) remains the operator entry point for the environment
variables. Implementation lives in
[tracing.go](../internal/observability/tracing.go),
[tracer.go](../internal/observability/tracer.go),
[httptrace.go](../internal/observability/httptrace.go) and
[spanlinks.go](../internal/observability/spanlinks.go).

## 1. Purpose

Every unit of I/O the bot performs is a span in one trace, and every log line written under that work
carries the trace id. A quote request the RFQ backend sends us arrives with a W3C `traceparent`; the
trace continues through the strategy, the webhook, the backend client, the JSON-RPC provider and the
transaction lifecycle, and each outbound call carries the headers so the next hop can continue it.

The posture mirrors the RFQ backend, which gets the same behaviour from `@symbiotic/backend-devkit`
(Node SDK, auto-instrumentation, OTLP/HTTP exporter, log correlation): configured only by `OTEL_*`
environment variables and never by YAML, off unless the devkit's switch is truthy, one span per
inbound request, per outbound HTTP call, per RPC call and per transaction, and trace ids on every log
line. Go has no auto-instrumentation, so the equivalent is wired by hand at the repo's existing seams.

Out of scope, deliberately: exporting metrics or logs over OTLP (Prometheus and zap JSON logs stay as
they are) and tracing the observability listener (probes are noise). See §10.

## 2. Enablement and configuration

All configuration is environment-driven and read by the OpenTelemetry Go SDK itself, except the on/off
switch. There is no `observability:` config field for tracing: deployment-specific endpoints stay out
of the YAML, matching `SENTRY_DSN` and the backend.

| Variable | Effect |
|---|---|
| `OTEL_EXPORTER_ENABLED` | The switch, with the devkit's accepted values: `1`, `true`, `yes`, `on`, `enabled` turn tracing on; anything else, including unset, leaves it off. Case and surrounding space are ignored. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` or `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Collector address; SDK default `http://localhost:4318`. Same OTLP/HTTP protobuf wire format the backend exports. |
| `OTEL_SERVICE_NAME` | Defaults to `vault-solver`. Deployments run one solver per process, so operators set e.g. `vault-solver-rfq`. |
| `OTEL_RESOURCE_ATTRIBUTES` | Merged into the resource, and wins over the defaults below. |
| `OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG` | SDK default `parentbased_always_on`, same as the backend. `parentbased_traceidratio` is the knob for taming root spans from background loops without dropping backend-parented traces. |
| `OTEL_PROPAGATORS` | Not read. The propagator is always W3C `tracecontext` + `baggage`, the Node SDK default the backend runs with. |
| `OTEL_EXPORTER_OTLP_HEADERS`, `_TIMEOUT`, `_COMPRESSION`, `OTEL_BSP_*` | Read by the exporter and the batch span processor as usual. |

`OTEL_TRACES_EXPORTER` and `OTEL_EXPORTER_OTLP_PROTOCOL` are ignored: the exporter is always OTLP over
HTTP. `OTEL_EXPORTER_OTLP_PROTOCOL=grpc` is not honored.

Resource attributes always set: `service.name` (default above), `service.version` (`version.Version`),
`vault_solver.commit` (`version.Commit`), `vault_solver.solvers` (comma-joined configured solver
names), `chain.id` (`cfg.Chain.ChainID`), plus the SDK's telemetry attributes.

What the backend sends and expects: the `/quote` call to a solver carries `traceparent` (and
`tracestate`, `baggage` when present) but no `X-Request-Id`, so the solver's middleware keeps minting
its own request id. Every backend response echoes or mints an `X-Request-Id`; that is a request id,
not a trace id, which is why §6 links quotes to fills solver-side.

## 3. Architecture

### 3.1 Startup

`observability.NewTracing(ctx, info, log) (shutdown, enabled)` runs in `cmd/vault-solver/run.go` right
after the logger is built, and **never fails startup**. It always installs the composite W3C
`TraceContext` + `Baggage` propagator, so even with tracing disabled an inbound `traceparent` is
carried through the context, forwarded on outbound calls and stamped on logs, while nothing is
recorded or exported. When the switch is truthy it builds an OTLP/HTTP exporter and a batching
`TracerProvider` from the environment and registers it globally; an exporter or resource failure is
logged at Error and tracing simply stays disabled. `otel.SetErrorHandler` logs export failures at
Info, so a dead collector is visible without paging. The startup line carries `tracing: true|false`.
Shutdown flushes on a fresh 5 s context after the solvers and txmanager have drained.

### 3.2 The tracer wrapper

Every package takes its own tracer with `observability.NewTracer(importPath, solverName)` and starts
spans through it:

```go
var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/rfq", Name)

ctx, end := tracer.Start(ctx, "rfq.quote.decide", observability.AttrStrategy.String(name))
defer func() { end(err) }()
```

- `Start` stamps `solver=<name>` on every span it creates, the way `run.go` stamps the logger, so any
  span can be filtered by integration without joining on the resource. Shared components (txmanager,
  chain) pass `""` and carry no `solver` attribute.
- `Tracer.Raw()` resolves the provider lazily and then caches it against a package-level generation
  counter that `NewTracing` (and the test helper) bump after `otel.SetTracerProvider`. Resolving per
  span start takes the global delegate's mutex while tracing is disabled, and the SDK's tracer-map
  lock once a real provider is installed; the generation cache keeps both off the per-span path.
  Resolving once in `NewTracer` would instead bind a package's tracer to the no-op provider forever,
  because the global provider delegates only once. `observability.InvalidateTracers()` is the hook
  anything else that installs a provider must call.
- `end(err)` sets status `Error`, records the error with its go-errors stack, and adds `reason_code`
  when the error exposes one. A `context.Canceled` error ends the span with status unset and a
  `cancelled` event, the same rule the metrics use for skipped outcomes. `end` is idempotent.
- `observability.Decline(ctx, decision, reason)` records an expected non-error outcome (no quote, not
  profitable, adapter paused, order already filled) as a `declined` event and leaves the status unset.
- `observability.SetAttributes(ctx, ...)` adds an id that becomes known mid-stage, such as a
  transaction hash after `Send` returns.
- `StartLinked` adds links to earlier spans (§6); `Tracer.Raw()` is for the rare span that must be
  held across goroutines rather than scoped to a function (txmanager's send span).

### 3.3 Boundaries

**Inbound HTTP.** `observability.TraceHandler(next, route)` wraps `otelhttp` outermost around each
quote server's existing chain, extracting the caller's trace context. The span is
`"<METHOD> <route>"` from the server's own bounded route allowlist, never the raw path; probe and docs
paths (`/health`, `/healthz`, `/ready`, `/readyz`, `/metrics`, `/docs`, `/openapi*`) produce no span.
Only the RFQ and UniswapX quote servers are traced. The observability listener is untraced by design.

**Outbound HTTP.** `observability.TraceTransport(base, peer)` is the outermost RoundTripper on every
`*http.Client` the bot builds, so the span covers the existing wrappers and `traceparent` goes out on
the wire. The span is `"<peer> <METHOD>"` with a `peer.service` attribute; `peer` is a short
integration name, never a URL. Generated OpenAPI and GraphQL clients are untouched — they receive the
client as configuration. `otelhttp` records the request URL as `url.full` and has no option to omit
it, so `TraceTransport` hands it a clone whose query string is cleared and restores the real URL on
the request that goes on the wire: an operator-configured peer URL (the webhook strategy's) may carry
a token in the query, and a span attribute is not the place for it. Wired peers: `rfq-backend`
(which also covers the internal discounts calls
made with the same client), `rfq-discounts`, `3f-api`, `lifi-order-server`, `uniswapx-api`,
`morpho-graphql`, `webhook`. A webhook strategy call is therefore a child span of the quote or fill
that triggered it, and the operator's receiver gets `traceparent`.

**Ethereum JSON-RPC.** `internal/chain` instruments `fallbackTransport.RoundTrip` directly rather than
through `otelhttp`, so there is exactly one span per logical JSON-RPC request no matter how many
endpoints are attempted. The span is named by the bounded method (`batch` for batches), is a client
span, and carries `rpc.system=jsonrpc`, `rpc.method`, `rpc.jsonrpc.request_id`, `chain.rpc.role`
(`read`/`write`/`shared`) and `chain.rpc.batch`. Each endpoint attempt adds an `attempt` event with
the role-local **ordinal** and its classified outcome — never the URL, the same rule the metrics
follow. Every attempt's request gets `traceparent` injected, so the RPC provider can continue the
trace. The span ends exactly where the metrics observation finishes: on response-body close, or
immediately on a transport-level failure. Status is Error for every outcome other than success.
Because the chain client is shared by all solvers, RPC spans carry no `solver` attribute; they inherit
whichever span is in the caller's context. `rpc.jsonrpc.request_id` is empty for batches (§10).

**Non-HTTP JSON-RPC (`ws://`, `wss://`, IPC).** These transports never reach `fallbackTransport`, so
the spans are produced on our side of the client instead. The dial itself is a `chain.rpc.connect`
client span carrying `chain.rpc.role` and `chain.rpc.transport` (`ws` or `ipc`); a failed dial ends it
with status Error. A websocket handshake is the one place this transport can carry a header, so
`traceparent`/`tracestate`/`baggage` are injected into the upgrade request from that connect span's
context — go-ethereum reconnects a dropped socket internally and replays the same headers, so the
provider can tie the connection back to the dial but **never to an individual call**, and IPC has no
handshake at all. Each call is then spanned locally: `internal/chain/calls.go` shadows exactly the
backend methods this repo calls (`CallContract`, `HeaderByNumber`, `HeaderByHash`, `FeeHistory`,
`SuggestGasTipCap`, `EstimateGas`, `TransactionReceipt`, `BalanceAt`, `CodeAt`, `BlockNumber`,
`SendTransaction`, `NonceAt`, `PendingNonceAt`, `TransactionSenderBalanceAt`) and starts a client span
named by the JSON-RPC method with `rpc.system=jsonrpc`, `rpc.method`, `chain.rpc.role` and
`chain.rpc.transport`, so dashboards see one series across transports. A cancelled call and an
`ethereum.NotFound` (the null result a node returns for an unmined transaction or an unknown block)
end the span with an event and no Error status: over HTTP the same response classifies as a success,
and an unmined transaction is the txmanager's steady state, not a fault. `Multicall` is not spanned: it
reaches the chain through `CallContract`, which is where its `eth_call` span belongs. Read and write
endpoints are labelled separately, since only one of the two may be non-HTTP. On the HTTP path the
shadowed methods are plain passthroughs and the transport's spans are the only RPC spans — a new call
site through the client needs a new shadow or it goes untraced on websocket and IPC.

**WebSocket feeds.** Both feeds dial under a short `"<feed>.feed.connect"` span and inject
`traceparent` into the handshake headers, so a connection is findable in the trace backend. Per-message
handling is described per solver in §5.

### 3.4 txmanager

`Request` did not change and no submission site changed. `sendAsync` starts the transaction span
`"txmanager.send <label>"` from the caller's context, so it is an ordinary child of the submitting
fill span, and hands the span to the worker on the internal job. Starting it in `sendAsync` means the
span **covers the admission wait** as well as the broadcast. The worker continues that span on its own
contexts with `trace.ContextWithSpan`, legal even after the caller's context is cancelled, so the span
survives the manager's deliberate detachment and covers admission → broadcast → terminal outcome.

Children: `txmanager.broadcast` (fee quote, gas estimate, nonce, sign, send — each RPC call becomes a
grandchild automatically) and one `txmanager.replace` per replacement carrying `tx.attempt` and
`tx.cancellation`. Receipt polls are ordinary RPC child spans. Attributes: `solver` (from
`Request.Solver`), `tx.label`, `tx.hash` and `tx.nonce` once known, and terminal `tx.outcome`; status
is Error for `reverted`, `cancelled`, `submission_error` and `tracking_stopped`, and unset for
`confirmed` and `included_unconfirmed`. The send span **ends before the result is delivered** to the
caller, so a caller resuming its own trace never races the span it nests under. A send declined because
the lane is busy gets a `declined` event with `decision=not_admitted`, `reason=lane_busy`, and ends
without a `tx.outcome` (§10). `txmanager.account_poll` roots each account-poll tick.

The worker stores the request's `solver`-stamped logger on every context of the lifecycle, the send
span included, so `observability.Log(ctx)` puts `solver`, `label`, `trace_id` and `span_id` on every
lifecycle line — including the lines written long after the broadcast span has ended, from the
detached lifecycle goroutine.

### 3.5 Root spans for loops

Work that is not triggered by an inbound request needs a root span, or each of its outbound calls
becomes its own one-span trace. The rule: **every periodic tick or event handler that performs I/O
starts a span named `<solver>.<loop>`**, and its logs go through `observability.Log(ctx)` so they
carry that span. In-process hops that cross a channel carry a `trace.SpanContext` on the queued
struct (`resolvedOrder`, `submittedOrder`) so the consumer's span continues the producer's trace;
nothing else is stored in those structs.

### 3.6 Logs and Sentry

The base logger travels in the context. The rule: **log through `observability.Log(ctx)` wherever a
context is in hand** — it reads the context's logger and stamps the current span's `trace_id` and
`span_id` on it, falling back to the process logger `SetDefaultLogger` installed so no line is ever
dropped. `main` seeds the root context and `solver.Run` replaces it with the per-solver logger; any
scope that wants a narrower one stores it with `observability.WithLogger(ctx, log.WithValues(...))`
and passes that context down.

What is stored must always be the **base, non-trace logger**: `Log` stamps at retrieval, so storing a
stamped one would emit two `trace_id` pairs (`logr` appends key/values and cannot dedupe) and would
pin the outer span's `span_id` on everything below. `observability.TraceLogger(ctx, log)` remains for
the few places that hold a logger no context can carry, such as the RPC fallback transport. The
Sentry sink promotes `trace_id` to an event tag next to `solver`, `logger` and `label`.

## 4. Span naming and attribute conventions

Span names are bounded by construction: an allowlisted route, an allowlisted RPC method, or a literal
from the tables in §5. Addresses, hashes and ids are **attributes, never span names**. A stage span is
named `<solver>.<pipeline>.<stage>`; the rule for new code is that a function that does I/O, runs a
strategy, or is a named step in a plan document's data flow gets a span. Strategy calls are always
their own stage carrying `strategy.name`.

Identifier attributes use **neutral keys** shared across solvers, because the `solver` attribute
already says which integration a span belongs to and an operator searching for a quote id should not
need to know the solver's prefix. Log lines keep their existing camelCase keys (`quoteId`, `orderId`,
`txHash`); only span attribute keys are dotted. The constants live in
[tracer.go](../internal/observability/tracer.go) and are always referenced by name.

| Attribute | Set on | Source |
|---|---|---|
| `solver` | every span started through `NewTracer` with a name, and txmanager send spans | package `Name` / `Request.Solver` |
| `chain.id` | the resource (one chain per process) | `cfg.Chain.ChainID` |
| `request.id` | inbound quote spans | RFQ: the `X-Request-Id` the middleware propagates or mints. UniswapX: the request id in the quote body |
| `quote.id` | quote spans, order/fill spans | solver quote id |
| `quote.trace_id` | a fill span that linked back to its quote | the linked span's trace id |
| `order.id`, `order.hash`, `order.onchain_id` | fill spans and everything under them | RFQ/LI.FI `orderId`, UniswapX order hash, LI.FI `onChainOrderId` |
| `auction.id`, `request.address` | 3F auction spans, RedStone auction and result spans | 3F auction id and Request address, RedStone auction id |
| `adapter.address` | quote, order and 3F auction/redeem spans | resolved adapter (3F: the offer's maker) |
| `strategy.name` | strategy stage spans | strategy registry key |
| `tx.label`, `tx.hash`, `tx.nonce`, `tx.outcome`, `tx.attempt`, `tx.cancellation` | txmanager spans; `tx.hash`/`tx.outcome` also on the solver's completion span | txmanager |
| `rpc.method`, `rpc.jsonrpc.request_id`, `chain.rpc.role`, `chain.rpc.batch`, attempt `endpoint` ordinal | RPC spans | fallback transport |
| `peer.service` | outbound HTTP client spans | the wiring's peer name |
| `reason_code` | any span ended with a classified error | the error's `ReasonCode()` |

`observability.AttrOfferID` and `observability.AttrVault` are declared alongside these for solvers that
grow an offer- or vault-scoped span; nothing sets them today.

`tx.hash` is set only once a transaction exists. A submission the manager rejected before
broadcasting carries the zero hash in its `Result`, and every site records `tx.outcome` alone rather
than an attribute that looks like a transaction and matches nothing.

Identifiers propagate downward by context: a stage span under a fill span does not repeat `order.id`,
because the trace view shows it on the parent. Nothing secret is ever an attribute — no API keys, no
signatures, no calldata, no RPC URLs (endpoints stay ordinals).

Events, not attributes, carry the non-error outcomes: `declined` (with `decision` and `reason`),
`cancelled`, `attempt` (RPC), and `link_miss` (§6).

### 4.1 Conventions for new tracing code

Three rules, established in review and mandatory for any new span site:

1. **Every span end is deferred**, through a named error return: `defer func() { end(err) }()`. The
   span then still ends when the pipeline panics, and an unended span is never exported.
2. **Log through `observability.Log(ctx)`**, never from a logger that already carries trace ids, and
   store only base loggers in a context (§3.6).
3. **Sentinel "expected skip" outcomes are declines, not errors**: record them with
   `observability.Decline` and end the span with `nil`. Only a real failure ends with `end(err)`. This
   mirrors the logging rule — `log.Error` only for conditions that should page.

## 5. Spans per solver

Span names are the contract. Everything under a stage (HTTP, RPC, txmanager) nests automatically
through the context.

**rfq** (`internal/solvers/rfq`) — see [RFQ-PLAN](RFQ-PLAN.md)

| Span | Starts in | Notes |
|---|---|---|
| `POST /quote` | `TraceHandler` (server span) | continues the backend's trace; `request.id` and `quote.id` set in `handleQuote` |
| `rfq.quote` | `quoteService.quote` | `adapter.address` set here when the chosen plan's legs share one adapter; a bad request or a no-quote outcome is a `declined` event, not an error |
| `rfq.quote.snapshot` | `snapshotCandidates` | chain read |
| `rfq.quote.decide` | `decideQuote` | strategy stage |
| `rfq.execution.sync` | `executionService.syncOnce` | root per poll cycle |
| `rfq.execution.poll` | `pollOpenOrders` | backend HTTP inside |
| `rfq.order` | `handleOrder` | `order.id`, `quote.id`, link to the quote span (§6); `adapter.address` once the chosen plan's legs share one adapter |
| `rfq.order.resolve` | `resolveExecutable` | backend fetch of the executable order |
| `rfq.order.plan` | strategy `BuildFillPlan` | strategy stage |
| `rfq.order.build` | `buildFillCalldata` | a terminal skip is declined on the order span, not on this stage |
| `rfq.order.submit` | `txm.Send` | `tx.hash`/`tx.outcome` on return |
| `rfq.order.report` | `reconcileTerminalStatus` | backend status reconcile |

There are no `rfq.quote.discounts` or `rfq.quote.sign` spans: the pipeline has no such steps.

**uniswapx** (`internal/solvers/uniswapx`) — see [UNISWAPX-PLAN](UNISWAPX-PLAN.md)

| Span | Starts in | Notes |
|---|---|---|
| `POST /quote` | `TraceHandler` | route labels `/quote`, `/ready`, `other` |
| `uniswapx.quote` | `Solver.quote` | a decline reason becomes a `declined` event |
| `uniswapx.quote.decide` | strategy call | strategy stage |
| `uniswapx.quote_refresh` | `refreshQuoteState` | root per refresh; chain reads inside |
| `uniswapx.orders.poll` | `pollSource` | root per poll |
| `uniswapx.order.track` | `trackOrder` | `order.hash`, `quote.id`, link to the quote span (§6); its span context rides on `resolvedOrder` through the orders channel |
| `uniswapx.fill` | `startFill` | continues the track span's trace; `tx.hash`/`tx.outcome` stamped on completion |
| `uniswapx.fill.plan` / `.build` / `.submit` | fill pipeline | `.submit` wraps `SendAsync`, which returns before the transaction resolves |
| `uniswapx.fill.complete` | `completePendingFill` | carries `tx.hash` and `tx.outcome` |

Because the send is asynchronous, `tx.hash` and `tx.outcome` live on `uniswapx.fill.complete` and
`uniswapx.fill`, not on `uniswapx.fill.submit`. An order this filler cannot fill is a `declined`
event, not an error.

**lifi** (`internal/solvers/lifi`) — see [LIFI-PLAN](LIFI-PLAN.md)

| Span | Starts in | Notes |
|---|---|---|
| `lifi.quotes.refresh` | `refreshQuotes` | root per cycle |
| `lifi.quotes.decide` | strategy call | strategy stage |
| `lifi.quotes.reconcile` | `reconcileQuotes` | one order-server HTTP child per pair |
| `lifi.quotes.suspend` | `suspendQuotes` | retiring the curve |
| `lifi.feed.connect` | `wsclient` dial | handshake carries `traceparent` |
| `lifi.order.<event>` | `admitOrderMessage` | two names, bounded by `orderMessageSpanName`: `lifi.order.user:vm-order-submit` for the only event the feed dispatches, `lifi.order.other` for everything else. `order.id`, `order.onchain_id`, `quote.id` |
| `lifi.order.process` | order worker | child of the message span; the span context rides on the queued `submittedOrder` |
| `lifi.order.plan` / `.reserve` / `.deposit` / `.submit` / `.complete` | fill pipeline | `.reserve` and `.deposit` are re-entered per retry with `tx.attempt`; `.complete` carries `tx.hash` |

One `lifi.order.process` span covers an order for as long as anything in the worker still references
it — pending fills, capacity retries, deposit retries — not just one pass; a shutdown that drops
queued retries ends every still-open span rather than leaking it. LiFi fills are **not** linked to a
quote; see §6.

**3f / bridgefacilitator** (`internal/solvers/bridgefacilitator`) — see [3F-PLAN](3F-PLAN.md)

| Span | Starts in | Notes |
|---|---|---|
| `3f.sync` | `discoverAndOffer` | root per poll |
| `3f.offers.reconcile` | `reconcileOffers` | the API offer listing |
| `3f.auction.view` | `buildOfferInput` | batch stage under `3f.sync` |
| `3f.offer.decide` | strategy call | batch stage under `3f.sync`, tagged `strategy.name` |
| `3f.auction` | per offer the strategy returns | `auction.id`, `adapter.address`, `request.address` |
| `3f.offer.build` / `3f.offer.submit` | `buildSignedOffer` / `submitOfferIfLaneReady` | under `3f.auction` |
| `3f.redeem` | `redeemAll` | root |
| `3f.redeem.read` | `readyToRedeem` | per adapter scan, `adapter.address` |
| `3f.redeem.submit` | batched finalize `Send` | one link per matched offer, plus `offer.linked_count` (§6) |

3F lists every auction in one response and decides for all of them in one call, so `3f.auction.view`
and `3f.offer.decide` are **siblings** of the per-auction spans rather than children of one.

**redstone-oev** (`internal/solvers/redstoneoev`) — see [OEV-PLAN](OEV-PLAN.md)

| Span | Starts in | Notes |
|---|---|---|
| `oev.feed.connect` | `wsclient` dial | handshake carries `traceparent` |
| `oev.auction` | `handleAuction` | root per auction; `auction.id` |
| `oev.auction.bid` | `buildBidWithContext` | strategy stage |
| `oev.auction.candidates` / `.size` / `.bundle` / `.economics` | `strategies/default` | in that order: economics prices the bundle the previous stage built |
| `oev.auction.send` | `ws.Send` | a solve dropped by the full send queue is a `declined` event |
| `oev.auction.result`, `oev.liquidation.result`, `oev.blacklisted` | result handlers | `auction.id`, link back to the auction span (§6); the result span carries `oev.won` |
| `oev.monitor` | monitor tick | root; Morpho GraphQL and chain reads inside |

Ingress outcomes decided before `handleAuction` runs (a frame that never reaches the handler) have no
span. The default strategy takes its tracer from `strategies.Deps.Solver`, which carries the solver
name down into the strategy package without importing the solver package back.

## 6. Linking a fill to its quote

A quote and its fill are separate context trees: in RFQ the order is discovered minutes later by
polling, in UniswapX it arrives through the orders channel from a poll, in 3F the offer is re-listed
by the API and settled from an on-chain read, in RedStone the result is a later websocket frame. §3.5
means a fill trace starts at the solver's own loop tick, not at the quote. The two are joined with a
**span link** — the fill is later work in its own trace, and trace backends render a link as a
clickable jump — plus a `quote.trace_id` attribute and a `quoteTraceId` log key so the join also works
without the trace backend.

**Best effort means exactly that.** A miss (process restart, eviction, an unknown id, tracing
disabled) produces a fill span with no link and a `link_miss` event naming the key; the fill continues
under its loop span with a fresh trace id exactly as if linking did not exist. The map is
process-local and in-memory by design: never persisted, never consulted on a path that can fail, and
`Lookup` cannot return an error. Nothing about a fill's behaviour depends on it.

`observability.NewSpanLinks(maxEntries)` is the shared mechanism — bounded, TTL per entry,
mutex-guarded, 1024 entries by default (a span context is 40 bytes). `Remember(ctx, key, ttl)` stores
the current span context; empty keys and contexts without a valid span context are ignored, so callers
never check tracing state first. `Lookup(key)` returns `(trace.Link, bool)` and never deletes a live
entry, so the same quote can be linked from a retry or a later status update. Keys are lowercased and
trimmed; the oldest entry is evicted past the cap.

| Solver | Remembered (key → TTL) | Looked up by |
|---|---|---|
| **rfq** | `handleQuote` after a quote is returned: key `quoteId`, flat 10 minutes. The quote response carries no expiry, so the fixed window stands in for the backend's award latency. | `handleOrder`, by the polled order's `quoteId`. The link and `quote.trace_id` go on `rfq.order`; `quoteTraceId` on the order logger. |
| **uniswapx** | `quoteHandler` after a quote is returned: key `quoteId`, 10 minutes. Indicative and hard quotes share a request shape, so both are remembered and the later wins. | `trackOrder`, by the resolved order's `quoteId`. The link goes on `uniswapx.order.track`; because `uniswapx.fill` continues that trace, the link is visible from the fill. |
| **lifi** | Not linked. | — |
| **3f** | `3f.offer.submit` after `createOffer` succeeds, under **two** keys: `req:<request address>` and `auction:<adapter>:<auction id>`. Both expire at the offer's expiration plus an hour. | `redeemReady` resolves `req:` keys for the requests being finalized — `3f.redeem.submit` gets one link per match plus `offer.linked_count`. `reconcileOffers` resolves the `auction:` key to stamp `quoteTraceId` on its status-change log lines. |
| **redstone-oev** | `handleAuction` when the solve is sent: key = auction id, TTL = `reservationTTL` (5 min), the same window the reservation lives for. | `handleAuctionResult`, `handleLiquidationResult` and `handleBlacklisted` link their span back to the auction span and stamp `quoteTraceId` on the log line. |

**Why LI.FI is not linked.** The solver publishes standing quotes per asset pair rather than per
request, and the order server assigns the quote id we only learn from the order message. There is no
quote event of ours to remember, so there is nothing to link from. The `quote.id` LI.FI reports is
recorded as an attribute instead, and the whole order path is already one trace rooted at its feed
message.

**Why not the backend's request id.** The RFQ backend's order list returns a fresh per-response
`requestId` and no trace id, and it does not send `X-Request-Id` on the quote request, so nothing
arriving from the backend identifies the quote trace. If the backend later returns the quote's trace
id on the order item, RFQ switches its lookup to that value and the in-memory map becomes a fallback;
nothing else changes (§10).

**3F's two keys exist because the API discards the created offer id.** The Request contract address is
the only identifier present both at offer time and on the settlement path, which reads Requests
on-chain; the `(adapter, auction)` pair is what the API's offer listing reports.

## 7. Overhead and failure isolation

Tracing must never slow down or break a quote, a fill or a transaction.

- **Nothing on a hot path waits on tracing.** Spans are recorded in memory and handed to the SDK's
  batch span processor, which exports from its own goroutine. Its queue is bounded
  (`OTEL_BSP_MAX_QUEUE_SIZE`, SDK default 2048) and **drops** spans when full rather than blocking the
  producer. No request-path code performs network I/O for tracing.
- **Exporter failures are invisible to callers.** A dead or slow collector surfaces only through the
  `otel.SetErrorHandler` Info log; no tracing error is ever returned into solver code.
- **Startup never fails because of tracing.** A bad `OTEL_*` value logs at Error and leaves tracing
  disabled; the bot starts and runs exactly as without it.
- **Disabled means near-zero cost.** The global provider stays the no-op one: `Start` returns a
  non-recording span, `end` is a no-op, the `otelhttp` wrappers create non-recording spans, and the
  only residual work is the W3C header parse on inbound requests and `Log(ctx)` reading the context's
  logger, which adds two fields only when a remote context is present.
- **Enabled cost is bounded and off-path.** Span start and end cost a few microseconds — the
  `internal/observability` benchmark measures the no-op and the recording path, sequentially and
  under `RunParallel`, and the cached tracer keeps concurrent starts off the global provider's
  mutexes — attributes are a
  handful of bounded strings, no per-request goroutines are created, and the RPC span reuses the body
  classification the metrics already do. Volume is controlled by `OTEL_TRACES_SAMPLER`.
- **Linking cannot fail a fill.** The map is mutex-guarded and capped; `Lookup` never errors, and a
  miss adds an event and nothing else.
- **No panics from the tracing API.** The OTel API is nil-safe on ended spans, invalid contexts and a
  missing provider; `end(nil)` and a double `end` are both safe.
- **Shutdown is bounded.** The provider flushes for at most 5 s on a fresh context after the solvers
  and txmanager have drained, so a stuck collector cannot hold the process.

## 8. Tests

`internal/observability/tracetest` installs an in-memory `SpanRecorder` for a test, invalidates the
cached tracers, restores the previous provider afterwards, and exports the span assertions every
suite shares (`Ended`, `Names`, `Attr`, `RequireChildOf`, `HasEvent`, `RequireNoErrorSpans`). Because
it imports `observability`, that package's own recorder-driven tests live in `package
observability_test`. Covered: the enablement table over
`OTEL_EXPORTER_ENABLED` values, the exported resource (service identity plus `telemetry.sdk.*`),
`TraceLogger` and `Log(ctx)` with and without a span, the context logger's nested-span stamping and
its fallback to the default logger, `TraceHandler` extracting a
`traceparent` and filtering probe routes, `TraceTransport` injecting a matching `traceparent` and
keeping the query string out of `url.full` while the wire request keeps it, the
tracer's error / cancellation / `reason_code` behaviour and its re-resolution across providers,
`SpanLinks` remember, lookup, TTL expiry and
eviction, one RPC span per logical request with an event per attempt, the txmanager span tree and its
`trace_id`-carrying lifecycle logs, and per solver the expected span tree by name with the identifier
attributes and the error-versus-decline distinction. A benchmark in `internal/observability` records
`Start`/`end` cost with and without a provider, sequentially and under `RunParallel`, so a regression
is visible in review. Existing suites
run against the no-op provider and prove there is no behaviour change when tracing is off.

## 9. Where the code lives

| File | Responsibility |
|---|---|
| `internal/observability/tracing.go` | `NewTracing` startup/shutdown, the enablement switch, `TraceLogger` |
| `internal/observability/ctxlog.go` | `WithLogger`, `Log`, `SetDefaultLogger` — the logger carried in the context |
| `internal/observability/tracer.go` | `Tracer`, `Start`/`StartLinked`/`EndFunc`, `Decline`, `SetAttributes`, `InvalidateTracers`, the `Attr*` constants |
| `internal/observability/httptrace.go` | `TraceHandler`, `TraceTransport` |
| `internal/observability/spanlinks.go` | `SpanLinks` |
| `internal/observability/tracetest` | test provider installation and the shared span assertions |
| `internal/chain/trace.go` | JSON-RPC spans, attempt events, header injection, the connect span |
| `internal/chain/calls.go` | per-call spans for websocket/IPC endpoints |
| `internal/txmanager/trace.go` | the send span's start and terminal end |
| `internal/solvers/<name>/tracing.go` | each solver's tracer, link keys, TTLs and span helpers |

## 10. TODO

- [ ] backend returns quote trace id on order list (would replace the RFQ in-memory link)
- [ ] observability listener untraced by design
- [ ] omit `rpc.jsonrpc.request_id` when empty (batches) instead of setting an empty attribute
- [ ] set `tx.outcome=not_admitted` on declined txmanager sends, and skip an empty `solver` attribute
- [ ] cap or aggregate the per-offer `declined` events on `3f.offers.reconcile`
- [ ] the RPC span's success `attempt` event is timestamped at body close, not at the attempt
- [ ] a no-op `txmanager.replace` tick emits an empty span, and replace spans carry no `tx.hash`
- [ ] the LI.FI quote-loop tick (`runConnectedQuoteLoop`) is unspanned, so `shouldRefreshQuotes` lines carry no trace ids
- [ ] 3F `refreshTargetsAndHydrate` emits a root `3f.offers.reconcile`, and the health-tick reconcile is untraced
- [ ] no unit tests for `NewTracer("")`, `NewSpanLinks(0)`, `Raw()`, or `TraceTransport` with a non-nil base
