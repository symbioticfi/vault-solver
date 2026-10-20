# Merkle execution host

The `merkle` solver consumes the independently versioned
`github.com/symbioticfi/merkle-executor/sdk` module pinned in go.mod.
The SDK implements recipe/tree/proof/role checks,
pinned runtime assembly, simulation, canonical receipts and durable recovery.
It also exposes opt-in off-chain providers, RPC, signing and a standalone sender;
this host keeps its existing shared sender. Hosts explicitly supply credential
lookup, so importing or constructing SDK components never implicitly reads the
process environment.

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

The host registers the SDK's shared Aave V3 capacity provider before its generic
recipe input provider, and uses the shared curator permission source.
The active root determines the requested permission artifact at
`GET /merkle/{adapter}/{root}` under the configured curator API URL; the SDK checks
the chain, adapter, account, root, leaf hashes and every proof before preparation. Financial amounts
come from the recipe's read-dependent computation, bound directly as a result or
through an exact input equality. Bounds validate the chosen amount; they do not
choose it. For example, a recipe can compute `min(balance, liquidity)` without
embedding either actual amount. The SDK checks every lower bound and cap.
For guarded Aave borrowing, the shared provider supplies capacity after replaying
the authenticated supply prefix at the pinned block. The recipe still selects the
actual amount through its `Min` chain and enforces its health floor. Pool, asset,
account and health policy come from the committed permission; vault-solver adds no
Aave sizing implementation or configuration.
Unconstrained inputs and manual account, tree and inputs configuration fields
are rejected. Unused `"nonzero"` payable permission markers are accepted; selecting
a native-value leaf for recipe execution remains rejected.

The Aave provider changes the host's durable provider roster. Finish or reconcile
active Merkle jobs with their existing binary before upgrading. Incompatible
pending journals fail closed; never clear state to bypass ownership checks.
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
remain business logic in vault-solver. The independent [`threef-merkle`](THREEF-MERKLE.md) and [`rfq-merkle`](RFQ-MERKLE.md)
solvers supply that host logic. `merkle` rejects protocol-specific configuration.

The repository is private. Go builds need read access to merkle-executor;
CI uses MERKLE_SDK_READ_TOKEN with contents:read on that repository. Docker uses
a BuildKit secret, never a build argument or an image-layer credential. Set the
read token in MERKLE_SDK_READ_TOKEN before `make docker`; the target passes it
through the secret mount. Local
users can use their normal GitHub credential helper with GOPRIVATE set to the
Merkle module prefix. Consumers use the immutable Go module requirement, with
no production replace or dependency on a sibling checkout.
