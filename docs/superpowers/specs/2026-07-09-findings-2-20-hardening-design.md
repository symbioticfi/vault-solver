# Findings 2–20 Hardening Design

**Date:** 2026-07-09

**Status:** Approved for planning

**Branch:** `stage`

## Objective

Implement audit findings 2 through 20 as one coordinated hardening program while preserving the
repository's generic-framework/integration boundary. Finding 1 is explicitly out of scope. The work
will land as dependency-ordered, independently reviewable commits; generated code will only change
through its vendored source and generation target.

Success means every selected finding is either covered by a regression test or, for toolchain and
documentation-only work, by a deterministic verification command. The final tree must pass the full
Go 1.26.5 format, build, race/coverage test, lint, generated-code drift, and documentation checks.

## Constraints and assumptions

- The existing public YAML shape remains backward compatible except where a security invariant
  requires rejecting a previously accepted unsafe value.
- No database is introduced. Transaction, RFQ, and OEV tracking remain in-memory and reconcile with
  their authoritative API/on-chain sources after restart.
- Protocol-specific changes remain under `internal/solvers/<name>/`; shared HTTP, RPC, signer,
  transaction, and process-lifecycle mechanisms stay generic.
- Generated Go under `api/` is never hand-edited.
- Finding 1 remains excluded. In particular, this work does not add digest pins for reusable
  workflows or container base images. A base-image tag changes only as required for Go 1.26.5.
- No deployment, push, or pull request is part of this implementation.

## Delivery structure

The implementation is split into seven changesets, in dependency order:

1. Toolchain and deterministic generation: findings 17 and 19.
2. Generic runtime safety: findings 7, 10, and 11.
3. Transaction lifecycle and the shared supervision foundation: findings 2 and 16.
4. RFQ correctness and cache behavior: findings 4, 9, and 15.
5. OEV transport, state, result processing, search, and accrual: findings 5, 6, 8, 12, and 13.
6. 3F expiry, exact rates, and salted domains: findings 3 and 14.
7. Boundary characterization and documentation reconciliation: findings 18 and 20, with relevant
   tests and docs also landing beside the code they describe.

## 1. Toolchain and generation

### Go 1.26.5

All exact Go pins move together: `go.mod`'s `toolchain`, Docker builder tag and `GOTOOLCHAIN`, local
scripts, and contributor commands in `CLAUDE.md`. The language directive remains `go 1.26`.

### Reproducible generated code

`hack/openapi-generator-cli.sh` will require both a generator version and a committed SHA-256 for the
corresponding JAR. It will verify cached and newly downloaded JARs before execution and delete a bad
cache entry rather than running it. The checksum is owned by the Makefile next to the version pin.

A `check-generated` target will regenerate all committed bindings/clients from vendored ABIs,
OpenAPI documents, GraphQL schema, and operations, then fail if the generated paths differ from Git.
CI gets a dedicated generation-drift job with the required Java and pinned Go codegen tools. This
job does not re-vendor live upstream artifacts; network-fetched schemas/specs are deliberately not
part of a deterministic check.

## 2. Generic runtime safety

### RPC preflight and safe endpoint labels

`chain.Dial` will receive the configured chain ID and individually probe every primary, fallback,
and distinct write endpoint before returning a client. Every endpoint must answer `eth_chainId` and
match the configured ID. This prevents a healthy primary from hiding a wrong-chain fallback until a
production failover. This intentionally makes every configured fallback reachable-at-startup rather
than best-effort; an unusable safety endpoint is treated as invalid configuration.

Logs and errors will identify endpoints by ordinal plus a sanitized label containing at most scheme
and host. Userinfo, path, raw query, and fragment are never emitted. Full URLs remain available only
inside the transport for dialing and duplicate detection.

The unused generic `chain.wsUrl` field is removed rather than left as a configuration promise. OEV's
solver-local WebSocket URL is unaffected.

### Bounded generated-client responses

A small generic RoundTripper wrapper will cap response bodies before generated clients read them.
It will reject an oversized declared `Content-Length` early and wrap all other bodies in a reader
that returns a distinct size-limit error. The 3F and RFQ clients compose this wrapper with their
existing timeout/routing transports. Limits are constants chosen above valid protocol payload sizes,
not operator-controlled allocation knobs. The already-bounded Morpho GraphQL client remains as-is.
Tests exercise the real generated clients with both declared-length and chunked oversized responses.

### Fee validation

Configuration rejects NaN, infinities, and negative `maxFeeGwei`/`tipGwei`. When both values are
explicit, `maxFeeGwei` must be at least `tipGwei`. If a configured max fee is paired with a node-
suggested tip above that cap, transaction construction fails explicitly. The fee cap is never
silently raised to the tip.

### Supervised process lifecycle

The root process will run the observability listener, transaction manager, and all solvers in one
`errgroup`. Unexpected observability bind/serve errors are fatal and cancel siblings. A generic fatal
reporter lets a nested component surface its child error before joining work blocked on a root-owned
sibling. The root clears readiness and cancels its worker context immediately; the sibling still
returns its authoritative outcome, and all components return only after their children have stopped:

- the transaction dispatcher joins confirmation/replacement trackers;
- RFQ joins its HTTP server and poller and performs bounded graceful shutdown;
- OEV joins monitor/ops/WebSocket pumps and settlement-attribution work.

Expected context cancellation maps to clean shutdown; operational failures retain context and reach
the top-level error.

## 3. Transaction lifecycle state machine

The manager remains the only allocator of the signing EOA's nonce, but nonce allocation/broadcast is
separated from receipt tracking. A single dispatcher serializes initial transactions. Once it has a
definite rejection or a signed hash in an accepted/ambiguous state, it can process the next queued
request; confirmation trackers run under the manager's supervised lifetime. Thus one slow receipt no
longer blocks unrelated solvers from obtaining later nonces and broadcasting their work.

### Explicit result states

The internal lifecycle and returned `Result` gain a typed state and nonce. Results retain every
attempted hash plus the canonical/final hash, receipt, and error. States include `not_broadcast`,
`rejected`, `broadcast_unknown`, `pending`, `confirmed`, `reverted`, and `unresolved`; callers normally
observe a terminal state, while logs/status transitions retain the ambiguous and pending phases.
Terminal meanings are:

- `rejected`: construction/signing or a definite pre-admission RPC rejection; the nonce is reusable;
- `confirmed`: a successful canonical receipt with the requested confirmations;
- `reverted`: a failed canonical receipt with the requested confirmations;
- `unresolved`: the logical transaction may have been accepted but could not be resolved within the
  bounded replacement policy; its nonce must never be reused for different calldata.

The signed transaction hash is computed before broadcast. Transport failures, timeouts, "already
known", and other errors that may occur after admission are treated conservatively as ambiguous,
commit the nonce, and enter tracking. Only a small allowlist of deterministic validation failures is
classified as rejected.

A `SafeToRetry` helper is true only for a definite rejection before admission. Consumer code uses
that helper rather than inferring safety from `Err != nil` or a zero receipt.

### Canonical confirmations and transient failures

Receipt polling treats `NotFound` and transient RPC errors as retryable until the tracking deadline.
A receipt is not terminal until its block is still canonical: the tracker re-fetches the header at
the receipt block and compares the block hash. A disappeared or mismatched receipt returns to pending
tracking. Reverts are reported only after the same canonical confirmation rule as successes.

### Replacement policy

Pending tracking is bounded by validated YAML settings for pending interval, fee-bump basis points,
and maximum replacements. A replacement uses identical chain ID, nonce, destination, value, calldata,
and gas limit with monotonically increased EIP-1559 tip/max fee. It never exceeds an explicit max-fee
cap. Before changing fees, the tracker may re-broadcast the identical signed bytes; all hashes for one
nonce remain eligible for receipt reconciliation, and exactly one logical result is delivered.

If the cap prevents a valid bump or all attempts expire, the result is `unresolved`, not a generic
failure. RFQ records such a transaction as submitted and reconciles against the backend rather than
re-arming it. 3F records the affected redemption batch as pending and relies on its on-chain resync
before permitting a retry. Neither caller interprets ambiguity as "nothing was sent".

## 4. RFQ correctness and bounded state

### Bind the executable response to the requested order

The backend adapter will select an exact `orderId` match from `/orders`, reject duplicate matches, and
reject a non-empty response that does not contain the requested ID. Execution will decode
`encodedOrder` first and treat the signed order as authoritative for filler, token input, amount,
deadline, and outputs. Backend-projected filler/output fields are checked for equality when present
but never used to construct the fill. The configured executor must equal the decoded order's filler.
Deadlines and amounts remain `big.Int` throughout validation so a large valid `uint256` cannot truncate
through `int64`.

This binds strategy input, required output, swaps, and final calldata to one signed object and makes a
misordered or malicious backend response fail before transaction submission.

### Fail-closed adapter reads and config

RFQ configuration uses the non-zero address parser for `executor`. Inventory recovery requires a
successful, decodable `paused()` result; a reverted or malformed pause read drops the adapter just as
`paused == true` does. `getMaxAssets` and `getMaxRate` retain their existing positive-value checks.

### Amortized fill-plan eviction

The default strategy will no longer scan its entire three-hour plan map for every quote. It keeps a
next-sweep timestamp under the existing mutex, performs a full expiry sweep at a bounded interval,
and removes an individually requested expired entry on lookup. Hot-path insertion remains O(1)
amortized while periodic sweeps keep memory bounded. Time remains injectable for deterministic tests.

## 5. OEV hardening

### WebSocket transport

Configuration accepts `wss://` in production. Plain `ws://` is allowed only when the parsed hostname
is `localhost` or an IP for which `IsLoopback` is true. URLs with missing hosts, credentials, or other
schemes fail validation. Each successful Gorilla connection receives a fixed read limit before any
subscription or read pump starts, bounding a malicious frame independently of JSON decoding.

### Per-component freshness

The ops snapshot will carry independent timestamps for executor state, callback balance, loan/ETH
rate, gas predictor state, and latest-head gas limit. A refresh merges only successfully read values
and timestamps; reusing a previous balance or predictor never refreshes that component's age. Missing
or older-than-`maxStateAge` required components fail the bid with `stale_state`, with component names
and ages in logs. Executor nonce/deposit bookkeeping still runs whenever that read succeeds, even if
another component fails.

### Duplicate liquidation results

A bounded, WebSocket-goroutine-owned result cache will prefer the result/auction ID, then normalized
transaction hash, then a deterministic identity of the exact frame when both are absent. Duplicate
deliveries from broadcast and callback-specific topics are dropped before reservation release,
breaker updates, metrics, logs, and gas attribution. Auction dedup remains separate.

### Bounded beam search

Beam expansion will probe trials into lightweight descriptors and maintain only the best 64 in a
bounded min-heap instead of materializing and sorting every `64 × candidates` state. Equal-score
sequence numbers preserve today's deterministic stable ordering. Only the retained frontier is
deep-materialized, copying the selected-leg slice, affected collateral budget, and affected replay
state. The search still scans the full candidate set. Golden/reference tests must prove selection
parity. Benchmarks cover 100, 1,000, and 10,000 candidates, realistic gas-derived depths, execution
time, and allocations; an invariant test bounds materialized states by beam width × depth.

### Coherent Morpho fee and borrow rate

The production monitor will resolve the Morpho deployment from the configured callback, then enrich
API-discovered markets with exact on-chain `market(id)` state and `borrowRateView(params, state)` at a
single pinned block. The on-chain market tuple supplies the exact fee; the IRM call consumes that same
tuple, so fee, totals, last-update, and rate are coherent. A generic `MulticallAt` primitive will allow
the OEV reader to pin both batches to a block without leaking protocol details into `internal/chain`.

The monitor will fetch that block's header and use its timestamp for snapshot/auction freshness.
GraphQL `state.timestamp` is Morpho's last market-update time, not the block timestamp, and will no
longer be used as if it were the latter.

Markets with a missing/reverting Morpho or non-zero IRM read are excluded rather than assigned zero
accrual. An actual zero IRM legitimately receives a zero borrow rate.
The local accrual math stays in `internal/morpho`; parity tests compare locally accrued totals and
borrower debt against independently calculated vectors at the pinned timestamp.

## 6. 3F expiry and exact signed data

### Offer lifetime versus discovery

Offer lifetime becomes `intervals.offerTTL`. When omitted it defaults dynamically to twice the
configured discovery interval, and an explicit value shorter than one discovery interval is rejected.
The default therefore cannot reproduce the current 30-minute-offer/one-hour-discovery gap while still
allowing an operator to choose a tighter valid cadence. Expiration is calculated from one injected
clock read and the same value is signed, sent, and cached. The new field is documented in the example,
README-facing configuration guidance, and 3F plan.

### Exact auction rate

The vendored 3F schema changes numeric fields to their real semantic types: signature-bearing chain,
auction, and offer IDs become `integer`/`int64`; `maxRate` is generated as a double with its documented
tenth-of-a-basis-point granularity; and the optional domain salt is represented as a nullable 32-byte
hex string. Generated code is then refreshed from that schema without changing the upstream numeric
`maxRate` wire contract.

At the handwritten boundary, `maxRate` is validated and converted once to an integer count of tenth-
basis-points. Strategy inputs use the exact integer form; the webhook retains the semantic
`maxRateBps` field but emits an exact decimal string such as `"50.5"`. Eligibility
compares `maxRateDeciBps` with `minYieldBps × 10`; expected return is integer arithmetic
`principal × maxRateDeciBps / 100_000`, rounded down. No money-facing path uses `float32`, `float64`, or
`big.Float` after boundary conversion.

### Salted EIP-712 domains

Offer signing uses a domain value object containing name, version, chain ID, verifying contract, and
optional salt. The EIP-712 domain type string includes `bytes32 salt` only when salt is present, in the
standard field order. Salt must decode to exactly 32 bytes. Unsalted behavior remains byte-for-byte
compatible. Golden signatures and independent `apitypes` parity tests cover salted and unsalted
domains, large chain IDs, and malformed salt rejection.

## 7. Characterization tests and documentation

Money- and trust-boundary tests are added before behavioral changes:

- 3F: generated-client JSON fixtures, exact rate conversion, complete API-auction → Multicall state →
  strategy → signed POST flow, tracker update only after 2xx, redeem-calldata flow, expiration
  invariant, and salted/unsalted EIP-712 vectors;
- RFQ: ABI-shaped multicall success/failure matrices, fail-closed pause reads, exact order selection,
  backend-versus-signed-order mismatch rejection, and final fill-calldata decoding;
- OEV: pinned-block Morpho/IRM multicall decoding, component-freshness failure matrices, duplicate
  result suppression, beam-search parity/benchmarks, and accrual vectors;
- signer: production hex-key and encrypted-keystore paths, recovered hash signer, EIP-155 transaction
  sender, concurrent signing under the race detector, malformed-secret redaction, and wrong-passphrase
  behavior.

Documentation is updated in the same changeset as behavior. The reconciliation includes:

- removing generic `chain.wsUrl` from config examples and plans;
- correcting the README's 3F chart filename and deployment description;
- making RFQ internal/external quote and execution scoping statements consistent with code, and using
  current adapter/discount terminology, including removal of stale quote-discount and public-discount
  claims;
- removing stale "first/only solver", dynamic-adapter, transaction-manager, and WebSocket claims from
  the 3F plan;
- renaming stale 3F `BridgeFacilitatorAdapter` references to `ThreeFAdapter`;
- updating live TODO sections when an audited gap is completed.

## Verification

Each changeset runs focused tests first. Before completion, the repository must pass, using Go 1.26.5:

```text
golangci-lint run --fix
go build ./...
go test -race -cover ./...
golangci-lint run
make check-generated
```

The OEV beam benchmark is recorded with `go test -run '^$' -bench Bundle -benchmem` for before/after
comparison. No success claim is made from cached or partial output; every final gate is run against the
finished working tree.
