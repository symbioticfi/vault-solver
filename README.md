# Vault Solver

A Go service that monitors configured [Symbiotic](https://symbiotic.fi) vaults and runs one or more
pluggable solver integrations. Each integration owns its protocol API, pricing, signing, reconciliation, and
settlement logic; the framework supplies shared chain access, signer, observability, lifecycle orchestration,
and a nonce-serialized transaction manager.

> **Status:** early build. Operator configuration examples live under [`config/`](config); architecture,
> protocol plans, and contributor navigation start at [`docs/README.md`](docs/README.md). Engineering rules
> are in [`CLAUDE.md`](CLAUDE.md), with `AGENTS.md` symlinked to it.

## Architecture

- `cmd/vault-solver` — CLI and process composition.
- `internal/solver` — solver interface, registry, and run wrapper.
- `internal/solvers/<name>` — self-contained protocol integrations.
- `internal/{config,chain,signer,txmanager,observability}` — integration-agnostic infrastructure.
- `internal/liquidlane` — shared LiquidLane protocol facts and planning primitives.
- `api/` — committed generated contract bindings and external API clients.

The service keeps no database. It rebuilds operational state from chain views and protocol APIs. See
[Architecture](docs/ARCHITECTURE.md) for dependency direction and package ownership.

## Solvers

Config contains one or more `solvers` entries, at most one per runtime name. All entries share the chain client
and signer. Transaction-sending integrations also share one transaction manager so a single EOA never races on
nonces; externally settled OEV does not start it.

| `solver.name` | Integration | Package | Plan | Example config |
|---|---|---|---|---|
| `3f-bridge-facilitator` | 3F bridge-loan auctions | `internal/solvers/bridgefacilitator` | [3F](docs/3F-PLAN.md) | [YAML](config/3f.example.yaml) |
| `rfq-filler` | Symbiotic RFQ quoting and filling | `internal/solvers/rfq` | [RFQ](docs/RFQ-PLAN.md) | [YAML](config/rfq.example.yaml) |
| `redstone-oev` | RedStone OEV liquidations | `internal/solvers/redstoneoev` | [OEV](docs/OEV-PLAN.md) | [YAML](config/redstone-oev.example.yaml) |
| `lifi-samechain` | LI.FI same-chain intents | `internal/solvers/lifi` | [LI.FI](docs/LIFI-PLAN.md) | [YAML](config/lifi.example.yaml) |
| `uniswapx-filler` | UniswapX V2 RFQ and public filling | `internal/solvers/uniswapx` | [UniswapX](docs/UNISWAPX-PLAN.md) | [YAML](config/uniswapx.example.yaml) |

### 3F Bridge Facilitator

Bids in 3F bridge-loan auctions through one or more `ThreeFAdapter`s, funds won loans, and redeems matured
requests. Operators may configure an explicit adapter list or discover adapters from an on-chain factory. Every
target must authorize the configured offer signer through ERC-1271; an empty factory is valid and continues to
be polled. Per-request caps and funding headroom are read on-chain.

### RFQ Filler

Serves `POST /quote`, polls awarded RFQ orders, replans against current LiquidLane state, and settles through
the RFQ Executor. `external` mode scopes work to directly authorized configured adapters; `internal` mode also
uses signed private discounts. `tokensToQuote` controls token admission, while `minAmountsIn` optionally declines
undersized requests with HTTP 204. The sender EOA must be an authorized Executor caller.

### RedStone OEV

Bids in RedStone Atom OEV auctions and, when selected, liquidates Morpho positions through a LiquidLane-backed
callback. RedStone submits settlement, so this integration signs bids but does not use the shared transaction
manager. Optional gas/oracle configuration enables after-cost economics; `maxBidWei` bounds spend and `dryRun`
observes would-bids without sending them.

### LI.FI Same-Chain Intents

Publishes standing LiquidLane-backed quotes, recovers active matches after startup/reconnect, consumes opened
escrow orders from the LI.FI feed, and settles through `LiquidLaneLifiExecutor`. `external` mode uses direct
filler authorization; `internal` mode also accepts signed discounts. Only on-chain escrow orders are supported;
gasless Compact, Permit2/3009, Dutch, and future-scheduled orders are outside the current scope. The configured
settler must report `governanceFee() == 0`.

### UniswapX Quoter and Filler

Serves the UniswapX RFQ quote webhook and polls exclusive and optional public V2 orders for profitable
LiquidLane-backed fills. `external` mode requires directly authorized adapters; `internal` mode may combine
direct routes with fresh signed discounts. Quote ingress must enforce Uniswap's source-IP policy. V2 exact-input
and exact-output orders are supported; V1, V3, native outputs, mixed-token outputs, and secondary-DEX routing are
not.

## Strategies

Each solver selects a local strategy in its own config:

- `default` — built-in in-process sizing, pricing, and route selection;
- `webhook` — sends typed facts to an operator-owned HTTP service and validates the returned plan before funds
  can move.

Protocol transport, signatures, deadlines, fresh reads, calldata, and settlement remain solver-owned. See
[Strategy architecture](docs/strategy-plan.md) for the trust boundary.

## Requirements

- Go toolchain pinned by [`go.mod`](go.mod).
- Reachable EVM RPC and a signer key referenced indirectly through config.
- `make tools` for pinned local `golangci-lint` and `abigen` under `.tools/bin`.
- Java only when regenerating OpenAPI clients; Docker/Anvil only for their respective integration tests.

## Quickstart

```bash
make tools
make doctor
make build
./bin/vault-solver version
./bin/vault-solver config validate --config config/3f.example.yaml
./bin/vault-solver run --config config/3f.example.yaml
```

Development checks:

```bash
make verify-fast TARGET=./internal/solver
make format
make verify
```

Debug logging is disabled by default. Enable it through `observability.debug: true` or explicit `--debug`:

```bash
./bin/vault-solver run --config config/3f.example.yaml --debug
```

## Configuration

The framework strictly decodes common blocks and passes each opaque `solvers[].config` node to its integration
for a second strict decode. Unknown keys fail fast. Every integration has an annotated example linked from the
solver table and associated with [`config/vault-solver.schema.json`](config/vault-solver.schema.json) for editor
completion. The schema is structural; the offline CLI is the semantic authority:

```bash
vault-solver config validate --config config.yaml
```

Validation expands non-secret environment references and checks common fields, integration config, selected
strategy config, and whether a transaction-sending process has a fee cap. It does not dial RPC/API endpoints or
read the secret values named by `*Env` fields.

Common blocks:

- `chain` — primary read RPC, optional ordered read fallbacks, one non-fallback write RPC, chain ID, and
  Multicall3 override;
- `signer` — either a private-key environment-variable name or keystore path plus passphrase-variable name;
- `txManager` — confirmations, fee cap/tip policy, broadcast/replacement/pending/shutdown timeouts;
- `observability` — `/metrics`, `/healthz`, and `/readyz` listener plus debug logging;
- `solvers` — integration names and integration-owned config blocks.

Non-secret `${VAR}` values are expanded while loading YAML. Secrets are never interpolated into the parsed
config: fields such as `keyEnv`, `passphraseEnv`, and `apiKeyEnv` name environment variables that are read only
at the point of use. Never commit a live key, endpoint, or `*.local.*` config.

### Transaction safety

Signed broadcasts and startup nonce reads use `chain.writeRpcUrl`, or the primary RPC when omitted; they never
fail over between endpoints. A transaction-sending process starts only when latest and pending nonce match.
The EOA must be exclusive to that process, including private relay submissions. An occupied or conflicted nonce
lane pauses new external commitments while accepted work and exact-hash reconciliation continue.

The manager owns fee selection, gas estimation, replacement, same-nonce cancellation, canonical receipt
confirmation, and bounded shutdown. See [Transaction manager lifecycle](docs/TXMANAGER.md) before changing or
operating this path.

## Code generation

Generated code is committed for hermetic builds. Refresh the vendored contract first, then regenerate:

```bash
make refresh-abi FORGE_OUT=../rfq/out
make refresh-openapi
make refresh-rfq-openapi
make refresh-lifi-openapi
make refresh-uniswapx-openapi
make refresh-morpho-graphql-schema
make generate
```

The complete artifact-to-command map is in [Development](docs/DEVELOPMENT.md). Never hand-edit generated Go.

## Contributing

Read [`CLAUDE.md`](CLAUDE.md), inspect the focused change map in
[`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md), unit-test new logic, and finish with:

```bash
make format
make verify
```
