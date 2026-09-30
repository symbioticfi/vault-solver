# OpenZeppelin Relayer transaction processing research

Reference: [OpenZeppelin Relayer](https://github.com/OpenZeppelin/openzeppelin-relayer), reviewed at
commit [`b790686deafed19a1b682462cf83b1fcd58e8345`](https://github.com/OpenZeppelin/openzeppelin-relayer/tree/b790686deafed19a1b682462cf83b1fcd58e8345).
The findings below describe that revision's EVM implementation. Vault Solver implements the selected
mechanisms independently; no OpenZeppelin source code was copied.

## Upstream processing model

The EVM lifecycle is `Pending → Sent → Submitted → Mined → Confirmed`, with failure, expiry and
cancellation outcomes. These stages are repository records and separate jobs rather than one request
waiting synchronously for confirmation. [Admission](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/domain/relayer/evm/evm_relayer.rs#L223-L302)
creates the transaction record, schedules status monitoring first, then queues preparation. Failure to
schedule monitoring prevents preparation, avoiding an unmonitored signed transaction.

[Preparation](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/domain/transaction/evm/evm_transaction.rs#L651-L879)
estimates gas, calculates fees and checks balance before reserving a nonce. It persists the nonce
before signing, so a failed signer call can retry with the same reservation. Signed bytes and their
hash are persisted as `Sent` before a submission job broadcasts them. Acknowledged submission becomes
`Submitted`; the status worker later observes inclusion and confirmations.

The transaction counter is separate from the records. Redis uses an [atomic increment](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/repositories/transaction_counter/transaction_counter_redis.rs#L115-L152)
for allocation. [Nonce synchronization](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/domain/relayer/evm/nonce.rs#L73-L169)
raises the counter to the chain's nonce floor without overwriting newer reservations. Health processing
detects gaps beneath tracked work and fills them with zero-value self-transactions. Upstream documents
a reservation-before-record race mitigated by a settling delay and rechecks; Vault Solver's single
lifecycle does not introduce that allocation pattern.

The pipelines permit several pending nonces per sender. Default [worker concurrency](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/queues/queue_type.rs#L95-L118)
is 50 preparation, 75 submission and 100 EVM status jobs, configurable independently. Repository storage
can be [memory or Redis](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/docs/configuration/storage.mdx#L23-L91):
memory loses transaction state on restart, whereas Redis retains it. Job queues are a separate
infrastructure choice; upstream supports [Redis, SQS, Pub/Sub and RabbitMQ](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/README.md#L326-L338).

## Retry, replacement and outcome recovery

[Broadcast handling](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/domain/transaction/evm/evm_transaction.rs#L891-L1061)
distinguishes known transactions from nonce errors. A consumed nonce triggers receipt reconciliation
rather than replay at a fresh nonce. RPC calls have [backoff, jitter and provider failover](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/services/provider/retry.rs#L165-L231).
[Status processing](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/domain/transaction/evm/status.rs#L650-L761)
re-drives stuck stages, while submission jobs themselves have [zero queue retries](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/constants/worker.rs#L1-L19).

Fee replacement retains the nonce and [bumps both EIP-1559 fee fields](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/domain/transaction/evm/price_calculator.rs#L298-L360).
[Cancellation](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/domain/transaction/evm/evm_transaction.rs#L1261-L1333)
replaces exposed work with a same-nonce self-send, then keeps tracking it. Historical hashes remain
available for [winner recovery](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/domain/transaction/evm/status.rs#L1470-L1556).
The [mined receipt determines cancellation](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/domain/transaction/evm/status.rs#L190-L310):
the original transaction may win despite cancellation being requested.

## What Vault Solver adopts

This change adopts durable signed ownership and restart recovery. It does **not** reproduce
OpenZeppelin's processing of multiple pending nonces. Vault Solver retains one unresolved signed
lifecycle, its existing fee horizon, canonical receipt checks, confirmation policy, nonce-conflict
gates and shared generic txmanager. No Redis, queue service or new operational service is required.

Optional `txManager.stateFile` stores that lifecycle in a private file on operator-provisioned
persistent storage. Empty `stateFile` retains memory-only behavior, analogous to upstream's optional
memory versus persistent repository storage. Every signed original, replacement and cancellation is
written before broadcast using a synced temporary file, atomic replacement and directory sync. This
also applies the ordering to replacements: upstream's [resubmission path](https://github.com/OpenZeppelin/openzeppelin-relayer/blob/b790686deafed19a1b682462cf83b1fcd58e8345/src/domain/transaction/evm/evm_transaction.rs#L1080-L1250)
broadcasts its replacement before persisting the new hash.

Startup validates chain, sender and signed attempts, then pauses admission and readiness while
recovering their exact hashes. Unresolved work is conservatively cancelled at its owned nonce:
process-local `Obsolete` callbacks and solver capacity reservations cannot be reconstructed. Recovery
never broadcasts the business call at a new nonce or extends its saved cancellation deadline. The
original can still win; its canonical receipt remains authoritative. Unconfirmed or interrupted
tracking retains state, and storage failure keeps admission paused. The journal is an ownership record,
not a replacement for protocol-specific on-chain and API reconciliation.
