# Transaction manager

This is the source of truth for the shared transaction lifecycle in `internal/txmanager`.
Integration plans describe calldata, protocol deadlines, capacity and quote policy; they link here for
admission, fees, nonce ownership, receipts and shutdown. The [README](../README.md) remains the operator
entry point. Configuration is defined by [config.go](../internal/config/config.go) and wired in
[run.go](../cmd/vault-solver/run.go); implementation lives in
[txmanager.go](../internal/txmanager/txmanager.go).

## 1. Ownership and admission

One process has one chain client, signer and transaction manager shared by its transaction-sending
solvers. Only one signed lifecycle is actively tracked **per process**. Each initial send reads the latest
mined account nonce from the sending endpoint and uses that lowest unconsumed nonce. Fresh work can replace
an unmined call, including after restart; it never queues a higher nonce just because pending counts the
old call. Independent processes can share the EOA without a database,
shared file, leader, coordinator or peer discovery. They may race on a nonce; an initial collision ends
that request promptly and leaves later work eligible. The [replica plan](REPLICA-PLAN.md) records the
account and integration limits.
An integration submitted externally can return false from `RequiresTxManager`; an external-only process
does not initialize/start the manager or require `txManager.maxFeeGwei`.

Solvers build calldata and submit a `Request`; they never sign or broadcast transactions directly.
`Send` waits for the terminal result. `SendAsync` waits for admission and returns a result channel;
its name does not mean admission is non-blocking. `TrySend` declines an occupied lane without signing;
when admitted, it waits for the result. Caller context and `Deadline` bound pre-admission waiting.
Once enqueued, the manager owns execution: caller cancellation is not proof the signed call cannot land.
Definitive pre-sign/submission failures can finish without a receipt; accepted ambiguous sends stay tracked.

`Available()` reports initial mined-nonce RPC readiness; `Idle()` reports absence of queued/admitted demand. `LaneReady()`
requires both. Quote producers that cannot account for pending work use `LaneReady`; RFQ uses `Available`
and subtracts pending fills through its own reservations. Already-owned recovery work can continue during
contention. Process readiness (`/readyz`) follows `Available`, so an owned pending transaction does not
by itself take quote servers out of rotation; startup and shutdown do.
Subscribers receive coalesced change notifications and must re-read state and unsubscribe when done.

## 2. Request contract

| Field | Owner and meaning |
|---|---|
| `To`, `Data`, `Value` | Integration supplies the destination, generated calldata and native value; nil value means zero. |
| `GasLimit` | Zero requests exact-call estimation after admission and before signing, with 5% headroom. A supplied limit is reused for normal replacements. |
| `MaxFeePerGas` | Optional profitability ceiling for every attempt of this business call, including reuse of an abandoned nonce. |
| `Deadline` | Optional wall-clock submission and pending-tracking deadline. Integrations derive it from the earliest applicable protocol deadline without extending validity during RPC/planning. |
| `Obsolete` | Context-aware protocol status check before signing and after a receipt sweep finishes without a valid receipt. True drops an unsigned call or abandons pending tracking; errors preserve ownership. Either way the result's `Err` wraps the exported `ErrRequestObsolete`, so the integration can retire the work instead of retrying it. It is not an authorization mechanism. |
| `Confirmations` | Optional override of the manager confirmation depth; an explicit zero skips depth waiting. |
| `Label`, `Solver` | Stable operation name and owning integration for logs/metrics/Sentry. |

The generic manager must not interpret protocol order IDs, statuses, adapters or economic policy.
Gas estimation execution failures wrap `ExecutionRevertError`, preserving the original error and
decoded RPC revert bytes when supplied. The integration decodes protocol errors, reconciles current
business state and chooses log severity. The manager logs estimate transport/RPC availability failures
at Error, but does not page on execution reverts before integration reconciliation.

## 3. Configuration and time budgets

The public YAML block is `txManager`. Values below are application defaults, after config loading.

| Setting | Default | Meaning |
|---|---:|---|
| `confirmations` | 2 | Blocks after inclusion; a request may override. Zero in YAML is treated as unset. |
| `maxFeeGwei` | Required for transaction senders | Finite positive global EIP-1559 ceiling for every broadcast. |
| `broadcastTimeoutMs` | 5000 | Independent timeout of each submission RPC. |
| `accountPollIntervalMs` | 30000 | Complete sender balance/nonce telemetry refresh cadence. |
| `replacementIntervalMs` | 30000 | Fallback bump cadence while fee history is unreadable; also bounds the internal read budgets below. |
| `pendingTimeoutMs` | 300000 | Abandon an unresolved owned call and remember its nonce/fee floor for a fresh business request. Must be at least the replacement interval. |
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
`min(1 second, replacementInterval/2)`; a stalled call's gas re-estimate, fallback included, uses
`min(5 seconds, replacementInterval/2)` and ends no later than the call's submission deadline. These
internal read budgets are separate from broadcast timeout. The initial estimate, before signing, has no
budget of its own: the request's `Deadline` and manager shutdown bound it, so a request without `Deadline`
(3F `redeem`) can hold the worker and the nonce lane while a read endpoint withholds its estimate
([3F plan §10](3F-PLAN.md#10-pending--deferred-items-post-phase-3)).

Each initial mined nonce read and replacement/fee-hint nonce check has its own
`min(2 seconds, replacementInterval/2)` timeout, bounded by the caller or lifecycle context. Receipt
confirmation retains bounded header and ancestry reads; no account-confirmation proof runs before signing.

Account refresh uses a 5-second context. Backends must honor cancellation. The replacement loop ticks
every `blockTimeMs/2` and acts only on a new block; `replacementIntervalMs` only paces the fallback bump
when fee windows stay unreadable, and an ambiguous broadcast may still be rebroadcast while more than
`broadcastTimeoutMs + blockTimeMs` remains before its deadline.

## 4. Fees, replacements and abandonment

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
    bytes are rebroadcast, and after two such rebroadcasts a minimal bump replaces them (`stall`). The
    re-estimate runs on the lifecycle goroutine under its own budget (§3), so a read endpoint that never
    answers it holds up receipts for at most that budget. If the request deadline or pending timeout
    passes during it, tracking is abandoned without rebroadcasting expired work;
  - anything else holds, because waiting is free.

  A reprice uses the ordinary replacement rule below (at least a 12.5% bump of both fields, the fresh fees
  when higher, capped at the request or global limit, capped-rebroadcast at the cap).
- **Abandonment** at the request deadline, pending timeout or `Obsolete` releases the local lifecycle
  without broadcasting. The result is `abandoned`, with the attempted hash and an unknown execution result.
  The manager remembers the nonce and highest signed fees; the next eligible business request can replace
  it while a bounded latest-state nonce check still shows it unused.
- **Fallback.** If fee windows stay unreadable for `replacementIntervalMs`, the pending attempt gets one
  cached-bump replacement per interval (`fallback`), so it cannot freeze. Unreadable windows use the same
  read-streak logging as receipt reads.
- **Quote pricing.** `MaxFeePerGas` prices the size of the latest signed call and includes the retained
  replacement floor retained after abandonment or an initial underpriced response. A bounded mined-nonce
  check uses the hint only at exactly that nonce and fails pricing closed if unavailable. The returned
  ceiling includes one ordinary bump when the global cap permits it, so fresh planning accounts for
  the cost of replacing pending work.
  Startup (`ValidateFeeHeadroom`) rejects a congested tip cap that cannot fit under the initial fee limit.

Ordinary initial sends reserve one 12.5% bump under the request and global ceilings for a replacement.
Reusing an abandoned nonce may consume that headroom when its required floor is higher, but still obeys
the full request and global ceilings.
Replacements choose at least a 12.5% bump and fresh fees when available; unavailable fresh fees fall back
to bumping the last signed fees. A fresh request reusing an abandoned nonce has new destination, calldata,
value and gas; both fee fields must exceed the remembered floors by at least 12.5%. Its own profitability
ceiling and the global ceiling remain authoritative. If the fresh call cannot fit, it fails before signing
and retains the hint for later eligible work. The bot never sends zero-value self-transfers to clear nonces.

All signed variants are retained by exact hash. An ambiguous send does not prove absence from the
network. The next evaluated block rebroadcasts the uncertain attempt's exact bytes once, without adding a
duplicate hash or changing fees; later evidence can reprice it. A request deadline or pending timeout
bypasses that grace retry and abandons tracking. At the fee cap, the latest applicable attempt can be rebroadcast unchanged.
A `nonce too low` response never by itself authorizes re-signing the calldata at a different nonce.

The tracking bound is the earlier of the pending timeout and a supplied `Deadline`. It is checked again
after fee or gas lookup, so a deadline coinciding with a replacement tick produces no late broadcast.
Abandonment does not prove the old call cannot land: a timeout can end tracking of still-valid work, and
a protocol status/deadline check is separate from inclusion. Solvers reconcile current business state
before rebuilding a retry.


The base-fee bound holds only on chains using the Ethereum base-fee rule (mainnet, Hoodi, Sepolia).
Evaluated windows are the latest blocks, so a lifecycle that goes unevaluated for longer than the window
judges only the most recent blocks.

## 5. Receipt polling and confirmation

The lifecycle goroutine owns mutable attempts, sweep progress, fee/deadline state, failure streaks
and outcomes. One reader goroutine performs pending receipt RPC I/O only. The owner sends a copied
attempt over an unbuffered channel and receives its receipt/error alongside lifecycle timers. At most
one pending receipt read is outstanding; the reader neither changes ownership nor logs lifecycle events.

Each RPC gets its own timeout, bounded by the lifecycle context. There is no shared sweep deadline that
truncates later hashes. Sequential reads avoid a burst when replacements accumulate. A slow RPC can
delay discovery of a later receipt, but cannot hold up the owner's timer handling. Other owner I/O
(broadcast, fee and nonce reads, obsolescence and confirmation checks, the stall gas re-estimate) runs
on the owner itself, so each call carries its own bound: while one runs, the owner services no receipt,
timer or shutdown request, and the lifecycle context is detached from manager cancellation until the
shutdown drain expires. New owner I/O must add a bound of its own. Signing is local CPU work today; a
remote signer must bound `SignTx` itself and honor the request/lifecycle context.

An ordinary sweep covers a fixed number of tracked variants in round-robin order. Between RPCs, the
newest appended variant gets a priority read without resetting the ordinary cursor. Priority and ordinary
reads alternate while both are available, preventing repeated replacements from starving older hashes.
Intermediate new variants superseded before a priority read enter the next ordinary sweep. Poll ticks
never queue overlapping sweeps; every attempt remains tracked. `NotFound` is a successful RPC
without a receipt. RPC failure streaks are judged per sweep, not cleared by one hash while another fails.
Only after such a sweep completes without a valid receipt does the owner evaluate `Obsolete`: a protocol
terminal status may describe our own successful fill. The callback is not run on intervening poll ticks.
Pending deadline and timeout timers are handled independently of receipt RPC progress. Once due, a final
receipt grace of at most one pending receipt-read budget gives an available owned receipt precedence;
incomplete or unavailable sweeps then return execution-unknown abandonment and release the lane.
`Obsolete` remains evaluated after a complete ordinary sweep. Once a valid receipt begins confirmation,
the manager follows confirmation policy rather than abandoning included work at the pending deadline.
This ordering reduces the receipt/status race but cannot make separate on-chain reads atomic.
Invalid receipts are separately rejected: receipt/block number must exist, transaction hash must match,
and block hash must be nonzero.

Only the owner accepts a receipt candidate and begins confirmation; the reader is idle during that wait.
Before waiting for depth, a canonical preflight uses a stable head and hash-addressed parent ancestry
under one overall receipt-read budget, capped by any final receipt grace. An unproven receipt resumes
pending tracking; only a proven inclusion continues confirmation beyond the pending deadline.
Confirmation repeats the stable-head ancestry proof at the configured depth without endpoint affinity. Read fallbacks remain available. Incoherent/unavailable
snapshots are retried; two consecutive missing receipts or a proven fork change can resume pending
tracking. A read error breaks the consecutive-miss streak. A receipt for an older signed variant remains
valid evidence for the same nonce.

After delivering a terminal result, the manager stops watching that lifecycle. It does not detect later
reorgs of completed transactions or reopen integration orders.

| Outcome | Meaning |
|---|---|
| `confirmed` | Normal call succeeded and satisfied confirmation policy. |
| `included_unconfirmed` | Successful inclusion observed, but confirmation waiting ended with an error. |
| `reverted` | Receipt reports execution failure; an error during confirmation is retained in the result. |
| `abandoned` | Pending timeout, request deadline or `Obsolete` ended tracking without another broadcast. `Err` wraps `ErrAbandoned`, and also `ErrRequestObsolete` for an obsolete call. The attempted hash is retained without a receipt; execution remains unknown. |
| `submission_error` | Submission/pre-sign path failed without a retained pending lifecycle, including an unsigned call `Obsolete` dropped (`Err` wraps `ErrRequestObsolete`). |
| `tracking_stopped` | Lifecycle tracking stopped before a terminal receipt was established. |
| `nonce_conflict` | Initial broadcast returned nonce-too-low or replacement-underpriced. The attempted hash is retained in the result, with no receipt. The lane is released immediately; execution is unknown. |
| `nonce_consumed` | The sending endpoint reports a higher mined nonce after every tracked hash returned `NotFound`. No receipt is synthesized; execution is unknown and account observation is subject to lag/reorgs. |

`Result.NotAdmitted` distinguishes admission rejection from an admitted operation failure. `Err`, receipt
and outcome must be interpreted together; abandonment is a terminal tracking result with unknown execution.
Operational counters are not a canonical accounting ledger.
`abandoned`, `nonce_conflict` and `nonce_consumed` require protocol/backend reconciliation before an integration decides whether to retry;
the manager never moves the original calldata to a new nonce automatically. The result's hash identifies
an owned attempt, not a discovered winning transaction or proof of successful inclusion.

## 6. RPC routing, nonce conflicts and restart

`chain.rpcAttemptTimeoutMs` configures the HTTP(S) transport's per-endpoint attempt cap (default
20,000 ms), including response-body reads, on all read/write clients. Shorter caller
deadlines remain authoritative and are shared across remaining fallback endpoints. Fee/receipt read
budgets and `txManager.broadcastTimeoutMs` are unchanged. This controls the solver's wait for an
endpoint such as eRPC, not eRPC's own upstream retry policy; WebSocket/IPC behavior is unchanged.
Both `chain.Dial` and `chain.DialWithMetrics` accept the attempt timeout explicitly and use the
same transport path; enabling metrics does not change how the timeout is selected.

Normal broadcasts and latest mined admission account nonce reads use one non-fallback write endpoint:
`chain.writeRpcUrl`, or primary `chain.rpcUrl` when omitted. An explicit write endpoint is chain-ID checked.
Fee, receipt and state reads use the ordinary read client/fallbacks. Signed bytes are never automatically
replayed across read endpoints. Account telemetry (balance plus mined and pending nonce gauges) reads only
through the read client, via the optional `accountTelemetryBackend` capability `chain.Client` provides, so a
submission relay that rate-limits reads never stalls the refresh; its pending nonce may lag a private
submission until the primary RPC sees it, which is acceptable for a gauge and never used for admission. General transport behavior
remains documented in the [README configuration section](../README.md#configuration).

### Fresh mined nonce and initial collisions

`Initialize` verifies one bounded latest mined nonce read. Every new broadcast reads
`eth_getTransactionCount(address, "latest")` again immediately before signing, without a cached counter,
empty-pool requirement or account-confirmation proof. Pending state never advances this selection, so
fresh requests do not queue higher nonces behind unmined work. All replicas can replace the lowest
unconsumed nonce after a restart, without remembering or querying the pending call.
If the manager retains a fee hint, it applies only when its nonce equals that mined count. Advancement
or a lower nonce after a reorg discards an inapplicable hint; a failed nonce read defers signing and
retains it. Failed preparation, signing or definitive submission also retains the applicable hint.
The sending endpoint must serve current mined state; private pending visibility is not required.
The local lifecycle slot serializes this process's work only. Two processes can read the same nonce.

An initial nonce-too-low or replacement-underpriced response yields `OutcomeNonceConflict`, an error
wrapping `ErrNonceConflict`, and the exact attempted hash with no receipt. The worker ends that request
and releases the slot immediately. An initial underpriced candidate records its own attempted fee caps,
even when no earlier local hint exists. The next fresh request at that nonce increases both fields by
at least 12.5%; profitability quoting includes the floor, and request/global ceilings still apply.
This progressively discovers a usable bid without retrieving foreign private transaction fees.
The manager does not track the competing transaction and does not
re-sign business calldata at another nonce. An `already known` response and transport ambiguity retain
normal ownership and receipt tracking: they may describe an accepted transaction.

The next request reads mined state again. A failed nonce read produces a submission error before
signing and does not create a persistent account pause. `Available` reports initial RPC readiness;
`Idle` and `LaneReady` continue to reflect local admission demand. No idle account recovery monitor runs.

### Owned replacements and uncertain execution

Every replacement path checks the sending endpoint's latest mined nonce with its own bounded RPC,
including exact rebroadcasts. If it has advanced beyond the owned nonce, no more
replacement bytes are signed/broadcast. A nonce RPC error defers replacement; pending advancement alone
cannot suppress it because this process's own unmined submission can advance pending.

Owned receipts retain priority and ordinary validation, canonical ancestry and confirmation depth.
Only a complete sweep where every known hash returned `NotFound` permits `OutcomeNonceConsumed` from a
higher mined nonce. RPC errors, malformed receipts or an unresolved owned receipt withhold that path.
The result wraps `ErrNonceConsumed`, retains the original attempted hash, and has no receipt. It does
not claim inclusion, cancellation, failed execution or a winning peer; the account read may race mining,
receipt publication or a reorg. Neither uncertain nonce outcome authorizes automatic calldata replay.
Each solver queries authoritative business state and rebuilds any retry under its existing bounds.

The fee hint concerns only this process's abandoned lifecycle or underpriced initial attempt. There is no unknown-nonce watchdog,
startup gap-clearing transaction or peer ownership lookup. It contains a nonce and fee floor, not old
calldata or an instruction to retry an order. If different relays accepted competing transactions at the
same nonce, a fresh replacement may still compete with another replica's candidate; the account count
provides no exclusive ownership.

### Restart and fee limits

Signed attempts and fee hints exist only in memory. Restart loses old hashes, calldata, fees and
deadlines, but still selects the lowest unconsumed nonce from mined state. A fresh eligible business call
can replace unknown pending work there; underpriced responses rebuild a fee floor over subsequent
fresh requests. No later nonce is newly queued behind that unknown call. This policy deliberately
trades pending-transaction pipelining for recovery without durable state or replica coordination.

Replacement remains conditional on eligible fresh work, sender funds, current mined state and eventual
inclusion. A foreign transaction's unknown fee floor may exceed the new request's profitability ceiling
or the global ceiling; a particular solver order can also exhaust its retry/deadline budget before
reaching that floor. Replica contention can replace still-valid work and increase fees. No ordering,
fairness or recovery of old receipts is promised. For controlled maintenance,
reconcile outstanding write-route submissions before reusing the EOA. Confirmation of owned receipts does not make later deep reorgs impossible.

## 7. Shutdown

Accepted tracking uses a manager lifecycle context detached from caller/intake cancellation. Solvers stop
new commitments and finish their protocol preparation while the shared manager remains available to
accepted work. Manager shutdown closes admission and drains ordinary receipt tracking and fee
replacements for at most `shutdownTimeoutMs`, then cancels lifecycle RPCs and delivers a
`tracking_stopped` result with the shutdown-deadline error so process teardown can proceed.
It sends no shutdown transaction. This does not guarantee mining or
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
`nonce_conflict` and `nonce_consumed` are expected results with unknown business execution. Each records a terminal request
and lifecycle outcome without synthesizing receipt gas or paid-fee accounting.

### Metrics

Names include the `solver_bot_` prefix; deployment identity is supplied by scrape target labels.
Definitions: [metrics.go](../internal/txmanager/metrics.go),
[account_metrics.go](../internal/txmanager/account_metrics.go).

| Component | Metric | Labels | Meaning |
|---|---|---|---|
| Txmanager | `solver_bot_txmanager_requests_total` | `label`, `outcome` | Terminal results of logical on-chain operations, including the uncertain nonce outcomes. This is the request funnel for every solver, not proof of mined execution. |
| Txmanager | `solver_bot_txmanager_inflight` | `label` | Requests accepted by the txmanager worker and still awaiting a terminal result; sustained values expose stuck transactions or nonce congestion. |
| Txmanager | `solver_bot_txmanager_gas_used_total` | `label`, `outcome` | Receipt gas for mined transactions, including reverts. Divide by the matching request count for average gas; this is gas units, not native-token cost. |
| Txmanager | `solver_bot_txmanager_fee_paid_wei_total` | `label`, `outcome` | Actual native-token fee paid by mined transactions, calculated from receipt `gasUsed × effectiveGasPrice`, including reverted transactions. |
| Txmanager | `solver_bot_txmanager_replacements_total` | `label`, `kind`, `reason` | Successfully broadcast replacements and exact rebroadcasts (`kind` = `replacement`, `rebroadcast`). Replacement `reason` is `validity`, `congestion`, `stall`, `gas` or `fallback`; a rebroadcast is `stall`, `uncertain` (ambiguous first broadcast) or `capped` (fee cap reached). Spikes expose fee-policy, relay or congestion problems that terminal outcomes alone cannot show. |
| Txmanager | `solver_bot_txmanager_admission_rejections_total` | `label`, `reason` | Requests rejected before the signed worker lifecycle. Reasons are `manager_stopped`, `deadline_exceeded`, `caller_cancelled`, or bounded fallback `other`; an expected busy `TrySend` probe is excluded. |
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

Passive late-receipt observation, its per-solver telemetry hooks and dashboard panels are deferred to
a separate follow-up PR. The core nonce change keeps ordinary owned receipt tracking; executions after
local abandonment can still be absent from local success metrics until that follow-up is adopted.

Receipt tests cover independent RPC budgets, timer handling during blocked reads, new/old hashes,
confirmation-time reorg recovery and teardown. Unit tests verify abandonment without a self-transfer,
fresh business nonce reuse despite a pending count above it, mined-nonce advancement, retained hints on
RPC/preparation/submission failures, replacement fee floors, request/global ceilings and bounded shutdown.
Solver tests verify current business-state reconciliation, fresh retries and obsolete-result precedence.
The local Anvil target verifies real fee replacement after a base-fee spike, abandonment followed by a
different business call at the same nonce, restarted recovery without old hints through underpriced
fresh requests, and three independent managers executing distinct orders as mined nonces advance.
These public-pool tests do not establish private-provider retention or consistency guarantees.

Fee tests cover the base-fee bound, tip rule, repricing decisions, fee-window parsing, next-block estimate
and fallbacks, stale send heads, stall rebroadcasts, congestion, validity and gas growth. A stalled gas
estimate remains bounded and cannot cause a broadcast after the request tracking deadline.
Run repository-required build, race/coverage and lint gates for implementation changes. Local and hosted
checks do not establish deployment or production rollout status.

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
