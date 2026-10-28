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
| F03 | Fixed | Bounded SDK read batches retain IDs and whole-response evidence. All-null receipt/header batches can use read fallbacks; partial evidence and RPC errors stay with the selected endpoint. Pending batches disable eRPC empty-result retries. Real SDK tests cover reversed/missing/duplicate IDs, mixed errors, large envelopes and write exclusion. |
| F04 | Fixed; compatibility regression corrected | Funding Manage gates must become zero when a recognized market principal is zero, either directly or through proven arithmetic data flow. This accepts the current frontend's Result gates for `min(saturatingSub(principal, cash), capacity)`; no migration is needed. The original blanket Result rejection was a compatibility regression masked by rewriting fixture gates in the test loader. That rewrite was removed; both original recipe fixtures now pass unmodified, with table-driven acceptance/rejection cases for arithmetic and cross-market dependencies. Literal and unrelated gates remain rejected. Canonical guarded mint internals and direct principal gates remain unchanged. This validates activation, not the economic ownership of arbitrary funding leaves. |
| F05 | Fixed | UniswapX strategy context is bounded by the quote budget, caller and remaining snapshot lifetime. Final publication checks cancellation and wall/chain expiry, including equality. Handler regressions require HTTP 204 and decline accounting for expected timeout. |
| F06 | Fixed | The HTTP boundary preserves original JSON `rateBasisSeconds`; exact integer math rejects negative, fractional and overflowing periods before economics. JSON regressions include float32 rounding, negative underflow, large int64 and annualization boundaries. Omitted/null/zero retains the legacy comparison. |
| F07 | Fixed | RFQ-Merkle skips malformed matching discount entries and tries later healthy routes. Bad-first/good-second ID and capacity regressions require authoritative outer simulation. |
| F08 | Fixed | RFQ-Merkle samples wall time before its header RPC. A delayed-header regression verifies that RPC latency consumes the sending deadline with a leading chain clock. |
| F09 | Fixed | Connector-specific read, decode and domain failures exclude only that connector. Healthy connector and native routes remain. Shared header/token metadata, cancellation and batch transport/envelope failures abort the observation. |
| F10 | Fixed | Each connector is observed once per pinned quote, shared across discount alternatives. Seven isolated getter calls per connector use bounded SDK read batches, preserving the zero caller and separate rollback-only capacity previews. A review-discovered WebSocket/IPC compatibility regression was fixed in the shared reader, with a real WebSocket regression. Tests cover 256 alternatives and 256 distinct connectors; no latency benchmark is claimed. |
| F11 | Fixed; full auction scan retained | Live commitments are maintained before bidding. Each pass checks every relevant unanswered open/solvable auction, filtering historical/foreign candidates and memoizing positive and negative eligibility results per pin. The added eligibility count cap, configuration and rotating cursor were removed to preserve full-scan behavior. Tests cover all 257 candidates in one pass, complete eligibility before POST, maker-specific duplicate filtering, maintenance ordering, memoization and final snapshot validation. |
| F12 | Fixed | Morpho positions query includes configured chain identity; the adapter validates each returned chain and requested market. Raw fetched rows bound pagination even when all identities are rejected. GraphQL output was regenerated through the pinned Make target. |
| F13 | Fixed | LI.FI has validated read-inactivity and pong-write budgets. Application/control heartbeat traffic renews the read deadline; silent recovered sockets cancel connection work and reconnect with recovery-gated readiness. Tests cover silence, recovery and heartbeat variants. |
| F14 | Fixed | Schema/ABI refresh recipes fail fast, validate staged output and replace files only after successful extraction. ABI groups finish all extraction before any replacement. Offline tests preserve originals for fetch, pipeline, malformed input and generator failures. Individual replacements are atomic; multi-file rename is not a filesystem transaction. |
| F15 | Fixed | Default 3F schema refresh selects the development contract already selected by the drift registry, retaining the intentional `rateBasisSeconds` surface. |
| F16 | Fixed | Offline `--check-coverage` exits nonzero for unregistered or missing schemas, and PR CI runs it with synthetic registration/refresh tests. Scheduled live drift monitoring remains separate. |
| F17 | Fixed | Docker Dependabot selects `/deploy`, containing the image manifest. |
| F18 | Fixed | Strategy documentation describes one executor with 1..32 guarded funding pairs, replacing the obsolete 1..256-executor model. |
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
bypasses of their authoritative simulation. A fresh independent review found the WebSocket/IPC regression noted in F10; its
regression test first reproduced the failure and passed after the shared reader fix.
The reviewer found no remaining P1/P2 issues. Full gate results are recorded in the pull request. No live credentials, API mutations,
external-chain transactions, merges or deployments form part of this verification.

The F04 compatibility correction reran formatting, the full build, all race/coverage
tests and lint. Before the correction, both unmodified funding fixtures and nine
valid arithmetic Result cases failed the focused regression; all pass afterward.

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
