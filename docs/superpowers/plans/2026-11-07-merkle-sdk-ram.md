# Merkle SDK RAM migration implementation plan

> **For agentic workers:** Use subagent-driven-development to implement the scoped tasks below.

**Goal:** Pin the SDK to an immutable upstream commit (recorded in `go.mod` and `docs/MERKLE-SDK.md`) and migrate all host integrations to its shared recipe engine and process-local ownership.

**Architecture:** The existing host transaction manager remains the only signer and nonce owner. A RAM sender bridge retains signed attempts before broadcast and resumes them through that same manager. Generic recipes use SDK planning; RFQ validates actual nested execution results plus the unchanged complete outer fill; 3F retains typed mint settlement and offer identities in RAM.

**Constraints:** Preserve recipe authority, permissions, amount/witness checks, native contracts, caller context, canonical guards, deadlines and account reservations. Remove SDK journals, recovery and `stateDir` configuration. Do not change other solver integrations or use live credentials.

- [x] Capture the baseline Merkle race suite, then pin the immutable upstream SDK version and checksums.
- [x] Replace `internal/merklesender` journals with Submit/Poll/Busy; cover shared-lane fencing, ambiguous attempts, canonical receipts and deadline-safe resumption through `internal/txmanager`.
- [x] Migrate generic Merkle configuration/factories/tests; delete obsolete persisted-work recovery and revision logic.
- [x] Migrate RFQ discovery/admission and RAM fill intents. Preserve full outer-fill settlement checks and inspect real inner results through an eth_call-only lens.
- [x] Migrate 3F typed mint settlement, RAM offer identities and guard-only offer authorization. Preserve policy, lease, role and original/current block checks before API mutation.
- [x] Update README, architecture/runbooks, examples and integration commands. Run format/lint, build, race/coverage, tooling, local Anvil suites, and independent review.

Baseline: `go test -race ./internal/merklesender ./internal/solvers/merkle/...` passed before changing the dependency. All integration evidence must remain local Anvil/API-fixture evidence, separate from deployment or live execution.

Validation: Go 1.27.3 `go mod tidy`, `go mod verify`, `go build ./...`, `make test`, `make test-tooling`, `make test-txmanager-anvil`, `make test-rfq-merkle-anvil`, `make test-merklesender-anvil`, and golangci-lint with and without autofix passed. The RFQ integration target also verifies reproducible probe bytecode. After the final test addition, affected sender, transaction-manager and generic Merkle packages passed race/coverage again. Independent review found and resolved a normal-head-advancement rejection in RFQ discovery; the added regression preserves prepared-block reorg rejection.

No live-provider credentials, external signing, deployment or migration were exercised. Publication targets `stage`.

## Runtime isolation and throughput follow-up

- [x] Adopt the SDK's shared candidate isolation, exact prefix fallback and bounded same-tick drain. Keep strict startup discovery and shared-pin failure checks.
- [x] Notify existing host loops after canonical sender completion through a coalescing channel. Publish the transaction result before notification; retain timer cadence for uncertain outcomes.
- [x] Wake generic Merkle, 3F and RFQ hosts. Check the next RFQ award immediately after a pending fill settles.
- [x] Cover publication order, resumed canonical notification, coalescing and RFQ wakeup without waiting for the three-second timer. Independent host review found no code regressions.
- [x] Pin SDK `1a6567ec6867a649952024d2b45c9737f98cc438` and repeat the required repository and local Anvil gates before publishing the PR update. Go 1.27.3 tidy/verify, build, full race/coverage, lint with and without autofix, tooling and all three integration targets passed. Final Anvil runs use 1.9.0 to match CI. The reorg fixture permits only the documented transient nonce-consumed observation while asserting ownership and signer reservation, then requires canonical completion of the exact owned hash.
