# Transaction manager lifecycle

`internal/txmanager` owns the transaction-sending EOA. Solvers supply calldata and protocol deadlines; they
do not choose nonces, sign, broadcast, replace, or reconcile transactions directly.

## Core invariant

Exactly one signed nonce lifecycle may be unresolved at a time. A later request is not signed behind a missing
lower nonce. All transaction-sending solvers in one process therefore share the same manager, signer, and write
endpoint. One EOA must never be controlled by two processes.

## Source map

| File | Responsibility |
|---|---|
| `txmanager.go` | public contract, owned state, worker startup, and synchronous/async send entry points |
| `admission.go` | lane demand, waiting, admission failures, and profitability fee query |
| `lifecycle.go` | initial broadcast, pending polling, receipt outcome, and result delivery |
| `replacement.go` | exact rebroadcast, fee-bumped replacement, cancellation, and active-lifecycle shutdown |
| `fees.go` | fee history, gas estimation, signing, and write-RPC submission |
| `nonce.go` | nonce initialization, conflict classification, exact-hash reconciliation, and readiness notification |
| `confirmation.go` | stable-head receipt confirmation and canonical ancestry proof |
| `helpers.go` | immutable request/fee copies and numeric/time helpers |

Tests mirror these responsibilities; shared deterministic fakes live in `test_helpers_test.go`.

## Lifecycle

```text
wait for lane
  -> admit request
  -> obsolete check
  -> estimate gas when needed
  -> select bounded fees
  -> assign nonce and sign
  -> broadcast
  -> poll exact attempts
       ├── canonical receipt -> confirmation tracking -> terminal result
       ├── replacement tick -> exact rebroadcast once after ambiguous send, then fee bump
       ├── request obsolete/deadline/pending timeout -> same-nonce cancellation
       └── nonce conflict -> pause new commitments and reconcile exact hashes
```

A pre-admission failure is marked `NotAdmitted`. Once admitted, the manager owns the lifecycle until it can
return a terminal receipt/result or its finite shutdown deadline expires.

## Concurrency ownership

| State | Owner/synchronization | Rule |
|---|---|---|
| Admission queue and signing/broadcast order | one `Start` worker goroutine | never perform these concurrently |
| Lifecycle occupancy | `lifecycleSlot` plus `admissionDemand` | `Idle` and `LaneReady` include queued demand |
| Local nonce and runtime conflict | `Manager.mu` | a conflict pauses new external commitments |
| Active unmined lifecycle | `unminedMu` | shutdown may request cancellation of the owned nonce |
| Readiness subscribers | `laneStateMu` | notifications carry no state; consumers re-read `LaneReady` |
| Result delivery | per-lifecycle `sync.Once` | every admitted caller receives at most one terminal result |

Do not introduce lock nesting or move network I/O under a mutex. Preserve the worker's ordering when splitting
or extracting code.

## Fees and cancellation

- `maxFeeGwei` is the global EIP-1559 ceiling, including cancellation.
- Normal sends reserve fee-bump headroom so a cancellation can still replace them under the global cap.
- A request-specific cap constrains its call and normal replacements; cancellation may exceed that request cap
  but never the global cap.
- A positive `tipGwei` is a floor. Zero derives the minimum gas-weighted p25 reward from the latest five blocks.
- An ambiguous first broadcast gets one exact-byte rebroadcast before later replacements change fees.
- Same-nonce cancellation uses a 21,000-gas self-transfer.

## Receipts and nonce conflicts

Receipt finality is proven against a stable head and hash-linked ancestry rather than endpoint affinity. A
post-signing `nonce too low` response never causes calldata to be re-signed at another nonce. The manager checks
every tracked exact hash; until ownership is proven, admissions and readiness remain fail-closed.

## Startup and shutdown

Startup requires the write endpoint's latest and pending nonces to match. This detects ordinary unknown pending
work but cannot reveal a private future transaction queued beyond a nonce gap, which is why the EOA must be
exclusive to the process.

On shutdown, solvers stop new external commitments and drain accepted work before the manager stops. The manager
then requests cancellation when nonce ownership is known and drains exact attempts for at most
`shutdownTimeoutMs`. Orchestrator SIGTERM grace must cover solver preparation, pending/cancellation time, and the
manager shutdown timeout.

## Integration checklist

- Use `Send`/`SendAsync` for accepted obligations and `TrySend` only for deliberately non-blocking admission.
- Set `CancelAt` from the protocol deadline rather than a local convenience timeout.
- Keep `Obsolete` read-only, context-aware, and non-authoritative on errors.
- Use stable low-cardinality request labels.
- Hold protocol capacity reservations until the returned lifecycle result is terminal.
- Gate new external commitments on `LaneReady`, not only nonce safety.

Focused verification:

```bash
make verify-race TARGET=./internal/txmanager
make test-txmanager-anvil # requires Anvil
```
