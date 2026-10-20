# RFQ orders through Merkle recipes

`rfq-merkle` is an independent solver in `internal/solvers/rfq-merkle`.
It polls awarded `/api/v1/orders?filler=…&orderStatus=open`, obtains signed terms
from the backend discounts API, and builds:

`EOA → RFQ Executor → RecipeExecutor → MerkleAccount → LiquidLaneConnector`.

Use [the example config](../config/rfq-merkle.example.yaml). The EOA must be an
authorized caller of the RFQ Executor. Each configured connector must be registered
in the Reactor's factory. Its RecipeExecutor must name the RFQ Executor as caller
and the connector's MerkleAccount as account.

Configuration contains only the backend URL, durable state directory, RFQ executor,
curator service and 1..256 connector-to-RecipeExecutor mappings. Duplicate connector
addresses are rejected, including checksum variants. Polling is every three seconds;
each iteration has a thirty-second context and fetches at most 100 open orders.
Curator HTTP limits are ten seconds and 2 MiB. Secret configuration contains
environment variable names. This solver has no quote server or adapter inventory.
Use a separate RFQ Executor/filler identity for the adapter solver; two pollers must
not compete for the same awarded orders.

The signed order tuple supplies the entire input and sum of output payments.
Matching advertised connectors are tried in address/discount-ID order, and one must
cover the complete award using both its advertised capacity and pinned on-chain
budget. No market pricing, splits or adapter/Merkle liquidity aggregation occurs.
An upstream source must award compatible orders; this solver does not publish quotes.

The SDK discovers the committed recipe and active root, authenticates the curator
tree, verifies masks/proofs and roles, and checks the canonical snapshot around
simulation. The host fills only permitted runtime bytes. The signed swap pins the
connector, selector and RWA, binds amountIn to input 0, and requires the committed
account cash, RFQ payment and RWA balance deltas. Input 1 is the exact awarded output.
It simulates the complete outer RFQ fill from the EOA so Reactor funding, recipe
authorization and output settlement are checked together.

The host preserves upstream discount integers and signature bytes. It checks ABI
framing and route identity; signatures and discount economics are enforced by the
contracts. It does not translate discount scales or verify EIP-712 signatures locally.
The current canonical `LLExecuting` recipe requires an exact payout, so an order
whose required output excludes connector surplus remains unsupported. Multiple
output tokens and split connector routes also remain unsupported.

Only one fill is admitted at a time. Its intent is fsynced before admission; the
SDK journal persists each signed original and replacement before broadcast through
the shared txmanager. The earlier order/discount deadline bounds unsigned sending
and replacement. Signed recovery restores the lane fence during construction,
reconciles the original bytes without fetching new terms, and releases ownership
only after canonical confirmed inclusion or revert is acknowledged. A prepared
unsigned intent is rebuilt from fresh orders on restart. Mount an exclusive durable
directory, and never run independent processes with the same signer.

The RFQ bridge ABI comes from [RFQ PR 25](https://github.com/symbioticfi/rfq/pull/25),
commit `4c85c1b76d44ec6e2ee249528d69891799c9108d`. Connector and recipe semantics
come from adapters commit `d506348870eb2d379ddae69407bc8b63422291d6`.
`make refresh-merkle-abi` projects only solver-used entries from canonical Foundry
artifacts; `make bindings` regenerates typed helpers. No runtime contract changes
are required by this integration.

Tests cover signed award sizing, route capacity, unchanged recipe/permissions,
complete outer simulation, revoked roles, reorgs, intent persistence, sequential
admission and recovery without new API terms. Local fixtures do not establish an
authenticated live backend, deployed bridge or mainnet execution.
