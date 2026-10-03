# Independent processes sharing a transaction sender

The transaction manager reads the latest mined account nonce for every fresh send. Three independent
processes can keep the same EOA without a database, shared file, leader election, replica identity or peer communication.
There is no feature flag. Operator setup lives in the
[README](../README.md#independent-processes-with-the-same-eoa); the
[transaction manager plan](TXMANAGER-PLAN.md) defines normal receipt, fee, confirmation and shutdown behavior.

## 1. Nonce selection and local ownership

Each process has one signer and one manager shared by its local transaction-sending solvers. The local
slot admits one actively tracked signed lifecycle at a time. `Initialize` checks a bounded mined nonce read;
foreign pending transactions do not delay startup. Before preparing every new request, the manager reads
`eth_getTransactionCount(address, "latest")` from the sending endpoint again. Fresh work always uses the
lowest unconsumed nonce; pending counts never advance initial signing. This deliberately gives up
queuing higher nonces before lower ones are mined, so an expired private call cannot create a new gap
after every process restarts. The chosen nonce is captured before estimation and the final protocol
status check, then kept through signing. A canonical owned receipt at the requested confirmation depth
sets a process-local floor to its nonce plus one, including reverts. Selection uses the maximum of this
floor and the current mined nonce; unconfirmed/orphaned receipts and sibling outcomes do not advance it.
There is no unchecked cached increment or extra account-confirmation gate before signing.
A process-local fee hint applies only when its nonce equals the current mined nonce and its
`pendingTimeoutMs` lifetime has not elapsed. Mined advancement or a lower nonce after a reorg discards
an inapplicable hint; unavailable state fails the next request before signing and retains an unexpired
hint. A fresh request or quote discards a hint whose required bump cannot fit the global fee ceiling,
then uses its already validated fresh market fees. A request-only ceiling skips the hint for that
send without raising or renewing it; later affordable requests retain the known replacement floor
until its original expiry. This does not alter active owned replacement caps.

The RPC read and broadcast are separate operations. Two replicas can choose the same nonce. An initial
nonce-too-low or replacement-underpriced response returns `nonce_conflict` with `ErrNonceConflict`, the
attempted hash, and no receipt. Rejected initial fees do not create, ratchet or renew an owned hint.
Fresh requests re-read market fees after the nonce cooldown; an applicable accepted/uncertain hint
raises both fields by at least 12.5%. Failed preparation retains an unexpired applicable hint; ceilings
are never raised to repair the nonce.
The worker releases its local slot immediately. The next request reads
mined state again. It neither cancels the competing transaction nor silently retries business calldata at a
new nonce. `already known` and transport uncertainty continue normal tracking, because acceptance remains
possible. A mined nonce RPC failure prevents signing that request; it does not permanently pause later work.

Any underpriced response starts a local nonce cooldown of one configured `horizon.blockTimeMs`. Every
fresh request at that nonce waits, including a different queued order, so another order cannot bypass
the delay and keep bidding. The worker polls current mined state at the manager polling cadence and
resumes if the nonce changes; request deadlines and the manager context bound the wait. Expiry permits
one fresh attempt even if no transaction mined, so a lost private transaction cannot stall indefinitely.
The cooldown requires no peer discovery, storage or extra configuration and resets on restart.

Accepted or uncertain broadcasts retain exact signed variants and ordinary same-nonce fee replacement.
Each replacement first checks latest mined state. If it has advanced, broadcasting stops.
An underpriced owned replacement or exact rebroadcast yields immediately as `abandoned` at Info for
protocol reconciliation. Retain every signed hash and the previous accepted or transport-uncertain fee
hint; a rejected replacement never ratchets fees toward the cap. It starts the same nonce cooldown for
fresh work. Transport-ambiguous failures retain their existing tracking and Error reporting.
Owned receipts keep priority, canonicality validation and the configured confirmation depth. After every
owned hash returned `NotFound`, a higher mined nonce can end tracking with `nonce_consumed` and
`ErrNonceConsumed`, the original attempted hash, and no receipt. Account reads and receipt publication
are not atomic, so even an actual winner can receive this unknown-execution outcome.

`abandoned` is also an unknown execution result: the request deadline, pending timeout or `Obsolete`
ends tracking, releases the local lane and records a nonce/fee hint for the next fresh business call.
No cancellation transaction is sent. Replacement requires at least a 12.5% increase in both fee fields
under the fresh request and global ceilings while the hint is unexpired and can fit. Otherwise
current capped market fees are used: expiry or a global-cap mismatch discards the hint, while a
request-only mismatch retains it without renewing its lifetime. Failed preparation/submission retains an
applicable hint without extending its lifetime. Without a fresh eligible request, the old call can remain pending and still land if valid.

None of these outcomes proves a successful fill, a failure or which peer won. The generic
layer does not know business orders. It never fabricates receipts, gas/paid-fee accounting or a winning
hash; solvers own current business-state reconciliation and any freshly built retry.

## 2. Provider and restart limits

The configured sending endpoint must serve current mined account state. Pending visibility across
replicas is not required for nonce selection. A stale mined view can still cause collisions. Nonce counts cannot
identify another transaction's calldata, price, age or ownership, and cannot reliably expose private
hidden work or queued transactions beyond a gap. Read endpoints still provide ordinary receipt and
canonical header checks; no EIP-1898 account-state extension is required.

There is no unknown-nonce watchdog, startup gap-clearing transaction or EOA cancellation. Only an
abandoned accepted/transport-uncertain local lifecycle supplies the temporary fee hint; processes do not
recover peers' business calldata.
If distinct relays accepted competing candidates at the same nonce, fresh replacements can still
compete: nonce counts do not establish exclusive ownership.

Signed attempts and fee hints are in memory. Restart forgets their hashes and fees, but fresh business
work still targets the lowest unconsumed nonce using fresh market fees. Rejected bids do not discover
the unknown fee floor. A fresh call can execute when market fees meet the relay's replacement policy,
the old call is dropped, or the account nonce advances. Different replicas may replace still-valid
business calls and increase each other's fees. Recovery requires eligible work, sender funds, current
mined state and eventual inclusion. A foreign fee floor may exceed a fresh order's profitability budget
or the global ceiling, and an individual order's retry/deadline policy may stop before that floor is reached.
The design does not promise ordering, fairness, recovery of old hashes/receipts, or protection
against reorgs beyond the chosen receipt confirmation depth. Controlled maintenance reconciles outstanding
private write-route submissions before reusing the EOA.

## 3. Solver reconciliation

Nonce liveness does not make quote caches, liquidity reservations, offer budgets or exclusive obligations
global. All remain process-local. Protocol order/bid nonces are separate from the EOA transaction nonce.

| Integration | Response to `abandoned`, `nonce_conflict` or `nonce_consumed` |
| --- | --- |
| [RFQ](RFQ-PLAN.md) | Query backend order status. A terminal status retires the item; a fresh open-order poll can rebuild its plan/signatures after the poll delay within its deadline. Initial nonce conflicts do not spend the business retry budget; accepted uncertain outcomes use `maxNonceRetries`. Missing/unknown/error status stays bounded reconciliation work. |
| [3F](3F-PLAN.md) | The next normal redemption poll reads `canWithdraw` and builds a fresh batch, excluding finalized requests. Offers remain independently managed. |
| [UniswapX](UNISWAPX-PLAN.md) | Release local capacity and invalidate inventory; defer to fresh open-order polling without counting a fill success or execution/breaker failure. |
| [LI.FI](LIFI-PLAN.md) | Release local capacity and immediately re-read on-chain order status. Claimed/refunded orders retire; eligible orders and failed status reads enter the replay-coalescing timer queue. Every retry rebuilds from fresh state, with preserved backoff and the admission-time mapped order deadline, without requiring redelivery or reconnect. |
| [OEV](OEV-PLAN.md) | Settlement is submitted externally and does not use this manager. |

RFQ also reconciles typed estimate execution reverts and mined reverts before assigning business
failure. Unknown or malformed estimate reverts have a separate unsigned retry count bounded by
`maxNonceRetries`, with polling/deadline bounds; exhaustion fails with Error once. Known Reactor or
Executor setup/transfer errors fail with Error once after backend reconciliation, and `ExpiredRequest`
expires the order. Stale open listings cannot re-arm these permanent unsigned failures. Missing backend
views wait for reconciliation without another estimate until the deadline. Mined reverts use the signed
retry budget when fresh backend state remains open. Exact Reactor `NonceUsed()` data
retires sending immediately and releases capacity; bounded backend observation distinguishes a filled
order from explicit invalidation. Only a valid backend fill hash absent from every local signed attempt
is credited once as `fill/peer`, with no local success amounts. Known backend completion wins over local
expiry on the final reconciliation poll.

## 4. Validation and observability

`abandoned`, `nonce_conflict` and `nonce_consumed` are expected request/lifecycle outcomes, with attempted hashes and
no synthetic receipts. Solver completion spans record an expected decline; subsequent business retries
use fresh protocol state. Real RPC failures retain Error reporting. Generic on-chain reverts are Info
outcomes whose owning integration chooses severity after business reconciliation; expected RFQ races
and peer fills do not page or become failed-fill metrics.

Unit tests cover fresh mined reads with foreign pending work, immediate local release after nonce collisions,
bounded/recoverable RPC failures, receipt priority, mined-nonce replacement suppression and solver retry
accounting, rejected-fee non-escalation, hint expiry/global-cap recovery, low-cap rejection followed
by affordable replacement, renewed snapshot safety and
request/global ceilings. An Anvil race test synchronizes three independent same-key managers at nonce 0, mines a winner, and rebuilds the remaining eligible orders
at nonces 1 and 2 after mined state advances. It verifies all three distinct transactions execute canonically.
Another scenario abandons a pending business call, verifies no
self-transfer was sent, and mines a fresh different business call replacing its unused nonce. A manager
recreated with no hints preserves fresh market fees across underpriced responses and executes at
nonce 0 once unknown pending work is dropped.
These public-pool tests do not
establish retention or consistency guarantees for a private submission provider.

Central rate alerts under [`alerts/`](../alerts/README.md) join each pod's counters to the shared
sender identity before aggregating by namespace and EOA. Sustained nonce consumption without peer
fills, expirations after exhausted nonce retries and repeated fee-ceiling decisions are separate
signals. Thresholds, windows and pending duration belong to the rule YAML, not the process. Local
exhaustion stops resubmission but keeps backend reconciliation until terminal state or the order
deadline; only an evidenced later expiry advances the expiry counter. Expected single races and
cap decisions stay Info, while real transport faults retain Error reporting. Rule provisioning and
notification routing remain the monitoring operator's responsibility.
