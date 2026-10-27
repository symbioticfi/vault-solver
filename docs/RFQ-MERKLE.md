# RFQ orders through Merkle recipes

`rfq-merkle` is an independent solver in `internal/solvers/merkle/rfq`.
It polls awarded `/api/v1/orders?filler=…&orderStatus=open`, obtains signed terms
from the backend discounts API, and builds:

`EOA → RFQ Executor → RecipeExecutor → MerkleAccount → LiquidLaneConnector`.

Use [the example config](../config/rfq-merkle.example.yaml). The EOA must be an
authorized caller of the RFQ Executor. Each configured connector must be registered
as version `2` in the Reactor's Liquid Lane adapter factory (version `1` is the
native adapter). Its RecipeExecutor must name the RFQ Executor as caller
and the connector's MerkleAccount as account.

Configuration contains only the backend URL, durable state directory, RFQ executor,
curator service and 1..256 connector entries. Each entry maps 1..64 RWA assets
to their dedicated RecipeExecutors. Every executor is unique across the configuration;
there is no shared custody executor or asset fallback. A recipe that requires issuer
redemption includes those calls for its asset. Duplicate connector and asset addresses
are rejected, including case variants. Polling is every three seconds;
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
connector, selector and one RWA, binds amountIn to input 0, and requires the committed
account cash, RFQ payment and RWA balance deltas. Input 1 is the exact awarded output.
The recipe contains exactly one signed connector swap and only these two upstream
inputs. `LLExecuting` retains acquired inventory when its redemption leaves are empty;
otherwise its configured issuer calls execute before the complete outer fill succeeds.
It simulates the complete outer RFQ fill from the EOA so Reactor funding, recipe
authorization and output settlement are checked together.

The host puts the awarded connector amount and unchanged signed terms in
`discountSwapInputs`. It forwards the SDK-assembled `RecipeExecutor.execute`
calldata in the original `executorData` `Call[]`; no RFQ-specific recipe tuple or
funding reconstruction is used.

The host preserves upstream discount integers and signature bytes. It checks ABI
framing and route identity; signatures and discount economics are enforced by the
contracts. It does not translate discount scales or verify EIP-712 signatures locally.
The permission reserves a bounded buffer for the EOA/ERC-1271 discount signer and
a fixed 65-byte slot for the backend EOA protocol countersignature. The packer
keeps the actual signature bytes and ABI lengths, then the SDK verifies the leaf,
mask and proof. The signer length mask uses low bits for a `2^n - 1` capacity;
the current Curator profile uses 4,095 bytes. An oversized signature, malformed
offset, mutable protocol length or changed fixed field fails assembly.
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
commit `de6dbd617bf3c1e8b4120419cbe5800efae7d410`. Connector and recipe semantics
come from adapters commit `d506348870eb2d379ddae69407bc8b63422291d6`.
`make refresh-merkle-abi` projects only solver-used entries from canonical Foundry
artifacts; `make bindings` regenerates typed helpers. No runtime contract changes
are required by this integration.

Tests cover signed award sizing, route capacity, unchanged recipe/permissions,
complete outer simulation, revoked roles, reorgs, intent persistence, sequential
admission and recovery without new API terms. Local fixtures do not establish an
authenticated live backend, deployed bridge or mainnet execution.
