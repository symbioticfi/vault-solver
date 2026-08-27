# vault-solver — RedStone OEV solver (plan)

The **`redstone-oev`** solver: an off-chain bidder for RedStone Atom OEV auctions. The built-in
`default` strategy captures price-driven liquidations on **Morpho Blue** and exits the seized RWA
collateral through **one** Symbiotic LiquidLane adapter (one vault) in the same atomic transaction. The
solver boundary itself is generic: the solver owns the callback address, a strategy returns bid amount
and callback-specific `operationData`, and the solver signs the outer RedStone Executor payload. It
follows the framework boundary and conventions in [`../CLAUDE.md`](../CLAUDE.md). §6 records the
verified ground truth (Executor source, wire schemas, Sepolia testbed, proven live liquidations) the
design rests on.

---

## 1. What the solver does

RedStone Atom inverts MEV liquidations: instead of publishing a price update and letting searchers race,
RedStone runs an off-chain ~400 ms WebSocket auction among approved solvers for the right to be the
liquidator. The winning solver's signed payload is bundled **with the price update and the liquidation in
one atomic transaction**, submitted by RedStone's auctioneer.

Per auction tick, end to end:

1. **Auction frame** arrives over WSS (`oev/liquidations`). We consume the **prices only** — the price
   push is oracle-scoped, so once an auction settles for an oracle every borrower underwater at that
   price is liquidatable by our callback, not just the positions RedStone listed (§3.1). We target our
   own independently-discovered underwater set; the frame's `positions[]` are ignored.
2. **Hot path** (`DecideBid` → envelope check → sign, sync, in-memory except configured webhook — budget ≈
   400 ms minus WS RTT): build the solver envelope snapshot, delegate bid/skip selection to the configured
   strategy, check the returned execution envelope (`bidAmount`, `operationData`), run the O(1)
   pre-bid gates, sign the **EXECUTOR_V6** EIP-191 payload, and reply `{"op":"solve", …}`. The default
   strategy is the current Morpho strategy: it builds the candidate set from our tracked at-risk positions,
   recomputes health at the settlement price with shared Morpho math, sizes seize/exit legs, signs the
   Morpho callback auth, encodes callback `operationData`, and selects by after-cost net profit with `gas:` or gross profit without it.
3. **Settlement** (on-chain, RedStone submits — not us): the Executor verifies our signature + nonce,
   applies the price update to the oracle, then calls our callback's `liquidate(bid, solver,
   operationData)` → per leg `Morpho.liquidate(…)` → Morpho pushes the seized collateral to the callback
   and invokes `onMorphoLiquidate`, where the callback sells the WHOLE seizure through its one immutable
   `LiquidLaneAdapter` in a single `swap` after recomputing current min-out, then approves Morpho's
   repayment pull — then the Executor calls `payBid(bid)` and the callback pays it in native.
   The Executor **catches** a callback revert (`LiquidationFailed(solver, nonce)`): the nonce is still
   consumed and the gas liability still debited from the deposit — the liquidation just doesn't settle
   (§6.2). A price-update revert, by contrast, reverts the whole tx.
4. **Bookkeeping** (background ops loop): a periodic poll refreshes Executor and adapter state; our
   `liquidation-result` also requests the same refresh asynchronously without blocking the WS reader.
   The circuit breaker is fed by the WS `liquidation-result` push (a `success:false` frame for our callback →
   `recordFailure`); we are a state-reading bot and do **not** run chain log scans. Realized profit is the
   callback's loan-token balance, not event accounting.

Two roles, two pots (see §6.2 for the verified Executor mechanics):

- **Signer EOA's deposit on the Executor** (≥ `MIN_DEPOSIT` = 0.00001 ETH on Sepolia) — a *rolling gas
  prepayment*: after every settlement, win or revert, `(gasUsed + 35k) × tx.gasprice` is deducted and
  paid to the auctioneer. This gas debit is **independent of the auction** (the winner is decided purely
  by bid amount) and is **not reverted if the deposit underpays** — so it does NOT change the signed bid
  amount. The pre-bid solver gate only checks the Executor `MIN_DEPOSIT` floor; callback/gas economics are
  strategy-owned. With `gas:` enabled, the default strategy requires converted bundle profit to cover its
  route-aware gas estimate, bid, and configured margin; without it, native deposit headroom is still enforced
  but bundle selection uses gross loan-token profit. The operator tops the deposit up out-of-band.
- **Callback contract's native balance** — pays the bid via `payBid` (forwarded to RedStone's
  collector); **owner-refilled** out-of-band. Liquidation profit accumulates in the callback's ERC-20
  balance; the owner withdraws it (`withdrawERC20`). The bot never self-funds.

**One bot, one vault, one loan token, one venue (Morpho).** The `SymbioticOevSolver` pins a single
immutable `LiquidLaneAdapter` at construction, so the solver serves exactly that adapter's vault. A market is
tracked only when its loan token equals that adapter's vault loan token (resolved on-chain). A single
auction is answered by ONE bid bundling every gas-fit leg positive under the configured economic mode (one loan token, no per-token
grouping). **One solver runs per vault** (spec §11: RWA curators set per-vault
discounts, so swaps can't aggregate across vaults): a second vault/loan token is a second
`SymbioticOevSolver` + a second process.

---

## 2. Architecture

A self-contained `internal/solvers/redstoneoev/` implementing `solver.Solver` — **no framework edits**
(CLAUDE.md modularity rule):

- **`Solver` owns the execution envelope; strategy owns callback-specific execution.** The solver keeps
  WS lifecycle, auction/result routing, Executor state, in-flight auction reservations, breaker gates,
  the Executor deposit floor, EXECUTOR_V6 signing, and the final solve payload. The
  configured strategy gets a compact `BidInput` with the solver-configured callback, Executor deposit
  and its protocol minimum, and returns
  `skip` or final `bidAmount + operationData`. `default` is the current Morpho liquidation
  strategy; `webhook` is an external trusted decider over JSON. The solver checks only the generic
  execution envelope before signing: unsafe skip payloads, non-positive bids, empty `operationData`,
  and insufficient Executor deposit fail closed. Callback balances, gas
  profitability, and operation-specific funding assumptions are strategy-owned.
- **Settlement truth comes over WS, not from logs.** The breaker's failure feed is RedStone's
  `liquidation-result` push: a `success:false` frame whose `liquidator` is our callback → `recordFailure`,
  tripping the breaker after N in the window. We gate on `liquidator == callback` (same won-detection as
  `auction-result`) because the frame arrives on both the broadcast `oev/liquidations` and the
  callback-scoped `oev/notify/<callback>` subscription, so a result may belong to another solver. We are a
  state-reading bot, **not** a log-indexer: there is no `FilterLogs` scan. A result for our callback wakes
  the solver-owned Executor/adapter refresh loop; it is not forwarded into the strategy interface. If a
  settlement receipt is available from RedStone's `txHash`, the bot decodes callback `LegResult`/`PayBidResult` logs from that
  receipt for diagnostics. Realized profit is read off the callback's loan-token balance / balance sheet,
  not event accounting.
- **`Run(ctx)`** owns the resilient WS client (connect with `x-api-key`, subscribe `oev/liquidations` +
  `oev/notify/<callback>` for the solver-configured callback, reconnect with backoff + jitter, ~7 h proactive
  rotation, staleness watchdog), the hot-path handler, the strategy's refresh loops, and the Executor-state
  ops loop. It joins every background loop on shutdown (`sync.WaitGroup`) so no goroutine outlives `Run`.
  Caches are immutable snapshots swapped atomically (`atomic.Pointer`), read lock-free on the hot path.
- **The solver sends no transactions** — RedStone's auctioneer submits the settlement tx; Executor deposit
  management is out-of-band. Its registration sets `ExternallySubmitted`, so an OEV-only process does not
  require `txManager.maxFeeGwei` and neither initializes nor starts the nonce lane. `deps.TxManager` remains
  unused and the example needs no `txManager` section; a mixed process still starts the one shared manager for
  its transaction-sending solvers.
- **`deps.Signer` is the EXECUTOR_V6 signer.** The bid digest is `keccak256(abi.encode("EXECUTOR_V6",
  chainId, callback, keccak256(operationData), bidWei, nonce, maxTxGasPrice))` wrapped in EIP-191
  (`personal_sign`), signed via `Signer.SignHash`. The signer EOA **is** the wallet holding the Executor
  deposit (the Executor recovers the signer and debits *its* deposit/nonce). A KMS split is later
  hardening, same as 3F/RFQ.
- **On-chain reads use latest-state `chain.Multicall` in background loops only** — nothing on the hot path
  touches the network except the final `ws.Send`. The solver reads envelope state (Executor
  deposit/nonce/lock and latest header gas limit) plus the configured adapter snapshot (vault/loan token,
  redeemable collateral set, per-collateral `getMaxRate`/`getMaxAssets`, route-liquidity balances, and
  callback filler authorization) and passes it in `BidInput`. Shared gas-oracle facts live in the solver
  cache; production Morpho state and callback-specific data live
  in the default strategy. The Sepolia test monitor is the only path that reads Morpho
  `market()`/`position()` on-chain, over explicit test seeds. `chain.Multicall`
  itself packs `aggregate3` + does its own
  `eth_call` + unpacks via the v2 Multicall3 binding — every binding is abigen --v2 now (no v1 path remains).
- **Validate-everything, fail closed.** Auction frames are external input that drives funds: per-frame
  count is bounded; a paused/dry/unserved vault yields no quote → no bid; malformed fields skip the leg,
  never panic. Solver-owned pre-bid gates cover the breaker, fresh Executor state, Executor deposit floor,
  locked signer, and the generic execution envelope. Strategy-owned gates cover snapshot block epoch,
  callback balance, configured bundle profitability, and the adapter's `isFiller` gate. The default strategy computes the bid **off-chain** from the
  configured `strategy.config.bid.bidEth` floor and optional `strategy.config.bid.totalBundleProfitBps` (spec §8); optional solver-owned `maxBidWei`
  caps the final signed bid for every strategy. The contract carries no on-chain bid cap.
- **Metrics** on the shared registry (`deps.Metrics.Registerer()`, nil-safe): auctions/bids/wins/
  failed-liquidations counters, a `skips_total{reason}` vector, a hot-path latency histogram, and deposit
  gauges. The breaker halts bidding after N failed liquidations in a rolling window,
  and immediately on a `blacklisted` frame.
- **Bindings.** The RedStone `Executor`, `IMorpho` (subset), `IAdaptiveCurveIrm`, `IOracle`, and our
  `SymbioticOevSolver`, and `AggregatorV3` feeds are **abigen --v2** bindings under `api/bindings/oev/*`
  (vendored ABIs in `api/abi/`: the external contracts hand-vendored, the Executor ABI mirroring the
  verified Blockscout source — §6.2; `SymbioticOevSolver` is the OEV callback contract from rfq `src/oev`),
  following the repo's `BINDINGS_V2` pattern (vendor → generate → commit). The reader builds every
  Multicall3 sub-call and decodes every return/event blob through the bindings' typed `PackXxx`/`UnpackXxx`
  (and `UnpackXxxEvent`), so an ABI change breaks the build at the call site — matching the rfq reader. The
  LiquidLane read-side binding and the ERC-4626 vault binding live in their neutral shared groups
  (`api/bindings/liquidlane/adapter`, `api/bindings/erc4626`), reused by RFQ and OEV.
- **Config**: all addresses / URLs / caps from the YAML `solvers[].config`; the API key via `apiKeyEnv`
  and the signer key via the framework `signer.keyEnv` (`os.Getenv` at point of use — never in the
  struct, never logged).

### Component map (file → responsibility)

The shared Morpho package is intentionally small: `internal/morpho` holds only reusable Morpho Blue math and
state accounting. The GraphQL wire binding is generated from the full vendored Morpho schema plus
`api/graphql/morpho/operations/*.graphql` into `api/morphographql`, with
`api/graphql/morpho/operations.json` recording the exact generated query strings. The hand-written Morpho API
adapter is OEV-local because it parses directly into the OEV monitor snapshot.

| File | Responsibility |
|---|---|
| `solver.go` | `Register`, factory, `Run` (loops + join), `handleAuction` → `buildBid`, ops loop, the head-stable Executor cache (`cachedState`/`stateCache`), Executor deposit floor, and outer EXECUTOR_V6 signing |
| `strategy.go` | OEV strategy factory/construction, lean `BidInput` construction (including solver-owned callback + adapter snapshot), and generic `BidOutput` validation |
| `strategies/registry.go` | OEV-local strategy registry/factory; built-ins self-register with policy metadata (for example, whether a solver bid cap is required), and custom strategies can register by name |
| `strategies/types/` | OEV strategy input/output/interface and webhook JSON wire encoding (lower-camel, decimal strings, strict output decode) |
| `strategies/default/strategy.go` | Default Morpho strategy runtime: owns Morpho monitor lifecycle, snapshot staleness, candidate scoring, bundle pricing, callback auth signing, operationData encoding, and bid/skip output |
| `strategies/webhook/` | OEV webhook strategy adapter over the shared `internal/webhook` JSON client |
| `strategies/default/candidates.go` | auction frame → `[]evalItem` (production and Sepolia test monitor both use the auctioned frame price) + default strategy candidate sizing (`candidates` → `sizeLeg`); the candidate set is our own tracked positions (`workerCandidates`) — the frame's pushed positions are not consumed |
| `strategies/default/bundle.go` | single-token bounded bundle selection: after-cost net search with `gas:`, gross loan-profit search without it, plus route-aware gas limits in both modes |
| `strategies/default/sizing.go` | adapter pricing primitives (`swapOutFor`/`collForBudget`) and `sizeLeg`, the per-candidate single-swap leg sizing/decision over the shared Morpho math |
| `strategies/default/chainreader.go` | default-strategy Morpho/test-state and callback-balance reads; gas feeds are solver-owned shared facts |
| `strategies/default/operationdata.go` | ABI-encode Morpho callback `operationData`: auction auth, capped max-seize legs, loan-denominated profit floors, and callback-auth signature |
| `internal/morpho/math.go` | shared Morpho Blue math: health, LIF, share/asset conversions, Taylor accrual (exact big.Int rounding) |
| `internal/liquidlane/gas` | shared optional Chainlink token/native facts, route prediction, and adapter swap gas units |
| `strategies/default/monitor.go` | Morpho API snapshot, atomic hot-path state, and adapter-scoped market filtering |
| `strategies/default/morphoapi.go` | OEV-local adapter over generated Morpho GraphQL operations: `markets` returns adapter-scoped market state (§3.4), `marketPositions` returns at-risk position state capped at `maxTrackedPositions` (§3.2) |
| `api/morphographql` | generated Morpho GraphQL binding from vendored schema + explicit operation documents |
| `chainreader.go` | solver-owned Executor/adapter accounting plus optional shared gas-oracle snapshot |
| `reservations.go` | in-flight auction reservation + pending-auction snapshot + auction-id de-dup (`seenAuctions`) |
| `wsclient.go` | resilient WS client: reconnect/backoff/jitter, ~7 h rotation, heartbeat, subscribe replay |
| `wsmessages.go` | wire types pinned to RedStone's zod + a captured auction frame, including auction identity/key hashing |
| `eip191.go` | EXECUTOR_V6 digest + EIP-191 `SignBid` via `Signer.SignHash` (golden + parity tests) |
| `noncestore.go` | strictly-ascending nonce high-water mark, reconciled with the on-chain getter |
| `breaker.go` | failed-liquidation rolling-window breaker + `blacklisted`-frame halt |
| `metrics.go` | nil-safe Prometheus collectors on the shared registry, labeled by configured strategy |
| `config.go` | typed, validated `parseConfig`; solver-owned config plus nested `strategy.name/config` |

---

## 3. Candidate discovery & the shared health core

The candidate set is **purely** our own independently-tracked at-risk positions (Morpho API + config
seeds). We know each market's oracle/params, so we compute liquidatability ourselves over our own set,
evaluated at the auction's prices; the frame supplies **prices only**. Every candidate is a
`Candidate{marketId, borrower, market, position}` evaluated through the **one** shared path — so
liquidatability, accrual, sizing, and the profit floor are computed identically. Market state (totals +
IRM rate for accrual) and the position both come from our own monitor snapshot; no pushed position is
trusted to size a leg.

### 3.1 The frame supplies prices, not positions

`operationData` is **ours** — we sign it and the auctioneer never decodes it (verified: it sorts bids by
`bid` only; §6.3 won an auction with arbitrary `operationData`). The price push is **oracle-scoped**, not
position-scoped, so once an auction settles for an oracle **every** borrower underwater at the pushed
price is liquidatable by our callback. We exploit exactly that: ignore RedStone's `positions[]` and
target our full independently-discovered underwater set, evaluated at the frame's pushed price. The one
dependency we keep: an auction must fire and push the price for that oracle.

### 3.2 Morpho API snapshot

In production (`strategy.config.morphoApiUrl` set on the default strategy), the monitor snapshots **all Morpho data from the Morpho GraphQL API**:

- `markets(where: {loanAssetAddress_in, collateralAssetAddress_in, chainId_in})` discovers markets served
  by the configured adapter's on-chain loan token and redeemable collateral set.
- The same market query returns immutable params plus accrued market state (`borrowAssets`,
  `borrowShares`, `supplyAssets`, `supplyShares`, `timestamp`, `blockNumber`, optional `price`).
- `marketPositions(orderBy: HealthFactor, orderDirection: Asc, where: {marketUniqueKey_in,
  healthFactor_lte: discoveryMaxHealthFactor})` returns the at-risk position state (`borrowShares`,
  `collateral`) for those markets. `maxTrackedPositions` is the logical cap; the OEV Morpho client
  keeps each live request within observed API limits (`marketUniqueKey_in` chunks of 100, pages of 1000)
  and then sorts/truncates the combined result by health factor.

The API is not trusted blindly. Each market is locally validated by re-deriving the Morpho market id from
`(loanToken, collateralToken, oracle, irm, lltv)` and by checking the adapter pair
(`loan == adapter.vault().asset()`, collateral in `tokensToRedeem`). Malformed numbers, zero addresses, missing
collateral, bad ids, and transport/GraphQL errors fail closed and keep the prior snapshot. The hot bidding
path still has **no network I/O**: it reads this immutable snapshot, applies local Morpho math at the
auction price, replays same-market liquidations, and builds calldata.

The only adapter reads left in production monitor refreshes are the static scoping data needed to query
Morpho (`adapter.vault().asset()` and `tokensToRedeem`). Per-auction strategy input carries the live adapter
exchange snapshot instead: `paused`, per-collateral `getMaxRate`/`getMaxAssets`, token decimals, vault
`freeAssets`/`withdrawable`, and acquire balances. That lets any strategy price and capacity-check the
configured LiquidLane route without doing its own adapter reads on the bid path.

`strategy.config.morphoApiUrl` is a production hard requirement for the default strategy. The Sepolia
harness is the only exception: `strategy.config.testMonitor.markets` and `.positions` provide a fixed
seed set, and the bot reads Morpho `market`/`position` state on-chain from the callback's `MORPHO()`
getter. Public `api.morpho.org` does not index the custom Sepolia deployment.

### 3.3 Snapshot concurrency model

The monitor exposes one `atomic.Pointer[snapshot]`, written by **exactly one goroutine** (the monitor
run loop builds a fresh, never-mutated snapshot and swaps it) and read concurrently by the WS goroutine
via `candidates()`. Readers never lock — they `Load` the current pointer. The Executor-state cache
(`cachedState`/`stateCache` in `solver.go`) and the nonce high-water mark follow the same single-writer /
lock-free-read model.

Every mutable snapshot records exactly one source block and that block's timestamp. API markets from a
different `state.blockNumber` are dropped instead of being mixed into the same snapshot; the test monitor
uses the latest RPC header block. The default strategy hot path fails closed with `stale_epoch` when a non-empty snapshot
has no block tag/timestamp or when its block timestamp is more than a small Ethereum/Sepolia block-time
window behind the auction timestamp. This allows ordinary one-block monitor/API lag without letting a
stuck API cache bid indefinitely. The solver ops loop is separate: it refreshes Executor state, latest
header gas limit, configured adapter snapshot, and optional gas-oracle facts on `opsPollMs` and after our `liquidation-result`.
`Run` requires one coherent initial snapshot before opening the auction stream; configured gas-token
coverage or oracle-read failures therefore fail startup instead of leaving a ready process that cannot bid.
The LiquidLane adapter is solver-owned config; its exchange price/capacity/liquidity snapshot is strategy
input. The default strategy uses that same cached adapter snapshot to scope its Morpho monitor and loan
decimals instead of maintaining a second adapter reader. It owns only its callback-balance refresh.

**Stale-state gates.** Every background-refreshed cache the hot path reads stamps a wall-clock `updatedAt`
**only on a successful store**. The solver owns the ops-loop `cachedState` (Executor accounting, adapter
snapshot, gas limit, and optional gas prices) and skips with `executor_state_stale` when it exceeds
`intervals.executorStateMaxAgeMs`. The
default strategy owns its monitor snapshot (Morpho markets/positions) and callback-balance state, and returns `stale_state` for its own
stale caches. A loop that keeps failing while serving its prior data stops bidding instead of running on
arbitrarily old state. Startup config validation enforces each owner separately:
`intervals.opsPollMs < intervals.executorStateMaxAgeMs` in the solver, and
`strategy.config.monitorPollMs < strategy.config.maxStateAgeMs` in the default strategy.

### 3.4 Market scope

The tracked Morpho markets are **discovered from the solver-owned adapter snapshot**, not configured or
read again by the default strategy. The pair is fully on-chain-derivable from the pinned adapter: loan =
`adapter.vault().asset()`, collateral = the adapter's redeemable token set (`getTokensToRedeemLength()` +
`tokensToRedeem(i)`). The API query uses that pair and validates returned ids locally. The auction
frame's pushed positions are never a discovery source.

---

## 4. Economics & sizing

### 4.1 The three prices

Sizing uses the right price at each step; conflating them is the easiest way to bid into a loss:

1. **Market price** — what we expect on the oracle *at settlement* → drives liquidatability and what we
   owe Morpho per unit seized (with local accrual). Production uses the **auctioned** frame price (the
   auctioneer applies the frame's pushed price atomically before our callback). The Sepolia test monitor
   uses cached `oracle.price()` because the dev endpoint does *not* apply the frame price (§6.6). Sizing
   against the wrong one reverts `InvalidSwapRate`.
2. **Adapter redemption rate** (`getMaxRate` = the adapter oracle price × (1 − curator `minDiscount`),
   read on-chain + cached, decimals-correct) → the loan token we expect for the seized RWA, minus an
   extra `swapHaircutBps` safety cushion. Off-chain sizing uses it to estimate expected output; the leg
   encodes only `maxSeizeAssets`, and the no-preview callback recomputes current output in
   `onMorphoLiquidate` before approving repayment.

Gas is **not** a sizing input and not part of the bid amount: the auction winner is decided purely by bid,
and the `(gasUsed + 35k) × tx.gasprice` liability is debited from the deposit AFTER settlement, independent
of the auction, and is not reverted if the deposit underpays (§6.2). It is still real cost, so with the
optional top-level `gas:` block the default strategy uses
`strategy.config.bid.bidEth` as the bid floor but selects live bundles by
`loanToNative(bundleGrossLoan, rate) - gasNative(bundle) - bidNative(bundle)` and only bids when the selected bundle clears
`ceil(bidNative * strategy.config.bid.minBundleProfitBidBps / 10000)`. `bidNative(bundle)` is
`max(strategy.config.bid.bidEth, loanToNative(bundleGrossLoan, rate) * strategy.config.bid.totalBundleProfitBps / 10000)`.
The rate comes from the shared cached gas-oracle facts; the hot path never does feed I/O. Without `gas:`,
selection uses gross loan-token profit, the fixed `bidEth`, and one-base-unit signed profit floors.
`gasNative(bundle) = gasUnits(bundle, cachedPredictorState) × maxTxGasPriceWei`.
`maxTxGasPriceWei` is also the value signed into EXECUTOR_V6; using one ceiling avoids winning an auction
with a gas cap too low for RedStone's settlement tx.
When `gas:` is enabled, the route-aware gas estimate is converted to loan units at `maxTxGasPriceWei` and
signed into each leg as `minProfit`; the callback does no ETH↔loan conversion. Morpho liquidation is kept as a fixed per-leg component because fork measurements
showed only about 3k gas of branch spread across the observed partial/full sizing cases, while the LiquidLane
swap leg is classified as acquire-only, allocate+sync, or deallocate+allocate+sync from cached adapter/vault
getter state. If state is missing or insufficient, the leg falls back to the conservative unknown route.
Both modes retain gas-limit fitting, Executor deposit headroom/reservations, and callback bid funding.

**Profit = swap proceeds − repayment** (in the loan token). When the auctioned price crashes below the
adapter's NAV rate, a leg captures both the liquidation bonus *and* that gap. Off-chain `sizeLeg` only takes
a leg when expected adapter output exceeds repayment. With `gas:` enabled, the bundle must also clear gas,
dynamic bid, and `minBundleProfitBidBps` in `selectNetBundle`. On-chain, the no-preview callback executes each signed
`maxSeizeAssets` leg fail-soft through Morpho, sells the actual seizure, and reverts that leg unless realized
output covers Morpho's actual repayment plus the signed leg `minProfit`. After all fail-soft legs, the
callback reverts unless realized total loan profit clears the signed `minBundleProfit`, and only then enables
`payBid`.

**One sizing strategy: MAX.** By default, target 100 % of collateral and let the debt/liquidity clamps size
it down. This captures bad-debt opportunities: the unit economics are still `swap proceeds − repayment`,
but a full-collateral bad-debt liquidation can have more total profit because it does not leave the final
collateral slice behind. If settlement routing ever has issues with full-collateral cases, set
`strategy.config.sizing.allowFullLiquidation: false`; that hard-disables full seize and uses the fixed 90 % partial fallback.
Profit is **linear in seize**, so this extracts the most per won auction while keeping an explicit kill
switch.

### 4.2 One liquidation, one swap

The contract is single-adapter: it seizes the collateral once and sells the WHOLE seizure through its one
immutable `LiquidLaneAdapter` in a single `swap`. A leg therefore carries one capped `maxSeizeAssets`, not a
per-vault split or a stale min-out.
`sizeLeg` produces one leg per liquidatable candidate:

- The seize starts from either all collateral (`allowFullLiquidation: true`) or the fixed 90 % partial
  fallback (`false`), then is CLAMPED by two bounds: the borrower's full debt (`maxSeizeForFullDebt` — so a
  small-debt / large-collateral position can't over-seize and revert the Morpho `borrowShares` underflow)
  and the adapter's `getMaxAssets` redemption liquidity
  (`collForBudget` — so the expected output never exceeds what the vault can allocate and revert
  `InsufficientAllocate`).
- Expected output is computed at the adapter's discounted `getMaxRate` minus `swapHaircutBps` for
  profitability and route/budget checks only. It is not encoded into `operationData`; the callback
  recomputes the current amount out before deciding whether to execute.

### 4.3 Bundling

With `gas:` configured, `selectNetBundle` selects live bundles by after-cost net profit:
`loanToNative(sumProfitLoan, rate) - gasNative(bundle) - bidNative(bundle)`. It uses a bounded beam search over
deterministic gross-ordered subsets, so a locally best leg cannot block a lower-gross subset with better
shared-liquidity/gas economics. Every scored leg is expected-positive before gas; the final bundle must clear
gas, bid, and `strategy.config.bid.minBundleProfitBidBps`. Without `gas:`, selection uses gross profit but
still applies the same gas envelope and route-capacity constraints.

Bundle depth is not configured. Each trial bundle must fit the gas envelope. Adapter amount/rate math
stays in the default strategy sizing path, because it is tied to Morpho liquidation economics here.
Adapter route prediction and swap gas units live in `internal/liquidlane/gas`, while RedStone settlement
overhead stays in the default strategy:
`predictedGas <= 85% * min(latestHeader.gasLimit, observed RedStone settlement cap 2M)`. Predicted gas is fixed bundle gas
(`100k` callback base + `35k` Executor debit surcharge + `40k` per updated RedStone feed), plus route-aware
first-leg and marginal gas. The current no-preview fork calibration is: acquire `300k` first / `140k`
marginal, allocate `530k` first / `350k` marginal, deallocate `650k` first / `450k` marginal, unknown
`850k` first / `650k` marginal. Beam search is bounded by candidate
count `N`, gas-fit depth `L`, and fixed width `W = 64`. It first sorts candidates in `O(N log N)`, then each
depth evaluates at most `W*N` extensions and sorts at most `W*N` trial states, so the practical bound is
`O(N log N + L*W*N*log(W*N))` time with `O(W*N)` transient states per depth. With
`maxTrackedPositions=10000`, `W=64`, and the observed 2M RedStone settlement cap, `L` is about 2 worst-route
legs or 10 acquire-only legs before other filters.

A per-collateral cumulative `getMaxAssets` cap skips a leg that would over-commit a collateral's shared
adapter liquidity (several same-collateral legs would otherwise revert `InsufficientAllocate` on settlement).
Multiple borrowers from the same Morpho market are allowed only through sequential local replay:
after each candidate leg, the selector applies Morpho's seize-driven `liquidate` accounting to the simulated
market and re-sizes the next same-market candidate against that post-state. Independent precomputed same-market
legs must never be copied directly into `operationData`.
The mode is configuration-driven in both live and dry-run operation. `maxTxGasPriceWei` remains the hard
ceiling for the `tx.gasprice` signed into EXECUTOR_V6 and the native gas-reservation assumption.

**The bid has a floor and optional profit share.** With `gas:` enabled, the solver bids
`max(strategy.config.bid.bidEth, grossProfitNative * strategy.config.bid.totalBundleProfitBps / 10000)`: the floor keeps thin auctions simple,
while without it the bid is exactly `bidEth` and both bps knobs must be zero. Gas-enabled bundles must clear
the after-cost profitability check; both modes require the callback to hold the bid native and the Executor deposit to cover
the Executor `MIN_DEPOSIT` floor plus the default strategy's predicted settlement gas. Every
`auction-result` frame is logged with the winning `bid` and whether we won, so a win-rate controller can
later consume those results; the bid remains solver-bounded off-chain (spec §8).

---

## 5. Settlement, reservation & safety

- **In-flight reservation.** A sent bid records `reservedBid{nonce, at, auctionID, callback}` as lifecycle
  state. The solver passes pending auction IDs to the strategy; strategy-specific duplicate-position,
  callback funding, and gas risk avoidance stay inside the strategy because the solver no longer knows
  callback operation semantics or Morpho legs. The default strategy keeps its own per-auction bid, predicted
  gas, and `(market, borrower)` reservations. It can continue bidding independent positions after a missed
  result frame without overcommitting callback or Executor funds. A resolved reservation remains charged
  until callback balance has been refreshed after resolution, so a winning bid cannot reuse stale balance.
  Result frames release the reservation immediately when we lose `auction-result` or when our
  `liquidation-result` arrives. Nonce reconciliation and `reservationTTL` are backstops for missed frames.
- **Adapter authorization (one gate, fail closed).** The default strategy treats the configured adapter as
  usable only if its own swap-caller predicate passes (`callback == marketMaker() || callback == owner() ||
  isFiller(marketMaker, callback)`) — so the bot never bids a leg whose swap would revert `InvalidCaller`.
  The single immutable `LiquidLaneAdapter` is pinned at construction, so this curator `setFiller` gate is the
  only adapter-routing preflight.
- **Other pre-bid gates**: solver Executor cache age (`executor_state_stale`, vs
  `intervals.executorStateMaxAgeMs`), default-strategy cache age (`stale_state`, vs
  `strategy.config.maxStateAgeMs`), snapshot block epoch (`stale_epoch`), duplicate-auction
  de-dup + `timeoutMs` drop, optional operator bid cap (`bid_cap`), configured after-cost profitability (`gas_unprofitable`), Executor deposit floor
  (`deposit_low`) + callback-native funding. The default bid is bounded **off-chain** by
  `strategy.config.bid.bidEth` plus optional `strategy.config.bid.totalBundleProfitBps`, and the solver can cap the final signed bid with `maxBidWei`.
  There is no on-chain bid cap. A deposit below MIN_DEPOSIT raises an `oev_deposit_below_floor` gauge + error log.
  The operator tops up that deposit and the callback's payBid native balance out-of-band.
- **No self-funding.** The bot moves no funds outside the signed settlement: the callback's payBid native
  pool is owner-refilled out-of-band and the signer's Executor gas deposit is topped up out-of-band. The
  bot holds a signing key but its only fund-moving action is the signed bid the Executor settles.

### The `SymbioticOevSolver` contract

The on-chain settlement contract is the OEV `SymbioticOevSolver` in rfq `src/oev/`: a single-adapter router
(Morpho-only on-chain) whose constructor pins ONE immutable `LiquidLaneAdapter` and one `AUTH_SIGNER`.
The Executor calls `liquidate(bid, solver, operationData)`. The callback verifies the default-strategy
callback auth (`auctionKey`, `bidAmount`, `minBundleProfit`, `deadline`, and capped legs), marks
`auctionKey` used, then processes legs fail-soft. The deadline is solver-local replay protection for the
callback auth (`now + strategy.config.bid.authTtlMs`, default 60s); the auction's sub-second `timeoutMs` remains an
off-chain send gate.

Each leg carries `marketId`, `borrower`, `maxSeizeAssets`, and a loan-denominated `minProfit`.
`SymbioticOevSolver` is a no-preview callback: it reads immutable Morpho market params, clamps the signed
max seize by the borrower's live collateral, calls `Morpho.liquidate`, and lets `onMorphoLiquidate`
validate economics from actual `repaidAssets`. Morpho is invoked with `try/catch`, so a
stale/healthy/reverting leg emits `LegResult` and the bundle can continue. During `onMorphoLiquidate`, the
callback sells the seizure through the immutable `LiquidLaneAdapter` at the current adapter rate, approves
Morpho repayment, and requires the realized gain to cover repayment plus the leg's signed `minProfit`. A
skipped or reverted leg contributes **zero** to the bundle profit — the bundle gate
(`BundleResult.bidAuthorized`) compares only the successful legs' realized profit against the signed
`minBundleProfit`.

The payBid native pool is owner-funded (`receive`) and owner-withdrawable (`withdrawNative`/`withdrawERC20`).
`payBid` is gated on the bundle outcome: it pays the authorized bid amount only when `liquidate` cleared the
`minBundleProfit` gate; otherwise (gate failed, or no matching authorized liquidation) it emits
`PayBidResult(..., false)` and pays nothing — which the Executor records as `BidUnderpaid`, an event
RedStone counts toward deposit slashing / blacklisting. A skipped leg in a multi-leg bundle can therefore
convert a profitable settlement into a `BidUnderpaid` strike (see §10).

Settlement events emitted on-chain (`LegResult` and `PayBidResult`; the Executor's
`LiquidationFailed(solver indexed, nonce)`) document settlement and post-mortem reasons. The generic solver
does not decode callback-specific receipt logs; it only attributes actual gas from the receipt. The breaker
is still fed by the WS `liquidation-result` push (§2).

---

## 6. Verified ground truth

Primary sources: the RedStone OEV docs; `redstone-finance/redstone-evm-examples/oev`; the **verified
Executor source** (Blockscout Sepolia, impl behind proxy `0xfdFB1862…EBd`); Morpho Blue
(`morpho-org/morpho-blue@main`); and live Sepolia state. Deployed-address manifests are
operator-maintained outside this repo.

### 6.1 Wire protocol

- Connect: WSS + `x-api-key` header. ≤30 connections/key; server pings after 120 s idle; connections
  force-closed ~8 h (rotate proactively at ~7 h).
- Subscribe: `{"op":"subscribe","topic":"oev/liquidations"}`, `oev/feeds`
  (flat feed auctions are observed but not used as liquidation triggers), and
  `oev/notify/<callback-lowercase>`;
  re-send after every reconnect.
- **Auction frame** (confirmed from 35+ live Sepolia frames):
  `{"op":"auction","id":"<uuid>","timestamp":<ms>,"timeoutMs":<ms>,"payload":{positions,prices}}`. The
  live timing field is **`timeoutMs`** (not the docs example's `durationMs`). `positions[]` carries market
  id, borrower, token addresses + decimals, and pushed balances; `prices{oracle → 1e36-scaled string}`.
  The WS type keeps only `prices`: `positions[]` is ignored because the monitor's Morpho snapshot is the
  position source of truth (§3.1–§3.2).
- **Solve frame**: `{"op":"solve","id":"<auction id>","data":{"bid","nonce","operationCallback",
  "operationData","liquidationSig","maxTxGasPrice","borrowers"?}}` — `bid` is a decimal **ether** string;
  bids are sorted descending, highest wins, late replies discarded.
- **Notify frames**: `auction-result {bid, liquidator}` (we won iff `liquidator == callback.toLower()`),
  `liquidation-result {success, txHash, …}`, `blacklisted {liquidator, msg}`.
- Frame schemas are vendored verbatim from RedStone's zod at
  [`../openapi/redstone-oev-ws.zod.ts`](../openapi/redstone-oev-ws.zod.ts); Go structs are pinned to it
  by tests. The on-chain half follows vendor-and-generate (the Executor ABI from the verified source).

### 6.2 Executor semantics (from verified source)

```
execute(callback, operationData, liquidationSig, bidAmount, nonce, maxTxGasPrice, priceAdapter, priceUpdate)
  onlyAuctioneer; require(tx.gasprice <= maxTxGasPrice); settlement tx currently arrives with about 2M gas
  solver = ecrecover(EIP-191(keccak256(abi.encode("EXECUTOR_V6", chainid, callback,
                              keccak256(operationData), bidAmount, nonce, maxTxGasPrice))))
  require(!locked[solver]); require(nonce > nonces[solver]); nonces[solver] = nonce
  require(deposits[solver] >= MIN_DEPOSIT)              // 0.00001 ETH on Sepolia
  priceAdapter.call(priceUpdate)                        // price lands BEFORE liquidate; revert ⇒ whole tx reverts
  callback.liquidate(bidAmount, solver, operationData)  // revert ⇒ LiquidationFailed(solver, nonce)
  on success: callback.payBid(bidAmount) with 100k gas  // underpay ⇒ BidUnderpaid; paid → oevCollector
  liability = (gasUsed + 35k) * tx.gasprice; deposits[solver] -= min(liability, deposit) → auctioneer
```

Implications: the nonce is **strictly greater** than the stored one and is consumed even on a failed
liquidation (but not by losing/unsubmitted bids) — so we send `nonces(signer)+1` as a high-water mark and
resync from the getter. Crucially, the on-chain nonce jumps to the *winning* bid's signed nonce, which
reflects our local monotonic counter (incremented on every auction we bid, won or lost) — so a nonce delta
is **not** a failure count, which is why the breaker is fed by the WS `liquidation-result` push for our
callback (§2), not by nonce arithmetic. The deposit is a rolling gas prepayment; **bid acceptance is purely off-chain by
`bid`** (the deposit gate is on-chain at settlement only). `deposit()` credits `msg.sender` — the signer
EOA funds its own per-signer deposit; the callback cannot. `requestWithdraw` sets `locked` (no bidding)
until a 24 h cooldown. The Executor is a UUPS proxy upgradable by RedStone.

### 6.3 EXECUTOR_V6 signing — proven, auction won

A well-formed solve signed by the project signer won a live auction (`auction-result.liquidator == our
address`, lowercased) **with `deposits(signer)=0`** — confirming RedStone bid selection is off-chain by
`bid`, while Executor deposit enforcement happens at settlement. The solver still preflights the cached
Executor deposit floor before signing. Golden vector (`eip191_test.go`; testnet key in
`OEV_SIGNER_PRIVATE_KEY`):

```
chainId 11155111, callback = signer, bid 1e14 wei, nonce 1, maxTxGasPrice 50e9, operationData over 2 borrowers
keccak(operationData) = 0x0a85a1be3cf06539edd05476a60cca5482e8ef0c4fa0bb6c1cf3f79fd0945509
digest                = 0x78f6eb68948cfeb1e16a81b050c111bf099628ff9dc51debb55f0b4fff2c7e5a
```

### 6.4 Morpho Blue (ported with exact rounding in `internal/morpho/math.go`)

- Health: `borrowed = toAssetsUp(borrowShares, totBorrowAssets, totBorrowShares)`;
  `maxBorrow = collateral.mulDivDown(price, 1e36).wMulDown(lltv)`; liquidatable iff `maxBorrow < borrowed`.
  Oracle price scale `1e36 × 10^(loanDec − collDec)`.
- LIF `= min(1.15e18, WAD.wDivDown(WAD − 0.3e18.wMulDown(WAD − lltv)))` (lltv 0.86 ⇒ ≈1.0438).
- Seize-driven: `repaidShares` from `seizedAssets.mulDivUp(price,1e36).wDivUp(LIF).toSharesUp(…)`; repaid
  amount re-derived `toAssetsUp` (rounds against the liquidator). `VIRTUAL_SHARES=1e6`, `VIRTUAL_ASSETS=1`.
- Accrual: `interest = totalBorrowAssets.wMulDown(borrowRateView.wTaylorCompounded(elapsed))` (3-term
  Taylor); borrow *shares* never change on accrual.
- Callback order inside `liquidate`: state updates → collateral `safeTransfer` to caller →
  `onMorphoLiquidate(repaidAssets, data)` → `safeTransferFrom(caller, morpho, repaidAssets)` (so the
  callback must end holding ≥ `repaidAssets` loan token + approval).
- **Griefing**: a third-party 1-wei repay/shrink can stale a seize-derived full-debt clamp. RedStone Atom
  settlement is private, so the default accepts full-collateral sizing to capture bad-debt opportunities;
  `strategy.config.sizing.allowFullLiquidation: false` is the fallback if an environment needs the old partial-collateral
  posture.

### 6.5 Sepolia testbed

Two stacks: RedStone's shared testbed (their Executor + a custom Morpho Blue + a
TLOAN(6dp)/TCOL(18dp) market, 3 borrowers
underwater at different prices), and — to avoid RedStone's registry-owner gates — our **own** Symbiotic
core + TLOAN vault + TCOL LiquidLane adapter + the single-adapter `SymbioticOevSolver` (owner = signer).
Verified on-chain: `getAmountOut(TCOL,1e18)=2000e6`, `minDiscount=10000` (**ppm**, not bps — 1e6 = 100 %,
so 10_000 = 1 %), `isFiller(marketMaker,callback)=true`. LiquidLane gotchas: `swap` takes `tokenIn` from
the adapter's own balance (callback `transfer`s then `swap`s in one tx), bounded by
`getMaxRate`/`getMaxAssets`; `getMaxAssets`/`withdrawable` are **non-view** (read via `eth_call`).

### 6.6 Proven live on Sepolia

The single-adapter stack settles real OEV liquidations end-to-end:

- **Fork rehearsal** — on a Sepolia fork, the `SymbioticOevSolver` deployed + wired, the price dropped, and
  a real liquidation settled through real Morpho + the real LiquidLane adapter (status 1, collateral
  seized, profit retained = swap proceeds − repayment, matching callback settlement events).
- **Live** — with the Sepolia test monitor configured and `dryRun: false`, the bot won a live RedStone
  auction and settled a **two-leg** liquidation through the callback (both borrowers seized, **+61.66 TLOAN** retained
  on-chain), paying `payBid` from callback native and debiting the deposit by the gas liability. Earlier
  single-leg runs proved +17.55 and +30.83 TLOAN. A deliberate unfunded revert confirmed the failure path
  (`LiquidationFailed`, nonce consumed, deposit debited the gas liability per §6.2).
  > Note: earlier live runs were against prior callback builds. The current no-preview callback keeps
  > auction auth, signed min-profit checks, fail-soft leg execution, and bundle-level profit gating, while
  > removing the expensive pre-liquidation preview path.

**Dev-settlement caveat.** On the dev endpoint the auctioneer passes `priceAdapter=address(0)` with a
frozen `priceUpdate`, so settlement does **not** write the auctioned price — every settlement reverts
`HEALTHY_POSITION` unless the feed is moved out-of-band first. Hence dev testbeds configure
`strategy.config.testMonitor` and move the feed directly (size against cached `oracle.price()`), while
production omits that block because the auctioneer applies the frame price atomically. The Sepolia
profile also uses the test monitor because
public `api.morpho.org` does not index RedStone's custom test Morpho.

### 6.7 Test harness & money model

RedStone's Node harness (testnet keys in its own `.env`) makes repeatable live liquidations possible. The
market oracle is a MorphoChainlinkOracleV2 over a mock 8-dec `collateralFeed`; `oracle.price() =
feedAnswer × 1e16`. The harness moves price via `collateralFeed.setAnswer(priceUSD×1e8)` — the *same*
feed→oracle path prod uses, only the trigger differs. Testbed orchestration lives outside this repo: reset,
price move, balance sheet, callback top-up, deposit top-up, recycle, and rebalance are operator-owned
steps. Positions don't self-heal: a partial liquidation strips the LIF bonus and leaves them *more*
underwater, so a reset re-arms them. Note: the public Alchemy Sepolia endpoint accepts writes into a private pool without
relaying them — broadcast settlement-adjacent txs via a public relay (e.g.
`ethereum-sepolia-rpc.publicnode.com`); the bot itself only reads + connects WS, so its RPC choice is
immaterial.

### 6.8 Testing / CI gating

The default `make test` (`go test -race -cover ./...`, no build tags) runs **hermetic tests only** — no
network, no chain. The opt-in live suite is excluded from CI by build tags:

- `make test-oev-live` → `//go:build live`, `TestLive*` across `internal/solvers/redstoneoev/...` —
  Morpho API borrower-discovery and token-pair market-autodiscovery checks against the live GraphQL
  schema, plus compile coverage for the optional Sepolia fork payload dump (skipped unless RPC/signing
  env is present).

Every other `*_test.go` is hermetic (config matrix, sizing/golden, EIP-191 golden+parity, single-token
bundling, WS integration via in-process `httptest`, chain-reader decoders against hand-packed ABI bytes)
and runs in CI. The contract (the OEV `SymbioticOevSolver` in the `rfq` repo's `src/oev/`, with its Forge
suite) covers the single-adapter settlement path; deploy via `script/DeployOevOwnCore.s.sol`
then wire/operate with external operator tooling. The bot's role ends at
signing + sending the WS solve (proven hermetically by `wsintegration_test.go`); RedStone's auctioneer
submits the actual `Executor.execute`.

---

## 7. Configuration & operations

All addresses / URLs / caps come from the YAML `solvers[].config` (CLAUDE.md: config is king). Secrets are
referenced by env-var name and read at point of use. The full annotated profile is
[`../config/redstone-oev.example.yaml`](../config/redstone-oev.example.yaml).

| Field | Meaning |
|---|---|
| `ws.url` / `ws.apiKeyEnv` | RedStone WSS endpoint; `x-api-key` read from the named env var |
| `executor` | Executor proxy |
| `adapter` | solver-owned single LiquidLane adapter |
| `callback` | solver-owned callback passed to RedStone Executor and into strategy input |
| `dryRun` | when true, sign and log would-bids without sending solve frames; default false |
| `strategy.name` | `default` for the built-in strategy backed by Morpho state, or `webhook` for an external decider |
| `strategy.config.{url,timeout,headers,maxRequestBytes,maxResponseBytes}` | webhook base URL and transport limits/headers; env-backed values remain env-var names in parsed config and resolve only while constructing the HTTP client; OEV route is `POST /decide-bid` |
| `strategy.config.morphoApiUrl` / `discoveryMaxHealthFactor` | default-strategy production Morpho snapshot endpoint and API health-factor band |
| `strategy.config.testMonitor.{markets[],positions[]}` | Sepolia-only fixed market and borrower seeds for the on-chain harness monitor; omit in production |
| `strategy.config.maxTrackedPositions` | logical cap for at-risk positions retained from Morpho API pages |
| `gas.{nativeUsdFeed,nativeMaxAge,tokenUsdFeeds[]}` | optional shared token/native gas conversion; the token entry must cover the adapter loan asset |
| `strategy.config.bid.{bidEth,authTtlMs,totalBundleProfitBps,minBundleProfitBidBps}` | default-strategy minimum bid, callback-auth replay window, optional gross-profit bid share, and optional bundle margin after gas + bid; the bps fields require `gas:` |
| `strategy.config.{monitorPollMs,maxStateAgeMs}` | default-strategy refresh cadence and cache staleness cutoff |
| `maxTxGasPriceWei` | always-on signed gas-price cap and native reservation assumption; also prices converted economics when `gas:` is enabled |
| `maxBidWei` | solver-owned per-auction cap on the final signed bid; required for `webhook`, optional for `default` |
| `strategy.config.sizing.{allowFullLiquidation,swapHaircutBps}` | default-strategy full-collateral policy and swap cushion |
| `breaker.maxFailures` / `windowMs` | failed-liquidation rolling-window halt plus immediate blacklist halt |
| `intervals.opsPollMs` | solver Executor-state refresh cadence |
| `intervals.executorStateMaxAgeMs` | max age of solver Executor cache before `executor_state_stale` (§3.3) |

Hard cutovers already applied:

- `OEV_DRY_RUN`, `OEV_TEST_MONITOR`, `OEV_TEST_MARKETS`, and `OEV_TEST_POSITIONS` are removed as
  direct runtime inputs. Use `dryRun` and `strategy.config.testMonitor.{markets,positions}` in YAML;
  non-secret env expansion remains available inside the file.
- OEV strategy fields moved under `strategy.config`: `morphoApiUrl`, discovery/monitor settings, `bid`,
  and `sizing`. Solver-owned `maxTxGasPriceWei` is top-level. The old unified
  `intervals.maxStateAgeMs` split into `intervals.executorStateMaxAgeMs` for solver state and
  `strategy.config.maxStateAgeMs` for default-strategy state. Unknown legacy keys fail strict decoding.
- Static `bid.loanPerEth`, `bid.minBundleProfitLoan`, and `sizing.minLegProfitLoan` are removed. The former
  `strategy.config.loanEthFeed` moved to the optional common `gas:` block; when enabled, the strategy converts
  predicted gas, bid, and margin into signed loan-denominated profit floors before bidding. The old key is
  rejected: map `ethUsd`/`maxAgeMs` to `gas.nativeUsdFeed`/duration-valued `gas.nativeMaxAge`, and map
  `loanUsd` to a `gas.tokenUsdFeeds[]` entry for the adapter's actual loan asset with its own `maxAge`.
- configurable `bid.gasBase` / `bid.gasPerLeg` is removed. Gas is route-aware from code constants plus
  cached adapter/vault state.
- configurable `bid.maxLegsPerBid` is removed. Bundle depth is derived from the cached latest header gas
  limit, the observed RedStone settlement cap, and the route-aware gas prediction.
- `sizing.maxSeizeFractionBps` is removed. Use `strategy.config.sizing.allowFullLiquidation`.

Operational controls remain file-driven:

| YAML field | Meaning |
|---|---|
| `dryRun: true` | sign and log would-bids, never send solves |
| `strategy.config.testMonitor.markets[]` | fixed Sepolia harness Morpho market ids |
| `strategy.config.testMonitor.positions[]` | fixed Sepolia harness borrower addresses |

Production sets `dryRun` according to the approved rollout, omits `strategy.config.testMonitor`, and
requires `strategy.config.morphoApiUrl` for the default strategy. Non-secret `${VAR}` expansion may be
used inside the YAML; environment variables are not read as a separate operational-config channel.

Sepolia harness operation and deployed-address manifests live outside this repo. The solver only needs
the config values for the selected environment plus the runtime secrets referenced by env-var name.

The Executor deposit is a rolling gas prepayment. Every settlement, including callback reverts, debits
`(gasUsed + 35k) * tx.gasprice` from the signer deposit. The callback native balance pays only `payBid`.
Before signing, the solver requires the Executor deposit to be above `MIN_DEPOSIT`; callback native balance
and optional gas profitability are strategy-owned. The default strategy also skips a selected bundle when its
predicted settlement gas would leave the Executor deposit below the floor.

The default strategy logs route-aware predicted gas in bundle economics. Tune `internal/liquidlane/gas`
constants just above observed successful settlements, not as a blunt worst-case multiplier. The current model
was checked on a Sepolia fork against both callback variants. Full no-preview wrapper settlements consumed
about 469,911 gas for one acquire leg, 588,048 for two acquire legs, 703,664 for one allocate leg, 969,948
for two allocate legs, and 817,877 for acquire→allocate (all including the `+35k` Executor surcharge and a
single `~40k` RedStone feed update). The preview variant was consistently more expensive on success
(roughly +190k to +440k gas for one/two-leg common cases) and only helped one narrow high-min-profit fail
case, so the no-preview variant is the preferred production callback. Unknown routes use a separate
conservative fallback because no cached route state means the solver cannot price the path fairly.

---

## 10. TODO / refinements

- **Keep calibrating predictor constants from real settlements.** The default strategy logs predicted
  route/gas; tune constants above worst observed successful settlements.
- **BidUnderpaid exposure on multi-leg bundles.** In gas-enabled mode, legs settle fail-soft with zero
  contribution, while the signed `minBundleProfit` (gas + bid + margin) assumes the whole bundle lands — one skipped leg can fail
  the bundle gate, so `payBid` pays nothing and the Executor emits `BidUnderpaid`, which RedStone counts
  toward slashing/blacklisting. Mitigations to evaluate: derive `minBundleProfit` so the gate passes when
  the strongest leg lands; feed `BundleResult.bidAuthorized == false` (from receipt decode) into the
  breaker; prefer single-leg bundles near the profit floor.
- **Adapter budget calibration.** The callback no longer signs a per-leg `maxAssets`; it takes the live
  adapter rate and relies on the per-leg profit floor. Solver-side, keep the cached per-collateral
  `getMaxAssets` budget clamp as the `InsufficientAllocate` defense with an over-reserve buffer for rate
  rises.
