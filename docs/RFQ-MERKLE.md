# RFQ orders through Merkle recipes

`rfq-filler` selects its path from local configuration: the existing `adapters`
list preserves native quoting and fills; `recipeExecutors` selects Merkle
quoting and fills. Setting both fails before startup. The execution implementation
is shared with the execution-only `rfq-merkle` plugin in `internal/solvers/merkle/rfq`.
Protocol-neutral connector logic lives in `internal/liquidlane/connector`: recipe route
discovery, signed-discount framing, recipe assembly, SDK admission of the outer simulation
and the RAM spend ledger. The signer lane, its single in-flight fill and discount selection
live in `internal/solvers/merkle/filler`, shared with the UniswapX and LI.FI Merkle fillers
([overview](MERKLE-FILLERS.md)). The RFQ package keeps the order feed and the Executor `fill` envelope.
It polls awarded `/api/v1/orders?filler=…&orderStatus=open`, obtains signed terms
from the backend discounts API, and builds:

`EOA → RFQ Executor → AccessManager → RecipeExecutor → MerkleAccount → LiquidLaneConnector`.
A recipe whose caller is the RFQ Executor uses the direct path.

Use [the example config](../config/rfq-merkle.example.yaml). Configure the RFQ
`executor` and a `recipeExecutors: [...]` list. The first is the RFQ filler;
the list contains the funding recipes.
The EOA must be an authorized caller of the RFQ Executor. The solver derives each
payment connector from the original recipe's output-token approval using input 1,
checks its account and output asset, and authenticates the recipe's on-chain caller.
For an AccessManager caller, grant the RFQ Executor a role with zero execution delay
for the RecipeExecutor's `execute(bytes[],bytes[],uint256[],bytes[])` selector.
The SDK checks `canCall` at the pinned block and wraps the unchanged recipe calldata
in `AccessManager.execute(recipeExecutor, calldata)`. Missing or delayed grants
reject route initialization and fill construction. A later revocation also fails the
complete outer simulation. Recipe grants on the MerkleAccount remain assigned to
the RecipeExecutor, not to the manager or RFQ Executor. No manager address is added
to solver configuration: it is the recipe's authenticated caller.
The connector must be registered as version `2` in the Reactor's Liquid
Lane adapter factory (version `1` is a native adapter).

Backend quote requests carry capacity, price, token decimals and discount precision.
Connector quotes, discount listings and resolved terms require explicit precision
`100000000`, validated against the locally configured connector path. Missing or native
precision does not select a different execution path.
The solver derives account and recipe identities from the verified startup route cache.
Exactly one configured recipe must match the connector, RWA swap permission and output
asset; missing or ambiguous routes are declined. All supported RFQ quote paths (native external, native discounted and connector
discounted) perform no quote-time RPC, metadata warming or permission-service calls, and account/recipe identities are absent from backend
requests. The backend maps each connector to a `recipeExecutors` list and measures
each recipe against the requested RWA. It emits one capacity per connector/discount,
bounded by the lowest applicable recipe capacity, including zero; a failed recipe
read suppresses connector capacity. Award selection uses the same cached resolver;
fill construction checks fresh permissions, state and the complete outer settlement.
The SDK retrieves original recipe bytes at the executor's known `recipeBlock`.
Execution verifies current permissions and the complete outer fill; recipes remain unchanged.

Both full and `quoteOnly` Merkle deployments require `curator.url`: they use the same
startup discovery to verify the configured recipe's connector and permitted RWA tokens.
Discovery failure aborts startup. Quote-only mode reads permissions without starting
the shared fill transaction manager or polling orders. It does not discover routes from backend discount listings.
Merkle fills keep their active intent in RAM; `stateDir` is removed and rejected.
All configured recipes share one fill owner and the host
transaction manager.
Quote capacity is grouped by account and funding asset, not executor. Overlapping
recipes are alternatives; they cannot multiply that balance within a quote.
Use one order poller per RFQ Executor/filler identity; a second process for quotes must
set `quoteOnly: true`.
Polling is every three seconds. Once a pending fill settles, the loop checks the
next award immediately; pending work and uncertain failures use the normal cadence.
Each iteration has a thirty-second context and fetches at most 100 open orders.
Curator HTTP limits are ten seconds and 2 MiB. Secret configuration contains
environment variable names. The execution-only plugin has no quote server. Native RFQ mode keeps its existing
configurable polling limits.

The signed order tuple supplies the entire input and sum of output payments.
The solver tries a single connector first. When it needs multiple connectors, it sizes
legs using the shared Liquid Lane allocator and the backend's advertised capacity,
clipped by each connector's current `remainingAssets(token)` policy budget and
shared account/asset capacity. Each leg uses its own exact on-chain payout, and the
legs must cover the signed input and output together. Capacity calculation belongs
to the backend. It estimates funding from each recipe's reads and checks current
roles with batched RPC calls. The complete outer settlement simulation checks ordered
withdrawals and shared liquidity before submission.
The quote path consumes the backend capacity and price and subtracts shared
account/asset reservations without RPC reads. The execution-only plugin consumes awards
from the backend.
The wall-clock observation is sampled before the header RPC; its latency consumes the unsigned submission deadline, including
when the chain clock is ahead of the host.

The SDK reads the committed `recipeHash` and `recipeBlock` at the pinned block,
retrieves `SetRecipe` only from that publication block and executor, and checks its body
against the current commitment. No historical block range is scanned. It reads the active root, authenticates the curator
tree, verifies masks/proofs and roles, and checks the canonical snapshot around
simulation. Each recipe uses its leg's signed terms, input amount and exact connector
payout. The amounts are Inputs 0 and 1. Recipe gates, additional actions and balance
checks remain curator-owned. The recipe and permission publication formats are unchanged;
call data and proofs come from the existing curator permission-tree service.
`LLExecuting` retains acquired inventory when its redemption leaves are empty;
otherwise its configured issuer calls execute before the complete outer fill succeeds.
It simulates the complete outer RFQ fill from the EOA so Reactor funding, recipe
authorization and output settlement are checked together.
The shared external-simulation admission path authenticates the inner call, fresh
metadata and roles, then runs that exact outer fill at the supplied pinned block
and transaction gas budget before closing both original and fresh block pins.
RFQ supplies no numerical result witness. The awarded fill's complete settlement
is its work condition; generic inner `HasWork` classification is not required.
A successful fill can therefore use existing RFQ Executor inventory even when an
inner Manage gate is closed, provided every committed recipe check and the full
signed award still succeeds. Generic recipe paths retain numerical witness and
result-based readiness validation. RFQ requires ordinary `eth_call` only; it does
not fetch recipe bytecode or install a nested return-data probe.

The host puts each connector amount and its unchanged signed terms in
`discountSwapInputs`. The `executorData` `Call[]` contains each SDK-assembled
`RecipeExecutor.execute` call, optionally wrapped by that recipe's AccessManager.
All legs settle atomically in the same Reactor fill.

The host preserves upstream discount integers and signature bytes. It checks ABI
framing and route identity; signatures and discount economics are enforced by the
contracts. It does not translate discount scales or verify EIP-712 signatures locally.
The host ABI-encodes valid hex signatures unchanged, including arbitrary-length
contract-wallet bytes. The SDK verifies the leaf, mask and proof; the committed
calldata length, offsets and pinned fields must admit the supplied signatures.
Existing fixed 65-byte templates need compatible permissions for other lengths.
The complete outer fill simulation must still succeed before admission.
The canonical `LLExecuting` recipe checks the exact payout for each leg.
An order can pay multiple recipients in the same output token.

Only one fill is admitted at a time. Its awarded order, immutable calldata and
deadline stay in RAM through ambiguous submission. Each signed original and
replacement is captured before broadcast through the process transaction manager.
The earlier order/discount deadline bounds unsigned sending and replacement.
Resumption observes the same signed attempts and nonce without fetching new terms;
only a canonical owned inclusion or revert releases the signer fence. Restart
loses intent and transaction tracking while the transaction may still execute.
Settle old work before upgrading and use one active process per signer.

Quote inventory must match the verified recipe connector, RWA, account and asset.
Quoting stays available during a fill; execution remains sequential. Before
submission, each connector payout is reserved against its funding account/asset
capacity. Other capacity and the unreserved remainder stay quotable. Ambiguous
submissions keep that reservation, including the gap after sender release and
before the fill loop records its canonical receipt. Unsigned failures and canonical
reverts release it. Confirmed spends remain subtracted from older inventory until
that capacity's snapshots reflect inclusion. Confirmed details compact after fresh
inventory or the signed deadline; an inclusion watermark prevents later stale
snapshots from reopening that capacity. Pending reservations survive their deadlines
until the signed outcome is known.
Completed award IDs are retained until their signed chain deadlines, preventing
a still-listed award from being filled twice. Reservations, watermarks and completed
IDs are RAM only and are lost on process exit. SDK lifecycle readiness keeps the
quote service in rotation while the transaction lane is occupied.

The RFQ bridge ABI comes from [RFQ PR 25](https://github.com/symbioticfi/rfq/pull/25),
commit `de6dbd617bf3c1e8b4120419cbe5800efae7d410`. Connector and RecipeExecutor use the unchanged canonical adapters contract surface.
`make refresh-merkle-abi` projects only solver-used entries from canonical Foundry artifacts;
`make bindings` regenerates typed helpers. Capacity requires no connector/executor contract
extension, extra recipe metadata or separate funding configuration. Missing executors and
invalid backend capacity fail closed; execution still requires the full outer settlement simulation.

Tests cover signed award sizing, route capacity, unchanged recipe/permissions,
complete outer simulation, immediate/missing/delayed AccessManager grants, revoked
roles, reorgs, RAM intent retention, sequential
admission and retries without new API terms. `make test-rfq-merkle-anvil` executes
the complete outer fill against pinned canonical Reactor, Executor, RecipeExecutor,
MerkleAccount, LiquidLaneConnector and OpenZeppelin AccessManager bytecode with real EIP-712 signatures. It
checks successful simulation and mined settlement, invalid signed economics,
funding lost after the capacity observation, an unauthorized sending EOA, and
manager-routed settlement with missing authorization or invalid signatures.
It also checks a signed two-connector fill through distinct AccessManagers and
atomic rollback when the second grant is revoked.
The fixture substitutes only external ERC20s, registries and the price feed; proxy
administration and deployment are outside its scope. Bytecode provenance and the
offline regeneration command are in `internal/solvers/merkle/rfq/testdata/outer-fill/`.
Local fixtures do not establish an
authenticated live backend, deployed bridge or mainnet execution.
