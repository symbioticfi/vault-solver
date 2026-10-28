# vault-solver — Solver-Local Strategy Architecture

The **strategy** is the decision-making core of a solver. This document records the *generic* strategy
boundary shared by every solver. Concrete per-solver contracts (the actual input/output types) live in
that solver's own plan under `docs/` and in its `strategies/types` package — never here.

## Core principle

A solver splits into two parts:

- **The solver skeleton** — everything that faces the outside world: discovering work, reading
  chain/API state, validating untrusted protocol data, and normalizing it into the shared domain types
  the strategy consumes, then signing and submitting whatever the strategy decides. It holds the key
  and moves funds, but it makes no economic decision.
- **The strategy** — the brain. Given the solver's input snapshot, it decides what to do and returns a
  concrete, ready-to-execute plan.

The flow is one-directional and trusted:

```
solver maps external state to typed facts  →  strategy decides  →  solver executes the output
```

**The strategy is trusted for economic decisions, and it is the core of the solver.** The solver does
not re-price, clamp, re-rank, or replace the strategy's allocation. Pricing, sizing, ranking, and route
selection live *inside* the strategy implementation. Before moving funds, the solver still verifies
execution integrity against the fresh snapshot it supplied: route identity, token pair, exact input
coverage, achievable output, shared capacity, gas floor, and protocol timing. Protocol parsing,
token/address admission, freshness reads, replay, caching, and recovery also stay in the solver
skeleton. This keeps the boundary crisp: swapping in a different strategy (including an external one)
never adds economic decision logic to the solver, while malformed or stale calldata cannot cross the
execution boundary.

Concretely:

- The solver provides **typed facts, not decisions**. Protocol DTOs and contract return values are
  converted once, at the solver boundary, into canonical entities such as `liquidlane.Route`,
  `Inventory`, `QuoteCandidate`, and `FillQuote`. It does not rank routes or choose an allocation.
- The strategy returns a **complete economic plan**. The solver canonicalizes it against the supplied
  snapshot and rejects inconsistencies; it never changes which routes won or their economics. The
  solver also owns transaction-only values such as nonce, signature, and EIP-712 domain, which the
  strategy can never supply.

## The contract

Each solver defines its own decision interface — one method per decision point — in its own
`strategies/types` package. **There is no single cross-solver `Strategy` type: each solver's interface
is unique to its workflow** (a quote/fill solver's differs from an auction solver's, which differs from
a bidding solver's). What every solver shares is the *pattern*, not the signature — a solver-built
input of validated domain facts in, a strategy-decided output out.

For direction, the 3F solver's interface looks like this:

```go
// package types — internal/solvers/bridgefacilitator/strategies/types
type Strategy interface {
    DecideOffers(ctx, OfferInput) (OfferOutput, error)
}
```

where `OfferInput` carries only raw facts (adapter liquidity/caps, open auctions, offers already held)
and `OfferOutput` is the list of offers for the solver to sign and submit. A quote/fill solver instead
exposes a quote decision and a fill decision; a bidding solver a single bid decision. The concrete
types are documented in each solver's plan (`docs/3F-PLAN.md`, `docs/RFQ-PLAN.md`, …) and defined in
its `strategies/types` package — this document intentionally does not restate them.

Solvers that use LiquidLane liquidity also follow
[`LIQUIDLANE-CONVENTIONS.md`](LIQUIDLANE-CONVENTIONS.md): shared LiquidLane packages define
read-side facts (`Route`, `Inventory`, `QuoteCandidate`, `FillQuote`, authorization, ids, freshness). The shared snapshot
reader composes direct and physical state plus optional gas facts for LI.FI and UniswapX. OEV consumes the
same optional oracle facts while retaining its protocol-specific signed gas cap and deposit checks. RFQ-like exact-input
paths can normalize amount-independent inventory against current per-route oracle quotes. The RFQ solver
performs that protocol-to-LiquidLane normalization before calling its strategy; UniswapX and LI.FI already
enter their strategies as typed LiquidLane inventory. Their default strategies build the same `QuoteTask`; RFQ, LI.FI, and
UniswapX normalize fresh execution facts into the same `FillTask`. The shared engine owns LiquidLane
route selection, capacity, input coverage, buffer, minimum-output, and gas calculations. Each solver
still owns candidate discovery, protocol input/output mapping, lifecycle, strategy interface, calldata,
and the fixed gas envelope around its protocol-specific executor call.

## Selection and configuration

Strategy selection is solver-local: the generic framework does not parse, validate, or route strategy
configs. It owns only solver lifecycle.

```yaml
solvers:
  - name: <solver>
    config:
      strategy:
        name: default        # solver-local strategy name (omit ⇒ default)
        config: {}            # opaque to the framework and the solver skeleton
```

`solvers[].config` is opaque to the framework; each solver decides whether it supports a `strategy`
field and which names exist. Inside a solver:

```go
type StrategySpec struct {
    Name   string
    Config yaml.Node
}

type StrategyFactory func(raw yaml.Node) (types.Strategy, error)
```

Each solver keeps a local registry/factory. A strategy self-registers from its own package `init()`
under a solver-local unique name; the solver-level factory only routes by `name`, and the selected
strategy owns parsing and validating its own `config` node.

## Built-in strategies

Two strategy kinds are conventional across solvers:

- **`default`** — the in-process strategy that ships with the solver. It is the reference decision
  logic and needs no external service. It may validate its own output as thoroughly as it likes; that
  is internal to the strategy.
- **`webhook`** — delegates the decision to an external HTTP service. It POSTs the solver's input
  snapshot as JSON and hands back the plan the service returns. The external decider is the brain; the
  in-process handler is transport-only and adds no economic decision logic of its own.
  This is the seam for running custom decision logic out-of-process without forking the solver.

UniswapX also registers a local `single` strategy using the shared LiquidLane
[single-source algorithm](#single-source-selection). Strategy registration remains solver-local.

These strategies plug into the same decision boundary: the solver validates and executes their output the same
way, so a solver is never coupled to which strategy is loaded. RFQ and LI.FI share `internal/tokenpolicy`
for `tokensToQuote` admission. Both mark admitted inputs as single-route only in `permissioned` scope
and reject strategy output that aggregates routes; route selection and economics remain strategy-owned.

## Adding your own strategy

Strategies are pluggable per solver, and there are two ways to add one.

**Out-of-tree, no Go changes — run a `webhook`.** Point the solver's `strategy` at your own HTTP
service (see below). It receives the solver's raw-facts input as JSON and returns the plan to execute;
the solver runs it subject to the same solver-owned structural and safety constraints as an in-tree
strategy. This is the fastest path and keeps your decision logic in your own
codebase and language.

**In-tree — register a new strategy on the solver.** To ship a strategy alongside a solver, implement
that solver's interface (each is unique — you implement the one the target solver defines):

1. Create a package under the solver's `strategies/<name>/` and implement the solver's strategy
   interface (e.g. `DecideOffers` for 3F), consuming its `strategies/types` input and returning its
   output type.
2. Add a `NewFromConfig(raw yaml.Node) (types.Strategy, error)` constructor that parses your own
   `strategy.config` node — the framework hands it to you opaque, so you own its schema and validation.
3. Self-register from your package `init()` via the solver's local
   `strategies.Register("<name>", NewFromConfig)`, under a name unique within that solver.
4. Ensure the package is imported so its `init()` runs (blank-import it where the solver wires its
   strategies).
5. Select it in config: `strategy: { name: <name>, config: { … } }`.

Either way the solver skeleton is untouched: it provides the same input and executes whatever plan your
strategy returns — so the correctness of the decision is entirely yours to own.

LiquidLane quote/fill strategies intentionally receive no chain client or logger through their registry:
all current reads are represented in the typed input. A different workflow may define explicit
strategy-owned dependencies only when the strategy itself genuinely owns that I/O.

## Shared LiquidLane strategies: `internal/liquidlane/strategies`

Strategy interfaces, implementations, and registration belong to each solver. LiquidLane provides
pure calculations: `greedy.SolveQuote` / `greedy.SolveFill` and `single.SolveQuote` / `single.SolveFill`.
Their shared task/result structures, gas arithmetic, and fill validation live in
`internal/liquidlane/strategies`.
RFQ and LI.FI call greedy directly. UniswapX's local implementation binds the quote/fill functions and
gas policy once during construction, sharing protocol adaptation between `default` and `single`.
Unsupported local strategy names fail at initialization.

`FillSolution` snapshots selected `FillRoute`s and their gas cost, exposes the available net output,
and distributes the required output through `Finalize` without changing its allocation. Returned amounts
are independent copies. Candidate selection and capacity allocation remain algorithm-owned.

`QuoteTask` accepts normalized, already-priced candidates, an exact input or output, route limit, buffer,
an optional gas pricing model, and an explicit input-coverage rule. Greedy quote solving owns deterministic
ranking, direct/private alternative selection, route splitting, gas deduction, and fixed-point sizing.
Exact input uses the RFQ-style forward allocator. Exact output uses the same one-pass greedy route selection in output units, adds buffer
and gas, and converts each selected output leg directly to input with upward rounding. It neither binary
searches input nor enumerates route combinations; harmless excess output is executor surplus. RFQ
supplies the price-impact coverage rule without gas pricing; UniswapX supplies strict coverage plus its
buffer and gas pricing.

LI.FI adapts the same exact-input task to its standing range wire format. It solves each geometric range at
both endpoints, caps each endpoint at the largest fixed-point rate that cannot overquote its integer output,
then applies a linear conservative floor over route alternatives, worst-case complete-plan gas, and
rounding. Every emitted minimum must still map to positive integer output.

For fills, RFQ, LI.FI, and UniswapX pass current amount-specific `FillQuote`s to `SolveFill`. `FillTask`
also carries pending `CapacityID` reservations, freshness, route limit, price buffer,
input coverage, and an optional gas pricing model. The engine selects routes, enforces shared capacity,
charges complete-plan gas once when that model is present, and returns `FillSolution`, exposing
`MaxAmountOut` followed by
`Finalize(requiredAmountOut)`. RFQ maps it to Executor legs without introducing RFQ gas config; LI.FI
resolves OIF `OutputContext`/`FillAfter`; UniswapX resolves signed-order output/deadline. LI.FI and
UniswapX build the gas model from their existing runtime facts.
The canonical `FillRoute` and fill-plan validation live in `strategies`. Optional `CapacityLimits`
separate physical vault budgets from each source's limit. With these budgets, allocation and validation
deduct pending reservations from both the domain and each source: `MaxAssets` can include adapter or
delegator limits, and the ledger does not attribute reservations to adapters. This conservatively reduces
availability even for a source another fill did not use. UniswapX supplies these budgets to every fill
strategy and retains them across source filtering. Omitting them preserves existing RFQ and LI.FI behavior.

Each solver owns its `liquidlane.CapacityLedger` and reservation lifecycle. The ledger supports atomic
replacement against a revision; UniswapX uses this before preflight and rechecks revisions before
returning quotes. All UniswapX strategies can quote unreserved liquidity while the sender is busy.
RFQ keeps its existing reservation accounting; LI.FI keeps its busy-lane quote gate, with a follow-up
tracked in [its open items](LIFI-PLAN.md#10-open-items). Nonce conflicts still block quoting.
Inventory reads remain ordinary latest-state Multicalls. Caches, refresh triggers, retries, and
reservation release remain solver-owned; reservations are not shared across solvers or processes.

### Single-source selection

`internal/liquidlane/strategies/single` provides protocol-neutral `SolveQuote` and `SolveFill` functions.
They reuse greedy pricing, sizing, capacity, and gas calculations for each candidate separately.
One candidate must cover the whole request, even when a caller otherwise permits absorbing uncovered input.
Quotes compare final output for exact input or required input for exact output, with candidate-ID ties.
A higher rate cannot compensate for insufficient volume. Fill solving also evaluates complete sources
individually; the protocol adapter applies the order's required output to the selected solution.
UniswapX prepares both local strategies through `greedy.ReserveInventoryCapacity`, subtracting pending
reservations from each source after its inventory reserve. Default then divides shared capacity across
routes; single evaluates each remaining source without fixed shares between alternatives. Only one
alternative can be selected, and quotes do not reserve liquidity.

The caller supplies eligible sources, capacity budgets, quote buffer, and any gas pricing. Source modes,
quote-to-order source preferences, fresh-source retries, and protocol deadlines belong to each solver's
integration, not to the shared selection algorithm. The single-source algorithm has no UniswapX dependency.

Current local integrations are:

| Solver | Local strategy names | Shared LiquidLane algorithm |
|---|---|---|
| RFQ | `default` | `greedy` |
| LI.FI | `default` | `greedy` with solver-owned standing quote ranges |
| UniswapX | `default`, `single` | `greedy`, `single` |

RFQ and LI.FI reject `strategy.name: single` during initialization; their quote contracts need separate
integration. UniswapX source preferences, fallback, and gas policy are described in its
[single-source integration](UNISWAPX-PLAN.md#single-source-integration). RedStone OEV is unchanged.

All integrations bind their static event/outcome, amount-kind, state-view, and external-operation labels
through the generic workflow metric families. Solver packages still own those protocol-specific enums and
unique gauges/histograms; the framework does not know them. RFQ, LI.FI, and UniswapX share the same
`fill/success` event and token-native amount kinds; RFQ and UniswapX additionally classify `failure`,
`not_admitted`, and `obsolete` (the order was settled elsewhere before our fill landed). UniswapX records pre-submission declines separately as `fill/declined`.
Unknown event/outcome, amount-kind, or state-view observations increment one bounded
contract-drift counter instead of disappearing silently. Detailed gas, fee, and transaction lifecycle
accounting remains in the [shared transaction manager](TXMANAGER-PLAN.md#8-observability).
The generic HTTP chain transport records bounded logical requests and endpoint attempts by read/write/shared
role, method, ordinal endpoint, and outcome; configured URLs and error strings never become labels.
`solver_bot_solver_info{solver}` exposes bounded config-time membership for fleet joins.

A process is one execution lane: one chain client (primary/fallback reads and optional private write endpoint), one signer, and one nonce-serialized txmanager shared by its configured solvers. Different
signer/RPC tuples run as separate processes with disjoint solver subsets and unique Prometheus `instance`
(or deployment-supplied `lane`) target labels. Committed dashboards use the standard Kubernetes `pod` target
label and query namespace/pod options from Prometheus rather than embedding deployment names. Application metrics do not carry URLs or deployment names.
The [shared manager ownership contract](TXMANAGER-PLAN.md#1-ownership-and-admission) serializes local
signed lifecycles and reads the latest mined nonce before every new send. Fresh requests use the lowest
unconsumed nonce, including after restart. Accepted/transport-uncertain abandonment retains a fee hint
for at most `pendingTimeoutMs`; fresh work discards it if its required bump cannot fit the global fee
ceiling. A lower request ceiling skips the hint for that send without discarding or renewing it, so
later affordable work retains the known replacement floor until its original expiry.
Rejected initial fees neither create nor ratchet that hint. Several processes can share the EOA, but nonces and off-chain commitments are not allocated atomically across processes; see the
[replica plan](REPLICA-PLAN.md).

The manager abandons tracking at the pending timeout, request deadline or terminal business-status check
without sending a cancellation transaction. Execution remains unknown. RFQ owns the retry policy: backend
reconciliation followed by one configured poll interval and a fresh open-order poll can re-enter fill planning.
Accepted uncertain outcomes use its `maxNonceRetries` budget. Generic `FailedCall` and
`SafeERC20FailedOperation` errors, unknown and malformed unsigned estimate
reverts have a separate retry count bounded by the same setting; exhaustion logs Error once and retires
the unsigned failure so stale open listings cannot re-arm it. Known Reactor/Executor setup
errors fail with Error once after backend reconciliation, `ExpiredRequest` expires, and indexed peer
completion takes precedence. Missing views keep bounded observation without another estimate. Initial
nonce collisions use the same backend reconciliation, poll delay and order deadline without spending
that budget. Each retry resolves the executable order,
chain inputs, strategy plan, and discount signatures again. Retry budgets remain process-local; neither strategy code
nor the generic manager decides whether to replay a protocol order.

The adjacent `internal/liquidlane/discounts` package owns the discount rules shared by RFQ, LI.FI, and
UniswapX: parse and filter live offers, bind offers to physical routes, price them from the signed discount, cap capacity,
derive amount-specific candidates, and revalidate resolved id/adapter/token/deadlines plus the current
output floor. Resolution timing is deliberately not hidden behind a common strategy facade: LI.FI
pre-resolves and refreshes state, while UniswapX and RFQ resolve selected routes. Each solver maps the
validated `discounts.Signed` into its own generated executor binding.

## Shared transport: `internal/webhook`

`internal/webhook` is a generic HTTP JSON client:

- HTTP JSON `POST`, configurable timeout, request/response body byte caps (default 1 MiB each)
- literal or env-backed headers (parsed config retains only the env-var name; `NewClient` resolves it)
- strict response decode; non-2xx and empty-body responses are errors
- typed non-2xx status errors, so each solver strategy can distinguish permanent input rejection from a
  retryable endpoint failure without putting protocol policy in the shared client

It has no solver names, no strategy registry, and no per-solver DTOs — each solver's webhook strategy
owns its own wire types (conventionally lower-camel JSON with decimal strings for big integers,
provided by that solver's `strategies/types`).

```yaml
strategy:
  name: webhook
  config:
    url: https://strategy.example.com/decide
    timeout: 500ms
    maxRequestBytes: 1048576
    maxResponseBytes: 1048576
    headers:
      authorization:
        env: STRATEGY_AUTH_HEADER
```


## Merkle SDK host boundary

The Merkle host consumes independently versioned SDK identity discovery, active-root
permission fetching, recipe input derivation, bounded recipe evidence caching,
pinned batched reads and state-override numeric preparation. It rejects manual
account/tree/input configuration and fingerprints its derived account, signer,
curator source and runtime policy version for recovery. Complete original-code
simulation and canonical block fences remain mandatory. The shared lens replays
authenticated state-changing prefixes before later numerical reads; unresolved
provider outputs, future input sources and executor self-queries fail closed.
The host registers the SDK's `providers/aave` capacity provider before generic
recipe input derivation. It replays authenticated supply prefixes and reads Aave
capacity through a pinned eth_call code override; the committed recipe chooses
the actual amount and enforces its health floor. The implementation is shared
with the executor daemon; vault-solver contains no Aave sizing implementation.
Adding this provider changes the durable roster. Finish or reconcile active
Merkle jobs before upgrading; recovery rejects incompatible pending journals
without clearing their signer ownership.
Canonical permission bundles may include unused `"nonzero"` payable markers;
selecting such a leaf for recipe execution remains unsupported.

The immutable SDK also classifies successful original execution results using the
authenticated recipe: all Manage gates closed with no Call or Approve work is idle.
Request IDs and bound arguments do not supply readiness policy. Unsigned admission
rechecks the gates; malformed results, reverts and closing canonical-check failures
remain errors. The existing signed journal recovery path remains unchanged, and
on-chain completion assertions remain part of the recipe rather than host protocol
recognition.

The host's SDK transport continues to use the existing shared transaction manager's
owner fence and persistence callbacks; it contains no copied sender or nonce engine.
The SDK batch engine uses the existing HTTP fallback/tracing path with strict response
identity validation. Merkle requires HTTP(S) read endpoints while other solvers keep
ordinary WebSocket/IPC support. Vault-side LL/3F pricing, discounts and auction/order
business logic stay in their existing integration packages; they are not implicitly
wired through the Merkle host. Executor-daemon Merkl/CoW provider integration remains
owned by merkle-executor.


`rfq-merkle` separately consumes awarded RFQ orders through connector recipes. It
uses no adapter inventory, economic strategy or shared capacity aggregation. The
signed order supplies amounts; the SDK authenticates the immutable program and
simulates the complete outer RFQ settlement. The Merkle hosts share
`internal/merklesender`, which forwards owner fences, signed-byte persistence and
optional execution deadlines to the single txmanager. See [RFQ-MERKLE.md](RFQ-MERKLE.md).

`threef-merkle` independently owns one Merkle recipe executor for one curator BF/default
maker. Its recipe contains 1..32 distinct guarded funding/underlying markets, with
one API client, planner, provider, exclusive store and sender. One Run goroutine
maintains existing commitments before scanning open/solvable auctions for new bids
while the sender is idle. Positive and negative membership are memoized only at the
current pin; optional new-bid simulations are bounded by the configured
`maxEligibilityChecksPerTick` and resume from an in-memory cursor on the next tick.
The account-level durable intent serializes self-originated offers across markets. Its API-key PUSH offer loop uses authenticated recipe/policy scope and idle
account cash; its SDK provider binds native aggregate mint authorizations. The
provider authenticates canonical mint principal identities before validating funding
activation. Funding gates accept direct principals and arithmetic Results with a
proven zero-principal dependency, including the frontend's bounded cash shortfall.
The proof tracks each market separately through checked arithmetic; reads are
unconstrained. Canonical mint gates still use their own principal input directly.
The generic `merkle` solver has no 3F configuration or provider dispatch. All 3F Merkle
code lives in one package; API, reservation and durable intent checks remain inside
that integration. Both recipe keepers share store/sender startup through
`internal/merklesender.OpenKeeper`, including recovery validation before Run.
See [THREEF-MERKLE.md](THREEF-MERKLE.md) for the flow and validation boundaries.
