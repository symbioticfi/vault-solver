# vault-solver

A Go service that monitors a configured selection of [Symbiotic](https://symbiotic.fi) vaults and
runs a pluggable **solver** strategy against them. A solver is a self-contained integration with an
external protocol that sources, prices, or routes liquidity on top of a Symbiotic vault adapter; the
bot handles discovery, pricing/signing, on-chain reads, reconciliation, and settlement for it.

The framework is **solver-agnostic**: each integration lives in its own package, registers itself,
and is selected by config — adding one never touches the generic engine. The available solvers are
described under [Solvers](#solvers) below.

> **Status:** early build. Engineering guidelines: [`CLAUDE.md`](./CLAUDE.md). Per-solver scope,
> architecture, and roadmap live under [`docs/`](docs). On-chain adapters live in the sibling `rfq`
> repo and are consumed here only via generated ABI bindings.

## Architecture at a glance

- **`cmd/vault-solver`** — process bootstrap: flags, logging, signal-driven shutdown.
- **`internal/solver`** — generic `Solver` interface, registry, and engine.
- **`internal/solvers/<name>/`** — one self-contained package per integration (today
  `bridgefacilitator/`); all protocol-specific logic lives here.
- **`internal/{config,chain,signer,txmanager}`** — solver-agnostic infra: two-stage config, vault /
  Multicall3 reads, a pluggable signer, and a nonce-serialized transaction sender shared across solvers.
- **`api/`** — committed codegen: contract `bindings/` (abigen, grouped per integration) and protocol
  API clients (e.g. the 3F `threef/` client via oapi-codegen), each refreshable from upstream.

State is intentionally minimal: open positions, redemption readiness, and liquidity are read from
on-chain views and the relevant protocol API on each tick — no database. See
[`docs/3F-PLAN.md`](docs/3F-PLAN.md) §3.

## Solvers

The running solver is chosen by `solver.name` in config; its `solver.config` block is typed and
validated by that solver (two-stage decode). Adding a solver touches **no** framework code — see the
recipe in [`CLAUDE.md`](./CLAUDE.md).

| `solver.name` | Integration | Status | Docs |
|---|---|---|---|
| `3f-bridge-facilitator` | 3F (Grunt) bridge-loan auctions | Live on Sepolia dev | [`docs/3F-PLAN.md`](docs/3F-PLAN.md) |
| _(planned)_ `rfq` | RFQ liquidity | Planned | — |
| _(planned)_ `redstone-oev` | Redstone / OEV | Planned | — |

### 3F Bridge Facilitator — `3f-bridge-facilitator`

Acts as a Bridge Facilitator in 3F's bridge-loan auctions, on top of a Symbiotic
`BridgeFacilitatorAdapter`:

- **Discover** open auctions via the 3F API (matched to a target vault by deposit asset == collateral).
- **Price & size** an offer at the auction's `maxRate`, capped by fundable vault liquidity and curator
  exposure (per-request / total-sleeve / max-concurrent).
- **Sign & submit** the offer (EIP-712), with the adapter as the on-chain maker (verified via EIP-1271
  against an owner-set offer-signer key).
- **Fund** a won loan just-in-time inside the adapter's consume callback (self-allocation from vault
  liquidity), then **redeem** repaid loans permissionlessly — realizing principal + yield back to the
  vault.

Onboarding generates a 3F facilitator API key (EIP-712) and registers the adapter as the facilitator
offer-address. Because 3F allows exactly **one offer-address per facilitator**, this solver serves a
**single `vault` + `adapter` pair**. The on-chain `BridgeFacilitatorAdapter` lives in the sibling
`rfq` repo, consumed via `api/bindings/3f/`. Config block: `apiBaseUrl`, `apiKeyEnv`, `minReturnBps`,
`vault`, `adapter`, `exposure`, `intervals` — see
[`config/config.example.yaml`](config/config.example.yaml). Design, decisions, and the live TODO
list: [`docs/3F-PLAN.md`](docs/3F-PLAN.md).

## Requirements

- Go (toolchain version pinned in [`go.mod`](./go.mod); auto-fetched by recent Go releases).
- For regenerating codegen: `make tools` (installs pinned `abigen`, `oapi-codegen`, `golangci-lint`).
- A reachable EVM RPC endpoint and a signing key (see Configuration).

## Quickstart

```bash
make build            # build ./bin/vault-solver
./bin/vault-solver version
make test             # go test -race -cover ./...
make lint             # golangci-lint
./bin/vault-solver run --config config/sepolia.yaml
```

The CLI is built with [Cobra](https://github.com/spf13/cobra); run `vault-solver --help` for the
command list (`run`, `version`). Debug logging is off by default; enable it with
`observability.debug: true` in config or the `--debug` flag (the flag wins):

```bash
./bin/vault-solver run --config config/sepolia.yaml --debug
```

## Configuration

Config is YAML with a two-stage decode: the framework reads `solver.name` to select the
implementation and hands the opaque `solver.config` block to that solver to type. A documented
example lives at `config/config.example.yaml` (per-instance vault selection, exposure caps,
intervals). **Never commit a real key or live config** — keys are supplied via env/file behind the
`Signer` interface; `*.local.*` and `.env` are gitignored.

## Code generation

Generated code is committed for hermetic builds; refresh from upstream on demand:

```bash
make refresh-abi FORGE_OUT=../rfq/out   # re-vendor contract ABIs from a Foundry build
make refresh-openapi                    # re-pull the live 3F OpenAPI spec
make generate                           # regenerate bindings + API client
```

## Contributing

Engineering conventions — the modular framework/integration boundary, config-driven configuration,
modern Go 1.26 style, the required test/lint/format gate, and secure-coding rules — are in
[`CLAUDE.md`](./CLAUDE.md) ([`AGENTS.md`](./AGENTS.md) points there). Every change must keep
`make format && make test && make lint` green and unit-test new logic.