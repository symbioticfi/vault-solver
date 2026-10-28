# Audit fixes, 28 October 2026

This ledger preserves finding IDs from the local deep audit of
`6314af5ca3a59d42d8d91843b4c789ade9facf50`. The fixes are stacked on
`codex/recipe-funding-capacity-20261028` (PR #238), preserving its Merkle funding
capacity work. They do not promote `stage` or change production configuration.

## Finding dispositions

| ID | Disposition | Change and regression evidence |
|---|---|---|
| F01 | Fixed | HTTP fallback rebuilds Basic Authorization from each endpoint's own URL. Real HTTP client tests cover authenticated/unauthenticated primary and fallback combinations and immutable caller headers. |
| F02 | Fixed | A shared zap core redacts URL userinfo, paths, queries and fragments before stdout and Sentry. Tests preserve diagnostics, bound fields, concurrent reuse, sampling, sink filtering and write error handling. This is URL redaction, not arbitrary secret detection. |
| F03 | Fixed | SDK receipt/header batches retain whole-response evidence and use read fallbacks for ordinary all-null results. Pending batches disable eRPC empty-result retries. WebSocket/IPC batches use the existing geth connection. Batch body-read failures follow the ordinary transport fallback path, including truncated responses. |
| F04 | Withdrawn | The added funding-gate data-flow proof is removed. Additional funding Manage steps follow the trusted curator-authored recipe. The later simplification also removes canonical mint guard comparison; SDK/contract permission validation remains. |
| F05 | Fixed | UniswapX strategy context is bounded by the quote budget, caller and remaining snapshot lifetime. Final publication checks cancellation and wall/chain expiry, including equality. Handler regressions require HTTP 204 and decline accounting for expected timeout. |
| F06 | Fixed; simplified | The HTTP boundary preserves exact JSON rateBasisSeconds before generated float32 conversion. Added decimal length/exponent caps and the required-exact-source rejection are removed; internal DTO callers retain their original fallback and numeric constraints. |
| F07 | Withdrawn | The original RFQ-Merkle discount handling is restored, without skipping malformed backend entries to seek another route. |
| F08 | Fixed | RFQ-Merkle samples wall time before its header RPC. A delayed-header regression verifies that RPC latency consumes the sending deadline with a leading chain clock. |
| F09 | Withdrawn | The original connector quote path is restored, without per-connector error isolation or fallback based on invalid contract responses. |
| F10 | Withdrawn | Connector deduplication and batched getter changes are reverted with the connector quote path. The independently useful WebSocket/IPC SDK transport fix remains under F03. |
| F11 | Fixed; full auction scan retained | Live commitments are maintained before bidding. Each pass checks every relevant unanswered open/solvable auction, filtering historical/foreign candidates and memoizing positive and negative eligibility results per pin. The added eligibility count cap, configuration and rotating cursor were removed to preserve full-scan behavior. Tests cover all 257 candidates in one pass, complete eligibility before POST, maker-specific duplicate filtering, maintenance ordering, memoization and final snapshot validation. |
| F12 | Fixed; query filter only | Morpho borrower queries include the configured runtime chain. Extra returned chain/market/address cross-checks and their metadata selections are removed; the API applies the requested filters. Generated bindings were regenerated through the pinned Make target. |
| F13 | Fixed | LI.FI has validated read-inactivity and pong-write budgets. Application/control heartbeat traffic renews the read deadline; silent recovered sockets cancel connection work and reconnect with recovery-gated readiness. Tests cover silence, recovery and heartbeat variants. |
| F14 | Fixed | Schema/ABI refresh recipes fail fast, validate staged output and replace files only after successful extraction. ABI groups finish all extraction before any replacement. Offline tests preserve originals for fetch, pipeline, malformed input and generator failures. Individual replacements are atomic; multi-file rename is not a filesystem transaction. |
| F15 | Fixed | Default 3F schema refresh selects the development contract already selected by the drift registry, retaining the intentional `rateBasisSeconds` surface. |
| F16 | Fixed | Offline `--check-coverage` exits nonzero for unregistered or missing schemas, and PR CI runs it with synthetic registration/refresh tests. Scheduled live drift monitoring remains separate. |
| F17 | Fixed | Docker Dependabot selects `/deploy`, containing the image manifest. |
| F18 | Fixed | Strategy documentation describes one executor with recipe-defined funding pairs, replacing the obsolete 1..256-executor model. |
| F19 | Test gap closed | Composed real SDK state store, submission manager, host Transport and shared txmanager execute against local Anvil. Tests assert signed persistence before original/replacement broadcast, restart fencing, canonical receipt, acknowledgement ordering, persistence failure and failed acknowledgement. No runtime corruption was asserted by the audit. |
| F20 | Contract-boundary fixture added | Pinned canonical Reactor, Executor, RecipeExecutor, MerkleAccount and LiquidLaneConnector execute the complete outer RFQ fill. Real signatures and mined balance assertions cover success; invalid terms, corrupted discount/order signatures, insufficient latest-state funding and authorization must fail. Provenance, external fixture substitutions and regeneration are recorded in the fixture directory. This does not establish live deployment or issuer settlement. |
| F21 | Investigation resolved; authority unchanged | At zero pair budget, the canonical positive-principal, budget-consuming membership check cannot attest an unjournaled UI offer. Such offers require explicit UI cancellation. Own journaled unaccepted intent remains cancellable; accepted mint windows remain immutable; automatic bids stay blocked. Tests and the runbook state this limit. Broader cancellation needs an explicit protocol/interface decision. |

## Verification

All checks below passed locally on the final source tree using
`GOWORK=off GOTOOLCHAIN=go1.27.3` (golangci-lint 2.15.0):

```sh
golangci-lint run --fix
go build ./...
go test -race -cover ./...
golangci-lint run
make test-tooling
make test-txmanager-anvil
make test-merklesender-anvil
make test-rfq-merkle-anvil
```

Integration-tag lint for the chain, shared sender and RFQ-Merkle packages also
passed with zero issues. `go mod tidy` left both module files unchanged. The RFQ
fixture regenerated byte-for-byte through `make refresh-rfq-merkle-fixture` from
the pinned clean contract checkouts.

Anvil suites run isolated local chains without forks or upstream APIs. RED/GREEN
regressions were run for runtime fixes; fixture tests also reject deliberate
bypasses of their authoritative simulation. A fresh independent review found the WebSocket/IPC regression noted in F03; its
regression test first reproduced the failure and passed after the shared reader fix.
Review and full gate results are recorded in the pull request. No live credentials, API mutations,
external-chain transactions, merges or deployments form part of this verification.

The input-hardening rollback restores the original RFQ paths and removes the
extra 3F funding proof and Morpho response cross-checks. Correctness fixes for
valid data, deadlines, transport, connection liveness and numeric precision remain.
Existing baseline authorization, amount and recovery behavior is unchanged.

## Historical plans 001–014

The original untracked plans retain their identity and were not overwritten.
Their older Go/linter commands do not apply to this branch. Remaining older items
are recorded here rather than silently marked fixed by F01–F21.

| Earlier ID | Reconciliation |
|---|---|
| 001 | Fulfilled: `make lint` exists. |
| 002 / 003 | Superseded by their earlier replacement plans. |
| 004 | Fulfilled: neutral parse/chain helpers and typed bindings exist. |
| 005 | Shared workflow metrics fulfill the metrics intent; bespoke collector proposal is stale. |
| 006 | Fulfilled: Prometheus pin is 1.25.0. |
| 007 | Partly open: editor/env examples and negative RFQ count/interval default semantics remain separate work. Build-all CI now explicitly runs `make build`. |
| 008 | Mostly superseded/fulfilled; refresh residual integration wording before executing. F18 fixes the audited 3F paragraph. |
| 009 | Partly open: OEV WebSocket read-limit scope remains separate. Old gas goroutine/callback targets were removed. F13 concerns LI.FI inactivity. |
| 010 | Open: signer package tests remain separate from this audit's composed journal test. |
| 011 | Partly fulfilled by existing 3F pipeline/redeem/cache tests. Reassess remaining exact calldata/batch assertions against current workflows. |
| 012 | Fulfilled: geth/abigen 1.17.8 and websocket 1.5.3 pins align; ABI differences were formatting. |
| 013 | Stale scope: reconnect tests exist and old auth/callback targets were removed; reassess remaining lifecycle coverage. |
| 014 | Direction proposal: a generic cross-solver dry-run switch was not selected. |

Optional audit directions (new health metrics, LI.FI pending-capacity utilization
and live Uniswap qualification) were not part of these finding fixes.

## Curator-owned recipe simplification

The solver extracts 3F market/Input identities without reconstructing the canonical
guard or imposing mint/funding gates, group limits, mask policy or account-policy
cross-checks. The guard reference is test data only. RFQ connector preparation fills
signed swaps and award Inputs without proving balance-delta guards or requiring
one particular gate/swap count. The SDK and contracts retain permission validation,
complete simulation and journal recovery. The shared SDK also accepts explicitly
fixed amounts and curator-supplied masked preimages without a provider ownership rule.
