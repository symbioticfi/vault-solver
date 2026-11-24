# Protocol fills through Merkle recipes

RFQ, UniswapX and LI.FI orders can settle through a `LiquidLaneConnector` funded by a Merkle
recipe instead of native adapters. UniswapX and LI.FI each have a separate solver for it, with
its own name and config:

| Protocol | Adapter-backed solver | Merkle solver |
|---|---|---|
| UniswapX | `uniswapx-filler` | `uniswapx-merkle` |
| LI.FI | `lifi-samechain` | `lifi-merkle` |
| RFQ | `rfq-filler` with `adapters` | `rfq-filler` with `recipeExecutors`, or the execution-only `rfq-merkle` |

A solver's name fixes its fill path. The adapter-backed UniswapX and LI.FI solvers reject
`recipeExecutors` and `curator`; the Merkle solvers have no `adapters`, `solverMode`, `gas` or
`liquidityLens` settings.

## Layout

| Package | Owns |
|---|---|
| `internal/liquidlane/connector` | Protocol-neutral connector logic: recipe route discovery and cache, signed-discount framing, recipe assembly, connector inventory and payment reads, SDK admission of the outer simulation, the RAM spend ledger |
| `internal/solvers/merkle/filler` | What every protocol filler shares: config (`executor`, `recipeExecutors`, `curator`), the signer lane on the SDK transaction manager with its single in-flight fill, advertised connector discount selection, the planner that splits a settlement across connectors, the callback-funded swap flow, quote inventory and reservations |
| `internal/solvers/merkle/rfq` | Awarded-order polling and the RFQ Executor `fill` envelope ([runbook](RFQ-MERKLE.md)) |
| `internal/solvers/merkle/uniswapx` | The `uniswapx-merkle` solver: registration and config |
| `internal/solvers/merkle/lifi` | The `lifi-merkle` solver: registration and config |
| `internal/solvers/uniswapx`, `internal/solvers/lifi` | The protocol engines both solvers of a protocol run: quoting, order intake, retries, metrics. Each engine has one `settlement` seam (`settlement.go`) with two implementations: adapters (`adapterSettlement`) and recipes (`recipeSettlement` in `recipes.go`, with the executor envelope) |

RFQ pulls awarded orders itself (`Engine.Run`). The UniswapX and LI.FI engines push each
accepted order into the lane (`Swaps.Fill`, then `Engine.AwaitNative`), so dedup, retry,
breaker and exclusivity accounting are the same for both solvers of a protocol. The engine
never branches on configuration: the registered solver passes its settlement at construction
(`NewRecipeSolver`). Retiring the adapter path of a protocol means deleting its adapter
solver's registration and config and the `adapterSettlement` implementation.

## Settlement

RFQ: the Reactor delivers the awarded input to the connector through `discountSwapInputs`
and the recipe pays the awarded output, so the connector must be registered as version `2`
in the Reactor's Liquid Lane adapter factory.

UniswapX and LI.FI: the settlement contract holds the order input inside its callback and
runs two calls for each connector the fill draws on:

1. `tokenIn.transfer(connector, amountIn)`, with that connector's share of the input;
2. `RecipeExecutor.execute(...)`, wrapped in `AccessManager.execute(recipeExecutor, ...)`
   when the recipe's caller is an AccessManager.

The recipe's signed connector swap sends the payment back to the settlement contract, which
then settles the order outputs. No adapter-factory registration is involved.

These orders carry no awarded amount, so the solver takes each connector's own payment for
the input it receives: `getAmountOut(tokenIn, amountIn) * (1e8 - discount) / 1e8`, read at the
pinned recipe block and rounded like the contract. Together the payments must cover the order
output; the surplus stays with the settlement contract. The complete settlement transaction is
simulated from the signer at that block before it is submitted.

## Required on-chain setup per protocol

- A recipe and `RecipeExecutor` whose connector swap permission pins the protocol's
  settlement contract as payment recipient. One recipe serves one settlement contract.
- The recipe's caller is the settlement contract, or an AccessManager that grants it a
  zero-delay role for `execute(bytes[],bytes[],uint256[],bytes[])` on the RecipeExecutor.
- The solver EOA is an authorized caller of the settlement contract.
- The backend advertises the connector's discounts with `discountPrecision: "100000000"`
  and a funding `maxAssets`.

## Configuration

```yaml
solvers:
  - name: uniswapx-merkle   # or lifi-merkle
    config:
      executor: "0x…"        # protocol settlement contract: recipe caller and payment recipient
      recipeExecutors: ["0x…"]
      curator:
        url: https://curator.example/api/v1
      # uniswapx-merkle: discounts.baseUrl; lifi-merkle: privateDiscountsUrl
      # plus the protocol's quote, order-server, token-scope and strategy settings
```

`recipeExecutors`, `curator.url` and the discount service are required. The protocol settings
(contracts, order and quote servers, token scope, strategy) are the same fields as in the
adapter-backed solver; the `webhook` strategy is rejected. See
[uniswapx-merkle.example.yaml](../config/uniswapx-merkle.example.yaml) and
[lifi-merkle.example.yaml](../config/lifi-merkle.example.yaml).

## Behaviour and limits

- Merkle fills use the SDK transaction manager; the native `txManager` lane is not started
  for a Merkle solver. Deploy each solver process with its own signer. A Merkle solver and the
  adapter-backed solver of the same protocol may share a process only on separate quote ports.
- Logs and metrics carry the solver's own name (`solver="uniswapx-merkle"`, `solver="lifi-merkle"`),
  including the protocol gauges such as `uniswapx_ready`, `uniswapx_pending_fills` and
  `lifi_active_quotes`. Dashboards and alerts that select `uniswapx-filler` or `lifi-samechain`
  do not include them; queries that select only a namespace add both solvers' series together.
- One fill is in flight per process. UniswapX defers the next order to its retry path and
  declines quotes until the fill settles, so it does not win an exclusive order it could only
  fill late; LI.FI queues the next order behind the pending fill like an order blocked on
  reserved capacity.
- An order settles through one connector when one can pay it. Otherwise the fill is split across
  connectors with the allocator quotes use, at fresh connector prices, in one settlement call;
  RFQ awards split the same way. UniswapX and LI.FI split across up to three connectors under
  the rules of their adapter-backed solvers, for quotes and fills alike: splitting needs the
  `all` or `permissionless` token scope, because tokens in the `permissioned` scope are
  single-route, and the UniswapX `single` strategy always uses one connector. The example
  profiles use `permissioned`, so they never split.
- Quote inventory is the advertised connector discounts priced at the connector's current
  policy (`remainingAssets`, `minDiscount`, oracle price) at one block. Capacity is the
  advertised funding clipped by the connector budget, minus the lane's pending and
  not-yet-reflected spends.
- Gas is paid by the sender; gas accounting in quotes is not supported.
- The recipe pays the exact amount priced at the simulation block. A state change between
  simulation and inclusion (an oracle update, a connector policy change, another fill spending the
  same funding) reverts the fill. The order is then planned again from fresh state: UniswapX on
  its retry path, counting the failure toward its breaker; LI.FI after re-reading the order
  status, at most twice per order.
- A pending fill is bounded by the order and discount deadlines. It is not cancelled early
  when the order becomes unfillable.
- A fill is submitted only while the order and its signed discount stay valid for
  `discounts.minimumValidity` (UniswapX) or 12 seconds (LI.FI).
- The fill decision is the connector's: the configured strategy prices quotes, but its fill
  step (minimum amount, webhook veto, LI.FI exclusive-limit context) is not consulted. An order
  the settlement contracts reject fails the simulation and is retried as a failure.
- A connector that pays too little or lacks capacity, including funding this lane spent and the
  listing does not reflect yet, is an expected decline. A failed read, an unreachable curator or
  discount service, and a lane that refuses the transaction before signing are failures: LI.FI
  retries the order for up to 30 seconds, logging the retries at debug and the final give-up as
  an error, and then leaves it to the next feed recovery; UniswapX retries on its poll interval
  and counts simulation and pre-sign refusals toward its breaker. LI.FI retries the same way an
  order whose signed discount, not the order itself, expires before a fill could land.
- A connector read that fails for the whole inventory keeps the previously published quotes,
  like a failed adapter snapshot; offers that only some connectors cannot price are dropped and
  mark the refresh degraded.
- If the transaction manager stops before a fill settles, the wait ends like a stopped native
  manager. If the sender drops a transaction without a receipt, the lane is stalled: it keeps
  the intent and its reservation, admits no fills and blocks quotes until the process is
  restarted after the transaction has been checked. RFQ keeps such an intent the same way.
- Fill outcome counters, including `fill{outcome="success"}` and its last-event timestamp, are
  recorded for recipe fills, and so is the per-pair `mined_success` fill event. Receipt-derived
  fill amounts are not.
- The active intent lives in RAM, as in [RFQ](RFQ-MERKLE.md).
