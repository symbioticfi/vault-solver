# Shared transaction manager

Both vault-solver and merkle-executor use the independently versioned SDK
`transaction.Manager`. Vault-solver has no local transaction engine or sender
bridge. Runtime state is RAM only, with no restart recovery.

## 1. Ownership and admission

One solver process has one configured signer and one manager. Separate processes
use separate signers by trusted configuration. The existing `solvers:` list
selects integration workers; those workers share the process manager.

Solvers and providers own discovery, existing-order selection, priority,
liquidity reservations and protocol reconciliation. The manager owns one
unresolved nonce and its signed attempts. Its unbuffered handoff serializes
chosen calls without introducing a domain queue. `Available` reports whether
native admission is available; `LaneReady` also excludes queued or active work.
Workers check these views through their normal polling lifecycle.

The host supplies configuration, RPC, signer and logger. Construction starts no
work. The host calls `Initialize`, then runs `Start` and keeps RPC alive until its
bounded shutdown drain ends. Simulate-only and externally settled integrations
do not initialize or start a transaction worker.

## 2. Request and result

Native `Send` and `SendAsync` requests contain destination, calldata, optional
business deadline, a stricter fee ceiling and an optional obsolescence callback.
Labels identify the integration in logs. The SDK does not branch on protocols.

A result contains signed hashes, an error and an optional canonical receipt.
`Included`, `Reverted` and `Uncertain` derive facts from this evidence.
`NotAdmitted` distinguishes rejection before the worker accepts a request. An
error with signed hashes never proves that the call cannot execute.

Merkle keepers use `manager.Sender(name)` with `Submit`, `Poll` and `Busy`.
That ticket retains the immutable plan through uncertainty. Only its own
canonical receipt authorizes settlement. A canonical older call at the same
nonce discards the newer plan without settlement; the keeper then replans.

## 3. Configuration and time budgets

| YAML field | Default | Meaning |
| --- | --- | --- |
| `confirmations` | 2 | Blocks after inclusion required by the host. |
| `maxFeeGwei` | Required for sending | Global maximum fee per gas. |
| `maxGas` | 16,777,216 | Gas budget, also bounded by the fresh block gas limit. |
| `pollIntervalMs` | 2000 | Receipt and readiness polling cadence. |
| `replacementIntervalMs` | 30000 | Minimum interval between fee replacements. |
| `shutdownTimeoutMs` | 60000 | Maximum signed-work drain after cancellation. |

The operation context bounds first signing. Business deadlines independently
bound later signatures and replacements. RPC work has bounded contexts and
honors earlier applicable deadlines. Configuration contains environment variable
names for secrets, never secret values.

## 4. Fees, replacements and expiry

Recent market tips and twice the current base fee plus tip determine pricing,
within the global and request ceilings. `MaxFeePerGas` uses the same policy for
quote economics. Both fee fields rise by at least 12.5% above accepted fees for a
replacement. Rejected signed candidates retain identity without raising that
accepted floor. Exact rebroadcasts retain signed bytes.

Before every signature the manager rechecks nonce, gas, business validity and
fresh sending-endpoint funding for the final maximum gas cost. The signer must
preserve the exact payload and configured identity. Signed identity is retained
before any broadcast, including timeout and rejection responses.

Native expiry or obsolescence returns an uncertain result if already signed.
The nonce record retains all hashes; a freshly validated request may replace
that nonce. A keeper ticket instead remains busy and observes without further
signing after expiry. There is no cancellation transaction or arbitrary pending
timeout that erases signed ownership. At most 256 signed variants are retained
at one unresolved nonce; reaching that bound stops new signing.

## 5. Receipt polling and confirmation

All retained hashes are checked, including older replaced requests. A terminal
receipt must match the owned transaction, canonical inclusion block, ancestry
and requested depth. Reverts require the same proof as successes. Reorganized,
missing or malformed receipts do not authorize settlement. Foreign nonce
advancement without an owned canonical receipt remains unknown. An older winning
request is never credited to a newer call.

## 6. RPC boundaries

Broadcasts and mined nonce reads use the configured sending endpoint. Funding
reads try that endpoint first and may use the shared reader when a submission
relay rejects balance reads. Gas, fee and receipt reads use the shared chain
reader and its read fallbacks.
HTTP endpoint attempts are bounded by `chain.rpcAttemptTimeoutMs` and any shorter
operation deadline. Transport owns endpoint selection; SDK transaction code owns
signing and canonical receipt validation.

## 7. Shutdown

Worker cancellation and manager drain begin together. Shutdown stops admission
and further business signing while observing already signed work within
`shutdownTimeoutMs`. Waiting workers receive the actual result or an uncertain
shutdown result. RPC remains available until the drain finishes. A timeout cannot
make a signed transaction disappear.

## 8. Observability

Hosts own tracing and integration metrics. They record fill and cost metrics
from returned canonical receipts. The SDK has no metrics registry, receipt
observer callbacks, periodic account sampler or late-receipt telemetry queue.
A native result returned before eventual inclusion may have no later fill
metric; protocol reconciliation still owns business state.

## 9. Boundaries

The shared lifecycle does not coordinate external liquidity or orders between
processes. Contracts and committed recipes enforce authorization, amounts and
protocol safety. Restart loses attempts, order identities and reservations while
external work may continue. Settle existing work before an upgrade.

## 10. Verification

Unit and local Anvil tests cover signing identity, funding, ambiguous submission,
replacement fees and ceilings, deadlines, cancellation, canonical reverts,
reorganizations and older-attempt ownership. Host checks cover RFQ, LI.FI, 3F,
claims and keeper settlement. API fixtures and local chain evidence do not prove
deployment or live execution. Tests never use live signing credentials.
