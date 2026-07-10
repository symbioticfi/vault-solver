# vault-solver — Solver-Local Strategy Architecture

The **strategy** is the decision-making core of a solver. This document records the *generic* strategy
boundary shared by every solver. Concrete per-solver contracts (the actual input/output types) live in
that solver's own plan under `docs/` and in its `strategies/types` package — never here.

## Core principle

A solver splits into two parts:

- **The solver skeleton** — everything that faces the outside world: discovering work, reading
  chain/API state, assembling a snapshot of **raw facts**, then signing and submitting whatever the
  strategy decides. It holds the key and moves funds, but it makes no economic decision.
- **The strategy** — the brain. Given the solver's input snapshot, it decides what to do and returns a
  concrete, ready-to-execute plan.

The flow is one-directional and trusted:

```
solver builds input (raw facts)  →  strategy decides  →  solver executes the output verbatim
```

**The strategy is a trusted module, and it is the core of the solver.** The solver does not re-verify,
re-price, clamp, re-rank, or otherwise modify the strategy's output — it executes it as given. Any
validation, sizing bounds, eligibility filtering, replay, caching, or recovery that a decision needs
lives *inside* the strategy implementation, not in the solver skeleton. This keeps the boundary crisp:
swapping in a different strategy (including an external one) never requires the solver to grow — or
second-guess — decision logic.

Concretely:

- The solver provides **raw facts, not decisions** — available liquidity and caps, discovered work
  items, current state, things it already holds. It never pre-computes a sizing, a ranking, or a
  candidate selection for the strategy.
- The strategy returns a **complete plan the solver runs as-is**. The solver's only remaining
  responsibility is execution integrity — values that are properties of the transaction rather than
  the decision (nonce, signature, EIP-712 domain). Those the solver sets itself, and the strategy can
  never supply them.

## The contract

Each solver defines its own decision interface — one method per decision point — in its own
`strategies/types` package. **There is no single cross-solver `Strategy` type: each solver's interface
is unique to its workflow** (a quote/fill solver's differs from an auction solver's, which differs from
a bidding solver's). What every solver shares is the *pattern*, not the signature — a solver-built
input of raw facts in, a strategy-decided output out.

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

type StrategyFactory func(raw yaml.Node, deps StrategyDeps) (types.Strategy, error)
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
  in-process handler is transport-only and adds no decision logic and no second-guessing of its own.
  This is the seam for running custom decision logic out-of-process without forking the solver.

Both plug into the same trusted boundary: the solver executes their output the same way, so a solver
is never coupled to which strategy is loaded.

## Adding your own strategy

Strategies are pluggable per solver, and there are two ways to add one.

**Out-of-tree, no Go changes — run a `webhook`.** Point the solver's `strategy` at your own HTTP
service (see below). It receives the solver's raw-facts input as JSON and returns the plan to execute;
the solver runs it verbatim. This is the fastest path and keeps your decision logic in your own
codebase and language.

**In-tree — register a new strategy on the solver.** To ship a strategy alongside a solver, implement
that solver's interface (each is unique — you implement the one the target solver defines):

1. Create a package under the solver's `strategies/<name>/` and implement the solver's strategy
   interface (e.g. `DecideOffers` for 3F), consuming its `strategies/types` input and returning its
   output type.
2. Add a `NewFromConfig(raw yaml.Node, deps) (types.Strategy, error)` constructor that parses your own
   `strategy.config` node — the framework hands it to you opaque, so you own its schema and validation.
3. Self-register from your package `init()` via the solver's local
   `strategies.Register("<name>", NewFromConfig)`, under a name unique within that solver.
4. Ensure the package is imported so its `init()` runs (blank-import it where the solver wires its
   strategies).
5. Select it in config: `strategy: { name: <name>, config: { … } }`.

Either way the solver skeleton is untouched: it provides the same input and executes whatever plan your
strategy returns — so the correctness of the decision is entirely yours to own.

## Shared transport: `internal/webhook`

The only shared strategy-adjacent package is `internal/webhook`, a generic HTTP JSON client:

- HTTP JSON `POST`, configurable timeout, request/response body byte caps (default 1 MiB each)
- literal or env-backed headers (parsed config retains only the env-var name; `NewClient` resolves it)
- strict response decode; non-2xx and empty-body responses are errors

It has no solver names, no strategy registry, and no per-solver DTOs — each solver's webhook strategy
owns its own wire types (conventionally lower-camel JSON with decimal strings for big integers,
provided by that solver's `strategies/types`).

Money-facing fractional facts stay exact across this boundary too: a solver normalizes them to its
own integer unit before invoking a strategy, and its webhook wire type renders the value as a decimal
string. A webhook must not reintroduce binary floating-point into pricing or eligibility decisions.

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
