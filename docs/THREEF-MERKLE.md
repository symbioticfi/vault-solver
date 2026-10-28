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
      stateDir: /persistent/threef-merkle
      executor: "0x0000000000000000000000000000000000000010"
      curator:
        url: https://curator.example.invalid/api/v1
      url: https://bf.3f.xyz
      apiKeyEnv: THREEF_CURATOR_API_KEY
      broker: "0x0000000000000000000000000000000000000040"
```

The account is the maker. A dedicated, 3F-approved BF EOA and API key serve one
curator/default account. The executor's account and caller are discovered together at a
pinned block; its account must be the default maker and caller must match the shared signer. Mainnet is required.
Mode defaults to `simulate`; switch to `send` to authorize offers and mint execution.
One configuration selects one executor and BF/default account. The recipe supplies
its funding/underlying pairs and request/principal/yield Inputs. Inactive groups
receive zero principal; the recipe determines which calls execute. The solver adds
no group-count limit, canonical guard comparison, mint-gate rule, or separate
funding-gate proof. Fixed withdrawals and additional Call/Approve steps use the
curator-authorized program. Permission response and SDK input bounds still apply.
Provider identity settings participate in recovery identity.
Use one exclusive durable state directory
and one process per curator BF. Do not also list this executor in `merkle.strategies`
or share that BF/key with another independently allocating bot.

`threeF` is not a field of the generic `merkle` solver. Drain any unresolved offer
or signed work before changing solver configuration. Do not discard a journal or
use a new empty directory to evade recovery checks. The SDK restores signed owner
fences at startup before any solver runs and persists every signed original and
replacement before broadcast; the host drains the shared sender before closing stores.

Polling and mint strategy cadence are 15 seconds, operations are bounded to 30 seconds,
and the shared recipe cache holds the configured recipe within 16 MiB. Curator `url` and optional `apiKeyEnv` use the SDK's
10-second/2-MiB limits. Existing live commitments are maintained first. Each pass then
checks every relevant open/solvable auction without a blocking offer from this maker,
matching its funding asset to the configured markets. Positive and negative policy
eligibility results are reused only within the same pinned snapshot. The complete
candidate list is checked before bidding, with no per-tick check cap or rotating cursor.
Auctions are ordered by ID and markets by recipe order. Each eligible market gets
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

The host decodes the funding pair/policy and the principal, yield and request Input
bindings from the account capability. It does not reconstruct or compare its
internal guards. The SDK authenticates permissions and simulates the complete recipe.
A pinned read-only policy simulation resolves native factory/Facility
membership and verifies the exact funding pair's budget debit; an API position
manager address is never substituted for collateral underlying. This simulation
is a read, not a live transaction or proof of signer access.

Automatic offers conservatively use **idle cash already held by the account**.
Capacity is the minimum of that cash, the policy's remaining pair budget, the
recipe principal cap and the auction's advertised request size, with cash reserved across all live offers of the same funding token and budget reserved only across offers for the exact funding/underlying pair. The bot keeps at most one
self-originated live offer at a time, preserving its full configuration binding
until terminal across all configured markets. Another market cannot cancel, overwrite or rekey that live intent; an ambiguous POST blocks every market until its exact API row is reconciled. The workflow may cancel only its own intent or an eligible request within its current market. Multiple UI-originated offers and aggregate mints remain supported. Supply cash to the account
before enabling automatic bidding. The actual mint recipe can still use its
committed funding sources; API-key offers created in the UI are reconciled by the
same mint provider. This bot does not promise a quote against liquidity that may
need a later withdrawal from another market or the adapter.

Yield is the ceiling of principal times the policy's minimum divided by 1e8;
this is the nonannual yield/principal ratio. The auction's exact decimal maximum
rate bounds that offer, scaled by its `rateBasisSeconds` over a 365-day year when
provided. Both the rate and period use the original JSON numbers before generated float32
decoding. Negative, fractional and overflowing periods are rejected; an omitted,
null or zero period retains the legacy unscaled comparison. The policy
floor is not annualized. One unsigned PUSH POST uses the BF as wire `maker`, omits
`chainId`, `executionMode` and signatures, and sets `useCallback:false`. The API
stores the account as maker. A five-minute offer expiration is independent of the
post-acceptance mint lease. Unaccepted offers reserve their full principal;
accepted mint windows reserve their pending principal. Manual UI cancellations
suppress automatic replacement for that auction. Unaccepted commitments within authenticated cancellation scope that exceed
cash/budget or the bot's changed terms are cancelled before new offers are created.
At zero remaining pair budget, the native check cannot attest an unjournaled UI
request: the canonical `ThreeFPolicy.consume` requires positive principal and
`AssetRateLimits.consume` rejects disabled or exhausted budgets. The bot therefore
leaves those UI offers for explicit UI cancellation. It may still cancel its own
journaled unaccepted intent, and it never cancels an accepted mint window. Zero
budget always blocks automatic bids. A separate budget-independent membership
interface or an explicit policy change is required to broaden UI cancellation;
API asset metadata alone does not grant that authority.

Each random nonce and immutable create intent is fsynced **before** the POST.
After an uncertain response or restart, the bot reconciles the account/auction/nonce
and exact terms against API inventory. Maintenance and bidding share one authenticated
offer inventory and one auction response. Eligibility considers live commitments
and open/solvable candidates with the relevant funding token, not historical
auctions. Positive and negative results are memoized per request/market at the
current pin. A confirmed cancellation or creation ends the pass and discards its
inventory. Historical auctions with live commitments still participate in maintenance. It never blindly retries an unsigned POST.
An absent ambiguous intent remains blocked even after local expiration; investigate
provider history before manually resolving it. Never delete state to clear a
pending signed transaction. Simulation mode does not write the journal, submit
or cancel offers.

`AWAITING_MINT` is readiness, not funding completion. The provider reads the native
aggregate `mintAuthorization(account)` at the recipe's block, allowing several
accepted offers and later partial-fill windows. It does not derive mint amounts
from one offer row. Across every guarded group it uses the earliest eligible current API mint deadline (canonical request address breaks ties), reserves a
30-second margin and checks current enrollment/lease again after full simulation,
immediately before unsigned admission. Earlier requests for another funding pair
or outside the current policy budget, yield floor or recipe cap are skipped so
they cannot block a later eligible request.
The SDK's full recipe simulation and send checks remain mandatory. The account
charges policy, approves, mints, receives PT/YT, verifies exact cash/PT/YT/budget
deltas and clears allowance atomically. Confirmed reconciliation requires the
canonical receipt and exactly one matching policy debit. Signed recovery retains
its original bytes and ownership even when the external lease later expires.

Provider list responses are capped at 2 MiB and inventories at 4096 rows. IDs must
be exact positive integers below 2^24 because the generated upstream client uses
float32 IDs; monetary quantities remain integer strings and `big.Int`. Redirects
are refused and upstream error bodies/credentials are redacted. The canonical
OpenAPI and solver-used contract ABI entries are vendored and regenerated with the Makefile.
`make refresh-merkle-abi` projects those entries directly from the contracts build using
`hack/merkle-abi.jq`, preserving their complete tuple definitions.

NAV discovers native account PT/YT independently of API enrollment and callback
history. Open PT uses face principal; YT uses native residual value; settled claims
use the exact PT-priority waterfall. Projected APR deliberately remains unavailable
for open YT without authoritative forecast economics and settlement horizon.

Validation covers host API wire payloads, durable ambiguous submissions/restart,
reservations and cancellation, aggregate authorizations, expired leases, curator-owned
guards and confirmed receipts. The canonical contracts' local 3F suite is
separate from live-provider approval, authenticated mainnet enrollment and minting.

## Implementation and validation

All hand-written 3F Merkle logic lives in `internal/solvers/merkle/threef`:
API transport/JSON admission, offer sizing and journal recovery, policy snapshots,
recipe input extraction and the mint provider. One Run goroutine evaluates guarded markets sequentially. Live commitment
maintenance precedes a full scan of relevant unanswered auctions. One API client, planner, provider, exclusive store and submission sender serve one executor strategy. The SDK keeper owns mint progress. It admits no new offer work
while a signed recipe is pending. `internal/merklesender.OpenKeeper` shares only
SDK store/sender startup and recovery with the generic recipe host. There is no
optional 3F dispatch in `merkle` and no dependency on `bridgefacilitator` strategies.

Tests retain API wire/identity/bounds checks, ambiguous-POST restart recovery,
reservations, cancellation, aggregate authorizations, lease expiry, alternative curator
guards and canonical receipt evidence. Dedicated startup tests cover mainnet,
shared caller discovery, grouped recipes, cross-account rejection, reorgs, invalid accounts and failure before state creation. Cross-market tests retain live/ambiguous account intents and prevent another pair budget from cancelling a foreign UI offer.
Regression tests also cover committed funding gates, exact raw JSON rate periods,
historical filtering, negative membership memoization,
full auction scans before bidding, maintenance before optional RPC failure and zero-budget
cancellation ownership.
Authenticated live-provider approval, enrollment and mainnet minting require
separate validation; local fixtures do not establish them.
