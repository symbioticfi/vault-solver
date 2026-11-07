# Merkle SDK RAM migration implementation plan

> **For agentic workers:** Use subagent-driven-development to implement the scoped tasks below.

**Goal:** Pin the SDK to an immutable upstream commit (recorded in `go.mod` and `docs/MERKLE-SDK.md`) and migrate all host integrations to its shared recipe engine and process-local ownership.

**Architecture:** The existing host transaction manager remains the only signer and nonce owner. A RAM sender bridge retains signed attempts before broadcast and resumes them through that same manager. Generic recipes use SDK planning; RFQ validates the unchanged complete outer fill through shared external-simulation admission; 3F retains typed mint settlement and offer identities in RAM.

**Constraints:** Preserve recipe authority, permissions, amount/witness checks, native contracts, caller context, canonical guards, deadlines and account reservations. Remove SDK journals, recovery and `stateDir` configuration. Do not change other solver integrations or use live credentials.

- [x] Capture the baseline Merkle race suite, then pin the immutable upstream SDK version and checksums.
- [x] Replace `internal/merklesender` journals with Submit/Poll/Busy; cover shared-lane fencing, ambiguous attempts, canonical receipts and deadline-safe resumption through `internal/txmanager`.
- [x] Migrate generic Merkle configuration/factories/tests; delete obsolete persisted-work recovery and revision logic.
- [x] Migrate RFQ discovery/admission and RAM fill intents. Preserve full outer-fill settlement checks through shared external-simulation admission; no numerical witness or inner return data is needed.
- [x] Migrate 3F typed mint settlement, RAM offer identities and guard-only offer authorization. Preserve policy, lease, role and original/current block checks before API mutation.
- [x] Update README, architecture/runbooks, examples and integration commands. Run format/lint, build, race/coverage, tooling, local Anvil suites, and independent review.

Baseline: `go test -race ./internal/merklesender ./internal/solvers/merkle/...` passed before changing the dependency. All integration evidence must remain local Anvil/API-fixture evidence, separate from deployment or live execution.

Validation: Go 1.27.3 `go mod tidy`, `go mod verify`, `go build ./...`, `make test`, `make test-tooling`, `make test-txmanager-anvil`, `make test-rfq-merkle-anvil`, `make test-merklesender-anvil`, and golangci-lint with and without autofix passed. After the final test addition, affected sender, transaction-manager and generic Merkle packages passed race/coverage again. Independent review found and resolved a normal-head-advancement rejection in RFQ discovery; the added regression preserves prepared-block reorg rejection.

No live-provider credentials, external signing, deployment or migration were exercised. Publication targets `stage`.

## Runtime isolation and throughput follow-up

- [x] Adopt the SDK's shared candidate isolation, exact prefix fallback and bounded same-tick drain. Keep strict startup discovery and shared-pin failure checks.
- [x] Notify existing host loops after canonical sender completion through a coalescing channel. Publish the transaction result before notification; retain timer cadence for uncertain outcomes.
- [x] Wake generic Merkle, 3F and RFQ hosts. Check the next RFQ award immediately after a pending fill settles.
- [x] Cover publication order, resumed canonical notification, coalescing and RFQ wakeup without waiting for the three-second timer. Independent host review found no code regressions.
- [x] Pin SDK `26e64712a1ec85379175e0aa0450a23d20c5eb24` and repeat the required repository and local Anvil gates before publishing the PR update. Go 1.27.3 tidy/verify, build, full race/coverage, lint with and without autofix, tooling and all three integration targets passed. Final Anvil runs use 1.9.0 to match CI. The reorg fixture permits only the documented transient nonce-consumed observation while asserting ownership and signer reservation, then requires canonical completion of the exact owned hash.

## RFQ external simulation simplification

- [x] Adopt the explicit SDK external-simulation admission API; preserve shared identity, role and canonical-pin checks and full outer settlement from the real signer.
- [x] Delete the unused nested result probe, artifact, generator, verification target and probe-only tests. Retain generic numerical/result validation.
- [x] Verify ordinary eth_call-only readers, empty outer returns, exact calldata/signer/block/gas, canonical contract rejection fixtures, and the distinction between awarded settlement and generic inner readiness. Update the SDK pin, required gates and both PRs.

Validation on SDK `26e64712a1ec85379175e0aa0450a23d20c5eb24` with `GOWORK=off`: tidy/verify, lint with and without autofix, build, full race/coverage, tooling and all three Anvil integration targets passed. RFQ executes seven canonical-contract cases, including a funded swap rejected by its committed native assertion. Independent review found no admission bypass; generic witness/readiness checks and shared opening/closing guards remain intact. SDK hosted CI passed.

## Funding admission follow-up

- [x] Require fresh balance coverage for final unsigned maximum cost before signatures, including value, fee floors and replacement gas.
- [x] Prove insufficient/unavailable funding causes no signature/capture/broadcast and an initial Merkle reservation releases; prove funded retry settles and replacement failures retain prior signed work.
- [x] Pin the published SDK and complete required repository/Anvil gates, independent review and PR #277 update. Signed RPC rejection ownership remains unchanged; no cancellation workflow or new recovery state.

Funding validation with published SDK `26e64712a1ec85379175e0aa0450a23d20c5eb24` and `GOWORK=off`: tidy/verify, format/lint autofix and plain lint (zero issues), build, full race/coverage, tooling and all three Anvil 1.9.0 targets passed. Initial funding failures produce no signature/capture/broadcast; exact coverage, invalid data, fee floors, value and replacement retention are covered. The real shared-manager Merkle release/funded-retry regression passed ten race-enabled Anvil runs. Independent review found no production blockers; its test scheduling observation was fixed with a bounded lane-release wait.

## Same-account starvation fix

- [x] Pin public SDK `51d79326d077d6e4411afd03aafbb29aa5b6b5fd` (`v0.0.0-20261107231501-51d79326d077`). Before a provisional settlement task excludes another executor sharing its account, the SDK admits the collected prefix and rebuilds composition and reservations from surviving members. Idle or rejected tasks release provisional reservations; prepared evidence and task identity are reused, and final selected work still receives full admission.
- [x] Repeat Go 1.27.3 tidy/verify, format, build, full race/coverage, lint, tooling and all three Anvil 1.9.0 targets with `GOWORK=off`. Host runtime code and configuration stay unchanged.

Validation on the published SDK above passed: `go mod tidy`, `go mod verify`, `make format`, `go build ./...`, `make test`, `make lint`, `make test-tooling`, `make test-txmanager-anvil`, `make test-rfq-merkle-anvil` and `make test-merklesender-anvil`. Both lint runs reported zero issues. Tests use local Anvil 1.9.0 and API fixtures; no live signing, provider credentials or deployment was used.
