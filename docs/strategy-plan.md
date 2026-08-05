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
reader composes direct and physical state plus gas facts for LI.FI and UniswapX. RFQ-like exact-input
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

## Shared LiquidLane strategy: `internal/liquidlane/strategies/greedy`

The current shared LiquidLane algorithm is explicitly named `greedy`. Adding another algorithm means a
sibling package under `internal/liquidlane/strategies`; it does not require a second runtime registry or
solver config knob until a real deployment needs selectable behavior. Sharing this pure decision engine
does not create a cross-solver `Strategy` facade. `QuoteTask` accepts
normalized, already-priced candidates, an exact input or output, route limit, buffer, an optional gas
pricing model, and an explicit input-coverage rule. `SolveQuote` owns deterministic ranking, direct/private
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

The adjacent `internal/liquidlane/discounts` package owns the discount rules shared by RFQ, LI.FI, and
UniswapX: parse and filter live offers, bind offers to physical routes, cap advertised rate/capacity,
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
