# vault-solver — UniswapX Quoter + Filler solver (plan)

Adding a **UniswapX RFQ quoter + filler** to `vault-solver` as the **`uniswapx-filler`** solver, following the
framework boundary and conventions in [`../CLAUDE.md`](../CLAUDE.md). We become a UniswapX **market maker**:
we expose the quote webhook Uniswap's RFQ server calls, win exclusivity, then settle the order on Uniswap's
Reactor — sourcing liquidity from our Symbiotic vault `LiquidLaneAdapter`s (vaults-first; a secondary-DEX hop
is a later step).

---

## 1. What the solver does

UniswapX RFQ ("Exclusive Dutch Auction") has the same broad shape as our own Symbiotic RFQ — signed orders,
a settlement Reactor, off-chain quoters/fillers — but owns a different wire contract, order codec, lifecycle,
and strategy boundary. The two solvers reuse neutral LiquidLane primitives and framework services, not one
shared strategy facade.

- **Quote webhook (the ≤500ms hot path).** Uniswap's RFQ server makes a synchronous HTTP `POST` to a webhook
  URL we register with them. The UniswapX strategy prices direct LiquidLane routes from `getAmountOut` and
  can select advertised signed-discount candidates, then applies its price buffer and optional gas floor. We respond
  `200` with `amountOut` and our `filler` = the on-chain `LiquidLaneUniswapXExecutor` address, or decline with
  an empty `204`. Exact-input and exact-output requests are supported for quote protocols `v1` and
  `v2`. That wire field is not used to infer indicative versus hard phase: Uniswap intentionally hides
  the phase from quoters.
- **Order ingestion.** If our quote wins, Uniswap's per-order cosigner finalizes the auction terms and
  we pick it up by **polling `GET /orders`** (≤6 RPS, Uniswap's stated filler rate limit), dedup by
  `orderHash`. The generated client follows upstream spec version 2.0.0 and its typed `DutchV2OrderEntity`.
  Uniswap's `order-notification` push webhooks are **deprecated for new integrations** (Filler FAQ), so
  polling is the only implemented delivery channel (§4.2).
- **Settlement.** We call **`executeWithCallback`** on Uniswap's V2 Dutch Order Reactor; our
  `LiquidLaneUniswapXExecutor` implements `IReactorCallback.reactorCallback`, sources the output token from
  one or more `LiquidLaneAdapter`s, and approves it back to the Reactor. Inputs are pulled from the swapper
  via Permit2. The current prototype supports typed direct `swap` and signed `discountSwap` routes, plus
  multiple same-token outputs (including fee outputs). A decaying exact-output order may resolve to more
  input in the execution block than the solver planned against; the routes consume their planned input and
  the executor retains the positive difference as filler surplus.
- **Safety.** Fail-closed pre-fill validation gates, quote-state epochs, post-fill snapshot refresh, and a
  fade-aware circuit breaker (Uniswap penalizes win-but-don't-fill — see §4, §6).
- **State** — in-memory only: an immutable refreshed inventory and optional gas snapshot, its epoch, pending-fill capacity
  reservations, exclusive-obligation reconciliation state, order dedup/retry state, and breaker timestamps;
  TTL-swept.

**Scope decisions (locked):**

| Decision | Choice | Rationale |
|---|---|---|
| Our role | **Quoter + Filler (market maker)** | We provide liquidity for our own vault assets on UniswapX. |
| Liquidity source | **Symbiotic vaults first, including signed private discounts**; secondary-DEX hop later | Reuse LiquidLane direct and discount liquidity; widen pairs later. |
| Order version | **V2 first (mainnet)** | "Goal is mainnet" (V2). Add a codec boundary when a second real order version is implemented. |
| Pricing v1 | **Redemption rate − fixed haircut**, optional gas-aware floor | Ship fast, tune later (matches how `3f`/`rfq` shipped). |
| On-chain executor | **UniswapX-specific** `LiquidLaneUniswapXExecutor.sol` | Smallest, auditable surface; no multi-venue abstraction yet. |
| Code organization | **UniswapX-local `default` + `webhook` strategies** | The solver owns its protocol-specific decision contract; neutral LiquidLane facts/math and webhook transport stay shared (§2.1). |

**Directionality (structural constraint):** `LiquidLaneAdapter`s are one-way — they consume a
token-to-redeem and pay out the vault asset. So the only fillable orders are **RWA-in → vault-asset-out**:
`tokenIn` must be a token-to-redeem on a configured direct route or a valid advertised signed-discount route,
and `tokenOut` must be that adapter's ERC-20 vault asset. Native-ETH output is outside the initial route model
and is declined in v1; §7 records an optional WETH-unwrapping extension. Everything else — including the
reverse-direction opposing probe (§4.1) — auto-declines.

**Out of scope (v1):** legacy V1 limit orders, V3/Tempo order type, mixed-token or native-token outputs,
secondary-DEX sourcing, self-funding, a competitive
exclusivity-override pricing controller, and quoting any pair our vaults can't settle. Exact on-chain
`exclusivityOverrideBps` is still applied when evaluating a public order during another filler's window.

---

## 2. How it maps onto the framework

A self-contained `internal/solvers/uniswapx/` implements `solver.Solver`; protocol behavior stays out of the
generic framework (CLAUDE.md modularity rule). Shared lifecycle hardening in `internal/{chain,txmanager,solver}`
and `cmd/` remains protocol-neutral. Code organization follows the repo's **solver-local strategy
architecture** (see `docs/strategy-plan.md`) and the shared LiquidLane read/type conventions
(`docs/LIQUIDLANE-CONVENTIONS.md`). §2.5 is the consolidated reuse-vs-delta implementation checklist.

### 2.1 Solver-local strategy, shared LiquidLane primitives

UniswapX owns its strategy contract and registry under `internal/solvers/uniswapx/strategies/`:

```
internal/solvers/uniswapx/
  strategy.go
  strategies/
    registry.go
    types/types.go       # DecideQuote / DecideFill
    default/
    webhook/
```

The solver owns order decoding, Dutch amount resolution, quote serving, pending-fill reservations, chain
snapshots, exclusive-obligation reconciliation, preflight, and construction of the transaction request. The
shared txmanager owns the signed lifecycle. For each RFQ request, the strategy receives its concrete amount
plus the latest inventory and optional gas snapshot and returns one `amountIn`/`amountOut` pair. At fill time
it receives a fresh chain snapshot and returns an immediately executable LiquidLane route plan. A candidate
may carry a `DiscountID`; offer discovery and fill-time resolution of signed terms stay solver-owned.

Only proven neutral packages are shared: `internal/liquidlane` for route/inventory types, capacity IDs,
fixed-point math, readers, and signed-discount client/types; `internal/liquidlane/snapshot` for the
common direct/physical inventory, amount-specific fill, and optional gas snapshot read path;
`internal/liquidlane/strategies/greedy` for normalized `QuoteTask`/`FillTask` solving, capacity accounting,
minimum-output distribution, and gas conversion; `internal/liquidlane/strategies` for canonical fill
routes, pending-capacity reservations, gas pricing, and webhook-plan validation; and
`internal/webhook` for bounded remote-decision transport. The RFQ solver normalizes its protocol facts
to shared `QuoteCandidate`s; UniswapX and LI.FI strategies start from shared `Inventory`. Their default
strategies feed the same quote engine. RFQ, LI.FI, and UniswapX pass fresh amount-specific fill
candidates to the same fill engine, then adapt its result to RFQ Executor, OIF, or Reactor lifecycle.
Public strategy contracts, registries, config decoding, webhook payloads,
protocol output/deadline handling, and calldata remain solver-local.
The shared discounts package also owns offer-to-route matching, advertised fill-quote derivation, and
fresh selected-term validation. UniswapX owns only request timing, token policy, resolve-selected
orchestration, and mapping validated terms into its executor ABI. Its chain reader accepts explicit
executor, caller, and route facts rather than the protocol config. Reactor binding remains a deployment
assertion because the PR19 ABI has no getter.

- **`EXACT_OUTPUT`:** the strategy solves the concrete requested output against current capacity, price
  buffer, and gas, and returns the required input. There is no published range or cached quote route.
- **Candidate construction:** UniswapX receives no inventory in the quote request, so the solver refreshes
  configured direct inventory and `getAmountOut` values in the background. In internal mode it also resolves
  advertised `(adapter, tokenIn)` routes on-chain; configured adapters scope quotes when present, while an
  empty list produces discount-only quotes from all valid advertisements. Direct and signed-discount
  candidates for one physical route share the same capacity domain.

### 2.2 Reuse of the generic layer

- **`Run(ctx)`** binds the UniswapX quote listener before starting the `GET /orders` and fill loops. A later
  server failure is reported through `ReportFatal`, which cancels the process runtime and drops global
  readiness so the shared txmanager starts shutdown immediately. Cancellation stops new fill admission;
  queued orders are released without signing, while accepted transaction results drain within the configured
  shutdown budget before `Run` returns.
  The framework observability server (`:9090`) stays separate.
- **The quote server is a bounded strict-JSON stdlib handler.** The public quote schema is not available in
  the order-service OpenAPI and remains a hand-vendored, tested boundary (§4.1, §4.3).
- **`/metrics`** is the framework's shared registry; the solver registers bounded quote, poll, fill,
  readiness, and breaker collectors via `deps.Metrics.Registerer()`.
- **Fills go through the shared `txmanager` asynchronously** (CLAUDE: solvers never send directly). It keeps
  at most one unresolved signed lifecycle; later fills wait outside admission and signing, so the process
  cannot create a future transaction queued behind a missing lower nonce. `CancelAt` is the earliest order,
  signed-discount, or protocol-signature deadline. It is derived from chain time, translated to a wall-clock
  deadline without extending the remaining validity, and also bounds the pre-sign wait. The wall-clock
  observation anchor is captured before the chain-time RPC so lookup and planning latency consume, rather
  than extend, the remaining validity. The active lifecycle is replaced by a same-nonce cancellation on
  expiry or, when nonce ownership is not conflicted, shutdown, and drained to a terminal result. Shutdown
  stops waiting after `shutdownTimeoutMs`, cancels outstanding RPC work, and returns a deadline error so
  SIGTERM cannot hang indefinitely; the deployment grace period must be longer than that bound.
- **Fees remain dynamic within explicit ceilings.** A positive `tipGwei` is the mandatory priority-fee
  floor. A higher node suggestion is advisory and is clamped to available fee-cap headroom; zero uses the
  median p75 priority reward from the latest five blocks, also clamped to headroom, and fails new submissions
  closed when `eth_feeHistory` is unavailable or invalid. Replacements use the greater of fresh fees and a
  12.5% bump; when a replacement fee read is unavailable, the cached fees are bumped instead. `maxFeeGwei`
  is the absolute global ceiling and normal sends reserve
  cancellation headroom below it. With gas accounting disabled, UniswapX supplies no request ceiling. With
  gas accounting enabled, `MaxFeePerGas` returns the profitability ceiling including one normal replacement,
  and the initial send reserves that replacement inside the ceiling. Cancellation may exceed the request
  ceiling but never the global ceiling. Startup rejects a configured positive tip floor that cannot fit
  beneath the initial cap after both reserved bumps.
- **Signed attempts are retained by exact hash.** An ambiguous send is never treated as definitely absent or
  re-signed at another nonce. A consumed/colliding nonce is reconciled against every exact attempt. During a
  replacement of an already tracked lifecycle, a receipt proven canonical against a stable head resolves
  ownership immediately, so readiness can resume while the single lifecycle still holds admission through
  its configured confirmation depth. Initial-broadcast collisions, and replacements without an owned
  canonical receipt, keep further sends, quotes, and readiness fail-closed until terminal reconciliation or
  operator action; a later receipt reorg restores the pause. Each confirmation check proves that the receipt
  block is in the stable head's
  ancestry by following hash-addressed parent headers, so correctness does not depend on endpoint affinity
  or a load balancer serving one fork. Unavailable or incoherent snapshots are retried through the normal
  read fallbacks. Startup likewise rejects any write-endpoint latest/pending nonce mismatch before readiness.
  Exact-attempt ownership is in-memory: the packaged Compose deployment restarts automatically after an
  unclean exit even though nonce equality cannot prove that a private hidden attempt is gone. If such an
  attempt later consumes the reused nonce, admission and readiness remain fail-closed for operator
  reconciliation; automatic restart does not reconstruct ownership.
- **Pending capacity stays reserved through transaction completion**, then remains unavailable to quotes
  until a fresh post-fill snapshot is published.
- **On-chain reads use `chain.Multicall`** through the solver's LiquidLane reader; the strategy receives
  validated inventory plus gas snapshots and current fee inputs only when gas accounting is configured.
- **Addresses + URLs come from `solver.config`**; secrets (`UNISWAP_API_KEY`, the solver key) via `*Env`
  indirection (`os.Getenv` at point of use).
- **Signer** — the framework's single EOA is the UniswapX **solver** (holds the role on `LiquidLaneUniswapXExecutor`,
  submits `execute`). We do **not** cosign or sign orders in production (that's Uniswap Labs + the swapper);
  our EIP-712 code is verification + test-only signing.

### 2.3 Component map (file → responsibility)

| Go (`internal/solvers/uniswapx/`) | Responsibility | Reuse vs net-new |
|---|---|---|
| `solver.go` | factory, dependency wiring, startup validation, and `Run` lifecycle | mirror `rfq` |
| `config.go` | typed config: addresses, servers, optional gas feeds, breaker, adapters/token policy, strategy | mirror `rfq` |
| `server.go` / `apitypes.go` / `middleware.go` | bounded quote webhook (`POST /quote`), `/health`, `/healthz`, `/ready`; source-IP auth stays at ingress | net-new |
| `quote_refresh.go` | background inventory and optional gas snapshots, epoch binding, and atomic publication | net-new |
| `polling.go` | exclusive and public V2 polling; dedup/retry admission and exclusive reconciliation | net-new |
| `execution.go` | fill planning, discount resolution, executor calldata, preflight, async submission, and completion | mirror `rfq` + net-new |
| `chainreader.go` | config-independent executor/route checks plus refreshed inventory/rate and optional gas snapshots | reader port |
| `strategies/` | UniswapX-local contract, registry, `default`, and `webhook` decisions (§2.1) | net-new |
| `order.go` | V2 Dutch codec, hashes, signature/exclusivity validation | net-new |
| `orderclient.go` | generated-client adapter, authenticated polling, one ≤6 RPS limiter, pagination/body bounds | net-new |
| `state.go` / `health.go` / `metrics.go` | reservations, quote epochs, exclusive obligations, dedup/backoff/breakers, readiness and metrics | net-new |

**On-chain:** the RFQ contracts repository owns `LiquidLaneUniswapXExecutor.sol`, its interfaces, and
contract tests. `rfq-integration` consumes a pinned RFQ contracts revision, while this solver vendors only
the executor ABI and generated binding under `api/bindings/uniswapx/` — see §7/P3.

### 2.4 Configuration (`solver.config`)

One code path; per-environment differences are pure YAML (CLAUDE.md "config is king"). Secrets via `*Env`
indirection (read with `os.Getenv` at point of use, never stored in the parsed config). `chain` / `signer` /
`txManager` / `observability` use the same shared framework schema as the `rfq` profile, with
UniswapX-specific timing and fee values. The current profile is
[`config/uniswapx.example.yaml`](../config/uniswapx.example.yaml); the abbreviated shape is:

```yaml
chain: { rpcUrl: "${ETH_RPC_URL_MAINNET}", writeRpcUrl: "${WRITE_RPC_URL}", chainId: 1 }
txManager:
  confirmations: 2
  maxFeeGwei: 50
  tipGwei: 0
  replacementIntervalMs: 5000
  pendingTimeoutMs: 300000
  shutdownTimeoutMs: 60000

solvers:
  - name: uniswapx-filler
    config:
      reactor:  "0x00000011F84B9aa48e5f8aA8B9897600006289Be"  # V2 Dutch Order Reactor (per chain)
      executor: "0x…LiquidLaneUniswapXExecutor"
      adapters: ["0x…liquidLaneAdapter"]
      solverMode: internal
      tokensToQuote: permissioned
      permissionedTokens: ["0x…tokenToRedeem"]
      quoteServer: { listenAddress: ":42080", refreshInterval: 12s, quoteTtl: 30s }
      orderServer:
        baseUrl: "https://api.uniswap.org/v2"
        apiKeyEnv: UNISWAPX_ORDER_API_KEY
        pollInterval: 1s
        sources: { exclusiveV2: true, publicV2: true }
      discounts: { baseUrl: "https://rfq.example", httpTimeout: 2s, minimumValidity: 15s }
      gas:
        nativeUsdFeed: "0x…"
        nativeMaxAge: 1h
        tokenUsdFeeds: [{ token: "0x…asset", feed: "0x…", maxAge: 1h }]
      breaker: { maxFailures: 3, window: 5m }
      strategy: { name: default, config: { priceBufferBps: 20 } }
```

The `gas:` block is optional. When omitted, quote and fill decisions do not subtract gas and the solver
skips gas-state and Chainlink reads. Transaction submission remains dynamically priced, but the first fee
quote is not reused as a hard replacement ceiling, so the solver pays the cost without passing it through to
the quote. With `tipGwei: 0`, recent fee-history rewards avoid relying on a potentially unusable node tip
suggestion. Suggestions and rewards are advisory and are clamped to available headroom; a positive value
remains the mandatory operator-controlled floor and fallback. `maxFeeGwei` remains the absolute ceiling
described in §2.2.

The configured write RPC is chain-ID checked at startup. Signed broadcasts plus both mined and pending
account-nonce reads are pinned to that endpoint; fee, receipt, and other state reads use the primary/read
fallbacks. A signed broadcast is never replayed across those endpoints. Startup requires the write endpoint's
mined and pending nonces to match. Because standard nonce methods cannot reveal a future transaction queued
beyond a nonce gap, the signer EOA must remain exclusive to this process.

Startup scans the executor's indexed `callers(uint256)` entries for the framework signer and checks
executor bytecode. In external mode it also requires every configured adapter to authorize the executor as a
direct filler. The PR19 ABI has no `isCaller` helper or Reactor getter, so the configured Reactor must still
be matched to the implementation's immutable during deployment.
`solverMode: external` (default) forbids `discounts`, requires a non-empty `adapters` list, and requires every
configured adapter to authorize the executor as a direct filler. `solverMode: internal` requires `discounts`
and makes `adapters` optional.
When the list is non-empty it scopes quote candidates and direct fills; fill-time signed-discount recovery
remains unrestricted, exactly as in RFQ. Without configured adapters the solver operates discount-only.
Every advertised route is resolved on-chain and accepted only when its asset/decimals, current capacity and
rate, minimum discount, token policy, and—when gas accounting is enabled—configured gas feed are valid. Direct candidates are always
restricted to configured adapter addresses, so a dynamically discovered signed route cannot silently become
a direct route. If the discounts API is unavailable, internal mode continues with configured direct routes;
without configured adapters it publishes an empty quote state until the API recovers.

### 2.5 Implementation delta vs `rfq` — what reuses, what changes, what's manual

The working checklist for the build: everything below is either lifted from the shipped `rfq` solver,
deliberately different from it, or an operational step `rfq` never needed. (Re-verified against the local
RFQ and UniswapX code on 2026-07-20.)

**Reused as-is:**

- `internal/liquidlane/` — shared LiquidLane route/capacity types, fixed-point math, readers, gas helpers,
  and signed-discount client/types.
- `internal/webhook/` — the neutral remote-decider transport client, verbatim (backs the optional
  `webhook` strategy).
- A thin UniswapX reader composes those shared readers for startup route resolution, authorization,
  inventory/rate snapshots, optional gas snapshots, and fill-time quotes (§2.1).
- Solver scaffolding patterns: `init()` registration + factory, solver-local strategy selection through
  `strategy: {name, config}`, bounded quote server, poll loop, and calldata-only submission through the
  shared `txmanager`.

**Done differently from `rfq` (the real implementation work):**

| # | Area | `rfq` does | `uniswapx` must do |
|---|---|---|---|
| 1 | Quote-time inventory | Backend sends `adapters[]` (maxAssets/maxRate/decimals) in the `/quote` body; on-chain inventory read is recovery-only | **Self-source on-chain** over configured direct adapters plus internal advertised discount routes. Price from the background-refreshed snapshot (≤500ms) — §2.1 |
| 2 | Quote wire contract | Backend schema, `x-rfq-shared-secret`, 204 decline, 422 on schema violation | UniswapX quote schema, **`204` decline**, `requestId` echo, independent opposing-probe handling; published source IPs are enforced at ingress, not through an invented application header — §4.1/§10.1 |
| 3 | Quoted price policy | Quotes the raw oracle `getAmountOut` (no margin) | UniswapX-local strategy applies the configured price buffer and optional gas-aware floor; below an enabled floor ⇒ decline — §2.1, §5 |
| 4 | `EXACT_OUTPUT` | Hard-rejected at validation | UniswapX prices the concrete requested output with current capacity and optional gas, returning the required input — §2.1, §5 |
| 5 | Order ingestion | Polls own backend `GET /orders`, decodes the Symbiotic Reactor order from the backend payload | Polls Uniswap `GET /orders?filler=<executor>` (≤6 RPS); **net-new V2 Dutch order codec + Permit2 witness EIP-712 + cosignature recovery** (`order.go`, the riskiest unit — P2) — §3.3, §4.2 |
| 6 | Pre-fill validation | Order-deadline + strategy↔order binding checks | Those **plus**: cosignature recovers to the swapper-authorized per-order `cosigner`, `exclusiveFiller == our executor`, decay window still fillable, resolved output at current block ≥ quoted floor — §6 |
| 7 | Settlement call | `Executor.fill(order, protocolSig, swaps[], discountSwaps[], executorData)` on our Reactor | `LiquidLaneUniswapXExecutor.execute(SignedOrder, FillCall)` → Uniswap reactor `executeWithCallback` → callback routes through direct `swap` or signed `discountSwap`; native output is declined in v1 — §7 |
| 8 | Failure economics | Failed fill ⇒ order re-armed next poll; no external penalty | **Fade penalty regime**: fail-closed gates before gas, local breaker, honor `blockUntilTimestamp`, quote only what inventory certainly fills — §6 |
| 9 | Private discounts | Offer discovery, fill-time signed resolution, and `discountSwap` calldata | Reuse shared discovery, route matching, candidate construction, and fresh-term validation; keep resolve timing and executor calldata inside UniswapX; implemented across P3/P5 |

**Manual / operational (no `rfq` analogue — `rfq` only needed a shared secret with our own backend):**

- Uniswap onboarding: intake form, `UNISWAP_API_KEY`, register the quote URL + filler address +
  chainIds (S3-provisioned by Uniswap), allowlist their RFQ source IPs — §10.1–10.2.
- Beta qualification: 5 exclusive fills with real funds, tx hashes emailed for manual promotion — §10.4.
- On-chain: land and deploy `LiquidLaneUniswapXExecutor`, configure the tx-sending EOA as a caller, and get
  the executor authorized as
  `isFiller`/`marketMaker` on each sourced adapter, confirm adapter `ALLOCATE_ROLE` — §10.3.
- Vendoring: `LiquidLaneUniswapXExecutor` ABI → `api/bindings/uniswapx/`, `uniswapx-service`
  `swagger.json` → generated poll client, hand-vendored quote-webhook structs — §4.3, P0.

---

## 3. UniswapX protocol reference (verified ground truth)

Collected and source-verified during planning; **re-verified 2026-07-20** against the UniswapX repo,
the uniswapx-sdk `constants.ts`, and developers.uniswap.org. Treat as the contract-of-record; re-verify
addresses against the live deployments page before going live.

### 3.1 Auction model & order versions per chain

UniswapX RFQ uses the **Exclusive Dutch Auction**: the winning quoter's `filler` address is set as
`exclusiveFiller` and may fill during a short exclusivity window before the order decays open to permissionless
fillers. A non-exclusive filler can override exclusivity only by paying the swapper extra
(`exclusivityOverrideBps`); the swapper is never worse off. When `exclusivityOverrideBps == 0` (strict
exclusivity, `ExclusivityLib` reverts `NoExclusiveOverride`) no override is possible at all; Uniswap's
hard-quote cosigner sets a nonzero default.

Mainnet RFQ is now branded **"UniswapX RFQ V2"** — an *off-chain* redesign (indicative quotes pre-signature
vs **hard quotes** post-signature, with hard-quoters "held fully accountable"). On-chain settlement is
unchanged: it still runs the `V2DutchOrderReactor`. Consequence for us: fade discipline (§6) is
program-critical, not just polite.

| Chain | Order type | Decay | Quoter-relevant? |
|---|---|---|---|
| **Ethereum mainnet (chainId 1)** | **V2** Dutch | time-based (`decayStartTime`/`decayEndTime`); exclusivity ~24 s (2 blocks) | **Yes — our first target** |
| Tempo, Base, Arbitrum, Avalanche, BNB, Unichain, Robinhood Chain | **V3** Dutch | block-based, nonlinear (`decayStartBlock`, `relativeBlocks[]`/`relativeAmounts[]`); exclusivity ~2–4 s | Yes (later) |

- Exclusivity window on mainnet is **"currently about 24 seconds (2 blocks)"** — long enough that ≤1 s
  polling (§4.2) comfortably fits inside it.
- The chain matrix keeps expanding — **confirm the live matrix with Uniswap** (§10). A V3 reactor is also deployed on mainnet but docs state mainnet RFQ does
  not route to it today.

### 3.2 Deployed addresses (mainnet, chainId 1 — verify before use)

| Contract | Address |
|---|---|
| V2 Dutch Order Reactor | `0x00000011F84B9aa48e5f8aA8B9897600006289Be` |
| V3 Dutch Order Reactor | `0x0000000015757c461808EA25Eb309638B62681cf` |
| OrderQuoter | `0xc6ef4C96Ee89e48Eff1C35545DBEED4Ad8dAC9D4` |
| Permit2 (all chains except zkSync Era — out of scope) | `0x000000000022D473030F116dDEE9F6B43aC78BA3` |
| Arbitrum V3 Reactor | `0xB274d5F4b833b61B340b654d600A864fB604a87c` |
| Base DutchV3 Reactor | `0x000000008a8330B5d1F43A62Bf4C673A49f27ba0` |

Reactor constructor (V2 and V3): `constructor(IPermit2 _permit2, address _protocolFeeOwner)`.

**Testnet reactors (from the SDK `REACTOR_ADDRESS_MAPPING`, NOT the docs deployments page — the docs omit
testnets).** These exist on-chain and are usable for settlement/fill testing (§8); there is **no RFQ *server***
on these chains, so the quote/order-delivery half can't be driven by Uniswap there.

| Chain | Order type | Reactor address |
|---|---|---|
| **Sepolia (11155111)** | **Dutch V2** | `0x0e22B6638161A89533940Db590E67A52474bEBcd` |
| Unichain Sepolia (1301) | Hybrid (v4) | `0x000000000C75276D956cc35218ca8f132D877957` |
| **Tempo (4217)** | Dutch V3 | `0x00000000fc1E66C9f582566EAd00108e55F1c0C6` (RPC `https://rpc.tempo.xyz`) |

Source of truth for deployed addresses is the SDK `sdks/uniswapx-sdk/src/constants.ts` `REACTOR_ADDRESS_MAPPING`
(Permit2 canonical on all incl. Sepolia). The public `uniswapx-tool` supports production-chain quote/order
flows; `Env.Beta` and `Env.Prod` both use the normal gateway with an environment flag. Testnets are reachable
only for direct on-chain settlement.

### 3.3 V2 Dutch order struct & cosignature

```
SignedOrder { bytes order; bytes sig }     // order = ABI-encoded V2DutchOrder; sig = swapper Permit2 signature

V2DutchOrder {
  OrderInfo{ reactor, swapper, nonce, deadline, additionalValidationContract, additionalValidationData }
  address cosigner                          // swapper-authorized per-order field; no global config/setter
  DutchInput  baseInput                     // token, startAmount, endAmount
  DutchOutput[] baseOutputs                 // token, startAmount, endAmount, recipient
  CosignerData{ decayStartTime, decayEndTime, exclusiveFiller, exclusivityOverrideBps, inputAmount, outputAmounts[] }
  bytes cosignature
}
```

**Cosignature verification (verbatim from `V2DutchOrderReactor._validateOrder`):**
```solidity
address signer = ecrecover(keccak256(abi.encodePacked(orderHash, abi.encode(order.cosignerData))), v, r, s);
if (order.cosigner != signer || signer == address(0)) revert InvalidCosignature();
```
Digest = `keccak256(orderHash ‖ abi.encode(cosignerData))`, signed **raw** (no EIP-191 prefix). Because
`cosigner` is a per-order field with no on-chain setter, **we can self-cosign in tests** with any key we hold —
this is what makes self-driven local E2E possible (§8).

The solver does **not** pin a cosigner address in YAML. The swapper's Permit2 witness signature commits to
`order.cosigner`, and the Reactor requires `cosignature` to recover to that address. Keeping a second static
allowlist in solver config would add no protocol validation and would fail closed on legitimate key rotation.

### 3.4 Settlement interfaces (Uniswap's reactor)

```solidity
function executeWithCallback(SignedOrder calldata order, bytes calldata callbackData) external payable;
interface IReactorCallback { function reactorCallback(ResolvedOrder[] memory, bytes memory) external; }
```
Flow: reactor validates order + swapper Permit2 sig → transfers input to our executor →
`reactorCallback(resolvedOrders, callbackData)` → we source the output + approve it to the reactor → reactor
delivers output to the swapper and verifies amounts.

---

## 4. API schemas & event/delivery model

**There is no websocket/stream.** Inbound traffic is exactly one surface — the **quote webhook we register
with Uniswap** (their config is S3-backed; we do not self-serve registration). Won orders are fetched by
**polling** (`GET /orders`); Uniswap's order-delivery webhooks are **deprecated for new integrations**
(§4.2). Schemas live in two repos: `uniswapx-parameterization-api` (quote webhook, **Joi**, no OpenAPI)
and `uniswapx-service` (order pool, Joi + an OpenAPI `swagger.json`).

### 4.1 Quote webhook — Uniswap → us (synchronous `POST`, ≤500ms)

- **Method/timeout:** `axios.post`, `application/json`; the current public requirement is **500ms on
  Ethereum** and **250ms on other chains**. This implementation targets Ethereum and configures a 450ms
  server timeout.
- **Two POSTs per request, in parallel:** the real quote **plus an "opposing" probe** (inverted `type`,
  swapped `tokenIn`/`tokenOut`) for price discovery. Since June 2026 (parameterization-api #456,
  "obfuscate two-sided RFQ quote") each of the two carries a **distinct fresh `requestId`** and they are
  sent in randomized order — the pair is **indistinguishable and uncorrelatable by design**. So: no
  probe-pairing logic anywhere; price every request independently and honestly. For us the probe's
  reverse direction (vault-asset → RWA) is structurally unfillable and auto-declines (§1 directionality).
- **Auth:** the public quote schema specifies no signed/header scheme. The FAQ publishes source IPs to
  allowlist (Beta `3.135.148.114`, Prod `3.138.88.28`); confirm any additional shared header during
  onboarding. Until then, authenticate at the ingress by source IP rather than inventing a required header.
- **Decline:** empty **`204 No Content`**, as required by the current Become a Quoter guide and FAQ. Never
  use `404`, which is an error rather than a normal non-quote.
- **Response must echo the (obfuscated) `requestId` received** or it's dropped (`RFQ_FAIL_REQUEST_MATCH`).
- **Breaker notification:** the same endpoint receives `{blockUntilTimestamp}` without a normal quote
  `requestId`; a trusted notification updates the quote breaker and zero clears it.

**Request body** (`PostQuoteRequestBodyJoi`, `QuoteRequest.toCleanJSON()`):
```jsonc
{
  "tokenInChainId": number,   // required
  "tokenOutChainId": number,  // required, MUST equal tokenInChainId (same-chain only)
  "requestId": string,        // required
  "tokenIn": string,          // required ERC20 (native ETH = 0x000...000)
  "tokenOut": string,         // required ERC20
  "amount": string,           // required base-unit integer string
  "swapper": string,          // 0x000...000 at quote time — swapper is hidden; price on pair+amount only
  "type": "EXACT_INPUT" | "EXACT_OUTPUT",
  "numOutputs": number,       // required >= 1
  "protocol": "v1" | "v2", // protocol version, not an indicative/hard phase signal
  "quoteId": string           // generated per wire request by Uniswap's WebhookQuoter
}
```

The inbound `PostQuoteRequestBodyJoi` used before fan-out omits `quoteId`, but `WebhookQuoter` assigns a
separate UUID to the real and opposing clean requests before posting them to quoter endpoints. The public
Become a Quoter guide documents the same wire field. The solver requires and echoes it for protocol
conformance and correlation, but does not treat it as a capacity lock. A captured Beta payload is still
required to verify the complete operational envelope and auth, not to resolve whether `quoteId` is sent.
The quote-time `swapper` may be the zero address, so quote admission must not require the final swapper.

**Response body** (`RfqResponseJoi`):
```jsonc
{
  "chainId": number, "requestId": string,        // echo requestId
  "tokenIn": string, "amountIn": string,
  "tokenOut": string, "amountOut": string,
  "filler": string,                              // our LiquidLaneUniswapXExecutor address
  "quoteId": string
}
```

### 4.2 Won/cosigned order delivery — us ← Uniswap (POLL-ONLY)

**Order webhooks are deprecated.** Per Uniswap's Filler FAQ, `order-notification` webhooks "were deprecated
on UniswapX due to degraded performance" and **new webhook integrations are no longer onboarded** —
"fillers should start with polling for orders and rate limit at 6 RPS". The implementation is poll-only;
no push abstraction is carried before a second real delivery source exists.

**POLL — `GET https://api.uniswap.org/v2/orders`** (mainnet), **≤6 RPS**. The exact Beta polling
transport remains an onboarding confirmation item (§10.4):
- Query: `orderStatus=open&filler=<us>&chainId=<id>` (+ `limit, cursor, sortKey=createdAt, sort, desc,
  orderHash(es), swapper, pair`). `orderStatus ∈ {open, expired, error, cancelled, filled, insufficient-funds}`.
- Response: `{ orders: OrderEntity[], cursor? }`; under upstream spec version 2.0.0, a Dutch V2 variant is
  the typed `DutchV2OrderEntity` with `encodedOrder`, `signature`, nested
  `cosignerData{decayStartTime, decayEndTime, exclusiveFiller, inputOverride, outputOverrides[]}`,
  `cosignature`, `createdAt`, `input`, `outputs[]`, `orderHash`, `chainId`, `swapper`, optional `txHash`,
  `quoteId`, and `requestId`. The `encodedOrder` + swapper `signature` *is* our
  `SignedOrder{order, sig}` — directly fillable.

**Ingestion design:** poll at 500–1000ms, inside the 6 RPS budget. Exclusive V2 then public V2 are fetched
independently under one limiter; each source has bounded pagination. Dedup by `orderHash`. Both sources use
the configured V2 Reactor/Executor pair and `GET /orders`; there is no legacy `/limit-orders` runtime path.
After each successful exclusive poll, tracked obligations past `decayStartTime` are reconciled by hash in
bounded batches through the same endpoint. Only a canonical successful fill whose block time is at or before
the deadline discharges the obligation. This includes another filler's soft override. A later fill by any
filler, including our executor, or any final non-filled state for an obligation observed live or recovered
after a runtime poll gap is a local fade. An already-terminal miss discovered only by initial startup history
reconciliation is logged and terminalized without opening a fresh local breaker. An order that is still
`open`, missing/unknown API data, or an unreadable receipt makes exclusive state unknown and blocks quotes
without opening the breaker; the next successful reconciliation retries it.

**Hard-quote phase** is run and cosigned by Uniswap; **we do not host it**. The same webhook receives both
indicative and hard RFQs, and the quoter cannot distinguish them. A fresh `quoteId` is generated for each
RFQ call, so reserving every response would make the first indicative round consume capacity and can cause
the later hard round for the same user intent to self-decline. The simple quoter is therefore stateless:
each request is priced from the latest snapshot, and only a fill transaction accepted for submission creates
a pending capacity reservation. The finalized cosigned order is authoritative; it is decoded, signature-
checked, repriced from current chain state, and simulated immediately before submission.

### 4.3 Vendoring plan (matches CLAUDE.md codegen discipline)

- **Poll client:** vendor upstream `uniswapx-service/swagger.json` at spec version 2.0.0 and generate
  `api/uniswapxservice` with the pinned Java openapi-generator. `DutchV2OrderEntity` and its nested fields
  come directly from that vendored contract. Normalization only supplies generator metadata; it does not
  invent response fields or patch generated Go.
- **Hand-vendored structs (no OpenAPI):** the quote webhook request/response is transcribed from the public
  guide plus the source `WebhookQuoter` wire construction into solver-local structs and decoded by a bounded
  strict-JSON handler. Replay a captured Beta payload before go-live to detect operational drift.
- **Onboarding step:** with `UNISWAP_API_KEY`, pull the *runtime* `/v2/uniswapx/docs` spec (gated; may be
  richer than the GitHub copy) and re-vendor if it differs (§10).

---

## 5. Pricing (v1: redemption rate − fixed haircut)

On the ≤500ms path, mirroring `rfq`'s "one multicall, decimals cached" discipline:

1. Map the request → internal; **decline (`204`) fast** on: unsupported protocol/type, wrong/`!=` chainId, unfillable
   direction (§1: `tokenIn` must be redeemable on an in-scope direct or signed-discount route *and*
   `tokenOut` that adapter's vault asset; native-ETH `tokenOut` is declined in v1 —
   this rule also auto-declines the opposing probe), or no viable inventory.
2. Read the atomically published direct LiquidLane inventory/rate snapshot, its optional gas snapshot, and its valid advertised
   signed-discount candidates. Filter inventory to the requested token pair before allocating shared capacity, so unrelated
   input-token routes backed by the same vault do not receive static shares. Matching routes and direct/private alternatives
   still share `CapacityID`, including reservations from accepted fills.
3. The UniswapX-local `Strategy.DecideQuote` selects a provisional route only to calculate the concrete
   request's executable output and, when gas accounting is configured, full estimated fill gas. It returns
   one `amountIn`/`amountOut` pair; below the enabled gas-aware floor or outside current capacity ⇒ decline.
4. Exact input returns the net output after price buffer and optional gas. Exact output uses the same greedy route
   selection in output units, adds buffer and gas, and converts the selected output legs directly to input
   with upward rounding. It neither binary-searches input nor enumerates route combinations; any produced
   output above the signed requirement remains executor surplus. No ladder, amount range, allocation, or
   quote-time route is published or retained.
5. Before publishing the result, recheck the snapshot pointer, quote epoch, and every blocking condition.
   Any fill reservation, breaker, exclusive-state change, unavailable txmanager nonce lane, or snapshot
   replacement during strategy execution turns the result into a decline.
6. Echo `requestId` and `quoteId`, and return `200` with `amountIn`, `amountOut`, and `filler` =
   `LiquidLaneUniswapXExecutor`. Do not mutate capacity on this path.

Same-token multi-output orders are priced by their total output and settled with one aggregate approval;
the Reactor distributes that token among recipients. Mixed-token outputs are declined during parsing and
rejected by the executor. Pricing is intentionally naive for v1; a competitive/win-rate controller (modeling
`exclusivityOverrideBps`, time-in-auction, competing fillers) is a later follow-up — the pricing policy
function is the seam to extend.

---

## 6. Safety & fade-aware circuit breaker

Uniswap penalizes **win-but-don't-fill** ("fade"): a temporary disable starting at **15 minutes**, increasing
**exponentially** for consecutive fades, surfaced as a `blockUntilTimestamp`. Sustained ≤500ms breaches can
also suspend. RFQ V2 explicitly holds hard-quoters "fully accountable" for winning quotes (§3.1), so safety
is economic, not just gas:

- **Fail-closed pre-fill gates** (before spending gas): cosignature recovers to the swapper-authorized
  per-order `cosigner`; `cosignerData.exclusiveFiller == our executor` (we actually won); order
  deadline/decay window still fillable; current strategy economics; and a final `eth_call` simulation
  against the current block. Any failure ⇒ skip, no tx.
- **Quote from bounded current capacity** — the latest inventory snapshot, optional gas floor, and reservations of
  already-submitted fills. Quote requests themselves stay stateless because their phase is unknowable. This
  means simultaneous winning hard quotes can contend; current-chain replanning and simulation fail closed,
  while the cold-start window and fade breakers limit the operational risk.
- **Block at order admission, not worker execution** — claiming an order invalidates quote state before it
  enters the bounded worker queue. The blocker remains until planning rejects the order or an accepted
  transaction installs its shared-capacity reservation, so queueing and the first chain-time read cannot
  advertise the same capacity again.
- **Invalidate quotes across state transitions** — a request may return only against the same snapshot epoch
  and blocker state it started with. A completed fill invalidates the snapshot before releasing its
  reservation, and the released capacity remains unavailable until a post-fill chain refresh publishes the
  next epoch.
- **Local breaker** halts quoting after repeated public-order preflight/submission failures; exclusive
  attempts are classified only by their tracked terminal reconciliation. Successful settlement resets it.
- **Honor trusted `blockUntilTimestamp` notifications** from Uniswap and expose the block/readiness state;
  readiness also fails when the latest published snapshot has no quotable inventory, while health remains
  liveness-only.
- **Gate quotes on transaction readiness:** an unavailable nonce lane blocks quote responses, the solver
  `/ready` endpoint, its readiness metric, and framework readiness. Exact-hash reconciliation and the
  fail-closed recovery rule are described in §2.2.
- **Track exclusive obligations locally:** every decodable order assigned to our executor is tracked until
  `decayStartTime`, even when later execution validation rejects it. After startup or an interrupted exclusive
  poll, the solver also reads recent filler history across all statuses so an order that became terminal while
  absent is recovered. History/parse uncertainty stops quoting instead of silently clearing obligations.
  Expired obligations are reconciled in batches against terminal order state and a canonical fill receipt at
  the shared tx manager's configured confirmation depth.
  Only a successful on-chain fill at or before the deadline clears the obligation; this makes another
  filler's timely soft override non-fade. A fill only after the deadline—including one mined through our
  executor—or a final non-filled state for an obligation observed live or recovered after a runtime poll gap
  opens an independent local fade breaker: Uniswap still counts the original quoter as faded once exclusivity
  expires unfilled. An already-terminal miss found only during initial startup recovery is retained and
  logged without starting a new local penalty window. Consequently, an unrelated or late successful fill
  cannot clear a live/runtime-recovered timed breaker. An `open` order, unknown status, or unknown receipt
  time invalidates quotes and retries reconciliation without guessing fade. The trusted
  `blockUntilTimestamp` remains the authoritative external penalty window.

---

## 7. On-chain settlement contract — `LiquidLaneUniswapXExecutor.sol`

The contract belongs in the canonical RFQ contracts repository. It is UniswapX-specific (no multi-venue
abstraction):

- `reactorCallback(ResolvedOrder[] calldata, bytes calldata callbackData)` — guarded `msg.sender == reactor`;
  routes each resolved order's input through the `LiquidLaneAdapter` named in `callbackData`, approves outputs
  back to the reactor.
- **Native-ETH output is unsupported in v1.** LiquidLane routes settle an ERC-20 vault asset, so the quote
  server and order validation currently reject zero-address output.
- `execute(SignedOrder, FillCall)` — caller-gated entrypoint matching the RFQ executor's owner-managed
  `setCallers` model; the executor contract is a transparent proxy, while the Reactor-facing filler address
  remains stable and the implementation's Reactor address is immutable.
- `FillCall.routes[]` carries only adapter, input amount, and required output. Typed `discountRoutes[]`
  carry adapter, input amount, and the signed discount/protocol terms; they intentionally have no duplicate
  `minAmountOut` field. The caller selects adapters, and the executor carries no second on-chain adapter
  allowlist. Any positive difference between resolved and routed input remains in the executor as filler
  surplus. This makes calldata planned in one block safe when an exact-output V2 Dutch input increases before
  execution, without timestamp prediction or waiting for `decayEndTime`. Direct adapters enforce their
  requested output, discount adapters enforce signed terms, and the Reactor atomically enforces aggregate
  order outputs. Pricing, capacity, and gas policy remain off-chain in the strategy.
- Owner-managed caller list and no sweep entrypoint. The published ABI exposes indexed `callers(uint256)`
  reads, but no caller-membership helper or Reactor getter. Startup scans those indexed entries with a safety
  bound and fails unless it finds the tx-sending EOA; it also validates executor bytecode. External mode
  additionally validates configured adapter authorization; internal mode filters unauthorized direct routes
  from each snapshot. Deployment must still bind the implementation to the expected Reactor out of band
  because that immutable cannot be read through the published ABI.
- ABI vendored → `api/bindings/uniswapx/`; executor calldata packed via abigen `--v2` (never
  `abi.Pack("...")`).
- Contract coverage includes mock-Reactor Forge tests. The integration harness also carries a captured-order
  mainnet-Reactor replay; a self-cosigned canonical-Reactor test remains P6.

**Optional follow-up — native output (not a launch blocker).** Canonical UniswapX Reactors accept native
output from a callback executor (see Uniswap's
[`SwapRouter02Executor`](https://github.com/Uniswap/UniswapX/blob/main/src/sample-executors/SwapRouter02Executor.sol)).
Supporting it later does not require native vaults or adapters: for an order
whose output token is the native sentinel, map the route to a configured WETH vault asset, receive WETH from
`LiquidLaneAdapter`, verify the WETH balance delta, unwrap exactly the required output, and forward that ETH
to the immutable Reactor. The executor already accepts ETH and forwards callback ETH to the Reactor; the
remaining work is explicit WETH configuration, exact-amount unwrap logic, and quote/order/fork coverage.
Never unwrap the executor's full standing WETH balance.

---

## 8. Validation & testing strategy

The public quoter onboarding flow is a **mainnet/Beta** program. The public `uniswapx-tool` currently exposes
production chain profiles; separately, the SDK maps a deployed **Sepolia Dutch V2 reactor**
(`0x0e22B6…BEBcd`) + canonical Permit2. Therefore settlement can be exercised on Sepolia or a mainnet fork,
while the quote-request half stays synthetic until we capture real Beta traffic. Confirm continued Sepolia
support and the exact Beta order transport during onboarding (§10.1).

**Layer 1 — quote-path, synthetic & local (no Uniswap, no funds).** We *are* the RFQ server: an `httptest`
harness + a small local mock POSTs schema-faithful requests (incl. the opposing probe with its distinct
obfuscated `requestId`, §4.1) at our webhook — asserting pricing, empty-`204` decline, successful `200`
response shape, and `requestId` echo. The public guide's `quoteId` differs from the public Joi schema, so a
captured Beta payload remains required before calling the fixture bit-for-bit faithful (§4.1, §10.1).

**Layer 2 — settlement, real Sepolia or mainnet fork (no funds).** Preferred: the **live Sepolia Dutch V2
reactor** (`0x0e22B6…BEBcd`, §3.2) + canonical Permit2 — a persistent, shareable public testbed; deploy our
`LiquidLaneAdapter` + vault + `LiquidLaneUniswapXExecutor` on Sepolia.
Alternative: an Anvil **mainnet fork** where canonical Permit2 + the real `V2DutchOrderReactor` already exist
(or deploy our own via UniswapX `DeployDutchV2.s.sol`, ctor `(IPermit2, protocolFeeOwner)`). Either way, drive
the whole
loop ourselves: build a V2 order as swapper → sign Permit2 witness → **self-cosign with our test key**
(per-order cosigner) → `LiquidLaneUniswapXExecutor.execute(signedOrder, callbackData)` → assert the swapper received
`tokenOut` and inventory moved. Exercises codec **parity** (our serialize/parse vs committed SDK fixtures), the
**cosignature golden** (vs the contract `ecrecover` digest §3.3), the settlement contract, and gas. Forge
integration test in the `rfq` repo.

**Layer 3 — full local loop and stress matrix.** Mock RFQ server → our webhook → self-cosign → fork fill,
end-to-end in one harness. The integration runner separately gates protocol E2E, quote conformance,
concurrent quote/fill load, quote-capacity backpressure, forced signed-discount-only fills, a bounded soak,
and restart/backend/RPC recovery. The fill burst asserts one successful transaction per order across equal
exclusive and public V2 waves; the resilience case keeps an order open across a solver restart
and requires exactly one Reactor `Fill` event. Closest to "real quote requests + validation" before Beta,
entirely ours.

**Layer 3.5 (optional) — permissionless mainnet soak.** Post-exclusivity open orders (and orders with a
nonzero `exclusivityOverrideBps`) are permissionlessly fillable, so once `LiquidLaneUniswapXExecutor` is deployed we
can opportunistically fill small open orders whose `tokenIn` we redeem — real mainnet settlement, real gas
data, **zero onboarding dependency**. Does not count toward Beta qualification (which needs *exclusive*
fills), purely de-risking; skip if flow for our tokens is negligible.

**Layer 4 — Uniswap Beta (mainnet, real funds; qualification gate only).** Register webhook URL + filler addr
(`UNISWAP_API_KEY`), drive orders with the public **`uniswapx-tool`** CLI (`UNISWAP_PRIVATE_KEY` for
`submit`), fill **5** within exclusivity (before `decayStartTime`), submit the tx hashes for **manual**
promotion. Do Layers 1–3 exhaustively first; use **minimum-size orders** in Beta to cap real-fund exposure to
≈ gas + tiny notional.

**SDK helpers for self-driven orders** (`@uniswap/uniswapx-sdk`): `V2DutchOrderBuilder` →
`buildPartial()` (swapper sign) → `cosignatureHash()` (sign with our key) → `cosignerData()` / `cosignature()`
/ `build()`; or `CosignedV2DutchOrder.fromUnsignedOrder(...)`. `OrderQuoter` simulates `resolve()` on a fork.

---

## 9. Build phases (code)

Each phase is a reviewable increment. A cross-repository phase is complete only after its changes are landed
in the owning repository and the integration harness pins the resulting revision.

- [x] **P0 — Scaffold + codegen.** Vendor UniswapX V2 reactor + Permit2 ABIs → `api/bindings/uniswapx/`;
  vendor `uniswapx-service/swagger.json` spec version 2.0.0 → generated typed poll client; scaffold the
  `uniswapx` package + `init()` register + blank-import from `main`. CGO-free build holds.
- [x] **P1 — UniswapX-local strategy layer.** Local contract + registry + `default`/`webhook`, background
  chain and optional gas snapshot, request-scoped exact-input/output pricing, and independent fill decision tests are present. `DiscountID` flows
  through strategy plans and the solver resolves fresh signed terms before execution. RFQ and UniswapX
  default quoting reuse `QuoteTask`; RFQ, LI.FI, and UniswapX default filling reuse `FillTask` while
  keeping their strategy contracts, protocol mapping, and lifecycle separate.
- [x] **P2 — V2 order codec.** V2 Dutch serialize/parse and Permit2 witness/cosignature validation have
  golden/parity coverage. Legacy V1 limit orders are deliberately unsupported; no premature V3 abstraction.
- [ ] **P3 — `LiquidLaneUniswapXExecutor.sol`.** Land the contract in the canonical RFQ repository with typed
  direct + signed-discount routes, actual balance-delta enforcement, same-token multi-output settlement, and
  mock-Reactor coverage. Regenerate and vendor the ABI here, pin the landed RFQ revision in `rfq-integration`,
  and add a canonical-Reactor self-cosign test. Native output remains an optional post-v1 extension (§7).
- [ ] **P4 — Quote webhook completion.** The bounded stateless server, local strategy pricing, health,
  readiness, metrics, phase-agnostic `v1`/`v2` and zero-swapper handling, empty-`204` decline, ingress-owned
  source-IP authentication, native-output decline, and request tests exist. Replay a captured Beta payload
  before marking the phase complete.
- [x] **P5 — Ingestion + execution completion.** Authenticated bounded polling, validation, preflight,
  async txmanager submission, receipts, pending-fill reservations, breaker, retries, and signed-discount
  discovery/resolution/calldata exist. Deadline, fee, nonce, exact-hash, readiness, and shutdown semantics are
  implemented as described in §2.2 and §6. Released capacity stays unavailable until a post-fill snapshot.
  Exclusive obligations are tracked through `decayStartTime` from admission (including execution-invalid
  awarded orders), recent terminal history is recovered after
  startup/poll gaps, and confirmed terminal receipts are batch-reconciled before clearing obligations,
  opening the independent local fade breaker for live/runtime misses, or recording a startup-only historical
  miss without a new breaker window.
- [ ] **P6 — Packaging + E2E.** The isolated local stack now passes quote → order → on-chain fill for
  exclusive V2 exact-input/exact-output/same-token multi-output, decaying public V2, public V2 with an
  exclusivity override, and a forced signed-discount-only route; the smoke test decodes executor calldata
  and verifies direct/private route
  selection. The local matrix also covers 240-request quote bursts, replay/collision/body/auth conformance,
  three concurrent fill waves, forced-discount concurrency, phase-agnostic stateless quote pressure, soak, solver
  restart, and temporary discount-backend/RPC outages. The integration harness contains a captured-order
  replay against the canonical mainnet Reactor. Remaining: land and pin the executor, add the self-cosigned
  canonical-Reactor case, then complete the Beta five-fill qualification and record the transaction hashes
  (§10).

---

## 10. Operational / non-code TODO list (live)

Tracked operational and onboarding steps — **update as items start/finish/drop** (CLAUDE.md plan-sync).

### 10.1 Confirm with Uniswap (Henrique / Andrey) — blockers to going live
- [ ] **Testnet posture:** the SDK maps a Sepolia Dutch V2 reactor (`0x0e22B6…BEBcd`, §3.2), while public
      quoter onboarding targets mainnet/Beta. Confirm whether Sepolia settlement remains supported and
      whether any testnet quote/order service exists.
- [ ] **Quote-webhook auth scheme** — exact header/secret (no signed scheme in source; FAQ publishes fixed
      RFQ source IPs to whitelist: Beta `3.135.148.114`, Prod `3.138.88.28`).
- [x] **Decline status code:** empty HTTP `204`, per the current Become a Quoter guide.
- [x] **Order delivery:** RESOLVED — order webhooks are **deprecated for new integrations** (Filler FAQ);
      delivery is **poll-only, `GET /orders` at ≤6 RPS**. Residual TODO: confirm poll auth + exact
      rate-limit enforcement at onboarding.
- [ ] **Quote-webhook registration** — how we register our quote URL, filler addr, `chainIds`,
      exclusive-filler status (the `WebhookConfiguration` is S3/Uniswap-provisioned).
- [ ] **Live chain matrix** for RFQ quoting and the order versions enabled for our onboarding account.
- [ ] **Real order flow for our assets** — is there meaningful RFQ flow for *our* vault collaterals on
      mainnet? (Determines whether quoting is worth it before the secondary-DEX hop.)
- [x] **Published limits:** quote response ≤500ms on Ethereum and ≤250ms on other chains; quote traffic is
      expected at about 1 RPS on Ethereum; order polling is capped at 6 RPS.
- [x] **`uniswapx-tool` source access:** the repository is public. Any separate reference quoter or private
      onboarding artifact still needs to be requested explicitly.
- [ ] **Captured real Beta quote-request payloads / a recording** to replay in CI (makes Layer-1 testing
      bit-for-bit faithful instead of schema-faithful).
- [x] **Order-service spec** — upstream GitHub `swagger.json` version 2.0.0 is vendored and generates the
      current typed Dutch V2 response. Confirm separately whether a spec exists for the
      parameterization-api (quote) surface.

### 10.2 Onboarding
- [ ] Submit the quoter intake form: **https://developers.uniswap.org/quoter**.
- [ ] Generate `UNISWAP_API_KEY` at developers.uniswap.org; provision the CLI submit key (`UNISWAP_PRIVATE_KEY`
      for `uniswapx-tool` only — distinct from our tx-sending EOA).
- [ ] Install and validate the public `Uniswap/uniswapx-tool` for Beta qualification.
- [ ] Hand Uniswap our **quote-server URL** + **filler (`LiquidLaneUniswapXExecutor`) address**.

### 10.3 On-chain prerequisites
- [ ] Land, review, and deploy `LiquidLaneUniswapXExecutor.sol` (mainnet).
- [ ] Deploy it with the V2 Reactor, owner, and initial caller addresses; fund the tx-sending caller EOA
      with ETH for gas (the prototype Reactor address is immutable, not owner-set).
- [ ] Configure the production signed-discount offer source. Local discovery, fill-time term resolution,
      calldata, and accounting pass end-to-end; keep production discount quoting disabled until the
      executor and backend configuration are deployed and revalidated together, then switch the deployment
      to `solverMode: internal`.
- [ ] Confirm the `LiquidLaneAdapter`(s) we'll source from authorize our executor as filler
      (`isFiller`/`marketMaker` — the adapter validates the swap *actor*, not the caller) for direct routes.
      Signed-discount-only routes use the adapter's signed authorization and are filtered independently.
- [ ] Confirm each sourced adapter holds `ALLOCATE_ROLE` on its vault's `UniversalDelegator` (vault-funded
      swaps revert without it) and watch for pending withdrawal-queue sweeps (they zero the vault-funded
      part of `getMaxAssets`).

### 10.4 Beta qualification (real funds, mainnet)
- [ ] Stand up the quote-webhook endpoint reachably (TLS, registered with Uniswap; source-IP allowlist per
      §10.1) and confirm the exact Beta order endpoint/auth with Uniswap before changing the poller URL.
- [ ] Drive **minimum-size** orders via `uniswapx-tool`; fill **5** within exclusivity (before
      `decayStartTime`).
- [ ] Collect the **5 tx hashes**; email them to our Uniswap contact for **manual** promotion review.
- [ ] On promotion: switch to the confirmed production order environment; widen order sizes per risk.

### 10.5 Deferred (post-v1)
- [ ] V3 order codec + reactor target where required; introduce a shared codec interface only when the
      second implementation proves it useful.
- [ ] Secondary-DEX sourcing in `reactorCallback` for pairs our vaults can't settle.
- [ ] Competitive/win-rate pricing controller (`exclusivityOverrideBps`, time-in-auction, competing fillers).
- [ ] Self-funding loops (keep solver-gas / pay-bid pots fed from profit) if needed.

---

## 11. Resources & references (collected)

### Docs (developers.uniswap.org)
- Architecture — https://developers.uniswap.org/docs/liquidity/uniswapx/concepts/architecture
- Auction types (RFQ + Exclusive Dutch) — https://developers.uniswap.org/docs/liquidity/uniswapx/concepts/auction-types
- UniswapX RFQ flow — https://developers.uniswap.org/docs/liquidity/uniswapx/concepts/uniswaprfq
- Become a Quoter — https://developers.uniswap.org/docs/liquidity/uniswapx/filling/mainnet/become-a-quoter
- Filling on Mainnet / Filler overview — https://developers.uniswap.org/docs/liquidity/uniswapx/filling/mainnet/filling-on-mainnet
- Filler FAQ — https://developers.uniswap.org/docs/liquidity/uniswapx/filling/faq
- Deployments — https://developers.uniswap.org/docs/liquidity/uniswapx/deployments
- Quoter intake form — https://developers.uniswap.org/quoter
- RFQ-on-Base/Arbitrum changelog — https://developers.uniswap.org/docs/changelog/active-notifications/uniswapx-rfq-auctions-on-base-and-arbitrum

### Repos
- Reactor/settlement contracts — https://github.com/Uniswap/UniswapX (deploy scripts `script/DeployDutchV2.s.sol`,
  `DeployDutchV3.s.sol`, `DeployOrderQuoter.s.sol`; fork tests `test/integration/*.t.sol`)
- TypeScript SDK — https://github.com/Uniswap/sdks/tree/main/sdks/uniswapx-sdk (builders, order parse/serialize,
  `permitData`, `resolve`, `cosignatureHash`). **Deployed-address source of truth:**
  `sdks/uniswapx-sdk/src/constants.ts` `REACTOR_ADDRESS_MAPPING` (incl. testnet reactors the docs omit).
- UniswapX CLI (Beta driver, **public**) — https://github.com/Uniswap/uniswapx-tool (`src/config.ts`:
  mainnet-only `ChainId`, `Env` Beta|Prod both → prod gateway; `src/approve.ts`: per-chain RPC list).
- **Quote webhook schema (Joi)** — https://github.com/Uniswap/uniswapx-parameterization-api
  (`lib/handlers/quote/schema.ts`, `lib/entities/QuoteRequest.ts`, `lib/entities/QuoteResponse.ts`,
  `lib/quoters/WebhookQuoter.ts`, `lib/constants.ts`, `lib/handlers/hard-quote/schema.ts`,
  `lib/providers/webhook/index.ts`)
- **Order pool** — https://github.com/Uniswap/uniswapx-service (`lib/handlers/get-orders/schema/*`,
  `lib/entities/Order.ts`; the `order-notification` handler still exists in-repo but the webhook program is
  deprecated for new integrations — see the Filler FAQ)
- **Filler FAQ (webhook deprecation, 6 RPS poll limit, source IPs)** —
  https://developers.uniswap.org/docs/liquidity/uniswapx/filling/faq
- OpenAPI spec (poll surface) — https://raw.githubusercontent.com/Uniswap/uniswapx-service/main/swagger.json
  (spec version 2.0.0 in an OpenAPI 3.0.0 document, base `https://api.uniswap.org/v2`, paths `/orders` and
  `/limit-orders`; this solver only polls `/orders`)

### Endpoints
- Production order API base — `https://api.uniswap.org/v2` (poll `GET /orders`); **gated (needs API key)**.
- The public CLI sends Beta/Prod trading commands to `https://trade-api.gateway.uniswap.org` with an
  environment flag. The exact Beta order-poll endpoint and auth for a filler are onboarding facts still to
  confirm; do not infer them from the CLI trading gateway.

### Internal repositories
- Sibling solver template — `vault-solver/internal/solvers/rfq/` + [`RFQ-PLAN.md`](RFQ-PLAN.md)
- Strategy architecture (solver-local `strategies/` registry (package `strategies`, `registry.go`) + `strategies/types` + `strategies/{default,webhook}`, shared
  LiquidLane allocator + `internal/webhook`) — [`strategy-plan.md`](strategy-plan.md)
- Framework conventions — [`../CLAUDE.md`](../CLAUDE.md)
- The RFQ contracts repository owns on-chain adapters and the UniswapX executor. `rfq-integration` pins the
  landed RFQ contracts revision. This solver repository contains only the generated/vendored executor ABI
  binding needed to build calldata.

### Beta program facts
- Gate: **5 valid exclusive fills** (before `decayStartTime`) → submit tx hashes → manual promotion.
- Env: `UNISWAP_API_KEY` (all Beta requests), `UNISWAP_PRIVATE_KEY` (CLI `submit`).
- Quote SLA: **≤500ms on Ethereum, ≤250ms on other chains**; decline with an empty HTTP `204` (§4.1).
- Won orders: **poll-only, `GET /orders` at ≤6 RPS** (order webhooks deprecated for new integrations).
- Fade penalty: **15 min**, exponential for consecutive fades; `blockUntilTimestamp` surfaced.
- Tokens: **no allow-list** (there *is* a blocklist — unsupportedtokens.uniswap.org) — open
  `tokenIn`/`tokenOut`; we **decline** (empty `204`) anything we can't price. Our fillable universe =
  pairs where `tokenIn` is redeemable through a configured direct route or a valid advertised signed-discount
  route **and** `tokenOut` is that adapter's ERC-20 vault asset. Native-ETH output is currently declined and
  remains an optional post-v1 extension (§7); the universe remains narrow by construction until the
  secondary-DEX hop.
