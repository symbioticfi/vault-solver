# Merkle execution host

The `merkle` solver consumes the independently versioned
`github.com/symbioticfi/merkle-executor/sdk` module pinned in go.mod.
The current pin is `997a561d872caeb5dac83d6b6c28a210e10828e6`
(`v0.0.0-20261108192150-997a561d872c`).
The SDK implements recipe/tree/proof/role checks,
pinned runtime assembly, simulation, canonical receipts and RAM settlement tasks.
It also exposes opt-in off-chain providers, RPC, signing and the shared transaction
manager used by both services. Hosts explicitly supply credential
lookup, so importing or constructing SDK components never implicitly reads the
process environment.

`keeper.Planner.PrepareExecution` returns the same execution-plan format for direct
calls and EIP-7702 batches across executors. The SDK detects delegation, simulates
the complete plan and retains detailed construction results. The host submits the
prepared transaction without another simulation. Signed transactions retain their exact
identity and process-local transaction ownership through in-process retries. All batch members
receive the same canonical receipt before settlement advances. At the recipe layer,
`DiscoverBatch` and `ValidatePreparedBatch` serve both one executor and many.
Generic planning, external RFQ admission and signing share `txManager.maxGas`
(default 16,777,216), further bounded by the current block gas limit.

Awarded LiquidLane connector orders run in the separate [`rfq-merkle` solver](RFQ-MERKLE.md).
Its RecipeExecutor caller is the RFQ Executor contract, so it must not be included in
`merkle.strategies`, whose caller is the process EOA. These integrations use the common SDK; workers configured
in one process use its transaction manager. Their liquidity and scheduling remain integration-owned.
`internal/merklehost.Curator` supplies their shared permission-service settings and
redirect-rejecting HTTP transport; each solver keeps its transport timeout.

The SDK reads the executor's `recipeHash` and `recipeBlock` at a pinned block,
then retrieves `SetRecipe` only from that known publication block and executor.
It verifies the original body against the current commitment and checks the
canonical snapshot. It does not scan historical block ranges. Generic, RFQ and
3F hosts use their existing shared chain reader; no recipe indexer URL is required.
Permission data still uses the curator service.

Each Merkle solver process has its own signer and SDK `transaction.Manager`. The host maps YAML, supplies RPC/signing dependencies and logger, then explicitly
initializes, starts and drains that manager. Merkle keepers use
`manager.Sender(name)`; the RFQ Merkle integration uses the same SDK lifecycle for its outer call. The SDK owns
nonce admission, fee policy, signing, replacements and canonical receipt checks.
Signed originals and replacements are retained in its single RAM lifecycle
before broadcast. Keeper ownership survives ambiguous outcomes until a canonical
owned result. Native solvers keep their existing `internal/txmanager`, fee policy, retries, telemetry and lifecycle.
Deploy each solver process with its own signer; the two lifecycle implementations do not coordinate nonce ownership.

Select this integration in the solver process YAML file:

```yaml
solvers:
  - name: merkle
    config:
      mode: simulate # switch to send for configured, authorized execution
      curator:
        url: https://curator-backend.example.invalid/api/v1
      strategies:
        - "0x0000000000000000000000000000000000000010" # deployed RecipeExecutor
```

`strategies` is a list of full nonzero executor addresses. Each canonical lowercase
address, including `0x`, is the strategy and log identifier. Order does not
change identity; duplicate addresses and different checksum spellings of the same
address are rejected. Strategy objects, custom labels and per-strategy options
are rejected before network access. The unrelated `solvers[].name` still selects
the solver integration.

List order sets processing priority. Each later executor sees earlier confirmed
effects; order does not reserve funds. Spending recipes must protect withdrawal
obligations. Pending transaction reconciliation runs first while preserving the remaining
order, and unsettled asynchronous work keeps its account reserved.

Candidate failures are reported individually while authenticated healthy candidates
continue through shared planning. A keeper cycle can settle immediately confirmed
work and submit other eligible executors, submitting each at most once per cycle.
Idle candidates may be prepared again after earlier confirmed effects.

The SDK automatically batches self-contained, state-driven recipes on the same
executor while successive executions remain useful and executable. Every cycle
includes the complete committed recipe, including its final collateral supply.
Constant action sets execute once. External inputs are prepared once; a provider
can return separately bound outputs for individual claims. Quotes, proofs and
result witnesses are never reused to extend a state-driven cycle. There is no
host loop, protocol-specific batching or per-executor switch. The existing chain
client supplies code overrides and pinned bytecode reads; the SDK revalidates the
exact atomic multicall against the original contract before submission.

Remove `stateDir` from Merkle YAML: strict decoding rejects it. There is no journal,
configuration fingerprint, saved plan or restart recovery. Process exit loses
transaction, order and reservation tracking while external work may continue.
Settle existing work before migration and use one active process per signer.

The process manager checks fresh sender funding against final maximum gas cost
before signing. An initial funding failure captures no signed attempt
and releases the Merkle reservation through the existing unsigned-failure path.
Funding changes after a broadcast do not release signed ownership; a failed fee
replacement keeps the earlier signed identity.

Chain and signer settings remain shared. Merkle maps `txManager.confirmations`,
`maxFeeGwei`, `maxGas`, `replacementIntervalMs` and `shutdownTimeoutMs` to the SDK.
The SDK polls every two seconds and bounds each submission RPC to five seconds.
Native horizon, pending-timeout, account-polling and late-receipt settings configure
only the native engine. Send mode requires the
process signer to match the on-chain executor caller; the executor must hold the
required leaf roles. Startup reads account and caller together at one pinned block,
deduplicates executors and closes that pin before accepting the identities.

The host registers the SDK's shared Aave V3 capacity provider and uses the shared
curator permission source. The planner derives deterministic recipe inputs directly.
The active root determines the requested permission artifact at
`GET /merkle/{adapter}/{root}` under the configured curator API URL; the SDK checks
the chain, adapter, account, root, leaf hashes and every proof before preparation. Financial amounts
come from the recipe's deterministic computation or fixed literal, bound directly or
through an exact input equality. Bounds validate the chosen amount; they do not
choose it. For example, a recipe can compute `min(balance, liquidity)` without
embedding either actual amount. The SDK checks every lower bound and cap.
For native Aave borrowing, the shared provider supplies capacity after replaying
the authenticated supply prefix at the pinned block. The recipe still selects the
actual amount through its `Min` chain and checks the same pool's
`getUserAccountData(account)` health word after borrowing. Pool, asset and account
are pinned, and the recipe commits a health floor of at least `1e18`; vault-solver adds no
Aave sizing implementation or configuration.
Unconstrained inputs and manual account, tree and inputs configuration fields
are rejected. Unused `"nonzero"` payable permission markers are accepted; selecting
a native-value leaf for recipe execution remains rejected.

Polling is fixed at 15 seconds, operation timeout at 30 seconds, strategy cadence at
one minute and the recipe cache at 16 MiB. YAML fields `pollInterval` and
`operationTimeout` are rejected, including values equal to these fixed limits.
Curator settings accept only `url`. Its HTTP timeout is fixed at
10 seconds and response limit at 2 MiB; `timeoutSeconds` and `maxResponseBytes`
are rejected even when set to zero or those fixed limits.

For generic execution, readiness comes from the authenticated recipe. After the complete original executor
simulation and canonical closing checks succeed, a recipe with no active Manage
actions and no Call or Approve steps is idle. A separate readiness read can gate a
Manage action while its bound request ID is zero; the host assigns no protocol
meaning to that ID. Each new plan is prepared against current state.
Malformed simulation results, failed recipe checks and RPC failures remain errors.
RFQ admits its exact inner call through the same identity, role and canonical-pin
checks but simulates the complete outer fill from the actual signer using
`ValidatePreparedBatchWithSimulation`. It supplies no numerical witness and uses
successful awarded settlement as its work condition. No nested result probe or
RFQ code-override capability is required.
Signed retries continue reconciling the exact retained transaction identities.
The recipe's own checks define the on-chain completion conditions; this host does
not infer settlement from issuer selectors or off-chain status.

Reviewed caller-independent getters use the standard Multicall3 address at the
pinned height. Independent calls, headers, publication logs and receipts share bounded JSON-RPC
arrays through vault-solver's existing HTTP fallback and tracing transport. Safe
aggregation and exact argument encoding come from the shared SDK implementation;
full simulations preserve their caller and remain isolated eth_calls. Missing
aggregate deployments and malformed batch responses fail without individual-call
retries. All chain RPC endpoints use HTTP(S), so every SDK batch shares the
same HTTP envelope validation, tracing and cross-endpoint read failover.

Read batches retain null-receipt/header failover only when every such member is null and the whole
response has valid matching IDs and no member errors. A nonnull receipt/header retains the entire
primary response, including absent replacement attempts. Pending lookup arrays carry the same eRPC
empty-retry opt-out as scalar lookups. Inspection preserves the SDK's 256-member/32-MiB envelope,
independently of the smaller metrics prefix. Complete malformed envelopes are left to the SDK;
response-body I/O failures follow ordinary read fallback.

`make test-merkle-sdk-anvil` exercises the SDK manager through host signing and
public sender APIs on a local chain. It checks signed identity capture, ambiguous
submission, canonical receipts, replacements and retained-work admission. CI runs
it in the existing Foundry-equipped integration job.

Read-dependent numeric values are resolved in one same-address code-override lens
call. Authenticated Manage/Approve/Call instructions execute in order before later
reads when their runtime data and input sources are already available. Prefixes
requiring unresolved provider outputs, future input sources or self-queries against
the overridden executor fail before external quote requests. The original executor's
complete simulation verifies the resolved values and canonical guards still run.
An RPC without state-override support cannot prepare a recipe requiring these
values. The SDK's `keeper.Provider` interface lets host integrations declare
authenticated recipe requirements and prepare fresh inputs. Optional typed
`provider.Task` values reconcile canonical executions and retain progress in RAM. LL/3F auction discovery, pricing,
signed offers, RFQ domain compatibility, reservations and order reconciliation
remain business logic in vault-solver. The independent [`threef-merkle`](THREEF-MERKLE.md) and [`rfq-merkle`](RFQ-MERKLE.md)
solvers supply that host logic. `merkle` rejects protocol-specific configuration.

The SDK is a public Go module. Standard `go mod download`, CI and `make docker`
use the immutable requirement and go.sum checksums through the public Go proxy.
No GitHub token, private-module configuration or sibling checkout is required.
If developing with a local `go.work`, set `GOWORK=off` to verify the pinned module.
Docker excludes workspace files so images always use that dependency.

## Transaction observability

The host explicitly registers the SDK's small metric set under `solver_bot_merkle_`:

- `transaction_requests_total{solver,label,outcome}` for terminal request outcomes.
- `transaction_inflight{solver,label}` for admitted work still awaiting a result.
- `transaction_broadcasts_total{solver,label,kind,outcome}` for initial sends, replacements and rebroadcasts.
- `transaction_gas_used_total{solver,label,outcome}` and `transaction_fee_paid_wei_total` for canonical costs.
- `transaction_unresolved_nonce` for retained signed ownership, including after an SDK caller returns.

Receipt costs are recorded once after the configured canonical confirmation depth,
including a retained attempt that wins after its caller has returned. Metrics never
change order state or authorize settlement. There is no SDK account sampler or
separate receipt-observer queue. Existing native transaction metrics and dashboards
remain unchanged.
