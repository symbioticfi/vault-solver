# Transaction manager

This is the source of truth for the shared transaction lifecycle in `internal/txmanager`.
Integration plans describe calldata, protocol deadlines, capacity and quote policy; they link here for
admission, fees, nonce ownership, receipts and shutdown. The [README](../README.md) remains the operator
entry point. Configuration is defined by [config.go](../internal/config/config.go) and wired in
[run.go](../cmd/vault-solver/run.go); implementation lives in
[txmanager.go](../internal/txmanager/txmanager.go).

## 1. Ownership and admission

One process has one chain client, signer and transaction manager shared by its transaction-sending
solvers. Only one signed nonce lifecycle may be unresolved. The same EOA must not be assigned to two
processes, even with different RPC URLs: the managers cannot coordinate nonce ownership across processes.
An integration submitted externally can return false from `RequiresTxManager`; an external-only process
does not initialize/start the manager or require `txManager.maxFeeGwei`.

Solvers build calldata and submit a `Request`; they never sign or broadcast transactions directly.
`Send` waits for the terminal result. `SendAsync` waits for admission and returns a result channel;
its name does not mean admission is non-blocking. `TrySend` declines an occupied lane without signing;
when admitted, it waits for the result. Caller context and `CancelAt` bound pre-admission waiting.
Once enqueued, the manager owns execution: caller cancellation is not proof the signed call cannot land.
Definitive pre-sign/submission failures can finish without a receipt; accepted ambiguous sends stay tracked.

`Available()` reports nonce safety; `Idle()` reports absence of queued/admitted demand. `LaneReady()`
requires both. Quote producers use `LaneReady`; already-owned recovery work can continue during contention.
Subscribers receive coalesced change notifications and must re-read state and unsubscribe when done.

## 2. Request contract

| Field | Owner and meaning |
|---|---|
| `To`, `Data`, `Value` | Integration supplies the destination, generated calldata and native value; nil value means zero. |
| `GasLimit` | Zero requests exact-call estimation after admission and before signing, with 5% headroom. A supplied limit is reused for normal replacements. |
| `MaxFeePerGas` | Optional profitability ceiling for the normal call and its replacements; cancellation may exceed it within the global ceiling. |
| `CancelAt` | Optional wall-clock cancellation deadline. Integrations derive it from the earliest applicable protocol deadline without extending validity during RPC/planning. |
| `Obsolete` | Context-aware protocol status check before signing and after a receipt sweep finishes without a valid receipt. True drops an unsigned call or starts same-nonce cancellation; errors preserve ownership. It is not an authorization mechanism. |
| `Confirmations` | Optional override of the manager confirmation depth; an explicit zero skips depth waiting. |
| `Label`, `Solver` | Stable operation name and owning integration for logs/metrics/Sentry. |

The generic manager must not interpret protocol order IDs, statuses, adapters or economic policy.

## 3. Configuration and time budgets

The public YAML block is `txManager`. Values below are application defaults, after config loading.

| Setting | Default | Meaning |
|---|---:|---|
| `confirmations` | 2 | Blocks after inclusion; a request may override. Zero in YAML is treated as unset. |
| `maxFeeGwei` | Required for transaction senders | Finite positive global EIP-1559 ceiling, including cancellation. |
| `tipGwei` | 0 | Positive mandatory priority-fee floor; zero selects fee-history pricing. |
| `broadcastTimeoutMs` | 5000 | Independent timeout of each submission RPC. |
| `accountPollIntervalMs` | 30000 | Complete sender balance/nonce telemetry refresh cadence. |
| `replacementIntervalMs` | 30000 | Pending replacement/rebroadcast cadence. |
| `pendingTimeoutMs` | 300000 | Switch an unresolved call to cancellation; must be at least the replacement interval. |
| `shutdownTimeoutMs` | 60000 | Hard bound on manager drain after shutdown begins. |

Polling defaults to 2 seconds in the Go manager; it is not a separate YAML field. Pending receipt reads
and obsolescence checks each use `min(2 seconds, replacementInterval/2)`; fee reads use
`min(1 second, replacementInterval/2)`. These internal read budgets are separate from broadcast timeout.
Account refresh uses a 5-second context. Backends must honor cancellation.

## 4. Fees, replacements and cancellation

A positive `tipGwei` is mandatory; a larger node suggestion is advisory and clamped to available headroom.
With zero tip, pricing uses the median gas-weighted p25 priority reward from the latest five blocks,
also clamped to headroom, so one low-reward block cannot drag the tip to zero. Missing/invalid fee history
fails new submissions closed; a positive floor provides an operator-controlled fallback. Startup rejects
a floor leaving no base-fee room under the initial cap; runtime admission also checks the current base fee.

Normal calls reserve one 12.5% bump below the global ceiling for cancellation. Initial sends reserve
another bump inside their normal ceiling for a replacement. Request profitability limits also bound
normal attempts. Replacements choose at least a 12.5% bump and fresh fees when available; unavailable
fresh fees fall back to bumping the last signed fees. Cancellation is a 21,000-gas, zero-value self-transfer
with the same nonce and may exceed the request cap, but never the global cap.

All signed variants are retained by exact hash. An ambiguous send does not prove absence from the
network. The next normal replacement tick rebroadcasts the uncertain attempt's exact bytes once, without
adding a duplicate hash or changing fees; a later tick can bump fees. A cancellation deadline or shutdown
bypasses that grace retry. At the fee cap, the latest applicable attempt can be rebroadcast unchanged.
A `nonce too low` response never by itself authorizes re-signing the calldata at a different nonce.

The cancellation bound is the earlier of the pending timeout and a supplied `CancelAt`. A cancellation
check may become due during fee lookup and promote the replacement. A deadline coinciding with a
replacement tick must not produce a second broadcast for the same tick.

## 5. Receipt polling and confirmation

The lifecycle goroutine owns mutable attempts, sweep progress, fee/cancellation state, failure streaks
and outcomes. One reader goroutine performs pending receipt RPC I/O only. The owner sends a copied
attempt over an unbuffered channel and receives its receipt/error alongside lifecycle timers. At most
one pending receipt read is outstanding; the reader neither changes ownership nor logs lifecycle events.

Each RPC gets its own timeout, bounded by the lifecycle context. There is no shared sweep deadline that
truncates later hashes. Sequential reads avoid a burst when replacements accumulate. A slow RPC can
delay discovery of a later receipt, but cannot hold up the owner's timer handling. Other owner I/O,
including broadcast, obsolescence and confirmation checks, retains its existing bounds.

An ordinary sweep covers a fixed number of tracked variants in round-robin order. Between RPCs, the
newest appended variant gets a priority read without resetting the ordinary cursor. Priority and ordinary
reads alternate while both are available, preventing repeated replacements from starving older hashes.
Intermediate new variants superseded before a priority read enter the next ordinary sweep. Poll ticks
never queue overlapping sweeps; every attempt remains tracked. `NotFound` is a successful RPC
without a receipt. RPC failure streaks are judged per sweep, not cleared by one hash while another fails.
Only after such a sweep completes without a valid receipt does the owner evaluate `Obsolete`: a protocol
terminal status may describe our own successful fill. The callback is not run on intervening poll ticks.
Deadline and shutdown cancellation remain independent of receipt progress. This ordering reduces the
receipt/status race but cannot make separate on-chain reads atomic; exact-hash reconciliation is retained.
Invalid receipts are separately rejected: receipt/block number must exist, transaction hash must match,
and block hash must be nonzero.

Only the owner accepts a receipt candidate and begins confirmation; the reader is idle during that wait.
Confirmation uses a stable head and hash-addressed parent ancestry to prove that the receipt belongs to
that head, without relying on endpoint affinity. Read fallbacks remain available. Incoherent/unavailable
snapshots are retried; two consecutive missing receipts or a proven fork change can resume pending
tracking. A read error breaks the consecutive-miss streak. A receipt for an older signed variant remains
valid evidence for the same nonce.

| Outcome | Meaning |
|---|---|
| `confirmed` | Normal call succeeded and satisfied confirmation policy. |
| `included_unconfirmed` | Successful inclusion observed, but confirmation waiting ended with an error. |
| `reverted` | Receipt reports execution failure; an error during confirmation is retained in the result. |
| `cancelled` | Successful cancellation receipt satisfied confirmation policy; `Err` explains that the requested call was cancelled. |
| `cancelled_unconfirmed` | Successful cancellation inclusion observed, but confirmation waiting ended with an error. This is not proof that it is safe to retry the call. |
| `submission_error` | Submission/pre-sign path failed without a retained pending lifecycle. |
| `tracking_stopped` | Lifecycle tracking stopped before a terminal receipt was established. |

`Result.NotAdmitted` distinguishes admission rejection from an admitted operation failure. `Err`, receipt
and outcome must be interpreted together; cancellation is a terminal result but not a successful fill.
Operational counters are not a canonical accounting ledger.

## 6. RPC routing, nonce conflicts and restart

Broadcasts and both latest/pending account nonce reads use one non-fallback write endpoint:
`chain.writeRpcUrl`, or primary `chain.rpcUrl` when omitted. An explicit write endpoint is chain-ID checked.
Fee, receipt and state reads use the ordinary read client/fallbacks. Signed bytes are never automatically
replayed across read endpoints. Balance telemetry prefers the write endpoint and can fall back to the
read client when a submission-only relay does not support balance reads. General transport behavior
remains documented in the [README configuration section](../README.md#configuration).

Startup requires write-endpoint latest and pending nonces to agree. Standard nonce methods cannot reveal
a future transaction queued beyond a gap or a private hidden submission; equality is not recovery proof.
Exact signed attempts are kept in memory. Before an upgrade from a build allowing multiple unresolved
nonces, drain the EOA's write-endpoint pool. After an unclean exit, reconcile outstanding private
submissions before reusing the EOA. Packaged Compose uses `unless-stopped`: automatic restart can reuse
a nonce before a hidden attempt becomes visible and does not reconstruct lost ownership.

A post-signing `nonce too low` reconciles exact owned hashes. During replacement of an already tracked
lifecycle, an owned receipt proven canonical against a stable head can resolve the conflict immediately;
the lane stays occupied until confirmation completes. An initial-broadcast collision, or replacement
without owned canonical evidence, pauses admission/readiness until reconciliation or operator action.
A later receipt reorg restores the conflict pause. Recovery never relies on guessing that the old call
cannot land or automatically moving its calldata to a different nonce.

## 7. Shutdown

Accepted tracking uses a manager lifecycle context detached from caller/intake cancellation. Solvers stop
new commitments and finish their protocol preparation while the shared manager remains available to
accepted work. Manager shutdown closes admission and requests active same-nonce cancellation when nonce
ownership is not conflicted. It drains for at most `shutdownTimeoutMs`, then cancels lifecycle RPCs and
delivers the shutdown-deadline error so process teardown can proceed. This does not guarantee mining or
confirmation before exit. Configure orchestrator grace for solver preparation/drain plus manager drain;
see the composition in [run.go](../cmd/vault-solver/run.go).

Every lifecycle exit cancels and joins its receipt reader, including one blocked delivering a result.
The manager hard stop can return before an uncooperative backend exits; worker teardown relies on RPC
context compliance, and process teardown remains the ultimate bound.

## 8. Observability

The generic manager logs with the request's solver and operation label. Receipt transport failures emit
one error at streak start, debug while repeating, an error reminder every five minutes and an info on
recovery. A successful RPC is not evidence of mined inclusion. Grouping/log transport is implemented in
[observability](../internal/observability/sentry.go), independently of protocol integrations.
Gas-estimation and receipt-revert logs omit calldata and simulator URLs to avoid copying transaction
authorizations into logs or Sentry; receipt-revert diagnostics retain the hash, label and nonce.

Each request runs under a `txmanager.send <label>` span opened in `sendAsync` from the caller's
context, so it is a child of the submitting fill span and covers the admission wait as well as the
broadcast. The worker carries that span on its own contexts, so it survives the manager's deliberate
detachment from the caller, and ends it with the terminal `tx.outcome` before the result is delivered.
Children are `txmanager.broadcast` and one `txmanager.replace` per replacement; account polls root
`txmanager.account_poll`. The per-request logger is derived from the send span, so every lifecycle
line carries `trace_id`. Spans, attributes, and the propagation rules are specified in
[TRACING-PLAN](TRACING-PLAN.md) §3.4–§4.

An active manager refreshes balance, latest nonce and pending nonce into one complete snapshot. Failed
refreshes retain the previous snapshot; account gauges are absent before first success. A locked
collector exports a scrape-consistent view. An external-only process exposes no txmanager account series.
Operation labels are stable names such as `redeem`, `rfq-fill`, `lifi-fill`, `uniswapx-fill`.

### Metrics

Names include the `solver_bot_` prefix; deployment identity is supplied by scrape target labels.
Definitions: [metrics.go](../internal/txmanager/metrics.go),
[account_metrics.go](../internal/txmanager/account_metrics.go).

| Component | Metric | Labels | Meaning |
|---|---|---|---|
| Txmanager | `solver_bot_txmanager_requests_total` | `label`, `outcome` | Terminal results of logical on-chain operations. This is the primary confirmed/reverted/submission-failure funnel for every solver. |
| Txmanager | `solver_bot_txmanager_inflight` | `label` | Requests accepted by the txmanager worker and still awaiting a terminal result; sustained values expose stuck transactions or nonce congestion. |
| Txmanager | `solver_bot_txmanager_gas_used_total` | `label`, `outcome` | Receipt gas for mined transactions, including reverts. Divide by the matching request count for average gas; this is gas units, not native-token cost. |
| Txmanager | `solver_bot_txmanager_fee_paid_wei_total` | `label`, `outcome` | Actual native-token fee paid by mined transactions, calculated from receipt `gasUsed × effectiveGasPrice`, including reverted and mined cancellation transactions. |
| Txmanager | `solver_bot_txmanager_replacements_total` | `label`, `kind` | Successfully broadcast replacements and cancellations. Spikes expose fee-policy or congestion problems that terminal outcomes alone cannot show. |
| Txmanager | `solver_bot_txmanager_admission_rejections_total` | `label`, `reason` | Requests rejected before the signed worker lifecycle. Reasons are `manager_stopped`, `nonce_conflict`, `deadline_exceeded`, `caller_cancelled`, or bounded fallback `other`; an expected busy `TrySend` probe is excluded. |
| Txmanager | `solver_bot_txmanager_admission_wait_duration_seconds` | `label`, `outcome` | Time from a real send request until worker admission or a terminal pre-admission outcome. `outcome` is `admitted` or one of the bounded rejection reasons; expected busy `TrySend` probes are excluded. |
| Txmanager | `solver_bot_txmanager_lifecycle_duration_seconds` | `label`, `outcome` | Time from worker admission through broadcast and terminal tracking. It excludes pre-admission nonce-lane wait, so use admission rejections alongside its latency distribution. |
| Txmanager | `solver_bot_txmanager_phase_duration_seconds` | `label`, `phase`, `outcome` | Time spent in each reached worker phase: `prebroadcast`, `pending`, or `confirming`. Reorgs may return a lifecycle to `pending`; the emitted sample contains the cumulative time spent in that phase. |
| Txmanager | `solver_bot_txmanager_account_info` | `address` | Constant `1` identifying the active public transaction-sender address; absent when no configured solver starts txmanager. Private key material is never exposed. |
| Txmanager | `solver_bot_txmanager_account_balance_wei` | — | Last complete native-token balance snapshot of the sender; absent until the first successful complete refresh. A distinct write endpoint is tried first, then the fallback-capable read client if the submission endpoint does not serve balance reads. |
| Txmanager | `solver_bot_txmanager_account_latest_nonce` | — | Mined nonce from the same complete snapshot; absent until the first successful refresh. |
| Txmanager | `solver_bot_txmanager_account_pending_nonce` | — | Pending nonce from the same complete snapshot; compare with latest nonce to detect unknown pending work. It is absent until the first successful refresh. |
| Txmanager | `solver_bot_txmanager_account_refreshes_total` | `outcome` | Complete periodic account snapshots classified as `success` or `error`; failed reads retain the previous scrape-consistent snapshot. |
| Txmanager | `solver_bot_txmanager_account_last_successful_refresh_timestamp` | — | Freshness of the retained balance and nonce snapshot; absent until the first successful refresh. |

## 9. Integration boundaries

| Integration | Keeps its own policy and links to this lifecycle |
|---|---|
| [3F](3F-PLAN.md#51-txmanager--nonce-serialized-sender) | Finalize multicall, off-chain offer signing, offer gating and continued reconciliation/redemption. |
| [RFQ](RFQ-PLAN.md) | Executor fill calldata, signed order/discount deadlines, quote gating and result accounting. |
| [LI.FI](LIFI-PLAN.md) | Finalise calldata, order-status obsolescence, protocol validity and capacity/retry bookkeeping. |
| [UniswapX](UNISWAPX-PLAN.md) | Reactor/executor calldata, earliest validity deadline, profitability ceiling and exclusive-order obligations. |
| [OEV](OEV-PLAN.md) | External settlement and protocol bid nonce; does not start txmanager in an OEV-only process. |

Protocol order/bid nonces are not the shared sender's transaction nonce. Strategy code receives facts and
returns decisions; it does not gain a signer, nonce manager or transaction-sending capability.

## 10. Verification and maintenance

Receipt tests cover 52 independent budgets, timer handling during blocked reads, new/old hashes, reorg
recovery, teardown and coincident cancellation/replacement ticks. Existing tests cover fee limits,
ambiguous broadcasts, nonce conflicts, admission, result semantics and logging. The local Anvil target
covers real pending replacements/cancellations; unit test success alone is not that integration proof.
Cancellation outcome tests also distinguish a satisfied confirmation policy from an interrupted wait;
RFQ tests consume that distinction when deciding whether another fill is safe.
Run repository-required build, race/coverage and lint gates for implementation changes. Current reader
validation is local; it does not establish deployment or production rollout status.

Keep shared lifecycle design and metric contracts here. Update integration plans only when their own
request construction, readiness, capacity or protocol behavior changes. Preserve operator-facing setup,
EOA exclusivity and maintenance warnings in README, with links here for the detailed mechanism.

### Receipt failure diagnostics

Existing sweep-level error suppression also carries distinct checked/all tracked hashes, completed
RPC count (including priority reads), elapsed sweep time and last RPC duration. `rpcBudgetTotalMs`
sums the effective per-call budgets of completed reads; there is no shared sweep deadline. The first
failed read supplies the logged hash, stable `reason_code` and `cancelCause`, even when later reads
succeed. Lifecycle shutdown retains its existing exit without an extra partial-sweep error log.
