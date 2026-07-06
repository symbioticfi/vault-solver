# vault-solver — Solver-Local Strategy Architecture

This document records the strategy boundary for vault-solver. Strategy selection is intentionally
solver-local: the generic solver framework does not parse, validate, or route strategy configs.

Stage scope:

- implemented: RFQ strategy layer with `default` and `webhook`
- documented only: 3F and RedStone future strategy boundaries

## Common Code

The common framework still owns only solver lifecycle:

```yaml
solvers:
  - name: rfq-filler
    config:
      strategy:
        name: default
        config: {}
      backendUrl: ${RFQ_BACKEND_URL}
```

`solvers[].config` is opaque to the framework. Each solver decides whether it supports a strategy
field and which strategy names exist. The solver-level strategy factory only routes by `name`; the
selected strategy implementation owns parsing and validating its own `config` node.

The common shape inside a solver config is:

```go
type StrategySpec struct {
    Name   string
    Config yaml.Node
}
```

Each solver keeps a local registry/factory:

```go
type StrategyFactory func(raw yaml.Node, deps StrategyDeps) (strategytypes.Strategy, error)
```

`default`, `webhook`, `morpho`, `aave`, or any custom local strategy may define a different typed
config. The solver must not know those fields unless the solver itself uses them.

The only shared strategy-adjacent package is `internal/webhook`:

- HTTP JSON `POST`
- timeout
- request/response body byte limits
- literal/env-backed headers
- strict response decode
- non-2xx and empty body errors

`internal/webhook` is only a transport client. It has no solver names, no strategy registry, and no
RFQ/3F/RedStone DTOs.

Webhook strategy config:

```yaml
strategy:
  name: webhook
  config:
    url: https://strategy.example.com/rfq
    timeout: 500ms
    maxRequestBytes: 1048576
    maxResponseBytes: 1048576
    headers:
      authorization:
        env: STRATEGY_AUTH_HEADER
```

The size limits cap JSON request/response bodies. Omitted limits default to 1 MiB each.

## RFQ

Package layout:

```text
internal/solvers/rfq/
  strategytypes/          # RFQ strategy input/output and interface
  strategies/default/   # local default strategy
  strategies/webhook/   # RFQ webhook adapter
```

Decision:

```go
DecideQuote(ctx, input) -> output
```

Both quote and recovery use the same decision. Recovery only adds `RequiredAmountOut`.

RFQ solver owns:

- `/quote` parsing and request validation
- chain/type/token scope checks
- adapter whitelist filtering
- quote/recovery candidate construction
- token decimals reads for local strategy pricing and solver replay validation
- pricing dependency exposed to local strategies when they need extra reads
- output validation and replay
- storage of execution records
- discount resolution at fill time
- calldata, signing, and tx submission

RFQ strategy owns:

- candidate scoring
- direct/discount leg selection
- input split across legs
- quote/decline decision

Input:

```go
type QuoteInput struct {
    RequestID         string
    QuoteID           string
    ChainID           int64
    Executor          address
    TokenIn           address
    TokenOut          address
    AmountIn          uint256
    RequiredAmountOut uint256?  // recovery only
    Candidates        []QuoteCandidate
    Now               time
}

type QuoteCandidate struct {
    ID            string
    Adapter       address
    Asset         address
    AssetDecimals int
    MaxAssets     uint256
    MaxRate       uint256
    DiscountID    bytes32?
}
```

Output:

```go
type QuoteOutput struct {
    Decision        Decision // quote | decline
    Reason          string?
    QuotedAmountOut uint256?
    Legs            []QuoteLeg
}

type QuoteLeg struct {
    CandidateID string
    AmountIn    uint256
    AmountOut   uint256
}
```

RFQ webhook wire format is RFQ-specific. It uses lower-camel JSON field names and decimal strings for
big integer values. The strategy input/output types provide their own JSON encoding so the webhook
adapter does not duplicate the contract.

Validation/replay:

- `decline` must not include quote data.
- `quote` must include positive `quotedAmountOut` and at least one leg.
- every leg must reference an input candidate.
- duplicate candidate use is rejected.
- candidate asset must equal `tokenOut`.
- leg `amountIn` and `amountOut` must be positive.
- leg `amountOut` must not exceed candidate `maxAssets`.
- leg `amountOut` must be achievable under candidate `maxRate`.
- sum of leg `amountIn` must equal input `amountIn`.
- sum of leg `amountOut` must equal `quotedAmountOut`.
- recovery rejects `quotedAmountOut < requiredAmountOut`.
- solver rebuilds execution records from input candidates, not from strategy-supplied adapter data.

## 3F Future Boundary

Decision:

```go
DecideOffers(ctx, input) -> output
```

The solver should call this once per discover tick after API reads, on-chain adapter reads, live-offer
cache pruning, and hard filtering.

Input:

```go
type OfferInput struct {
    Now        time
    Adapters   []AdapterSnapshot
    Auctions   []AuctionSnapshot
    Candidates []OfferCandidate
}

type AdapterSnapshot struct {
    ID                   string
    Adapter              address
    Vault                address
    Collateral           address
    Fundable             uint256
    OutstandingPrincipal uint256
    OpenCount            uint64
    PerRequestMax        uint256
    TotalMax             uint256
    MinYieldBps          uint256
    MaxConcurrent        uint64
}

type AuctionSnapshot struct {
    ID              string
    OriginalIndex   int
    Request         uint64
    Status          string
    DepositAsset    address
    AmountRequested uint256
    RemainingAmount uint256
    MaxRateBps      uint256
}

type OfferCandidate struct {
    ID        string // adapterID + ":" + auctionID
    AdapterID string
    AuctionID string
    Capacity  uint256
}
```

`OfferCandidate` is only the join between one adapter snapshot and one auction snapshot that passed
hard filtering. It intentionally does not duplicate `AmountRequested`, `RemainingAmount`, `MaxRateBps`,
`MinYieldBps`, fundable liquidity, or caps; those live in `Adapters` and `Auctions`. This avoids drift
inside one input snapshot.

`Capacity` is the maximum principal the solver can safely offer for that adapter-auction pair at this
snapshot, before strategic allocation across multiple candidates. It is computed from hard constraints,
roughly:

```text
min(adapter.Fundable,
    adapter.PerRequestMax,
    adapter.TotalMax - adapter.OutstandingPrincipal,
    auction.RemainingAmount)
```

No candidate is produced when hard constraints already fail, for example incompatible collateral,
invalid auction status, existing live offer from this solver for the same adapter-auction pair,
non-positive capacity, exhausted concurrency, or below-min-yield when configured as a hard policy.

Output:

```go
type OfferOutput struct {
    Offers []OfferDecision
    Reason string?
}

type OfferDecision struct {
    CandidateID    string
    Principal      uint256
    ExpectedReturn uint256
    Reason         string?
}
```

3F solver owns hard filtering and replay: auction status, collateral match, live adapter-auction
offers, positive per-candidate capacity, cumulative fundable liquidity, per-request cap, total cap,
remaining auction amount, open count, max concurrent loans, signing, submission, and offer cache
updates. The strategy owns ordering, offer sizing, yield/return preference, and skip reasons. Replay
must still verify cumulative limits because several candidates for the same adapter can each have
`Capacity == adapter.Fundable`, while only the combined selected principal must fit.

## RedStone Future Boundary

Decision:

```go
DecideBid(ctx, input) -> output
```

The solver should call this per RedStone auction frame. Strategies can have their own discovery
state, for example a default Morpho strategy.

Input:

```go
type BidInput struct {
    Now     time
    Context BidContext
    Auction AuctionSnapshot
    Adapter AdapterSnapshot
}

type BidContext struct {
    ChainID               uint64
    Executor              address
    ExecutorNativeDeposit uint256
    Callback              address
    CallbackNativeBalance uint256
}

type AuctionSnapshot struct {
    ID       string
    Deadline time
    Prices   []PriceUpdate
}

type AdapterSnapshot struct {
    Adapter    address
    Vault      address
    Collateral address

    // State read by the solver from the adapter/vault/backend before the bid decision.
    AvailableAssets uint256
    MaxAssets       uint256
    MaxRate         uint256
    Redeemable      uint256
}

```

Output:

```go
type BidOutput struct {
    Decision      Decision // bid | skip
    Reason        string?
    BidAmount     uint256?
    OperationData bytes?
}
```

RedStone is planned as single-adapter in this stage, so the input carries one `AdapterSnapshot`, not an
adapter list. `BidContext` holds addresses and native balances/deposits that describe the execution
envelope; auction and adapter snapshots stay focused on protocol state. Supported operation kinds are
not part of the input in this stage: the solver knows the callback/executor envelope and validates
returned `OperationData` before signing. The solver owns WebSocket lifecycle, auction deduplication,
deadlines, adapter reads, executor/callback envelope constraints, signing, WS send, and final
validation. Strategy owns opportunity scoring, bid sizing, operation data construction, and skip
reason.
