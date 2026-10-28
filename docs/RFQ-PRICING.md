# RFQ pricing for native adapters and Merkle connectors

The default RFQ solver can serve prices for a separately operated filler with
`quoteOnly: true`. This starts its normal authenticated `/quote` API and actual
strategy, but does not poll orders, enter an execution journal, or submit fills.
Set `executor` to the separately operated filler's identity. The default is false,
which preserves the native adapter solver's combined pricing and filling service.

The backend may offer an explicitly configured LiquidLane connector with
`kind: "connector"` and `discountPrecision: "100000000"`. This requires a signed
`discountId`. The pricing reader checks the canonical `LiquidLaneConnector` / `1`
domain and current account, output asset, decimals, minimum discount, canonical
`getMaxAssets` capacity and `getAmountOut` at a pinned block. An empty connector input-token
balance is admissible because Reactor delivers that token in the atomic outer
fill. Funding follows the on-chain recipe rather than a separate source configuration.
Native adapter inventory keeps its existing read path
and ppm precision.

Connector signed discount integers retain all digits. Exact candidates preserve
oracle price and discount separately, reproduce both contract floors, and invert
those floors when applying capacity. The webhook strategy input adds optional
`price`, `discount`, and `discountPrecision` decimal strings and `inputDecimals`;
`assetDecimals` remains the output-token scale. When present, a remote strategy
must calculate `floor(floor(amountIn * price * 10^assetDecimals /
(1e18 * 10^inputDecimals)) * (discountPrecision - discount) /
discountPrecision)` rather than approximating from `maxRate`. The solver validates
returned amounts against its exact candidates.

Signed connector terms are executed by `rfq-merkle` with the actual backend Order
and DiscountSwap protocol signatures. A native Executor cannot act as the connector's
account. Configure the backend solver's `adapters` catalog to include the connector
only for its capable Merkle filler, and run the associated default pricing service
with `quoteOnly: true` so both services do not race to fill the same award.

Merkle Liquid Lane routes read `getMaxAssets(tokenIn)` from the connector at the
quote block. The connector combines its budget with the linked RWA recipe's
rollback-only funding simulation. The pricing solver does not reconstruct funding
from balances or configured source lists. Routes still share reservations by account
and payment asset; a quoted capacity is not a reservation or a full-fill simulation.
