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

```

Each solver selects its built-ins explicitly in `strategy.go`. Constructors parse their
own deferred config. There are no mutable registries, side-effect imports, or initialization-order
requirements; strategy names remain local to each solver.

## Built-in strategies

Two strategy kinds are conventional across solvers:

- **`default`** — the in-process strategy that ships with the solver. It is the reference decision
  logic and needs no external service. It may validate its own output as thoroughly as it likes; that
  is internal to the strategy.
- **`webhook`** — delegates the decision to an external HTTP service. It POSTs the solver's input
  snapshot as JSON and hands back the plan the service returns. The external decider is the brain; the
  in-process handler is transport-only and adds no economic decision logic of its own.
  This is the seam for running custom decision logic out-of-process without forking the solver.

Both plug into the same decision boundary: the solver validates and executes their output the same
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

**In-tree — add a strategy to the solver selector.** To ship a strategy alongside a solver, implement
that solver's interface (each is unique — you implement the one the target solver defines):

1. Create a package under the solver's `strategies/<name>/` and implement the solver's strategy
   interface (e.g. `DecideOffers` for 3F), consuming its `strategies/types` input and returning its
   output type.
2. Add a `NewFromConfig(raw yaml.Node) (types.Strategy, error)` constructor that parses your own
   `strategy.config` node — the framework hands it to you opaque, so you own its schema and validation.
3. Add the constructor to the solver's `strategy.go` switch.
4. Declare any strategy policy there (for example, whether OEV requires an explicit bid cap).
5. Select it in config: `strategy: { name: <name>, config: { … } }`.

Either way the solver skeleton is untouched: it provides the same input and executes whatever plan your
strategy returns — so the correctness of the decision is entirely yours to own.

LiquidLane quote/fill strategies intentionally receive no chain client or logger through their constructors:
all current reads are represented in the typed input. A different workflow may define explicit
strategy-owned dependencies only when the strategy itself genuinely owns that I/O.

## Shared LiquidLane strategy: `internal/liquidlane/planning`

`internal/liquidlane/planning` owns quote and fill allocation, capacity partitioning, plan validation,
rounding and optional gas pricing. Quote and fill decisions share one package and no longer cross a
parent/child strategy hierarchy. Sharing this pure decision engine
does not create a cross-solver `Strategy` facade. `QuotePool` groups normalized, already-priced
candidates by physical route once and retains their deterministic ranking. LI.FI and UniswapX
use `NormalizeFixedInventory` to turn capacity-partitioned inventory into these candidates, including
the private-route buffer and conservative integer input capacity. RFQ uses `NormalizeOracleInventory`
with its amount-specific physical observation. Each `QuoteTask` supplies
an exact input or output, route limit, buffer, optional gas pricing and an explicit input-coverage rule.
`QuotePool.Solve` owns direct/private
alternative selection, route splitting, gas deduction, and fixed-point sizing. Exact input uses the RFQ-style
forward allocator. Exact output uses the same one-pass greedy route selection in output units, adds buffer
and gas, and converts each selected output leg directly to input with upward rounding. It neither binary
searches input nor enumerates route combinations; harmless excess output is executor surplus. RFQ
supplies the price-impact coverage rule without gas pricing; UniswapX supplies strict coverage plus its
buffer and gas pricing.

LI.FI adapts the same exact-input task to its standing range wire format. It solves each geometric range at
both endpoints, caps each endpoint at the largest fixed-point rate that cannot overquote its integer output,
then applies a linear conservative floor over route alternatives, worst-case complete-plan gas, and
rounding. Every emitted minimum must still map to positive integer output.

For fills, RFQ, LI.FI, and UniswapX pass current amount-specific `FillQuote`s to `SolveFill`. `FillTask`
also carries pending `CapacityID` reservations, freshness, route limit, buffer, input coverage, and an
optional gas pricing model. The engine selects routes, enforces shared capacity, charges complete-plan
gas once when that model is present, and returns `FillSolution`, exposing `MaxAmountOut` followed by
`Finalize(requiredAmountOut)`. RFQ maps it to Executor legs without introducing RFQ gas config; LI.FI
resolves OIF `OutputContext`/`FillAfter`; UniswapX resolves signed-order output/deadline. LI.FI and
UniswapX build the gas model from their existing runtime facts.
The canonical `FillRoute` and webhook fill validator are shared one level above `greedy`; the solver-owned
pending-capacity ledger lives in `internal/liquidlane`. Allocation policy remains replaceable, while local and remote strategies use the same
route identity, capacity, amount, and gas-floor invariants.
Public strategy interfaces, webhook DTOs, caches, protocol lifecycle, and calldata remain solver-local.

All integrations bind their static event/outcome, amount-kind, state-view, and external-operation labels
through the generic workflow metric families. Solver packages still own those protocol-specific enums and
unique gauges/histograms; the framework does not know them. RFQ, LI.FI, and UniswapX share the same
`fill/success` event and token-native amount kinds; RFQ and UniswapX additionally classify `failure` and
`not_admitted`. Unknown event/outcome, amount-kind, or state-view observations increment one bounded
contract-drift counter instead of disappearing silently. Detailed gas, fee, and transaction lifecycle
accounting remains in txmanager.
The generic HTTP chain transport records bounded logical requests and endpoint attempts by read/write/shared
role, method, ordinal endpoint, and outcome; configured URLs and error strings never become labels. An active
`txmanager` periodically installs one complete sender balance/latest-nonce/pending-nonce telemetry snapshot.
Nonce reads stay pinned to the write endpoint; balance tries it first and falls back to the ordinary read client
for submission-only relays. The previous complete snapshot is retained on failure. One locked collector emits the account
identity, refresh counters, values, and freshness from a single scrape-consistent state; processes whose solvers
do not start `txmanager` emit no txmanager account series. `solver_bot_solver_info{solver}` exposes bounded
config-time membership for fleet joins.

A process is one execution lane: one chain client (primary/fallback read set plus optional private write
endpoint), one signer, and one nonce-serialized txmanager shared by its configured solvers. Different
signer/RPC tuples run as separate processes with disjoint solver subsets and unique Prometheus `instance`
(or deployment-supplied `lane`) target labels. Committed dashboards use the standard Kubernetes `pod` target
label and query namespace/pod options from Prometheus rather than embedding deployment names. Application metrics do not carry URLs or deployment names.
The same EOA must not appear in two lanes because independent txmanagers cannot coordinate its nonce.

The adjacent `internal/liquidlane/discounts` package owns the discount rules shared by RFQ, LI.FI, and
UniswapX: parse and filter live offers, bind offers to physical routes, cap advertised rate/capacity,
derive amount-specific candidates, and revalidate resolved id/adapter/token/deadlines plus the current
output floor. Resolution timing is deliberately not hidden behind a common strategy facade: LI.FI
pre-resolves and refreshes state, while UniswapX and RFQ resolve selected routes. Each solver maps the
validated `discounts.Signed` into its own generated executor binding.

## Shared transport: `internal/webhook`

`internal/webhook` is a generic HTTP JSON client:

- HTTP JSON `POST`, configurable timeout, request/response body byte caps (default 1 MiB each)
- literal or env-backed headers (parsed config retains only the env-var name; the client resolves it for each request)
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

Webhook header configuration retains environment variable names. The client verifies referenced
variables during construction and resolves their current values for every request; it does not
retain the resolved secret in its configuration. HTTP redirects remain disabled.

## Shared reads and integrity boundaries

Gas pricing reads each distinct feed once and applies the strictest configured freshness bound when
assets share a feed. Route simulation copies only demanded adapters and vaults; adapters sharing a
vault consume one vault budget. Advertised discount quotes and fill quotes use the same route/rate/
minimum-discount/capacity checks, and a rejected duplicate does not suppress a later usable offer.

OEV's strategy owns its independent balance and position refresh loops. A bid validates and sizes
against one immutable Morpho snapshot; replacing the cache during a decision cannot replace its
position set. Interest accrual is shared by health calculation and market replay, and replay applies
repayment before calculating and socializing bad debt.

HTTP quote and webhook JSON boundaries accept one document and reject trailing documents. Contract
integer parsing rejects out-of-range unsigned values before calldata packing. Metrics registration
builds a private group before attaching it to the process registry, so a registration conflict leaves
no partially installed workflow collectors. Sentry is owned by the application logger, with an
independent event per write and one bounded flush at shutdown.

## Runtime ownership after the rewrite

`cmd/vault-solver` parses the command and calls `internal/app`. The application owns resource
construction, probe listener binding, the explicit solver construction and shutdown order. Constructors
receive concrete shared services; neither solvers nor strategies mutate a global registry. The
transaction manager outlives solver admission and drains accepted transactions before process exit.

Each integration owns one workflow:

| Integration | Mutable state owner | Decision and execution boundary |
| --- | --- | --- |
| 3F | one loop, adapter-indexed offers and current validated targets | auction projection → budgeted offers → protocol signatures; bounded redemption batches |
| RFQ | one order record contains lifecycle and attempts; poll worker executes | authenticated quote evaluation; immutable prepared fill before transaction admission |
| LI.FI | feed inbox and order worker; immutable queue observations for metrics | recovery generations → fresh plan → calldata → accepted-fill reservations and terminal result |
| UniswapX | order execution records, exclusive obligations, versioned quote snapshots | snapshot loading → guarded publication; prepared fill → admission → capacity reservation |
| RedStone OEV | one active bid decision; bid records plus bounded completed history | one checked position snapshot → bundle selection/pricing → signed bid; strategy owns callback/position reservations |

`internal/liquidlane/planning` owns allocation and fill validation. Physical alternatives are selected
before capacity is assigned; each vault has one budget. The capacity ledger updates aggregate amounts
when an entry is replaced or released, so quote reads need not scan pending orders. LI.FI and UniswapX
read through the same snapshot interface directly. Protocol-specific reads remain inside their solver.

UniswapX snapshot TTL begins before RPC work. Publication rejects an expired snapshot as well as one
invalidated by a concurrent fill. Millisecond configuration rejects overflow; configured adapter lists
reject canonical duplicates. OEV nonce exhaustion returns an error instead of wrapping to zero.

Wire DTOs, ABI layouts, signing domains, rounding constants and metric names remain explicit contracts.
Their existing golden, parity, rounding and metric assertions apply to the new workflows. Generated
clients and bindings remain generated artifacts under `api/`, outside the rewritten runtime scope.

HTTP adapters retain generated request builders and local protocol validation. Shared
`internal/httpclient.Execute` calls the generated request, closes its response on success and failure,
and unwraps upstream diagnostics while preserving the original cause. RFQ uses one response recorder for access
logs, HTTP metrics and panic recovery, including implicit successful writes and already-sent responses.
LI.FI resolves private discount candidates with at most four joined workers and preserves ranked order
when assembling successful results.

Fresh signed discounts bind both their selected identity and the physical quote's adapter, token pair
and exact input amount where supplied. Resolution and quote refresh use this same validation boundary.

LI.FI and UniswapX default strategies store one validated `planning.ExecutionPolicy`; raw YAML
strings do not survive construction. The common policy owns price/inventory buffers, minimum input
and the execution safety window. Gas pricing uses the same owned snapshot for exact cost and the
conservative ceiling; the ceiling calculation saturates in constant time. Range selection maintains
a frontier of alternatives that remain useful by rate, capacity, lifetime and direct/private cost.

Strategy webhook decimal fields use the shared non-negative decimal parser and strict single-document
JSON boundary. Missing-field encodings remain protocol-specific. RFQ open-order lists require an
identity and an `open` status before entering the order store. UniswapX rechecks snapshot identity and precise expiry after the strategy returns, so a slow decision cannot advertise expired data.

The input-token policy owns a sorted membership slice and returns independent sets to strategies.
Typed-data layouts remain protocol contracts: UniswapX derives its cosigner-data layout from the
order tuple, so decoding and signature verification cannot drift through duplicate definitions.
Gas estimation rejects zero or overflowing estimates before applying its 5% execution headroom.

### Reduced construction and state ownership

The application selects the five built-in constructors directly. Each integration selects its own
`default` or `webhook` strategy directly; there is no generic catalog or intermediate selection
package. `parse.NamedConfig` is the common opaque name/config shape, with decoding and validation
still owned by the integration.

One txmanager completion object owns result delivery for accepted work, including worker completion
and hard shutdown. Pending nonce state does not carry a second result channel or once guard.
UniswapX uses one guarded publication record for snapshot, epoch and planning count. LI.FI quote
session availability follows its disconnect channel rather than a second active flag.

`internal/bigmath` owns nil-preserving integer copies, zero defaults, decimal scales and copied minima.
Protocol math and rounding stay in their existing domain packages. Signed discount terms copy their
mutable numbers before explicit conversion to the generated contract tuple.

Range pricing shares immutable route preparation across intervals. LiquidLane owns the exact maximum
non-overquoting rate calculation; protocol adapters retain their range and response contracts.
`bigmath.Decimal` formats integer amounts by decimal-point placement, without constructing a scale or
dividing the integer. LI.FI rates and OEV bids reach it only after their existing non-negative checks.
OEV test configuration copies the supplied values once and fills missing defaults, including preserving
an explicit sizing override as a whole. Morpho parameter reads convert generated results directly at the
reader boundary and still validate the canonical market hash before using a market.

HTTP webhook headers are resolved directly into the outgoing request, with env-backed secrets still
read on every request. UniswapX response bodies own their byte budget and underlying close operation
in a single wrapper. RPC dialing selects transport options before constructing the shared RPC client;
HTTP fallback, redirect restrictions and non-HTTP transport support retain their existing behavior.

A prepared quote pool borrows immutable candidates; each amount query owns its computed amounts.
LI.FI reuses the pool across range endpoints and conservative-floor searches. Fill allocation groups
physical alternatives once using candidate indices, without copying full quotes into each group,
and compares validated quotes for the same exact input; shared vault
budgets and output-rounding bounds remain enforced. Gas prediction returns both selected routes and
saturated units in one pass, consuming only owned copies of the demanded adapter/vault balances.

OEV bundle branches share immutable source and replay state until a leg changes it. Replay owns the
changed market state, collateral budgets are calculated once per branch extension, and final output
legs receive owned amounts. Candidate sizing reads the accrued market state already in the candidate.
3F builds live offer identities and per-auction coverage together in one snapshot per discovery pass.
UniswapX cleans terminal order and resolved obligation history once per poll batch, retaining active
work and the existing expiry boundaries.
