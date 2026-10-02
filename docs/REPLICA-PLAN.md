# Independent processes sharing a transaction sender

This plan describes canonical nonce reconciliation, which always runs for transaction-manager users.
It keeps the existing EOA, signer interface, solver configuration and per-process transaction manager.
The [README](../README.md#independent-processes-with-the-same-eoa)
contains the operator configuration and three-container example; the [transaction manager plan](TXMANAGER-PLAN.md)
defines request, receipt, fee and shutdown semantics.

## 1. Scope and identity

Several processes may use the same chain and transaction-sender EOA without a database, shared writable
state, process membership, leader election or peer communication. Each process has its own chain client,
one signer and one transaction manager shared by its local solvers. Neither replica count nor replica
identity is passed into transaction execution. One unresolved signed lifecycle per process remains the
local concurrency bound.

Reconciliation does not assign separate transaction nonce sequences. The ordinary EOA has one sequential
nonce on-chain, and concurrent processes can read the same available nonce before either sends. The
chain decides whether a nonce was consumed. It does not allocate a nonce to a process or promise ordering,
fairness or inclusion. Same-nonce transactions, replacements and cancellations can compete on fees and
builder selection. A process can have its attempted transaction displaced while continuing to observe
the shared account.

The generic layer knows account nonces, signed hashes and receipts. Each solver owns business status,
deadlines, inventory and retry policy. Protocol order/bid nonces remain separate from the EOA transaction
nonce. No solver receives a cross-process lock or a global quote/reservation book.

## 2. Configuration and provider contract

Canonical nonce reconciliation is always active for every transaction-manager user, whether one process
or several use the EOA. It requires:

- The same configured chain, sender and intended read/write routes in each process; keep the current
  signer and solver config. Distinct scrape target labels identify operational instances.
- A write endpoint whose `eth_getTransactionCount(address, "pending")` includes accepted private
  transactions as well as public transactions. Broadcasts and admission latest/pending nonce reads
  stay on this non-fallback endpoint.
- Read endpoints that serve coherent headers and account nonce state at specific recent block hashes
  through EIP-1898 (`ReadNonceAtHash`, `requireCanonical: true`), for both the current head and its
  confirmation ancestor.
  Canonicality is checked independently of endpoint affinity; a numeric block read is not substituted
  when exact-hash reads fail.
- A cancellation route that accepts same-nonce self-cancellations: existing `chain.cancelRpcUrl` when
  configured, otherwise the ordinary write RPC. When using a private write RPC, configure an appropriate
  cancellation endpoint. This is the existing routing API; recovery does not require or imply delivery
  through a public mempool.
- A finite positive `txManager.maxFeeGwei`, existing cancellation/time budgets and sufficient sender
  funds. Each process applies its own fee policy; there is no shared fee auction or spending budget.

MEV Blocker documents that its pending count combines private and public transactions. That gives the
manager the provider's pending view, not an atomic reservation, complete visibility into every builder,
or a published transaction retention guarantee. Its hash-based transaction details require the hash;
the count does not recover an unknown attempt's fee, calldata or ownership. Provider outages or
unsupported exact-hash state reads preserve the wait rather than authorize a guessed nonce.
[MEV Blocker RPC reference](https://docs.mevblocker.io/reference/api/json-rpc-methods#eth_gettransactioncount),
[EIP-1898](https://eips.ethereum.org/EIPS/eip-1898).

## 3. Startup and fresh admission

Startup keeps readiness false while unknown contiguous pending activity exists or canonical read
account activity has not reached `txManager.confirmations`, even if the write endpoint lags. It cannot
infer an old attempt's hash, deadline, request confirmation override or fee cap after restart because
all signed state was in memory.

Fresh admission reads the sender nonce at both the current canonical head and its confirmation ancestor
by exact hash, validates their ancestry and verifies that the head remains stable across the proof.
Both account nonces must equal the write endpoint's latest and pending nonces before signing. Thus a
stale write endpoint cannot hide a read-head nonce advance that has not reached the required depth.
Incoherent or unavailable evidence is retried. Unknown work uses the configured manager confirmation
depth; per-request overrides lost on restart are not reconstructed.

Each nonce/header RPC in an admission, account-consumption or contested-cancellation proof has its own
`min(2 seconds, replacementInterval/2)` timeout. The entire proof follows its caller/lifecycle context
and still requires the final stable-head check. It does not share one short aggregate budget across
sequential ancestry reads, so a larger confirmation depth can complete on a healthy but slower endpoint.

Each new admission refreshes nonce evidence instead of trusting a local increment from a previous
request. Pending or insufficiently confirmed account activity, or unavailable proof, rejects the attempt
before signing (`NotAdmitted`) and pauses the local lane. An idle monitor retries the evidence check so
subsequent evidence can restore admission. Startup waiting respects process shutdown; ordinary
pre-admission waits retain caller deadlines. Unknown work first receives a bounded observation window;
the stale-recovery rule below applies after `pendingTimeoutMs`. Multiple unknown nonces, failed proof or
a recovery replacement that cannot fit under the cap can still leave the process unready indefinitely.

This cannot make the check and broadcast atomic. Two processes can pass the same check and sign at the
same nonce. The process continues tracking its exact signed variants through ambiguous send errors and
nonce/fee collisions. Its ordinary deadline, shutdown and obsolescence cancellation applies only to its
own tracked lifecycle, and can still compete with another process's same-nonce work.

### Unknown-nonce timeout recovery

The manager observes a single contiguous unknown nonce and records its first-seen time locally. It
retains that nonce even if the provider later drops it from pending. A lost pending entry does not
prove absence from builders, so admission remains blocked while the nonce is unresolved.

After `pendingTimeoutMs`, if the write endpoint's latest nonce still equals the recorded nonce and the
current unknown gap is at most one, recovery submits a zero-value self-transfer at that nonce with empty data, 21,000 gas, and
both EIP-1559 fee fields fixed to the current global cap. The configured cancellation route is used.
The candidate is retained and retried without moving the nonce or raising fees beyond the cap. A
mined nonce advance instead requires canonical confirmation; it is never cancelled as though unmined.
Recovery handles one unknown nonce at a time and does not guess how to drain multiple queued nonces.

An owned candidate rejected in a same-nonce collision keeps its original hashes. Only after the full
`PendingTimeout` age from immutable `firstSignedAt`, set immediately before the initial sign/send,
may it use the same capped cancellation. Before signing or rebroadcasting it, fresh latest/pending
checks and a stable zero-depth, exact-hash canonical-head proof must show that the current account nonce
equals both write latest and the tracked nonce. A lagging write endpoint cannot authorize cancellation
of mined but unconfirmed work, nor a future nonce after a reorg. Unknown startup cancellation already
passes the stricter fresh-admission head/ancestor proof. An earlier `Request.CancelAt` or shutdown does
not shorten this wait. Its known original/cancellation receipts retain the ordinary receipt-result path;
no business calldata is
moved to a new nonce by recovery.

Preexisting unknown work has no request result to deliver. The manager retains/rebroadcasts its exact
recovery candidate and reopens admission only on canonical confirmed account proof, regardless of which
transaction consumed the nonce. Confirmed consumption clears the timer; a newly observed single nonce
starts another timer. Restart loses that observation and candidate. Fresh idle probes and unknown
recovery hold the local lifecycle slot to avoid overlap with an admitted worker in this process.
Unresolved recovery keeps readiness false.

Processes with the same chain, EOA, nonce and cap build identical fields. The local signer produces
identical signed bytes; remote/custom signers may use different signatures because determinism is not
part of the `Signer` contract. Timing and provider views remain independent. Recovery can cancel a
healthy process's pending transaction, and the original can beat the cancellation. It never authorizes
replay of the unknown business call.

The full-cap recovery relies on normal calls' existing fee reserve for replacement headroom. A previous
full-cap cancellation, an older/higher cap, cap decreases or relay replacement-policy disagreement can
prevent acceptance. No fee cap is exceeded to force liveness. A mined recovery with tip equal to the cap
can spend `21,000 × maxFeePerGas` in gas fees. Confirmation proof still controls release after any winner;
elapsed time and RPC acceptance alone do not reopen the lane.

## 4. Receipt priority and unknown business results

Own signed receipts remain the strongest evidence for the submitted request. Receipt sweeps, exact-hash
validation, canonical ancestry and the request's confirmation policy retain their ordinary behavior.
Older signed variants can win, and cancellation can lose to the original call. Only a complete sweep
where every known hash returns `NotFound` enables the account-consumption path. RPC errors, malformed
receipts and unresolved owned receipt evidence keep tracking active rather than being treated as absence.

When no owned receipt settles the lifecycle, the nonce at the canonical confirmation ancestor can prove
that the tracked nonce has been consumed. This runtime proof does not require current-head nonce
equality: later account activity must not prevent resolving an older consumed nonce. The manager then
returns `OutcomeNonceConsumed` (`nonce_consumed`) with an error wrapping `ErrNonceConsumed` and ends its
local lifecycle. Admission is refreshed separately; newer
unknown account activity can still keep the lane paused. The result does not synthesize a receipt or
discover a canonical winning hash. Its attempted hash is not proof of inclusion. The outcome means the
business result is unknown: our own transaction
could have landed while its receipt was unavailable, or another transaction could have consumed the
nonce. It is neither `confirmed`, `cancelled` nor proof that our fill failed.

Receipt and nonce reads can straddle inclusion: a sweep can return `NotFound` before mining, then account
proof can succeed after mining. Even the canonical winning attempt can therefore return `nonce_consumed`;
its receipt becoming available later does not change the already delivered unknown result.

The manager never resubmits old business calldata at a new nonce automatically. An integration must
query its authoritative protocol/backend state, retain an unresolved business item while that query is
uncertain, and construct fresh executable work only when its own retry rules permit. Fresh plans must
revalidate deadline, current status, liquidity, fees and any time-sensitive signatures.

## 5. Integration behavior and limits

Account reconciliation always applies to transaction-manager users. Business reconciliation stays
inside each integration; its plan records the detailed policy.

| Integration | Response to unknown execution |
|---|---|
| [RFQ](RFQ-PLAN.md) | Re-query backend order state. A known terminal status retires the item; a known open order can retry only after the poll delay, within its deadline and existing retry budget, with a fresh fill plan and resolved signatures. Missing/unknown/error status keeps reconciliation pending within the order's bound. |
| [UniswapX](UNISWAPX-PLAN.md) | Release local capacity and invalidate quotes; retry through fresh order polling and revalidation. Nonce consumption is neither a successful fill nor a failed-attempt/breaker event. |
| [LI.FI](LIFI-PLAN.md) | Release the local reservation. No automatic retry is scheduled; further attempts require upstream WebSocket redelivery or REST recovery after reconnect, which checks on-chain status and validity and rebuilds the plan. A healthy feed does not periodically REST-poll an order lost through contention. |
| [3F](3F-PLAN.md) | Treat nonce consumption as an expected decline without a redemption-success metric. The next `canWithdraw` poll reads request state again and excludes finalized requests. Offers and capacity remain uncoordinated across processes. |
| [OEV](OEV-PLAN.md) | Settlement is submitted externally and does not use this transaction manager. Account reconciliation provides no new OEV replication guarantee. |

Local quote caches, in-flight sets, liquidity reservations, cancellation retry budgets and offer counters
are not shared. Simultaneous quotes/offers can observe the same liquidity and produce duplicate or
overcommitted off-chain promises. Global commitment accounting and arbitrary active-active safety for
all integrations are unsupported. UniswapX exclusive-order obligations, 3F offer capacity and other
upstream delivery rules still require their integration's deployment policy. Chain nonce recovery alone
does not satisfy those obligations or make backend writes globally serialized.

## 6. Restart, reorg and deployment limits

No signed attempts or business requests are persisted. A provider can omit a private attempt, forget it,
or report no contiguous pending gap while a transaction beyond a gap still exists. A previously
distributed attempt can later land even when startup observed nonce equality. If that consumes a locally
tracked nonce, account reconciliation can release the local lane once the consumption is confirmed; it
cannot recover the missing business receipt or retroactively prevent competing submissions.

Confirmation depth is a configured confidence boundary, not irrevocable finality. Canonicality checks
reject evidence that changes during reconciliation, but a later reorg deeper than the chosen depth can
undo a consumption already accepted. Unknown work after restart also loses any stricter original
per-request confirmation policy. Operators must select the manager depth with that limitation in mind.

All processes retain the existing graceful shutdown: stop solver intake/preparation, drain accepted
work, then stop the transaction manager within its shutdown bound. No lock is released and no leadership
is handed off. An abrupt exit loses owned attempt tracking; another process waits/reconciles account
evidence under the same provider assumptions. Existing quote/upstream routing remains operator-owned;
the example does not introduce a router or change signer permissions.

## 7. Observability and verification

Readiness reports this process's nonce evidence, startup and shutdown state; it is not leadership or a
guarantee that all processes share a global book. A process can remain ready while tracking its owned
transaction. Unknown pending/unconfirmed activity at admission and unresolved conflict evidence keep
its lane unavailable. Label each scrape target distinctly; never sum shared-account balances as though
they belong to separate EOAs.

`tx.outcome=nonce_consumed` is an expected contention/recovery outcome. It reports unknown business
execution and adds no synthetic receipt, gas or fee accounting. An attempted `tx.hash` must not be
displayed as the canonical winner. Solver business reconciliation and later successful retries have
their own observations; [TRACING-PLAN](TRACING-PLAN.md) defines the span contract.

Required verification covers pending and insufficiently confirmed startup, a stale write endpoint while
the canonical read-head nonce has advanced, fresh admission across independent managers, exact-hash
canonical account evidence, own-receipt priority, nonce consumption without an owned
receipt, timeout recovery after a pending entry disappears, deterministic local recovery candidates,
multiple unknown nonces, capped recovery rejection, delayed reads at larger confirmation depths,
per-call timeouts, RPC failures, reorgs and shutdown. Solver tests must prove an uncertain result is reconciled
before any fresh business request is built. `make test-txmanager-anvil` runs the race-enabled real Anvil
suite, including independent-manager contention; CI runs the same target. It complements scripted RPC
tests and does not prove private-relay retention or a production deployment. Build, race/coverage and lint gates
remain required for implementation changes.
