# Canonical RFQ outer-fill EVM fixture

`outer_fill_anvil_test.go` starts an isolated local Anvil chain. It does not fork,
use an upstream API, broadcast to an external chain, or require contract compilers
at test time. Run `make test-rfq-merkle-anvil` with `anvil` on PATH.

The full path executes unmodified canonical implementation runtime:

- RFQ `Reactor` and `Executor`: symbioticfi/rfq
  `425944096ad0ee06955e4de93c2380d0e8eb8764` (the source of the vendored RFQ ABI).
- Adapters `RecipeExecutor`, `MerkleAccount`, and `LiquidLaneConnector`:
  symbioticfi/adapters `beaf24c50835fbbe181e3135a64285de2aca1315` (the source of
  the unchanged connector and recipe contract surfaces).

`artifacts.json` contains the canonical Foundry creation bytecode, complete ABIs,
compiler settings and source Keccak hashes. The canonical implementations are
constructed with fixture dependencies, and their unmodified runtime is placed at
stable test addresses with fresh storage, then initialized through canonical
entrypoints. This models proxy execution storage without testing proxy
administration. RecipeExecutor and Reactor are deployed normally.

`Fixture.sol` substitutes only external dependencies: simple ERC20 tokens and
factory/root/oracle registries plus a constant-price feed. The EOA request,
protocol order, connector discount and discount-swap signatures are real ECDSA
EIP-712 signatures checked by the canonical contracts. The recipe commits the
three exact balance deltas and its swap leaf has real mask/proof/role enforcement.
The unused committed Query supplies approval/swap permission preimages.
No Solidity settlement, signature, economics or permission logic is mocked in Go.

The complete unchanged outer fill simulates from the host signer at a pinned
block and the original gas budget through an ordinary `eth_call`. Shared SDK
admission checks the inner recipe/call identity, fresh metadata and roles, and
closes original/fresh block pins around that simulation. RFQ supplies no numerical
witness and uses successful signed award settlement as its work condition, so no
inner return data, code override or probe artifact is needed.

Tests require a valid whole fill to simulate without changing state and then
actually mine with the expected input/output balances. Other cases must reach
the complete outer `eth_call` and fail on invalid discount economics, signatures,
cash missing before settlement simulation, and an EOA missing from Executor's
caller allowlist. Go fixtures retain recipe discovery, mask, role, reorg and RAM
ownership edge coverage.

Offline canonical-fixture regeneration uses built artifacts from clean checkouts at the pinned
revisions plus Solidity 0.8.28 for the small external fixture:

```sh
make refresh-rfq-merkle-fixture \
  RFQ_FIXTURE_ROOT=/path/to/pinned/rfq \
  ADAPTERS_FIXTURE_ROOT=/path/to/pinned/adapters \
  FIXTURE_SOLC=/path/to/solc-0.8.28
```

Build the canonical artifacts using each repository's checked-in Foundry
settings before regeneration. The generator rejects other repository revisions
and unresolved library links. Regeneration changes test fixtures only; it does
not modify the vendored production ABIs or their generated Go bindings.

This fixture establishes hermetic contract-boundary execution, not live backend,
proxy deployment, issuer-redemption, production permission or mainnet evidence.
