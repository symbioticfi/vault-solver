# vault-solver — IRIS solver (plan)

## 1. What the solver does

IRIS is a fixed-rate borrowing protocol. Borrowers open loans on Aave V3, Morpho Blue or
SparkLend at a fixed rate; a solver quotes that rate and posts a debt-token bond that absorbs
the gap between the fixed rate and the venue's floating rate. The solver earns the spread
(less the protocol fee) and loses bond when floating rates rise above the quote.

The `iris` solver answers IRIS RFQ coordinator quote requests on behalf of one Merkle
account. The account is the IRIS `solver`: it signs through ERC-1271 and Iris pulls the bond
from it at `take`. For each request the solver:

1. Finds the account's `iris-signing` recipes for the request's pair and allowed venues.
2. Prices the quote and computes the bond the bond lock module (BLM) requires.
3. Checks the account can fund the bond: debt-token balance, and the allowance the recipe
   will leave Iris.
4. Simulates the recipe execution from the signer. The recipe's own checks (debt range,
   rate floor, duration, bond ratio, deadline window, pending-bond cap) decide whether the
   terms are allowed.
5. Sends the execution, waits for its receipt, confirms `isValidSignature(digest)` and the
   Iris allowance on-chain, then returns the quote with a 65-byte placeholder signature.

There is no off-chain key signing quotes. The digest only exists if the account computed it
from the quote fields inside a curator-approved recipe.

## 2. How it maps onto the framework

- Package `internal/solvers/iris/`, solver name `iris`. It needs the Merkle SDK transaction
  manager (`RequiresMerkleTxManager`) and not the native one.
- Recipes and permissions come from the Merkle SDK, like the Merkle fillers:
  `recipes.DiscoverExecutors` checks every configured executor's account and caller,
  `recipes.DiscoverBatch` loads each executor's committed recipe from its `SetRecipe` event,
  and the curator (`GET /merkle/{adapter}/{root}`) supplies the permission tree.
- A market is not configured. It is decoded from the recipe's sign leaf (§7): Iris domain,
  BLM, collateral and debt tokens, overdue terms, bond LLTV, venue id and market data.
  Discovery builds an index from (collateral, debt) to markets. It runs at startup and
  again every `refreshInterval`; a failed refresh keeps the previous index.
- Each refresh first reads, in one multicall, every executor's `caller`, `recipeHash` and
  `recipeBlock` and the account `root`. These are the values the SDK compares before it
  admits a prepared recipe. If they equal the discovery snapshot the index was built from,
  discovery is skipped. Any `setRecipe` (even with the same body) moves `recipeBlock`, a new
  tree moves `root`, so no recipe or tree change can be skipped. AccessManager grants and
  account roles are not in these values; quote-time admission checks them on every quote.
- Execution calldata is `recipes.Assemble(recipe, tree, inputs, nil, nil)` with the ten free
  Quote words as inputs, routed with `recipes.RouteExecution` (direct caller or
  AccessManager). `recipes.ValidatePreparedBatchForSender` authenticates and simulates the
  exact transaction at a fresh block before it is sent.
- Sending uses the SDK `transaction.Manager` lane (`manager.Sender(Name)`): one execution in
  flight. A request that arrives while the lane is busy is declined.
- The HTTP server follows the uniswapx quote server: own listen address, `TraceHandler`,
  panic recovery, `Shutdown` on context end.

Not reused: `filler.Engine`. It is bound to one settlement contract and Liquid Lane
connector routes, while an IRIS execution targets whichever recipe executor serves the
market.

## 3. Protocol reference

- Iris `take(Quote, signature)` checks the deadline, the solver signature
  (`SignatureCheckerLib`: ECDSA, else ERC-1271), the nonce, the BLM requirement
  (`bond >= bondRequirement`) and pulls the bond from `solver` with `transferFrom`.
- EIP-712 domain: `EIP712Domain(uint256 chainId,address verifyingContract)`, no name or
  version. The Quote type has 19 fields; `data` is hashed as `keccak256(data)`.
- `IBlm.bondRequirement(Quote)` is a view; the solver calls it with the exact quote. IRIS
  ships no ABI artifact, so `api/abi/IrisBlm.json` is hand-vendored to that one function.
- Coordinator response budget: 5 s by default, raisable per solver at onboarding.
- `deadline` is accepted only within about 5 s of the 2-minute TTL: it must be close to
  120 s ahead of the coordinator's clock when the response arrives (§6).

## 4. API schemas

The coordinator POSTs the borrower request plus `quoteId` to the registered endpoint.
IRIS publishes no OpenAPI document. The API reference page is vendored as the contract of
record (`openapi/iris-rfq-api-reference.md`, `make refresh-iris-rfq-api`, drift-checked as
text), and the request and response types in `api.go` are hand-written from it. Fields are
decimal strings except `chainId`.

- Request: `requestId`, `quoteId`, `chainId`, `borrower`, `receiver`, `collateralToken`,
  `collateral`, `debtToken`, `debt`, `maxFixedRate`, `duration`, `venueBitmap`.
- Firm response (200): `requestId`, `quoteId`, `solver`, `blm`, `fixedRate`, `venueId`,
  `data`, `overdueRate`, `overduePeriod`, `bondLltv`, `bond`, `deadline`, `nonce`,
  `signature`.
- Indicative (`POST /quote/indicative`): the same pricing without signing; the response
  drops `deadline`, `nonce` and `signature` and adds `indicative: true`.
- Decline: 404 with optional `reasons[]` (`venue`, snake_case `reason`, optional `rate`).
- The registry entry's `headers` are the caller authentication: the solver requires a
  configured header whose value comes from an env var. It is mandatory, because every firm
  request spends gas.

## 5. Pricing

v1 is a configured rate table: a default fixed rate and optional overrides by debt token,
collateral token and venue id, most specific first. Rates are WAD and must be multiples
of 1e14. A market whose rate exceeds `maxFixedRate` declines with
`rate_above_max_fixed_rate` and the rate. Among several candidate markets the lowest rate
wins; ties keep executor order. The bond is the BLM requirement, raised by an optional
`bondBufferBps`.

Floating-rate aware pricing (venue rate plus spread and a duration premium) is §10 work.

## 6. Timing and safety

- Deadline. The signature must be mined before the response, and the deadline must be
  about 120 s ahead when the coordinator reads the response. The solver targets the next
  slot that the transaction can make: `inclusion = head timestamp + k * blockTime`, the
  first slot at least `submitLead` from now, and signs `deadline = inclusion + 120 +
  responseSlack`. The lead is rounded up to whole seconds. When the target skips the next
  slot, the solver waits until the slot before the target starts before sending, so the
  signature cannot land a slot early (which would put the deadline past the window). A
  receipt in a later block makes the deadline too short; that quote is declined after the
  fact (the digest stays harmless: it expires and its allowance share is bounded by the
  recipe cap).
- Budget. A request handler waits at most `responseBudget` for the receipt. With 12 s
  mainnet slots this needs IRIS to raise our coordinator timeout to roughly 25-30 s.
- Exposure. The recipe caps Iris's allowance at `maxPendingBond`; posted bonds are bounded
  only by the account's debt-token balance. `maxBond` in config is a per-quote cap below
  the recipe's own checks.
- Nonces are random 128-bit values, so unfilled quotes never block later ones.
- Fail closed: any read, simulation or confirmation error declines the request.

## 7. On-chain contract

Recipes come from the adapters repo (`script/recipes/IrisSigning.sol`, template
`iris-signing`; `IrisSettlement.sol`, template `iris-settlement`). The solver accepts a
recipe as an IRIS market only when:

- it has one Manage slot with one leaf targeting the account with `sign(bytes32,bytes,Hash[])`,
  and one Approve slot whose token is the leaf's debt token and whose spender is Iris;
- the domain separator equals `keccak256(abi.encode(DOMAIN_TYPEHASH, chainId, iris))`;
- buffer word 0 is the Quote type hash, word 2 (`solver`) is the account, and the plan
  hashes the trailing market data (empty for Aave V3) into word 19 and then the 20-word
  struct;
- Quote words 1, 3, 7, 8, 9, 10, 13, 15, 17 and 18 are bound to inputs 0 to 9 in that order
  and no other word is masked or bound.

Executors whose recipe is not an IRIS sign recipe are rejected at startup. Every executor
must share the configured account and have the signer (or an AccessManager granting it) as
caller.

## 8. Validation and testing

- Unit: config parsing (and the example config); digest parity with the account's sign
  buffer and with go-ethereum `apitypes`; sign-leaf decoding against a fixture built by the
  adapters `BuildRecipe` script (`testdata/iris-signing.json`) and its rejections; pricing
  table; slot and deadline selection; indicative pricing and declines against a fake chain;
  lane ownership and stall against a fake sender; handler authentication and declines.
- Fork (`make test-iris-fork`, build tag `integration`, needs `ETH_RPC_URL_MAINNET`): anvil forks
  mainnet and the test runs the built binary from a YAML config against the deployed Iris,
  its default BLM, WETH and USDC on Aave V3. It plays the coordinator and the borrower: it
  posts to the solver's endpoints and sends the borrower's `take`. The account and its
  executors run the pinned adapters bytecode; the recipes come from the adapters
  `BuildRecipe` script (`testdata/fork/generate.py`).

### 8.1 Fork test checklist

1. Setup: the account accepts a root holding the sign, approval and settlement leaves;
   each executor holds its role, recipe and caller; the account holds USDC.
2. Startup: the binary discovers one market (WETH/USDC, Aave V3, empty data, mainnet BLM)
   from the curator bundle and serves `/healthz`.
3. Auth: a request without the configured header gets 401.
4. Indicative quote: the bond equals the BLM requirement read independently, plus the
   buffer; nothing is sent on-chain.
5. Firm quote: a 65-byte zero signature and a deadline inside the coordinator window.
6. Coordinator checks, reproduced: the digest computed independently from the Iris domain
   is valid through `isValidSignature`, and the account's balance and Iris allowance cover
   the bond.
7. Take: the borrower's `take` with the zero signature opens the loan with the account as
   solver, pulls the bond from the account, pays the receiver and uses the nonce.
8. Replay: the same quote cannot be taken twice.
9. Recipe limits: debt above `maxDebt` and duration above `maxDuration` decline with
   `recipe_rejected` and send nothing.
10. Solver declines: rate above `maxFixedRate`, a venue bitmap without the market's venue,
    and an account that cannot fund the bond.
11. One lane: of two concurrent firm requests one is signed and one gets `solver_busy`.
12. Recipe changes: unchanged refreshes skip discovery; `setRecipe` triggers it; a caller
    change keeps the old index, quotes decline until it is restored, then sign again.
13. Expiry: a signed quote taken after its deadline reverts.
14. Settlement: after maturity and the borrower's repayment, the `iris-settlement` recipe
    claims the account's Iris balance and withdraws its bond back to the account.

Left to onboarding: the coordinator itself (delivery, its timeout, its off-chain check of
the zero signature and the registry headers).

## 9. Build phases

1. Plan, config, digest, market decoding, pricing (this change).
2. Discovery, quote flow, lane, HTTP server, metrics (this change).
3. Fork test against the deployed Iris (done); README and example config polish.
4. Settlement automation through `iris-settlement` (claims and excess bond).
5. Floating-rate aware pricing and exposure budgets.

## 10. TODO list (live)

### 10.1 IRIS onboarding

- [ ] Register the endpoint and the account as solver address.
- [ ] Raise the coordinator response timeout to about 30 s.
- [ ] Confirm the coordinator accepts a 65-byte zero signature for an ERC-1271 solver.
- [ ] Agree the authentication header.

### 10.2 Code

- [x] Plan.
- [x] Config, digest, market decoding, pricing.
- [x] Discovery and market index refresh.
- [x] Quote flow: funding, simulation, send, confirmation.
- [x] HTTP server, indicative endpoint, metrics.
- [x] Fork test with the real Iris (§8.1); also covers the `iris-settlement` recipe.
- [ ] Settlement automation (`iris-settlement`).
- [ ] Floating-rate pricing.

## 11. Resources

- IRIS docs: https://docs.iris.credit (RFQ API reference, validation, solver economics).
- iris-core: `src/Iris.sol`, `src/interfaces/IBlm.sol`.
- adapters: `docs/RECIPE-SIGNING.md` (Iris sections), `script/recipes/IrisSigning.sol`.
