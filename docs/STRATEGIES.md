# Strategy and planning boundaries

This is the shared contract for turning validated facts into an executable plan. The code shares the
pattern, not one universal cross-protocol interface.

## Ownership

Every workflow has one coordinator and one local planner boundary:

```text
protocol input -> normalize facts -> planner -> validate plan -> acquire capacity -> execute -> reconcile
```

- The coordinator owns I/O, ordering, retries, recovery, cancellation, metrics, and terminal outcomes.
- The planner owns pricing, sizing, ranking, route selection, and explicit decline decisions.
- Execution accepts a complete plan. It may reject stale or invalid facts, but does not silently re-plan.
- Generated API and contract types stop at the protocol boundary.

The planner is pure unless its protocol genuinely needs a long-lived read model. 3F, RFQ, LI.FI, and
UniswapX keep their planner contract and `default`/`webhook` implementations directly in the owning solver
package. RedStone OEV keeps its substantial Morpho read model and economics in
`internal/solvers/redstoneoev/policy`; its protocol-neutral decision values live in the adjacent `decision`
package so the coordinator and policy do not depend cyclically.

There is deliberately no `strategies/` framework, generic registry, package initialization, or common
`Strategy` interface.

## Configuration

The existing operator-facing YAML remains solver-local:

```yaml
strategy:
  name: default
  config: {}
```

`strategy` is a configuration term, not a code architecture. Each solver strictly decodes the opaque node,
selects a local `Planner`, and uses the same selection for offline validation and runtime construction.
`webhook` uses the shared bounded JSON client in `internal/webhook`, but owns its protocol request and response
types beside the local planner.

## LiquidLane planning

`internal/liquidlane` owns the canonical `Route`, `Inventory`, `QuoteCandidate`, `FillQuote`, `Plan`, and
`PlanLeg`. `internal/liquidlane/planning` owns the one planning algorithm used by RFQ, LI.FI, and UniswapX:

1. filter fresh inventory and evaluate direct/private alternatives;
2. group alternatives by physical route and `CapacityID`;
3. aggregate deterministic candidates under the caller's route limit;
4. price exact-input or exact-output quotes, including optional complete-plan gas;
5. finalize and validate one canonical `liquidlane.Plan`;
6. project claims into the process-wide `capacity.Book` immediately before execution.

Direct and discounted candidates over one route are alternatives, never additive capacity. A plan uses at most
one alternative per physical route. `maxRoutes = 1` is the same algorithm with a stricter bound, not a second
implementation.

The shared planner contains no protocol clients, signer, transaction manager, caches, or goroutines. Protocol
packages decide when facts are fresh enough, translate their request into a planning task, and translate the
result into protocol calldata or wire output.

## Adding policy

- Prefer changing the owning solver's concrete planner when the behavior is protocol-specific.
- Prefer changing `liquidlane/planning` only when RFQ, LI.FI, and UniswapX share the same physical-liquidity
  invariant.
- Add a new package only when it owns a substantial independent model or has a real second production consumer.
- Add a new in-process planner implementation beside its solver contract; do not recreate
  `strategies/{types,default,...}`.
- Use `webhook` for out-of-process policy. Treat its output as untrusted and validate it against the exact facts
  supplied to it.

Tests should pin pure economics with small tables and pin trust boundaries with one contract test per failure
class. Lifecycle and recovery tests belong to the coordinator; JSON tests belong to the webhook boundary;
allocation tests belong to LiquidLane planning.
