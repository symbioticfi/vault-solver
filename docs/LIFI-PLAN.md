# vault-solver — LI.FI / Catalyst same-chain intent filler (plan)

The **`lifi-samechain`** solver fills **same-chain on-chain** LI.FI Intents (Open Intents Framework /
Catalyst). The executor contract is the registered LI.FI solver identity. Its owner
authorizes runtime callers; a caller submits the selected direct `FillRoute[]` and discount-backed
`DiscountRoute[]`, and the executor uses the input settler's direct finalise path,
receives the claimed input RWA in the callback, redeems it through a Symbiotic
**LiquidLane adapter**, then fills and attests the output in one transaction.
Follows the framework boundary and
conventions in [`../CLAUDE.md`](../CLAUDE.md); the strategy layer follows
[`strategy-plan.md`](strategy-plan.md).

> **Status:** the on-chain-order path is implemented and has settled a real Sepolia order end to end.
> The solver parses matched escrow orders from the WebSocket feed, takes a fresh LiquidLane fill snapshot,
> runs the strategy decision, builds `LiquidLaneLifiExecutor.finaliseWithCurrentTimestamp(...)` calldata,
> confirms `InputSettlerEscrowLIFI.orderStatus(orderId) == Deposited`, and submits through the shared
> `txmanager`. Gasless opening is explicitly out of scope. The executor is registered once through EIP-1271
> and the framework signer is an authorized runtime caller; no per-fill solver signature is required. The
> latest ERC-1271-enabled executor ABI still requires the
> redeploy and authorization step tracked in §10 before the next live run.

---

## 1. What it does

A user opens/funds an intent on-chain: "here is X of RWA token `tokenIn`; pay me ≥ Y of `tokenOut`
(the redeemed underlying)." The LI.FI order server is still used for quote discovery, status tracking,
and matched-order delivery; it pushes the `StandardOrder` to us over the solver WebSocket. We settle
that already-opened order in **one atomic transaction**:

1. call `LiquidLaneLifiExecutor.finaliseWithCurrentTimestamp(order, routes, discountRoutes)` with the
   matched `StandardOrder` and selected direct/discount-backed LiquidLane routes,
2. the executor calls `InputSettler.finalise(...)` with `solver = destination = address(this)`; the input
   settler releases the opened order input to the executor and calls `orderFinalised(inputs, FillCall)` with
   the callback payload constructed by the executor,
3. inside the callback the executor redeems the received RWA through the LiquidLane adapter, then fills
   and attests the output.

Because input redemption and output fill are in one transaction, the executor does **not** need prefunded
output inventory for this path. The economic surplus is aggregate redeemed output minus the resolved order
output after the strategy's gas-aware checks. It remains in the executor; the current ABI has no sweep
entrypoint, so recovery requires the proxy administration path described in §7.

This is the same-chain specialization of the cross-chain OIF flow. Same-chain is strictly simpler:
`StandardOrder.inputOracle` and `MandateOutput.oracle` / `settler` all identify the OutputSettler because
there is no cross-chain proof relay. These order fields are separate from the order server's
supported-contract `oracle` kind. The user/order creator is responsible for the on-chain open step before
the solver sees the order.

---

## 2. How it maps onto the framework

A new self-contained `internal/solvers/lifi/` implementing `solver.Solver` — no framework edits
(CLAUDE.md modularity rule). Reused as-is:

- **`Run(ctx)`** connects to the LI.FI order server (WebSocket order feed), refreshes standing quotes,
  and evaluates every admitted order once for immediate execution; blocks until ctx cancels.
- **Fills go through the shared `txmanager`** — the solver builds the executor finalise calldata;
  txmanager owns the nonce, send, and receipt/revert. Same nonce-serialized EOA as every other solver.
- **On-chain reads use `chain.Multicall`** — adapter `getAmountOut` / `minDiscount` / `getMaxAssets` /
  `getMaxRate`, executor immutables/caller authorization, and filler authorization are batched where appropriate.
- **Signer/caller** — the framework EOA is the tx sender and must be authorized through
  `executor.setCallers(...)`. The registered LI.FI solver address is the executor contract itself.
- **Config, secrets** — order-server URL + `apiKeyEnv`, settler/executor/adapter addresses via
  `solver.config`; the LI.FI API key via `*Env` indirection. `solverMode` mirrors RFQ: `external` is
  direct-only, while `internal` enables the shared private-discounts backend.
- **Pluggable strategy** — both the standing-quote curve and the fill decision are a strategy
  (`DecideQuotes` + `DecideFill`; `default` in-process or `webhook` external), per
  [`strategy-plan.md`](strategy-plan.md). See §5.2.
- **Shared LiquidLane decision boundary** — direct/physical inventory, fill quotes, and gas state come
  from the common snapshot reader. Default allocation and external webhook plans converge on the same
  canonical fill routes, capacity reservations, gas floor, and fail-closed route validation before the
  LI.FI-specific OIF calldata mapping.

### Component / repo map

| Piece | Where | Responsibility |
|---|---|---|
| `LiquidLaneLifiExecutor` (Solidity) | `../rfq/src/lifi/` | Caller-gated solver/callback contract; `finaliseWithCurrentTimestamp(...)` calls `InputSettler.finalise`; `orderFinalised(..., FillCall)` redeems claimed input via LiquidLane, fills output, and attests; ERC-1271 validates domain-separated registration signatures against the current callers. |
| Vendored OIF interfaces/structs | `../rfq/src/lifi/interfaces/` | `IInputCallback`, `MandateOutput`, `StandardOrder`, OutputSettler `fill`/`setAttestation` surface. |
| `lifi` solver (Go) | `internal/solvers/lifi/` | Pricing, decision, finalise calldata with typed direct `FillRoute[]` plus discount-backed `DiscountRoute[]`, submit. |
| Order-server client (Go, generated) | `api/lifiorder/` ← `openapi/lifi-order.openapi.json` | Typed HTTP client for register / `quotes/submit` / `orders` (vendor→generate→commit, like `api/rfqbackend`). The WebSocket order feed is a thin hand-written client. |
| LI.FI strategies (Go) | `internal/solvers/lifi/strategies/` | `default` owns local quote/fill policy; `webhook` delegates to `/decide-quotes` and `/decide-fill` and validates returned route references. |
| LI.FI order server | external | Discovery: standing quotes + matched-order WS feed. |
| OIF settlers | on-chain (LI.FI-owned) | Order lifecycle; **we do not deploy these**. |

---

## 3. On-chain contract — `LiquidLaneLifiExecutor`

Contracts live in `../rfq/src/lifi/`. `LiquidLaneLifiExecutor` is a self-contained finalise + callback
executor for LI.FI opened orders. It does **not** reuse the RFQ `Reactor` — the OIF settlers already
own the order/nonce/settlement lifecycle. `LiquidLaneLifiExecutor.finaliseWithCurrentTimestamp` is the
tx entrypoint the Go solver calls.

### Interface

```solidity
// Caller-gated runtime entrypoint. The executor derives settler, solver, and destination itself.
function finaliseWithCurrentTimestamp(
    StandardOrder calldata order,
    FillRoute[] calldata routes,
    DiscountRoute[] calldata discountRoutes
) external;

// IInputCallback (vendored from OIF) — the settler calls this on `destination`.
function orderFinalised(uint256[2][] calldata inputs, bytes calldata call) external;

// Hashes the LI.FI message hash into the executor's EIP-712 registration domain.
function lifiRegistrationDigest(bytes32 messageHash) external view returns (bytes32);

// EIP-1271 registration only; accepts a domain-separated signature from any current caller.
function isValidSignature(bytes32 hash, bytes calldata signature) external view returns (bytes4);
```

The current contract also exposes `initialize`, `callers`/`setCallers`/`isCaller` plus standard Ownable
`owner()`. It is deployed behind a transparent proxy: settler addresses are implementation immutables;
owner, caller list, and EIP-712 state are initialized in proxy storage. The owner manages caller authorization.
ERC-1271 uses the same caller set but is not used by the Go fill path.

`inputs` are the RWA amounts delivered to the executor during finalise. The solver submits separate direct
and discount-backed route arrays. The executor constructs the callback `FillCall` from the canonical order
and those routes:

```solidity
struct FillCall {
    bytes32 orderId;          // OIF order id
    MandateOutput output;     // the single output to satisfy (token, amount, recipient, ...)
    uint32 fillDeadline;      // from the order
    FillRoute[] routes;               // direct-swap LiquidLane legs
    DiscountRoute[] discountRoutes;   // signed discount-backed legs
}
struct FillRoute {
    address adapter;
    uint256 amountIn;
    uint256 amountOut;
}
struct DiscountRoute {
    address adapter;
    uint256 amountIn;
    ILiquidLaneAdapter.DiscountSwap discountSwap;
    bytes protocolSignature;
}
```

### Execution flow

1. `finaliseWithCurrentTimestamp(order, routes, discountRoutes)` requires an authorized executor caller,
   computes the order id, and constructs callback data from `order.outputs[0]`, `order.fillDeadline`, and
   the supplied direct/discount-backed routes.
2. It calls `InputSettler.finalise(order, solveParams, bytes32(address(this)), call)` with
   `solveParams[0].solver = bytes32(address(this))`. The settler's direct path accepts this because its caller
   is the canonical solver contract.
3. After the input is claimed, `orderFinalised` accepts calls only from the immutable input settler, transfers
   each route's input to its adapter, executes `FillRoute[]` through direct `swap` and `DiscountRoute[]`
   through signed `discountSwap`. The canonical adapter verifies discount signer/protocol signatures and terms.
4. The OutputSettler resolves the accepted limit or exclusive-limit context authoritatively and pulls the
   amount it is owed; a shortfall or invalid context reverts the transaction. Dutch contexts are rejected by
   the solver before planning.
5. The executor calls `setAttestation(...)`. Any produced surplus stays in the executor.

### Authorization & safety

- **`INPUT_SETTLER`, `OUTPUT_SETTLER` immutable** (constructor). The Go solver verifies both at startup.
- **Zero governance fee** — this implementation intentionally does not model input deductions. Startup reads
  `InputSettler.governanceFee()` and fails unless it is exactly zero. Every admitted order repeats that read
  before order identification or planning; a non-zero or unreadable result skips the order and emits an error
  log while the process remains available for later orders. With that invariant, the Go solver also requires
  the calldata route-input sum to equal the gross order input.
- **Caller runtime gate** — only addresses installed by the owner through `setCallers` can call finalise.
  Startup verifies the framework signer through `isCaller`. The configured adapter list is the trusted route
  scope; the current executor intentionally has no second adapter allowlist.
- **ERC-1271** is used only for LI.FI account registration. It wraps LI.FI's message hash in the
  `LiquidLaneLifiExecutor` version `1` EIP-712 domain for the current chain and executor address, then accepts
  a signature from any current caller. It is not used on each fill.
- In `external` mode the executor must be a **registered direct filler** on every configured adapter.
  In `internal` mode direct candidates still require that authorization, but signed discount candidates
  do not: the adapter authorizes those through the discount signer and protocol cosign. The executor
  configured route scope remains mandatory in both modes.
- Attack surface is bounded: the solver only finalises orders that were already opened/funded on-chain,
  and the executor/output settler revert the whole tx unless redemption, fill, and attestation all
  succeed. A bad order can at worst cost a reverted fill attempt; it cannot redirect output or surplus.

### Placement & house style

`src/lifi/LiquidLaneLifiExecutor.sol` + vendored `src/lifi/interfaces/*` (mirroring the RFQ/OEV
contract style).
solc `0.8.28`, BUSL-1.1 header, `forge fmt` (120-col, tabs, double quotes, `int_types=long`), I-prefixed
interface with full NatSpec, section separators, `callers`/`setCallers`-style patterns. Tests:
`test/lifi/LiquidLaneLifiExecutor.t.sol` style unit tests +
an on-chain-order E2E script/test (modeled on
`catalystsystem/lifi-intent/test/integration/InputSettler7683LIFI.samechain.t.sol` and
`test/CoreMirrorIntegration.t.sol`), aiming for 100% line/branch coverage.

---

## 4. The order model & same-chain lifecycle

**`StandardOrder`** (OIF): `{ address user; uint256 nonce; uint256 originChainId; uint32 expires;
uint32 fillDeadline; address inputOracle; uint256[2][] inputs; MandateOutput[] outputs; }`. Inputs are
`[tokenId, amount]` (token as `uint256(uint160(addr))`).

**`MandateOutput`** (OIF): `{ bytes32 oracle; bytes32 settler; uint256 chainId; bytes32 token; uint256
amount; bytes32 recipient; bytes callbackData; bytes context; }`. Same-chain: `oracle == settler ==
OutputSettler`, `chainId == block.chainid`, empty `callbackData`, and `order.inputOracle ==
OutputSettler`. `context` is the OutputSettlerSimple pricing/access payload: empty or `0x00` = limit
amount (`output.amount`), `0x01` = Dutch amount, `0xe0` = exclusive limit, `0xe1` = exclusive Dutch.
The solver supports only limit and exclusive-limit contexts. It discards both Dutch variants at WebSocket
admission and logs the order identifiers and unsupported context type.

**Entrypoint:** the bot calls
`LiquidLaneLifiExecutor.finaliseWithCurrentTimestamp(order, routes, discountRoutes)`. The executor calls
the LI.FI opened-order direct finalise path with the current `block.timestamp`; both canonical solver and
destination are the executor itself, and it constructs the callback `FillCall` internally. Gasless
`openForAndFinalise` is not supported.

**Deployed addresses** (LI.FI-owned; integrate against these — do **not** deploy):
- `InputSettlerEscrowLIFI` / opened-order input settler: `0x000025c3226C00B2Cdc200005a1600509f4e00C0`
- OutputSettler (LIFI): `0x0000000000eC36B683C2E6AC89e9A75989C22a2e`
- (bare OIF reference set: `InputSettlerEscrow 0x1CC9260E285C2C8AC8D2E7102F3978056Ec1d0a8`,
  `OutputSettlerSimple 0x52602D7cc3D833F5d28ee6D01C7F82C9b2322e10` — deployed at identical addresses on
  Sepolia-family testnets + mainnets via CREATE2. Use the LI.FI addresses for order-server integration;
  confirm the exact settler the order server routes for our chain in P1.)

---

## 5. Off-chain solver design

### 5.1 Discovery (LI.FI order server)

The REST surface is the **generated `api/lifiorder` client** (from the vendored
`openapi/lifi-order.openapi.json`); all calls carry the `x-api-key` header (`LIFI_SOLVER_API_KEY`). Wire
shapes below are verified against the live `order-dev.li.fi` OpenAPI.

Account, chain, contract, and route prerequisites are the onboarding runbook in §8.1. LI.FI registers the
**executor contract** as the solver account through EIP-1271. On startup the solver verifies that the API
key's registered identities include the configured executor, then checks
`GET /api/v1/solver/supported-contracts` and, when needed,
merges the configured escrow InputSettler and OutputSettler into the complete list with `PUT`. The endpoint
has replace semantics, so the solver preserves existing entries, registers the two settlers only in their
respective `inputSettler` / `outputSettler` lists, and leaves `oracle` untouched. LI.FI correctly returns an
empty `oracle` list for this same-chain model; the order-level oracle identifiers still equal the configured
OutputSettler, but that does not make the settler an `oracle`-kind supported contract. This opts the solver
into opened escrow delivery over WebSocket; the same executor is the on-chain solver identity and callback
destination.

#### Identity, API key, and reputation

Our deployment convention is **one LI.FI API key ↔ one registered executor contract**. LI.FI supports
multiple registered accounts under one key, but this deployment deliberately keeps each executor on its own
key and reputation. All logical solvers or processes operating through one executor share its
`LIFI_SOLVER_API_KEY`, quotes, matched orders, and status.

The LI.FI API key, executor owner key, and authorized caller transaction key are distinct credentials.
Sharing the LI.FI identity does not make uncoordinated active-active processes safe: they would observe the
same order flow, and every authorized caller can submit the same fill. Multiple instances must use a single
active sender or shared order coordination; active/standby replicas are the simple supported deployment.

**Standing quotes** — on each `quoteIntervalMs` tick, or once per new block when
`quoteRefreshMode: block` (block mode polls at `quoteIntervalMs`, default 1s), compute one non-overlapping
price curve per RWA→underlying pair from `adapter.getMaxRate` / `getMaxAssets`, and
`POST /quotes/submit`:
```
{ quotes: [{ fromChain, toChain,        // toChain == fromChain for our same-chain routes
             fromAsset, toAsset, fromDecimals, toDecimals,
             ranges: [{ minAmount, maxAmount, quote }],   // quote = toAsset per 1 fromAsset, decimal string
             expiry, exclusiveFor }] }  // exclusiveFor = executor address → matched orders route only to us
```
`fromChain` and `toChain` are transport fields only: `orderClient` initializes both once from the solver's
configured runtime chain. Strategy outputs and quote-state keys contain only the local token pair, so a
same-chain solver cannot accidentally publish a mixed-chain curve.

There are two independent exclusivity layers. Quote `exclusiveFor = executor` tells the order server which
registered solver should receive a match. Supported on-chain exclusivity is encoded as an `0xe0` exclusive
limit context: before its start time only the encoded `exclusiveFor` address may fill; afterwards any allowed
solver may fill. The strategy resolves that context at decision time and skips an order that is not executable
by this executor now. Exclusive Dutch (`0xe1`) is unsupported and discarded on receipt. `quoteId` is optional
correlation metadata only: it is not an authorization input, is not used by the contract, and may be absent in
the WS event.

**Order feed** — subscribe to the WebSocket `user:vm-order-submit` event (respond to `ping` with
`pong`). The socket reader hands parsed orders to a bounded FIFO so slow chain reads do not block
heartbeats; queued replays are coalesced by on-chain order ID, and a full queue logs and drops the
newest message instead of growing memory without bound (the upstream replay can redeliver it). An
accepted message is evaluated once; the solver does not persist or retry it locally. Each message is a
`SubmitOrderDto`:
```
{ orderType?, quoteId,
  inputSettler,        // escrow-vs-Compact discriminator — must be the opened ESCROW settler
  order: StandardOrder, meta: { orderStatus: Signed|Delivered|Settled, onChainOrderId, ... } }
```
We do **not** listen to on-chain events for discovery. The order must arrive via the LI.FI WebSocket.
The fill path requires `inputSettler` = the configured escrow input settler and a live, not-yet-settled
status (`Signed`/`Delivered` today). LI.FI's opened-order WS message currently omits `orderType`, so an
absent value is accepted; an explicitly supplied value is fail-closed to the opened on-chain shapes we
know (`OnChainOrder` / `oif-user-open-v0`). A missing type is inferred only when
`meta.onChainOrderId` and `inputSettler` are present, and is not trusted by itself: the full
`StandardOrder`, configured escrow identity, canonical order ID, and `Deposited` on-chain status are still
required. It does
not require a gasless permit/3009 signature or backend `sponsorSignature`.

### 5.2 The strategy — owns both decisions

All pricing lives in a pluggable strategy (per [`strategy-plan.md`](strategy-plan.md)): the solver
maps adapter reads into typed LiquidLane facts and **executes** (publish quotes, send the tx); the strategy is
the brain for **both** decision points — the standing-quote curve *and* the fill decision — mirroring
rfq's `DecideQuote`/`BuildFillPlan`. LiquidLane route/inventory/fill-quote terminology follows
[`LIQUIDLANE-CONVENTIONS.md`](LIQUIDLANE-CONVENTIONS.md), so LIFI-specific route snapshots should map
from shared `Route`, `Inventory`, and `FillQuote` facts rather than defining a third LiquidLane shape.
LI.FI's standing range quote construction stays local. Its fill decision normalizes fresh facts into the
neutral `FillTask` also used by RFQ and UniswapX for LiquidLane route selection, shared-capacity
reservation, gas conversion, price buffering, and minimum-output distribution. LI.FI still resolves OIF
output contexts locally.

```go
type Strategy interface {
    // §5.1 standing-quote curve, from configured routes + live adapter facts.
    DecideQuotes(ctx, QuoteInput) (QuoteOutput, error)   // → per-pair ranges[] {minAmount,maxAmount,quote}
    // A matched WS order + fresh adapter reads → immediate fill or skip.
    DecideFill(ctx, FillInput) (*FillPlan, error)
}
```

- **`QuoteInput`** = shared `[]liquidlane.Inventory`, latest LiquidLane gas snapshot
  (adapter-local owner/market-maker `acquireBalance` and vault-level shared `freeAssets`/`withdrawable`), vault-level in-flight capacity
  reservations, chain time, server wall time, solver-owned quote expiry, and raw current
  `txmanager.MaxFeePerGas`. The shared LiquidLane predictor derives every adapter swap route as
  acquire/allocate/deallocate/unknown. The solver reads Chainlink native/USD and token/USD feeds at the
  latest state and passes a `tokenOut per native` snapshot to the strategy. Every distinct resolved
  adapter `tokenOut` must have a configured feed; missing coverage fails startup and stale/invalid rounds
  fail closed for that decision. Gas units are code-owned conservative constants: 250k fixed LI.FI
  settlement, shared LiquidLane route units, and 75k for each private route.
  `DecideQuotes` applies `inventoryReserveBps` before pricing, normalizes direct and private inventory into
  shared greedy candidates, and keeps at most three physical routes (one for permissioned inputs). LI.FI
  retains only the range-shaped protocol adapter: selected capacity is divided geometrically into at most
  `rangeCount` candidate ranges (default eight, hard protocol limit sixteen). For each
  `[inputLow,inputHigh]` the strategy calls the same exact-input `greedy.SolveQuote` used by concrete
  RFQ-style solvers at both endpoints. The lower of the two endpoint rates is capped by a linear conservative
  floor derived from the alternatives able to cover each route at `inputHigh`, worst-case complete-plan gas,
  and integer rounding. This covers interior route switches without enumerating route combinations. Two
  price-movement stages are deducted (quote→decision and decision→inclusion). There is no separate LI.FI
  quote planner or minimum-profit setting. If a candidate range starts below the conservative economic floor,
  the strategy finds the first lower bound whose fixed-upper floor yields a representable positive output and
  publishes that safe suffix. It omits the range only when no such suffix exists or endpoint pricing still
  fails closed. Solver-level token admission uses the shared
  `internal/tokenpolicy` policy also used by RFQ: `all` serves every input, `permissioned` serves only
  `permissionedTokens`, and `permissionless` serves only inputs outside that set. Only the
  `permissioned` scope is single-route: the solver passes that constraint into each strategy decision,
  the curve uses one physical route, and the solver rejects any fill plan that does not contain exactly
  one route. All live direct and private candidates for one route remain alternatives, never additive; the
  allocator chooses the best alternative able to cover each concrete leg. Routes sharing a vault share one
  conservative `CapacityID`; reserve is applied
  before in-flight amounts are subtracted. In internal mode, advertised discount inventory is bounded
  by current on-chain `getMaxAssets`/`getMaxRate` and its deadline. The backend discount and its already-net
  `maxRate` are validated together; the strategy must not apply the ppm discount to that rate again.
  Quote lifetime
  belongs to the solver cadence: by default the head is polled every second, quotes are recalculated once per
  new block, at most three physical routes are used, and `quoteTtl` is 36 seconds. Unchanged quotes are renewed
  when at most `max(quoteInterval, quoteTtl / 3)` remains, even when no new block is observed or the head poll
  fails. The strategy
  may only shorten that expiry to `discount deadline - executionDeadlineBuffer`.
  Capacity allocation is scoped to one token pair before the range curve is built. Different pairs backed
  by the same vault therefore each advertise the full currently unreserved `CapacityID` instead of receiving
  static shares. This is deliberately optimistic: an accepted fill reserves the shared domain and wakes quote
  refresh immediately, but two orders matched against the previous curves can still race. Fresh fill planning,
  the shared reservation ledger, and inclusion-time adapter checks prevent double spending; they do not promise
  that every concurrently matched order can be filled.
- **`FillInput`** = the matched signed `StandardOrder` output facts (`output.amount`, raw
  `output.context`) plus fresh `getAmountOut`, `minDiscount`, `getMaxAssets`, pending fill reservations
  by shared `CapacityID`, and the same latest LiquidLane gas facts. Direct candidates require current
  filler authorization. Internal discount candidates are resolved again through the
  backend, validated against the advertised ID/adapter/token/deadlines and adapter minimum, then priced
  as `getAmountOut * (1 - signedDiscount)`.
  `DecideFill` returns an immediate `*FillPlan` or `nil`; the solver does not retain or retry skipped orders.
  The `default` resolves the supported OutputSettlerSimple contexts: limit and exclusive limit both use
  `output.amount`, while an exclusive order for another solver before `startTime` is declined. Dutch and
  exclusive Dutch orders never reach the strategy because WebSocket admission discards them. It fills
  only when aggregate fresh output covers resolved amount + one execution price buffer + gas for
  every selected leg, the adapter asset matches `output.token`, and `fillDeadline`/`expires` plus
  private-signature deadlines have at least `executionDeadlineBuffer` remaining. The plan commits a target
  after downward `priceBufferBps` and an aggregate internal `minAmountOut = resolvedAmount + gas`. For direct
  routes, the calldata `amountOut` is the buffered target and the adapter either produces it or reverts. A
  private-discount swap uses its signed terms instead of calldata `amountOut`, so
  the strategy requires its full current output plus upward `priceBufferBps` to fit reserved capacity.
  The current adapter minimum is checked directly; there is no separate discount-headroom policy.
  Permissionless tokens may split the order across independent capacity domains. Selection keeps at most
  the best direct and best private candidate per physical route, then greedily assigns each remaining leg
  to the highest-rate candidate that can cover that route's full available share. Shared-vault capacity is
  reserved as each leg is selected. There is no capacity-first retry or plan comparison: once the complete
  allocation is built, full route-aware gas is charged and the plan is either executed or skipped.
  Routes sharing a vault consume one aggregate capacity and gas-liquidity budget. Direct and
  private candidates for the same route remain mutually exclusive. These route-planning mechanics live
  in the shared LiquidLane fill core; this strategy supplies the policy values and adapts the result to
  `FillPlan{Routes}`.

The order worker owns pending fills and their capacity reservations. It reserves each direct route's
target output and each private route's upward-buffered output against its shared `CapacityID` while an
accepted fill tx is in flight, passes the aggregate reservation snapshot to every later fill decision,
and releases it only when the shared tx manager returns after the globally configured confirmation depth.
A successful tx-manager admission immediately sends a coalesced refresh signal; confirmed completion and
reservation release send another. A single shared `CapacityLedger` is the source for
both fill planning and quote refresh, and the quote coordinator does not keep a second copy of per-order
reservations. On startup, when any economic payload changes, or when expiry enters the renewal
window, it submits the replacement curve directly; LI.FI overwrites the old quote for the pair. When a pair
stops quoting, it submits the last curve with an expiry in the past, which overwrites and immediately expires
the old server-side quote. An unchanged pair is not reposted on every calculation tick.

The solver then executes the result — publish the curve, or send one
`finaliseWithCurrentTimestamp(order, routes, discountRoutes)` tx from the
`FillPlan`. `default` is in-process. `webhook` posts the same raw snapshots to `/decide-quotes` and
`/decide-fill`; its response is a `FillPlan` or `null`. The solver normalizes adapters/capacity IDs from
trusted candidates and rejects unknown, oversized, duplicated, input-mismatched, capacity-conflicting,
or gas-negative routes before calldata construction. LI.FI owns the fixed settlement and
private-payload gas envelope; the shared gas calculator consumes it for both quote/fill decisions and
solver-side validation.

### 5.3 Build & submit

Split the strategy plan into direct executor `FillRoute[]` and discount-backed `DiscountRoute[]` by
`DiscountID` → require the combined input sum to equal the gross order input → pack
`LiquidLaneLifiExecutor.finaliseWithCurrentTimestamp(order, routes, discountRoutes)` via generated bindings
→ read `InputSettlerEscrowLIFI.orderStatus(orderId)` again → submit only when the status is `Deposited`.
The executor derives the solver identifier from `address(this)`. The WS handler
places parsed orders into an in-memory FIFO without blocking the socket reader, so ping/pong and later messages
continue while one planner evaluates accepted orders in arrival order. It reads
fresh state and gas, asks the strategy, builds calldata, and hands the result to txmanager. No `FillPlan`,
gas cap, adapter snapshot, discount resolution, or calldata waits in a second queue. The solver has no local
in-flight limit: every accepted order is handed to the shared txmanager as soon as planning finishes, but it
stays unsigned while another transaction lifecycle is active.
An admitted order first verifies `governanceFee() == 0`, then derives the canonical ID and verifies
`orderStatus == Deposited` before expensive route reads. It selects only configured routes matching both
order tokens. For private candidates it resolves the
signatures under one order-server timeout, then re-reads latest-state LiquidLane inventory and current block
time before each strategy decision. That decision-time max fee is passed as a hard per-request cap to `txmanager`.
Before broadcast, txmanager reserves replacement headroom inside that budget and rejects the fill if the current
base fee and priority fee no longer fit. It verifies `Deposited` again immediately before async submission. The
shared txmanager
serializes the complete signed lifecycle, so later fills wait unsigned until the active nonce reaches a terminal
receipt. Pending calls are fee-bumped within their decision cap. After the shared pending timeout, txmanager
cancels the active nonce with a same-nonce self-transfer; this cancellation is outside the fill's profitability
cap but remains bounded by the operator's required global `txManager.maxFeeGwei`. Normal sends reserve one
replacement bump below that global ceiling so cancellation still has fee headroom. LI.FI
requests use the earliest order or selected-discount deadline for admission and same-nonce cancellation, then
complete after the configured confirmation depth; the planner releases that fill's reservation only then.
Every later fill decision subtracts aggregate pending
capacity before route allocation. At inclusion, the LiquidLane adapter and OutputSettler enforce the requested
swap and resolved output; stale state therefore reverts atomically rather than being repriced by the executor.
There is no solver-level pending plan, timer, future-auction scheduling, or new fill attempt. The txmanager
may replace the same pending nonce as described above; that is fee management for one submission, not order
retry.
For a selected private candidate, the solver uses its `DiscountID` only as the off-chain resolution key,
then commits the fresh signed terms and both signatures inside a separate `DiscountRoute`; a missing or
mismatched resolution aborts before submission. Those two signatures authorize the private LiquidLane route
and are unrelated to LI.FI account or fill authorization.

### 5.4 Config block (sketch)

```yaml
solvers:
  - name: lifi-samechain
    config:
      strategy:
        name: default
        config:
          priceBufferBps: 20
          inventoryReserveBps: 500
          minAmount: "1000000"          # operator tokenIn floor; ranges may start higher
          rangeCount: 8                 # geometric ranges per pair; hard max is 16
          executionDeadlineBuffer: 12s
      gas:
        nativeUsdFeed: "0x…"
        nativeMaxAge: 1h                     # native/USD feed heartbeat
        tokenUsdFeeds:
          - token: "0x…"                # every resolved adapter tokenOut
            feed: "0x…"                 # token/USD Chainlink feed
            maxAge: 24h                 # this token/USD feed's heartbeat
      orderServer:
        baseUrl: https://order-dev.li.fi          # order.li.fi in prod
        wsUrl:   wss://order-dev.li.fi
        apiKeyEnv: LIFI_SOLVER_API_KEY
      solverMode: internal
      privateDiscountsUrl: ${RFQ_BACKEND_URL}
      inputSettler:  "0x000025c3226C00B2Cdc200005a1600509f4e00C0"
      outputSettler: "0x0000000000eC36B683C2E6AC89e9A75989C22a2e"
      executor:      "0x…"                          # registered EIP-1271 LiquidLaneLifiExecutor
      adapters:                                      # LiquidLane adapters (RWA→underlying); vault+asset resolved on-chain
        - "0x…"
      tokensToQuote: permissioned                    # all (default) | permissioned | permissionless
      permissionedTokens:                            # membership set; single-route only in permissioned scope
        - "0x…"
      quoteIntervalMs: 1000                          # block poll interval; default is 1000ms
      quoteTtl: 36s                                  # rolling expiry, about three Ethereum blocks
      quoteRefreshMode: block                       # block (default) | interval
```

---

## 6. Data flow (end to end)

```
LI.FI order server ──(WS: opened/funded StandardOrder)──▶ lifi solver
  price: fresh direct getAmountOut or signed-discount output; getMaxAssets → reserved cap
  decide: buffered target ≥ resolved output + gas, deadlines buffered ?  ── no ─▶ skip
     │ yes
  build direct FillRoute[] + discount-backed DiscountRoute[]; require combined Σ amountIn == order input
     │
  txmanager ─▶ LiquidLaneLifiExecutor.finaliseWithCurrentTimestamp(order, routes, discountRoutes)
              └▶ InputSettlerEscrowLIFI.finalise(... solver=destination=EXECUTOR ...)
                 ├─ deliver opened order RWA → EXECUTOR
                 └─ EXECUTOR.orderFinalised(inputs, FillCall):
                    direct swap(amountOut) or signed discount swap → EXECUTOR
                    OUTPUT_SETTLER.fill + setAttestation
  surplus (redeemed - resolved output) remains in EXECUTOR
```

---

## 7. Error handling & safety

- **Atomic revert-safety** is the backbone: if the redemption under-delivers, the adapter reverts, or
  the output fill/attestation fails, the entire tx reverts — no partial state, no stuck funds.
- **Pre-submit skips** (never send a doomed tx): insufficient output
  (aggregate buffered target below output amount + selected-leg gas), buffered private output above reserved
  capacity, invalid current private discount bounds, asset mismatch, deadline/expiry inside the execution
  buffer, or adapter paused.
- **Inclusion-time enforcement** — direct routes ask the adapter for the buffered target; private routes use
  the signed terms. If current adapter state cannot execute the request or the OutputSettler cannot pull the
  accepted order amount, the whole transaction reverts.
- **Gas-aware quotes** — the solver supplies the live txmanager fee cap, latest LiquidLane gas state, and
  Chainlink-derived token/native conversion as raw facts. Code-owned fixed settlement/private units combine
  with shared route prediction; there is no separate gas padding knob. `minAmount` is the operator's token-input
  floor. The per-range conservative floor covers complete-plan gas, both quote-time price windows, and rounding;
  ranges with no positive suffix are omitted.
- **Capacity safety** — routes sharing a vault share one conservative capacity domain. Each pair may advertise
  the full domain, while quote and fill planning subtract every in-flight buffered output from it. This
  optimistic publication can overbook across simultaneously matched pairs, but each fill still uses a fresh
  chain snapshot and the adapter enforces execution at inclusion. An accepted fill immediately requests quote
  replacement; its reservation remains until configured confirmations complete, and an economic change removes
  old server ranges before replacement.
- **Authorization safety** — startup validates executor immutables and requires the framework signer to be
  authorized by `executor.isCaller`. Startup and every admitted order also require
  `InputSettler.governanceFee() == 0`; fee-bearing input settlement is intentionally unsupported. External mode
  additionally requires direct `owner/marketMaker/isFiller` authorization for every route. Internal mode
  checks direct authorization dynamically and otherwise requires fresh adapter-verified discount signatures.
- **Staleness** — a matched order is priced against a fresh read at decision time, not the quote-time
  curve. Private-signature resolution is bounded by one timeout and followed by another adapter/block-time
  read, so network latency cannot silently preserve the pre-resolution capacity snapshot.
- **Competition** — same-chain fills are winner-take-all on-chain; `exclusiveFor` on our quotes routes
  matched orders to us, but a late/again-priced fill can still revert (already filled) → drop.
- **Private discounts** — internal mode uses shared `internal/liquidlane/discounts` discovery, physical-route
  matching, cap/rate clipping, and fresh signed-term validation. Advertised terms
  may shape standing quotes, but execution always resolves fresh signatures, recomputes output from the
  current adapter oracle, uses the selected discount ID only for off-chain resolution, and commits the typed
  signed payload in `DiscountRoute`.
- **No prefunded working inventory is required** (unlike OEV): the opened input funds each atomic
  redemption. A reverting fill spends gas only; accumulated executor surplus is a separate standing balance
  governed by the deployment and zero-fee invariant.
- **Executor surplus** may remain as a standing balance. The current split-route executor ABI has no sweep
  entrypoint; recovery therefore requires the deployment's proxy-upgrade administration rather than the
  runtime solver.

---

## 8. Development, testing & deployment

**Dev is testnet-first.** We develop against the real order server, real settlers, and a real
LiquidLane adapter on a public testnet, so the dev environment matches production one-for-one. The
local foundry loop (§8.3) is kept only for fast contract-unit iteration, not for integration.

### 8.1 Onboarding and prerequisites

The current dev onboarding is self-serve. Testnet uses `devintents.li.fi` and `order-dev.li.fi`; production
uses `intents.li.fi` and `order.li.fi`. Treat dev and production as separate environments: create and
register the identity in the target environment and do not assume a dev API key or registration is valid in
production.

#### Supported scope

| Supported | Rejected / out of scope |
|---|---|
| One configured EVM chain; same-chain input and output. | Cross-chain orders or a chain different from runtime config. |
| Already-opened `InputSettlerEscrowLIFI` order delivered over the LI.FI WebSocket. | Compact, Permit2, ERC-3009, gasless submit, and `openForAndFinalise`. |
| One ERC-20 input, one output, full fill. | Native input, multiple inputs/outputs, and partial fills. |
| `StandardOrder.inputOracle` and `MandateOutput.oracle` / `settler` identify the configured OutputSettler. | Unknown order settlers/oracles and non-empty output callback data. |
| The default strategy handles limit and exclusive-limit output contexts. | Dutch and exclusive Dutch are ignored globally. The default strategy rejects unknown or malformed contexts; a webhook strategy must decline every non-Dutch context it cannot resolve. |
| Immediate decide-and-send using current time and state. | Retaining or scheduling a future exclusive-limit order for later retry. |
| WebSocket discovery with an on-chain `Deposited` check before send. | On-chain event discovery or trusting WS status without the chain check. |

#### Ownership map

| Owner | Must provide |
|---|---|
| LI.FI | Solver identity/API key, executor-account registration, order server + WebSocket access, canonical OIF settler addresses, and support for the target chain. |
| Solver operator (us) | The executor owner EOA, an authorized caller EOA with gas funds, RPC access, a deployed EIP-1271 `LiquidLaneLifiExecutor`, YAML config, and monitoring. |
| LiquidLane adapter owner | A live adapter/route with capacity and rate data, plus direct filler authorization for our executor when direct execution is required. |
| Test order creator | A separate user EOA, input-token balance and approval, and the ability to open/fund an escrow order through `open`/`openFor`. The order server indexes that on-chain order. |

#### Required before startup

1. **Choose one chain and route.** The chain must be returned by the target order server's
   `/chains/supported` endpoint and host the LiquidLane adapter, input token, output token, and canonical
   LI.FI escrow InputSettler/OutputSettler. This solver is same-chain only; `fromChain == toChain`.
2. **Use the canonical settlers.** Put the target chain's opened-order `InputSettlerEscrowLIFI` and
   OutputSettler addresses in config. We do not deploy these contracts and we do not support Compact,
   Permit2/3009, gasless submit, or `openForAndFinalise` orders.
3. **Deploy our executor.** Deploy the ERC-1271-enabled `LiquidLaneLifiExecutor` implementation with immutable
   input/output settlers matching config, then a transparent proxy initialized with the owner EOA and an initial
   caller list containing the EOA from `signer.keyEnv`. Keep the proxy-admin owner separate and recorded. The
   proxy address is the configured and registered LI.FI solver account; the owner manages callers and callers
   can finalise. Configured adapters are the solver's trusted route scope.
4. **Create the API key and register the deployed executor.** Create the target-environment API key and fetch
   the server-issued message from `GET /api/v1/solver/register/message`. Compute its standard EVM
   `hashMessage(message)`, then sign the EIP-712 `LifiRegistration(bytes32 messageHash)` value with any current
   caller using domain `{ name: "LiquidLaneLifiExecutor", version: "1", chainId, verifyingContract: executor }`.
   Submit `POST /api/v1/solver/register` with `{ message, signature, account: executor,
   chain: "eip155:<chainId>" }`. LI.FI passes its message hash and the signature to
   `executor.isValidSignature`. Keep the key only in the environment named by `orderServer.apiKeyEnv`
   (normally `LIFI_SOLVER_API_KEY`). Under our deployment convention all processes using this executor
   share the key and reputation; use another executor and key for an independent deployment.
5. **Authorize LiquidLane execution.** For every configured adapter, verify its vault, output asset,
   redeemable input-token list, current capacity, and rate. Grant `setFiller(executor, true)` for direct
   routes. In `internal` mode a signed private-discount leg has its own authorization, but any direct
   fallback still needs filler authorization.
6. **Configure gas conversion.** Provide one native/USD Chainlink feed and one token/USD feed for every
   distinct adapter output asset. Set `gas.nativeMaxAge` and every `gas.tokenUsdFeeds[].maxAge`
   from the feed's heartbeat plus realistic publication slack; stale, non-positive, missing, or materially
   future-dated rounds fail the quote/fill decision closed.
7. **Optional private discounts.** `solverMode: external` needs no discount backend and serves only
   direct-authorized routes. `solverMode: internal` additionally requires a reachable
   `privateDiscountsUrl` and active signer/protocol policies for the configured adapters.

#### Deployment preparation

Before each testnet or production deployment, record enough information in the operator's normal release
process to reproduce and audit it: executor source revision and compiler settings, implementation constructor
arguments, proxy initializer arguments and proxy-admin owner, target chain, expected owner and settler addresses,
implementation/proxy addresses and transactions, verified runtime bytecode, LI.FI registration result, and
every adapter filler-authorization transaction. These values are
deployment-specific and are intentionally not pinned in this repository.

The minimum operator config is [`../config/lifi.example.yaml`](../config/lifi.example.yaml). Before
starting, replace every zero/placeholder address and provide these secrets without putting them in YAML:

| Environment | Config reference | Purpose |
|---|---|---|
| `SOLVER_PRIVATE_KEY` | `signer.keyEnv` | Authorized executor caller and tx sender; it may be separate from the owner. |
| `LIFI_SOLVER_API_KEY` | `orderServer.apiKeyEnv` | REST quote/supported-contract calls and WebSocket authentication. |
| RPC URL variables | `chain.rpcUrl` / optional write and fallback URLs | Current-state reads and transaction submission. |
| Chainlink feed variables | `gas.nativeUsdFeed`, `gas.tokenUsdFeeds[]` | Native gas cost converted into each output token; every feed has its own required max age. |
| `RFQ_BACKEND_URL` | `privateDiscountsUrl` | Required only for `solverMode: internal`. |

#### Startup preflight performed by the solver

Startup fails before quote publication when config is invalid, any configured adapter's `vault()`, vault
`asset()`, token list, or token decimals cannot be resolved, no adapter routes resolve, an output token has
no configured gas oracle, executor settler immutables do not match, the signer is not returned by
`executor.isCaller`, `InputSettler.governanceFee()` is non-zero or unreadable, the API
key does not list the executor as a registered solver identity, or external mode lacks direct filler
authorization. After those checks the solver reads `GET /api/v1/solver/supported-contracts`; if needed it
preserves the current lists and adds the configured escrow InputSettler and OutputSettler only to the
`inputSettler` and `outputSettler` lists with one replacement `PUT`. The `oracle` list is preserved unchanged
and may correctly remain empty.

#### Onboarding acceptance check

Onboarding is complete only when all of the following are observed in the target environment:

1. The process starts without route, oracle, executor, authorization, or supported-contract errors.
2. The order server accepts a non-empty same-chain quote whose `exclusiveFor` is the registered executor.
3. A user calls the canonical `open`/`openFor` path. The order server indexes the transaction without
   `POST /orders/submit` and delivers `user:vm-order-submit` with a full `StandardOrder` and
   `meta.onChainOrderId`; `quoteId` may be absent and no on-chain event listener is involved.
4. Immediately before submission the canonical order ID matches and on-chain status is `Deposited`.
5. The executor transaction succeeds atomically: input claim -> LiquidLane redemption -> OutputSettler
   fill/attestation. The user receives the required output and backend status becomes `Settled`.
6. Receipt gas is consistent with the conservative settlement constants and the submitted fee remains within
   the decision and global fee caps; any surplus is held by the executor.

### 8.2 Testnet dev environment (primary loop)
Target **Ethereum Sepolia** (chainId 11155111) — the intersection of LI.FI `order-dev` support, the
canonical OIF settlers (deployed there, §4 addresses), and the existing Symbiotic **LiquidLane adapter**
used by the redstone-oev / RFQ testbed.

The v1 route is **TCOL → TLOAN** (redstone-oev testbed): adapter `0xB5951fec…70b`, TCOL (RWA)
`0x17e892…A4D3`, TLOAN (underlying) `0x468BB3…4C9d`.

One-time setup:
1. **Contracts** — deploy the ERC-1271-enabled `LiquidLaneLifiExecutor` implementation and transparent proxy
   to Sepolia (`INPUT_SETTLER`/`OUTPUT_SETTLER` implementation immutables = the LI.FI addresses in §4;
   proxy initializer owner = admin EOA; initial callers include the framework signer).
2. **Solver identity** — register that deployed executor on `devintents.li.fi` through the V1 EIP-1271
   flow, and fund the framework caller EOA with Sepolia ETH.
3. **Filler auth** — the testbed owner `0x8124…7309` registers our executor as a filler on the adapter
   (`setFiller(executor, true)` / equivalent owner path).
4. **Config** — copy `config/lifi.example.yaml` into an operator-local config, point `orderServer` at
   `order-dev.li.fi`, and set the §4 settlers, deployed executor, TCOL->TLOAN adapter, RPC, and gas feeds.
   On first startup the solver preserves existing supported contracts and adds the escrow InputSettler plus
   the OutputSettler to their respective settler lists for `eip155:11155111`; it does not register the
   OutputSettler as an oracle.

The loop, on every change:
1. Run the bot → it submits an **exclusive** standing quote (`exclusiveFor = executor`) for the
   RWA→underlying route to `order-dev.li.fi`.
2. From a second user key, select the quote and call the canonical escrow `open`/`openFor` path. Do not call
   `POST /orders/submit`: the order server detects the on-chain order and delivers the full `StandardOrder`
   over WebSocket. For a manual run, use a `quoteTtl` long enough to complete quote selection and opening;
   keep the short rolling TTL in automated or production flows.
3. The order server matches it to our exclusive quote and pushes it over the WS feed → the bot prices
   it, calls `finaliseWithCurrentTimestamp`, and the executor finalises/redeems/fills it on Sepolia in
   the same tx.
4. Inspect the tx (redeem → fill → attest), the user's received output, and the executor's accrued
   surplus. Iterate.

This exercises the full real path — order server, WS, settlers, adapter, txmanager — every iteration.

### 8.3 Local contract loop (fast iteration only)
For quick Solidity iteration without a network: foundry/anvil, self-deploy the OIF settlers + a
real/mock adapter, open/fund an order, and drive the opened-order finalise path — the shape of Catalyst's
`InputSettler7683LIFI.samechain.t.sol`. This is the `forge test` unit/integration coverage of the
executor, **not** the integration loop (§8.2 is). The Go side is unit-tested against an `httptest`
order-server mock + a simulated/forked chain backend.

### 8.4 Mainnet deployment
- **We do not deploy the settlers** — LI.FI/OIF canonical deployments at fixed addresses.
- **Per chain we deploy** `LiquidLaneLifiExecutor` (+ register the executor as a filler on each target
  LiquidLane adapter), register that executor with LI.FI through EIP-1271, fund its runtime caller, and run
  the bot — the same steps as §8.2 but against
  `order.li.fi`. LI.FI is live on Ethereum, Base, Optimism, Arbitrum, Polygon, BSC, Katana, MegaETH,
  etc. (`order.li.fi/chains/supported` authoritative); v1 targets the chain(s) hosting the LiquidLane
  RWA adapters we serve.
- No official solver SDK — integrate against the order-server OpenAPI (`/docs`) + WS feed (+ the MCP
  server for scripted testing).

---

## 9. Build phases

Testnet-first: the executor is developed from P0 so every later phase integrates against the live
`order-dev.li.fi` + real settlers + real adapter (§8.2). The opened-order callback flow was proven through
a settled Sepolia order using the previous deployed executor; the latest split direct/discount route ABI
still requires the redeploy in phase 0.

0. **Done locally; Sepolia redeploy required** — `LiquidLaneLifiExecutor` implements domain-separated ERC-1271
   registration through its caller set and caller-gated runtime authorization in §3. Its Foundry unit suite
   and the real-settler Sepolia fork test pass. Deploy to Ethereum
   Sepolia, register it with LI.FI through EIP-1271, and register it as an adapter filler.
   The vendored ABI and Go binding are generated from the contract artifact at
   RFQ `main` commit `8b970bd`, including the split direct/discount route interface.
1. **Done locally** Order-server client — the vendored `openapi/lifi-order.openapi.json` + generated `api/lifiorder`
   client (register / `quotes/submit` / `orders`) plus a thin hand-written WS client for
   `user:vm-order-submit`, wired to the live `order-dev.li.fi`; register the executor account; config parsing
   + framework wiring (`solver.Register`, blank-import). `httptest`-backed unit tests, validated live.
2. **Done locally; previous ABI live Sepolia happy path proven** Pricing + decision + tx build — `default` strategy
   (direct executable getMaxRate for quotes; getAmountOut/minDiscount/getMaxAssets for fills;
   Chainlink gas conversion snapshots, code-owned settlement/private gas constants, pair-level route
   ladders, all live direct/private alternatives per route, shared capacity and shared LI.FI/UniswapX
   LiquidLane fill planning,
   asset match, immediate OutputSettlerSimple context resolution
   for limit and exclusive-limit outputs, with Dutch contexts rejected at WebSocket admission);
   executor-as-solver typed direct `FillRoute[]` plus discount-backed `DiscountRoute[]` finalise calldata;
   early/final `orderStatus == Deposited` checks; latest-state snapshots; raw live txmanager fee input; dynamic
   ranges; quote reconciliation; bounded replay-coalescing fill handoff, sequential nonce broadcast,
   pending-capacity-aware one-shot planning, confirmation-time reservation release, and execution-time contract validation.
   The ladder is quote-only: an awarded order is greedily replanned from current amount-specific quotes,
   and output above the resolved order amount remains in the executor; the current ABI has no sweep entrypoint.
   Unit-tested through the solver-level submit path and validated end-to-end on Sepolia with a
   WebSocket-delivered on-chain order matched to our exclusive quote: open tx
   `0xd3f619048a745fb896c2f6c8b4e3b42a65b104eb3035b2bb9c20cf9593623480`, fill tx
   `0x338aef70060093f6341538cd633fd4b5cfecc2fe2a10d77458946bf0e84fe960`, backend status `Settled`.
   The new executor asks direct adapters for the buffered `amountOut`, executes private routes from signed
   terms, and delegates context resolution and output sufficiency to the OutputSettler. If the order is not
   executable at decision time, it is dropped.
3. **Harden** — staleness/skip edge cases, revert handling, metrics on the shared observability server;
   a repeatable green E2E on Sepolia (`order-dev`).
4. **Mainnet** — deploy the executor per target chain, point config at `order.li.fi`, register each
   executor account through EIP-1271, and run.

---

## 10. Open items

- **Gas calibration: direct-finalise rerun required.** The previous signature-based executor's 51-test
  suite measured a maximum `finaliseWithCurrentTimestamp` call of 478,838 gas. Re-run Foundry gas reports
  after the direct-finalise contract cutover and compare the first Sepolia receipts before changing the
  conservative Go settlement constants.
  The first acquire-route budgets are 550k direct and 625k private from
  `250k fixed + LiquidLane route units (+75k private)`. Multi-route callback behavior was also exercised.
  Compare the first Sepolia receipts against these constants before mainnet rollout.
- **Executor redeploy** — deploy the split-route implementation recorded in phase 0 plus transparent proxy,
  initialize the runtime signer as a caller, register the proxy address with LI.FI, update config, and grant
  the proxy adapter filler authorization before E2E. Confirm the canonical InputSettler reports
  `governanceFee() == 0`; startup fails closed otherwise, and every admitted order rechecks it.
- **Opened-order callback flow: previously confirmed on Sepolia; contract-identity rerun required.** The
  executor uses the same opened-order callback path, but `finaliseWithCurrentTimestamp` now calls
  `InputSettler.finalise` as the registered solver contract, then receives/redeems inputs and
  fills/attests output via `orderFinalised(uint256[2][] inputs, bytes call)` in the same transaction.
  It is **opt-in** ("your solver has to support `orderFinalised`"). **Opt-in mechanism = resolved:** the solver
  ensures the escrow InputSettler and OutputSettler under their respective supported-contract kinds as
  described in §5.1 and §8.1; it leaves `oracle[]` unchanged, and an empty list is valid. The executor is the
  registered solver, finalise caller, and callback destination. The previous EOA-identity build proved the
  remainder of the live path: `order-dev`
  delivered an already-opened/funded escrow order over `user:vm-order-submit`, and the resulting Sepolia
  fill reached backend status `Settled`. Repeat that E2E after deploying and registering the new executor.
  The live feed may omit `orderType`; admission
  therefore requires `meta.onChainOrderId` and relies on the full escrow order plus canonical on-chain
  `Deposited` status. We do not support gasless
  Compact or permit2/3009 opening.
- **Wire schemas: RESOLVED** — the live order-server OpenAPI is valid 3.1, vendored at
  `openapi/lifi-order.openapi.json`, and generates `api/lifiorder` directly without a normalization shim.
  The WebSocket `user:vm-order-submit` event is outside the OpenAPI; the confirmed dev connection uses
  `wss://order-dev.li.fi`, `x-api-key`, and application-level `ping`/`pong`. Its opened-order payload and
  optional `quoteId` behavior are captured in §5.1.
- **Adapter filler registration** — our executor must be granted filler rights on each LiquidLane
  adapter (`setFiller(executor, true)` / equivalent owner path), by the adapter's vault creator.
  Onboarding prereq.
- **Sepolia testnet adapter — resolved (v1 dev route).** The redstone-oev testbed provides a usable
  Sepolia LiquidLane adapter: `0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b` (vault
  `0xb99F1FeA50f40Bb7C5E568c2De6D79dd0b61EB3A`), redeeming **TCOL** `0x17e892d4E802B01d7DA49Ca3542560f6851AA4D3`
  (RWA) → **TLOAN** `0x468BB3245BF520a0CD030BDE029c98aCEAF84C9d` (underlying). v1 same-chain route =
  TCOL→TLOAN on Ethereum Sepolia. Confirm the exact `tokenToRedeem`/underlying and that direct redemption
  (not only the OEV-seizure path) is permitted, via adapter reads. **Filler auth:** the testbed owner
  `0x812492C36b003837C30cB0B63960b86eC9B27309` grants our executor `isFiller` on the adapter.
- **v1 scope** — TCOL→TLOAN on Sepolia for dev; the mainnet RWA↔underlying adapter(s)/token(s)/chain(s)
  are a launch decision once the Sepolia loop is proven.
- **Reputation thresholds** — the numeric fill-rate/speed cutoffs that gate exclusive orders are
  undocumented; `exclusiveFor` on our own quotes should make this moot for v1.
- **`getMaxAssets` is non-view** (mutates) — read via a call, not a static-call, in the pricing path
  (mirror how the RFQ solver handles it).
- **Private-discount deployment config** — internal mode needs the reachable RFQ/private-discounts
  backend URL and live signer/protocol policies for the configured adapters. The code path is complete;
  Sepolia E2E still needs a real advertised discount and newly deployed executor ABI.
