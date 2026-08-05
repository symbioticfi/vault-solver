# Per-Leg SWAP Authorization

## Goal

Make RFQ `BUILD` preserve the authorization source selected for each confirmed LiquidLane leg:

- a direct leg returns solver-signed `SignedSwap` calldata; and
- a discount leg returns calldata containing the exact resolved signed discount.

One solver response may contain both call types. The confirmed leg order, adapters, input splits,
output asset, and liquidity domains remain immutable.

This design supersedes the signed-only treatment of discount-selected legs in
`2026-08-03-user-directed-swap-calldata-design.md`. Implementation must update that document and the
operator documentation so they no longer claim that every leg is converted to `SignedSwap`.

## Authorization Selection

Authorization is selected per persisted `FillLeg`, not per deployment or response:

```text
leg.DiscountID == nil  -> direct SignedSwap
leg.DiscountID != nil  -> resolved DiscountSwap
```

The existing confirmation record is sufficient. `FillLeg.DiscountID` is already copied from the
selected candidate and deep-copied by the swap store, so no new persistence field or wire field is
required.

Direct and discount variants of one physical route remain alternatives. Existing plan validation
continues to reject repeated physical routes, while a valid multi-route plan may mix authorization
types across different adapters.

## Confirmation

`CONFIRM` continues to re-read liquidity, re-run the strategy, preserve capacity domains, and persist
the exact selected plan.

Only direct legs require the framework signer to be authorized by their adapters. The solver reads and
validates EIP-712 domains for those direct adapters through the existing adapter-authorization path.
Discount legs do not require framework-signer permission because their authorization is the persisted
discount ID and the signatures resolved later during `BUILD`.

Confirmation does not resolve or store signed discount payloads. It uses the advertised discount
candidate and its validity horizon. A fresh payload is resolved only during the first uncached build.

Configured adapters retain the existing startup signer-authorization check because configured adapter
inventory is the solver's direct-liquidity surface. Dynamically advertised discount adapters are not
added to that startup requirement.

## Build Flow

The existing build lease, economic fingerprint, concurrency serialization, and immutable response cache
remain unchanged. The first uncached `BUILD` performs the following work atomically:

1. Partition the persisted plan into direct and discount legs without changing order.
2. Revalidate signer authorization and EIP-712 domains for direct adapters only.
3. Re-read the exact physical adapter quote for every persisted route and input split.
4. Construct a per-leg authorization result:
   - direct: validate the fresh output against the persisted floor and prepare a deterministic solver
     nonce;
   - discount: resolve the exact persisted discount ID, parse both signatures, validate the resolved
     identity and route, validate both deadlines, and calculate the fresh discounted output from the
     physical quote.
5. Re-run the shared-capacity checks using each leg's fresh executable output.
6. Batch-read nonce state:
   - direct legs check their deterministic signed-swap nonce; and
   - discount legs check the nonce carried by the resolved discount to detect prior invalidation.
7. Encode every call in persisted order. Any failure aborts the complete build; no partial call list or
   signature is returned.
8. Verify the fresh aggregate output is at least the confirmed aggregate floor, then cache the complete
   opaque call payload.

Retries with the same build ID and economic fingerprint return byte-identical cached calls and do not
resolve discounts, read nonces, or create signatures again. A failed first attempt remains retryable only
under the already-bound build ID and fingerprint. A different build ID or changed economic tuple remains
a conflict.

## Direct Call Encoding

Direct legs retain the existing EIP-712 value:

```text
SignedSwap(
  recipient = router,
  tokenIn   = confirmed tokenIn,
  amountIn  = persisted leg amountIn,
  amountOut = fresh executable amountOut, never below the persisted leg floor,
  caller    = router,
  signer    = framework signer,
  nonce     = keccak256(buildId, chainId, adapter, tokenIn, persisted call index),
  deadline  = exact BUILD deadline
)
```

The generated binding encodes `swap(SignedSwap,bytes)` through `TryPackSwap1`. Its selector remains
`0x9a4568b6`. The persisted call index is the position in the complete mixed plan, not the position among
direct legs.

## Discount Call Encoding

For a discount leg, the solver resolves `leg.DiscountID` through the existing
`liquidlane/discounts.Provider`. The parsed payload must match:

- the persisted discount ID;
- the persisted adapter;
- the confirmed input token;
- the persisted input split; and
- the confirmed output token through the exact physical quote.

The current signed discount must satisfy the adapter's current minimum discount and produce an output at
least equal to the persisted leg floor. That calculated output becomes the call's response `amountOut`
and participates in aggregate and capacity checks.

The generated binding encodes:

```text
swap(
  DiscountSwap(
    Discount(
      tokenToRedeem,
      discount,
      signer,
      protocol,
      nonce,
      deadline
    ),
    signerSignature,
    protocolDeadline
  ),
  protocolSignature,
  recipient = router,
  amountIn  = persisted leg amountIn
)
```

through `TryPackSwap0`. Its selector is `0x8fa5c671`. The call target remains the persisted adapter. The
framework signer, adapter EIP-712 domain, deterministic signed-swap nonce, and `SignHash` are not used for
that leg.

Both the discount deadline and protocol deadline must be strictly later than the exact BUILD deadline.
The solver does not silently shorten the chosen deadline, replace the discount, or fall back to direct
solver signing. A fresh payload that cannot cover the chosen execution window makes the confirmation
stale.

## Response and Downstream Boundaries

The `BUILD` response shape is unchanged. Each call still returns:

```json
{
  "to": "0x...",
  "data": "0x...",
  "amountIn": "...",
  "amountOut": "...",
  "tokenOut": "0x...",
  "liquidityDomain": "capacity:...",
  "validUntil": 0
}
```

`validUntil` remains the exact BUILD deadline. The backend continues to validate the response envelope,
adapter allowlist, accounting metadata, domains, totals, and aggregate output floor. It maps only `to`,
`amountIn`, and opaque `data` into the Router's `(adapter, amountIn, data)` tuple.

No backend or Router source change is required. Both already accept byte-aligned opaque calldata, and
their tests cover the real discount selector. The Router verifies factory registration, transfers the
declared input to the adapter, forwards the calldata, and transfers declared outputs. It does not decode
or select between adapter entrypoints.

## Failure Mapping

- `409 Conflict`: the resolved discount ID, adapter, token, deadlines, nonce state, current minimum
  discount, current route, capacity, or output floor no longer matches the confirmed leg.
- `502 Bad Gateway`: the discount provider is unavailable, returns an empty or malformed payload, or
  the generated binding cannot encode the validated payload.
- Existing direct authorization, signing, stale-state, and store errors retain their current mapping.

All errors remain sanitized at the HTTP boundary. A mixed build is all-or-nothing.

## Accepted Discount-ABI Limitation

The existing LiquidLane discount signatures do not bind the outer `recipient` or `amountIn` arguments,
and executing `DiscountSwap` does not consume its nonce. The payload is therefore a bearer authorization
that can be copied, replayed, or repacked with different outer arguments until its signed deadlines,
subject to adapter state and capacity. Publishing it in a user transaction also exposes it on-chain.

This limitation is explicitly accepted for this implementation. Packing the configured Router and exact
persisted input split expresses the intended transaction but does not cryptographically bind them.
Checking the discount nonce detects prior invalidation only; it does not make the payload single-use.

Changing the LiquidLane discount EIP-712 types or adapter entrypoint to bind caller, recipient, and amount
and consume a nonce would be a separate contract/protocol project. This implementation does not add
selector decoding to the backend or Router as a substitute for that missing binding.

## Tests and Documentation

Vault-solver coverage must demonstrate:

- direct-only BUILD still returns valid `0x9a4568b6` calldata;
- discount-only BUILD returns `0x8fa5c671` calldata containing the exact resolved terms, both signatures,
  Router recipient, and persisted amount;
- a mixed plan preserves call order and uses the full persisted index for direct nonce derivation;
- only direct adapters require framework-signer authorization and EIP-712 domains;
- discount resolution happens once across cached and concurrent retries;
- provider failures, malformed payloads, identity mismatches, deadline mismatch, invalidated nonce, stale
  physical state, and below-floor output fail with the intended status;
- one failed leg returns no partial payload; and
- existing build-ID and fingerprint idempotency remains intact.

Update `README.md`, `docs/RFQ-PLAN.md`, the original user-directed swap design, the completed historical
implementation plan where it describes the resulting behavior, the example configuration comments, and
documentation contract tests. Backend and Router tests should remain unchanged and green.

## Out of Scope

- changing the LiquidLane discount signature or adapter ABI;
- making discount authorizations single-use;
- decoding adapter selectors or arguments in the backend or Router;
- changing the public BUILD response shape or protocol version;
- changing quote selection, solver selection, or cross-solver routing; and
- changing the legacy order-filling path.
