# Solver repository agent-readability refactor

## Target and motivation

This refactor reduced the time and context needed to find the owning package, validate a change, and reason
about the two largest shared state machines. It continued on the existing `refactor/solver-tech-debt` branch
without rewriting its earlier solver simplification commits. `CLAUDE.md` remains canonical and `AGENTS.md`
remains its symlink, as requested.

## Patterns used

- **Contract-surface extraction:** solver catalog, config examples, JSON Schema, README rows, generated-code
  ownership, and dependency direction now have executable checks.
- **Bootstrap contract:** offline config validation uses integration registration metadata without constructing
  RPC, signer, API, or strategy runtime dependencies.
- **Async state-machine decomposition:** LI.FI and txmanager code moved into responsibility-named files while
  preserving one-goroutine/lock ownership and every function body.
- **Pure source partitioning:** LiquidLane reader responsibilities moved within the same package with no API or
  behavior change.

## Phases

1. Added pinned repository-local tooling and one read-only full verification command.
2. Replaced the oversized operator README with a docs map and shared architecture/lifecycle references.
3. Enforced generic/integration dependency direction and registry/catalog consistency.
4. Added integration-owned offline config validation, editor schema, and committed-example validation.
5. Split LiquidLane route/inventory/gas/auth reads and LI.FI inbox/recovery orchestration.
6. Split txmanager admission/lifecycle/replacement/fees/nonce/confirmation code and mirrored its tests.

## Tripwires that paid off

- Full example validation exposed non-parsing LI.FI zero-address and RFQ ellipsis placeholders.
- Final review found that constructing webhook strategies during offline validation resolved env-backed secret
  headers. Strategy registries now expose separate pure validators, pinned by a missing-secret regression test.
- Repeating registry tests exposed package-global test state that survived `go test -count=10`; tests now
  isolate that state without changing assertions.
- AST declaration comparisons proved all 24 LiquidLane, 21 LI.FI, 81 txmanager production, and 105 txmanager
  test function declarations retained identical normalized bodies.
- Ten repeated race runs covered LiquidLane, LI.FI, txmanager, registry, catalog, and architecture seams.
- The integration-tagged Anvil txmanager test still compiles after source partitioning.

## Surprises

The existing branch already contained UniswapX and LI.FI worker extractions, so this work reused those seams
rather than creating competing abstractions. The txmanager test splitter initially emitted an incomplete import
block; no checkpoint compiled in that intermediate state, and the corrected output was rechecked by AST,
race, build, and lint gates before commit.

## Candidate guideline improvements

- Provide Go equivalents for the JavaScript-specific baseline/checkpoint commands in the refactor guide.
- Recommend normalized AST declaration comparison as a tripwire for same-package Go file partitioning.
- Allow mature black-box package/race suites to serve as permanent pre/post extraction contracts when a move
  changes no function body; adding implementation-detail tests solely to satisfy a template would reduce value.
