# Merkle execution host

The `merkle` solver consumes the independently versioned
`github.com/symbioticfi/merkle-executor/sdk` module pinned in go.mod.
The SDK implements recipe/tree/proof/role checks,
pinned runtime assembly, simulation, canonical receipts and durable recovery.

Awarded LiquidLane connector orders run in the separate [`rfq-merkle` solver](RFQ-MERKLE.md).
Its RecipeExecutor caller is the RFQ Executor contract, so it must not be included in
`merkle.strategies`, whose caller is the shared EOA. These integrations share the SDK
and transaction manager; their liquidity and scheduling are independent.

This solver receives the same transaction manager and signer as all other
solvers. The SDK's transport is a thin bridge to that manager's Owner and
BeforeBroadcast hooks. Every signed original and fee replacement is persisted
before broadcast. A journal owner fences new admission by other solvers until
durable business acknowledgement. Recovery restores the fence during factory
construction, before the shared sender or any solver runs; it never replays a
stored recipe. Simulation mode refuses a recovered signed job rather than
allowing the other solvers to reuse its signer lane. Stores close after the
shared sender's shutdown drain, including on partial startup failure.

Add this solver alongside the existing solvers in the normal YAML file:

```yaml
solvers:
  - name: merkle
    config:
      mode: simulate # switch to send for configured, authorized execution
      stateDir: /persistent/merkle-state
      curator:
        url: https://curator-backend.example.invalid/api/v1
        # apiKeyEnv: CURATOR_API_TOKEN
      strategies:
        - "0x0000000000000000000000000000000000000010" # deployed RecipeExecutor
```

`strategies` is a list of full nonzero executor addresses. Each canonical lowercase
address, including `0x`, is the durable state and log identifier. Order does not
change identity; duplicate addresses and different checksum spellings of the same
address are rejected. Strategy objects, custom labels and per-strategy options
are rejected before network access. The unrelated `solvers[].name` still selects
the solver integration.

The supported runtime policy version is `1`. Unresolved journals fence ownership and
retain their original identity and configuration. The shared SDK is pinned to an immutable version.

Paths resolve from the process working directory; use absolute paths for mounted
configuration and persistent state. The regular chain/signer/txManager settings
still apply. Send mode requires the shared signer to match the on-chain executor
caller; the executor must hold the required leaf roles. Startup reads the executor
account and caller together at one pinned block,
deduplicates repeated executors and checks that block again. Every caller must
match the existing shared signer before state opens. Derived identity, curator
source and the fixed runtime policy version fingerprint active work; do not remove
or change a workflow while it owns unresolved work.
Mount durable state and exclude concurrent processes with this signer, even if
they use different state directories. A process-local lane cannot coordinate a
second service independently using the same private key.

The host uses the SDK's recipe input provider and shared curator permission source.
The active root determines the requested permission artifact at
`GET /merkle/{adapter}/{root}` under the configured curator API URL; the SDK checks
the chain, adapter, account, root, leaf hashes and every proof before preparation. Financial amounts
come from the recipe's read-dependent computation, bound directly as a result or
through an exact input equality. Bounds validate the chosen amount; they do not
choose it. For example, a recipe can compute `min(balance, liquidity)` without
embedding either actual amount. The SDK checks every lower bound and cap.
Unconstrained inputs and manual account, tree and inputs configuration fields
are rejected.
Polling is fixed at 15 seconds, operation timeout at 30 seconds, strategy cadence at
one minute and the recipe cache at 16 MiB. YAML fields `pollInterval` and
`operationTimeout` are rejected, including values equal to these fixed limits.
Curator settings accept only `url` and `apiKeyEnv`. Its HTTP timeout is fixed at
10 seconds and response limit at 2 MiB; `timeoutSeconds` and `maxResponseBytes`
are rejected even when set to zero or those fixed limits.

Readiness comes from the authenticated recipe. After the complete original executor
simulation and canonical closing checks succeed, a recipe with no active Manage
actions and no Call or Approve steps is idle. A separate readiness read can gate a
Manage action while its bound request ID is zero; the host assigns no protocol
meaning to that ID. Unsigned admission rechecks readiness against current state.
Malformed simulation results, failed recipe checks and RPC failures remain errors.
Signed recovery continues reconciling the exact persisted transaction bytes.
The recipe's own checks define the on-chain completion conditions; this host does
not infer settlement from issuer selectors or off-chain status.

Reviewed caller-independent getters use the standard Multicall3 address at the
pinned height. Independent calls, headers, logs and receipts share bounded JSON-RPC
arrays through vault-solver's existing HTTP fallback and tracing transport. Safe
aggregation and exact argument encoding come from the shared SDK implementation;
full simulations preserve their caller and remain isolated eth_calls. Missing
aggregate deployments and malformed batch responses fail without individual-call
retries. This SDK host requires HTTP(S) read endpoints; ordinary solvers retain
their existing WebSocket/IPC support.

Read-dependent numeric values are resolved in one same-address code-override lens
call. Authenticated Manage/Approve/Call instructions execute in order before later
reads when their runtime data and input sources are already available. Prefixes
requiring unresolved provider outputs, future input sources or self-queries against
the overridden executor fail before external quote requests. The original executor's
complete simulation verifies the resolved values and canonical guards still run.
An RPC without state-override support cannot prepare a recipe requiring these
values. SDK provider Prepare/Advance interfaces allow host integrations to supply
fresh auction data and reconcile confirmed executions. LL/3F auction discovery, pricing,
signed offers, RFQ domain compatibility, reservations and order reconciliation
remain business logic in vault-solver. The native 3F PUSH integration below supplies
that host logic; LiquidLane auction workflows remain a separate integration.

The repository is private. Go builds need read access to merkle-executor;
CI uses MERKLE_SDK_READ_TOKEN with contents:read on that repository. Docker uses
a BuildKit secret, never a build argument or an image-layer credential. Set the
read token in MERKLE_SDK_READ_TOKEN before `make docker`; the target passes it
through the secret mount. Local
users can use their normal GitHub credential helper with GOPRIVATE set to the
Merkle module prefix. Consumers use the immutable Go module requirement, with
no production replace or dependency on a sibling checkout.

## 3F API-key PUSH through MerkleAccount

The account is the maker. A dedicated, 3F-approved BF EOA and API key serve one
curator/default account. Add this optional block to the Merkle solver's `config`:

```yaml
threeF:
  executor: "0x0000000000000000000000000000000000000010"
  url: https://bf.3f.xyz
  apiKeyEnv: THREEF_CURATOR_API_KEY
  broker: "0x0000000000000000000000000000000000000040"
```

The executor must already be in `strategies`; its account and caller are discovered,
never supplied here. Mainnet is required. One block selects one 3F executor per
process and BF/default account. All provider settings participate in recovery
identity. Keep one exclusive durable state directory and one process per curator BF.
Do not share that BF/key with another independently allocating bot.

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

The host recognizes the complete canonical guarded leaf generated by
`ThreeFGuardedMint` at adapters commit `0427c657375512a87158010c323556739fe96d1b`.
It compares every pinned operation, query, check and mask, derives the account,
funding pair/policy and all eleven input bindings, and validates active permissions
and roles. A pinned read-only policy simulation resolves native factory/Facility
membership and verifies the exact funding pair's budget debit; an API position
manager address is never substituted for collateral underlying. This simulation
is a read, not a live transaction or proof of signer access.

Automatic offers conservatively use **idle cash already held by the account**.
Capacity is the minimum of that cash, the policy's remaining pair budget, the
recipe principal cap and the auction's advertised request size, less existing
same-funding-token offers across all target assets. The bot keeps at most one
self-originated live offer at a time, preserving its full configuration binding
until terminal; multiple UI-originated offers and aggregate mints remain supported. Supply cash to the account
before enabling automatic bidding. The actual mint recipe can still use its
committed funding sources; API-key offers created in the UI are reconciled by the
same mint provider. This bot does not promise a quote against liquidity that may
need a later withdrawal from another market or the adapter.

Yield is the ceiling of principal times the policy's minimum divided by 1e8;
this is the nonannual yield/principal ratio. The auction's exact decimal maximum
rate bounds that offer. One unsigned PUSH POST uses the BF as wire `maker`, omits
`chainId`, `executionMode` and signatures, and sets `useCallback:false`. The API
stores the account as maker. A five-minute offer expiration is independent of the
post-acceptance mint lease. Unaccepted offers reserve their full principal;
accepted mint windows reserve their pending principal. Manual UI cancellations
suppress automatic replacement for that auction. Unaccepted commitments exceeding
cash/budget or the bot's changed terms are cancelled before new offers are created.

Each random nonce and immutable create intent is fsynced **before** the POST.
After an uncertain response or restart, the bot reconciles the account/auction/nonce
and exact terms against API inventory. It never blindly retries an unsigned POST.
An absent ambiguous intent remains blocked even after local expiration; investigate
provider history before manually resolving it. Never delete state to clear a
pending signed transaction. Simulation mode does not write the journal, submit
or cancel offers.

`AWAITING_MINT` is readiness, not funding completion. The provider reads the native
aggregate `mintAuthorization(account)` at the recipe's block, allowing several
accepted offers and later partial-fill windows. It does not derive mint amounts
from one offer row. It uses the earliest current API mint deadline, reserves a
30-second margin and checks current enrollment/lease again after full simulation,
immediately before unsigned admission. Earlier requests for another funding pair
or outside the current policy budget, yield floor or recipe cap are skipped so
they cannot block a later eligible request.
The unchanged full recipe simulation and send checks remain mandatory. The account
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
reservations and cancellation, aggregate authorizations, expired leases, complete
guard matching and confirmed receipts. The canonical contracts' local 3F suite is
separate from live-provider approval, authenticated mainnet enrollment and minting.
