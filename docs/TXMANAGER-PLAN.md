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
| `GasLimit` | Zero requests exact-call estimation after admission and before signing, with `gas.headroomBps` headroom (5% by default). A supplied limit is reused for normal replacements. |
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

The fee and gas strategy adds four nested blocks under `txManager`. Unlike the flat fields above, they start
from their defaults before decoding, so an omitted key keeps its default while an explicit `0` or `false` is
honoured and validated. The zero value of `txmanager.Config` carries the same defaults (`Config.WithDefaults`,
applied by `New`); a `cmd/vault-solver` test pins the two sets together. `run.go` passes every knob explicitly,
and the basis-point and head-lag fields are pointers in `txmanager.Config` because zero is a valid setting there.

| Setting | Default | Meaning |
|---|---:|---|
| `fees.policy` | `legacy` | `legacy` keeps 2×base + tip with timer bumps; `horizon` prices an exact EIP-1559 validity horizon and reprices from block evidence. |
| `fees.blockTimeMs` | 12000 | Slot time for head lag, the next-block estimate and evaluation cadence. |
| `fees.minHorizonBlocks` | 2 | Fewest blocks, from the real next block, an attempt must stay valid for; below it the send is refused. |
| `fees.maxHorizonBlocks` | 6 | Horizon a horizon-policy send targets when the balance allows (at most 12). |
| `fees.pricingHorizonBlocks` | 5 | Quote-pricing and funding-gate horizon; at least `minHorizonBlocks + 2`, at most `maxHorizonBlocks`. |
| `fees.maxHeadLagBlocks` | 2 | Snapshot lag tolerated before a send waits for a newer head (`0` is strict). |
| `fees.tipFloorGwei` | 0.02 | Tip in blocks with room, and the tip the refusal floor assumes; at least one wei (0.000000001). |
| `fees.singleFullBlockTipGwei` | 0.1 | Tip when one of the last two blocks had no room for the gas limit. |
| `fees.congestedTipFloorGwei` / `congestedTipCapGwei` | 0.2 / 15 | Clamp of the demand-run tip; the ladder from `tipFloorGwei` up must be non-decreasing. |
| `fees.congestedRewardBlocks` / `congestedRewardPercentile` | 3 / 50 | A demand run follows the maximum of this reward percentile over this many latest blocks. |
| `fees.escalateAfterFullMisses` | 2 | Consecutive missed blocks without room before a congestion reprice. |
| `fees.stallAfterRoomyMisses` | 3 | Missed blocks with room before the stall response (re-estimate, rebroadcast). |
| `gas.headroomBps` | 500 | Gas-limit headroom over the estimate (0–5000), rounded down; 500 signs the historical estimate + estimate/20. |
| `gas.nextBlockEstimate` | true | Under horizon, estimate in next-block context; rejected or ignored overrides fall back. |
| `gas.fallbackHeadroomBps` | 1000 | Headroom over a plain fallback estimate; at least `headroomBps`, at most 10000. |
| `gas.estimateTimeoutMs` | 5000 | Bound on one gas estimate, separate from the fee-read budget. |
| `balance.guard` | true | Cap every attempt at what the balance funds; refuse what cannot stay valid for `minHorizonBlocks`. Both policies. |
| `balance.referenceGasUnits` | 0 | Fill gas limit quote pricing, the funding gate and the shadow evaluator assume; 0 turns the gate off. A UniswapX or LI.FI lane with `gas:` accounting requires it above 0. |
| `balance.fundingHysteresisBps` | 2000 | Extra balance over the gate threshold needed to become fundable again (0–10000). |
| `balance.targetEth` | 0 | Operator funding target, exported for alerts only; 0 leaves it unset. |
| `shadow.enabled` | true | Score virtual fills under both policies on every head, metrics only. |

Validation also requires `tipGwei: 0` under `horizon`, whose tip comes from the ladder. At startup
`ValidateFeeHeadroom` additionally rejects a `fees.tipFloorGwei` (both policies) or, under `horizon`, a
`fees.congestedTipCapGwei` at or above the initial cap `reserveFeeBump(normalFeeLimit)` (39.5 gwei at
`maxFeeGwei: 50`); the legacy policy never signs the ladder, so a low `maxFeeGwei` stays valid there.
It also rejects a `fees.tipFloorGwei` below one wei, checked in wei as it is signed: a positive gwei value
that truncates to a 0-wei tip would pass a gwei comparison, and every rung above it is at least the floor.
The strategy's rule that `referenceGasUnits > 0` wherever gas pricing is used is enforced by the solvers
that price gas, not here, since only they know it: the UniswapX and LI.FI factories refuse a `gas:` block
while the generic `Manager.ReferenceGasUnits()` is 0, because quote pricing would then size the tip for no
gas (every block roomy) and the funding gate would stay off. Deploy that key together with the image.
Bounds beyond the strategy's list (fallback headroom and hysteresis at 100%, the reward percentile in
(0, 100], at most 1024 reward blocks as eth_feeHistory serves) only reject nonsensical values.

Polling defaults to 2 seconds in the Go manager; it is not a separate YAML field. Pending receipt reads,
replacement nonce reads and obsolescence checks each use `min(2 seconds, replacementInterval/2)`; fee reads use
`min(1 second, replacementInterval/2)`. These internal read budgets are separate from broadcast timeout.
Account refresh uses a 5-second context. Backends must honor cancellation.
With the balance guard on (§4.1), a new send waits out a stale fee snapshot for at most two
`fees.blockTimeMs` (bounded by `CancelAt`), polling every `min(pollInterval, blockTime/4)`, and retries a
pinned balance read whose block the node does not have yet within the fee-read budget. The gas estimate
runs concurrently with those reads.

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

### 4.1 Balance guard

A relay accepts a transaction whose `gasLimit × maxFee + value` exceeds the sender balance without an
error and never lands it; each timer bump then makes it harder to fund, and only the 21000-gas cancel
lands. That is what failed on 2026-09-28. The guard (`balance.guard`, on by default under both fee
policies) never signs such an attempt. It needs the optional `ReadBalanceAtBlock` backend capability,
which `chain.Client` provides; a backend without it (unit-test mocks) runs with the guard off and one
startup log line says so. `balance.guard: false` turns it off without that line.

**Initial send order** (`broadcast`): a fee snapshot and the balance pinned to its head, overlapped with
the gas estimate; the guard; `Obsolete`; the nonce; sign and send. A refusal therefore signs nothing and
leaves the nonce free. A gas estimate that fails on its own ends the snapshot and balance reads, so the
send fails at once with the estimate's error (a real submission error, not `NotAdmitted`) instead of
waiting out a stale head and reporting a refusal a solver would retry.

1. **Snapshot.** The legacy fee read (latest header, plus `FeeHistory(5, latest, [25])` when `tipGwei`
   is 0). The next block's base fee `pb` is the one the fee history reports for the block after its
   newest (exact, computed by the node); with a positive `tipGwei` there is no history and `pb` is the
   protocol maximum after the header, `grow(base, 1)`. The snapshot's lag is the whole block times
   elapsed since the header's timestamp, plus one when the fee history is a block behind the header.
   It is stale when the history is more than one block from the header, when the lag exceeds
   `fees.maxHeadLagBlocks`, or when its block is below the block that included the previous lifecycle's
   attempt (a lagging upstream would otherwise report the balance from before that fill was paid).
   A stale snapshot is re-read for up to two block times within `CancelAt`, then refused (`stale_head`).
   The lag is wall-clock time since the header, so a chain that does not produce a block every
   `fees.blockTimeMs` (automine or idle anvil, an anvil mainnet fork, an idle devnet) has every send
   refused as `stale_head` once its head is more than `(fees.maxHeadLagBlocks + 1)` block times old: run
   such a chain with a matching block time (`anvil --block-time 12`, or lower `fees.blockTimeMs`), or set
   `balance.guard: false` there.
2. **Balance.** `eth_getBalance` pinned to the snapshot's block: by hash with `requireCanonical` when
   the history's newest block is the header, otherwise by that block's number. A block the node does not
   have is retried within the fee-read budget; any failure refuses (`stale_head`). It never falls back
   to `latest` or to the telemetry snapshot. The hash is the one go-ethereum computes from the header
   fields it knows (ethclient drops the node's own), so a header field that the go-ethereum version in
   `go.mod` does not hash would make every hash pin miss; three hash-pinned reads in a row that end not
   found are logged at Error once per streak, and at Info when a hash-pinned read succeeds again.
3. **Guard.** `aff = floor((balance − value) / gasLimit)`, and the floor
   `lo = fee(minHorizonBlocks + lag, floorTip)` where `fee(H, tip) = grow(pb, H−1) + tip`, `grow` is the
   exact EIP-1559 maximum (`x += max(x/8, 1)` per block, go-ethereum's denominator, deliberately not a
   knob), and `floorTip` is `fees.tipFloorGwei` or a larger mandatory `tipGwei`. The guard only acts when
   the balance binds (`aff` below the policy's fee cap): below `lo` it refuses with `ErrUnaffordable`
   (reason `unaffordable_one_block` when `aff ≥ pb + floorTip`, else `unaffordable`); otherwise it signs
   `maxFee = aff` with the tip clamped to `aff − pb`. A funded lane signs exactly what the policy priced;
   a request cap or policy fee already below the floor is left to the policy, so legacy behaviour is
   unchanged except where the balance binds.

Refusing below `fee(2 + lag)` gives up attempts the balance could keep valid for exactly one block if
the base fee rises; they are counted apart (`unaffordable_one_block`), and if they appear on a funded
lane `fees.minHorizonBlocks: 1` is the knob to revisit. A one-block send from a stale head is the silent
drop the guard exists to prevent.

`ErrUnaffordable` and `ErrStaleHead` are exported. The worker maps both to
`Result{Outcome: submission_error, NotAdmitted: true}`, counts them in `admission_rejections_total` by
reason, logs the refusal at Info (with the head, next base fee and lag), and records a `declined` event
instead of an error status on the broadcast and send spans. Solvers treat them as expected skips.

**Replacements and cancellations** are capped the same way. Before a new same-nonce attempt is priced,
the balance is read at the current head, pinned by hash, and the attempt's fee limit becomes
`min(limit, floor((balance − value) / gasLimit))`, with the cancellation's 21000 gas and zero value.
When that cannot fund the required 12.5% bump, or lies below the latest base fee, or (for a fill
replacement) the balance cannot be read, nothing new is signed and the existing capped exact rebroadcast
of the latest applicable attempt runs; it becomes valid again if the base fee recedes. A "replacement
capped" Info line is logged once per lifecycle, with the reason `balance`, `reserved_balance` or
`balance_unavailable`.

A cancellation never waits for the balance read: when it fails (a read error, a block the node does not
have within the fee-read budget, or a head below the previous inclusion), the cancellation is capped at
the balance the lifecycle already reserved, `floor(max(gasLimit × maxFee + value) / 21000)` over its
signed attempts. Each of them was checked against the balance when it was signed and their shared nonce
is not mined, so that balance still holds; a fill of at least 23,625 gas therefore always funds the first
cancellation's bump, and the deadline cancel still fires during a read outage. The fallback is logged at
Info once per lifecycle. It assumes the signer is not spent from by anything else, as nonce serialization
already does.

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

Three read-client primitives serve the fee and gas strategy; all are metered and traced like the
promoted reads. `ReadBalanceAtBlock` reads the signer balance pinned to one block, by hash as an
EIP-1898 object or by explicit number, and refuses tags such as `latest`; a node without the pinned
block answers with an error wrapping `ethereum.NotFound` (geth, erigon, nethermind and anvil wordings,
and EIP-1474 `-32001`), which the caller retries from a fresh head instead of reading another head. geth's
"hash is not currently canonical", the answer to a hash read with `requireCanonical` after a reorg replaced
that block, is the same stale pin and wraps `ethereum.NotFound` too. The balance guard pins a hash with
`requireCanonical: true`: without it a node may serve the reorged-out block's state, such as the balance
from before the previous fill was paid. `EstimateGasWithBlockOverrides`
sends the four-parameter `eth_estimateGas(call, parent, null, {number, time})`; `IsBlockOverridesUnsupported`
recognises an upstream that rejects the fourth parameter (`-32602`, too many or invalid arguments, also in a
non-2xx body) but never a revert or a missing parent block. `ProbeBlockOverrides` catches an upstream that
silently ignores it: it estimates, under a state override, code that stops only when `NUMBER` and `TIMESTAMP`
equal the `NextBlockOverrides` of the latest head and reverts otherwise. Checking the timestamp as well as the
number (the strategy's probe checks only the number) also catches an upstream that honours the number but not
the time, which matters because interest accrual in the filled vaults depends on it; and success counts only
when the estimate exceeds the 21000 intrinsic gas, since an upstream that drops the state override calls an
empty account and succeeds. A probe error (head unavailable, parent block missing) is inconclusive rather
than a verdict. `make test-chain-anvil` runs all three against a real EVM.

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

The Info `sent` line (and its "already known" and "uncertain" variants) carries `gasLimit`, `estimateMode`
(`latest` or `supplied`), `tip`, `maxFee` and `requiredBalance` (`gasLimit × maxFee + value`); under the
balance guard it adds `nextBaseFee`, `horizonBlocks` (blocks from the next one the fee cap stays valid at
the floor tip), `balance`, `balanceBound` and `headLagBlocks`. It is Info because RFQ and LI.FI run without
`--debug`. The "started" line names the sender address, the fee policy and whether the guard runs.
Guard refusals are declined, not failed, on the `txmanager.broadcast` and send spans
(`decision=not_admitted`, `reason` as in `admission_rejections_total`).

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
| Txmanager | `solver_bot_txmanager_admission_rejections_total` | `label`, `reason` | Requests rejected before signing. Before the worker lifecycle: `manager_stopped`, `nonce_conflict`, `deadline_exceeded`, `caller_cancelled`, or bounded fallback `other`; an expected busy `TrySend` probe is excluded. After worker admission, by the balance guard (§4.1): `unaffordable`, `unaffordable_one_block`, `stale_head`, and `fee_ceiling` (reserved for `Request.MaxFeeWei`, §10). These four start at zero for every label that reaches the worker, so later refusals are visible to `increase()`; a refusal of a label's very first lifecycle after a restart creates the series at 1, which `increase()` and `rate()` miss (the Info refusal log still records it). They are not also counted in `admission_wait_duration_seconds`, which already recorded the admission. |
| Txmanager | `solver_bot_txmanager_admission_wait_duration_seconds` | `label`, `outcome` | Time from a real send request until worker admission or a terminal pre-admission outcome. `outcome` is `admitted` or one of the bounded rejection reasons; expected busy `TrySend` probes are excluded. |
| Txmanager | `solver_bot_txmanager_lifecycle_duration_seconds` | `label`, `outcome` | Time from worker admission through broadcast and terminal tracking. It excludes pre-admission nonce-lane wait, so use admission rejections alongside its latency distribution. |
| Txmanager | `solver_bot_txmanager_phase_duration_seconds` | `label`, `phase`, `outcome` | Time spent in each reached worker phase: `prebroadcast`, `pending`, or `confirming`. Reorgs may return a lifecycle to `pending`; the emitted sample contains the cumulative time spent in that phase. |
| Txmanager | `solver_bot_txmanager_first_attempt_total` | `label`, `outcome`, `simulation` | Lifecycles that reached the chain: `first` (the first signed attempt landed within 3 blocks of its send head, exact rebroadcasts allowed), `late` (it landed later), `replaced` (a replacement or cancellation was signed before the call landed) or `cancelled` (the cancellation landed). `simulation` is `unknown` until next-block simulation classifies non-first lifecycles (§10). |
| Txmanager | `solver_bot_txmanager_inclusion_delay_blocks` | `label` | Blocks from the head the first attempt was signed at to the block that included the call (1 is the next block); cancellations are excluded. |
| Txmanager | `solver_bot_txmanager_pending_age_seconds` | `label`, `kind` | Age of the unresolved lifecycle's call (`fill`, since its first send) or cancellation (`cancellation`, since it began), refreshed on receipt polls and removed at inclusion or the end of the lifecycle. |
| Txmanager | `solver_bot_txmanager_attempt_tip_wei`, `_attempt_max_fee_wei`, `_attempt_gas_limit` | `label`, `kind` | Priority fee, fee cap and gas limit of the latest signed attempt, `fill` (initial send and replacements) or `cancellation`. |
| Txmanager | `solver_bot_txmanager_attempt_horizon_blocks` | `label` | Histogram of the blocks, from the next one, an initial attempt's fee cap stays valid at the floor tip when the guard priced it; below 3 with `balanceBound` in the `sent` log means the balance, not the policy, set it. |
| Txmanager | `solver_bot_txmanager_fee_next_base_fee_wei` | — | Next block's base fee from the latest guarded fee snapshot. |
| Txmanager | `solver_bot_txmanager_account_required_balance_wei` | `horizon` | Balance a reference fill (`balance.referenceGasUnits`, or the latest attempt's gas limit while that is 0) needs at that base fee and the floor tip: `min` (`fees.minHorizonBlocks`, can still send), `quote` (`fees.pricingHorizonBlocks`) and `full` (`fees.maxHorizonBlocks`). |
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

### Fee and gas strategy TODO

Deferred from the fee and gas strategy change (the balance guard, funding gate, `Obsolete` wiring, horizon
policy, next-block estimate and shadow evaluator ship in it):

- **Simulation classification (strategy PR5).** A time-boxed next-block `eth_call` when a stall triggers and
  before any cancel, filling `first_attempt_total{simulation}` (kept at `"unknown"` until then) and
  `pending_simulation_total`; `simulation.cancelOnRevert` only after 4 weeks of shadow data without a false revert.
- **`Request.MaxFeeWei`.** An optional per-request ceiling on the worst-case total fee (`floor(MaxFeeWei / G)` per
  gas, refused below the floor as `fee_ceiling`), and its solver wiring: UniswapX and LI.FI from their Chainlink
  native price, RFQ from its discount margin converted to native units.
- **Flip the code default to `horizon`** once horizon shadow first@3 is at least legacy's and 99.5% over 7 days
  including a base fee above 3 gwei, and the pooled real lifecycles pass the success gate.
- **Deploy charts** (vault-solver-deploy): add the new keys only after an image that knows them is live
  everywhere, because config decoding uses `KnownFields(true)`; never run an older image against newer keys.
  The exception is a UniswapX or LI.FI chart with `gas:` enabled: the new image refuses it without
  `balance.referenceGasUnits`, so that key ships in the same deploy as the image.
- **Guard gauges between sends.** `fee_next_base_fee_wei` and `account_required_balance_wei` are updated
  by guarded sends only; the funding gate's per-poll fee snapshot should refresh them on every account poll.
- **Glamsterdam (ePBS).** Before the mainnet fork, re-measure the fill gas profile, `referenceGasUnits`,
  `gas.headroomBps`, `fees.blockTimeMs` and the room/tip thresholds if the fork changes gas costs, the block gas
  limit or slot timing; revalidate on Sepolia first. The balance guard pins reads to a header hash that the
  go-ethereum version in go.mod computes (§4.1): bump it to a release that knows the fork's header fields before
  the fork activates on a chain the bot runs on, or every guarded send there is refused as `stale_head`.

Keep shared lifecycle design and metric contracts here. Update integration plans only when their own
request construction, readiness, capacity or protocol behavior changes. Preserve operator-facing setup,
EOA exclusivity and maintenance warnings in README, with links here for the detailed mechanism.

### Receipt failure diagnostics

Existing sweep-level error suppression also carries distinct checked/all tracked hashes, completed
RPC count (including priority reads), elapsed sweep time and last RPC duration. `rpcBudgetTotalMs`
sums the effective per-call budgets of completed reads; there is no shared sweep deadline. The first
failed read supplies the logged hash, stable `reason_code` and `cancelCause`, even when later reads
succeed. Lifecycle shutdown retains its existing exit without an extra partial-sweep error log.
