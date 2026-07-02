# vault-solver — RedStone OEV solver (plan)

The **`redstone-oev`** solver: an off-chain bidder for RedStone Atom OEV auctions that captures
price-driven liquidations on **Morpho Blue** and exits the seized RWA collateral through **one**
Symbiotic LiquidLane adapter (one vault) in the same atomic transaction. It follows the framework
boundary and conventions in [`../CLAUDE.md`](../CLAUDE.md). §6 records the verified ground truth
(Executor source, wire schemas, Sepolia testbed, proven live liquidations) the design rests on.

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
2. **Hot path** (`candidates` → `selectBundle` → sign, sync, in-memory, no I/O — budget ≈ 400 ms minus
   WS RTT): build the candidate set from our own tracked at-risk positions, recompute health at the
   settlement price with the shared Morpho math (incl. local interest accrual — never trust a pushed
   `current_ltv`), drop positions already committed by an in-flight bid, size each seize/exit leg (taken
   only if the adapter exit covers Morpho repayment), select the bundle by after-cost net profit, run the O(1) pre-bid
   gates, sign the **EXECUTOR_V6** EIP-191 payload, and reply `{"op":"solve", …}`.
3. **Settlement** (on-chain, RedStone submits — not us): the Executor verifies our signature + nonce,
   applies the price update to the oracle, then calls our callback's `liquidate(bid, solver,
   operationData)` → per leg `Morpho.liquidate(…)` → Morpho pushes the seized collateral to the callback
   and invokes `onMorphoLiquidate`, where the callback sells the WHOLE seizure through its one immutable
   `LiquidLaneAdapter` in a single `swap` after recomputing current min-out, then approves Morpho's
   repayment pull — then the Executor calls `payBid(bid)` and the callback pays it in native.
   The Executor **catches** a callback revert (`LiquidationFailed(solver, nonce)`): the nonce is still
   consumed and the gas liability still debited from the deposit — the liquidation just doesn't settle
   (§6.2). A price-update revert, by contrast, reverts the whole tx.
4. **Bookkeeping** (background ops loop): an Executor-state poll refreshes nonce/deposit/callback-balance.
   The circuit breaker is fed by the WS `liquidation-result` push (a `success:false` frame for our callback →
   `recordFailure`); we are a state-reading bot and do **not** run chain log scans. When a
   `liquidation-result.txHash` is available, we decode callback events from that receipt for attribution.
   Realized profit is the callback's loan-token balance, not event accounting.

Two roles, two pots (see §6.2 for the verified Executor mechanics):

- **Signer EOA's deposit on the Executor** (≥ `MIN_DEPOSIT` = 0.00001 ETH on Sepolia) — a *rolling gas
  prepayment*: after every settlement, win or revert, `(gasUsed + 35k) × tx.gasprice` is deducted and
  paid to the auctioneer. This gas debit is **independent of the auction** (the winner is decided purely
  by bid amount) and is **not reverted if the deposit underpays** — so it does NOT change the signed bid
  amount. The pre-bid deposit gate requires enough unreserved deposit for `MIN_DEPOSIT + predictedGas ×
  maxTxGasPriceWei`; in-flight bids reserve their predicted gas debit until they resolve. It is still real
  cost: the bot bids only if bundle loan profit, converted through the cached loan↔ETH rate, covers
  estimated gas plus the dynamic bid and the optional `bid.minBundleProfitBidBps` margin. Topped up
  out-of-band by the operator (`scripts/oev/oev-balance.sh`).
- **Callback contract's native balance** — pays the bid via `payBid` (forwarded to RedStone's
  collector); **owner-refilled** out-of-band. Liquidation profit accumulates in the callback's ERC-20
  balance; the owner withdraws it (`withdrawERC20`). The bot never self-funds.

**One bot, one vault, one loan token, one venue (Morpho).** The `SymbioticOevSolver` pins a single
immutable `LiquidLaneAdapter` at construction, so the solver serves exactly that adapter's vault. A market is
tracked only when its loan token equals that adapter's vault loan token (resolved on-chain). A single
auction is answered by ONE bid bundling every gas-fit, after-cost-positive leg (one loan token, no per-token
grouping). **One solver runs per vault** (spec §11: RWA curators set per-vault
discounts, so swaps can't aggregate across vaults): a second vault/loan token is a second
`SymbioticOevSolver` + a second process.

---

## 2. Architecture

A self-contained `internal/solvers/redstoneoev/` implementing `solver.Solver` — **no framework edits**
(CLAUDE.md modularity rule):

- **`Solver` owns the whole pipeline for one venue.** WS lifecycle, bid economics, in-flight
  reservation, EXECUTOR_V6 signing, and the breaker. There is exactly one
  venue (Morpho liquidation through one LiquidLane adapter / one loan token / one vault), so the bot
  reads the monitor snapshot directly: `s.fresh`/`s.candidates` (`candidates.go`) gate on cache
  freshness and turn liquidatable positions into sized, scored `scoredLeg`s via the shared Morpho math
  (`sizeLeg`). `selectBundle` bundles the legs into one bid; the callback runs each leg through its one
  `LiquidLaneAdapter`. A second venue would be a new solver package, not an in-package abstraction.
- **Settlement truth comes over WS, not from logs.** The breaker's failure feed is RedStone's
  `liquidation-result` push: a `success:false` frame whose `liquidator` is our callback → `recordFailure`,
  tripping the breaker after N in the window. We gate on `liquidator == callback` (same won-detection as
  `auction-result`) because the frame arrives on both the broadcast `oev/liquidations` and the
  callback-scoped `oev/notify/<callback>` subscription, so a result may belong to another solver. We are a
  state-reading bot, **not** a log-indexer: there is no `FilterLogs` scan. If a settlement receipt is
  available from RedStone's `txHash`, the bot decodes callback `LegResult`/`PayBidResult` logs from that
  receipt for diagnostics. Realized profit is read off the callback's loan-token balance / balance sheet,
  not event accounting.
- **`Run(ctx)`** owns the resilient WS client (connect with `x-api-key`, subscribe `oev/liquidations` +
  `oev/notify/<callback>`, reconnect with backoff + jitter, ~7 h proactive rotation, staleness
  watchdog), the hot-path handler, the monitor's market/position refresh loops, and the Executor-state
  ops loop. It joins every background loop on shutdown (`sync.WaitGroup`) so no goroutine outlives `Run`.
  Caches are immutable snapshots swapped atomically (`atomic.Pointer`), read lock-free on the hot path.
- **The solver sends no transactions** — RedStone's auctioneer submits the settlement tx; Executor deposit
  management is out-of-band. `deps.TxManager` is therefore unused, and the OEV config carries no
  `txManager` section.
- **`deps.Signer` is the EXECUTOR_V6 signer.** The bid digest is `keccak256(abi.encode("EXECUTOR_V6",
  chainId, callback, keccak256(operationData), bidWei, nonce, maxTxGasPrice))` wrapped in EIP-191
  (`personal_sign`), signed via `Signer.SignHash`. The signer EOA **is** the wallet holding the Executor
  deposit (the Executor recovers the signer and debits *its* deposit/nonce). A KMS split is later
  hardening, same as 3F/RFQ.
- **On-chain reads use latest-state `chain.Multicall` in background loops only** — nothing on the hot path
  touches the network except the final `ws.Send`. Production Morpho market/position state comes from the
  GraphQL snapshot; chain reads are limited to adapter/callback/Executor data (loan token, redeemable
  collateral set, filler status, route quotes, deposit/nonce, gas predictor). The Sepolia test monitor is
  the only path that reads Morpho `market()`/`position()` on-chain, over explicit test seeds. `chain.Multicall`
  itself packs `aggregate3` + does its own
  `eth_call` + unpacks via the v2 Multicall3 binding — every binding is abigen --v2 now (no v1 path remains).
- **Validate-everything, fail closed.** Auction frames are external input that drives funds: per-frame
  count is bounded; a paused/dry/unserved vault yields no quote → no bid; malformed fields skip the leg,
  never panic. Every pre-bid gate (snapshot block epoch, deposit gas headroom, callback balance, the
  bundle-level gas profitability check, the adapter's `isFiller` gate) fails closed. The bid is bounded **off-chain** by the
  configured `bid.bidEth` floor and optional `bid.totalBundleProfitBps` (spec §8) — the contract carries no on-chain bid cap.
- **Metrics** on the shared registry (`deps.Metrics.Registerer()`, nil-safe): auctions/bids/wins/
  failed-liquidations counters, a `skips_total{reason}` vector, a hot-path latency histogram, deposit
  and callback-native gauges. The breaker halts bidding after N failed liquidations in a rolling window,
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
| `solver.go` | `Register`, factory, `Run` (loops + join), `handleAuction` → `buildBid`, ops loop, the head-stable Executor/callback-balance/rate/gas-predictor cache (`cachedState`/`stateCache`) |
| `candidates.go` | auction frame → `[]evalItem` (price selection: auctioned frame price, or the test-only cached on-chain price) + the solver's I/O-free hot-path candidate sizing (`candidates` → `sizeLeg`); the candidate set is our own tracked positions (`workerCandidates`) — the frame's pushed positions are not consumed |
| `bundle.go` | single-token leg selection (`selectNetBundle`/`selectBundle`, `scoredLeg`/`chosenBundle`): live bidding chooses the bundle by bounded after-cost net search; dry-run/no-rate fallback ranks by gross loan profit; bid is `max(bidEth, grossProfitNative * totalBundleProfitBps / 10000)` |
| `sizing.go` | adapter pricing primitives (`swapOutFor`/`collForBudget`) and `sizeLeg`, the per-candidate single-swap leg sizing/decision over the shared Morpho math |
| `internal/morpho/math.go` | shared Morpho Blue math: health, LIF, share/asset conversions, Taylor accrual (exact big.Int rounding) |
| `monitor.go` | Morpho API snapshot, atomic hot-path state, single-adapter quote resolution |
| `morphoapi.go` | OEV-local adapter over generated Morpho GraphQL operations: `markets` returns adapter-scoped market state (§3.4), `marketPositions` returns at-risk position state capped at `maxTrackedPositions` (§3.2) |
| `api/morphographql` | generated Morpho GraphQL binding from vendored schema + explicit operation documents |
| `chainreader.go` | abigen --v2 binding instances (`api/bindings/oev/*`), keccak market-id re-derivation (`deriveMarketID`), adapter quote/filler/gas-predictor reads, and loan↔ETH feed reads |
| `fillerauth.go` | the single adapter's `isFiller` swap-caller preflight (`ReadFillerStatus`) |
| `reservations.go` | in-flight `payBid`/position reservation + auction-id de-dup (`seenAuctions`) |
| `wsclient.go` | resilient WS client: reconnect/backoff/jitter, ~7 h rotation, heartbeat, subscribe replay |
| `wsmessages.go` | wire types pinned to RedStone's zod + a captured auction frame |
| `eip191.go` | EXECUTOR_V6 digest + EIP-191 `SignBid` via `Signer.SignHash` (golden + parity tests) |
| `operationdata.go` | ABI-encode callback `operationData`: auction auth, capped max-seize legs, loan-denominated profit floors, and callback-auth signature |
| `callbackevents.go` | decode callback `LegResult` / `PayBidResult` receipt logs for settlement attribution |
| `noncestore.go` | strictly-ascending nonce high-water mark, reconciled with the on-chain getter |
| `breaker.go` | failed-liquidation rolling-window breaker + `blacklisted`-frame halt |
| `metrics.go` | nil-safe Prometheus collectors on the shared registry |
| `rate.go` / `gaspredictor.go` / `config.go` | loan↔ETH rate math, loan→native conversion, route-aware gas units, and `loanEthFeed` parsing |
| `config.go` | typed, validated `parseConfig` (shared parse/coerce helpers in `internal/parse`) |

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

In production (`morphoApiUrl` set), the monitor snapshots **all Morpho data from the Morpho GraphQL API**:

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

The only chain reads left in production monitor refreshes are adapter-specific cache data: adapter loan
token/redeemable collateral set/filler status and route quotes (`getMaxRate`, `getMaxAssets`, `paused`).
Those are not Morpho state and are needed to know whether the configured LiquidLane route can settle the
seized collateral.

`morphoApiUrl` is a production hard requirement. The Sepolia harness is the only exception: with
`OEV_TEST_MONITOR=true`, the bot reads a fixed seed set from `OEV_TEST_MARKETS`/`OEV_TEST_POSITIONS` and
reads Morpho `market`/`position` state on-chain from the callback's `MORPHO()` getter. Public
`api.morpho.org` does not index the custom Sepolia deployment.

### 3.3 Snapshot concurrency model

The monitor exposes one `atomic.Pointer[snapshot]`, written by **exactly one goroutine** (the monitor
run loop builds a fresh, never-mutated snapshot and swaps it) and read concurrently by the WS goroutine
via `candidates()`. Readers never lock — they `Load` the current pointer. The Executor-state cache
(`cachedState`/`stateCache` in `solver.go`) and the nonce high-water mark follow the same single-writer /
lock-free-read model.

Every mutable snapshot records exactly one source block and that block's timestamp. API markets from a
different `state.blockNumber` are dropped instead of being mixed into the same snapshot; the test monitor
uses the latest RPC header block. The hot path fails closed with `stale_epoch` when a non-empty snapshot
has no block tag/timestamp or when its block timestamp is more than a small Ethereum/Sepolia block-time
window behind the auction timestamp. This allows ordinary one-block monitor/API lag without letting a
stuck API cache bid indefinitely. The ops loop is separate: it refreshes Executor state, callback native
balance, loan↔ETH feeds, gas price, and gas-predictor getters on `opsPollMs`.

**Stale-state gate (rule for all background caches).** Every background-refreshed cache the hot path
reads stamps a wall-clock `updatedAt` **only on a successful store** — today the monitor snapshot
(Morpho markets/positions + adapter/vault quotes) and the ops-loop `cachedState` (Executor accounting,
callback balance, loan↔ETH rate, gas predictor). Before any bid, `staleStateGate` fails closed with
`stale_state` (an error log naming each stale component and its age) when any stamp is older than
`intervals.maxStateAgeMs` — a loop that keeps failing while serving its prior data stops bidding instead
of running on arbitrarily old state. Startup config validation enforces that every background poll
interval (`opsPollMs`, `monitorPollMs`) is strictly less than `maxStateAgeMs`. **Any future
background-refreshed state consumed by the bid path MUST follow the same pattern: stamp `updatedAt` on
successful store, join `staleStateGate`, and include its refresh interval in the startup
`< maxStateAgeMs` validation.**

### 3.4 Market scope

The tracked Morpho markets are **discovered from the adapter**, not configured. The (loan, collateral)
pair is fully on-chain-derivable from the pinned adapter: loan = `adapter.vault().asset()` (cached
immutable), collateral = the adapter's redeemable token set (`getTokensToRedeemLength()` +
`tokensToRedeem(i)`, cached). The API query uses that pair and validates returned ids locally. The auction
frame's pushed positions are never a discovery source.

---

## 4. Economics & sizing

### 4.1 The three prices

Sizing uses the right price at each step; conflating them is the easiest way to bid into a loss:

1. **Market price** — what we expect on the oracle *at settlement* → drives liquidatability and what we
   owe Morpho per unit seized (with local accrual). Production uses the **auctioned** frame price (the
   auctioneer applies the frame's pushed price atomically before our callback). A TEST-ONLY env flag,
   `OEV_ONCHAIN_PRICE_FOR_TEST=true`, instead sizes against our cached `oracle.price()` — required on the
   dev testbed, where settlement does *not* apply the frame price (§6.6). Sizing against the wrong one
   reverts `InvalidSwapRate`.
2. **Adapter redemption rate** (`getMaxRate` = the adapter oracle price × (1 − curator `minDiscount`),
   read on-chain + cached, decimals-correct) → the loan token we expect for the seized RWA, minus an
   extra `swapHaircutBps` safety cushion. Off-chain sizing uses it to estimate expected output; the leg
   encodes only `maxSeizeAssets`, and the no-preview callback recomputes current output in
   `onMorphoLiquidate` before approving repayment.

Gas is **not** a sizing input and not part of the bid amount: the auction winner is decided purely by bid,
and the `(gasUsed + 35k) × tx.gasprice` liability is debited from the deposit AFTER settlement, independent
of the auction, and is not reverted if the deposit underpays (§6.2). It is still real cost, so the bot uses
`bidEth` as the bid floor but selects live bundles by
`loanToNative(bundleGrossLoan, rate) - gasNative(bundle) - bidNative(bundle)` and only bids when the selected bundle clears
`ceil(bidNative * bid.minBundleProfitBidBps / 10000)`. `bidNative(bundle)` is
`max(bidEth, loanToNative(bundleGrossLoan, rate) * bid.totalBundleProfitBps / 10000)`.
The rate is required and comes from the cached dual-feed oracle
(`loanEthFeed`: ETH/USD + loan/USD); the hot path never
does feed I/O. `gasNative(bundle) = gasUnits(bundle, cachedPredictorState) × maxTxGasPriceWei`.
`maxTxGasPriceWei` is also the value signed into EXECUTOR_V6; using one ceiling avoids winning an auction
with a gas cap too low for RedStone's settlement tx.
The route-aware gas estimate is converted to loan units by the solver at `maxTxGasPriceWei` and signed into
each leg as `minProfit`; the callback does no ETH↔loan conversion. Morpho liquidation is kept as a fixed per-leg component because fork measurements
showed only about 3k gas of branch spread across the observed partial/full sizing cases, while the LiquidLane
swap leg is classified as acquire-only, allocate+sync, or deallocate+allocate+sync from cached adapter/vault
getter state. If state is missing or insufficient, the leg falls back to the conservative unknown route. A
live or dry-run config must provide a rate source because `operationData` carries loan-denominated
profit floors.

**Profit = swap proceeds − repayment** (in the loan token). When the auctioned price crashes below the
adapter's NAV rate, a leg captures both the liquidation bonus *and* that gap. Off-chain `sizeLeg` only takes
a leg when expected adapter output exceeds repayment. The bundle must also clear gas + dynamic bid +
`minBundleProfitBidBps` in `selectNetBundle`. On-chain, the no-preview callback executes each signed
`maxSeizeAssets` leg fail-soft through Morpho, sells the actual seizure, and reverts that leg unless realized
output covers Morpho's actual repayment plus the signed leg `minProfit`. After all fail-soft legs, the
callback reverts unless realized total loan profit clears the signed `minBundleProfit`, and only then enables
`payBid`.

**One sizing strategy: MAX.** By default, target 100 % of collateral and let the debt/liquidity clamps size
it down. This captures bad-debt opportunities: the unit economics are still `swap proceeds − repayment`,
but a full-collateral bad-debt liquidation can have more total profit because it does not leave the final
collateral slice behind. If settlement routing ever has issues with full-collateral cases, set
`sizing.allowFullLiquidation: false`; that hard-disables full seize and uses the fixed 90 % partial fallback.
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

`selectNetBundle` selects live bundles by after-cost net profit:
`loanToNative(sumProfitLoan, rate) - gasNative(bundle) - bidNative(bundle)`. It uses a bounded beam search over
deterministic gross-ordered subsets, so a locally best leg cannot block a lower-gross subset with better
shared-liquidity/gas economics. Every scored leg is expected-positive before gas; the final bundle must clear
gas, bid, and `bid.minBundleProfitBidBps`.

Bundle depth is not configured. Each trial bundle must fit the gas envelope:
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
Dry-run without a rate source keeps the old gross-profit `selectBundle` path so operators can observe flat
bids without configuring loan↔ETH conversion. `maxTxGasPriceWei` is the hard ceiling for the
`tx.gasprice` signed into EXECUTOR_V6 and live net selection.

**The bid has a floor and optional profit share.** The solver bids
`max(bidEth, grossProfitNative * bid.totalBundleProfitBps / 10000)`: the floor keeps thin auctions simple,
while the bps share can scale bids with larger bundles. Since the winner is decided purely by bid amount and
gas is debited post-settlement regardless, the bot bids only when the selected bundle clears the after-cost
profitability check, gated by the callback holding the bid native (payBid) and the Executor deposit holding
`MIN_DEPOSIT + predictedGas × maxTxGasPriceWei`. Every
`auction-result` frame is logged with the winning `bid` and whether we won, so a win-rate controller can
later consume those results; the bid remains solver-bounded off-chain (spec §8).

---

## 5. Settlement, reservation & safety

- **In-flight reservation.** A sent bid commits `reservedBid{wei, gasNative, nonce, at, positions}` against
  cached headroom: its payBid native, predicted Executor-deposit gas debit, and the `(market, borrower)`
  positions it liquidates. `buildBid` debits both funding pots (so two bids in one window can't double-spend
  the callback or over-commit gas deposit) **and drops in-flight
  positions from its candidates** — until a prior bid's settlement reflects on-chain, the snapshot still
  shows the position liquidatable, so re-bidding it would revert `HEALTHY_POSITION`. Result frames release
  the reservation immediately when we lose `auction-result` or when our `liquidation-result` arrives. Nonce
  reconciliation and `reservationTTL` are backstops for missed frames.
- **Adapter authorization (one gate, fail closed).** The configured adapter is usable only if its own
  swap-caller predicate passes (`ReadFillerStatus`: `callback == marketMaker() || callback == owner() ||
  isFiller(marketMaker, callback)`) — so the bot never bids a leg whose swap would revert `InvalidCaller`.
  The single immutable `LiquidLaneAdapter` is pinned at construction, so this curator `setFiller` gate is the
  only adapter-routing preflight.
- **Other pre-bid gates**: background-cache age (`stale_state`, vs `intervals.maxStateAgeMs` — §3.3), snapshot block epoch (`stale_epoch`), duplicate-auction
  de-dup + `timeoutMs` drop, after-cost profitability (`gas_unprofitable`), deposit gas headroom
  (`deposit_low`) + callback-native funding. The bid is bounded **off-chain** by `bid.bidEth` plus optional `bid.totalBundleProfitBps` — there is no on-chain bid cap. A deposit below MIN_DEPOSIT raises
  an `oev_deposit_below_floor` gauge + error log; insufficient predicted-gas headroom logs a structured
  skip with deposit/reserved/required values. Topping it up (and the callback's payBid native) is the
  operator's job (`scripts/oev/oev-balance.sh`).
- **No self-funding.** The bot moves no funds outside the signed settlement: the callback's payBid native
  pool is owner-refilled out-of-band and the signer's Executor gas deposit is topped up out-of-band. The
  bot holds a signing key but its only fund-moving action is the signed bid the Executor settles.

### The `SymbioticOevSolver` contract

The on-chain settlement contract is the OEV `SymbioticOevSolver` in rfq `src/oev/`: a single-adapter router
(Morpho-only on-chain) whose constructor pins ONE immutable `LiquidLaneAdapter` and one `AUTH_SIGNER`.
The Executor calls `liquidate(bid, solver, operationData)`. The callback verifies the solver-signed
auction auth (`auctionKey`, `bidAmount`, `minBundleProfit`, and capped legs), marks `auctionKey` used,
then processes legs fail-soft. It enables exactly that `payBid` only after realized bundle profit clears
`minBundleProfit`.

Each leg carries `marketId`, `borrower`, `maxSeizeAssets`, and loan-denominated `minProfit`.
`SymbioticOevSolver` is a no-preview callback: it reads immutable Morpho market params, calls
`Morpho.liquidate` directly with the signed `maxSeizeAssets`, and lets `onMorphoLiquidate` validate economics
from actual `repaidAssets`. Morpho is invoked with `try/catch`, so a stale/healthy/reverting leg emits
`LegResult` and the bundle can continue. During `onMorphoLiquidate`, the callback sells the seizure through
the immutable `LiquidLaneAdapter`, approves Morpho repayment, and requires the realized gain to cover repayment
plus the current leg gas floor. A reverted leg contributes its signed `minProfit` as skipped-cost penalty to
the final bundle profit gate.

The payBid native pool is owner-funded (`receive`) and owner-withdrawable (`withdrawNative`/`withdrawERC20`).
`payBid` is not an economic choice: after a valid `liquidate` call it pays the authorized bid amount, because
that is the solver's auction commitment. Calling `payBid` without a matching authorized liquidation emits
`PayBidResult(..., false)` and pays nothing.

Settlement events emitted on-chain (`LegResult` and `PayBidResult`; the Executor's
`LiquidationFailed(solver indexed, nonce)`) document settlement and post-mortem reasons. The bot does not
scan historical logs, but when WS supplies a `liquidation-result.txHash` it fetches that receipt and logs
decoded callback events. The breaker is still fed by the WS `liquidation-result` push (§2).

---

## 6. Verified ground truth

Primary sources: the RedStone OEV docs; `redstone-finance/redstone-evm-examples/oev`; the **verified
Executor source** (Blockscout Sepolia, impl behind proxy `0xfdFB1862…EBd`); Morpho Blue
(`morpho-org/morpho-blue@main`); and live Sepolia state. All deployed addresses are in
[`addresses.sepolia.json`](../scripts/oev/addresses.sepolia.json).

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
`bid`, while Executor deposit enforcement happens at settlement. The solver still preflights cached
deposit headroom before signing. Golden vector (`eip191_test.go`; testnet key in
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
  `sizing.allowFullLiquidation: false` is the fallback if an environment needs the old partial-collateral
  posture.

### 6.5 Sepolia testbed

Two stacks (full manifest in [`addresses.sepolia.json`](../scripts/oev/addresses.sepolia.json)): RedStone's
shared testbed (their Executor + a custom Morpho Blue + a TLOAN(6dp)/TCOL(18dp) market, 3 borrowers
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
- **Live** — under `OEV_ONCHAIN_PRICE_FOR_TEST=true` (and `OEV_DRY_RUN` unset → real bidding), the bot won a live RedStone auction and settled
  a **two-leg** liquidation through the callback (both borrowers seized, **+61.66 TLOAN** retained
  on-chain), paying `payBid` from callback native and debiting the deposit by the gas liability. Earlier
  single-leg runs proved +17.55 and +30.83 TLOAN. A deliberate unfunded revert confirmed the failure path
  (`LiquidationFailed`, nonce consumed, deposit debited the gas liability per §6.2).
  > Note: earlier live runs were against prior callback builds. The current no-preview callback keeps
  > auction auth, signed min-profit checks, fail-soft leg execution, and bundle-level profit gating, while
  > removing the expensive pre-liquidation preview path.

**Dev-settlement caveat.** On the dev endpoint the auctioneer passes `priceAdapter=address(0)` with a
frozen `priceUpdate`, so settlement does **not** write the auctioned price — every settlement reverts
`HEALTHY_POSITION` unless the feed is moved out-of-band first. Hence `OEV_ONCHAIN_PRICE_FOR_TEST=true` on
dev testbeds that move the feed directly (size against cached `oracle.price()`) and production unset (the
auctioneer applies the frame price atomically). The Sepolia profile also runs `OEV_TEST_MONITOR=true`,
because public `api.morpho.org` does not index RedStone's custom test Morpho.

### 6.7 Test harness & money model

RedStone's Node harness (testnet keys in its own `.env`) makes repeatable live liquidations possible. The
market oracle is a MorphoChainlinkOracleV2 over a mock 8-dec `collateralFeed`; `oracle.price() =
feedAnswer × 1e16`. The harness moves price via `collateralFeed.setAnswer(priceUSD×1e8)` — the *same*
feed→oracle path prod uses, only the trigger differs. `scripts/oev/oev-testrun.sh` drives the loop (reset → drop
price → run bot → status); positions don't self-heal (a partial liquidation strips the LIF bonus, leaving
them *more* underwater, so a `reset` re-arms them). `scripts/oev/oev-balance.sh` gives a read-only `sheet` (every
pool + readiness warnings) and owner-key writes (`recycle`, `topup-callback`/`topup-deposit`,
`rebalance`). Note: the public Alchemy Sepolia endpoint accepts writes into a private pool without
relaying them — broadcast settlement-adjacent txs via a public relay (e.g.
`ethereum-sepolia-rpc.publicnode.com`); the bot itself only reads + connects WS, so its RPC choice is
immaterial.

### 6.8 Testing / CI gating

The default `make test` (`go test -race -cover ./...`, no build tags) runs **hermetic tests only** — no
network, no chain. The opt-in live suite is excluded from CI by build tags:

- `make test-oev-live` → `//go:build live`, `TestLive*` — Morpho API borrower-discovery and token-pair
  market-autodiscovery checks against the live GraphQL schema.

Every other `*_test.go` is hermetic (config matrix, sizing/golden, EIP-191 golden+parity, single-token
bundling, WS integration via in-process `httptest`, chain-reader decoders against hand-packed ABI bytes)
and runs in CI. The contract (the OEV `SymbioticOevSolver` in the `rfq` repo's `src/oev/`, with its Forge
suite) covers the single-adapter settlement path; deploy via `script/DeployOevOwnCore.s.sol`
then wire/operate via `scripts/oev/oev-balance.sh setup-callback` + `oev-testrun.sh`. The bot's role ends at
signing + sending the WS solve (proven hermetically by `wsintegration_test.go`); RedStone's auctioneer
submits the actual `Executor.execute`.

---

## 7. Configuration & operations

All addresses / URLs / caps come from the YAML `solvers[].config` (CLAUDE.md: config is king). Secrets are
referenced by env-var name and read at point of use. The full annotated profile is
[`../config/redstone-oev.sepolia.example.yaml`](../config/redstone-oev.sepolia.example.yaml).

| Field | Meaning |
|---|---|
| `ws.url` / `ws.apiKeyEnv` | RedStone WSS endpoint; `x-api-key` read from the named env var |
| `executor` / `callback` / `adapter` | Executor proxy, `SymbioticOevSolver`, and the single LiquidLane adapter |
| `morphoApiUrl` / `discoveryMaxHealthFactor` | Production Morpho snapshot endpoint and API health-factor band |
| `maxTrackedPositions` | logical cap for at-risk positions retained from Morpho API pages |
| `loanEthFeed.{ethUsd,loanUsd,maxAgeMs}` | required dual-feed loan↔ETH rate source |
| `bid.bidEth` / `bid.totalBundleProfitBps` / `bid.minBundleProfitBidBps` | minimum bid, optional gross-profit bid share, and optional bundle margin after gas + bid |
| `bid.maxTxGasPriceWei` | signed gas-price cap and the gas price used for bundle/deposit gates |
| `sizing.allowFullLiquidation` / `swapHaircutBps` | full-collateral policy and swap cushion |
| `breaker.maxFailures` / `windowMs` | failed-liquidation rolling-window halt plus immediate blacklist halt |
| `intervals.{ops,monitor}PollMs` | ops refresh cadence and monitor snapshot cadence (each must be < `maxStateAgeMs`) |
| `intervals.maxStateAgeMs` | max age of any background cache before bidding fails closed on `stale_state` (§3.3) |

Hard cutovers already applied:

- Static `bid.loanPerEth`, `bid.minBundleProfitLoan`, and `sizing.minLegProfitLoan` are removed. Rate comes
  only from `loanEthFeed`; the solver converts predicted gas, bid, and margin into signed loan-denominated
  `minProfit` / `minBundleProfit` floors before bidding.
- configurable `bid.gasBase` / `bid.gasPerLeg` is removed. Gas is route-aware from code constants plus
  cached adapter/vault state.
- configurable `bid.maxLegsPerBid` is removed. Bundle depth is derived from the cached latest header gas
  limit, the observed RedStone settlement cap, and the route-aware gas prediction.
- `sizing.maxSeizeFractionBps` is removed. Use `sizing.allowFullLiquidation`.

Dev/test env knobs are not config fields:

| Env | Meaning |
|---|---|
| `OEV_ONCHAIN_PRICE_FOR_TEST=true|1` | size against cached `oracle.price()` instead of auction frame price |
| `OEV_TEST_MONITOR=true|1` | use the Sepolia harness monitor instead of Morpho API |
| `OEV_TEST_MARKETS` / `OEV_TEST_POSITIONS` | comma/space-separated Sepolia harness seeds for `OEV_TEST_MONITOR` |
| `OEV_DRY_RUN=true|1` | sign and log would-bids, never send solves |

Production leaves `OEV_ONCHAIN_PRICE_FOR_TEST` / `OEV_TEST_MONITOR` unset and requires `morphoApiUrl`.

Useful scripts live under [`../scripts/oev`](../scripts/oev). The full deployed-address manifest is
[`../scripts/oev/addresses.sepolia.json`](../scripts/oev/addresses.sepolia.json).

- `scripts/oev/oev-balance.sh sheet` shows Executor deposit, callback native, callback loan balance,
  and readiness warnings.
- `scripts/oev/oev-balance.sh topup-deposit` tops up the signer deposit on the RedStone Executor.
- `scripts/oev/oev-balance.sh topup-callback` tops up callback native for `payBid`.
- `scripts/oev/oev-balance.sh recycle` / `rebalance` move testbed funds back into the ready pools.
- `scripts/oev/oev-testrun.sh` drives reset → price move → bot run → status on the Sepolia testbed.

The Executor deposit is a rolling gas prepayment. Every settlement, including callback reverts, debits
`(gasUsed + 35k) * tx.gasprice` from the signer deposit. The callback native balance pays only `payBid`.
Before signing, the solver requires unreserved deposit for `MIN_DEPOSIT + predictedGas * maxTxGasPriceWei`
and unreserved callback native for the selected bid.

The solver logs predicted gas units/routes on bid, records actual/predicted gas ratio, and decodes callback
`LegResult`/`PayBidResult` logs when a `liquidation-result.txHash` receipt is available. Tune predictor
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

- **Keep calibrating predictor constants from real settlements.** The solver logs predicted route/gas and
  records actual/predicted gas ratio from `liquidation-result.txHash` receipts; tune constants above worst
  observed successful settlements.
