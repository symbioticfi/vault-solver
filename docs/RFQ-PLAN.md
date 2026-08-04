# vault-solver — RFQ Filler solver (plan)

Porting the TypeScript `@symbiotic/rfq-filler` into `vault-solver` as the **`rfq`** solver, following
the framework boundary and conventions in [`../CLAUDE.md`](../CLAUDE.md). 

---

## 1. What the filler does

The RFQ filler is the externally-owned **solver/executor** for Symbiotic RFQ. Unlike the 3F solver
(poll-only, outbound), it is a **request/response server + an order-filling poller**. It is
**poll-only** for order discovery — matching the refactored TS filler, which dropped the `/notify`
push path; orders are found exclusively by polling the backend.

- **HTTP server** — `POST /quote` (backend fans out a swap request carrying the candidate per-adapter
  inventory snapshot in `adapters[]`; the filler prices it, applies a discount, selects the best
  adapter legs, and returns an `amountOut`), `GET /health`, and
  the code-first OpenAPI surface (`/openapi.json`, `/openapi.yaml`, `/docs`). `/quote` is gated by an
  `x-rfq-shared-secret` header (the backend peer). When explicitly enabled, authenticated `POST /swap`
  returns user-directed calldata without submitting it. There is **no `/notify` endpoint**.
- **Poller** — every `pollInterval`, `GET /orders?filler=<executor>&orderStatus=open` from the
  backend, then drives each order through `queued → submitting → submitted → {filled|expired|failed}`.
- **Execution** — builds `Executor.fill(Order, protocolSig, Swap[], DiscountSwapInput[], bytes)` and
  sends it; the `Executor` calls the `Reactor`, which calls back into `Executor.execute()` to run the
  adapter `swap`s and satisfy the order's outputs. Each on-chain `Swap`'s `vault` slot is set to the
  leg's **adapter** address.
- **State** — in-memory only: `orders` (state machine) and `attempts`.

### User-directed swap calldata

`swapEnabled: true` adds a protocol-exact `v2` lifecycle on `POST /swap`; disabled deployments do not
register the route or advertise it in OpenAPI. The backend authenticates with the same
`x-rfq-shared-secret` used by `/quote`:

1. `DISCOVERY` supplies strictly increasing exact-input samples and an adapter inventory. The solver
   performs one coherent largest-sample read and returns attainable points plus canonical shared-vault
   capacity domains.
2. `CONFIRM` selects one exact discovery point. The solver re-reads current liquidity, re-runs the
   configured strategy, verifies the same domains and output floor, validates every selected adapter,
   and stores the ordered allocation in-memory until the earliest of the requested maximum deadline,
   configured TTL, and route validity. A longer requested deadline is shortened rather than rejected.
3. `BUILD` is bound to that immutable confirmation, Router, domains, one chosen unexpired deadline, and
   one build ID. It revalidates adapter state and exact capacity, then returns only signed-swap selector
   `0x9a4568b6`, including for plans selected from discount inventory. Private discount calldata is never
   exposed through this user-directed API. The backend may choose the earliest confirmation validity as
   one aggregate deadline. A retry may use a fresh transport-only request ID; the immutable payload is
   byte-identical, while a second build ID or changed economic tuple conflicts.

For a signed adapter call, recipient and caller are the configured Router. The adapter nonce is
deterministic over build ID, chain, adapter, input token, and call index. The BUILD response returns `to`,
opaque adapter `data`, and accounting metadata; the backend maps `to`, `amountIn`, and `data` to the
Router's three-field call tuple. There is no separate Router authorization signer, deadline, or signature.

The solver signs calldata only: it neither performs the Router's transfer-before-call funding nor
broadcasts a transaction. The public transaction uses ordinary ERC-20 approval and zero native value;
the Router transfers each exact leg directly from the user to its adapter, invokes the returned data,
then transfers exact declared outputs to recipients. Plans contain at most 64 calls, and BUILD
independently enforces its confirmed
aggregate output floor before returning. Aggregation across solvers is safe only when selected
liquidity-domain sets are disjoint.

Discovery and confirmation records are bounded, expiring, in-memory state. A process restart
invalidates them, so the backend must repeat `DISCOVERY` and `CONFIRM`; it must never reuse an old
`BUILD` against a restarted solver.

The `/quote` request inventory (`adapters[]`) still matches the TS `solverQuoteRequestSchema`, but the
solver maps that boundary shape into the shared LiquidLane terms from
[`LIQUIDLANE-CONVENTIONS.md`](LIQUIDLANE-CONVENTIONS.md): `Inventory` is
`adapter + tokenIn + tokenOut + maxAssets + maxRate`, and RFQ's external `asset` field is the shared
`tokenOut`. Pricing leg types are **direct** (`discountId == null`, public adapter rate) and
**discount** (`discountId != null`, a signature-gated private rate negotiated off-chain via the backend
`/discounts` flow). Both are in scope for full parity — discount legs are built in **P3** (§4), after
the direct path is solid; they are sequenced last, not dropped.

---

## 2. How it maps onto the framework

A new self-contained `internal/solvers/rfq/` implementing `solver.Solver` — no framework edits
(CLAUDE.md modularity rule). The generic layer is reused as-is:

- **`Run(ctx)`** starts the RFQ **HTTP listener** (`/quote` + optional `/swap` + `/health` + OpenAPI) *and* the poll
  loop, blocking until ctx cancels. The HTTP server is an RFQ-specific concern and lives in the RFQ
  package; the framework's observability server (`:9090`, metrics/health/ready) stays separate.
- **OpenAPI is code-first via Huma**: the request/response structs in `apitypes.go` carry validation
  tags (`enum`/`pattern`/`minimum`/`maximum`/`format:"uuid"`, …) that drive *both* inbound validation
  *and* the generated OpenAPI 3.1 spec served at `/openapi.json` + `/docs`. A schema violation returns
  **422** (Huma/RFC 9457), where the TS Hono/Zod filler returns 400 — same reject, different status
  code; this is the one deliberate external-interface deviation on `/quote`.
- **`/metrics` is served by the framework's shared observability server, not the RFQ HTTP server.**
  Prometheus metrics belong to the generic `internal/observability` layer (one registry, one
  `/metrics` on `:9090`), so the RFQ server doesn't expose its own. Instead the solver **registers its
  collectors on the shared registry** via `deps.Metrics.Registerer()` in the factory: a Huma/HTTP
  middleware records `rfq_filler_http_requests_total{method,route,status}` and
  `rfq_filler_http_request_duration_seconds` (route is allowlisted to bound cardinality). The
  framework also registers the standard Go runtime + process collectors, so `/metrics` carries CPU,
  memory, goroutines, GC, and FDs. Net effect: the same metric names the filler exposed, surfaced on
  the shared observability port rather than a per-solver endpoint.
- **Quote-server middleware** (outer→inner: body cap → access log → metrics → panic recovery): every
  request gets a generated/propagated `X-Request-Id` (echoed on the response and threaded into the
  handler context so quote logs carry it), an access log line (method/route/status/duration), the
  metrics above, a 1 MiB inbound body cap, and panic→500 recovery (the recovered panic is logged at
  Error, so it reaches the Sentry sink). The `http.Server` also sets read/write/idle timeouts.
- **Optional Sentry sink** — when `SENTRY_DSN` (and optional `SENTRY_ENVIRONMENT`) is set, the
  framework tees Error+ log entries to Sentry (a zap core in `internal/observability`), flushed on
  shutdown. Strictly opt-in: unset DSN ⇒ no sink. This is richer than the prior filler, which only
  init'd Sentry for uncaught crashes.
- **Fills go through the shared `txmanager`** (CLAUDE: solvers never send directly). The RFQ package
  builds the `Executor.fill` calldata; txmanager owns the nonce, send, and receipt/revert.
- **On-chain reads use the shared LiquidLane reader over `chain.Multicall`.** Exact-input pricing is
  route-specific and reads the executable amount after the adapter's current `minDiscount`; adapters
  that produce the same output asset are never collapsed into one oracle observation.
- **Addresses + backend URL come from `solver.config`** (config-is-king); secrets
  (`backendSharedSecret`, the caller key) via `*Env` indirection (`os.Getenv` at point of use).
- **Bindings** for `Executor`/`Reactor` (from the `rfq` build) and `LiquidLaneAdapter`/`UniversalDelegator`/
  `IVaultV2`/`IERC4626` (from a standalone `core-mirror` build) are vendored via `make refresh-abi` +
  `bindings` (two `FORGE_OUT`/`CORE_MIRROR_OUT` sources). The nested `fill`/order ABI is encoded/decoded
  via the generated bindings, never hand-rolled.
- **Signer** — the framework's single EOA is the RFQ **caller** (must be in the Executor's `callers`
  allowlist, added by the owner via `setCallers`).

### Component port map (TS → Go)

| TS (`rfq-filler/src/`) | Go (`internal/solvers/rfq/`) |
|---|---|
| `index.ts` + `server.ts` (Hono) | `solver.go` (`Run`: HTTP server + poll loop) + `server.go` (Huma routes/auth) |
| `api.ts` (Zod schemas) | `apitypes.go` (request/response structs + Huma validation tags) |
| `quote.ts` + `strategy.ts` | `quote.go` + `strategy.go` (quote-server wiring and typed input assembly) + `strategies/` (the pluggable decision layer: `default` = allocation, `webhook` = external decider) |
| `execution.ts` | `execution.go` (poll loop, order state machine, fill; fresh fill-plan production lives in the strategy) |
| `executor.ts` + `reactor`/`contracts.ts` | `order.go` (encode/decode reactor order, `fill` calldata) |
| `backend.ts` + `discounts.ts` | `backend.go` (thin adapter over the generated `api/rfqbackend` client for `/orders`) + shared `internal/liquidlane/discounts` (`/discounts`) |
| `contracts.ts` + `inventories.ts` | `chainreader.go` (multicall adapter/vault reads) + shared `chain` |
| `domain.ts` | `store.go` types + `strategies/types` (RFQ strategy input/output and fill plan) + shared `liquidlane.QuoteCandidate` |
| `config/env.ts` + deployment manifests | `config.go` (typed `solver.config`) |
| `db`/repositories | `store.go` (in-memory orders/attempts) |
| `metrics.ts` | `metrics.go` (collectors on the shared registry) + framework `internal/observability` (`/metrics` — see §2) |

### Pluggable strategy layer

Pricing, discount, and leg-selection are not baked into the solver — they live behind a per-solver
**strategy** interface (`DecideQuote` at quote time, `BuildFillPlan` at fill time), selected by a
`strategy: { name, config }` block. The solver owns transport (HTTP, chain reads, signing,
submission), validates protocol data, and normalizes backend inventory plus current adapter reads into
`[]liquidlane.QuoteCandidate`; the strategy owns only the decision. Two ship in-tree:

- **`default`** — the in-process greedy discount + leg selector. `BuildFillPlan` always decides again
  from current LiquidLane candidates and re-binds the result to the awarded order
  (`tokenIn`/`tokenOut`/`amountIn`, `quotedAmountOut ≥ required`). When aggregate
  adapter capacity cannot cover an exact-input request, it still returns the available `maxAssets` as
  output and assigns the residual input to the final leg, surfacing the shortfall as price impact
  instead of declining the quote.
- **`webhook`** — a transport-only adapter that delegates to an external decider over JSON.
  `BuildFillPlan` re-calls the decider with current candidates and the order's
  `amountIn`/`requiredAmountOut`.

Quote and fill use the same input shape; fill simply supplies freshly normalized candidates plus the
awarded `requiredAmountOut`. Both strategies reuse one `FillPlanFromQuote` structural validation path.
There is no quote-plan cache or default/webhook-specific Executor mapping.

`permissionedTokens` is both the membership set for `tokensToQuote` and, only when that scope is
`permissioned`, a solver-owned hard constraint. The solver sets `RequireSingleRoute` on both quote
and fill snapshots for admitted tokens in that scope. A strategy must choose one candidate that
covers the entire `amountIn`; partial candidates, including direct and discount variants of the same
adapter, cannot be combined. The default strategy chooses the best fully viable candidate and
declines if none exists. The solver independently rejects any quoted or fill plan whose leg count is
not exactly one, so webhook and fresh fill planning fail closed at the same boundary. The `all` and
`permissionless` scopes retain greedy multi-candidate aggregation.

`minAmountsIn` is a second solver-owned quote gate, independent of the token scope: an optional map of
input-token address → minimum request size in that token's **base units** (decimal string). It is
evaluated in `quoteService.quote` right after the token-scope check and before any adapter filtering or
chain read, so a below-minimum request costs nothing and returns the usual no-quote (`nil, nil` ⇒ HTTP
204). The comparison is strict: `amountIn == min` still quotes. Keys are parsed into `common.Address`,
so configured checksum casing does not matter; values must parse as positive integers (zero, negative,
non-numeric, or a zero/invalid address key is a startup error, as is the same token listed twice in
different casing). Tokens absent from the map have no floor. This is how RWA inputs (HYBOND, deJAAA,
deJTRSY) enforce a redemption-sized minimum without a per-token code path. Covered by
`gating_test.go` (`TestParseConfigMinAmountsIn`, `TestParseConfigMinAmountsInErrors`,
`TestQuoteMinAmountIn`) and `server_test.go` (`TestServer_QuoteBelowMinAmountNoContent`).

The generic strategy pattern and trust model (solver provides raw facts; the trusted strategy is the
brain; the solver only enforces its own structural and safety constraints) are documented once in
[`strategy-plan.md`](strategy-plan.md), shared with every solver. Shared LiquidLane fact conventions
are documented in [`LIQUIDLANE-CONVENTIONS.md`](LIQUIDLANE-CONVENTIONS.md). The concrete RFQ
input/output types (`QuoteInput`/`QuoteOutput`, `FillInput`/`FillPlan`) live in
`internal/solvers/rfq/strategies/types`; the candidate itself is the shared
`liquidlane.QuoteCandidate`.

The RFQ solver is the protocol adapter around the shared LiquidLane reader. It obtains current
amount-specific `FillQuote`s, and `NormalizeOracleInventory` binds each backend inventory entry to its
physical route and derives direct rates from
the executable `MaxAmountOut` (gross `getAmountOut` after current `minDiscount`), then submits normalized candidates to the same `QuoteTask` engine used by
UniswapX. Because the RFQ request carries only output-asset decimals, the solver resolves and caches
`tokenIn` decimals before interpreting either direct or signed-discount rates. It also binds each advertised
adapter to its on-chain vault and asset (using startup-resolved metadata for configured adapters) and rejects
asset or decimal mismatches. Before normalization, candidates sharing the resulting vault `CapacityID`
receive one bounded allocation of that shared output capacity, so a multi-adapter quote cannot promise the
same vault assets twice. At execution the solver repeats the read and normalization from fresh RFQ
inventories. The default strategy
converts those typed candidates into the same `FillTask` used by LI.FI and UniswapX. The tasks explicitly request RFQ's residual-input-as-price-impact
semantics; deterministic splitting, direct/private alternatives, capacity, and output sizing stay inside
the shared engine. RFQ omits its optional gas pricing model, so no RFQ gas configuration or strategy
payload fields are required. The strategy has no chain/logger dependencies and only maps shared
solutions to RFQ response/Executor legs.

Signed-discount HTTP transport, live-offer filtering, and selected-term identity/route/deadline validation
are shared through `internal/liquidlane/discounts`; RFQ keeps its backend candidate policy and executor ABI mapping.
Advertised offers are parsed once into typed addresses, ids, amounts, decimals, and deadlines before
they become `liquidlane.Inventory`; expired or malformed offers fail closed. RFQ keeps only its
executor-specific `discountSwap` calldata mapping.

---

## 3. Configuration (env-agnostic: local / hoodi / mainnet)

One code path; per-environment differences are pure YAML. Sketch of `solver.config` for `rfq`:

```yaml
solvers:
  - name: rfq-filler
    config:
      strategy:                                         # pluggable decision layer (omit ⇒ default)
        name: default                                   #   "default" (in-process) | "webhook" (external decider)
        config: {}
      backendUrl: https://rfq-backend.example
      backendSharedSecretEnv: RFQ_BACKEND_SHARED_SECRET # env var NAME (secret never in config)
      listenAddr: ":42073"                              # quote HTTP server (poll-only; no /notify)
      executor:             "0x…"                       # Executor (bot EOA is an authorized caller — setCallers allowlist)
      reactor:              "0x…"
      pollIntervalMs: 3000
      orderLimit: 20
      swapEnabled: false                                # opt-in authenticated POST /swap
      router: ""                                        # required deployed Router when enabled
      swapQuoteTtlMs: 30000                             # maximum in-memory discovery/confirmation lifetime
      solverMode: external                              # "external" (default) | "internal" — see below
      minAmountsIn:                                     # optional per-input-token floor (base units)
        "0x…tokenIn": "1000000000000000000"             # below ⇒ no quote (204); equal ⇒ still quotes
      adapters:                                         # LiquidLane adapter addresses (whitelist + fill planning)
        - "0x…liquidLaneAdapter"                        # vault + collateral resolved on-chain at startup
```

**`solverMode` — the single internal/external knob (default `external`).** The backend discounts API is
available only to **internal Symbiotic** solvers, so one mode flag drives both the discount-API gate and the
adapter whitelist (it replaces the earlier separate `adapterWhitelistEnabled` / `discountsEnabled` flags — a
config still carrying either is rejected at startup so operators migrate):

- **`external`** (default — the open-source filler external parties run): **never touches the discounts
  API** — skips `GET /discounts` in fill planning, never calls `POST /discounts` at fill (a surfacing discount
  leg is failed closed). It uses **only its own adapters**, which scope quoting/filling and are
  **required** (no discounts fallback → an empty list is rejected at startup). Before starting HTTP or
  polling, every configured adapter must directly authorize the executor through `owner`, `marketMaker`,
  or `isFiller`; startup fails otherwise and emits a structured error with mode, executor, configured
  adapters, and the underlying authorization reason.
- **`internal`**: uses **public discounts** (`GET`/`POST /discounts`). Its optional `adapters` scope the quote
  path and add permissioned inventory to fill planning; discount recovery during execution remains unrestricted.
  Direct and signed-discount candidates are **deduped** (`discountInventories` drops a discount whose adapter is
  already in the configured/permissioned set).

Both behaviours are **derived from `solverMode` on demand** — no redundant config fields. `Config` exposes
`usesDiscounts()` (`mode == internal`), `restrictsToAdapters()` (external execution with configured
adapters), and `quoteScopesToAdapters()` (either mode with configured adapters). `buildServices` uses those
facts to wire the discount gate and the two path-specific adapter scopes. Covered by
`discounts_disabled_test.go`, `config_test.go`
(`TestParseConfig_SolverMode`), and `solver_test.go` (`TestBuildServices_WhitelistWiring`).

The signing key is the framework `signer` (the caller EOA); `chain.rpcUrl/chainId` select the network.
The per-quote adapter inventories arrive in the `/quote` request body (`adapters[]`); the `adapters`
list serves two purposes:

- **Adapter whitelist** — scoping is per-path. The **quote** path scopes to the configured `adapters`
  whenever `adapters` is non-empty, in **both** `external` and `internal` mode (`quoteScopesToAdapters`):
  non-configured adapters in a `/quote` request are dropped (none left ⇒ 204), so an `internal`-mode
  filler advertises quotes only for its own adapter universe (e.g. a per-solver adapter). The **execution**
  path scopes to the configured `adapters` only in `external` mode (`restrictsToAdapters`): there backend
  discounts with a non-configured adapter are ignored during fill planning. `internal` mode never restricts
  filling — discount-driven planning may legitimately route through any advertised adapter — so with no
  `adapters` configured an `internal` filler quotes and fills through every advertised adapter.
- **Fill planning**: it bounds the candidate adapter universe the fill-time multicall scans
  (direct inventories are whitelisted by construction).

---

## 4. Build phases

All phases below are committed scope. Phasing is about sequencing and reviewable increments, not
dropping features.

0. **(done)** Vendor RFQ ABIs: `Executor`/`Reactor` from `../rfq/out`, and `LiquidLaneAdapter`/
   `UniversalDelegator`/`IVaultV2`/`IERC4626` from a standalone `core-mirror` build →
   `api/bindings/rfq/` + `api/bindings/{delegator,vaultv2,erc4626}`. CGO-free build holds.
1. **(done) Quote path** — `config.go`, bindings, route-specific amount quote reads (`paused`,
   `getMaxAssets`, `getAmountOut`, `minDiscount`; decimals cached), `strategy` pricing + discount + leg selection (direct legs), Huma HTTP server (`/quote`,
   `/health`, `/openapi.json` + `/docs`, shared-secret auth), in-memory store. Unit-tested (pricing
   golden numbers, config, httptest server).
2. **(done) Execution** — backend client (`/orders`), **poll-only** loop + order state machine
   (`queued→submitting→submitted→{filled|expired|failed}`), reactor-order decode + `Executor.fill`
   (mixed overload, golden selector test) via the shared txmanager (revert→failed), attempt tracking,
   signed-order filler/deadline/output terms as the execution source of truth with fail-closed backend
   envelope consistency checks,
   and on-chain **fresh fill planning via a single multicall** over the configured per-vault adapters
   (adapter views + `marketMaker`/`owner`/`isFiller` authorization filter). Direct legs only.
   Unit-tested (state machine with fakes, backend httptest).
3. **(done) Discount legs** — backend `/discounts` (`resolveDiscount` + `listDiscounts`),
   discount-swap encoding (`IReactorDiscountSwapInput` from the resolved signed discount) wired into
   `Executor.fill`, discount-aware strategy selection (legs price off the vault `maxRate`), and
   discount inventories in fill planning. Direct + discount fills now match the TS filler. Unit-tested
   (discount-leg selection, discount fill resolves + encodes).
4. **(done) Adapter whitelist** — port of TS filler PR #54: quoting/filling restricted to the
   configured `vaults[].adapter` set (originally `adapterWhitelistEnabled`; now auto-enabled by
   `solverMode: external` when `adapters` is non-empty — see §3),
   fill-time discounts filtered by the same set, and a guard that fails the order when a
   backend-resolved discount's adapter differs from the quoted strategy leg's adapter (no tx is
   sent; a still-open order is re-armed and re-evaluated next poll, matching the TS lifecycle).
   Unit-tested (whitelist build/filter, config flag + zero-address rejection, factory wiring, quote
   200/204 paths incl. disabled toggle, fill-time discount filter, mismatch → failed order with no
   tx).
5. **(done) Permissioned-scope single-route constraint** — when `tokensToQuote` is `permissioned`,
   quote and fill inputs use one candidate instead of aggregation, and the solver rejects multi-leg
   strategy/webhook outputs before publication or calldata construction. Input beyond that route's
   output capacity is absorbed as price impact, matching the other exact-input scopes. Cold fill
   planning applies the same constraint. Unit-tested across scope gating, permissionless aggregation,
   single-route capped output, webhook rejection, and fresh planning.
6. **(done) User-directed swap calldata** — opt-in authenticated `DISCOVERY`/`CONFIRM`/`BUILD`,
   immutable bounded confirmation state, capacity-domain-preserving aggregation, signed-only adapter
   calldata even for discount-selected plans, capped confirmation validity plus one chosen aggregate BUILD
   deadline, transport-only request-ID retries over immutable cached payloads, a 64-call bound,
   three-field Router calls, and fail-fast Router/static adapter validation. This path never
   sends a transaction and does not alter the legacy fill poller or its private discount execution.

**Reads are multicall-batched** end to end: amount-specific strategy evaluation uses the shared
per-route fill-quote batch (`paused`, `getMaxAssets`, `getAmountOut`, `minDiscount`), while inventory
refresh uses (`paused`, `getMaxAssets`, `getMaxRate`) — each adapter's `vault` and collateral `asset` are resolved once at startup (from
`adapter.vault()` / `vault.asset()`), not re-read per fill plan, so there are no per-read round-trips.

---

## 5. Open items / prerequisites

- **Authorized caller of the `Executor`** — the bot EOA must be added to the Executor's `callers`
  allowlist (owner-only `setCallers`) before fills land (onboarding
  prereq, analogous to 3F's offer-signer). Document; do not grant from the bot.
- **Per-environment inputs needed to run**: backend base URL, `Executor` / `Reactor` addresses, the
  LiquidLane adapter address list (`vaults`; adapter whitelist + fill planning — each adapter's vault and
  collateral are resolved on-chain at startup; with the whitelist enabled an empty list declines every
  quote), the backend shared secret, and the caller key (last two via env). Hoodi addresses are known
  from the TS deployment manifest; local from the rfq-integration local-stack deploy.
- **RPC**: a primary `chain.rpcUrl` plus optional `chain.rpcFallbackUrls` (HTTP(S), tried in order
  when the primary is unavailable). Fallback is implemented in the generic `internal/chain` layer as a
  barebones viem-style HTTP transport that fails over on transport/5xx/429 errors only (never on a
  JSON-RPC error such as a revert), so every read/send path inherits it unchanged. Endpoints are
  operator-configured (no hardcoded public-RPC lists); duplicates are de-duped; all must be the same
  chain. A single `rpcUrl` keeps the plain dial (any scheme).
- **Pricing follows the TS greedy port for all inputs** — permissioned inputs additionally use the
  single-route constraint above. A richer quoting strategy is a later follow-up (mirrors the
  3F pricing TODO), or an operator can plug their own via the `webhook` strategy (see the strategy
  layer below).
- **Discount-leg rate rounding** — a discount leg prices off the backend's advertised `maxRate`, which
  is the adapter oracle price with the discount already applied *and floored*. The adapter rounds down
  in the opposite order: `getAmountOut` floors `amountIn × price × 10^outDec / (1e18 × 10^inDec)`
  first, then `swap(DiscountSwap, ...)` applies the discount and floors again. The two nested roundings
  differ by at most one unit, and the difference falls our way often (roughly a fifth to a half of
  amounts at a non-zero discount) — so pricing at the raw `maxRate` predicts one unit more output than
  the adapter delivers. That is not an adapter revert (the adapter computes `amountOut` itself and
  `InvalidSwapRate` cannot trigger, since `discount ≥ minDiscount`); it reverts in
  `Reactor._fill`, which pulls the order's *signed* outputs out of the Executor after `execute()`
  returns. With no `priceBufferBps` in RFQ and `Finalize` distributing the full achievable output, the
  slack is exactly zero whenever the price has not moved since the quote, so the fill fails gas
  estimation and the order retries until it expires. `NormalizeOracleInventory` therefore re-derives
  every discount candidate's rate through `liquidlane.ConservativeAdvertisedRate`, which shaves one
  unit off the predicted output and converts it back to a rate; the round trip through
  `RateForAmountOut` floors, so downstream `AmountOutForRate` call sites need no change. Direct legs
  are unaffected — they already re-derive their rate from a live `getAmountOut` read. The exact
  alternative (clamp against `AmountOutAfterDiscount(GrossAmountOut, discount)`, as
  `discounts.AdvertisedFillQuotes` does) needs the discount ppm, which the `/quote` request's
  `adapters[]` entries do not carry; revisit if that field is ever added to the backend contract.
- **Quote latency** — `/quote` is synchronous in the backend's fan-out, so keep it cheap: pricing is
  one `getAmountOut` multicall, and `tokenIn` decimals are read once and cached. A warm quote is a
  single multicall; only the first quote for a not-yet-seen `tokenIn` adds a one-off `decimals` read.
  Keep it that way — don't add per-quote chain reads outside that one multicall.

### Parity with the current TS filler

**Status (verified against the current TS `rfq-filler` working tree): functional parity plus the
permissioned-scope single-route constraint described above.** The
pricing/sizing/leg-selection math, the `Executor.fill` selector + nested tuple encoding, the backend
endpoints actually used (`GET /orders` ×3 query shapes, `GET /discounts`, `POST /discounts` resolve),
and the fill-time RPC read/authorization set are all 1:1. The Go port adds a few **fail-closed
hardenings the TS filler lacks** — an order-deadline check before fill, a strategy↔order
`tokenIn`/`tokenOut`/`amountIn` binding, txHash validation on reconcile, a single-entry guard on the
batch discount-resolve shape, a conservative discount-leg rate that cannot out-predict the adapter's
nested rounding (§5), and TTL eviction of stale terminal orders (TS maps grow unbounded).
A few **intentional, non-fund-moving divergences** remain, by design:

- **Quote-time oracle revert** — a reverting `getAmountOut` makes the Go quote *skip that asset and
  price the rest* (multicall `allowFailure`), whereas the TS filler throws and fails the whole quote.
  Go is "price what you can"; revisit if strict all-or-nothing quoting is wanted.
- **Validation status code** — Huma returns **422** on schema violations vs the TS filler's **400**
  (see §2). The reject is identical; only the code differs.
- **Backend base-URL prefix** — the generated client embeds the spec's `/api/v1` prefix, so the Go
  `backendUrl` is the backend **host root**; the TS filler's base URL already includes the path. Set
  each deployment's `backendUrl` accordingly (mismatch ⇒ 404 on every backend call).
- **Internal discounts path** — the discounts API is internal-only and served under `/api-internal/v1`
  (orders stay on `/api/v1`). Rather than regenerate the client for a routing detail,
  the shared discounts client rewrites generated `/api/v1/discount(s)` requests to
  `/api-internal/v1/...` at its transport boundary; orders pass through unchanged. RFQ uses it through
  `internal/liquidlane/discounts`; LIFI reuses the same client and validation for its discount-backed
  fills. Covered by httptest assertions.
- The `{adapter, tokenToRedeem}` discount-resolve selector exists in TS types but is unused by
  execution (both sides resolve by `discountId`); Go omits it. Cosmetic.

- **Poll-only** — the `/notify` push endpoint was removed from the TS filler; the Go port has no
  `/notify` route, no wake channel, and `source` is always `"poll"`.
- **Backend field names** — the backend's executable order view emits **`protocolSignature`** (not
  `signature`); `backendOrder.ProtocolSignature` decodes that key. Discount payloads use **`adapter`**
  (not `vault`): `discountTerms`/`discountListItem` parse `json:"adapter"`; the on-chain
  `Discount.vault` slot is then filled from that adapter address (positional binding name unchanged).
  A wrong tag here silently zero-fills and breaks every fill, so these are pinned by tests.
- **Fill-time discount filter** — matches TS exactly: keep discounts where the adapter is
  whitelisted, `tokenToRedeem == tokenIn`, and the adapter is not already permissioned; the
  `asset == tokenOut` check is left to the strategy evaluator (no extra collateral pre-filter).
- **Adapter whitelist** — ports TS PR #54: the whitelist is the configured `vaults[].adapter` set
  (the Go config analogue of the TS deployment manifest's `vaults`). It was originally gated by an
  explicit `adapterWhitelistEnabled` flag (the TS `RFQ_FILLER_ADAPTER_WHITELIST_ENABLED` env); that flag
  has since been folded into **`solverMode`** (§3), and scoping is now per-path. The **quote** whitelist
  is enabled whenever `adapters` is non-empty in either mode (`quoteScopesToAdapters`); the **execution**
  whitelist (fill-time discount filtering) is `external`-only (`restrictsToAdapters`). Enforcement points:
  `/quote` adapter filtering (none left ⇒ 204) — quote-scoped; fill-time discount filtering — execution-scoped;
  and the unconditional fill-time resolved-discount ↔ strategy-leg adapter equality check (mismatch ⇒
  order failed, no tx; while the backend still lists the order open it is re-armed on the next poll
  and the discount re-resolved, so a transient mis-resolution self-heals — same lifecycle as TS).
  Deliberate divergences: the Go default profile is **`external`**, which **requires `adapters`** (an empty
  list is rejected at startup — no discounts fallback) and scopes quoting/filling to them; and Go rejects
  zero-address `adapters` entries at startup (the TS manifest schema does not), so a placeholder config
  cannot put `address(0)` on the whitelist.
- **`requestId`/`quoteId`** carry `format:"uuid"` to mirror the TS `z.uuid()` inbound validation.
- **Validation status code** — Huma returns **422** on schema violations vs the TS filler's **400**
  (see §2). The reject is identical; only the code differs.
- **LiquidLane adapter migration (supersedes the old InstantRedemptionAdapter).** The RFQ adapter is
  now core-mirror's **per-vault `LiquidLaneAdapter`** (the `delegator-simplify` branch), not the old
  one-adapter-many-vaults `InstantRedemptionAdapter`. Reads take **no vault arg**: `paused()`,
  `vault()`, `marketMaker()`, `isFiller(marketMaker, filler)`, `getMaxAssets(tokenToRedeem)`,
  `getMaxRate(tokenToRedeem)`, `getAmountOut(tokenToRedeem, amountIn)` (2-arg). The vault's collateral
  is `IERC4626(vault).asset()` (the vault is now ERC4626). `curatorRegistry`/`getCurator`, `limit`,
  `allocated`, and the `capAssetsByTokenLimit` cap are **gone**; authorization is `marketMaker()` /
  `owner()` / `isFiller()` == executor. The bindings come from a standalone `core-mirror` build
  (`CORE_MIRROR_ABIS`); the swap-side `ILiquidLaneAdapter` (rfq) and read-side `LiquidLaneAdapter`
  (core-mirror) share a basename, so the read ABI is vendored from core-mirror's build.
- **`Executor.fill` re-encoded.** swapInputs are now `(address adapter, (recipient,tokenIn,amountIn,
  amountOut) swap)[]` and discountSwapInputs `(address adapter, (discount{tokenToRedeem,…},sig,
  protocolDeadline), protocolSig, recipient, amountIn)[]` — the inner discount dropped its `vault`
  slot and the discount input dropped `amountOut`. Selector `0x2b137442` (pinned by the golden test).
- **Quote discount removed** — the quoted output is the adapter oracle `getAmountOut` directly;
  `quoteDiscountBps`/`applyQuoteDiscount` are gone (matches the current TS filler).
- **OpenTelemetry — intentionally not ported.** The TS filler declares `@opentelemetry/*` packages in
  `dependencies` but never initializes an SDK, tracer, or spans (no `opentelemetry.ts`, no `OTEL_*`
  reads in `src/`) — they are unused/dead deps (the OTel envs belong to the rfq-*backend*). So there
  is nothing to port; tracing is out of scope unless fleet-wide tracing is later required.

### Backend OpenAPI spec (vendored)

The RFQ backend serves its spec at `/api/v1/openapi.json` (hono-openapi, generated at runtime). It is
vendored at `openapi/rfq-backend.openapi.json` as the contract-of-record the `rfqbackend` client is
generated from, and refreshed with `make refresh-rfq-openapi` (`RFQ_OPENAPI_URL=...`).

- **The temp railway deployment is stale.** As of this writing it is built from a commit *before* the
  backend renamed discount `vault`→`adapter` and order `signature`→`protocolSignature`, so its served
  spec disagrees with both the current backend code and the current filler. The vendored file is
  generated from **current backend code**, not that deployment. Until the deployment is refreshed,
  regenerate the vendored spec from a backend running current code — e.g. `pnpm tsx
  scripts/dump-openapi.ts` in `rfq-backend` (builds the Hono app in-process, no DB, dumps the spec).
- **Generated Go client (`api/rfqbackend/`).** The spec now carries `components` schemas with `$ref`s
  (the earlier hono-openapi all-inlined limitation is fixed), so the client is generated with the Java
  **openapi-generator** (`make refresh-rfq-client`) — the only generator that ingests this OpenAPI 3.1
  spec (`oapi-codegen`/kin-openapi and `ogen` both reject its numeric `exclusiveMinimum` + `type:[…,null]`
  unions; see the Makefile note). `backend.go` is now a thin adapter that calls the generated client and
  projects its models into the solver's internal domain rows. Two deliberate carry-overs: the generated
  client embeds the spec's `/api/v1` path prefix (so `backendUrl` is the host root), and the
  `ResolveDiscountResponse` `anyOf` union is consumed via its single shape (the batch shape is accepted
  only when it contains exactly one entry — fail closed). `apitypes.go` is unchanged: it is the filler's
  own inbound `/quote` server contract (Huma validation tags), not a backend-client type.
