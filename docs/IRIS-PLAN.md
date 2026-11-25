# vault-solver — IRIS solver (plan)

## 1. What the solver does

IRIS is a fixed-rate borrowing protocol. Borrowers open loans on Aave V3, Morpho Blue or
SparkLend at a fixed rate; a solver quotes that rate and posts a debt-token bond that absorbs
the gap between the fixed rate and the venue's floating rate. The solver earns the spread
(less the protocol fee) and loses bond when floating rates rise above the quote.

The `iris` solver answers IRIS RFQ coordinator quote requests on behalf of one Merkle
account. The account is the IRIS `solver`: it signs through ERC-1271 and Iris pulls the bond
from it at `take`. For each request the solver:

1. Finds the account's offer executors (one per market, built in the curator UI) for the
   request's pair and allowed venues whose principal and tenor caps admit the request.
2. Prices the quote (configured rate, raised to the market's APR floor) and computes the
   bond the recipe will: the bond lock module (BLM) requirement plus a quarter, rounded up.
3. Checks the curator's Iris allowance covers this bond and every outstanding quote's, and
   with quotes outstanding that the account's cash already does (§6).
4. Simulates the recipe execution from the signer. The recipe draws any bond shortfall
   from the curator's funding venues and enforces its caps, floor and deadline bound.
5. Sends the execution, waits for its receipt, confirms `isValidSignature(digest)`, then
   returns the quote with a 65-byte placeholder signature.

There is no off-chain key signing quotes. The digest only exists if the account computed it
from the quote fields inside a curator-approved recipe.

## 2. How it maps onto the framework

- Package `internal/solvers/iris/`, solver name `iris`. It needs the Merkle SDK transaction
  manager (`RequiresMerkleTxManager`) and not the native one.
- Recipes and permissions come from the Merkle SDK, like the Merkle fillers:
  `recipes.DiscoverExecutors` checks every configured executor's account and caller,
  `recipes.DiscoverBatch` loads each executor's committed recipe from its `SetRecipe` event,
  and the curator (`GET /merkle/{adapter}/{root}`) supplies the permission tree.
- A market is not configured. It is decoded from the offer recipe (§7): the sign leaf gives
  the Iris domain and solver, the recipe's constant bindings give BLM, tokens, overdue
  terms, bond LLTV, venue id and payload hash, and its checks give the per-market limits.
  The recipe holds only the payload's hash, so the payload is recovered on-chain: empty for
  pool venues, `Morpho.idToMarketParams` for a Morpho market id (`morpho` in config), all
  Morpho markets in one multicall per discovery. Discovery builds an index from (collateral,
  debt) to markets; an executor whose recipe is not a usable offer program is logged and
  skipped, so it cannot take the other markets down. It runs at startup and
  again every `refreshInterval`; a failed refresh keeps the previous index.
- Each refresh first reads, in one multicall, every executor's `caller`, `recipeHash` and
  `recipeBlock` and the account `root`. These are the values the SDK compares before it
  admits a prepared recipe. If they equal the discovery snapshot the index was built from,
  discovery is skipped. Any `setRecipe` (even with the same body) moves `recipeBlock`, a new
  tree moves `root`, so no recipe or tree change can be skipped. AccessManager grants and
  account roles are not in these values; quote-time admission checks them on every quote.
- Execution calldata is `recipes.Assemble(recipe, tree, inputs, nil, nil)` with the nine
  keeper inputs (borrower, receiver, collateral, debt, duration, fixedRate, venueBitmap,
  deadline, nonce), routed with `recipes.RouteExecution` (direct caller or
  AccessManager). `recipes.ValidatePreparedBatchForSender` authenticates and simulates the
  exact transaction at a fresh block before it is sent.
- Sending uses the SDK `transaction.Manager` lane (`manager.Sender(Name)`): one execution in
  flight. Firm quotes run one at a time end to end, so each sees the capacity the previous
  one reserved; a request that arrives meanwhile is declined `solver_busy`.
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
of 1e14. A configured rate below a market's APR floor is raised to the floor. Markets whose
principal or tenor cap the request exceeds are skipped (`above_market_limits` when none is
left). A market whose rate exceeds `maxFixedRate` declines with
`rate_above_max_fixed_rate` and the rate. Among several candidate markets the lowest rate
wins; ties keep executor order. The bond is fixed by the recipe: `r + (r+3)/4` for the BLM
requirement `r`; `maxBond` caps it further.

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
  fact (the digest stays harmless: it expires, and its bond stays reserved until then).
  The recipe allows a deadline at most 30 minutes past the signing block; discovery rejects
  a market when `quoteTtl + responseSlack + blockTime` exceeds that bound.
- Budget. A request handler waits at most `responseBudget` for the receipt. With 12 s
  mainnet slots this needs IRIS to raise our coordinator timeout to roughly 25-30 s.
- Exposure. The curator's standing Iris allowance is the on-chain cap on bonds Iris can
  pull: every taken quote's loss is at most its bond, and Iris pulls nothing else from the
  solver. The recipe tops cash up only to the current bond, so two open quotes could share
  the same cash. The solver therefore reserves each signed quote's bond until its deadline
  and requires allowance (and, with quotes open, cash) to cover all of them. Reservations
  are in memory and end at the deadline, not at take: a restart forgets them (quotes live
  `quoteTtl`), and a taken quote stays counted until it expires. The direct allowance is
  required; the Permit2 fallback Iris supports is not used.
- Nonces are random 128-bit values, so unfilled quotes never block later ones.
- Fail closed: any read, simulation or confirmation error declines the request.

## 7. On-chain contract

Recipes and permissions are built by the curator UI (curator-frontend `iris-offers`,
`iris_auction_offers`, `iris_solver`, and the Claims template's Iris bond claim). The
solver accepts a recipe as an IRIS market only when:

- it takes exactly the nine keeper inputs and has no Approve slot;
- exactly one Manage action targets the account with `sign(bytes32,bytes,Hash[])`, under a
  literal gate of 1; other Manage actions (the bond funding withdrawals) are left to the
  recipe and the simulation;
- the domain separator equals `keccak256(abi.encode(DOMAIN_TYPEHASH, chainId, iris))`, the
  plan hashes the 20-word struct in one step, word 0 is the Quote type hash and word 2
  (`solver`) is pinned to the account;
- every other Quote word is masked and bound in order: the keeper's words to inputs 0 to 8,
  the bond (word 13) to a computed value, and BLM, tokens, overdue terms, bond LLTV, venue
  id and payload hash to constants (a literal, or the Add/MulDiv form the UI uses for words
  wider than a literal operand);
- the recipe bounds debt and duration from above, the fixed rate from below, and the
  deadline by `clock + L`.

Security review of the UI recipe (compiled bytes decoded and checked against
`RecipeExecutor`, `MerkleAccount` and `Iris.take`): the keeper cannot sign outside the
market. The leaf pins the selector, domain, type hash, solver, buffer size and hash plan;
the executor overwrites every masked word with the recipe's binding before checking the
leaf, so no masked byte is keeper-controlled. Market terms are constants; debt, tenor, APR
floor, nonce and deadline are checked; the bond is computed from the live BLM; funding
withdrawals are sized `min(bond - cash, capacity)`, pay only the account, and stop on any
Aave debt. Iris itself requires a nonzero collateral, debt and bond, the venue bit in the
borrower's bitmap, enabled BLM, LLTV and payload, and `msg.sender` to be the borrower or
their delegate. The pinned type hash also stops the sign leaf producing an Iris
`Authorization`. Residual risks, none a bypass: signed quotes cannot be revoked (they live
until their deadline, at most 30 minutes); the allowance, not the recipe, caps total open
exposure; repeated runs can move up to one bond from a venue into idle cash; the sign role
must only be granted to offer executors; a Permit2 allowance to Iris, if ever granted,
would bypass the allowance cap.

## 8. Validation and testing

- Unit: config parsing (and the example config); digest parity with the account's sign
  buffer and with go-ethereum `apitypes`; recipe decoding against the curator UI's own
  compile output (`testdata/ui/recipes.json`, from `make refresh-iris-fixture`) and its
  rejections; the recipe's bond rounding; Morpho payload recovery; pricing table; slot and
  deadline selection; per-market limits, floors, allowance and reservations against a
  fake chain; lane ownership and stall against a fake sender; handler authentication and
  declines.
- Fork (`make test-iris-fork`, build tag `integration`, needs `ETH_RPC_URL_MAINNET`): anvil forks
  mainnet and the test runs the built binary from a YAML config against the deployed Iris,
  its default BLM, Aave Core, Morpho Blue, wstETH and USDC. It plays the coordinator, the
  curator and the borrower: it posts to the solver's endpoints, sets the allowance and
  claims through the curator's own leaves, and sends the borrower's `take` and `repay`. The
  account, executors and AccessManager run pinned canonical bytecode; the permission tree
  and recipes are the UI fixture.

### 8.1 Fork test checklist

1. Setup: the account accepts the UI's root; an Aave and a Morpho offer executor (wstETH/
   USDC; 20k/30 days/4% and 5k/7 days/6.5%) and the bond-claim executor sit behind one
   AccessManager keeper; roles are granted per executor; the curator sets a 30k Iris
   allowance; the account's USDC sits in Aave, with no cash.
2. Startup: the binary discovers both markets, recovers the Morpho payload, skips the
   claim executor with a logged reason, and serves `/healthz`.
3. Auth: a request without the configured header gets 401.
4. Indicative quote: a request above the Morpho caps goes to Aave; the bond equals the
   BLM requirement read independently, padded like the recipe; nothing is sent.
5. Firm quote: the recipe withdraws exactly the bond from Aave; a 65-byte zero signature
   and a deadline inside the coordinator window.
6. Coordinator checks, reproduced: the digest computed independently from the Iris domain
   is valid through `isValidSignature`, and the account's balance and Iris allowance cover
   the bond.
7. Take: the borrower's `take` opens the loan with the account as solver, pulls the bond,
   pays the receiver and uses the nonce.
8. Replay: the same quote cannot be taken twice.
9. Per-market limits: debt above both caps, debt above the Morpho cap, tenor above both
   caps and a ceiling under the Morpho floor decline before anything is sent.
10. Reservation: with the first quote still reserved and no cash, a second quote declines
    `insufficient_bond_balance`.
11. Morpho market: once cash covers it, a quote signs on the Morpho market at the 6.5%
    floor with the payload read from Morpho, and the borrower takes it.
12. Solver declines: a bitmap without any market's venue, and a cleared allowance.
13. One lane: of two concurrent firm requests one is signed and one gets `solver_busy`.
14. Recipe and keeper changes: unchanged refreshes skip discovery; `setRecipe` triggers it;
    a caller change keeps the old index and quotes decline until it is restored; a revoked
    AccessManager role declines quotes until it is granted again.
15. Expiry: a signed quote taken after its deadline reverts.
16. Settlement: after maturity and repayment, the UI claim recipe returns the bond (and
    refuses an open loan), and the curator's claim leaf returns the solver proceeds.

Left to onboarding: the coordinator itself (delivery, its timeout, its off-chain check of
the zero signature and the registry headers).

## 9. Build phases

1. Plan, config, digest, market decoding, pricing (this change).
2. Discovery, quote flow, lane, HTTP server, metrics (this change).
3. Fork test against the deployed Iris (done); README and example config polish.
4. Curator UI recipes: one executor per market with per-market limits, bond funding from
   venues, reservations (done). Settlement is the UI's Claims recipe, run by its keeper.
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
- [x] Fork test with the real Iris (§8.1), on the curator UI's recipes.
- [x] Curator UI offer recipes: decoding, per-market limits, Morpho payloads, reservations.
- [ ] Release reservations at take (Take events) if they cap capacity in practice.
- [ ] Discover offer executors from the factory instead of listing them in config.
- [ ] Floating-rate pricing.

## 11. Resources

- IRIS docs: https://docs.iris.credit (RFQ API reference, validation, solver economics).
- iris-core: `src/Iris.sol`, `src/interfaces/IBlm.sol`.
- curator-frontend: `recipes/templates/iris-offers.ts`, `iris-bond-claim.ts`, `catalog/types/iris.ts`.
