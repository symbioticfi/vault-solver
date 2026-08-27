# Architecture

## Dependency direction

```text
cmd/vault-solver
  ├── generic framework: config, chain, signer, txmanager, solver, observability
  └── blank imports: internal/solvers/<integration>

internal/solvers/<integration>
  ├── generic framework interfaces
  ├── neutral shared packages such as liquidlane, morpho, parse, tokenpolicy, webhook
  └── generated protocol contracts under api/
```

The generic framework must never import an integration. One integration must never import another integration.
Code becomes shared only after at least two integrations use the same protocol-neutral concept or the same
external contract.

## Composition and startup

`cmd/vault-solver` is the composition root. Concrete solvers self-register a runtime factory, pure config
validator, and external-submission capability from package `init` functions; they are blank-imported only by
the command. The same metadata powers offline validation without constructing chain, signer, or API clients.
Runtime startup is:

1. load and strictly decode the generic YAML config;
2. initialize logging, metrics, chain reads, and signer;
3. construct every configured solver from its opaque config node;
4. initialize the shared transaction manager only when at least one solver submits transactions;
5. run solvers concurrently and cancel all on the first fatal error;
6. stop external commitments, drain accepted solver work, then stop the transaction manager.

See [Transaction manager](TXMANAGER.md) for nonce ownership and shutdown details.

## Configuration ownership

The generic layer owns `chain`, `signer`, `txManager`, `observability`, and the `solvers[].name` discriminator.
It deliberately keeps each `solvers[].config` as a `yaml.Node`. The selected integration performs a second
strict decode into its own validated type. Secrets remain environment-variable names in config and are read at
the point of use.

Operational values never become Go constants merely for convenience. Deployment-specific addresses, URLs,
chain IDs, rates, intervals, limits, and secret references belong in YAML or an authoritative upstream API.

## Shared packages

- `internal/liquidlane` is shared protocol infrastructure, not generic framework. Its consumers are RFQ,
  LI.FI, OEV, and UniswapX; 3F must not be forced through it.
- `internal/morpho` contains protocol math reused independently of one solver's orchestration.
- `internal/parse`, `internal/tokenpolicy`, and `internal/webhook` are small neutral helpers;
  `internal/tenderly` provides transaction-simulation links without owning protocol behavior.
- `api/` contains generated external contracts and clients. Hand-written adapters project generated wire
  types into solver-owned domain types.

## Adding an integration

1. Add `internal/solvers/<name>` implementing `solver.Solver` and an integration-owned config parser.
2. Register its factory and pure validator through `solver.Registration`; mark externally submitted settlement
   explicitly, then blank-import it from `cmd/vault-solver/root.go`.
3. Add generated external contracts under `api/` through the vendored-artifact Make targets.
4. Add the schema variant, annotated example config, root README solver-table row, integration plan, docs-index row,
   and focused tests.
5. Run the focused and repository gates documented in [Development](DEVELOPMENT.md).

The composition-root import is expected; adding protocol branches or fields to generic packages is not.
