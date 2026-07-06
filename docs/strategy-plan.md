# vault-solver — Solver-Local Strategy Architecture

This document records the strategy boundary for vault-solver. Strategy selection is intentionally
solver-local: the generic solver framework does not parse, validate, or route strategy configs.

Stage scope:

- implemented: RFQ and 3F strategy layers with `default` and `webhook`
- documented only: RedStone future strategy boundary

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
type StrategyFactory func(raw yaml.Node, deps StrategyDeps) (types.Strategy, error)
```

`default`, `webhook`, `morpho`, `aave`, or any custom local strategy may self-register from its own
package `init()` under a solver-local unique name. The solver only asks the registry for
`strategy.name`; the selected strategy owns parsing `strategy.config`.

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
  strategyregistry/       # RFQ-local strategy registry/factory
  strategies/default/   # local default strategy
  strategies/webhook/   # RFQ webhook adapter
```

Contract:

```go
type Strategy interface {
    DecideQuote(ctx, input QuoteInput) (QuoteOutput, error)
    BuildFillPlan(ctx, input FillInput) (*FillPlan, error)
}
```

`DecideQuote` is called by the quote server. `BuildFillPlan` is called at fill time. The strategy may
return a quote-time cached plan or rebuild one from the fill-time input. The strategy is a trusted
component: semantic validation, replay, cache matching, and recovery correctness live inside the
strategy implementation, not in the solver skeleton.

RFQ solver owns:

- `/quote` parsing and request validation
- chain/type/token scope checks
- adapter whitelist filtering
- quote/fill-time candidate construction
- discount resolution at fill time
- calldata, signing, and tx submission

RFQ strategy owns:

- candidate scoring
- direct/discount leg selection
- input split across legs
- quote/decline decision
- quote-time fill-plan caching
- fill-plan recovery after restart/cache miss
- strategy output validation, if the implementation wants it

Quote input:

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

Quote output:

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

Fill input/output:

```go
type FillInput struct {
    RequestID         string
    QuoteID           string
    ChainID           int64
    Executor          address
    TokenIn           address
    TokenOut          address
    AmountIn          uint256
    RequiredAmountOut uint256
    Candidates        []QuoteCandidate
    Now               time
}

type FillPlan struct {
    QuoteID         string
    RequestID       string
    TokenIn         address
    TokenOut        address
    AmountIn        uint256
    QuotedAmountOut uint256
    Legs            []FillLeg
}

type FillLeg struct {
    Adapter    address
    AmountIn   uint256
    AmountOut  uint256
    MaxRate    uint256
    DiscountID bytes32?
}
```

RFQ webhook wire format is RFQ-specific. It uses lower-camel JSON field names and decimal strings for
big integer values. The strategy input/output types provide their own JSON encoding so the webhook
adapter does not duplicate the contract.

Default strategy validation/replay:

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
- default strategy rebuilds execution records from input candidates, not from strategy-supplied adapter data.
- webhook strategy treats the remote decider as trusted and only rejects structurally unusable plans.

## 3F Strategy Boundary

Package layout:

```text
internal/solvers/bridgefacilitator/
  strategies/               # 3F-local strategy registry/factory (package strategies)
  strategies/types/         # 3F strategy input/output and interface (package types)
  strategies/default/       # local default strategy
  strategies/webhook/       # 3F webhook adapter
```

Decision:

```go
DecideOffers(ctx, input) -> output
```

The solver calls this once per discover tick after API reads, on-chain adapter reads, and live-offer
cache pruning. The solver builds a snapshot of **raw facts only** — adapter liquidity/caps, auction
facts, and the live offers it holds — and delegates every decision (selection, sizing, ordering,
dedup) to the trusted strategy. The solver computes no capacity or candidate joins.

Input:

```go
type OfferInput struct {
    Now        time
    Adapters   []AdapterSnapshot
    Auctions   []AuctionSnapshot
    LiveOffers []LiveOffer
}

type AdapterSnapshot struct {
    ID            string
    Adapter       address
    Vault         address
    Collateral    address
    Fundable      uint256 // getMaxAssets()
    OpenCount     int     // requestsLength()
    MaxAssets     uint256 // maxAssetsPerRequest, 0 = reject-all
    MinAssets     uint256 // minAssetsPerRequest, 0 = disabled
    MinYieldBps   uint256 // minYieldPerRequest converted from ppm to bps
    MaxConcurrent int      // MAX_REQUESTS
}

type AuctionSnapshot struct {
    ID              string
    AuctionID       int64
    OriginalIndex   int
    Request         address
    Status          string
    DepositAsset    address
    AmountRequested uint256
    RemainingAmount uint256
    MaxRateBps      float64
}

type LiveOffer struct {
    AdapterID string
    AuctionID int64
}
```

`LiveOffers` are the offers the solver already holds per (adapter, auction). The strategy uses them to
own duplicate-offer policy while the solver still supplies the raw live-cache facts. The solver no
longer computes per-adapter capacity; the strategy derives it from each `AdapterSnapshot`'s raw caps —
roughly `min(min(Fundable, MaxAssets), Fundable − committed)` gated by concurrency and `MinAssets`.

Output:

```go
type OfferOutput struct {
    Offers []OfferExecution
}

type OfferExecution struct {
    AuctionID      int64
    Request        address
    Maker          address
    Principal      uint256
    ExpectedReturn uint256
    Reason         string?
}
```

3F strategy owns collateral matching, live-offer dedup policy, positive/cumulative capacity checks,
per-request max/min assets, remaining auction amount, open count, max concurrent requests,
yield/return preference, ordering, offer sizing, expected return, and skip reasons. The default local
strategy implements the current most-fundable behavior and validates those invariants internally.

The 3F solver is an execution skeleton. It uses `AuctionID` to recover the raw auction EIP-712 domain,
signs the trusted strategy's execution offer, calls `createOffer`, and records the live-offer cache
after successful submission. Strategy output cannot set nonce or signature.

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
