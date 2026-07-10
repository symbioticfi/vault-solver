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
  inventory snapshot in `adapters[]`; the selected strategy prices it, selects direct and eligible
  signature-gated discount legs, caches the default strategy's fill plan by `quoteId`, and returns an
  `amountOut`), `GET /health`, and the code-first OpenAPI surface (`/openapi.json`, `/openapi.yaml`,
  `/docs`). `/quote` is gated by an `x-rfq-shared-secret` header (the backend peer). There is **no
  `/notify` endpoint**.
- **Poller** — every `pollInterval`, `GET /orders?filler=<executor>&orderStatus=open` from the
  backend, then drives each order through `queued → submitting → submitted → {filled|expired|failed}`.
- **Execution** — selects exactly one backend row matching the requested `orderId`, decodes its signed
  `encodedOrder`, and treats that tuple as authoritative for filler, input, amount, deadline, and
  outputs. Optional backend filler/output projections must agree. It then builds
  `Executor.fill(Order, protocolSig, Swap[], DiscountSwapInput[], bytes)`; each direct `SwapInput`
  carries the selected LiquidLane `adapter` explicitly.
- **State** — in-memory only: the default strategy's fill plans (by `quoteId`), order records (state
  machine), and attempt counts. Expired fill plans are lazily removed on lookup and swept at a bounded
  cadence; terminal orders retain their existing three-hour eviction.

The `/quote` request inventory (`adapters[]`) and the strategy use **adapter/asset** terminology
(`adapter`, `asset`, `assetDecimals`, `maxAssets`, `maxRate`, `discountId`) — a 1:1 match for the TS
`solverQuoteRequestSchema`. Pricing leg types: **direct** (`discountId == null`, public adapter rate)
and **discount** (`discountId != null`, a signature-gated private rate negotiated off-chain via the
backend `/discounts` flow). Both are implemented; discount legs are sequenced last, not dropped.

---

## 2. How it maps onto the framework

A new self-contained `internal/solvers/rfq/` implementing `solver.Solver` — no framework edits
(CLAUDE.md modularity rule). The generic layer is reused as-is:

- **`Run(ctx)`** owns the RFQ **HTTP listener** (`/quote` + `/health` + OpenAPI) and order poller in
  one `errgroup`. A listener failure cancels and joins the poller; parent cancellation drains and joins
  both before `Run` returns. The HTTP server is an RFQ-specific concern and lives in the RFQ package;
  the framework's separately supervised observability server (`:9090`, metrics/health/ready) stays
  separate, and failure of either listener is process-fatal.
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
  builds the `Executor.fill` calldata. Txmanager's dispatcher serializes nonce allocation, signing,
  and initial broadcast, then independent trackers supervise admitted/ambiguous transactions so one
  pending receipt does not block later nonces. Trackers require a canonical block-hash match plus the
  configured confirmation depth and use bounded same-nonce/same-payload fee replacements. Results use
  the exact states `not_broadcast`, `rejected`, `broadcast_unknown`, `pending`, `confirmed`, `reverted`,
  and `unresolved`; `SafeToRetry()` is true only for `not_broadcast` and `rejected`, and consumers never
  infer ambiguity from `Err`.
- **Fill outcomes reconcile explicitly.** `confirmed` enters local `submitted` and reconciles with the
  backend. `unresolved` also enters `submitted`, retains the newest signed hash/error, reconciles, and
  is never locally re-armed merely because `Err` is non-nil. `not_broadcast`, `rejected`, and `reverted`
  enter `failed`. An unexpected/intermediate state follows the conservative submitted/reconciliation
  path, never a local retry.
- **On-chain reads use `chain.Multicall`** (the adapter exposes many per-vault views per quote).
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
| `quote.ts` + `strategy.ts` | `quote.go` + `strategy.go` (quote-server wiring) + `strategies/` (the pluggable decision layer: `default` = pricing/discount/leg selection, `webhook` = external decider) |
| `execution.ts` | `execution.go` (poll loop, order state machine, fill; fill-plan production/recovery lives in the strategy) |
| `executor.ts` + `reactor`/`contracts.ts` | `order.go` (encode/decode reactor order, `fill` calldata) |
| `backend.ts` + `discounts.ts` | `backend.go` (thin adapter over the generated `api/rfqbackend` client: `/orders`, `/discounts`) |
| `contracts.ts` + `inventories.ts` | `chainreader.go` (multicall adapter/vault reads) + shared `chain` |
| `domain.ts` | `store.go` types + `strategies/types` (strategy input/output, fill plan, legs, candidates) |
| `config/env.ts` + deployment manifests | `config.go` (typed `solver.config`) |
| `db`/repositories | `store.go` (in-memory orders/attempts); the default strategy owns its fill-plan cache |
| `metrics.ts` | `metrics.go` (collectors on the shared registry) + framework `internal/observability` (`/metrics` — see §2) |

### Pluggable strategy layer

Pricing, discount, and leg-selection are not baked into the solver — they live behind a per-solver
**strategy** interface (`DecideQuote` at quote time, `BuildFillPlan` at fill time), selected by a
`strategy: { name, config }` block. The solver owns transport (HTTP, chain reads, signing,
submission) and hands the strategy a snapshot of raw facts (the request, the per-adapter inventory
candidates); the strategy owns the decision. Two ship in-tree:

- **`default`** — the in-process faithful port (greedy discount + leg selection). It caches its
  quote-time plan by `quoteId` and, on a cold cache, rebuilds from live on-chain state, re-binding the
  plan to the awarded order (tokenIn/tokenOut/amountIn, `quotedAmountOut ≥ required`). Expired plans
  are removed on lookup and by a bounded periodic sweep.
- **`webhook`** — a transport-only adapter that delegates to an external decider over JSON. It keeps
  **no local cache**: `BuildFillPlan` re-calls the decider at fill time (carrying the order's
  `amountIn`/`requiredAmountOut`), so the external implementer owns caching and fill-time validation.

The generic strategy pattern and trust model (solver provides raw facts; the trusted strategy is the
brain; the solver executes the output verbatim) are documented once in
[`strategy-plan.md`](strategy-plan.md), shared with every solver. The concrete RFQ input/output types
(`QuoteInput`/`QuoteOutput`, `FillInput`/`FillPlan`, `QuoteCandidate`) live in
`internal/solvers/rfq/strategies/types`.

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
      solverMode: external                              # "external" (default) | "internal" — see below
      adapters:                                         # LiquidLane adapter addresses (whitelist + recovery)
        - "0x…liquidLaneAdapter"                        # vault + collateral resolved on-chain at startup
```

**`solverMode` — the single internal/external knob (default `external`).** The backend discounts API is
available only to **internal Symbiotic** solvers, so one mode flag drives both the discount-API gate and the
adapter whitelist (it replaces the earlier separate `adapterWhitelistEnabled` / `discountsEnabled` flags — a
config still carrying either is rejected at startup so operators migrate):

- **`external`** (default — the open-source filler external parties run): **never touches the discounts
  API** — skips `GET /discounts` in recovery, never calls `POST /discounts` at fill (a surfacing discount
  leg is failed closed). It uses **only its own adapters**, which scope quoting/filling and are
  **required** (no discounts fallback → an empty list is rejected at startup). The quote path is not
  discount-filtered — the backend is trusted to send each solver the right adapters.
- **`internal`**: may call the backend's **internal-only discounts API** (`GET`/`POST /discounts`).
  Configured `adapters` scope quoting when non-empty, but execution is not adapter-restricted so
  discount-driven recovery can use any backend-advertised adapter. Configured adapters remain optional
  extra permissioned recovery inventory.

Both behaviours are **derived from `solverMode` on demand** — no redundant config fields. `Config` exposes
`usesDiscounts()` (`mode == internal`) and `restrictsToAdapters()` (`mode == external && len(adapters) > 0`),
which `buildServices` uses to wire the discount gate (into the execution service: recovery + fill) and the
adapter scoping (into both services). Covered by `discounts_disabled_test.go`, `config_test.go`
(`TestParseConfig_SolverMode`), and `solver_test.go` (`TestBuildServices_WhitelistWiring`).

The signing key is the framework `signer` (the caller EOA); `chain.rpcUrl/chainId` select the network.
The per-quote adapter inventories arrive in the `/quote` request body (`adapters[]`); the `adapters`
list serves two purposes:

- **Adapter whitelist** — scoping is per-path. The **quote** path scopes to the configured `adapters`
  whenever `adapters` is non-empty, in **both** `external` and `internal` mode (`quoteScopesToAdapters`):
  non-configured adapters in a `/quote` request are dropped (none left ⇒ 204), so an `internal`-mode
  filler advertises quotes only for its own adapter universe (e.g. a per-solver adapter). The **execution**
  path scopes to the configured `adapters` only in `external` mode (`restrictsToAdapters`): there backend
  discounts with a non-configured adapter are ignored during recovery. `internal` mode never restricts
  filling — discount-driven recovery may legitimately route through any advertised adapter — so with no
  `adapters` configured an `internal` filler quotes and fills through every advertised adapter.
- **Strategy recovery**: it bounds the candidate adapter universe the post-restart recovery
  multicall scans (recovery's direct inventories are whitelisted by construction).

---

## 4. Build phases

All three phases are committed scope — the goal is full parity with the TS filler, including discount
legs. Phasing is about sequencing and reviewable increments, not dropping features.

0. **(done)** Vendor RFQ ABIs: `Executor`/`Reactor` from `../rfq/out`, and `LiquidLaneAdapter`/
   `UniversalDelegator`/`IVaultV2`/`IERC4626` from a standalone `core-mirror` build →
   `api/bindings/rfq/` + `api/bindings/{delegator,vaultv2,erc4626}`. CGO-free build holds.
1. **(done) Quote path** — `config.go`, bindings, multicall reads (`getAmountOut` batched, decimals
   cached), `strategy` pricing + discount + leg selection (direct legs), Huma HTTP server (`/quote`,
   `/health`, `/openapi.json` + `/docs`, shared-secret auth), in-memory store. Unit-tested (pricing
   golden numbers, config, httptest server).
2. **(done) Execution** — backend client (`/orders`), **poll-only** loop + order state machine
   (`queued→submitting→submitted→{filled|expired|failed}`), reactor-order decode + `Executor.fill`
   (mixed overload, golden selector test) via the shared txmanager (explicit confirmed/unresolved/
   definite-failure reconciliation), attempt tracking,
   and on-chain **strategy recovery via a single multicall** over the configured per-vault adapters
   (adapter views + `marketMaker`/`owner`/`isFiller` authorization filter). Direct legs only.
   Unit-tested (state machine and transaction-outcome matrix with fakes, backend httptest).
3. **(done) Discount legs** — backend `/discounts` (`resolveDiscount` + `listDiscounts`),
   discount-swap encoding (`IReactorDiscountSwapInput` from the resolved signed discount) wired into
   `Executor.fill`, discount-aware strategy selection (legs price off the vault `maxRate`), and
   discount inventories in recovery. Direct + discount fills now match the TS filler. Unit-tested
   (discount-leg selection, discount fill resolves + encodes).
4. **(done) Adapter whitelist** — port of TS filler PR #54: quoting/filling restricted to the
   configured `vaults[].adapter` set (originally `adapterWhitelistEnabled`; now auto-enabled by
   `solverMode: external` when `adapters` is non-empty — see §3),
   recovery discounts filtered by the same set, and a fill-time guard that fails the order when a
   backend-resolved discount's adapter differs from the quoted strategy leg's adapter (no tx is
   sent; a still-open order is re-armed and re-evaluated next poll, matching the TS lifecycle).
   Unit-tested (whitelist build/filter, config flag + zero-address rejection, factory wiring, quote
   200/204 paths incl. disabled toggle, recovery discount filter, mismatch → failed order with no
   tx).

**Reads are multicall-batched** end to end: the quote path issues one `getAmountOut` aggregate3 (with
cached `decimals`), and recovery issues one 3-views-per-adapter aggregate3 (`paused`, `getMaxAssets`,
`getMaxRate`) — each adapter's `vault` and collateral `asset` are resolved once at startup (from
`adapter.vault()` / `vault.asset()`), not re-read per recovery, so there are no per-read round-trips.

---

## 5. Open items / prerequisites

- **Authorized caller of the `Executor`** — the bot EOA must be added to the Executor's `callers`
  allowlist (owner-only `setCallers`) before fills land (onboarding
  prereq, analogous to 3F's offer-signer). Document; do not grant from the bot.
- **Per-environment inputs needed to run**: backend base URL, `Executor` / `Reactor` addresses, the
  LiquidLane adapter address list (`vaults`; adapter whitelist + recovery — each adapter's vault and
  collateral are resolved on-chain at startup; with the whitelist enabled an empty list declines every
  quote), the backend shared secret, and the caller key (last two via env). Hoodi addresses are known
  from the TS deployment manifest; local from the rfq-integration local-stack deploy.
- **RPC**: a primary `chain.rpcUrl` plus optional `chain.rpcFallbackUrls` (HTTP(S), tried in order
  when the primary is unavailable). Fallback is implemented in the generic `internal/chain` layer as a
  barebones viem-style HTTP transport that fails over on transport/5xx/429 errors only (never on a
  JSON-RPC error such as a revert), and the read client inherits it unchanged. Transaction broadcasts
  use a separately dialed, single-endpoint client: `chain.writeRpcUrl` when configured, otherwise the
  primary `chain.rpcUrl`; an ambiguous broadcast failure never traverses read fallbacks. Endpoints are
  operator-configured (no hardcoded public-RPC lists) and duplicates are de-duped. At startup, every
  read endpoint (primary and fallback) plus any distinct write endpoint is preflighted against
  `chain.chainId`; unreachable or wrong-chain endpoints fail startup. Diagnostics identify endpoints
  only by safe origin labels (`scheme://host`), never by userinfo, path, query, or fragment. HTTP(S)
  endpoints, even a lone `rpcUrl`, use the bounded fallback transport; one supported non-HTTP
  endpoint preserves the plain `ethclient` dial.
- **Pricing is a faithful port for now** — the `default` strategy is a faithful port of the TS greedy
  discount + leg selection; a richer quoting strategy is a later follow-up (mirrors the 3F pricing
  TODO), or an operator can plug their own via the `webhook` strategy (see the strategy layer below).
- **Quote latency** — `/quote` is synchronous in the backend's fan-out, so keep it cheap: pricing is
  one `getAmountOut` multicall, and `tokenIn` decimals are read once and cached. A warm quote is a
  single multicall; only the first quote for a not-yet-seen `tokenIn` adds a one-off `decimals` read.
  Keep it that way — don't add per-quote chain reads outside that one multicall.

### Parity with the current TS filler

**Status:** the pricing, ABI encoding, backend endpoints, and recovery read set track the current TS
filler, while the Go service deliberately fails closed at additional trust boundaries. It requires an
exact `orderId` match, binds fill terms to the ABI-decoded signed order, rejects unknown pause state,
validates strategy/order terms, and bounds in-memory cache retention. A few **intentional,
non-fund-moving divergences** remain, by design:

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
  `internalDiscountTransport` (`backend.go`) rewrites the generated `/api/v1/discount(s)` requests to
  `/api-internal/v1/...` at the transport layer; orders pass through unchanged. Covered by the
  `backend_test.go` httptest assertions.
- The `{adapter, tokenToRedeem}` discount-resolve selector exists in TS types but is unused by
  execution (both sides resolve by `discountId`); Go omits it. Cosmetic.

- **Poll-only** — the `/notify` push endpoint was removed from the TS filler; the Go port has no
  `/notify` route, no wake channel, and `source` is always `"poll"`.
- **Backend field names** — the backend's executable order view emits **`protocolSignature`** (not
  `signature`); `backendOrder.ProtocolSignature` decodes that key. Discount payloads use **`adapter`**
  (not `vault`): `discountTerms`/`discountListItem` parse `json:"adapter"`; the on-chain
  `Discount.vault` slot is then filled from that adapter address (positional binding name unchanged).
  A wrong tag here silently zero-fills and breaks every fill, so these are pinned by tests.
- **Discount-recovery filter** — matches TS exactly: keep discounts where the adapter is
  whitelisted, `tokenToRedeem == tokenIn`, and the adapter is not already permissioned; the
  `asset == tokenOut` check is left to the strategy evaluator (no extra collateral pre-filter).
- **Adapter whitelist** — ports TS PR #54: the whitelist is the configured `vaults[].adapter` set
  (the Go config analogue of the TS deployment manifest's `vaults`). It was originally gated by an
  explicit `adapterWhitelistEnabled` flag (the TS `RFQ_FILLER_ADAPTER_WHITELIST_ENABLED` env); that flag
  has since been folded into **`solverMode`** (§3), and scoping is now per-path. The **quote** whitelist
  is enabled whenever `adapters` is non-empty in either mode (`quoteScopesToAdapters`); the **execution**
  whitelist (recovery discount filtering) is `external`-only (`restrictsToAdapters`). Enforcement points:
  `/quote` adapter filtering (none left ⇒ 204) — quote-scoped; recovery discount filtering — execution-scoped;
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
