# Independent processes sharing a transaction sender

The transaction manager reads the latest mined account nonce for every fresh send. Three independent
processes can keep the same EOA without a database, shared file, leader election, replica identity or peer communication.
There is no feature flag. Operator setup lives in the
[README](../README.md#independent-processes-with-the-same-eoa); the
[transaction manager plan](TXMANAGER-PLAN.md) defines normal receipt, fee, confirmation and shutdown behavior.

## 1. Nonce selection and local ownership

Each process has one signer and one manager shared by its local transaction-sending solvers. The local
slot admits one actively tracked signed lifecycle at a time. `Initialize` checks a bounded mined nonce read;
foreign pending transactions do not delay startup. Before every new signing, the manager reads
`eth_getTransactionCount(address, "latest")` from the sending endpoint again. Fresh work always uses the
lowest unconsumed nonce; pending counts never advance initial signing. This deliberately gives up
queuing higher nonces before lower ones are mined, so an expired private call cannot create a new gap
after every process restarts. There is no cached increment or account-confirmation gate.
A process-local fee hint applies only when its nonce equals the current mined nonce. Mined advancement
or a lower nonce after a reorg discards an inapplicable hint; unavailable state fails the next request
before signing and retains it.

The RPC read and broadcast are separate operations. Two replicas can choose the same nonce. An initial
nonce-too-low or replacement-underpriced response returns `nonce_conflict` with `ErrNonceConflict`, the
attempted hash, and no receipt. An underpriced candidate records its own attempted fees. The next fresh
request at that nonce raises both fee fields by at least 12.5%, and its profitability quote includes the
required floor. Failed preparation retains the hint; ceilings are never raised to repair the nonce.
The worker releases its local slot immediately. The next request reads
mined state again. It neither cancels the competing transaction nor silently retries business calldata at a
new nonce. `already known` and transport uncertainty continue normal tracking, because acceptance remains
possible. A mined nonce RPC failure prevents signing that request; it does not permanently pause later work.

Accepted or uncertain broadcasts retain exact signed variants and ordinary same-nonce fee replacement.
Each replacement first checks latest mined state. If it has advanced, broadcasting stops.
Owned receipts keep priority, canonicality validation and the configured confirmation depth. After every
owned hash returned `NotFound`, a higher mined nonce can end tracking with `nonce_consumed` and
`ErrNonceConsumed`, the original attempted hash, and no receipt. Account reads and receipt publication
are not atomic, so even an actual winner can receive this unknown-execution outcome.

`abandoned` is also an unknown execution result: the request deadline, pending timeout or `Obsolete`
ends tracking, releases the local lane and records a nonce/fee hint for the next fresh business call.
No cancellation transaction is sent. Replacement requires at least a 12.5% increase in both fee fields
under the fresh request and global ceilings; failed preparation/submission retains the hint. Without a
fresh eligible request, the old call can remain pending and still land if valid.

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
abandoned local lifecycle or an underpriced initial attempt supplies the fee hint; processes do not recover peers' business calldata.
If distinct relays accepted competing candidates at the same nonce, fresh replacements can still
compete: nonce counts do not establish exclusive ownership.

Signed attempts and fee hints are in memory. Restart forgets their hashes and fees, but fresh business
work still targets the lowest unconsumed nonce. Underpriced responses rebuild a fee floor over fresh
requests without knowing the old transaction or its sender. Different replicas may replace still-valid
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
| [LI.FI](LIFI-PLAN.md) | Release local capacity. A later WebSocket redelivery or reconnect REST recovery rechecks on-chain order status and rebuilds the request. A healthy connected feed does not periodically replay REST after catch-up. |
| [OEV](OEV-PLAN.md) | Settlement is submitted externally and does not use this manager. |

RFQ also reconciles typed estimate execution reverts and mined reverts before assigning business
failure. Unsigned estimate races use polling/deadline bounds without spending the signed retry budget;
mined reverts use that budget when fresh backend state remains open. Exact Reactor `NonceUsed()` data
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
accounting, underpriced fee-floor progression and request/global ceilings. An Anvil race test synchronizes
three independent same-key managers at nonce 0, mines a winner, and rebuilds the remaining eligible orders
at nonces 1 and 2 after mined state advances. It verifies all three distinct transactions execute canonically.
Another scenario abandons a pending business call, verifies no
self-transfer was sent, and mines a fresh different business call replacing its unused nonce. A manager
recreated with no hints replaces unknown higher-fee pending work at nonce 0 through fresh priced requests.
These public-pool tests do not
establish retention or consistency guarantees for a private submission provider.
