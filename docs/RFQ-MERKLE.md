# RFQ orders through Merkle recipes

`rfq-merkle` is an independent solver in `internal/solvers/merkle/rfq`.
It polls awarded `/api/v1/orders?filler=…&orderStatus=open`, obtains signed terms
from the backend discounts API, and builds:

`EOA → RFQ Executor → RecipeExecutor → MerkleAccount → LiquidLaneConnector`.

Use [the example config](../config/rfq-merkle.example.yaml). The EOA must be an
authorized caller of the RFQ Executor. Each configured connector must be registered
as version `2` in the Reactor's Liquid Lane adapter factory (version `1` is the
native adapter). Each RWA's on-chain `recipeExecutors(token)` binding must name a RecipeExecutor with
the RFQ Executor as caller and the connector's MerkleAccount as account.

Configuration contains the backend URL, durable state directory, RFQ executor,
curator service and an explicit allowlist of 1..256 connectors. Per-RWA executors and
funding capacity come from each connector on-chain; no local executor map or source
list is maintained. Recipe discovery, capacity and connector bindings are read at the
same pinned block. A recipe that requires issuer redemption includes those calls for
its asset. Duplicate connector addresses are rejected, including case variants.
Polling is every three seconds;
each iteration has a thirty-second context and fetches at most 100 open orders.
Curator HTTP limits are ten seconds and 2 MiB. Secret configuration contains
environment variable names. This solver has no quote server or adapter inventory.
Use a separate RFQ Executor/filler identity for the adapter solver; two pollers must
not compete for the same awarded orders.

The signed order tuple supplies the entire input and sum of output payments.
Matching advertised connectors are tried in address/discount-ID order, and one must
cover the complete award using both its advertised capacity and pinned on-chain
`getMaxAssets(token)` result. This getter caps the connector budget by a rollback-only
execution of the committed recipe's funding prefix, including current permissions and
sequential source liquidity. No market pricing, splits or adapter/Merkle liquidity aggregation occurs.
An upstream source must award compatible orders; this solver does not publish quotes.
The wall-clock observation is sampled before the header RPC; its latency consumes the unsigned submission deadline, including
when the chain clock is ahead of the host.

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
The host ABI-encodes valid hex signatures unchanged, including arbitrary-length
contract-wallet bytes. The SDK verifies the leaf, mask and proof; the committed
calldata length, offsets and pinned fields must admit the supplied signatures.
Existing fixed 65-byte templates need compatible permissions for other lengths.
The complete outer fill simulation must still succeed before admission.
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
commit `de6dbd617bf3c1e8b4120419cbe5800efae7d410`. Connector, recipe and capacity semantics
come from [adapters PR 14](https://github.com/symbioticfi/adapters/pull/14),
commit `9b693f34f6819f6c4e09f3751d84c2d1098a92c1`.
`make refresh-merkle-abi` projects only solver-used entries from canonical Foundry
artifacts; `make bindings` regenerates typed helpers. Deploy the matching connector and RecipeExecutor versions and publish capacity
witnesses before enabling these routes. Missing, stale or failing capacity does not
fall back to account cash.

Tests cover signed award sizing, route capacity, unchanged recipe/permissions,
complete outer simulation, revoked roles, reorgs, intent persistence, sequential
admission and recovery without new API terms. `make test-rfq-merkle-anvil` executes
the complete outer fill against pinned canonical Reactor, Executor, RecipeExecutor,
MerkleAccount and LiquidLaneConnector bytecode with real EIP-712 signatures. It
checks successful simulation and mined settlement, invalid signed economics,
funding lost after the capacity observation, and an unauthorized sending EOA.
The fixture substitutes only external ERC20s, registries and the price feed; proxy
administration and deployment are outside its scope. Bytecode provenance and the
offline regeneration command are in `internal/solvers/merkle/rfq/testdata/outer-fill/`.
Local fixtures do not establish an
authenticated live backend, deployed bridge or mainnet execution.
