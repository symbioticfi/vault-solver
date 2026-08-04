# User-directed swap calldata design

**Status:** Implemented design
**Date:** 2026-08-03
**Scope:** RFQ solver private API and LiquidLane adapter calldata construction

## Summary

The RFQ solver will add a private, authenticated `POST /swap` operation with three tagged phases:
`DISCOVERY`, `CONFIRM`, and `BUILD`. The phases let the RFQ backend inspect an advisory liquidity curve,
confirm one exact-input quote, and obtain an ordered set of executable adapter calls without asking the
solver to broadcast a transaction.

`CONFIRM` stores the exact allocation selected at quote time. `BUILD` must use that allocation: it may
raise quoted output when fresh state is better, but it must not change adapters, input splits, output
token, call order, or lower any leg below its confirmed floor. If the stored allocation is no longer
executable, the solver returns a stale-quote error and the backend starts again at discovery.

Every built leg uses `swap(SignedSwap,bytes)` signed by the solver's existing framework signer. This
includes allocations selected from discount inventory: the solver preserves their adapter, route,
input split, domain, order, and floor, but never resolves or returns a private signed-discount payload.

The caller and recipient are the backend-supplied Router. The Router transfers the
ordinary ERC-20 input to the adapter before invoking the returned calldata. The solver never includes
that transfer in `data`, never invokes the transaction manager, and never broadcasts these calls.

The existing `/quote` endpoint and open-order polling/execution path remain unchanged.

## Goals

- Give the backend a cheap cumulative exact-input curve before it selects a size.
- Turn an exact confirmation into a short-lived, immutable allocation.
- Build calldata idempotently for a backend-supplied `buildId`.
- Bind every direct authorization to the intended Router, adapter, chain, token, allocation, and quote
  deadline.
- Reuse current candidate normalization, capacity accounting, strategies, generated adapter bindings,
  and signer infrastructure.
- Expose canonical shared-capacity domains so the backend cannot select the same vault/output capacity
  through more than one solver.

## Non-goals

- Broadcasting, simulating, or monitoring the Router transaction.
- Replacing the current RFQ `/quote` and order fill flow.
- Reserving liquidity on chain or promising that state cannot change after confirmation.
- Supporting exact-output requests, native-token input, arbitrary recipients, or arbitrary callers.
- Adding a new signer, database, migration, contract, or generated binding.
- Letting `BUILD` silently find a replacement allocation when the confirmed one becomes stale.

## API contract

### Transport and common rules

- Method and path: `POST /swap` on the existing RFQ HTTP listener.
- Authentication: the existing `x-rfq-shared-secret` header, compared in constant time. All three phases
  require it. This endpoint is a signing oracle and must not be made public.
- The existing request-ID, access-log, recovery, metrics, timeout, and 1 MiB body-limit middleware apply.
- `protocol` is required and equals `"v2"`; `phase` is required and is exactly one of
  `DISCOVERY`, `CONFIRM`, or `BUILD`.
- `requestId`, `quoteId`, `solverQuoteId`, and `buildId` use canonical UUID strings where present.
- Each phase attempt has a fresh transport-only `requestId`; it is excluded from BUILD idempotency.
  `discoveryRequestId` binds confirmation back to the discovery batch whose floor and domains it must
  reproduce.
- Addresses are validated EVM addresses. Response addresses are lowercase `0x` strings.
- Amounts are unsigned `uint256` values encoded as base-10 strings. Zero input is invalid.
- Calldata is a lowercase, even-length `0x` hex string.
- Timestamps are Unix seconds and must fit `uint48` when used in adapter calldata.
- Only same-chain, exact-input, single-output-token swaps with distinct input and output tokens are supported.
- Requests, confirmed plans, and BUILD responses contain at most 64 adapters/calls.
- The response repeats its phase. Phase-specific fields are required exactly as described below; a field
  from another phase does not change the operation's meaning.

### `DISCOVERY`

Request:

```json
{
  "protocol": "v2",
  "phase": "DISCOVERY",
  "requestId": "8fcc1d0d-246d-4e8e-9620-13c76857b31a",
  "quoteId": "92b1be9d-25c1-4eca-80d1-fd1338ab57d2",
  "chainId": 1,
  "swapper": "0x7777777777777777777777777777777777777777",
  "tokenIn": "0x1111111111111111111111111111111111111111",
  "tokenOut": "0x2222222222222222222222222222222222222222",
  "sampleAmountsIn": [
    "400000000000000000",
    "1000000000000000000"
  ],
  "adapters": [
    {
      "adapter": "0x3333333333333333333333333333333333333333",
      "asset": "0x2222222222222222222222222222222222222222",
      "assetDecimals": 6,
      "maxAssets": "2000000000",
      "maxRate": "1000000000000000000"
    },
    {
      "adapter": "0x4444444444444444444444444444444444444444",
      "asset": "0x2222222222222222222222222222222222222222",
      "assetDecimals": 6,
      "maxAssets": "1200000000",
      "maxRate": "1000000000000000000"
    }
  ]
}
```

Successful response:

```json
{
  "protocol": "v2",
  "phase": "DISCOVERY",
  "requestId": "8fcc1d0d-246d-4e8e-9620-13c76857b31a",
  "quoteId": "92b1be9d-25c1-4eca-80d1-fd1338ab57d2",
  "chainId": 1,
  "swapper": "0x7777777777777777777777777777777777777777",
  "tokenIn": "0x1111111111111111111111111111111111111111",
  "tokenOut": "0x2222222222222222222222222222222222222222",
  "points": [
    {
      "amountIn": "400000000000000000",
      "amountOut": "802000000",
      "liquidityDomains": [
        "capacity:1:0x6666666666666666666666666666666666666666:0x2222222222222222222222222222222222222222"
      ]
    },
    {
      "amountIn": "1000000000000000000",
      "amountOut": "1998000000",
      "liquidityDomains": [
        "capacity:1:0x6666666666666666666666666666666666666666:0x2222222222222222222222222222222222222222"
      ]
    }
  ]
}
```

The request points are a strictly increasing, duplicate-free list chosen by the backend and shared by
every solver in the same allocation auction. The response points describe the solver's cumulative curve,
not marginal per-adapter quotes. Both coordinates are cumulative totals from zero. Each returned
`amountIn` must exactly equal one requested sample; interpolation and solver-selected sample sizes are
forbidden. Every point also returns the unique capacity-domain set used by that exact plan so the backend
can optimize only globally disjoint allocations. A requested size that cannot be fully covered is omitted.
A valid request with no liquidity returns `200` with an empty `points` array.

Discovery reads current candidates and runs the configured strategy, but creates no confirmation,
signature, nonce, or reservation. Points are advisory and may only be used to choose a size for a later
`CONFIRM`; the backend must not treat interpolation or a point as an executable quote.

### `CONFIRM`

Request:

```json
{
  "protocol": "v2",
  "phase": "CONFIRM",
  "requestId": "2ac09473-0c50-4db0-ad22-9417522f3ca2",
  "discoveryRequestId": "8fcc1d0d-246d-4e8e-9620-13c76857b31a",
  "quoteId": "92b1be9d-25c1-4eca-80d1-fd1338ab57d2",
  "chainId": 1,
  "swapper": "0x7777777777777777777777777777777777777777",
  "tokenIn": "0x1111111111111111111111111111111111111111",
  "tokenOut": "0x2222222222222222222222222222222222222222",
  "amountIn": "1000000000000000000",
  "minAmountOut": "1998000000",
  "deadline": 1785780300,
  "adapters": [
    {
      "adapter": "0x3333333333333333333333333333333333333333",
      "asset": "0x2222222222222222222222222222222222222222",
      "assetDecimals": 6,
      "maxAssets": "2000000000",
      "maxRate": "1000000000000000000"
    },
    {
      "adapter": "0x4444444444444444444444444444444444444444",
      "asset": "0x2222222222222222222222222222222222222222",
      "assetDecimals": 6,
      "maxAssets": "1200000000",
      "maxRate": "1000000000000000000"
    }
  ]
}
```

Successful response:

```json
{
  "protocol": "v2",
  "phase": "CONFIRM",
  "requestId": "2ac09473-0c50-4db0-ad22-9417522f3ca2",
  "discoveryRequestId": "8fcc1d0d-246d-4e8e-9620-13c76857b31a",
  "quoteId": "92b1be9d-25c1-4eca-80d1-fd1338ab57d2",
  "solverQuoteId": "ed972bed-60a9-499e-ab25-0d4d09b4aa5a",
  "chainId": 1,
  "swapper": "0x7777777777777777777777777777777777777777",
  "tokenIn": "0x1111111111111111111111111111111111111111",
  "tokenOut": "0x2222222222222222222222222222222222222222",
  "amountIn": "1000000000000000000",
  "amountOut": "1998000000",
  "liquidityDomains": [
    "capacity:1:0x6666666666666666666666666666666666666666:0x2222222222222222222222222222222222222222"
  ],
  "validUntil": 1785780300
}
```

`amountOut` is the scalar floor for the exact `amountIn`, must be at least `minAmountOut`, and uses
the exact domain set advertised for that discovery point. The solver succeeds only if it can fully cover
the input. A well-formed request that cannot be fully covered returns `204` and stores nothing.

On success, the solver stores an immutable confirmation record containing:

- the request tuple and absolute `validUntil`;
- the ordered candidate identities selected by the strategy;
- each leg's adapter, exact `amountIn`, confirmed `amountOut` floor, output token, and authorization source;
- the internal `CapacityID` values used to validate shared-vault capacity; and
- the unique external liquidity-domain set.

The record is held in the solver's bounded, concurrency-safe in-memory store until expiry. It is not a
liquidity reservation. A process restart invalidates outstanding `solverQuoteId` values, after which the
backend must discover and confirm again.

`deadline` is a requested maximum. `validUntil` is the earliest of that maximum, the configured
swap-quote TTL, and any selected authorization constraint known at confirmation time. A request beyond
the local validity horizon is shortened, not rejected. The cap is fixed in the record and is never
extended by `BUILD`.

### `BUILD`

Request:

```json
{
  "protocol": "v2",
  "phase": "BUILD",
  "requestId": "5e56f7c0-3840-4545-a8ca-e942ce3f3d71",
  "quoteId": "92b1be9d-25c1-4eca-80d1-fd1338ab57d2",
  "solverQuoteId": "ed972bed-60a9-499e-ab25-0d4d09b4aa5a",
  "buildId": "7423df2b-957b-47d5-acbc-21c3bd8a614e",
  "chainId": 1,
  "swapper": "0x7777777777777777777777777777777777777777",
  "tokenIn": "0x1111111111111111111111111111111111111111",
  "tokenOut": "0x2222222222222222222222222222222222222222",
  "amountIn": "1000000000000000000",
  "minAmountOut": "1998000000",
  "deadline": 1785780300,
  "liquidityDomains": [
    "capacity:1:0x6666666666666666666666666666666666666666:0x2222222222222222222222222222222222222222"
  ],
  "router": "0x5555555555555555555555555555555555555555"
}
```

Successful response:

```json
{
  "protocol": "v2",
  "phase": "BUILD",
  "requestId": "5e56f7c0-3840-4545-a8ca-e942ce3f3d71",
  "quoteId": "92b1be9d-25c1-4eca-80d1-fd1338ab57d2",
  "solverQuoteId": "ed972bed-60a9-499e-ab25-0d4d09b4aa5a",
  "buildId": "7423df2b-957b-47d5-acbc-21c3bd8a614e",
  "chainId": 1,
  "swapper": "0x7777777777777777777777777777777777777777",
  "router": "0x5555555555555555555555555555555555555555",
  "tokenIn": "0x1111111111111111111111111111111111111111",
  "tokenOut": "0x2222222222222222222222222222222222222222",
  "amountIn": "1000000000000000000",
  "amountOut": "2001000000",
  "liquidityDomains": [
    "capacity:1:0x6666666666666666666666666666666666666666:0x2222222222222222222222222222222222222222"
  ],
  "validUntil": 1785780300,
  "calls": [
    {
      "to": "0x3333333333333333333333333333333333333333",
      "data": "0x9a4568b6",
      "amountIn": "400000000000000000",
      "amountOut": "803000000",
      "tokenOut": "0x2222222222222222222222222222222222222222",
      "liquidityDomain": "capacity:1:0x6666666666666666666666666666666666666666:0x2222222222222222222222222222222222222222",
      "validUntil": 1785780300
    },
    {
      "to": "0x4444444444444444444444444444444444444444",
      "data": "0x9a4568b6",
      "amountIn": "600000000000000000",
      "amountOut": "1198000000",
      "tokenOut": "0x2222222222222222222222222222222222222222",
      "liquidityDomain": "capacity:1:0x6666666666666666666666666666666666666666:0x2222222222222222222222222222222222222222",
      "validUntil": 1785780300
    }
  ]
}
```

For readability the example `data` values show only the four-byte selector; production responses contain
the complete ABI-encoded arguments and signatures.

The request tuple must exactly match the stored confirmation: quote, swapper, chain, tokens, `amountIn`,
and `minAmountOut == confirmed amountOut`. BUILD chooses one exact deadline satisfying
`now < deadline <= confirmation.validUntil`; an aggregate quote therefore uses the earliest selected
solver validity. The first build ID and economic fingerprint freeze that choice. A later retry changes
only its transport `requestId`.
`router` must equal the solver's configured nonzero Router contract.

The response preserves the stored leg order. The sum of call `amountIn` values equals the top-level
`amountIn`; the sum of call `amountOut` values equals the top-level `amountOut`; and the latter is at
least `minAmountOut`. Fresh state may increase one or more leg outputs. It may not lower a leg below its
stored floor even if another leg could offset the difference.

`BUILD` re-reads state for the exact persisted candidates. It must not add or remove calls, change an
adapter or input split, reorder calls, change output token, or substitute a different candidate. Any such
need makes the confirmation stale and produces no calldata. Call construction is all-or-nothing: an error
on one leg returns no partial response or usable signatures.

The Router executes each call in order:

1. transfer exactly that call's `amountIn` of the common `tokenIn` to `to`;
2. call `to` with `data` and zero native value; and
3. enforce its own aggregate output-floor/balance-delta check.

This transfer is required because the signed adapter entrypoint consumes tokens already present at the
adapter. The Router's aggregate check remains necessary even though every signed call carries a
conservative per-leg `amountOut`.

## Allocation, identity, and deduplication

Internal candidate identity remains `liquidlane.NewCandidateID(route, discountID)`. Exact duplicate
candidates are removed before strategy evaluation, while conflicting duplicates fail closed. Direct and
discount variants of one physical adapter route are alternatives; one allocation cannot contain both.

The external `liquidityDomain` is the canonical lowercase `CapacityID` already used by the solver. For
resolved LiquidLane routes it identifies the shared vault/output capacity rather than merely one adapter:

```text
capacity:<chainId>:<lowercase vault address>:<lowercase output token address>
```

`liquidityDomains` is a unique set. More than one call in the same solver confirmation may consume one
domain because the solver accounts for their combined capacity; every call names the domain it consumes.
A plan containing the same adapter twice is invalid rather than being emitted as two calls.

The RFQ backend owns global deduplication. Before accepting a composed swap, it rejects an intersection
between the domain sets selected from different solver confirmations. After `BUILD`, it verifies that the
unique call-domain set exactly equals the confirmed set and repeats the cross-confirmation intersection
check. It also rejects duplicate adapter targets across the final transaction. These are hard errors; the
backend must not drop a conflicting leg or silently reallocate it.

## Direct `SignedSwap` construction

For every persisted leg, construct:

```text
SignedSwap(
  recipient = router,
  tokenIn   = confirmed tokenIn,
  amountIn  = persisted leg amountIn,
  amountOut = fresh amountOut, never below the persisted leg floor,
  caller    = router,
  signer    = framework signer address,
  nonce     = deterministic nonce described below,
  deadline  = exact chosen BUILD deadline (never after confirmation validUntil)
)
```

The digest uses the adapter's EIP-712 domain and the exact primary type:

```text
SignedSwap(address recipient,address tokenIn,uint256 amountIn,uint256 amountOut,address caller,address signer,uint256 nonce,uint48 deadline)
```

The framework signer's `SignHash` output is the 65-byte `r || s || v` signature with `v` in `{27,28}`.
Encode the struct and signature with the generated binding's `TryPackSwap1`; the selector is `0x9a4568b6`.
The ordinary unauthenticated `swap(Swap)` overload is never used for this API.

### EIP-712 domain and signer authorization

For every configured swap adapter, read `eip712Domain()` from chain and cache the validated result. Do not
hardcode a name, version, chain, or verifying contract. Require the advertised chain ID to equal the
solver chain, the verifying contract to equal the adapter, and reject unsupported salt/extensions or an
unsupported fields bitmap. A domain mismatch is a startup/configuration failure, not a best-effort quote.

When the swap API is enabled, the existing framework signer must be accepted by every configured adapter
as its owner, market maker, or delegated filler (`isFiller(marketMaker, signer)`). Startup fails if that
invariant cannot be established. The Router itself does not need filler authorization: it invokes the
signed overload, the signature authorizes the terms, and `caller = router` binds use of those terms to the
Router.

### Deterministic nonce and idempotency

`buildId` is supplied by the trusted backend and is immutable for one confirmed build. Parse its UUID into
16 bytes and derive each direct leg's nonce as:

```text
NONCE_TYPEHASH = keccak256(
  "VaultSolverSwapNonce(bytes16 buildId,uint256 chainId,address adapter,address tokenIn,uint256 callIndex)"
)

nonce = uint256(keccak256(abi.encode(
  NONCE_TYPEHASH,
  bytes16(buildId),
  uint256(chainId),
  adapter,
  tokenIn,
  uint256(callIndex)
)))
```

`callIndex` is the zero-based position in the persisted allocation, including legs originally selected
from discount inventory. It does not change between retries. The adapter consumes nonces per input token.

The confirmation store binds the first successful `buildId` to `solverQuoteId` and caches only the
immutable build payload. Concurrent or repeated requests with the same economic fingerprint return the
same calls and signatures without creating another allocation or nonce; each response envelope echoes
its current transport-only `requestId`. Reusing either ID with different economic fields is an
idempotency conflict. A second `buildId` cannot build the same confirmation.

Before first returning a signed leg, query `isUsedNonce(tokenIn, nonce)`. A used nonce is a conflict; do
not probe a new nonce, alter the call index, or issue a replacement authorization. Once an on-chain
execution consumes the nonce, replay is impossible by contract even if a previously cached response is
presented again.

## Discount-selected allocations

Discount candidates remain valid strategy inputs and retain their distinct candidate identity during
discovery and confirmation. BUILD does not resolve their private signatures. It re-reads the persisted
physical route and emits a fresh `SignedSwap` on that exact adapter only when the current conservative
output still meets the persisted per-leg floor. Otherwise BUILD returns a stale-confirmation conflict.
No mode emits `DiscountSwap` calldata through the user-directed endpoint.

## Validity and state races

- Add a file-driven `swapEnabled` flag, defaulting to false, a `router` address, and a positive
  `swapQuoteTtlMs`, defaulting to 30 seconds. Enabling requires `router` to be a nonzero contract.
  Configured adapters are domain/signer validated during startup. Internal-mode adapters supplied
  dynamically by the backend are validated and cached before confirmation and revalidated for build;
  enabling the API does not require a static adapter list.
- CONFIRM caps a requested maximum deadline at `validUntil`; it does not reject a longer maximum.
- A confirmation may be built only with `now < BUILD.deadline <= validUntil`.
- `SignedSwap.deadline`, Router execution deadline, Router authorization deadline, response `validUntil`,
  and every call `validUntil` equal that exact chosen BUILD deadline.
- BUILD rechecks the chosen deadline after dependencies and immediately before caching or returning.
  It is not a reservation or guarantee against intervening chain state.
- Expired confirmations and build-cache entries are swept. The store has a fixed upper bound; once full,
  it rejects new confirmations rather than evicting live ones.

## Errors

The API uses Huma's normal structured error body and never serializes raw RPC errors, discount payloads,
signatures, secrets, or calldata in error details.

| Status | Meaning |
| --- | --- |
| `200` | Successful phase; discovery may contain zero points. |
| `204` | A valid `CONFIRM` request cannot be fully covered. |
| `400` | Malformed JSON, invalid phase-specific fields, mismatched build tuple, invalid amount/address/UUID, or unsupported chain/token pair. |
| `403` | Missing or incorrect shared secret. |
| `404` | Unknown `solverQuoteId` (including records lost on restart). |
| `409` | Stale allocation, duplicate liquidity domain, used deterministic nonce, second build ID, or other idempotency conflict. |
| `410` | Known confirmation expired. |
| `429` | Live confirmation store is at its fixed capacity. |
| `502` | Required RPC, backend discount, or signing dependency failed unexpectedly. |

`DISCOVERY` and `CONFIRM` distinguish normal lack of liquidity from dependency failure. They must not turn
an RPC/backend outage into an empty curve or `204`. `BUILD` never degrades an error into a smaller or
different allocation.

## Components and compatibility

Implementation stays under `internal/solvers/rfq/`: phase-tagged API types and handler registration,
a swap service, a bounded confirmation store, EIP-712/nonce helpers, and tests. Shared LiquidLane reader,
strategy, capacity, and discount helpers should be reused rather than forked. The generated
`api/bindings/liquidlane/adapter` package already exposes both required overloads and is not edited.

When `swapEnabled` is false, `/swap` is not registered and existing deployments behave exactly as before.
When enabled, the new service runs beside:

- authenticated `POST /quote` quote fan-out;
- open-order polling and reconciliation; and
- Executor-based on-chain order fills through the shared transaction manager.

The new phases do not insert records into the order store, reserve existing execution capacity, call the
Executor/Reactor, or send through `TxManager`. Existing RFQ behavior, metrics, configuration defaults, and
solver-mode discount gates remain compatible. Deployment must separately authorize the configured signer
on every adapter before enabling the feature.

The backend integration must treat `solverQuoteId`, the exact confirmation tuple, the global domain set,
and `buildId` as immutable. It is responsible for Router transaction assembly, token transfers, aggregate
minimum-output enforcement, global domain collision rejection, submission, and post-submit monitoring.

## Verification plan

### API and lifecycle

- OpenAPI and handler tests cover the single path, all phase tags, authentication, body cap, lowercase
  output normalization, decimal amounts, and phase-specific validation.
- Discovery tests pin exact requested sample echoing, cumulative/monotonic points, omitted uncovered
  samples, exact final coverage, empty liquidity, token policy, candidate dedupe, and deterministic
  tie-breaking.
- Confirmation tests pin exact coverage, scalar floor math, unique adapter domains, persisted call order,
  `CapacityID` accounting, TTL calculation, expiry, capacity limits, and restart/unknown-ID behavior.
- Build tests prove the request tuple must match, the exact input allocation and call order cannot change,
  every leg retains its own floor, upward output improvement is allowed, and failures return no partial
  calls.

### Encoding and security

- Golden EIP-712 tests independently reconstruct the domain, type hash, struct hash, digest, recovered
  signer, `v`, deadline, Router caller/recipient, and all signed amounts.
- Binding round-trip tests pin the signed-only selector `0x9a4568b6`, lowercase calldata, `to`, output
  token, and domain fields, including confirmations selected from discount inventory.
- Domain tests reject wrong chain, wrong verifying contract, unsupported fields/salt/extensions, and RPC
  failure. Authorization tests cover owner, market maker, delegated filler, unauthorized signer, and the
  fact that Router filler authorization is not required.
- Nonce tests pin the derivation byte-for-byte, stable zero-based call indexes, concurrent retries with
  fresh request IDs, field-conflicting reuse, second build IDs, per-token used-nonce checks, and no
  replacement nonce.
- Discount-selection tests prove BUILD never calls the discount resolver and always emits same-adapter
  signed calldata while retaining the confirmed floor.
- Domain tests reject duplicate adapters within one plan and backend contract tests reject intersecting
  canonical capacity-domain sets across solvers before and after build.

### Regression and operational gates

- A fake transaction manager records zero sends for every swap phase.
- Existing `/quote`, poller, Executor fill, discount fill, and reconciliation tests continue unchanged.
- Metrics distinguish phase and bounded outcome without labeling by UUID, address, calldata, or signature.
- Run `go test -race ./...`, `go test -coverprofile=coverage.out ./...`, `go build ./...`, and the repository's
  configured lint/format checks with Go 1.26.5.

## Acceptance criteria

The design is complete when an authenticated backend can discover a cumulative curve, confirm an exact
short-lived floor and globally unique adapter domains, and idempotently build the exact stored allocation
as lowercase adapter calls. Every leg is a valid Router-bound `SignedSwap`, including legs selected from
discount inventory; no private discount payload is exposed; no leg drops below its own confirmed floor;
no duplicate domain is accepted; and no new path broadcasts a transaction or changes legacy RFQ execution.
