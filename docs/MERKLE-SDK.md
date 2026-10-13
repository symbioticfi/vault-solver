# Merkle execution host

The `merkle` solver consumes the independently versioned
`github.com/symbioticfi/merkle-executor/sdk` module pinned in go.mod. No daemon
package, protocol provider, transaction engine or legacy vault-solver code is
copied from merkle-executor. The SDK implements recipe/tree/proof/role checks,
pinned runtime assembly, simulation, canonical receipts and durable recovery.

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
address are rejected. Old strategy objects, custom labels and per-strategy options
are rejected before network access. The unrelated `solvers[].name` still selects
the solver integration.

The address-only policy version is `address-only-v3`. Finish active jobs with the
previous binary and configuration before upgrading, including terminal jobs whose
shared sender remains occupied. Old custom or prefixed keys remain in inventory
and fence unresolved ownership. This schema never migrates or aliases old journals;
the shared immutable SDK pin remains unchanged.

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
The active root determines the requested permission artifact; the SDK checks the
chain, root, leaf hashes and every proof before preparation. Amount inputs use an
exact read-dependent equality when present, otherwise the greatest amount allowed
by all preceding recipe bounds, with at least one read-dependent upper bound.
The SDK checks every lower bound and cap. Unconstrained inputs
and the old account, tree and inputs configuration fields are rejected. Finish all
active jobs with the previous binary and configuration before this migration.
Polling, operation timeout, strategy cadence and the 16 MiB recipe cache use fixed
bounded defaults.

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
remain business logic in vault-solver. This extraction does not claim those
auction integrations already execute through Merkle connectors.

The repository is private. Go builds need read access to merkle-executor;
CI uses MERKLE_SDK_READ_TOKEN with contents:read on that repository. Docker uses
a BuildKit secret, never a build argument or an image-layer credential. Set the
read token in MERKLE_SDK_READ_TOKEN before `make docker`; the target passes it
through the secret mount. Local
users can use their normal GitHub credential helper with GOPRIVATE set to the
Merkle module prefix. Consumers use the immutable Go module requirement, with
no production replace or dependency on a sibling checkout.
