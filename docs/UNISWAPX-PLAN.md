# vault-solver — UniswapX Quoter + Filler solver (plan)

Adding a **UniswapX RFQ quoter + filler** to `vault-solver` as the **`uniswapx-filler`** solver, following the
framework boundary and conventions in [`../CLAUDE.md`](../CLAUDE.md). We become a UniswapX **market maker**:
we expose the quote webhook Uniswap's RFQ server calls, win exclusivity, then settle the order on Uniswap's
Reactor — sourcing liquidity from our Symbiotic vault `LiquidLaneAdapter`s (vaults-first; a secondary-DEX hop
is a later step).

---

## 1. What the solver does

UniswapX RFQ ("Exclusive Dutch Auction") is, structurally, the same shape as our own Symbiotic RFQ — signed
orders, a settlement Reactor, off-chain quoters/fillers — so this solver reuses the `rfq` solver's machinery
almost wholesale, plus a UniswapX protocol adapter.

- **Quote webhook (the ≤500ms hot path).** Uniswap's RFQ server makes a synchronous HTTP `POST` to a webhook
  URL we register with them. We price the requested swap off the `LiquidLaneAdapter` oracle (`getAmountOut`),
  apply a haircut + gas floor, and respond `200` with `amountOut` (or `amountIn` for `EXACT_OUTPUT`) **and our
  `filler` = the on-chain `UniswapXExecutor` address** — that is how we claim last-look exclusivity. We
  **decline** when we can't/won't price (see §4 for the decline semantics).
- **Order ingestion.** If our quote wins, Uniswap's API (the cosigner = Uniswap Labs) finalizes the order and
  we pick it up by **polling `GET /orders`** (≤6 RPS, Uniswap's stated filler rate limit), dedup by
  `orderHash`. Uniswap's `order-notification` push webhooks are **deprecated for new integrations**
  (Filler FAQ), so poll is the only delivery channel; the `OrderSource` seam keeps push addable if
  Uniswap ever re-enables it (§4.2).
- **Settlement.** We call **`executeWithCallback`** on Uniswap's V2 Dutch Order Reactor; our
  `UniswapXExecutor` implements `IReactorCallback.reactorCallback`, sources the output token from the
  `LiquidLaneAdapter` (vaults-first), and approves it back to the Reactor. Inputs are pulled from the swapper
  via Permit2. This is the analog of our own `Executor.execute`, but for Uniswap's reactor interface.
- **Safety.** Fail-closed pre-fill validation gates + a fade-aware circuit breaker (Uniswap penalizes
  win-but-don't-fill — see §4, §6).
- **State** — in-memory only: `quotes` (by `quoteId`), `orders` (state machine), `attempts`; TTL-swept.

**Scope decisions (locked):**

| Decision | Choice | Rationale |
|---|---|---|
| Our role | **Quoter + Filler (market maker)** | We provide liquidity for our own vault assets on UniswapX. |
| Liquidity source | **Symbiotic vaults first**; secondary-DEX hop later | Reuse `LiquidLaneAdapter` redemption pricing; widen pairs later. |
| Order version | **V2 first (mainnet); codec abstracted for V3** | "Goal is mainnet" (V2). Tempo + most L2s are V3 — slots in behind the same interface later. |
| Pricing v1 | **Redemption rate − fixed haircut**, gas-aware floor | Ship fast, tune later (matches how `3f`/`rfq` shipped). |
| On-chain executor | **UniswapX-specific** `UniswapXExecutor.sol` | Smallest, auditable surface; no multi-venue abstraction yet. |
| Code organization | **Sibling solver reusing rfq's `default` + `webhook` strategies** — the strategy layer (contract, registry, strategies) is promoted to a shared package on second use, alongside `internal/liquidlanemath` + `internal/webhook` | The strategies are protocol-neutral (verified, §2.1): selection/validation/caching/recovery all transfer; only candidate construction and pricing policy are solver-side. |

**Directionality (structural constraint):** `LiquidLaneAdapter`s are one-way — they consume a
token-to-redeem and pay out the vault asset. So the only fillable orders are **RWA-in → vault-asset-out**:
`tokenIn` must be a token-to-redeem on a whitelisted adapter *and* `tokenOut` that adapter's vault asset
(native-ETH `tokenOut` maps to a WETH vault asset via executor unwrap, §7). Everything else — including
the reverse-direction opposing probe (§4.1) — auto-declines.

**Out of scope (v1):** V3/Tempo order type (codec stubbed behind the interface), multi-output orders
(`numOutputs > 1` declined), secondary-DEX sourcing, self-funding, exclusivity-override (`exclusivityOverrideBps`)
economics, and quoting any pair our vaults can't settle.

---

## 2. How it maps onto the framework

A new self-contained `internal/solvers/uniswapx/` implementing `solver.Solver` — **no framework edits**
(CLAUDE.md modularity rule). Code organization follows the repo's **solver-local strategy architecture**
(see `docs/strategy-plan.md`). §2.5 is the consolidated reuse-vs-delta implementation checklist.

### 2.1 Shared strategy logic — reusing the rfq strategy layer

> Supersedes this plan's earlier `internal/symbiotic/` shared-tier proposal, in favor of the repo's
> **solver-local strategy architecture** (`docs/strategy-plan.md`): the framework never parses/routes
> strategy configs; each solver defines its own **strategy contract and registry**; and the genuinely
> cross-solver pieces live in **`internal/liquidlanemath/`** (the LiquidLane fixed-point rate math:
> `AmountOutForRate`, `MaxAmountInForRate`, `MinAmountInForAmountOut`, `RateForAmountOut`, `RATE_SCALE`
> 1e18) and **`internal/webhook/`** (a neutral HTTP-decider transport client — timeouts, body caps,
> env-backed headers, strict decode).

**Verified against the rfq strategy implementation (2026-07-06): the `default` and `webhook` strategies
are protocol-neutral and reusable by `uniswapx` as-is.** Everything the `default` strategy does —
candidate matching by `tokenOut`, oracle pricing through the `Pricing` seam, greedy rate-sorted leg
selection on `liquidlanemath`, the validation/replay rules (legs reference input candidates, no
duplicates, ≤ `maxAssets`, achievable under `maxRate`, sums reconcile), the TTL fill-plan cache keyed by
`quoteId`, and `BuildFillPlan` recovery with the `RequiredAmountOut` gate — operates purely on neutral
types (addresses, big.Ints, candidates). The same holds for the `webhook` strategy and its JSON wire
contract. Neither knows anything about the RFQ backend; the only rfq-ness is the import path. The math is
a small fraction — **the strategies' real content is the adapter/leg selection + plan
validation/caching/recovery, and all of it transfers.**

So `uniswapx` does **not** mirror per-solver copies — it triggers CLAUDE.md's shared-code rule
("hand-written domain adapters stay inside the owning solver *unless a second solver actually reuses
them*"): **on second use, the strategy layer is promoted out of `rfq/` into a shared package** and both
solvers consume it. Each solver still routes by its own `strategy: {name, config}` — the shared part is
the contract, registry, and the two strategy implementations.

```
internal/
  {config,chain,signer,txmanager,solver,observability}/   # framework — unchanged, protocol-agnostic
  liquidlanemath/    # SHARED: rate math, reused verbatim
  webhook/           # SHARED: remote-strategy transport, reused verbatim
  llstrategy/        # PROMOTED from internal/solvers/rfq/ when uniswapx lands (naming TBD)
    types/           #   Strategy{DecideQuote, BuildFillPlan}, Pricing seam, QuoteInput/FillPlan + wire JSON
    registry/        #   name→factory registry
    strategies/{default,webhook}/   # reused by rfq AND uniswapx, unchanged logic
  solvers/
    rfq/             # consumes the promoted layer; backend candidate construction + discounts stay here
    uniswapx/        # consumes the promoted layer; chain candidate construction + UniswapX plumbing here
```

- **Reused as-is:** `internal/liquidlanemath/`, `internal/webhook/`, and the promoted strategy layer —
  contract, registry, `default` + `webhook` strategies with their selection/validation/caching/recovery
  logic unchanged. The discount branch is simply never taken (uniswapx candidates carry no `DiscountID`).
- **Two things that need no strategy changes at all** (they compose from outside):
  - **Pricing policy (haircut + gas floor)** is uniswapx **solver-side**, applied *after*
    `DecideQuote`: quote `planOutput − haircutBps` to Uniswap while the plan sources the full amount
    (the spread is the margin), and gate on the gas-aware min-profit floor before responding. The shared
    `default` strategy stays policy-free — rfq behavior untouched.
  - **The fill-time decay gate** maps directly onto `BuildFillPlan(RequiredAmountOut = resolved decayed
    output at fill block)` — the strategy's built-in `QuotedAmountOut ≥ RequiredAmountOut` check *is*
    the "price didn't decay past our quote" gate from §6.
- **The one contract delta — `EXACT_OUTPUT`:** `QuoteInput` is exact-input-only (`AmountIn` required, no
  trade type). Options: (a) **v1 declines `EXACT_OUTPUT`** — zero contract change; or (b) add an
  optional trade-type/amount-out field to the shared contract + an output-driven loop in `default`
  (additive; rfq unaffected, it only ever sends exact-input). Decide in P1; (a) is the default posture.
- **Candidate construction is solver-owned** (the strategy architecture keeps chain reads with the
  solver/strategy, not in a shared tier), and here lies the one **inversion vs `rfq`**: rfq receives quote-time candidates
  (`maxAssets`/`maxRate`/`assetDecimals`) *in the backend's `/quote` request* and reads on-chain only for
  recovery; UniswapX's quote request carries **no inventory**, so `uniswapx` builds candidates from chain
  on every quote — its own `chainreader.go` modeled on rfq's (paused/`getMaxAssets`/`getMaxRate`, the
  `marketMaker`/`owner`/`isFiller` authorization filter, startup vault/asset resolution, shared decimals
  cache) over the configured `adapters`. Hot-path consequence: that's a second multicall next to
  `getAmountOut` — either merge both into one `aggregate3` or refresh candidates in a background loop and
  quote off the cached snapshot (≤500ms budget); decide in P1/P4. (If the rfq/uniswapx readers start
  drifting, extracting a shared LiquidLane reader is a later, separate refactor — not assumed here.)
- **Cost/risk:** the promotion is a pure package move (import-path change, logic untouched) guarded by
  the existing strategy tests; the behavior-preserving extraction of rfq's selection logic already
  happened in the rfq strategy refactor. `uniswapx`'s P1 shrinks to: the package move, the candidate
  reader, and (if chosen) the additive exact-output extension.

### 2.2 Reuse of the generic layer (unchanged)

- **`Run(ctx)`** starts the UniswapX **quote webhook server** *and* the `GET /orders` poll loop, blocking
  until ctx cancels. The framework observability server (`:9090`) stays separate.
- **The quote server is code-first OpenAPI via Huma** — request/response structs carry validation
  tags driving both inbound validation and the served spec (same approach as `rfq`).
- **`/metrics`** is the framework's shared registry; the solver registers its collectors via
  `deps.Metrics.Registerer()` in the factory (HTTP middleware records request/latency by route).
- **Fills go through the shared `txmanager`** (CLAUDE: solvers never send directly). We build the
  `UniswapXExecutor.execute` / reactor `executeWithCallback` calldata; txmanager owns nonce/send/receipt.
- **On-chain reads use `chain.Multicall`** (via the solver's candidate reader + the strategy's `Pricing`
  seam, §2.1).
- **Addresses + URLs come from `solver.config`**; secrets (`UNISWAP_API_KEY`, the keeper key) via `*Env`
  indirection (`os.Getenv` at point of use).
- **Signer** — the framework's single EOA is the UniswapX **keeper** (holds the role on `UniswapXExecutor`,
  submits `execute`). We do **not** cosign or sign orders in production (that's Uniswap Labs + the swapper);
  our EIP-712 code is verification + test-only signing.

### 2.3 Component map (file → responsibility)

| Go (`internal/solvers/uniswapx/`) | Responsibility | Reuse vs net-new |
|---|---|---|
| `solver.go` | `Run`: quote server + poll loop; `init()` self-register; factory | mirror `rfq` |
| `config.go` | typed `solver.config` (reactor/permit2/cosigner/executor addrs, ports, pricing knobs, adapters) | mirror `rfq` |
| `server.go` / `apitypes.go` / `middleware.go` | Huma quote webhook (`POST /quote`, ≤500ms), `/health`, OpenAPI; auth (each request priced independently — probe is indistinguishable, §4.1) | mirror `rfq` + net-new |
| `quote.go` / `chainreader.go` | quote orchestration: scope checks, build `QuoteCandidate`s from chain (§2.1), call the shared `Strategy.DecideQuote`, apply the solver-side haircut + gas floor, map to response + `filler` | mirror `rfq` + reader port |
| *(shared)* strategy layer | contract + registry + `default`/`webhook` strategies, promoted out of `rfq/` (§2.1) | **reused as-is** |
| `order.go` | **UniswapX V2 Dutch order codec** (serialize/parse), Permit2 witness EIP-712, cosignature verify; reactor calldata via abigen | **net-new** |
| `ordersource.go` | `OrderSource` — poll `GET /orders` (≤6 RPS), dedup by `orderHash`; interface seam for a future push channel | net-new (poll mirrors `rfq`) |
| `execution.go` | validation gates, build callbackData, `executeWithCallback` via txmanager, reconcile, breaker | mirror `rfq` + net-new |
| `store.go` / `metrics.go` | in-memory quotes/orders/attempts (TTL-swept); collectors on shared registry | mirror `rfq` |
| `backend.go` | thin adapter over the generated `uniswapx-service` poll client + the hand-vendored quote-webhook structs | mirror `rfq` |

**On-chain (sibling `rfq` repo, `src/uniswapx/`):** `UniswapXExecutor.sol` — see §7. ABI vendored to
`api/bindings/uniswapx/` via `make refresh-abi`.

### 2.4 Configuration (`solver.config`)

One code path; per-environment differences are pure YAML (CLAUDE.md "config is king"). Secrets via `*Env`
indirection (read with `os.Getenv` at point of use, never stored in the parsed config). `chain` / `signer` /
`txManager` / `observability` come from the framework block, unchanged from the `rfq` profile. A full profile
(`config/uniswapx.mainnet.example.yaml`) lands in build phase P4.

```yaml
solvers:
  - name: uniswapx-filler
    config:
      orderType: v2                          # v2 now; v3 later (codec is versioned)
      reactor:  "0x00000011F84B9aa48e5f8aA8B9897600006289Be"  # V2 Dutch Order Reactor (per chain)
      permit2:  "0x000000000022D473030F116dDEE9F6B43aC78BA3"
      cosigner: "0x…UniswapLabs"             # verified on every order (per-order cosigner field §3.3)
      executor: "0x…UniswapXExecutor"        # our callback contract == the `filler` we quote
      quote:
        listenAddr: ":42080"                 # ≤500ms webhook (POST /quote, /health, OpenAPI)
        apiKeyEnv: UNISWAP_API_KEY           # env var NAME (secret never in config)
        authHeader: "x-api-key"              # static header Uniswap registers (confirm §10.1)
        declineMode: zeroAmount              # 200 + amountOut "0" — a 404 lands in Uniswap's axios ERROR path (§4.1)
      orderSource:
        poll: { url: "https://api.uniswap.org/v2", intervalMs: 1000 }  # GET /orders, ≤6 RPS; dedup by orderHash
      strategy:                              # shared strategy layer (docs/strategy-plan.md), routed by name
        name: default                        # default (greedy direct-leg) | webhook (remote decider)
        config: {}                           # opaque to the solver; parsed/validated by the strategy
      pricing:                               # uniswapx solver-side policy, applied after DecideQuote (§2.1)
        haircutBps: 30
        priceBasis: auctioned                # auctioned (prod) | onchain (size vs cached oracle)
        minProfitWei: "1000000"
        loanPerEth: "0"                      # base units per 1 ETH; enables gas-netting (0 => flat)
        estGasPerFill: 300000
        maxTxGasPriceWei: "60000000000"      # 60 gwei — bounds per-tx gas price
      breaker: { maxFailures: 3, windowMs: 3600000 }  # + honor Uniswap blockUntilTimestamp
      adapters:                              # vaults-first inventory; vault+asset resolved on-chain at startup
        - "0x…liquidLaneAdapter"
```

### 2.5 Implementation delta vs `rfq` — what reuses, what changes, what's manual

The working checklist for the build: everything below is either lifted from the shipped `rfq` solver,
deliberately different from it, or an operational step `rfq` never needed. (Verified against the `rfq`
code 2026-07-06 — see §2.1 for the extraction evidence.)

**Reused as-is:**

- `internal/liquidlanemath/` — the LiquidLane fixed-point rate math (`AmountOutForRate`,
  `MaxAmountInForRate`, `MinAmountInForAmountOut`, `RateForAmountOut`, `RATE_SCALE` 1e18), verbatim.
- `internal/webhook/` — the neutral remote-decider transport client, verbatim (backs the optional
  `webhook` strategy).
- `rfq`'s `default` and `webhook` strategies — selection, validation/replay, fill-plan caching, and
  recovery — **reused as-is via the promoted shared strategy package** (§2.1); the discount branch is
  never taken (UniswapX candidates carry no `DiscountID`).
- The chain-reader surface, ported as the uniswapx candidate reader (§2.1): batched `getAmountOut`,
  paused/`getMaxAssets`/`getMaxRate` reads, adapter→vault→asset startup resolution, the
  `marketMaker`/`owner`/`isFiller` authorization filter, shared decimals cache.
- Solver scaffolding patterns 1:1: `init()` registration + factory, the shared strategy
  contract/registry (`Strategy{DecideQuote, BuildFillPlan}` + `Pricing` seam + `strategy:
  {name, config}` config routing), Huma code-first quote server + middleware stack, in-memory TTL-swept
  store keyed by `quoteId`, poll loop + order state machine shape, calldata-only submission through the
  shared `txmanager`, collectors on `deps.Metrics.Registerer()`.

**Done differently from `rfq` (the real implementation work):**

| # | Area | `rfq` does | `uniswapx` must do |
|---|---|---|---|
| 1 | Quote-time inventory | Backend sends `adapters[]` (maxAssets/maxRate/decimals) in the `/quote` body; on-chain inventory read is recovery-only | **Self-source on-chain** over configured `adapters`: the recovery read becomes the quote-time read. Hot path: merge the inventory + `getAmountOut` multicalls into one `aggregate3`, or price off a background-refreshed snapshot (≤500ms) — §2.1 |
| 2 | Quote wire contract | Backend schema, `x-rfq-shared-secret`, 204 decline, 422 on schema violation | UniswapX Joi schema (hand-vendored, golden-tested), static API-key header + source-IP allowlist, **`200`+`amountOut:"0"` decline**, echo the obfuscated `requestId`, answer the indistinguishable opposing probe independently — §4.1 |
| 3 | Quoted price policy | Quotes the raw oracle `getAmountOut` (no margin) | Apply **`haircutBps` + gas-aware min-profit floor** solver-side *after* `DecideQuote` (the shared strategy stays policy-free; the spread is the margin); below floor ⇒ decline — §2.1, §5 |
| 4 | `EXACT_OUTPUT` | Hard-rejected at validation | Shared `QuoteInput` is exact-input-only: **v1 declines `EXACT_OUTPUT`** by default; optional additive contract extension + output-driven loop if flow warrants — §2.1, §5 |
| 5 | Order ingestion | Polls own backend `GET /orders`, decodes the Symbiotic Reactor order from the backend payload | Polls Uniswap `GET /orders?filler=<executor>` (≤6 RPS); **net-new V2 Dutch order codec + Permit2 witness EIP-712 + cosignature recovery** (`order.go`, the riskiest unit — P2) — §3.3, §4.2 |
| 6 | Pre-fill validation | Order-deadline + strategy↔order binding checks | Those **plus**: cosigner recovers to configured `cosigner`, `exclusiveFiller == our executor`, decay window still fillable, resolved output at current block ≥ quoted floor — §6 |
| 7 | Settlement call | `Executor.fill(order, protocolSig, swaps[], discountSwaps[], executorData)` on our Reactor | `UniswapXExecutor.execute(SignedOrder, callbackData)` → Uniswap reactor `executeWithCallback` → `reactorCallback` runs the adapter swaps; callbackData built from the cached `strategyRecord.Legs` (map 1:1 onto `Swap{recipient,tokenIn,amountIn,amountOut}`); **native-ETH outputs unwrap WETH and forward ETH** — §7 |
| 8 | Failure economics | Failed fill ⇒ order re-armed next poll; no external penalty | **Fade penalty regime**: fail-closed gates before gas, local breaker, honor `blockUntilTimestamp`, quote only what inventory certainly fills — §6 |
| 9 | Not ported at all | Discount legs, backend `/discounts`, `solverMode` internal/external split | None of it — direct legs only; scoping is just the configured `adapters` list |

**Manual / operational (no `rfq` analogue — `rfq` only needed a shared secret with our own backend):**

- Uniswap onboarding: intake form, `UNISWAP_API_KEY`, register the quote URL + filler address +
  chainIds (S3-provisioned by Uniswap), allowlist their RFQ source IPs — §10.1–10.2.
- Beta qualification: 5 exclusive fills with real funds, tx hashes emailed for manual promotion — §10.4.
- On-chain: deploy `UniswapXExecutor`, grant the keeper EOA, get the executor authorized as
  `isFiller`/`marketMaker` on each sourced adapter, confirm adapter `ALLOCATE_ROLE` — §10.3.
- Vendoring: UniswapX reactor + Permit2 ABIs → `api/bindings/uniswapx/`, `uniswapx-service`
  `swagger.json` → generated poll client, hand-vendored quote-webhook structs — §4.3, P0.

---

## 3. UniswapX protocol reference (verified ground truth)

Collected and source-verified during planning; **re-verified 2026-07-06** against the UniswapX repo,
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
- The chain matrix keeps expanding (Robinhood Chain + Arc were registered June 2026) — **confirm the live
  matrix with Uniswap** (§10). A V3 reactor is also deployed on mainnet but docs state mainnet RFQ does
  not route to it today.

### 3.2 Deployed addresses (mainnet, chainId 1 — verify before use)

| Contract | Address |
|---|---|
| V2 Dutch Order Reactor | `0x00000011F84B9aa48e5f8aA8B9897600006289Be` |
| V3 Dutch Order Reactor | `0x0000000015757c461808EA25Eb309638B62681cf` |
| ExclusiveDutchOrderReactor (V1) | `0x6000da47483062A0D734Ba3dc7576Ce6A0B645C4` |
| OrderQuoter | `0x54539967a06Fc0E3C3ED0ee320Eb67362D13C5fF` *(docs report several variants — verify per chain)* |
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
| Sepolia (11155111) | Dutch V1 | `0xD6c073F2A3b676B8f9002b276B618e0d8bA84Fad` |
| Unichain Sepolia (1301) | Hybrid (v4) | `0x000000000C75276D956cc35218ca8f132D877957` |
| **Tempo (4217)** | Dutch V3 | `0x00000000fc1E66C9f582566EAd00108e55F1c0C6` (RPC `https://rpc.tempo.xyz`) |

Source of truth for deployed addresses is the SDK `sdks/uniswapx-sdk/src/constants.ts` `REACTOR_ADDRESS_MAPPING`
(Permit2 canonical on all incl. Sepolia). The `uniswapx-tool` CLI's quote/order flow is **mainnet-only**
(`ChainId` enum = {1, 42161, 8453}; `Env.Beta`/`Env.Prod` both hit the prod gateway) — testnets are reachable
only for direct on-chain settlement.

### 3.3 V2 Dutch order struct & cosignature

```
SignedOrder { bytes order; bytes sig }     // order = ABI-encoded V2DutchOrder; sig = swapper Permit2 signature

V2DutchOrder {
  OrderInfo{ reactor, swapper, nonce, deadline, additionalValidationContract, additionalValidationData }
  address cosigner                          // per-order field (Uniswap Labs in prod); no reactor ctor/setter
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

- **Method/timeout:** `axios.post`, `application/json`, **500ms on every chain** (`WEBHOOK_TIMEOUT_MS_DEFAULT
  = 500`; the FAQ's "250ms on non-mainnet" figure is stale). `ECONNABORTED` past that → we silently lose.
  Source: `lib/quoters/WebhookQuoter.ts`, `lib/constants.ts`.
- **Two POSTs per request, in parallel:** the real quote **plus an "opposing" probe** (inverted `type`,
  swapped `tokenIn`/`tokenOut`) for price discovery. Since June 2026 (parameterization-api #456,
  "obfuscate two-sided RFQ quote") each of the two carries a **distinct fresh `requestId`** and they are
  sent in randomized order — the pair is **indistinguishable and uncorrelatable by design**. So: no
  probe-pairing logic anywhere; price every request independently and honestly. For us the probe's
  reverse direction (vault-asset → RWA) is structurally unfillable and auto-declines (§1 directionality).
- **Auth:** **no signed scheme** — static headers we register (e.g. an API key) are sent verbatim. The FAQ
  also publishes fixed RFQ source IPs to whitelist (Beta `3.135.148.114`, Prod `3.138.88.28`). Confirm at
  onboarding.
- **Decline:** **`200` with `amountOut: "0"`** — the one form that demonstrably flows through the graceful
  `isNonQuote()` path. Do **not** use `404`: axios' default `validateStatus` rejects non-2xx, so a 404
  lands in Uniswap's *error* path (`HTTP_ERROR`, same bucket as a timeout). The public docs say `204` —
  confirm its handling at onboarding (§10.1), but `zeroAmount` is the shipped default.
- **Response must echo the (obfuscated) `requestId` received** or it's dropped (`RFQ_FAIL_REQUEST_MATCH`).

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
  "protocol": string,         // default "V1"
  "quoteId": string           // optional uuid
}
```

**Response body** (`RfqResponseJoi`):
```jsonc
{
  "chainId": number, "requestId": string,        // echo requestId
  "tokenIn": string, "amountIn": string,
  "tokenOut": string, "amountOut": string,       // "0" => decline
  "filler": string,                              // our UniswapXExecutor address
  "quoteId": string
}
```

### 4.2 Won/cosigned order delivery — us ← Uniswap (POLL-ONLY)

**Order webhooks are deprecated.** Per Uniswap's Filler FAQ, `order-notification` webhooks "were deprecated
on UniswapX due to degraded performance" and **new webhook integrations are no longer onboarded** —
"fillers should start with polling for orders and rate limit at 6 RPS". So there is no push half:
`OrderSource` is poll-only, behind an interface seam that admits a push channel if Uniswap re-enables one.

**POLL — `GET https://api.uniswap.org/v2/orders`** (mainnet; Beta base `https://beta.api.uniswap.org/v2`),
**≤6 RPS**:
- Query: `orderStatus=open&filler=<us>&chainId=<id>` (+ `limit, cursor, sortKey=createdAt, sort, desc,
  orderHash(es), swapper, pair`). `orderStatus ∈ {open, expired, error, cancelled, filled, insufficient-funds}`.
- Response: `{ orders: OrderEntry[], cursor? }`; a Dutch V2 entry carries `encodedOrder, signature,
  cosignature, cosignerData{decayStartTime, decayEndTime, exclusiveFiller, inputOverride, outputOverrides[]},
  input, outputs[], orderHash, chainId, swapper, txHash, quoteId, requestId, nonce, ...`. The
  `encodedOrder` + swapper `signature` *is* our `SignedOrder{order, sig}` — directly fillable.

**Ingestion design:** poll tight — `intervalMs` ≈ 500–1000, well inside the 6 RPS budget. Against the ~24 s
mainnet exclusivity window a 1 s cadence costs at most ~1 s of the window. Dedup by `orderHash`.

**Hard-quote** (`POST /hard-quote`, parameterization-api) — the synchronous cosigning flow where the KMS
cosigner sets `exclusiveFiller` to the `filler` we returned (with a nonzero default
`exclusivityOverrideBps`). **We do not implement it**; it explains how our quote becomes a won order.

### 4.3 Vendoring plan (matches CLAUDE.md codegen discipline)

- **Poll client:** vendor `uniswapx-service/swagger.json` (OpenAPI 3.0.0; raw GitHub URL in §11) → generate the
  Go client via the Java openapi-generator (same pipeline as `openapi/rfq-backend.openapi.json`). Covers only
  `/orders`, `/limit-orders`, `/nonce`.
- **Hand-vendored structs (no OpenAPI):** the quote webhook (request/response) — transcribed from the Joi
  files into Go structs with Huma validation tags, **golden-tested** against committed fixtures. (The
  hard-quote and deprecated push-notification shapes are reference-only; not vendored.)
- **Onboarding step:** with `UNISWAP_API_KEY`, pull the *runtime* `/v2/uniswapx/docs` spec (gated; may be
  richer than the GitHub copy) and re-vendor if it differs (§10).

---

## 5. Pricing (v1: redemption rate − fixed haircut)

On the ≤500ms path, mirroring `rfq`'s "one multicall, decimals cached" discipline:

1. Map the request → internal; **decline (`200`+`amountOut:"0"`) fast** on: wrong/`!=` chainId, unfillable
   direction (§1: `tokenIn` must be a token-to-redeem on a whitelisted adapter *and* `tokenOut` that
   adapter's vault asset; native-ETH `tokenOut` maps to a WETH vault asset, unwrapped at settlement §7 —
   this rule also auto-declines the opposing probe), `numOutputs > 1`, or no viable inventory.
2. Build `QuoteCandidate`s (solver-owned, §2.1): cached `tokenIn` decimals + the candidate/authorization
   reads + **one Multicall3 `getAmountOut`** across candidate adapters for the matching asset (served to
   the strategy through its `Pricing` seam).
3. `Strategy.DecideQuote` (the shared `default` strategy): greedy direct-leg selection on
   `internal/liquidlanemath`. Then the **solver-side policy** (§2.1): `quote = planOutput − haircutBps`,
   then a **gas-aware min-profit floor** (est. fill gas × gas price, netted at the configured loan/ETH
   rate). Below floor ⇒ **decline**.
4. Persist the strategy by `quoteId` (TTL-swept), return `200` with `amountOut` (or `amountIn` for
   `EXACT_OUTPUT`) + `filler` = `UniswapXExecutor`.

`EXACT_OUTPUT`: the shared `QuoteInput` contract is exact-input-only, so **v1 declines `EXACT_OUTPUT` by
default** (§2.1). If flow data says it matters, the additive path is an optional trade-type/amount-out
field on the shared contract plus an output-driven loop in `default` walking the same rate-sorted legs on
`liquidlanemath.MinAmountInForAmountOut` — `rfq` is unaffected either way (it only ever sends
exact-input). Pricing is intentionally naive for v1; a competitive/win-rate controller (modeling
`exclusivityOverrideBps`, time-in-auction, competing fillers) is a later follow-up — the pricing policy
function is the seam to extend.

---

## 6. Safety & fade-aware circuit breaker

Uniswap penalizes **win-but-don't-fill** ("fade"): a temporary disable starting at **15 minutes**, increasing
**exponentially** for consecutive fades, surfaced as a `blockUntilTimestamp`. Sustained ≤500ms breaches can
also suspend. RFQ V2 explicitly holds hard-quoters "fully accountable" for winning quotes (§3.1), so safety
is economic, not just gas:

- **Fail-closed pre-fill gates** (in `execution.go`, before spending gas): cosignature recovers to the
  configured `cosigner`; `cosignerData.exclusiveFiller == our executor` (we actually won); order
  `deadline`/`decayStartTime` still fillable; resolved output at current block ≥ our cached quoted output
  (price didn't decay past our floor — the `priceBasis: auctioned|onchain` knob applies); strategy↔order
  token/amount binding. Any failure ⇒ skip, no tx.
- **Quote only what we can certainly fill** — inventory present + gas floor cleared — so we rarely fade.
- **Local breaker** (the OEV `breaker{maxFailures, windowMs}` pattern) halts *quoting* after N reverts in a
  window.
- **Honor `blockUntilTimestamp`** from Uniswap — stop quoting until it passes; surface as a metric + log.

---

## 7. On-chain settlement contract — `UniswapXExecutor.sol`

New contract in the sibling `rfq` repo (`src/uniswapx/`), UniswapX-specific (no multi-venue abstraction),
mirroring our existing `Executor.sol` role-gating:

- `reactorCallback(ResolvedOrder[] calldata, bytes calldata callbackData)` — guarded `msg.sender == reactor`;
  routes each resolved order's input through the `LiquidLaneAdapter` named in `callbackData`, approves outputs
  back to the reactor.
- **Native-ETH outputs:** the reactor pays native outputs from **its own balance**, not via `transferFrom` —
  for a native-output order the callback unwraps the WETH received from the adapter and forwards ETH to the
  reactor within the callback; ERC-20 outputs are approved and pulled. In scope for v1 (ETH-output flow is
  likely the largest real market for a WETH-vault asset like wstETH).
- `execute(SignedOrder, bytes callbackData)` — entry gated to our **keeper EOA** (owner/role), so only we
  trigger our own exclusive fills; calls `IReactor(uniReactor).executeWithCallback(order, callbackData)`.
- `callbackData` = ABI-encoded `(adapter, swapParams)[]`, built off-chain in `execution.go` from the cached
  strategy. The **secondary-DEX route** is a later variant of this same blob (vaults-first now).
- Owner-set: reactor address, adapter allowlist, sweep/rescue.
- ABI vendored → `api/bindings/uniswapx/`; `executeWithCallback` calldata packed via abigen `--v2` (never
  `abi.Pack("...")`).
- **Forge fork integration test** mirrors UniswapX's own `test/integration/*.t.sol` (self-cosign loop, §8).

---

## 8. Validation & testing strategy

**There is no testnet RFQ *server*, but there IS a testnet *reactor*.** Uniswap's quote/order API is mainnet
only (Beta is *mainnet*, production contracts, real funds — the `uniswapx-tool` CLI confirms `ChainId` =
{1, 42161, 8453}). **However**, the SDK ships **deployed Sepolia reactors** (§3.2) — a live **Dutch V2 reactor
on Sepolia** (`0x0e22B6…BEBcd`) + canonical Permit2 — so the settlement/fill half can be exercised on a real
public testnet, not just a fork. The quote-request half stays self-driven/synthetic until Beta.

**Layer 1 — quote-path, synthetic & local (no Uniswap, no funds).** We *are* the RFQ server: an `httptest`
harness + a small local mock POSTs requests built to the exact `uniswapx-parameterization-api` schema (incl.
the opposing probe with its distinct obfuscated `requestId`, §4.1) at our ≤500ms webhook — asserting pricing,
decline (`amountOut:"0"`), the `200` shape, `requestId` echo. Wire format is pinned, so synthetic ≈ real for
the contract.

**Layer 2 — settlement, real Sepolia or mainnet fork (no funds).** Preferred: the **live Sepolia Dutch V2
reactor** (`0x0e22B6…BEBcd`, §3.2) + canonical Permit2 — a persistent, shareable public testbed; deploy our
`LiquidLaneAdapter` + vault + `UniswapXExecutor` on Sepolia (we already run Sepolia/Hoodi deployments).
Alternative: an Anvil **mainnet fork** where canonical Permit2 + the real `V2DutchOrderReactor` already exist
(or deploy our own via UniswapX `DeployDutchV2.s.sol`, ctor `(IPermit2, protocolFeeOwner)`). Either way, drive
the whole
loop ourselves: build a V2 order as swapper → sign Permit2 witness → **self-cosign with our test key**
(per-order cosigner) → `UniswapXExecutor.execute(signedOrder, callbackData)` → assert swapper received
`tokenOut` and inventory moved. Exercises codec **parity** (our serialize/parse vs committed SDK fixtures), the
**cosignature golden** (vs the contract `ecrecover` digest §3.3), the settlement contract, and gas. Forge
integration test in the `rfq` repo.

**Layer 3 — full local loop.** Mock RFQ server → our webhook → self-cosign → fork fill, end-to-end in one
harness. Closest to "real quote requests + validation" before Beta, entirely ours.

**Layer 3.5 (optional) — permissionless mainnet soak.** Post-exclusivity open orders (and orders with a
nonzero `exclusivityOverrideBps`) are permissionlessly fillable, so once `UniswapXExecutor` is deployed we
can opportunistically fill small open orders whose `tokenIn` we redeem — real mainnet settlement, real gas
data, **zero onboarding dependency**. Does not count toward Beta qualification (which needs *exclusive*
fills), purely de-risking; skip if flow for our tokens is negligible.

**Layer 4 — Uniswap Beta (mainnet, real funds; qualification gate only).** Register webhook URL + filler addr
(`UNISWAP_API_KEY`), drive orders with the private **`uniswapx-tool`** CLI (`UNISWAP_PRIVATE_KEY` for
`submit`), fill **5** within exclusivity (before `decayStartTime`), submit the tx hashes for **manual**
promotion. Do Layers 1–3 exhaustively first; use **minimum-size orders** in Beta to cap real-fund exposure to
≈ gas + tiny notional.

**SDK helpers for self-driven orders** (`@uniswap/uniswapx-sdk`): `V2DutchOrderBuilder` →
`buildPartial()` (swapper sign) → `cosignatureHash()` (sign with our key) → `cosignerData()` / `cosignature()`
/ `build()`; or `CosignedV2DutchOrder.fromUnsignedOrder(...)`. `OrderQuoter` simulates `resolve()` on a fork.

---

## 9. Build phases (code)

Each phase is a reviewable increment; all are committed scope.

- **P0 — Scaffold + codegen.** Vendor UniswapX V2 reactor + Permit2 ABIs → `api/bindings/uniswapx/`; vendor
  `uniswapx-service/swagger.json` → generated poll client; scaffold the `uniswapx` package + `init()` register
  + blank-import from `main`. CGO-free build holds.
- **P1 — Strategy layer promotion.** Promote the strategy layer (contract + registry +
  `default`/`webhook` strategies) out of `internal/solvers/rfq/` into the shared package (§2.1) — a
  pure package move, logic untouched, existing strategy tests carried along; `rfq` adopts the new
  import path. Build the uniswapx candidate `chainreader` (settle the
  merge-multicalls-vs-cached-snapshot hot-path decision, §2.1) and lock the `EXACT_OUTPUT` posture
  (decline vs additive contract extension).
- **P2 — V2 order codec + cosignature** (the riskiest unit, front-loaded). `Serialize`/`Parse`, Permit2 witness
  EIP-712, `CosignatureDigest`/`RecoverCosigner`. Golden tests + SDK-parity tests against committed fixtures.
- **P3 — `UniswapXExecutor.sol`** in the `rfq` repo + forge fork integration test (self-cosign loop). ABI
  vendored here.
- **P4 — Quote webhook.** Huma server (`POST /quote` ≤500ms, `/health`, OpenAPI), independent per-request
  pricing (probe indistinguishable, §4.1), `amountOut:"0"` decline, static-header auth;
  redemption-minus-haircut pricing + gas floor; in-memory store. Unit-tested (pricing golden, httptest incl.
  opposing probe, decline paths).
- **P5 — Ingestion + execution.** `OrderSource` (poll `GET /orders` ≤6 RPS behind the source seam, dedup
  by `orderHash`); validation gates; build callbackData; `executeWithCallback` via txmanager; reconcile;
  fade-aware breaker + `blockUntilTimestamp`. Unit-tested (state machine with fakes, poll dedup, gates).
- **P6 — Packaging + E2E.** Local mainnet-fork harness (Layers 1–3); config profiles; metrics; then Beta
  5-fill qualification (operational, §10).

---

## 10. Operational / non-code TODO list (live)

Tracked operational and onboarding steps — **update as items start/finish/drop** (CLAUDE.md plan-sync).

### 10.1 Confirm with Uniswap (Henrique / Andrey) — blockers to going live
- [x] **Testnet:** RESOLVED — no testnet RFQ *server* (Uniswap quote/order API is mainnet/Beta only, per the
      `uniswapx-tool` CLI), **but** the SDK ships a live **Sepolia Dutch V2 reactor** (`0x0e22B6…BEBcd`, §3.2)
      usable for settlement testing (§8 Layer 2). TODO: confirm with Uniswap whether the Sepolia reactor is
      maintained/safe to rely on, and whether any testnet RFQ server is planned.
- [ ] **Quote-webhook auth scheme** — exact header/secret (no signed scheme in source; FAQ publishes fixed
      RFQ source IPs to whitelist: Beta `3.135.148.114`, Prod `3.138.88.28`).
- [x] **Decline status code:** RESOLVED — default `200`+`amountOut:"0"` (a 404 lands in axios' error path,
      §4.1). Residual TODO: confirm whether the docs' `204` is also accepted in prod.
- [x] **Order delivery:** RESOLVED — order webhooks are **deprecated for new integrations** (Filler FAQ);
      delivery is **poll-only, `GET /orders` at ≤6 RPS**. Residual TODO: confirm poll auth + exact
      rate-limit enforcement at onboarding.
- [ ] **Quote-webhook registration** — how we register our quote URL, filler addr, `chainIds`,
      exclusive-filler status (the `WebhookConfiguration` is S3/Uniswap-provisioned).
- [ ] **Live chain matrix** for RFQ quoting (V3 set as of 2026-06: Arbitrum, Avalanche, Base, BNB, Tempo,
      Unichain, Robinhood Chain; Arc registered June 2026).
- [ ] **Real order flow for our assets** — is there meaningful RFQ flow for *our* vault collaterals on
      mainnet? (Determines whether quoting is worth it before the secondary-DEX hop.)
- [ ] **Rate limits** (req/sec) — not published.
- [ ] **`uniswapx-tool` CLI access** (private GitHub) + any private **reference quoter server**.
- [ ] **Captured real Beta quote-request payloads / a recording** to replay in CI (makes Layer-1 testing
      bit-for-bit faithful instead of schema-faithful).
- [ ] **Runtime OpenAPI** — pull `/v2/uniswapx/docs` spec with `UNISWAP_API_KEY`; re-vendor if richer than the
      GitHub `swagger.json`. Confirm whether a spec exists for the parameterization-api (quote) surface.

### 10.2 Onboarding
- [ ] Submit the quoter intake form: **https://developers.uniswap.org/quoter**.
- [ ] Generate `UNISWAP_API_KEY` at developers.uniswap.org; provision the keeper key (`UNISWAP_PRIVATE_KEY`
      for CLI `submit` only — distinct from our on-chain keeper EOA).
- [ ] Get added to the private `Uniswap/uniswapx-tool` GitHub.
- [ ] Hand Uniswap our **quote-server URL** + **filler (UniswapXExecutor) address**.

### 10.3 On-chain prerequisites
- [ ] Deploy `UniswapXExecutor.sol` (mainnet) + grant our keeper EOA the executor role.
- [ ] Set the executor's reactor address + adapter allowlist; fund the keeper EOA with ETH for gas.
- [ ] Confirm the `LiquidLaneAdapter`(s) we'll source from authorize our executor as filler
      (`isFiller`/`marketMaker` — the adapter validates the swap *actor*, not the caller).
- [ ] Confirm each sourced adapter holds `ALLOCATE_ROLE` on its vault's `UniversalDelegator` (vault-funded
      swaps revert without it) and watch for pending withdrawal-queue sweeps (they zero the vault-funded
      part of `getMaxAssets`).

### 10.4 Beta qualification (real funds, mainnet)
- [ ] Stand up the quote-webhook endpoint reachably (TLS, registered with Uniswap; source-IP allowlist per
      §10.1) and point the poller at the Beta base URL.
- [ ] Drive **minimum-size** orders via `uniswapx-tool`; fill **5** within exclusivity (before
      `decayStartTime`).
- [ ] Collect the **5 tx hashes**; email them to our Uniswap contact for **manual** promotion review.
- [ ] On promotion: flip from Beta base URL to production; widen order sizes per risk.

### 10.5 Deferred (post-v1)
- [ ] V3 order codec + reactor target (Tempo + L2s) behind the same `OrderCodec` interface.
- [ ] Secondary-DEX sourcing in `reactorCallback` for pairs our vaults can't settle.
- [ ] Competitive/win-rate pricing controller (`exclusivityOverrideBps`, time-in-auction, competing fillers).
- [ ] Multi-output (`numOutputs > 1`) orders.
- [ ] Self-funding loops (keep keeper-gas / pay-bid pots fed from profit) if needed.

---

## 11. Resources & references (collected)

### Docs (developers.uniswap.org)
- Architecture — https://developers.uniswap.org/docs/liquidity/uniswapx/concepts/architecture
- Auction types (RFQ + Exclusive Dutch) — https://developers.uniswap.org/docs/liquidity/uniswapx/concepts/auction-types
- UniswapX RFQ flow — https://developers.uniswap.org/docs/liquidity/uniswapx/concepts/uniswaprfq
- Become a Quoter — https://developers.uniswap.org/contracts/uniswapx/fillers/mainnet/becomequoter
- Filling on Mainnet / Filler overview — https://developers.uniswap.org/contracts/uniswapx/fillers/filleroverview
- Deployments — https://developers.uniswap.org/contracts/uniswapx/deployments
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
  https://developers.uniswap.org/contracts/uniswapx/fillers/webhooks
- OpenAPI spec (poll surface) — https://raw.githubusercontent.com/Uniswap/uniswapx-service/main/swagger.json
  (OpenAPI 3.0.0, base `https://api.uniswap.org/v2`, paths `/orders`, `/limit-orders`, `/nonce`)
- UniswapX CLI (Beta driver, **private**) — `Uniswap/uniswapx-tool`

### Endpoints
- Production order API base — `https://api.uniswap.org/v2` (poll `GET /orders`); **gated (needs API key)**.
- Beta API base — `https://beta.api.uniswap.org/v2`; Beta docs (Swagger UI) — `.../v2/uniswapx/docs`; **gated**.

### Internal (this monorepo)
- Sibling solver template — `vault-solver/internal/solvers/rfq/` + [`RFQ-PLAN.md`](RFQ-PLAN.md)
- Strategy architecture (solver-local `strategytypes`/`strategyregistry`/`strategies`, shared
  `internal/liquidlanemath` + `internal/webhook`) — [`strategy-plan.md`](strategy-plan.md)
- Framework conventions — [`../CLAUDE.md`](../CLAUDE.md)
- On-chain adapters/executor live in the sibling `rfq` repo (consumed via `api/bindings/`)

### Beta program facts
- Gate: **5 valid exclusive fills** (before `decayStartTime`) → submit tx hashes → manual promotion.
- Env: `UNISWAP_API_KEY` (all Beta requests), `UNISWAP_PRIVATE_KEY` (CLI `submit`).
- Quote SLA: **≤500ms on all chains**; decline `200`+`amountOut:"0"` (docs mention `204`; never `404`, §4.1).
- Won orders: **poll-only, `GET /orders` at ≤6 RPS** (order webhooks deprecated for new integrations).
- Fade penalty: **15 min**, exponential for consecutive fades; `blockUntilTimestamp` surfaced.
- Tokens: **no allow-list** (there *is* a blocklist — unsupportedtokens.uniswap.org) — open
  `tokenIn`/`tokenOut`; we **decline** (`amountOut:"0"`) anything we can't price. Our fillable universe =
  pairs where `tokenIn` is a token-to-redeem on a whitelisted `LiquidLaneAdapter` **and** `tokenOut` is that
  adapter's vault asset (native-ETH `tokenOut` maps to WETH, §7) — narrow by construction until the
  secondary-DEX hop.
