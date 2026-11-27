> ## Documentation Index
> Fetch the complete documentation index at: https://docs.iris.credit/llms.txt
> Use this file to discover all available pages before exploring further.

# API Reference

> Request and response schemas, field bounds, and error codes for the Iris RFQ coordinator.

Three wire shapes matter: what a borrower sends the coordinator, what the coordinator sends a solver, and what a solver returns. Each comes in two forms: firm (`POST /quote`), a signed quote the borrower can take, and indicative (`POST /quote/indicative`), an unsigned rate preview nothing can take.

> [!TIP]
>
> Coordinator endpoint URLs are not published. They are shared directly during onboarding — reach out through [Telegram](https://t.me/+549-WHvSTztiZTI1) or the team contact links in the docs header.

> [!WARNING]
>
> The deployed coordinator answers the browser origins of Iris's own apps only. Every request must carry an allowlisted `Origin` header; one that is missing or unrecognised returns `403` with `{"detail": "Origin not allowed"}` before the body is parsed. Server-side callers that sent no `Origin` were accepted until 2026-08-23 and are now rejected.
>
> Since 2026-08-25 the endpoint is also screened at the edge, ahead of that check. A request arriving without a browser challenge token is answered there with `202` and an HTML challenge page instead of a quote, so a scripted caller sees the challenge rather than the `403`. Only a browser running an Iris app mints the token, so neither `/quote` nor `/quote/indicative` can be driven from a script or a server.

## Conventions

* **Numerics are decimal strings**, never JSON numbers. `"1000000"`, not `1000000`. The one exception is `chainId`, which is a number.
* **Addresses are EIP-55 checksummed.** All-lowercase hex is also accepted and normalised to EIP-55 on ingress. The two rejected forms are mixed-case hex with an invalid checksum and all-caps hex.
* **Rates and LLTVs are WAD** (`1e18` = 100%). `"81000000000000000"` is 8.1%.
* **Amounts are in the token's own decimals.** No implicit scaling.
* **Times are seconds**; `deadline` is a unix timestamp.

## `POST /quote`

Submits a borrower intent and returns the best valid quote from the round.

### Request body

| Field | Type | Bounds |
| - | - | - |
| `requestId` | string | UUIDv7 |
| `chainId` | number | `1` (Ethereum) |
| `borrower` | string | EIP-55 address, non-zero |
| `receiver` | string | EIP-55 address, non-zero |
| `collateralToken` | string | EIP-55 address; must be a protocol-supported token |
| `collateral` | string | uint256, ≥ 1 |
| `debtToken` | string | EIP-55 address; must be a protocol-supported token |
| `debt` | string | uint256, ≥ 1 |
| `maxFixedRate` | string | WAD, `1e14` – `1e18` (1 bp – 100%) |
| `duration` | string | seconds, `86400` – `63072000` (1 day – 2 years) |
| `venueBitmap` | string | uint256, ≥ 1; every set bit must be a registered venue id, and every venue it names must have a market for the pair |

```json theme={null}
{
  "requestId": "0199a3f1-8c00-7000-8000-000000000000",
  "chainId": 1,
  "borrower": "0xaaccE4EC50328120eF3899a978E87FcA6B8bBE84",
  "receiver": "0xaaccE4EC50328120eF3899a978E87FcA6B8bBE84",
  "collateralToken": "0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0",
  "collateral": "1000000000000000000",
  "debtToken": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
  "debt": "1000000000",
  "maxFixedRate": "90000000000000000",
  "duration": "7776000",
  "venueBitmap": "3"
}
```

> [!NOTE]
>
> `venueBitmap` is checked venue by venue, so a bit naming a venue with no market for the pair is rejected even when another named venue could serve it. Setting the Morpho Blue bit for a pair with no Morpho Blue market returns `400` with `venueBitmap allows morphoBlue but no morphoBlue market exists for 0x…/0x…; clear that venue bit`, whether or not another venue's bit is also set. Clear the bit rather than relying on another venue to cover it.

### Response `200`

The winning quote's fields inline, plus `allQuotes`, every quote that survived validation in that round, the winner included.

```json theme={null}
{
  "requestId": "0199a3f1-8c00-7000-8000-000000000000",
  "quoteId": "0199a3f1-8c01-7000-8000-000000000000",
  "solver": "0xe05fcC23807536bEe418f142D19fa0d21BB0cfF7",
  "blm": "0xB0ffB27D9D7f160d588c669329F807e58aDd4eeF",
  "fixedRate": "81000000000000000",
  "venueId": "0",
  "data": "0x",
  "overdueRate": "162000000000000000",
  "overduePeriod": "604800",
  "bondLltv": "900000000000000000",
  "bond": "905000000",
  "deadline": "1785235562",
  "nonce": "1",
  "signature": "0x…",
  "permit2Nonce": "0",
  "permit2Signature": "0x…",
  "allQuotes": [{ "…": "same shape, one entry per surviving quote" }]
}
```

Every field is populated on the response except the permit2 pair, which appears only when the winning quote carries it; `quoteId` is generated if the solver omitted it. Responses echo the caller's origin in `Access-Control-Allow-Origin` when it is allowlisted, so only Iris's own apps can read a response from a browser.

### Errors

| Status | `errorCode` | When |
| - | - | - |
| `202` | none | Answered at the edge, not by the coordinator: the request carried no browser challenge token. The body is an HTML challenge page, not JSON |
| `400` | `VALIDATION_ERROR` | Schema failure, unsupported token, a `venueBitmap` bit outside registered venues, or a named venue with no market for the pair |
| `403` | none | The `Origin` header is missing or not allowlisted; `detail` is `"Origin not allowed"` |
| `403` | `FORBIDDEN` | Rejected by the edge abuse filters before reaching the coordinator |
| `404` | `QUOTE_ERROR` | No solver returned a valid quote; `detail` is `"No quotes available"` and `reasons` lists why solvers declined |
| `422` | `VALIDATION_ERROR` | Body was not valid JSON |
| `500` | `INTERNAL_ERROR` | Unexpected failure |

```json theme={null}
{
  "detail": "No quotes available",
  "errorCode": "QUOTE_ERROR",
  "id": "0199a3f1-8c00-7000-8000-000000000000",
  "reasons": [{ "venue": "AaveV3", "reason": "reserve_frozen" }]
}
```

A `404` is a normal outcome, not a fault: it means every solver declined, timed out, or had its quote dropped in validation.

`reasons` collects the decline reasons solvers gave in their own `404` bodies (see [Declining with reasons](#declining-with-reasons)), with each venue and reason pair listed once across solvers. Where several solvers gave a `rate` for the same pair, the lowest one is kept, since that is the rate the borrower could get. It is an empty array when no solver gave a reason, so it does not account for solvers that timed out or had their quote dropped. `errorCode` and `detail` are unchanged, so callers that only check those keep working.

## `POST /quote/indicative`

Returns a rate preview before the borrower has connected a wallet. The round runs like `POST /quote`, but only solvers whose registry entry sets an `indicativeEndpoint` are called, and they answer unsigned. Nothing this route returns can be submitted to `Iris.take()`: to borrow, request a firm quote from `POST /quote`.

### Request body

The `POST /quote` body with `borrower` and `receiver` optional. When given, each must still be an EIP-55 address, non-zero. Every other field keeps its bounds, and the same request checks run before any solver is called: supported tokens, registered venues, and a market for the pair on every named venue.

```json theme={null}
{
  "requestId": "0199a3f1-8c00-7000-8000-000000000000",
  "chainId": 1,
  "collateralToken": "0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0",
  "collateral": "1000000000000000000",
  "debtToken": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
  "debt": "1000000000",
  "maxFixedRate": "90000000000000000",
  "duration": "7776000",
  "venueBitmap": "3"
}
```

### Response `200`

The winning quote's terms inline, plus `allQuotes`, every indicative quote that survived validation. The shape is the `POST /quote` response with `indicative: true` added and without `quoteId`, `deadline`, `nonce`, `signature` or the permit2 pair.

```json theme={null}
{
  "requestId": "0199a3f1-8c00-7000-8000-000000000000",
  "indicative": true,
  "solver": "0xe05fcC23807536bEe418f142D19fa0d21BB0cfF7",
  "blm": "0xB0ffB27D9D7f160d588c669329F807e58aDd4eeF",
  "fixedRate": "81000000000000000",
  "venueId": "0",
  "data": "0x",
  "overdueRate": "162000000000000000",
  "overduePeriod": "604800",
  "bondLltv": "900000000000000000",
  "bond": "905000000",
  "allQuotes": [{ "…": "same shape, one entry per surviving quote" }]
}
```

### Errors

The same as [`POST /quote`](#errors), including the edge `202`, the origin `403`, and the `404` with `reasons` when no solver returned a valid quote.

## Solver webhook

What the coordinator POSTs to a registered solver's `endpoint`.

### Request

Identical to the `POST /quote` body, plus a `quoteId` minted for this call. Each solver in a round receives a different `quoteId` and the same `requestId`.

| Field | Type | Notes |
| - | - | - |
| `quoteId` | string | UUIDv7, injected per solver |
| *all request-body fields* | | Passed through unchanged |

### Response

Return `200` with the body below, or decline with `404` / `bond: "0"` (a `404` may say why, see [Declining with reasons](#declining-with-reasons)). The permit2 pair is optional together: omit both fields to fund the bond from your standing ERC-20 allowance to `Iris`, or supply both to stage the Permit2 fallback. Half a pair is rejected.

| Field | Type | Bounds |
| - | - | - |
| `requestId` | string | UUIDv7; must match the request |
| `quoteId` | string | UUIDv7, optional (generated if omitted) |
| `solver` | string | EIP-55 address, non-zero; the EOA whose key signed, or the deployed ERC-1271 wallet that accepts the signature. Must match your registered signing address |
| `blm` | string | EIP-55 address, non-zero; must be enabled on `Iris` |
| `fixedRate` | string | WAD, `1e14` – `1e18`; must be ≤ `maxFixedRate` |
| `venueId` | string | `0` – `127`; a venue **index**, and its bit must be set in `venueBitmap` |
| `data` | string | hex; must be an enabled market-data payload |
| `overdueRate` | string | WAD, `0` – `1e18`; added to `fixedRate` after maturity |
| `overduePeriod` | string | seconds, `0` – `2592000` (30 days) |
| `bondLltv` | string | WAD, `0` – `1e18`; must be enabled on `Iris` |
| `bond` | string | uint256; `"0"` declines |
| `deadline` | string | unix seconds; within the 2-minute quote TTL |
| `nonce` | string | uint256, ≥ 1 |
| `signature` | string | hex over the on-chain `Quote`, 65 bytes to 4 KiB: 65-byte `r \|\| s \|\| v` from an EOA, or what your ERC-1271 wallet's `isValidSignature` accepts |
| `permit2Nonce` | string | Optional, only alongside `permit2Signature`; uint48, `0` – `281474976710655`; `0` is valid |
| `permit2Signature` | string | Optional, only alongside `permit2Nonce`; hex over the Permit2 `PermitSingle`, in the same form as `signature` |

> [!WARNING]
>
> The `deadline` window is tight: it is accepted only within the final \~5 seconds before the 2-minute TTL expires. Set `deadline` to **exactly 120 seconds ahead of your own clock** at signing time. The slack exists to absorb the trip back to the coordinator, not to allow shorter-lived quotes.
>
> The window is measured when your response reaches the coordinator. If you fix `deadline` before a slow step, such as an on-chain transaction that makes an ERC-1271 signature valid, set it about 120 seconds after the moment you expect your response to arrive, not after the moment you fixed it.

Signature construction is covered in [Get Started](https://docs.iris.credit/rfq/running-a-solver/get-started.md#3-sign-the-quote); the checks these fields face are in [Validation](https://docs.iris.credit/rfq/running-a-solver/validation.md).

### Declining with reasons

A `404` decline may carry a body saying why you declined. The coordinator passes the reasons on to the borrower in its own `404` (see [Errors](#errors)).

```json theme={null}
{
  "reasons": [
    { "venue": "AaveV3", "reason": "reserve_frozen" },
    { "venue": "MorphoBlue", "reason": "rate_above_max_fixed_rate", "rate": "46200000000000000" }
  ]
}
```

| Field | Type | Bounds |
| - | - | - |
| `reasons` | array | At most 16 entries |
| `reasons[].venue` | string | Venue name, 1 to 32 letters or digits |
| `reasons[].reason` | string | snake\_case code: starts with a lowercase letter, then lowercase letters, digits or `_`, at most 64 characters |
| `reasons[].rate` | string | Optional. The venue's rate, WAD as a decimal string, from 1 bp (`100000000000000`) to 100% (`1000000000000000000`). Send it on a `rate_above_max_fixed_rate` decline so the borrower learns the rate they could ask for |

The body is optional, and only a `404` is read for it; a `bond: "0"` decline carries no reasons. A list that breaks any bound is ignored as a whole, never treated as an error: the decline still counts as a decline.

### Indicative requests

For `POST /quote/indicative`, the coordinator POSTs to your entry's `indicativeEndpoint` instead, with the same `headers`, response budget and scoping filters as `endpoint`. Entries without an `indicativeEndpoint` are not called. The request is the indicative request body plus a `quoteId`; `borrower` and `receiver` are left out, not sent as `null`, when the caller gave none.

Return `200` with an unsigned quote, or decline exactly as for a firm request. The response takes the firm response's fields from `requestId` to `bond` with the same bounds, leaves out `deadline`, `nonce`, `signature` and the permit2 pair, and adds one field:

| Field | Type | Bounds |
| - | - | - |
| `indicative` | boolean | Must be `true`; a response without it is dropped |

`solver` still has to match your registered address, although nothing is signed. The coordinator does not read your balance or allowances for an indicative quote, so decline with a `404` when you could not fund the bond a firm quote would need, for example `{"reasons": [{"venue": "solver", "reason": "insufficient_bond_balance"}]}`.

## Registry entry

The shape of one registry entry; an organisation registers one entry per endpoint. Entries are created and updated by the team during onboarding, not self-service; see [Get Started](https://docs.iris.credit/rfq/running-a-solver/get-started.md#1-get-registered).

| Field | Type | Required | Notes |
| - | - | - | - |
| `name` | string | Yes | Per-entry label in logs and per-solver metrics; distinct per entry |
| `endpoint` | string | Yes | HTTPS URL receiving quote requests |
| `indicativeEndpoint` | string | No | URL receiving indicative requests; see [Indicative requests](#indicative-requests). Omit to receive none |
| `hash` | string | Yes | Opaque 32-byte organisation identifier, assigned by the team; shared across an organisation's entries |
| `address` | string | Yes | Solver signing address; responses whose `solver` differs are dropped |
| `headers` | object | No | Sent verbatim on every request; the hook for authenticating the caller |
| `overrides.timeout` | number | No | Response budget in milliseconds; default `5000`, at most `60000` |
| `chainIds` | number\[] | No | Restrict to specific chains. Omit for all |
| `supportedTokens` | string\[] | No | Restrict by **debt** token, EIP-55. Omit for all |
| `supportedVenues` | string\[] | No | Restrict by venue name, `aaveV3`, `morphoBlue` or `sparkLend`; matched against the request's `venueBitmap`. Omit for all |

```json theme={null}
{
  "name": "example-solver",
  "hash": "0x0000000000000000000000000000000000000000000000000000000000000001",
  "address": "0xe05fcC23807536bEe418f142D19fa0d21BB0cfF7",
  "endpoint": "https://solver.example.com/rfq",
  "indicativeEndpoint": "https://solver.example.com/rfq/indicative",
  "headers": { "x-api-key": "…" },
  "overrides": { "timeout": 5000 },
  "chainIds": [1],
  "supportedTokens": ["0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"],
  "supportedVenues": ["aaveV3", "morphoBlue"]
}
```

Changes are applied by the team and normally propagate within 5 minutes. A failed refresh leaves the cached registry in place until the next interval, so a change occasionally takes longer.


This documentation is built and hosted on [Mintlify](https://mintlify.com), a developer documentation platform.