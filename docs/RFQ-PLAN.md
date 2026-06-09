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
  adapter legs, persists the strategy by `quoteId`, and returns an `amountOut`), `GET /health`, and
  the code-first OpenAPI surface (`/openapi.json`, `/openapi.yaml`, `/docs`). `/quote` is gated by an
  `x-rfq-shared-secret` header (the backend peer). There is **no `/notify` endpoint**.
- **Poller** — every `pollInterval`, `GET /orders?filler=<executor>&orderStatus=open` from the
  backend, then drives each order through `queued → submitting → submitted → {filled|expired|failed}`.
- **Execution** — builds `Executor.fill(Order, protocolSig, Swap[], DiscountSwapInput[], bytes)` and
  sends it; the `Executor` calls the `Reactor`, which calls back into `Executor.execute()` to run the
  adapter `swap`s and satisfy the order's outputs. Each on-chain `Swap`'s `vault` slot is set to the
  leg's **adapter** address.
- **State** — in-memory only: `strategies` (by `quoteId`), `orders` (state machine), `attempts`.

The `/quote` request inventory (`adapters[]`) and the strategy use **adapter/asset** terminology
(`adapter`, `asset`, `assetDecimals`, `maxAssets`, `maxRate`, `discountId`) — a 1:1 match for the TS
`solverQuoteRequestSchema`. Pricing leg types: **direct** (`discountId == null`, public adapter rate)
and **discount** (`discountId != null`, a signature-gated private rate negotiated off-chain via the
backend `/discounts` flow). Both are in scope for full parity — discount legs are built in **P3** (§4),
after the direct path is solid; they are sequenced last, not dropped.

---

## 2. How it maps onto the framework

A new self-contained `internal/solvers/rfq/` implementing `solver.Solver` — no framework edits
(CLAUDE.md modularity rule). The generic layer is reused as-is:

- **`Run(ctx)`** starts the RFQ **HTTP listener** (`/quote` + `/health` + OpenAPI) *and* the poll
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
- **On-chain reads use `chain.Multicall`** (the adapter exposes many per-vault views per quote).
- **Addresses + backend URL come from `solver.config`** (config-is-king); secrets
  (`backendSharedSecret`, the caller key) via `*Env` indirection (`os.Getenv` at point of use).
- **Bindings** for `Executor`/`Reactor` (from the `rfq` build) and `LiquidLaneAdapter`/`UniversalDelegator`/
  `IVaultV2`/`IERC4626` (from a standalone `core-mirror` build) are vendored via `make refresh-abi` +
  `bindings` (two `FORGE_OUT`/`CORE_MIRROR_OUT` sources). The nested `fill`/order ABI is encoded/decoded
  via the generated bindings, never hand-rolled.
- **Signer** — the framework's single EOA is the RFQ **caller** (holds `CALLER_ROLE` on the Executor).

### Component port map (TS → Go)

| TS (`rfq-filler/src/`) | Go (`internal/solvers/rfq/`) |
|---|---|
| `index.ts` + `server.ts` (Hono) | `solver.go` (`Run`: HTTP server + poll loop) + `server.go` (Huma routes/auth) |
| `api.ts` (Zod schemas) | `apitypes.go` (request/response structs + Huma validation tags) |
| `quote.ts` + `strategy.ts` | `quote.go` + `strategy.go` (pricing, discount, leg selection) |
| `execution.ts` | `execution.go` (poll loop, order state machine, fill, recovery) |
| `executor.ts` + `reactor`/`contracts.ts` | `order.go` (encode/decode reactor order, `fill` calldata) |
| `backend.ts` + `discounts.ts` | `backend.go` (thin adapter over the generated `api/rfqbackend` client: `/orders`, `/discounts`) |
| `contracts.ts` + `inventories.ts` | `chainreader.go` (multicall adapter/vault reads) + shared `chain` |
| `domain.ts` | `store.go` types + `strategy.go` types (records, legs, inventories) |
| `config/env.ts` + deployment manifests | `config.go` (typed `solver.config`) |
| `db`/repositories | `store.go` (in-memory strategies/orders/attempts) |
| `metrics.ts` | `metrics.go` (collectors on the shared registry) + framework `internal/observability` (`/metrics` — see §2) |

---

## 3. Configuration (env-agnostic: local / hoodi / mainnet)

One code path; per-environment differences are pure YAML. Sketch of `solver.config` for `rfq`:

```yaml
solver:
  name: rfq-filler
  config:
    backendUrl: https://rfq-backend.example
    backendSharedSecretEnv: RFQ_BACKEND_SHARED_SECRET   # env var NAME (secret never in config)
    listenAddr: ":42073"                                # quote HTTP server (poll-only; no /notify)
    executor:             "0x…"                         # Executor (bot EOA holds CALLER_ROLE)
    reactor:              "0x…"
    pollIntervalMs: 3000
    orderLimit: 20
    vaults:                                             # per-vault LiquidLane adapters (recovery only)
      - { address: "0x…vault", adapter: "0x…liquidLaneAdapter", asset: "0x…collateral" }
```

The signing key is the framework `signer` (the caller EOA); `chain.rpcUrl/chainId` select the network.
The per-quote adapter inventories arrive in the `/quote` request body (`adapters[]`), so no static
list is needed for quoting. The `vaults` list is only consulted for post-restart **strategy recovery**
(it bounds the candidate universe `readPermissionedVaultInventories` scans).

---

## 4. Build phases

All three phases are committed scope — the goal is full parity with the TS filler, including discount
legs. Phasing is about sequencing and reviewable increments, not dropping features.

0. **(done)** Vendor RFQ ABIs from `../rfq/out` → `api/bindings/rfq/` (InstantRedemptionAdapter,
   Executor, Reactor, ICuratorRegistry; IVaultV2 reused). CGO-free build holds.
1. **(done) Quote path** — `config.go`, bindings, multicall reads (`getAmountOut` batched, decimals
   cached), `strategy` pricing + discount + leg selection (direct legs), Huma HTTP server (`/quote`,
   `/health`, `/openapi.json` + `/docs`, shared-secret auth), in-memory store. Unit-tested (pricing
   golden numbers, config, httptest server).
2. **(done) Execution** — backend client (`/orders`), **poll-only** loop + order state machine
   (`queued→submitting→submitted→{filled|expired|failed}`), reactor-order decode + `Executor.fill`
   (mixed overload, golden selector test) via the shared txmanager (revert→failed), attempt tracking,
   and on-chain **strategy recovery via a single `readPermissionedVaultInventories` multicall** over
   the configured vault universe (per-vault adapter views + `marketMaker`/`getCurator` authorization
   filter). Direct legs only. Unit-tested (state machine with fakes, backend httptest).
3. **(done) Discount legs** — backend `/discounts` (`resolveDiscount` + `listDiscounts`),
   discount-swap encoding (`IReactorDiscountSwapInput` from the resolved signed discount) wired into
   `Executor.fill`, discount-aware strategy selection (legs price off the vault `maxRate`), and
   discount inventories in recovery. Direct + discount fills now match the TS filler. Unit-tested
   (discount-leg selection, discount fill resolves + encodes).

**Reads are multicall-batched** end to end: the quote path issues one `getAmountOut` aggregate3 (with
cached `decimals`), and recovery issues one 6-views-per-vault aggregate3 — no per-read round-trips.

---

## 5. Open items / prerequisites

- **`CALLER_ROLE` on the `Executor`** — the bot EOA must be granted it before fills land (onboarding
  prereq, analogous to 3F's offer-signer). Document; do not grant from the bot.
- **Per-environment inputs needed to run**: backend base URL, `Executor` / `InstantRedemptionAdapter`
  / `Reactor` / `CuratorRegistry` addresses, the backend shared secret, and the caller key (last two
  via env). Hoodi addresses are known from the TS deployment manifest; local from the rfq-integration
  local-stack deploy.
- **RPC**: a primary `chain.rpcUrl` plus optional `chain.rpcFallbackUrls` (HTTP(S), tried in order
  when the primary is unavailable). Fallback is implemented in the generic `internal/chain` layer as a
  barebones viem-style HTTP transport that fails over on transport/5xx/429 errors only (never on a
  JSON-RPC error such as a revert), so every read/send path inherits it unchanged. Endpoints are
  operator-configured (no hardcoded public-RPC lists); duplicates are de-duped; all must be the same
  chain. A single `rpcUrl` keeps the plain dial (any scheme).
- _(resolved)_ Recovery now filters direct vaults by executor authorization via
  `readPermissionedVaultInventories` (`marketMaker`/`curatorRegistry.getCurator`
  /`isFiller`). Quote-time inventories come from the backend (already authorized), so this only
  affects post-restart recovery, where an unauthorized vault would surface as a reverting fill.
- **Pricing is a faithful port for now** — same discount + greedy-leg selection as the TS
  `selectBestStrategy`; a richer quoting strategy is a later follow-up (mirrors the 3F pricing TODO).
- **Quote latency** — `/quote` is synchronous in the backend's fan-out, so keep it cheap: pricing is
  one `getAmountOut` multicall, and `tokenIn` decimals are read once and cached. A warm quote is a
  single multicall; only the first quote for a not-yet-seen `tokenIn` adds a one-off `decimals` read.
  Keep it that way — don't add per-quote chain reads outside that one multicall.

### 1:1 parity notes (refactored filler resync)

- **Poll-only** — the `/notify` push endpoint was removed from the TS filler; the Go port has no
  `/notify` route, no wake channel, and `source` is always `"poll"`.
- **Backend field names** — the backend's executable order view emits **`protocolSignature`** (not
  `signature`); `backendOrder.ProtocolSignature` decodes that key. Discount payloads use **`adapter`**
  (not `vault`): `discountTerms`/`discountListItem` parse `json:"adapter"`; the on-chain
  `Discount.vault` slot is then filled from that adapter address (positional binding name unchanged).
  A wrong tag here silently zero-fills and breaks every fill, so these are pinned by tests.
- **Discount-recovery filter** — matches TS exactly: keep discounts where `tokenToRedeem == tokenIn`
  and the adapter is not already permissioned; the `asset == tokenOut` check is left to the strategy
  evaluator (no extra collateral pre-filter).
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
- **Quote discount removed** (mirrors filler `ac65587`): the quoted output is the adapter oracle
  `getAmountOut` directly; `quoteDiscountBps`/`applyQuoteDiscount` are gone.
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
