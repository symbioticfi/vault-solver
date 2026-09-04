# Architecture

`vault-solver` is one application containing five protocol workflows. A workflow owns protocol semantics and
recovery; the application owns shared process resources and lifecycle.

## Dependency direction

```text
cmd/vault-solver -> internal/app -> internal/solvers/<protocol>
                         |                    |
                         v                    +-> internal/liquidlane/planning
               shared chain, signer,          +-> generated api bindings
               txmanager, capacity,
               observability
```

- `cmd/vault-solver` is CLI and the explicit composition root only.
- `internal/app` starts integrations, publishes readiness, cancels intake, drains accepted work, and stops the
  shared transaction lane.
- Solver packages never import one another and generic packages never import solvers.
- Generated packages are boundary leaves; handwritten adapters project them into local facts.

## Runtime flow

1. Load and strictly validate generic YAML plus each opaque solver config.
2. Construct chain, signer, observability, one transaction manager, and one capacity book.
3. Construct configured protocol coordinators through the immutable command descriptor list.
4. Start the transaction lane before integrations can accept executable work.
5. Run coordinators concurrently; the first fatal error cancels the application.
6. Stop new commitments, let integration-specific shutdown preparation finish, drain accepted transactions,
   and stop shared services exactly once.

Each solver has one coordinator. It owns source cadence, mutable state, retry/recovery ordering, and terminal
reconciliation. Its local planner receives validated facts and returns a complete plan; the coordinator does
not re-price or rerank it during execution.

Concrete state owners make this rule enforceable: OEV runs its market `FactSource` outside the planner; LI.FI's
order book stores one exclusive phase and receives explicit plan/decline/capacity-blocked decisions; RFQ separates
validation and planning from the funds-moving tail of one linear submission flow; and UniswapX keeps per-order
transitions in one ledger and aggregate blocking state in one breaker.

## Shared liquidity and execution

`internal/liquidlane` owns canonical adapter, route, inventory, candidate, snapshot, and execution-plan values.
`internal/liquidlane/planning` owns deterministic quote/fill evaluation, aggregation, gas economics, and final
plan validation. RFQ, LI.FI, and UniswapX use the same physical `CapacityID` accounting and receive the same
process-wide `capacity.Book` instance.

3F offer exposure and RedStone OEV executor/position exposure remain protocol-local because their identity and
terminal evidence differ from accepted LiquidLane fills. OEV keeps its substantial Morpho model in
`internal/solvers/redstoneoev/policy`; the other solvers keep planners directly in their root package.

`internal/txmanager` is the only owner of transaction admission, nonce allocation, broadcast, replacement,
confirmation, and shutdown drain. A solver builds calldata and submits a request; it never sends a transaction
or allocates a nonce directly.

## Configuration ownership

The generic layer owns `chain`, `signer`, `txManager`, `observability`, and each `solvers[].name`. The selected
solver strictly decodes its own opaque `solvers[].config` node. Deployment values stay in YAML or an
authoritative upstream response. Secret-bearing fields name environment variables and secrets are read only at
the point of use.

## Adding an integration

1. Add one self-contained package under `internal/solvers/<name>` implementing `app.Integration`, `Factory`, and
   pure `ValidateConfig`.
2. Add one immutable descriptor to `cmd/vault-solver/composition.go`.
3. Generate external clients or bindings from vendored contract artifacts.
4. Add the config/schema/example and update the owning integration document.
5. Add small planner tests plus boundary/lifecycle tests appropriate to its risk, then run the repository and
   E2E gates in [Development](DEVELOPMENT.md).

See [Strategy and planning boundaries](STRATEGIES.md), [LiquidLane conventions](LIQUIDLANE-CONVENTIONS.md), and
[Transaction manager](TXMANAGER.md) for the detailed shared contracts.
