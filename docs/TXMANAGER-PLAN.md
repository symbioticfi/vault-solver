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
requires both. Quote producers that cannot account for pending work use `LaneReady`; RFQ uses `Available`
and subtracts pending fills through its own reservations. Already-owned recovery work can continue during
contention. Process readiness (`/readyz`) follows `Available`, so a pending transaction never takes quote
servers out of rotation; only startup, shutdown and nonce conflicts do.
Subscribers receive coalesced change notifications and must re-read state and unsubscribe when done.

## 2. Request contract

| Field | Owner and meaning |
|---|---|
| `To`, `Data`, `Value` | Integration supplies the destination, generated calldata and native value; nil value means zero. |
| `GasLimit` | Zero requests exact-call estimation after admission and before signing, with 5% headroom. A supplied limit is reused for normal replacements. |
| `MaxFeePerGas` | Optional profitability ceiling for the normal call and its replacements; cancellation may exceed it within the global ceiling. |
| `CancelAt` | Optional wall-clock cancellation deadline. Integrations derive it from the earliest applicable protocol deadline without extending validity during RPC/planning. |
| `Obsolete` | Context-aware protocol status check before signing and after a receipt sweep finishes without a valid receipt. True drops an unsigned call or starts same-nonce cancellation; errors preserve ownership. Either way the result's `Err` wraps the exported `ErrRequestObsolete`, so the integration can retire the work instead of retrying it. It is not an authorization mechanism. |
| `Confirmations` | Optional override of the manager confirmation depth; an explicit zero skips depth waiting. |
| `Label`, `Solver` | Stable operation name and owning integration for logs/metrics/Sentry. |

The generic manager must not interpret protocol order IDs, statuses, adapters or economic policy.

## 3. Configuration and time budgets

The public YAML block is `txManager`. Values below are application defaults, after config loading.

| Setting | Default | Meaning |
|---|---:|---|
| `confirmations` | 2 | Blocks after inclusion; a request may override. Zero in YAML is treated as unset. |
| `maxFeeGwei` | Required for transaction senders | Finite positive global EIP-1559 ceiling, including cancellation. |
| `broadcastTimeoutMs` | 5000 | Independent timeout of each submission RPC. |
| `accountPollIntervalMs` | 30000 | Complete sender balance/nonce telemetry refresh cadence. |
| `replacementIntervalMs` | 30000 | Fallback bump cadence while fee history is unreadable; also bounds the internal read budgets below. |
| `pendingTimeoutMs` | 300000 | Switch an unresolved call to cancellation; must be at least the replacement interval. |
| `shutdownTimeoutMs` | 60000 | Hard bound on manager drain after shutdown begins. |
| `horizon.maxBlocks` | 6 | Blocks (3–12) the initial fee cap keeps the full tip valid at the maximum base-fee increase. |
| `horizon.blockTimeMs` | 12000 | Slot time: sets the twice-per-block evaluation tick and the next-block estimate timestamp. |
| `horizon.tipFloorGwei` | 0.02 | Tip while the last two blocks had room for the call. |
| `horizon.fullBlockTipGwei` | 0.1 | Tip when one of the last two blocks had no room. |
| `horizon.congestedTipFloorGwei` / `congestedTipCapGwei` | 0.2 / 15 | Clamp on the market tip used when both had no room. |
| `horizon.congestedRewardBlocks` / `congestedRewardPercentile` | 3 / 50 | That market tip: the highest reward percentile over these recent blocks. |
| `horizon.escalateAfterFullBlocks` | 2 | Consecutive full blocks a valid pending call must lose before its tip is repriced. |
| `horizon.stallAfterBlocks` | 3 | Blocks with room a valid pending call may lose before it is re-estimated and rebroadcast. |
| `horizon.gasHeadroomBps` / `fallbackGasHeadroomBps` | 500 / 1000 | Headroom over the next-block estimate, and over a latest-state estimate when the read RPC rejects block overrides. |

Polling defaults to 2 seconds in the Go manager; it is not a separate YAML field. Pending receipt reads,
replacement nonce reads and obsolescence checks each use `min(2 seconds, replacementInterval/2)`; fee reads use
`min(1 second, replacementInterval/2)`. These internal read budgets are separate from broadcast timeout.
Account refresh uses a 5-second context. Backends must honor cancellation. The replacement loop ticks
every `blockTimeMs/2` and acts only on a new block; `replacementIntervalMs` only paces the fallback bump
when fee windows stay unreadable, and an ambiguous broadcast may still be rebroadcast while more than
`broadcastTimeoutMs + blockTimeMs` remains before its deadline.

## 4. Fees, replacements and cancellation

Every call is priced for inclusion in the next block at the lowest spend. Spend is
`gasUsed × (baseFee + tip)`; the fee cap only decides validity, so the manager spends headroom on the cap
and economizes on the tip. This replaced an earlier policy that priced from the fee-history reward median
with a 2× base-fee cap and an optional `tipGwei` floor, and bumped every pending call on a
`replacementIntervalMs` timer: `tipGwei` was removed, and `replacementIntervalMs` now only paces the
fallback below.

- **Fee cap from the exact base-fee bound.** One read of the latest header and an `eth_feeHistory` window
  (`max(congestedRewardBlocks, escalateAfterFullBlocks, stallAfterBlocks, 2)` blocks) gives the next block's
  base fee as the node computes it. The cap is that base fee grown by the EIP-1559 maximum,
  `max(baseFee/8, 1)` per full block (go-ethereum's `DefaultBaseFeeChangeDenominator`, not a knob), for
  `maxBlocks − 1` blocks, plus the tip. At the default 6 that is about 1.8× the next base fee. The initial
  cap reserves a replacement bump under the request and global limits (below); a lower limit shortens the
  horizon, and one below the next base fee fails the send. Missing or invalid fee history fails new
  submissions closed.
- **Tip from block fullness.** A block "has room" when `gasLimit × (1 − gasUsedRatio)` covers the call's
  gas. Room in both of the last two blocks means any positive tip is included, so the call pays
  `tipFloorGwei`; one full block pays `fullBlockTipGwei`; only a run of two full blocks, where the call must
  outbid the marginal transaction, pays the highest `congestedRewardPercentile` reward of the last
  `congestedRewardBlocks` blocks, clamped to the congested floor and cap.
- **Gas against the next block.** Without a request gas limit, the call is estimated with
  `eth_estimateGas(call, "latest", null, {number: head+1, time: head.time + blockTime})`, so time-dependent
  work the latest block already did (Morpho interest accrual, for one) is sized for the block the call
  targets, plus `gasHeadroomBps`. An endpoint that rejects the fourth parameter (invalid params) falls back to
  a latest-state estimate with `fallbackGasHeadroomBps`, logged once per process at Info, as does a backend
  without the chain client's next-block method; a reverting next-block estimate fails the send. An endpoint
  that silently ignores the parameter returns a latest-state estimate with the tighter headroom.
- **Repricing on block evidence.** The loop evaluates each new block (older heads from another endpoint are
  ignored) against the current attempt, counting only blocks after the one it was sent at. That send head is
  the fresher of the fee window's newest block and the latest header, and the wall clock bounds it: no more
  blocks than elapsed slots, plus one in flight, can follow a send, so a read endpoint that served a stale fee
  window when the call went out cannot turn blocks mined before the send into blocks it missed:
  - a fee cap that would lapse within two blocks at the tip floor is repriced (`validity`);
  - the last `escalateAfterFullBlocks` blocks full while the attempt was valid, with the tip rule now asking
    for at least a replacement bump, reprice the tip (`congestion`);
  - `stallAfterBlocks` blocks with room lost while valid mean the relay dropped the attempt, a builder it
    cannot reach built them, or the call outgrew its gas. A normal call is re-estimated for the next block
    and replaced with a larger gas limit once the raw estimate exceeds its limit (`gas`); otherwise its exact
    bytes are rebroadcast, and after two such rebroadcasts a minimal bump replaces them (`stall`);
  - anything else holds, because waiting is free.

  A reprice uses the ordinary replacement rule below (at least a 12.5% bump of both fields, the fresh fees
  when higher, capped at the request or global limit, capped-rebroadcast at the cap).
- **Cancellation** starts at the deadline, shutdown or `Obsolete` triggers and is sent at once. Its fees come
  from the same rule for 21,000 gas, and it is then repriced by the same block evidence; a cancellation whose
  broadcast failed is retried every tick.
- **Fallback.** If fee windows stay unreadable for `replacementIntervalMs`, the pending attempt gets one
  cached-bump replacement per interval (`fallback`), so it cannot freeze. Unreadable windows use the same
  read-streak logging as receipt reads.
- **Quote pricing.** `MaxFeePerGas` returns one replacement bump over the fee cap for the size of the latest
  signed call, so a request ceiling taken from it at quote time still leaves the fill a full horizon.
  Startup (`ValidateFeeHeadroom`) rejects a congested tip cap that cannot fit under the initial fee limit.

Normal calls reserve one 12.5% bump below the global ceiling for cancellation. Initial sends reserve
another bump inside their normal ceiling for a replacement. Request profitability limits also bound
normal attempts. Replacements choose at least a 12.5% bump and fresh fees when available; unavailable
fresh fees fall back to bumping the last signed fees. Cancellation is a 21,000-gas, zero-value self-transfer
with the same nonce and may exceed the request cap, but never the global cap.

All signed variants are retained by exact hash. An ambiguous send does not prove absence from the
network. The next evaluated block rebroadcasts the uncertain attempt's exact bytes once, without adding a
duplicate hash or changing fees; later evidence can reprice it. A cancellation deadline or shutdown
bypasses that grace retry. At the fee cap, the latest applicable attempt can be rebroadcast unchanged.
A `nonce too low` response never by itself authorizes re-signing the calldata at a different nonce.

The cancellation bound is the earlier of the pending timeout and a supplied `CancelAt`. A cancellation
check may become due during fee lookup and promote the replacement. A deadline coinciding with a
replacement tick must not produce a second broadcast for the same tick.

The base-fee bound holds only on chains using the Ethereum base-fee rule (mainnet, Hoodi, Sepolia).
Evaluated windows are the latest blocks, so a lifecycle that goes unevaluated for longer than the window
judges only the most recent blocks.

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
| `cancelled` | Successful cancellation receipt satisfied confirmation policy; `Err` explains that the requested call was cancelled and wraps `ErrRequestObsolete` when `Obsolete` started the cancellation. |
| `cancelled_unconfirmed` | Successful cancellation inclusion observed, but confirmation waiting ended with an error. This is not proof that it is safe to retry the call. `Err` also wraps `ErrRequestObsolete` when `Obsolete` started the cancellation. |
| `submission_error` | Submission/pre-sign path failed without a retained pending lifecycle, including an unsigned call `Obsolete` dropped (`Err` wraps `ErrRequestObsolete`). |
| `tracking_stopped` | Lifecycle tracking stopped before a terminal receipt was established. |

`Result.NotAdmitted` distinguishes admission rejection from an admitted operation failure. `Err`, receipt
and outcome must be interpreted together; cancellation is a terminal result but not a successful fill.
Operational counters are not a canonical accounting ledger.

## 6. RPC routing, nonce conflicts and restart

`chain.rpcAttemptTimeoutMs` configures the HTTP(S) transport's per-endpoint attempt cap (default
20,000 ms), including response-body reads, on all read/write/cancellation clients. Shorter caller
deadlines remain authoritative and are shared across remaining fallback endpoints. Fee/receipt read
budgets and `txManager.broadcastTimeoutMs` are unchanged. This controls the solver's wait for an
endpoint such as eRPC, not eRPC's own upstream retry policy; WebSocket/IPC behavior is unchanged.
Both `chain.Dial` and `chain.DialWithMetrics` accept the attempt timeout explicitly and use the
same transport path; enabling metrics does not change how the timeout is selected.

Normal broadcasts and both latest/pending account nonce reads use one non-fallback write endpoint:
`chain.writeRpcUrl`, or primary `chain.rpcUrl` when omitted. An explicit write endpoint is chain-ID checked.
Optional `chain.cancelRpcUrl` routes same-nonce zero-value self-cancellations to a dedicated, chain-ID-checked
endpoint. Initial cancellation, later cancellation fee bumps, and exact cancellation rebroadcasts all use
that route. Normal fill replacements and nonce reads continue through the ordinary write endpoint.
An empty cancellation URL preserves the ordinary write route; a configured endpoint's error never triggers
cross-endpoint fallback. `txmanager` selects the optional `SendCancellationTransaction` backend capability
only for cancellation attempts; plain EVM backends without that capability keep using `SendTransaction`.
Fee, receipt and state reads use the ordinary read client/fallbacks. Signed bytes are never automatically
replayed across read endpoints. Account telemetry (balance plus mined and pending nonce gauges) reads only
through the read client, via the optional `accountTelemetryBackend` capability `chain.Client` provides, so a
submission relay that rate-limits reads never stalls the refresh; its pending nonce may lag a private
submission until the primary RPC sees it, which is acceptable for a gauge and never used for admission. General transport behavior
remains documented in the [README configuration section](../README.md#configuration).

Startup requires write-endpoint latest and pending nonces to agree. Standard nonce methods cannot reveal
a future transaction queued beyond a gap or a private hidden submission; equality is not recovery proof.
Exact signed attempts are kept in memory. Restart reconciliation must include any separately configured
cancellation endpoint. Before an upgrade from a build allowing multiple unresolved
nonces, drain the EOA's write-endpoint pool. After an unclean exit, reconcile outstanding private
submissions before reusing the EOA. Packaged Compose uses `unless-stopped`: automatic restart can reuse
a nonce before a hidden attempt becomes visible and does not reconstruct lost ownership.

A post-signing `nonce too low` reconciles exact owned hashes. During replacement of an already tracked
lifecycle, an owned receipt proven canonical against a stable head can resolve the conflict immediately;
the lane stays occupied until confirmation completes. An initial-broadcast collision, or replacement
without owned canonical evidence, pauses admission/readiness until reconciliation or operator action.
A later receipt reorg restores the conflict pause. Recovery never relies on guessing that the old call
cannot land or automatically moving its calldata to a different nonce.

Every replacement attempt first checks the write endpoint's latest **mined** nonce with a bounded RPC.
This covers ordinary fee bumps, initial and subsequent cancellations, uncertain exact rebroadcasts and
rebroadcasts at the fee cap. A successful send response from a private relay does not prove the nonce
is still usable. If the mined nonce has advanced beyond the tracked nonce, no replacement is signed or
broadcast: the same exact-hash reconciliation above runs and unexplained consumption pauses admission
and readiness. Receipt polling continues, and a delayed canonical owned receipt can recover normally.
Nonce advancement alone never synthesizes a success/cancellation result, discards tracked attempts or
replays the request at a new nonce. A nonce RPC error defers that replacement until a later tick; it does
not permanently mark a conflict. Pending nonce advancement alone does not suppress replacements.
This check cannot make inclusion and submission atomic; broadcast errors and receipt tracking remain
necessary to reconcile an inclusion racing the subsequent send.

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
| Txmanager | `solver_bot_txmanager_replacements_total` | `label`, `kind`, `reason` | Successfully broadcast replacements, cancellations and exact rebroadcasts (`kind` = `replacement`, `cancellation`, `rebroadcast`). Replacement `reason` is `validity`, `congestion`, `stall`, `gas` or `fallback`; a cancellation reports why cancellation started (`pending_timeout`, `request_deadline`, `shutdown`, `obsolete`); a rebroadcast is `stall`, `uncertain` (ambiguous first broadcast) or `capped` (fee cap reached). Spikes expose fee-policy, relay or congestion problems that terminal outcomes alone cannot show. |
| Txmanager | `solver_bot_txmanager_admission_rejections_total` | `label`, `reason` | Requests rejected before the signed worker lifecycle. Reasons are `manager_stopped`, `nonce_conflict`, `deadline_exceeded`, `caller_cancelled`, or bounded fallback `other`; an expected busy `TrySend` probe is excluded. |
| Txmanager | `solver_bot_txmanager_admission_wait_duration_seconds` | `label`, `outcome` | Time from a real send request until worker admission or a terminal pre-admission outcome. `outcome` is `admitted` or one of the bounded rejection reasons; expected busy `TrySend` probes are excluded. |
| Txmanager | `solver_bot_txmanager_lifecycle_duration_seconds` | `label`, `outcome` | Time from worker admission through broadcast and terminal tracking. It excludes pre-admission nonce-lane wait, so use admission rejections alongside its latency distribution. |
| Txmanager | `solver_bot_txmanager_phase_duration_seconds` | `label`, `phase`, `outcome` | Time spent in each reached worker phase: `prebroadcast`, `pending`, or `confirming`. Reorgs may return a lifecycle to `pending`; the emitted sample contains the cumulative time spent in that phase. |
| Txmanager | `solver_bot_txmanager_account_info` | `address` | Constant `1` identifying the active public transaction-sender address; absent when no configured solver starts txmanager. Private key material is never exposed. |
| Txmanager | `solver_bot_txmanager_account_balance_wei` | — | Last complete native-token balance snapshot of the sender; absent until the first successful complete refresh. Read through the read client, never the write endpoint. |
| Txmanager | `solver_bot_txmanager_account_latest_nonce` | — | Mined nonce from the same complete snapshot; absent until the first successful refresh. |
| Txmanager | `solver_bot_txmanager_account_pending_nonce` | — | Pending nonce from the same complete snapshot as the primary read RPC sees it, so it may lag a privately submitted transaction; compare with latest nonce to detect unknown pending work. It is absent until the first successful refresh. |
| Txmanager | `solver_bot_txmanager_account_refreshes_total` | `outcome` | Complete periodic account snapshots classified as `success` or `error`; failed reads retain the previous scrape-consistent snapshot. |
| Txmanager | `solver_bot_txmanager_account_last_successful_refresh_timestamp` | — | Freshness of the retained balance and nonce snapshot; absent until the first successful refresh. |

## 9. Integration boundaries

| Integration | Keeps its own policy and links to this lifecycle |
|---|---|
| [3F](3F-PLAN.md#51-txmanager--nonce-serialized-sender) | Finalize multicall, off-chain offer signing, offer gating and continued reconciliation/redemption. |
| [RFQ](RFQ-PLAN.md) | Executor fill calldata, signed order/discount deadlines, backend order-status obsolescence, quote gating and result accounting. |
| [LI.FI](LIFI-PLAN.md) | Finalise calldata, order-status obsolescence, protocol validity and capacity/retry bookkeeping. |
| [UniswapX](UNISWAPX-PLAN.md) | Reactor/executor calldata, earliest validity deadline, order-API status obsolescence, profitability ceiling and exclusive-order obligations. |
| [OEV](OEV-PLAN.md) | External settlement and protocol bid nonce; does not start txmanager in an OEV-only process. |

Protocol order/bid nonces are not the shared sender's transaction nonce. Strategy code receives facts and
returns decisions; it does not gain a signer, nonce manager or transaction-sending capability.

## 10. Verification and maintenance

Receipt tests cover 52 independent budgets, timer handling during blocked reads, new/old hashes, reorg
recovery, teardown and coincident cancellation/replacement ticks. Existing tests cover fee limits,
ambiguous broadcasts, nonce conflicts, admission, result semantics and logging. The local Anvil target
covers a real pending call repriced after a base-fee spike and real cancellations; unit test success alone
is not that integration proof.
Cancellation outcome tests also distinguish a satisfied confirmation policy from an interrupted wait;
RFQ tests consume that distinction when deciding whether another fill is safe.
Fee tests cover the base-fee bound, the tip rule, each repricing decision, fee-window parsing, the
next-block estimate and its fallbacks, a stale send head, and scripted-chain lifecycles for stall
rebroadcasts, congestion, validity, gas growth, unreadable windows and deadline cancellation. Lifecycle tests that need a replacement
mine a block that supplies the evidence, since no timer bumps a pending call.
Run repository-required build, race/coverage and lint gates for implementation changes. Current reader
validation is local; it does not establish deployment or production rollout status.

For each deployment, check that its read RPC honours `eth_estimateGas` block overrides (the
"next-block gas estimate unsupported" Info line says it does not) and watch `replacements_total{reason}`: sustained `stall` rebroadcasts point at the relay, `gas` at contention on
the same vaults, and `fallback` at unreadable fee history.

Keep shared lifecycle design and metric contracts here. Update integration plans only when their own
request construction, readiness, capacity or protocol behavior changes. Preserve operator-facing setup,
EOA exclusivity and maintenance warnings in README, with links here for the detailed mechanism.

### Receipt failure diagnostics

Existing sweep-level error suppression also carries distinct checked/all tracked hashes, completed
RPC count (including priority reads), elapsed sweep time and last RPC duration. `rpcBudgetTotalMs`
sums the effective per-call budgets of completed reads; there is no shared sweep deadline. The first
failed read supplies the logged hash, stable `reason_code` and `cancelCause`, even when later reads
succeed. Lifecycle shutdown retains its existing exit without an extra partial-sweep error log.
