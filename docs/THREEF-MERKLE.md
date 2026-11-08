# 3F Merkle execution

The `threef-merkle` solver runs native API-key PUSH offers for one MerkleAccount
and executes one guarded mint recipe covering all configured funding pairs. The adapter-based `3f-bridge-facilitator`
solver and generic `merkle` solver run independently. No liquidity is aggregated
between their accounts. Use [the example YAML](../config/threef-merkle.example.yaml)
with the regular chain, signer and shared transaction manager configuration.

```yaml
solvers:
  - name: threef-merkle
    config:
      mode: simulate
      executor: "0x0000000000000000000000000000000000000010"
      policy: ${THREEF_POLICY}
      curator:
        url: https://curator.example.invalid/api/v1
      url: https://bf.3f.xyz
      apiKeyEnv: THREEF_CURATOR_API_KEY
      broker: "0x0000000000000000000000000000000000000040"
```

The account is the maker. A dedicated, 3F-approved BF EOA and API key serve one
curator/default account. The executor's account and caller are discovered together at a
pinned block; its account must be the default maker and caller must match the shared signer.
Set `THREEF_POLICY` to the deployed policy. Its `account()` must match the executor
account at startup and at each offer snapshot. Every execution group must charge
that same policy. The chain comes from the host configuration. Use
`https://bf.3f.xyz` for mainnet or `https://bf.dev.gcp.3f.xyz` for Sepolia, with
credentials and contracts for the selected network.
Mint permissions can bind amounts to runtime inputs or to on-chain recipe results.
The provider discovers the `consume` call and its bindings; it does not assume an
inner input layout. For on-chain amounts it supplies only the request and records
the expected authorization amounts as result witnesses checked by the SDK's full
transaction simulation. Dynamic collateral pairs come from pending auction metadata
and are attested by the policy before use.
Mode defaults to `simulate`; switch to `send` to authorize offers and mint execution.
One configuration selects one executor and BF/default account. The configured policy and 3F auction catalog supply the offer funding pairs;
position-manager and collateral-token reads resolve their underlying assets.
The recipe supplies execution bindings for request/principal/yield Inputs. Inactive groups
receive zero principal; the recipe determines which calls execute. The solver adds
no group-count limit, canonical guard comparison, mint-gate rule, or separate
funding-gate proof. Fixed withdrawals and additional Call/Approve steps use the
curator-authorized program. Permission response and SDK input bounds still apply.
Retained offer intents bind the executor, recipe, account, policy and exact
upstream terms for in-process retries.
The SDK retrieves the original `SetRecipe` event only at the executor's known
`recipeBlock`, verifies its current commitment and checks the canonical snapshot.
Use one process per curator BF. Do not also list this executor in `merkle.strategies`
or share that BF/key with another independently allocating bot.

`threeF` is not a field of the generic `merkle` solver. The Merkle SDK runtime
and offer intents live in RAM. Remove `stateDir` from YAML; strict decoding rejects
it. Settle unresolved offers and signed work before upgrading or restarting.
Restart loses tracking and reservations while transactions and offers may continue
externally; no saved journal or signed-owner recovery remains.

Polling and mint strategy cadence are 15 seconds. Operations are bounded to
30 seconds, and the shared recipe cache holds the configured recipe within 16 MiB. The public Curator API uses `url` with the SDK's
10-second/2-MiB limits. Existing live commitments are maintained first. Each pass then
checks every relevant open/solvable auction without a blocking offer from this maker,
matching its funding asset to the configured markets. Positive and negative policy
eligibility results are reused only within the same pinned snapshot. The complete
candidate list is checked before bidding, with no per-tick check cap or rotating cursor.
Auctions are ordered by ID and markets by their first relevant auction. Each eligible market gets
one bidding pass over its complete eligible auction list. An API mutation ends
the pass so the next pass reads fresh inventory.
Financial amounts, account identity and manual inputs are not configuration fields.

One-time setup follows the [3F onboarding guide](https://docs.3f.xyz/bridge-facilitator/onboarding):
3F approves the dedicated BF EOA; that EOA generates its API key with a private
off-chain `GenerateFacilitatorApiKey` signature (grunt-api, version 1, chain 1).
Generate and rotate that key through the curator UI or the provider's onboarding tooling.
Load it into the bot's `apiKeyEnv` using your secret manager.
The account approves the `EnrollMaker(broker,maker,deadline)` digest through its
permitted `sign` call. The frontend can submit this enrollment and set the account
as the BF's default maker using the key. Enable maker submissions. Key rotation is
an explicit BF-owner operation; the running solver never rotates keys on startup.
The key may be given to the curator UI for listing, creation and cancellation.
It is resolved from the named environment variable at each HTTP request, never
persisted in a workflow or logged. Missing/disabled enrollment, wrong BF or a changed
default maker stops the workflow before mutations.

For execution, the host decodes the funding pair/policy and the principal, yield
and request Input bindings from the account capability. It does not reconstruct or compare its
internal guards. The SDK authenticates permissions and simulates the complete recipe.
A pinned read-only policy simulation resolves native factory/Facility
membership and verifies the exact funding pair's budget debit; an API position
manager address is never substituted for collateral underlying. This simulation
is a read, not a live transaction or proof of signer access.

Offer discovery and sizing do not call Curator or decode a permission bundle.
They use the configured policy, auction metadata, on-chain asset resolution and
native policy eligibility simulation. Before each API mutation, shared SDK
discovery authenticates fresh account, caller, recipe and root metadata. The host
rechecks every committed leaf role and closes both original and fresh block pins.
This offer-only authorization needs no executable mint. Curator is needed only to prepare execution
with the actual permission data and Merkle proofs. Configure the executor recipe
to support the policy's enabled funding pairs; additional recipe-specific amount
restrictions are applied during execution, not advertised as policy capacity.

Automatic offers conservatively use **idle cash already held by the account**.
New offers use cryptographically random nonces from 1 through 2^63-1, encoded as decimal
strings. Although the on-chain field is uint256, 3F's offer storage is limited to 64 bits.
The API client rejects larger nonces locally. A saved nonce wider than 64 bits is
discarded only when there is no matching API offer and no confirmed offer ID,
allowing the next eligible auction to proceed. Other uncertain offers retain their nonce and terms.
Capacity is the minimum of that cash, the policy's remaining pair budget, the
request's uint128 amount range and the auction's advertised request size.
Like the standalone 3F solver, it can offer on multiple auctions: each live offer
reserves cash for its funding token and budget for its funding/underlying pair.
Only the remainder is available for the next offer. Each auction has its own
RAM intent; creating another offer cannot overwrite it. Reservations include
UI offers and unresolved submissions, without counting an API row and its retained intent
twice. Missing API rows retain their reservations. Unknown POST outcomes retry the
same maker, auction, nonce and terms after rechecking native eligibility, remaining
cash, pair budget and rates. Another market cannot cancel or rekey that intent.
The workflow may cancel its own unaccepted offers or eligible requests within its
current market. Supply cash to the account
before enabling automatic bidding. The actual mint recipe can still use its
committed funding sources; API-key offers created in the UI are reconciled by the
same mint provider. This bot does not promise a quote against liquidity that may
need a later withdrawal from another market or the adapter.

Yield is the ceiling of principal times the policy's minimum divided by 1e8;
this is the nonannual yield/principal ratio. The auction's exact decimal maximum
rate bounds that offer, scaled by its `rateBasisSeconds` over a 365-day year when
provided. Both the rate and period use the original JSON numbers before generated float32
decoding. Negative, fractional and overflowing periods are rejected; an omitted,
null or zero period uses the unscaled comparison. The policy
floor is not annualized. One unsigned PUSH POST uses the BF as wire `maker`, omits
`chainId`, `executionMode` and signatures, and sets `useCallback:false`. The API
stores the account as maker. A five-minute offer expiration is independent of the
post-acceptance mint lease. Unaccepted offers reserve their full principal;
accepted mint windows reserve their pending principal. Manual UI cancellations
suppress automatic replacement for that auction. Unaccepted commitments within authenticated cancellation scope that exceed
cash/budget or the bot's changed terms are cancelled before new offers are created.
At zero remaining pair budget, the native check cannot attest a UI
request without a retained intent: the canonical `ThreeFPolicy.consume` requires positive principal and
`AssetRateLimits.consume` rejects disabled or exhausted budgets. The bot therefore
leaves those UI offers for explicit UI cancellation. It may still cancel its own
retained unaccepted intent, and it never cancels an accepted mint window. Zero
budget always blocks automatic bids. A separate budget-independent membership
interface or an explicit policy change is required to broaden UI cancellation;
API asset metadata alone does not grant that authority.

Each random nonce and immutable create intent is retained in RAM **before** the POST.
After an uncertain response, the bot reconciles the account/auction/nonce
and exact terms against API inventory. Maintenance and bidding share one authenticated
offer inventory and one auction response. Eligibility considers live commitments
and open/solvable candidates with the relevant funding token, not historical
auctions. Positive and negative results are memoized per request/market at the
current pin. A confirmed cancellation or creation ends the pass and discards its
inventory. Historical auctions with live commitments still participate in maintenance. Unknown POST outcomes retry only the original immutable request. A definite rejection of the first POST clears that unsent intent. Rejection of a retry does not clear an earlier uncertain submission, and local expiration alone cannot prove an accepted mint authorization is gone. A confirmed offer missing from API inventory is not reposted; its principal stays reserved. Other auctions can use the remaining capacity. A pending signed transaction retains ownership. Simulation mode never submits
or cancels offers.

`AWAITING_MINT` is readiness, not funding completion. The provider reads the native
aggregate `mintAuthorization(account)` at the recipe's block, allowing several
accepted offers and later partial-fill windows. It does not derive mint amounts
from one offer row. Across every guarded group it uses the earliest eligible current API mint deadline (canonical request address breaks ties), reserves a
30-second margin and closes preparation by checking current enrollment/lease
after full simulation. The completed plan is submitted without another simulation. Earlier requests for another funding pair
or outside the current policy budget, yield floor or recipe cap are skipped so
they cannot block a later eligible request.
The SDK's full recipe simulation and owned submission checks remain mandatory. The account
charges policy, approves, mints, receives PT/YT, verifies exact cash/PT/YT/budget
deltas and clears allowance atomically. Confirmed reconciliation requires the
canonical receipt and exactly one matching policy debit. Signed retries retain
their original identities and ownership even when the external lease later expires.

Provider list responses are capped at 2 MiB and inventories at 4096 rows. IDs must
be exact positive integers below 2^24 because the generated upstream client uses
float32 IDs; monetary quantities remain integer strings and `big.Int`. Redirects
are refused. API errors expose the HTTP status, not response bodies or credentials. The canonical
OpenAPI and solver-used contract ABI entries are vendored and regenerated with the Makefile.
`make refresh-merkle-abi` projects those entries directly from the contracts build using
`hack/merkle-abi.jq`, preserving their complete tuple definitions.

NAV discovers native account PT/YT independently of API enrollment and callback
history. Open PT uses face principal; YT uses native residual value; settled claims
use the exact PT-priority waterfall. Projected APR deliberately remains unavailable
for open YT without authoritative forecast economics and settlement horizon.

Validation covers host API wire payloads, ambiguous in-process submissions,
reservations and cancellation, aggregate authorizations, expired leases, curator-owned
guards and confirmed receipts. The canonical contracts' local 3F suite is
separate from live-provider approval, authenticated mainnet enrollment and minting.

## Implementation and validation

All hand-written 3F Merkle logic lives in `internal/solvers/merkle/threef`:
API transport/JSON admission, offer sizing and RAM intent reconciliation, policy snapshots,
recipe input extraction and the mint provider. One Run goroutine evaluates guarded markets sequentially. Live commitment
maintenance precedes a full scan of relevant unanswered auctions. One API client, planner, provider and RAM sender serve one executor strategy. The SDK keeper owns mint progress. It admits no new offer work
while a signed recipe is pending. `transaction.Manager.Sender` uses the same SDK
transaction engine and signer lane as the generic recipe host and native solvers. There is no
optional 3F dispatch in `merkle` and no dependency on `bridgefacilitator` strategies.

Tests retain API wire/identity/bounds checks, ambiguous-POST retry identity,
reservations, cancellation, aggregate authorizations, lease expiry, alternative curator
guards and canonical receipt evidence. Dedicated startup tests cover mainnet and Sepolia,
shared caller discovery, configured policy/account binding, grouped recipes, cross-account rejection, reorgs, invalid accounts and failure before API mutation. Multi-auction tests cover shared cash, separate pair budgets, partial remaining capacity, RAM retry identity and cancellation isolation.
Regression tests also cover committed funding gates, exact raw JSON rate periods,
offers without a permission service, on-chain pair discovery, historical filtering, negative membership memoization,
full auction scans before bidding, maintenance before optional RPC failure and zero-budget
cancellation ownership.
Authenticated live-provider approval, enrollment and mainnet minting require
separate validation; local fixtures do not establish them.
