# OpenTelemetry tracing for vault-solver — design

Status: approved for implementation, 2026-09-16.

## 1. Goal

Every unit of I/O the bot performs is a span in one trace, and every log line written under that
work carries the trace id. A quote request that the RFQ backend sends us arrives with W3C
`traceparent`; we continue that trace through the strategy, the webhook, the backend client, the
JSON-RPC provider, and the transaction lifecycle, and each outbound call carries the trace headers so
the next hop can continue it.

The posture mirrors the RFQ backend, which gets all of this from `@symbiotic/backend-devkit/otel`
(Node SDK, auto-instrumentations, OTLP/HTTP protobuf exporter, pino log correlation):

- configured only by the standard `OTEL_*` environment variables, never by YAML;
- disabled (no exporter, no SDK) unless `OTEL_EXPORTER_ENABLED` is truthy, the devkit's switch;
- one span per inbound request, per outbound HTTP call, per RPC call, per transaction;
- trace and span ids on every log line under a traced context;
- default sampler `parentbased_always_on`, overridable with `OTEL_TRACES_SAMPLER`.

Go has no auto-instrumentation, so the equivalent is wired by hand at the repo's existing seams.

## 2. Non-goals

- A backend change to expose the quote's trace id on the order list. Quote-to-fill linking (§12)
  is done solver-side and best effort; if the backend later returns a quote trace id, the RFQ link
  can switch to it.
- Exporting metrics or logs through OTLP. Prometheus and zap JSON logs stay as they are.
- Tracing the observability listener (`/metrics`, `/healthz`, `/readyz`). Probes are noise.
- Instrumenting the non-HTTP RPC dial path (`ws://`, IPC). It bypasses the fallback transport
  and stays untraced; documented, not fixed.

## 3. Enablement and configuration

All configuration is via environment variables, read by the Go SDK itself except for the on/off
switch. No `config.ObservabilityConfig` change. The env-only choice matches Sentry (`SENTRY_DSN`) and
the backend, and keeps deployment-specific endpoints out of the YAML.

The backend's devkit (`@symbiotic/backend-devkit` 2.0.0, `dist/otel`) is exactly this: `initOpenTelemetry`
returns early unless `OTEL_EXPORTER_ENABLED` is truthy, then starts `NodeSDK` with only the
auto-instrumentations and no other options, so every remaining setting comes from the standard
`OTEL_*` variables. The Go side mirrors that switch and defers everything else to the SDK.

| Variable | Effect |
|---|---|
| `OTEL_EXPORTER_ENABLED` | The switch, same accepted values as the devkit: `1`, `true`, `yes`, `on`, `enabled` turn tracing on; anything else (including unset) leaves it off. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` or `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Collector address; SDK default `http://localhost:4318`. Same OTLP/HTTP protobuf wire format the backend exports. |
| `OTEL_SERVICE_NAME` | Defaults to `vault-solver`. Deployments run one solver per process, so operators set e.g. `vault-solver-rfq`. |
| `OTEL_RESOURCE_ATTRIBUTES` | Merged into the resource. |
| `OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG` | SDK default `parentbased_always_on`, same as the backend. `parentbased_traceidratio` is the knob for taming root spans from background loops without dropping backend-parented traces. |
| `OTEL_PROPAGATORS` | Not read; the propagator is always W3C `tracecontext` + `baggage`, which is the Node SDK default the backend runs with. |
| `OTEL_EXPORTER_OTLP_HEADERS`, `_TIMEOUT`, `_COMPRESSION`, `OTEL_BSP_*` | Read by the exporter and batch processor as usual. |

`OTEL_TRACES_EXPORTER` is read by the Node SDK only; the Go side always uses OTLP over HTTP and the
README says so. `OTEL_EXPORTER_OTLP_PROTOCOL=grpc` is not honored.

What the backend sends and expects, verified from the devkit and `src/clients/solver/solver.client.ts`:

- The `/quote` call to a solver carries `traceparent` (and `tracestate`, `baggage` when present) from
  the Node HTTP auto-instrumentation. It does **not** carry `X-Request-Id`; the solver's existing
  middleware therefore mints its own request id today and keeps doing so.
- Every backend response echoes or mints an `X-Request-Id` header (pino-http `genReqId`). It is a
  request id, not a trace id. Because vault-solver already sends `X-Request-Id` on backend calls, the
  backend's access log for our calls carries our request id and, via the pino instrumentation, the
  trace id we inject.

Resource attributes always set: `service.name` (default above), `service.version`
(`version.Version`), `vault_solver.commit` (`version.Commit`), `vault_solver.solvers` (comma-joined
configured solver names), `chain.id` (`cfg.Chain.ChainID`), plus the SDK's telemetry attributes.
Semantic-convention keys are used only where a stable one exists (`service.*`, `rpc.*`, the `http.*`
and `server.*` keys otelhttp emits); everything else is under the `vault_solver.` or a domain prefix.

## 4. Framework: `internal/observability` tracing

New file `internal/observability/tracing.go`:

- `Tracing{Name, Version, Commit string; Solvers []string; ChainID uint64}` describes the process.
- `NewTracing(ctx, info Tracing, log logr.Logger) (shutdown func(context.Context) error, enabled bool)`
  never returns an error: a setup failure is logged at Error and tracing stays disabled (§13):
  1. Always installs the composite W3C `TraceContext` + `Baggage` propagator. With tracing disabled
     the global provider stays the no-op one, but an inbound `traceparent` is still carried through
     the context, forwarded on outbound calls, and stamped on logs. Nothing is recorded or exported.
  2. When `OTEL_EXPORTER_ENABLED` is truthy (§3), builds `otlptracehttp.New(ctx)` with no options (env-driven), a resource from
     `resource.WithFromEnv` merged with the attributes in §3, and
     `sdktrace.NewTracerProvider(WithBatcher, WithResource)` (the SDK applies the env sampler).
     Registers it with `otel.SetTracerProvider`.
  3. Sets `otel.SetErrorHandler` to log exporter failures at Info through `log`, so a dead collector
     is visible without paging.
- `TraceLogger(ctx, log) logr.Logger` returns `log.WithValues("trace_id", …, "span_id", …)` when the
  context carries a valid span context, otherwise `log` unchanged.
- `TraceHandler(next http.Handler, route func(*http.Request) string) http.Handler` wraps
  `otelhttp.NewHandler`. Span name is `"<METHOD> <route>"` using the caller's bounded route function
  (never the raw path). Requests whose route is a probe or docs path (`/health`, `/healthz`, `/ready`,
  `/readyz`, `/openapi.*`, `/docs`) are filtered out and produce no span.
- `TraceTransport(base http.RoundTripper, peer string) http.RoundTripper` wraps
  `otelhttp.NewTransport`. Span name is `"<peer> <METHOD>"` (e.g. `rfq-backend POST`); otelhttp
  adds the URL, host, and status attributes. `peer` is a short integration name, never a URL.

Wiring in `cmd/vault-solver/run.go`: right after `NewLogger`, `NewTracing` runs; the startup log
line gains `tracing: true|false`; shutdown is deferred after the solvers and txmanager finish, on a
fresh 5s context like the observability server drain.

Tracers are obtained per package from the global provider through the §9.1 wrapper:
`var tracer = observability.NewTracer("<import path>", Name)`. `solver.Deps` does not grow a tracer field: the global is the OTel-idiomatic carrier, `otelhttp`
already uses it, and it reaches the three strategy registries that take no deps at all. Tests that
assert spans install an `sdktrace` provider with a `tracetest.SpanRecorder`; everything else sees the
no-op provider and stays unchanged.

New direct dependencies, pinned to the OTel API already in `go.mod`:
`go.opentelemetry.io/otel@v1.46.0` (api, `trace`, `propagation`, `semconv`),
`go.opentelemetry.io/otel/sdk@v1.46.0`,
`go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@v1.46.0`,
`go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.71.0`.

## 5. Logs carry trace ids

Loggers are passed by value in this codebase and never live in contexts. The rule: **whoever starts
a span derives the logger for that scope with `TraceLogger` and passes it down as today.** Concretely:

- RFQ `/quote` and UniswapX `/quote` handlers: the per-request logger is derived from the request
  context (after `TraceHandler` has extracted the parent), so every log line of a quote carries the
  backend's trace id. The existing `X-Request-Id` handling stays; the request id becomes a span
  attribute (`request.id`) as well.
- Each solver loop iteration span (§8) derives its logger the same way.
- txmanager derives the per-request logger from the transaction span (§7), so lifecycle logs carry
  the trace id of the submitting work.
- The Sentry sink promotes `trace_id` to an event tag next to `solver`, `logger`, and `label`.

## 6. Boundaries

### 6.1 Inbound HTTP

| Server | Change |
|---|---|
| RFQ (`internal/solvers/rfq/server.go`, huma over mux) | `TraceHandler` applied outermost around the existing chain (`MaxBytesHandler` → `logRequests` → metrics → `recoverPanics`), route from the existing `routeLabel` allowlist. `handleQuote` sets `request.id`, `quote.id`, and `adapter.address` span attributes from the body (§9.3). |
| UniswapX (`internal/solvers/uniswapx/server.go`, plain mux) | `TraceHandler` outermost around `recoverQuoteServer`; a small bounded route function (`/quote`, probes). The quote handler sets `request.id` / `quote.id` attributes. |
| Observability listener | Untraced. |

### 6.2 Outbound HTTP

Every `*http.Client` construction gets `TraceTransport` as the **outermost** RoundTripper so the span
covers the existing wrappers. Generated OpenAPI clients are untouched; they receive the client via
`Configuration.HTTPClient` as today.

| Site | Peer name | Composition |
|---|---|---|
| `internal/solvers/rfq/backend.go:60` | `rfq-backend` | `TraceTransport(backendRequestTransport{base: DefaultTransport})`. The same client is handed to the discounts client, so `rfq-backend-internal` calls are covered. |
| `internal/liquidlane/discounts/client.go:67` (`NewClient`, used by LiFi and UniswapX) | `rfq-discounts` | `TraceTransport(DefaultTransport)` |
| `internal/solvers/bridgefacilitator/apiclient.go:34` | `3f-api` | `TraceTransport(DefaultTransport)` |
| `internal/solvers/lifi/orderclient.go:32` | `lifi-order-server` | `TraceTransport(DefaultTransport)` |
| `internal/solvers/uniswapx/orderclient.go:36` | `uniswapx-api` | `TraceTransport(responseLimitTransport{…})` |
| `internal/solvers/redstoneoev/strategies/default/morphoapi.go:77` | `morpho-graphql` | `TraceTransport(DefaultTransport)`; the hand-built request already uses the caller's context |
| `internal/webhook/client.go:206` | `webhook` | `TraceTransport(DefaultTransport)`; `CheckRedirect` guard unchanged. A webhook strategy call is therefore a child span of the quote or fill that triggered it, and the operator's webhook receives `traceparent`. |

### 6.3 Ethereum JSON-RPC

`internal/chain/fallback.go` `fallbackTransport.RoundTrip` is the single chokepoint for HTTP RPC and
already parses the JSON-RPC envelope and classifies outcomes. It is instrumented directly rather than
through `otelhttp`, so there is exactly one span per logical RPC request, named by method:

- Span `"<rpc.method>"` (bounded by the existing `boundedRPCMethodName`; batches are named `batch`)
  with attributes `rpc.system=jsonrpc`, `rpc.method`, `rpc.jsonrpc.request_id`, `chain.rpc.role`
  (`read`/`write`/`shared`), `chain.rpc.batch`.
- One span event per endpoint attempt with the endpoint **ordinal** (never the URL, same rule as the
  metrics) and its classified outcome.
- Each attempt's cloned request gets `traceparent` injected via the global propagator, so the RPC
  provider (eRPC and the like) can continue the trace.
- The span ends exactly where `rpcRequestObservation.finish` runs today: on body `Close` for the
  observed and classified bodies, or immediately on a transport-level failure. Span status is Error
  for every outcome other than `success`.
- Because `rpc.DialOptions(..., rpc.WithHTTPClient(...))` already supplies this transport and
  go-ethereum builds each HTTP request from the `CallContext` context, no other chain change is needed.
  `Multicall`/`CallContract` and every solver read inherit whatever span is in the caller's context.

### 6.4 WebSockets

Both feeds inject `traceparent` into the handshake headers under a short `"<feed> connect"` span per
dial attempt, so a connection can be found in the trace backend. Per message:

- LiFi (`internal/solvers/lifi/wsclient.go` → `execution.go`): each order message is handled under a
  span `lifi.order.<event>` with `order.id`, `order.onchain_id`, `quote.id` attributes. The queued
  `submittedOrder` carries the span context, so the order worker's span is a child of the message
  span even though it runs on the detached `workCtx`.
- RedStone (`internal/solvers/redstoneoev/wsclient.go` → `auction.go`): each auction message is
  handled under `oev.auction` with `auction.id`, ending when the solve is sent, dropped from the
  send queue, or declined. Result and blacklist messages get their own short spans carrying the same
  `auction.id` attribute and a link back to the auction span (§12).

## 7. txmanager

`Request` does not change. `Send`, `TrySend`, and `SendAsync` all receive the caller's context, so
`sendAsync` starts the transaction span `"txmanager.send <label>"` there, as an ordinary child of the
caller's span, and hands the span to the worker on the internal `job`. No submission site changes.

The worker continues that span on its own contexts with `trace.ContextWithSpan` (legal even after
the caller's context is cancelled), so the span covers admission wait → broadcast → terminal outcome.
Child spans: `"txmanager.broadcast"` (fee quote, gas estimate, nonce, sign, send; each RPC call is a
grandchild automatically), `"txmanager.replace"` per replacement with `tx.attempt`. Receipt polls
are ordinary RPC child spans from §6.3; they are bounded by the pending window and sampled with the
parent. Span attributes: `solver`, `tx.label`, `tx.hash` (once known), `tx.nonce`, terminal
`tx.outcome`; status Error for `reverted`, `cancelled`, `submission_error`, and `tracking_stopped`.
The quote and order ids live on the parent fill span in the same trace (OTel spans cannot be read
back, so they are not copied). When the result is delivered, the submitting fill span receives
`tx.hash` and `tx.outcome` too, so a search by transaction hash finds both the fill trace and the
txmanager spans.

The per-request logger (`requestLog`) is derived with `TraceLogger` from the transaction span's
context, so `solver`, `label`, `trace_id`, and `span_id` appear on every lifecycle line.

## 8. Root spans for solver loops

Work that is not triggered by an inbound request needs a root span so its outbound calls have a
parent instead of each becoming its own one-span trace. Rule: **every periodic tick or event handler
that performs I/O starts a span named `<solver>.<loop>`** and derives its logger from it. Known loops:

| Solver | Loops |
|---|---|
| rfq | `rfq.execution.sync` (poll open orders → per order child `rfq.order`); `rfq.quote` runs under the inbound server span |
| uniswapx | `uniswapx.orders.poll` (per order child `uniswapx.order.track`, carried across the orders channel to `uniswapx.fill`), `uniswapx.quote_refresh` |
| lifi | `lifi.quotes.refresh` per quote cycle (`lifi.quotes.suspend` on shutdown), order handling per §6.4 with retries re-entering `lifi.order.reserve` / `lifi.order.deposit` |
| 3f | `3f.sync` per API poll (offers reconcile, per-auction children), `3f.redeem` |
| redstone-oev | `oev.monitor` per monitor tick, auction handling per §6.4 |
| txmanager | `txmanager.account_poll` per account poll tick |

The full stage list per solver, with the function each span starts in, is §9.4.

In-process hops that cross a channel or errgroup carry `trace.SpanContext` on the queued struct
(`resolvedOrder`, `submittedOrder`) so the consumer span is a child; nothing else is stored in structs.

## 9. Span conventions: stages, identifiers, errors

### 9.1 One helper, used everywhere

`internal/observability` exposes a thin wrapper so call sites do not repeat OTel boilerplate:

```go
// tracer := observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/rfq", Name)
ctx, end := tracer.Start(ctx, "rfq.quote.decide", attrs...)
defer func() { end(err) }()   // or end(nil) on the success path
```

- `Start` stamps `solver=<name>` on every span the tracer creates, the way `run.go` stamps the logger,
  so any span can be filtered by solver type without joining on the resource.
- `end(err)` sets status `Error`, records the error (`RecordError` with the go-errors stack) and
  the `reason_code` when the error carries one (`backendRequestError`, order diagnostics), then ends
  the span. A `context.Canceled` error ends the span with status unset and a `cancelled` event, the
  same rule the metrics use for skipped outcomes.
- `Decline(ctx, decision, reason string)` records an expected non-error outcome (no quote, not
  profitable, adapter paused, order already filled) as a `declined` event with `decision` and
  `reason` attributes and leaves the status unset. This mirrors the logging rule: `Error` only for
  conditions that should page, expected skips at `V(1)`. Solvers call it at the site that already
  classifies the decision outcome (the quote service's `quoteDecision*` outcomes, the fill path's
  skip reasons), not inside strategies.
- `SetAttributes(ctx, attrs...)` adds to the current span; used when an id becomes known mid-stage
  (a transaction hash after `Send` returns, an order id after the backend responds).

### 9.2 Stage spans through the core functions

Tracing stops at I/O boundaries only if the pipeline between them is opaque. Every quote and fill
pipeline gets one span per stage at the altitude of the existing service functions, so a trace shows
which stage was slow or failed, and each stage records its own error. The stage list per solver is in
§9.4; the rule for new code: a function that does I/O, runs a strategy, or is a named step in a plan
document's data flow gets a span named `<solver>.<pipeline>.<stage>`.

Strategy calls are always a stage (`<solver>.quote.decide`, `<solver>.fill.plan`) with a
`strategy.name` attribute (`default`, `webhook`, `greedy`), so the webhook HTTP span nests under a
named decision span.

### 9.3 Identifier attributes

Identifiers use **neutral keys** shared across solvers, because the `solver` attribute already says
which integration a span belongs to and an operator searching for a quote id should not need to know
the solver's key prefix. Log lines keep their existing camelCase keys (`quoteId`, `orderId`, `txHash`);
only span attribute keys are dotted.

| Attribute | Set on | Source |
|---|---|---|
| `solver` | every span started through `NewTracer`, and txmanager spans | package `Name` / `Request.Solver` |
| `chain.id` | resource (one chain per process) | `cfg.Chain.ChainID` |
| `request.id` | inbound quote spans, backend client spans | `X-Request-Id` (inbound and outbound) |
| `quote.id` | quote spans, fill spans, txmanager spans | solver quote id (RFQ/UniswapX `quoteId`, LiFi quote id, 3F offer id doubles as `offer.id`) |
| `order.id` | fill spans and everything under them | RFQ `orderId`, LiFi `orderId`, UniswapX order hash (`order.hash` as well) |
| `order.onchain_id` | LiFi | `onChainOrderId` |
| `offer.id`, `auction.id` | 3F offer/auction spans, RedStone auction spans | 3F ids, RedStone `AuctionMessage.ID` |
| `adapter.address`, `vault.address` | quote and fill spans | resolved adapter / vault |
| `strategy.name` | strategy stage spans | strategy registry key |
| `tx.label`, `tx.hash`, `tx.nonce`, `tx.outcome`, `tx.attempt` | txmanager spans; `tx.hash` and `tx.outcome` are also set on the submitting fill span when the result arrives | txmanager |
| `rpc.method`, `rpc.jsonrpc.request_id`, `chain.rpc.role`, `chain.rpc.batch`, attempt `endpoint` ordinal | RPC spans | fallback transport |
| `reason_code` | any span ended with a classified error | error's `ReasonCode()` |

Identifiers are propagated downward by context: a stage span started under a fill span does not
repeat `order.id`; the trace view shows it on the parent. txmanager spans are children of the fill
span too (§7) and carry only what the manager knows: `solver`, `tx.*`.

Addresses and hashes are attributes, never span names; span names stay bounded (§9.2 pattern, RPC
method allowlist, route allowlist). Nothing secret is ever an attribute: no API keys, no signatures,
no calldata, no private URLs (RPC endpoints stay ordinals).

### 9.4 Stage spans per solver

Span names below are the contract; the function named is where the span starts. Everything under a
stage (HTTP, RPC, txmanager) nests automatically through the context. "strategy" stages carry
`strategy.name`.

**rfq** (`internal/solvers/rfq`)

| Span | Starts in | Notes |
|---|---|---|
| `POST /quote` | `TraceHandler` (server span) | child of the backend's auction span; `request.id`, `quote.id`, `adapter.address` set in `handleQuote` |
| `rfq.quote` | `quoteService.quote` (`quote.go:95`) | one child per helper it calls: `rfq.quote.snapshot` (chain read), `rfq.quote.discounts` (discount lookup), `rfq.quote.decide` (strategy `DecideQuote`), `rfq.quote.sign` |
| `rfq.execution.sync` | `executionService.syncOnce` (`execution.go:101`) | root; `rfq.execution.poll` child around `pollOpenOrders` (`:111`, backend HTTP inside) |
| `rfq.order` | `handleOrder` (`execution.go:139`) | `order.id`, `quote.id`, link to the quote span (§12); status from the terminal outcome |
| `rfq.order.resolve` | `resolveExecutable` (`execution.go:277`) | backend fetch of the executable order |
| `rfq.order.plan` | strategy `BuildFillPlan` | strategy stage |
| `rfq.order.build` | calldata construction in `submitOrder` (`execution.go:155`) | |
| `rfq.order.submit` | `txm.Send` (`execution.go:229`) | `tx.hash`/`tx.outcome` set on return |
| `rfq.order.report` | terminal status reconcile / backend status update | |

**uniswapx** (`internal/solvers/uniswapx`)

| Span | Starts in | Notes |
|---|---|---|
| `POST /quote` | `TraceHandler` | `request.id`, `quote.id` set in `quoteHandler` (`server.go:39`) |
| `uniswapx.quote` | `Solver.quote` (`server.go:145`) | child `uniswapx.quote.decide` (strategy) |
| `uniswapx.quote_refresh` | quote snapshot refresh (`quote_refresh.go`) | root per refresh; chain reads inside |
| `uniswapx.orders.poll` | `pollSource` (`polling.go:123`) | root; one child `uniswapx.order.track` per accepted order (`trackExclusive`, `claim` at `:168`), carrying `order.hash`, `quote.id`; its span context rides on `resolvedOrder` (`order.go:70`) through the channel |
| `uniswapx.fill` | `startFill` (`execution.go:126`) | child of the order's track span; children `uniswapx.fill.plan` (strategy), `uniswapx.fill.build`, `uniswapx.fill.submit` (`SendAsync` `:255`), `uniswapx.fill.complete` (`completePendingFill` `:422`, sets `tx.hash`, `tx.outcome`) |

**lifi** (`internal/solvers/lifi`)

| Span | Starts in | Notes |
|---|---|---|
| `lifi.quotes.refresh` | `refreshQuotes` (`quotes.go:187`) | root per cycle; children `lifi.quotes.decide` (strategy), `lifi.quotes.reconcile` (`reconcile` `:366`, one order-server HTTP child per pair); `lifi.quotes.suspend` for `suspendQuotes` (`:143`) |
| `lifi.feed.connect` | `wsclient.go:92` dial | handshake carries `traceparent` |
| `lifi.order.<event>` | `parseOrderMessage` (`execution.go:556`) | `order.id`, `order.onchain_id`, `quote.id`; span context rides on `submittedOrder` (`order.go:47`) through the inbox and both retry queues (same pointer) |
| `lifi.order.process` | `runOrderWorker` per order (`execution.go:427`) | child of the message span; children `lifi.order.plan` (`planning.go`), `lifi.order.reserve` (reservation, re-entered on `order_retry`), `lifi.order.deposit` (re-entered on `deposit_retry`, `tx.attempt`), `lifi.order.submit` (`submission.go:85`), `lifi.order.complete` |

**3f / bridgefacilitator** (`internal/solvers/bridgefacilitator`)

| Span | Starts in | Notes |
|---|---|---|
| `3f.sync` | `discoverAndOffer` (`solver.go:245`) | root per poll; child `3f.offers.reconcile` (`reconcileOffers` `:182`, `listOffers` HTTP inside) |
| `3f.auction` | per auction inside `discoverAndOffer` | `auction.id`, `adapter.address`, `request.address`; children `3f.auction.view` (auction view build), `3f.offer.decide` (strategy), `3f.offer.build` (`buildSignedOffer` `offer.go:17`), `3f.offer.submit` (`createOffer` `apiclient.go:58`) |
| `3f.redeem` | `redeemAll` (`redeemer.go:19`) | root; children `3f.redeem.read` (`readyToRedeem`), `3f.redeem.submit` (`Send` `redeemer.go:75`), links to the offer spans of the redeemed requests (§12) |

**redstone-oev** (`internal/solvers/redstoneoev`)

| Span | Starts in | Notes |
|---|---|---|
| `oev.feed.connect` | `wsclient.go:126` dial | handshake carries `traceparent` |
| `oev.auction` | `handleAuction` (`auction.go:194`) | root; `auction.id`; child `oev.auction.bid` (`buildBidWithContext` `:301`) with strategy stages `oev.auction.candidates`, `oev.auction.size`, `oev.auction.economics`, `oev.auction.bundle` from `strategies/default`; `oev.auction.send` around `ws.Send` (`:229`, a dropped frame is a `Decline`) |
| `oev.auction.result` | `handleAuctionResult` (`auction.go:82`) | `auction.id`, won/lost as attribute, link to the auction span (§12) |
| `oev.liquidation.result` | `handleLiquidationResult` (`auction.go:101`) | `auction.id`, `tx.hash`, link to the auction span |
| `oev.monitor` | monitor tick (`strategies/default/monitor.go`) | root; Morpho GraphQL and chain reads inside |

RedStone logs the auction id as `auction` on the bid path and `id` on the result path; the span
attribute is `auction.id` on both, and the plan harmonizes the log key to `auctionId` while touching
those lines.

## 10. Docs and operator surface

- `README.md` Observability section: an "OpenTelemetry tracing" subsection listing the variables in
  §3, noting the devkit-only variables that are ignored, and a compose/`docker run` example next to
  the Sentry one. `deploy/docker-compose.yml` gains commented `OTEL_*` lines.
- Each `config/*.example.yaml` observability block gets a comment line pointing at the env variables,
  next to the existing Sentry comment.
- New cross-cutting `docs/TRACING-PLAN.md`: this design, span naming and attribute conventions, and a
  §10 TODO list (the §2 non-goals recorded as known gaps). Each solver plan gets a one-line pointer;
  `docs/RFQ-PLAN.md` "OpenTelemetry — intentionally not ported" is rewritten to point here.
- `CLAUDE.md` logging paragraph gains one sentence: spans are started at I/O boundaries and loop
  ticks, and the logger for that scope is derived with `TraceLogger`.

## 11. Testing

All with `tracetest.SpanRecorder` installed via `otel.SetTracerProvider` in the test (and reset).

- `tracing_test.go`: enablement table over `OTEL_EXPORTER_ENABLED` values (unset, `true`, `1`, `on`,
  `false`, `garbage`) matching the devkit's accepted set;
  `TraceLogger` with and without a span; `TraceHandler` extracts a `traceparent` from an `httptest`
  request into a child span and filters probe routes; `TraceTransport` injects `traceparent` matching
  the parent span into a request seen by an `httptest` server.
- `fallback_test.go`: one span per logical request named by method, one event per attempt with the
  ordinal, span ends on body close, status Error on fallover exhaustion and on `null` results.
- `txmanager` tests: the transaction span's parent is the submitting span; broadcast and RPC spans
  nest under it; `requestLog` output carries `trace_id`.
- RFQ and UniswapX server tests: a `traceparent` on `/quote` yields a server span with that trace id
  and the body attributes; the outbound backend/webhook call in the same test carries the same trace id.
- Stage spans (§9): per solver, one test drives a quote and a fill through the existing `httptest` /
  simulated-backend fixtures and asserts the expected span tree by name, the `solver` attribute on
  every span, the identifier attributes on the top spans, and that a failing stage ends with status
  Error and a recorded exception while a decline ends with a `declined` event and no error status.
- `NewTracer`/`end`/`Decline` unit tests: error, `context.Canceled`, `reason_code` extraction.
- `SpanLinks` (§12): remember/lookup, TTL expiry, eviction past `maxEntries`, lowercase keys.
- Linking per solver: RFQ order poll links to the remembered quote; UniswapX `resolvedOrder` carries
  the link into the fill; 3F redeem links by Request address; RedStone result
  links by auction id; and the miss case emits `link_miss` without failing the fill.
- Existing suites stay green with the no-op provider (no behavior change when disabled).

Gate: `make format && make test && make lint && go build ./...`.

## 12. Linking a fill to its quote (best effort, all solvers)

A quote and its fill are separate context trees. In RFQ the order is discovered minutes later by
polling the backend; in UniswapX it comes through an orders channel from a poll; in 3F the offer is re-listed by the API poll and settled from an on-chain read;
in RedStone the result is a later websocket frame. After §8 a fill trace starts at the solver's own
loop tick, not at the quote. This section joins the two with a **span link** (the fill is later work
in its own trace, and trace backends render a link as a clickable jump) plus a `quote.trace_id`
attribute and a `quoteTraceId` log key so logs can be joined without the trace backend.

Best effort means: a miss (process restart, eviction, unknown id, tracing disabled) produces a fill
span without a link and a `link_miss` event naming the key; the fill continues under its loop span
with a fresh trace id exactly as if linking did not exist. The map is process-local and in-memory
by design: it is never persisted, never consulted on a code path that can fail, and a lookup cannot
return an error. Nothing about a fill's behaviour depends on the map.

### 12.1 Shared mechanism

The sweep found that no solver keeps quote-keyed state today (RFQ's store is by order id, UniswapX
by order hash, 3F by adapter+auction, RedStone by auction id; LiFi has no quote-specific event at
all, see its row). Rather than retrofit the stores, `internal/observability` gets one small helper:

```go
links := observability.NewSpanLinks(maxEntries)          // bounded, TTL per entry, mutex-guarded
links.Remember(key string, ctx context.Context, ttl time.Duration)  // stores the current span context
link, ok := links.Lookup(key)                             // trace.Link (+ trace id for logs/attrs)
```

Entries expire at their TTL and the oldest is evicted past `maxEntries` (1024 by default; a span
context is 40 bytes, so this is negligible). Keys are lowercased ids; each solver owns one instance
and chooses its key and TTL. Lookup never deletes: the same quote can be linked from a retry or a
later status update.

### 12.2 Per solver

| Solver | Remember (key → TTL) | Lookup, link, and stamp |
|---|---|---|
| **rfq** | In `handleQuote` after a quote is returned: key `quoteId`, TTL = quote validity + 1 min (the deadline the quote was issued with). | `handleOrder` (`execution.go:139`): the polled order carries `QuoteID` (`upsertQueued` at `:126`, `orderRecord.QuoteID`). Link on the `rfq.order` span; `quote.trace_id` attribute; `quoteTraceId` on the order logger. |
| **uniswapx** | In `Solver.quote` after a quote is returned: key `quoteId`, TTL = quote validity + 1 min. Both indicative and hard quotes are remembered; the hard one wins on overwrite. | `pollSource` (`polling.go:123`) when building `resolvedOrder` (`order.go:333`, `QuoteID`): the `uniswapx.order.track` span gets the link, and because `uniswapx.fill` is its child the link is visible on the fill trace. Stamp `quoteTraceId` on both loggers. |
| **lifi** | Not linked. The solver publishes standing quotes per asset pair; LiFi later sends an order that matches one of them, so there is no quote-specific event on our side to remember. The `lifi.order.<event>` span carries the `quote.id` LiFi reports, and the whole order path is one trace from the message span (§6.4). | — |
| **3f** | In `3f.offer.submit` after `createOffer` succeeds: key = the auction's Request contract address (`types.OfferExecution.Request`, known at offer time from the auction view), TTL = offer expiration + 1 h. The API discards the created offer id (`apiclient.go:59`), and the settlement path works from Request addresses read on-chain, so the address is the only id present at both ends. | `redeemReady` (`redeemer.go:56`): for each request being finalized, look up its address; the `3f.redeem` span gets one link per matched offer and `offer.linked_count`. `reconcileOffers` (`solver.go:182`) also looks up by `(adapter, auctionId)` → a second key `auction:<adapter>:<id>` remembered at the same time, and stamps `quoteTraceId` on the status-change log lines. |
| **redstone-oev** | In `handleAuction` when the solve is sent (`auction.go:229`): key `auction.ID`, TTL = `reservationTTL` (5 min, `reservations.go:39`), the same window the reservation itself is kept for. | `handleAuctionResult` (`auction.go:82`) and `handleLiquidationResult` (`auction.go:101`): link the result span to the auction span; `quoteTraceId` on the result log line. `handleBlacklisted` gets the same lookup for its `ID`. |

### 12.3 Why not the backend's `X-Request-Id` or a backend change

The backend's order list returns a fresh per-response `requestId` and no trace id, and it does not send
`X-Request-Id` on the quote request (§3), so nothing arriving from the backend identifies the quote
trace. If the backend later returns the quote's trace id on the order item, the RFQ row above switches
its lookup to that value and the solver-side map becomes a fallback; nothing else changes.

## 13. Overhead and failure isolation

Tracing must never slow down or break a quote, a fill, or a transaction. The rules, and how the
design meets them:

- **Nothing on a hot path waits on tracing.** Spans are recorded in memory and handed to the SDK's
  `BatchSpanProcessor`, which exports from its own single goroutine. Its queue is bounded (SDK
  default 2048 spans, `OTEL_BSP_MAX_QUEUE_SIZE`) and **drops** spans when full rather than blocking
  the producer. No request-path code performs network I/O for tracing.
- **Exporter failures are invisible to callers.** A dead or slow collector surfaces only through
  the `otel.SetErrorHandler` Info log (§4); no error is ever returned into solver code.
- **Startup never fails because of tracing.** If `NewTracing` cannot build the exporter or resource
  (bad `OTEL_*` values), it logs at Error and returns the disabled no-op configuration; the bot
  starts and runs exactly as without tracing.
- **Disabled means near-zero cost.** With `OTEL_EXPORTER_ENABLED` unset the global provider is the
  no-op one: `tracer.Start` returns a non-recording span with no allocation of note, `end` is a
  no-op, `otelhttp` wrappers create non-recording spans, and the only residual work is the W3C
  header parse on inbound requests and `TraceLogger` adding two fields when a remote context is
  present.
- **Enabled cost is bounded and off-path.** Span start/end is microseconds; attributes are a
  handful of bounded strings; no per-request goroutines are created; the RPC span reuses the body
  classification the metrics already do. Volume is controlled by `OTEL_TRACES_SAMPLER` (§3).
- **Linking cannot fail a fill (§12).** The map is a mutex-guarded in-memory map capped at 1024
  entries; `Lookup` returns `(link, ok)` and never an error; a miss adds an event and nothing else.
- **No panics from the tracing API.** The OTel API is nil-safe and never panics on ended spans,
  invalid contexts, or a missing provider; `end(nil)` and double `end` are safe.
- **Shutdown is bounded.** The provider flushes for at most 5 s on a fresh context after solvers and
  txmanager have drained (§4); a stuck collector cannot hold the process.
- **Verified in tests (§11).** The stage-span tests run with a recorder, the disabled-mode test
  asserts a request produces no recorded span and the same response, and a benchmark in
  `internal/observability` records the cost of `Start`/`end` with and without a provider so a
  regression is visible in review.
