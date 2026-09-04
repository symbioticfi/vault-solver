# LiquidLane conventions

LiquidLane is shared liquidity infrastructure, not a solver. This document is the compact standard for
RFQ, LI.FI, OEV, UniswapX, and any new solver that consumes `LiquidLaneAdapter` state.

## Ownership

| Package | Owns |
|---|---|
| `internal/liquidlane` | canonical adapter/vault/route/candidate/plan types, coherent latest-state `Quote`/`Fill` reads, authorization, ids, and rate math |
| `internal/capacity` | the single process-wide pending-capacity book and leases shared by all filling solvers |
| `internal/liquidlane/planning` | lane evaluation, route/capacity aggregation, quote/fill allocation, plan validation, reservation projection, and optional settlement gas pricing |
| `internal/liquidlane/gas` | shared optional Chainlink token/native facts plus neutral acquire/allocate/deallocate/unknown route prediction from current adapter/vault state |
| `internal/liquidlane/discounts` | signed-discount HTTP client, live-offer filtering, route matching, fill-quote construction, and fresh-signature validation |
| `internal/solvers/<name>` | cadence, caches, local planner inputs, protocol messages, calldata, execution, and reconciliation |

The generic framework must not know about LiquidLane. Protocol execution is never shared: RFQ, LI.FI,
OEV, and UniswapX use different contracts, signatures, status models, and callbacks. LI.FI and UniswapX
do share the identical LiquidLane route-planning calculation before their local protocol adapters build
those different executions.

## Canonical model

Every route is one-way:

```text
tokenIn -> LiquidLaneAdapter -> tokenOut
```

`tokenIn` is a member of `tokensToRedeem`; `tokenOut` is `adapter.vault().asset()`. External APIs may
use `asset`, `collateral`, or other names, but adapters must map them to `tokenIn`/`tokenOut` before the
facts reach a strategy.

| Type | Meaning |
|---|---|
| `Adapter` | stable adapter, vault, output token, and output decimals |
| `Route` | stable adapter + `tokenIn` + `tokenOut` direction and decimals |
| `Inventory` | latest executable capacity/rate for one route |
| `FillQuote` | latest executable output for one concrete `amountIn` |
| `Auth` | direct caller authorization facts |
| `gas.Snapshot` | adapter-local owner/market-maker acquire balances plus vault-level shared free/withdrawable liquidity |

Core field rules:

- `MaxAssets` is the current output cap in `tokenOut` units.
- Direct inventory `MaxRate` is `getMaxRate(tokenIn)` and already includes `minDiscount`. A `FillQuote`
  derives the same conservative fixed-point fact from `MaxAmountOut / AmountIn`, so fill-time private
  offers are bounded without another RPC call.
- Discount `MaxRate` comes from the discounts backend and already includes its advertised discount. It
  arrives already floored, while the adapter floors `getAmountOut` first and applies the discount
  second, so pricing directly at it can predict one unit above what the adapter pays. Re-derive it for
  the concrete `amountIn` with `liquidlane.ConservativeAdvertisedRate` before quoting or sizing a leg.
- `GrossAmountOut` is raw `getAmountOut`; `MaxAmountOut` is the executable amount after discount.
- `MinDiscount` is the adapter's current lower bound for a fill.
- `ValidUntil` is an external offer deadline. Inventory does not carry a duplicate read timestamp;
  solvers pass current chain/server time separately with each strategy decision.
- Shared values are copied at constructors and treated as immutable after entering a cache or strategy.

Stable ids are lowercase and content-derived:

```text
route:<chainId>:<adapter>:<tokenIn>:<tokenOut>
capacity:<chainId>:<vault>:<tokenOut>
candidate:<routeId>
candidate:<routeId>:discount:<discountId>
```

`CapacityID`, not `RouteID`, is the accounting boundary. Routes backed by the same vault/output pool are
not independent liquidity.

## Reads and freshness

LiquidLane reads always target RPC `latest` through ordinary `chain.Multicall`.

- Do not use historical block tags or require archive-capable RPCs.
- By default, capacity comes from each adapter's `getMaxAssets`. When a solver configures
  `liquidityLens`, the shared reader calls the lens for the adapter's cross-adapter deallocation-cascade
  estimate instead. The selected source is deployment config; strategies receive only the resulting fact.
- Batch related calls once per logical read. A single Multicall is internally coherent enough for current
  quoting; separate protocol reads may naturally observe adjacent heads.
- Do not attach an exact block number to latest inventory. Use decision-time chain state, TTLs, and
  protocol deadlines.
- A latest snapshot is an estimate until transaction inclusion. Strategies apply reserve, two-sided price
  movement, minimum profit, gas, and deadline padding. Execution contracts revalidate current rate and
  capacity; where the protocol permits, they clamp to a signed economic floor before reverting atomically.
- Stable metadata (`vault`, asset, decimals, redeemable tokens) may be cached. Mutable inventory is
  refreshed according to solver cadence or immediately before a fill.

The shared reader exposes facts:

```go
ResolveRoutes(ctx, adapters) ([]Route, error)
ReadInventory(ctx, routes) ([]Inventory, error)
ReadFillQuotes(ctx, routes, tokenIn, amountIn) ([]FillQuote, error)
ReadGasSnapshot(ctx, routes) (*gas.Snapshot, error)
ReadAdapterSnapshot(ctx, adapter, filler) (AdapterSnapshot, error)
ReadAuth(ctx, adapters, filler) ([]Auth, error)
FilterAuthorized(ctx, inventory, filler) ([]Inventory, error)
FilterAuthorizedRoutes(ctx, routes, filler) ([]Route, error)
```

Implementation rules:

- Use generated `PackXxx`/`UnpackXxx` helpers and Multicall batches.
- Bound `tokensToRedeem`; reject invalid addresses, decimals, rates, caps, and discounts.
- Fail startup when a configured adapter's stable `vault`, output asset, output decimals,
  `tokensToRedeem` list, or input-token decimals cannot be resolved. Silently running with a partial
  configured adapter or route set is not allowed.
- Treat an unreadable `paused` or authorization result as unavailable.
- Treat an unreadable adapter-local `acquireBalance` as zero for gas prediction. This deliberately
  selects an allocate/deallocate/unknown route with an equal or higher gas budget.
- Skip a bad route without hiding a batch transport error.
- Direct authorization is `filler == marketMaker || filler == owner || isFiller(marketMaker, filler)`.

Block polling is allowed as a refresh trigger. Receipt confirmations and protocol epochs may also use block
numbers. The restriction is specifically against historical state calls and exact-block LiquidLane reads.

## Planning boundary

The solver normally reads LiquidLane through `Reader.Quote` or `Reader.Fill` and passes immutable `Inventory` or `FillQuote` facts into its
planner. The application owns one process-wide `capacity.Book`; RFQ, LI.FI, and UniswapX receive the same
pointer and use it for accepted fills. Planners receive only its aggregate reservation snapshot; they do not
maintain a second per-order book.
Runtime values such as current block time, the txmanager fee cap, and `gas.Snapshot` are also facts. The
shared predictor owns adapter swap route units. Amount-specific RFQ-like protocols normalize inventory
against current per-physical-route `FillQuote`s through `NormalizeOracleInventory`; they never group
oracle prices by output token because the oracle, discount floor, and executable output belong to the
adapter route. Protocol adapters then map those facts into
`QuoteTask` or `FillTask`; the shared engine returns `QuoteSolution` or `FillSolution`. It ranks
already-priced candidates, enforces one alternative per physical route, solves exact input/output,
splits across route caps, allocates shared `CapacityID` budgets, applies explicit uncovered-input policy,
and, when an optional gas pricing model is supplied, converts a complete LiquidLane settlement gas
estimate into `tokenOut`. RFQ and UniswapX use the same quote engine. RFQ, LI.FI, and UniswapX use the
same fill engine; each then maps the solution into its
own wire response or executor plan. RFQ omits gas pricing, so its configuration and strategy payloads do
not gain gas fields; LI.FI and UniswapX supply gas pricing from their existing runtime facts. Candidate
discovery, protocol output resolution,
webhook DTOs, lifecycle, and calldata remain solver-local. A transaction-level calculation charges
settlement gas once, consumes acquire
balances per adapter, and consumes free/withdrawable liquidity once per shared vault. A strategy may reduce
all three budgets by its existing inventory reserve before route classification so a near-boundary plan is
priced as the next more expensive route. It must not add a standalone full-tx gas estimate for every route.

If the set of reads is itself strategy-dependent, inject a narrow read-only capability such as `Pricing`
or `LiquidLaneState`. Do not inject a signer, tx manager, or unrestricted chain client into the strategy.
The capability must accept `context.Context`, batch calls, return typed facts, and be replaceable by a fake.

Every fill plan crosses the same solver-owned execution boundary, whether it came from the built-in or
webhook strategy. The shared validator resolves every route reference back to a supplied candidate,
canonicalizes adapter/capacity/discount identity, checks amount totals, current output, gas floor, and
pending `CapacityID` reservations, then returns a cloned plan. Protocol deadlines, the executor's fixed
gas envelope, and wire semantics remain solver-local.

Webhook strategies should receive the same facts in their request. A remote strategy may own its own RPC
only when that deployment deliberately accepts different freshness and availability from the local path.

## Discounts and capacity

Direct and signed-discount inventory for the same route are alternative ways to use the same capacity.
Never sum them. `internal/liquidlane/planning` encodes the one-candidate-per-route rule for quote and fill
tasks across RFQ, LI.FI, and UniswapX; execution reservations use the shared `CapacityID`.
Concrete RFQ and UniswapX requests allocate capacity after filtering to their pair. LI.FI does the same for
each standing pair curve, so multiple curves may advertise the same unreserved vault capacity; accepted-fill
reservations are then subtracted from every curve sharing that `CapacityID`.

For signed discounts:

1. Treat HTTP `nonce` values as base-10 uint256 strings, parse them into `*big.Int`, and reject hex,
   signs, non-digits, and values wider than 256 bits. The EIP-712 value remains the same uint256.
2. List and validate advertised offers for quote construction.
3. Never apply `discount` to backend `maxRate` a second time — but do re-derive the rate for the
   concrete `amountIn` with `ConservativeAdvertisedRate`. The backend floors the discount into the
   rate while the adapter floors `getAmountOut` first, so the raw rate can price a unit above what the
   adapter pays, and an over-predicted leg leaves the filler short of the order's signed outputs.
4. Resolve signatures again immediately before fill.
5. Recheck id, adapter, tokens, current discount bounds, and deadlines.
6. Reserve capacity for upward price movement: discount swaps release their full computed output and
   cannot be reduced to a requested amount.
7. Pass a discount candidate only when the solver's executor can settle `discountSwap` atomically.

Discount discovery, parsing, physical-route matching, cap/rate clipping, advertised fill-quote
construction, and fresh signed-term binding/deadline/output validation are shared. Solvers still own when
resolution happens: LI.FI pre-resolves a bounded candidate set and refreshes adapter state before deciding;
UniswapX resolves only the selected route; RFQ receives backend candidates and resolves selected legs.
Generated executor calldata and protocol lifecycle remain solver-local.

## Solver profiles

| Solver | LiquidLane usage | Solver-local responsibility |
|---|---|---|
| RFQ | amount-specific quote and fresh fill inventory; narrow pricing capability may read during strategy evaluation | RFQ order lifecycle and Reactor/Executor calldata |
| LI.FI | latest inventory on tick/block refresh; fresh `FillQuote` for each received order | range curves, OIF contexts, exclusivity, and immediate-fill lifecycle |
| OEV | background latest inventory stored by Morpho market id | Morpho discovery, auction pricing, safety haircut, liquidation sizing, bundle/gas/deposit accounting |
| UniswapX | short-lived background quote inventory; fresh `FillQuote` for each admitted order | quote request policy, Reactor orders, Permit2/cosigner rules, auction curve, order status, and fill calldata |

3F does not use LiquidLane and should not be forced through these types. Shared code is justified by the
protocol dependency, not by making every solver look identical.

## Adding another solver

1. Resolve configured adapters into shared routes.
2. Choose latest-state refresh cadence and a stale-data policy.
3. Map shared facts into a small solver-specific strategy input.
4. Account capacity by `CapacityID` and in-flight execution.
5. Keep gas, margin, price movement, auctions, and exclusivity in the strategy.
6. Re-read amount-specific state and protocol status before sending funds-moving calldata.
7. Keep execution, signatures, wire DTOs, and failure state machines in the solver package.

The shared package provides current LiquidLane facts. Solvers decide when those facts are sufficient and
contracts enforce the final executable truth.
