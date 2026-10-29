# RFQ orders through Merkle recipes

`rfq-filler` selects its path from local configuration: the existing `adapters`
list preserves native quoting and fills; `recipeExecutor` selects Merkle
quoting and fills. Setting both fails before startup. The execution implementation
is shared with the execution-only `rfq-merkle` plugin in `internal/solvers/merkle/rfq`.
It polls awarded `/api/v1/orders?filler=…&orderStatus=open`, obtains signed terms
from the backend discounts API, and builds:

`EOA → RFQ Executor → RecipeExecutor → MerkleAccount → LiquidLaneConnector`.

Use [the example config](../config/rfq-merkle.example.yaml). Configure the RFQ
`executor` and a separate `recipeExecutor`; they identify different contracts.
The EOA must be an authorized caller of the RFQ Executor. The solver derives its
payment connector from the original recipe's output-token approval using input 1,
checks its account and output asset, and requires the recipe caller to be the RFQ
Executor. The connector must be registered as version `2` in the Reactor's Liquid
Lane adapter factory (version `1` is a native adapter).

Backend quote requests carry capacity, price, token decimals, account and discount precision, without
`recipeExecutor`. Neither quote requests nor discount listings select the execution
target. The backend keeps its own matching executor binding for capacity reads and
routing; operators must configure the same executor on both sides. The SDK retrieves
original recipe bytes at the executor's known `recipeBlock`. Execution verifies current
permissions and the complete outer fill. Recipes remain unchanged.

Merkle fills require a durable `stateDir` and curator permission service. With
`quoteOnly: true`, `rfq-filler` opens no execution journal and needs no curator URL.
Both plugins derive one payment connector from the configured `recipeExecutor`;
there is no separate connector list. Use one order poller per RFQ Executor/filler
identity; a second process for quotes must set `quoteOnly: true`.
Polling is every three seconds;
each iteration has a thirty-second context and fetches at most 100 open orders.
Curator HTTP limits are ten seconds and 2 MiB. Secret configuration contains
environment variable names. The execution-only plugin has no quote server. Native RFQ mode keeps its existing
configurable polling limits.

The signed order tuple supplies the entire input and sum of output payments.
Discounts for the recipe-derived connector are tried in discount-ID order, and one must
cover the complete award using the backend's advertised capacity, clipped by the current
`remainingAssets(token)` policy budget. Capacity calculation belongs to the backend: it
estimates funding from the configured recipe's reads and checks current roles with batched RPC calls.
The advertised `maxAssets` is an estimate; ordered withdrawals and shared liquidity
are checked by the solver's complete outer settlement simulation before admission.
The regular RFQ quote path consumes the backend capacity and price and subtracts shared
account/asset reservations without RPC reads. The solver does not reconstruct capacity or
require extra recipe queries. No market pricing, splits or adapter/Merkle liquidity aggregation occurs.
An upstream source must award compatible orders; this solver does not publish quotes.
The wall-clock observation is sampled before the header RPC; its latency consumes the unsigned submission deadline, including
when the chain clock is ahead of the host.

The SDK reads the committed `recipeHash` and `recipeBlock` at the pinned block,
retrieves `SetRecipe` only from that publication block and executor, and checks its body
against the current commitment. No historical block range is scanned. It reads the active root, authenticates the curator
tree, verifies masks/proofs and roles, and checks the canonical snapshot around
simulation. The host fills the matching connector swap's signed terms and supplies amountIn
and awarded payment as Inputs 0 and 1. Recipe gates, additional actions and balance
checks remain curator-owned. The recipe and permission publication formats are unchanged;
call data and proofs come from the existing curator permission-tree service.
`LLExecuting` retains acquired inventory when its redemption leaves are empty;
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
commit `de6dbd617bf3c1e8b4120419cbe5800efae7d410`. Connector and RecipeExecutor use the unchanged canonical adapters contract surface.
`make refresh-merkle-abi` projects only solver-used entries from canonical Foundry artifacts;
`make bindings` regenerates typed helpers. Capacity requires no connector/executor contract
extension, extra recipe metadata or separate funding configuration. Missing executors and
invalid backend capacity fail closed; execution still requires the full outer settlement simulation.

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
