# Sepolia deployment

The [shared Sepolia basis](https://github.com/symbioticfi/adapters/blob/714a3f6eb8aa80b5eb6cbd29bb6087f2a31c9bca/script/deploy/testnet/manifests/sepolia-basis-20261030T194357Z.json) contains infrastructure on chain
`11155111`, including the Reactor, Liquid Lane factory and connector implementation,
policy factory, RecipeExecutor factory, lending venues and issuer models. It has
no vault, adapter, account or executor instance. RFQ backend starts with an empty
solver inventory; the basis alone does not provide a running quote or fill service.

Create the vault, Merkle adapter, recipe executor and payment connector through
Sepolia Curator. The protocol owner must approve the exact vault/adapter pair in
the AdapterRegistry before allocation. Publish permissions and the committed
recipe for the actual account, caller and funding route.

For RFQ, copy [the Merkle config](../config/rfq-merkle.example.yaml) to an
operator-local file and set `chain.chainId` to `11155111`, Sepolia read/write RPCs,
the Sepolia RFQ backend and matching Curator backend including `/api/v1`.
`config.executor` is a separately deployed RFQ Executor authorized for the filler;
`config.recipeExecutor` is the curator's RecipeExecutor whose caller is that RFQ
Executor. Neither address is provided by this basis. Configure the same
RecipeExecutor binding under the exact connector in the RFQ backend's
`solvers[].adapters[]` entry. Keep the existing complete outer fill simulation.
See [RFQ setup](RFQ-MERKLE.md) for caller, state and settlement requirements.

For native 3F offers, use [the 3F config](../config/threef-merkle.example.yaml)
with the actual guarded RecipeExecutor and an approved BF, then complete
[3F enrollment](THREEF-MERKLE.md). Start in `mode: simulate`. The official
RequestFactory and its baseline in the manifest are shared source facts, not
executor addresses or approval for a new BF.

Do not launch the solver with empty or made-up executor entries. The framework
requires at least one configured solver, and both Merkle modes require actual
committed recipes. Issuer-model deployment does not establish live issuer access,
liquidity or full runtime support; the configured route must pass its simulation.
