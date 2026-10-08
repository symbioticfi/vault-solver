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
      pollInterval: 15s
      operationTimeout: 30s
      strategies:
        - name: rebalance
          executor: "0x..." # deployed RecipeExecutor
          account: "0x..." # its MerkleAccount
          recipeHash: "0x..." # optional expected commitment
          tree: /config/permissions.json
          interval: 1m
          inputs: {0: "1000000"} # decimal uint256, token base units
```

Paths resolve from the process working directory; use absolute paths for mounted
configuration and persistent state. The regular chain/signer/txManager settings
still apply. Send mode requires the shared signer to match the on-chain executor
caller and hold the required leaf roles. Configuration and amounts fingerprint
active work; do not remove or change a workflow while it owns unresolved work.
Mount durable state and exclude concurrent processes with this signer, even if
they use different state directories. A process-local lane cannot coordinate a
second service independently using the same private key.

This first host supplies configured bounded inputs; the committed recipe reads
live liquidity and enforces its arithmetic and call bounds. SDK provider
Prepare/Advance interfaces allow host integrations to supply fresh auction
amounts and reconcile confirmed executions. LL/3F auction discovery, pricing,
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
