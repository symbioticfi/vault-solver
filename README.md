# Vault Solver

A Go service that monitors a configured selection of [Symbiotic](https://symbiotic.fi) vaults and
runs a pluggable **solver** against them. A solver is a self-contained integration with an external
protocol that sources, prices, or routes liquidity on top of a Symbiotic vault adapter; the bot
handles discovery, pricing/signing, on-chain reads, reconciliation, and settlement for it.

The framework is **solver-agnostic**: each integration lives in its own package, registers itself,
and is selected by config — adding one never touches the generic engine. The available integrations
are listed under [Solvers](#solvers).

> **Status:** early build. Engineering guidelines: [`CLAUDE.md`](./CLAUDE.md). Per-solver scope,
> architecture, and roadmap live under [`docs/`](docs).

## Architecture at a glance

- **`cmd/vault-solver`** — process bootstrap: flags, logging, signal-driven shutdown.
- **`internal/solver`** — generic `Solver` interface, registry, and engine.
- **`internal/solvers/<name>/`** — one self-contained package per integration; all protocol-specific
  logic lives here.
- **`internal/{config,chain,signer,txmanager}`** — solver-agnostic infra: two-stage config, vault /
  Multicall3 reads, a pluggable signer, and a shared nonce dispatcher with concurrent transaction
  confirmation/replacement trackers.
- **`api/`** — committed codegen: contract `bindings/` (abigen) and protocol API clients, each
  refreshable from upstream.

State is intentionally minimal — positions, liquidity, and readiness are read from on-chain views and
the relevant protocol API on each tick; no database.

## Solvers

Solvers are listed in config under `solvers:` — one or more, **at most one entry per solver type**.
Every solver in the process shares the chain client, signer, and the single nonce-owning
`txManager`, so multiple solvers on one EOA never race on nonces. Each entry's `config` block is typed
and validated by its own solver. Adding a solver touches **no** framework code — see the recipe in
[`CLAUDE.md`](./CLAUDE.md).

| `solver.name` | Integration | Docs | Example config |
|---|---|---|---|
| `3f-bridge-facilitator` | 3F (Grunt) bridge-loan auctions | [plan](docs/3F-PLAN.md) | [yaml](config/3f.example.yaml) |
| `rfq-filler` | Symbiotic RFQ quoting + order filling | [plan](docs/RFQ-PLAN.md) | [yaml](config/rfq.example.yaml) |
| `redstone-oev` | RedStone OEV liquidations | [plan](docs/OEV-PLAN.md) | [yaml](config/redstone-oev.example.yaml) |

The `3f-bridge-facilitator`, `rfq-filler`, and `redstone-oev` solvers expose a pluggable
**strategy** — the built-in `default` or an external `webhook` you run; see
[Strategies](#strategies).

### 3F Bridge Facilitator — `3f-bridge-facilitator`

Acts as a Bridge Facilitator in **[3F (Grunt)](https://3f.xyz)**'s bridge-loan auctions, on top of one
or more Symbiotic `BridgeFacilitatorAdapter`s. 3F auctions the right to front a bridge loan; this solver bids on behalf
of its adapters, funds the loans it wins just-in-time, and permissionlessly redeems repaid loans back
to the vault with yield.

It holds no API key: each adapter is registered with 3F by its vault creator, who sets this solver's
signer as the adapter's EIP-1271 signer, so offers are authorized by signature alone. Design, config,
and roadmap: [`docs/3F-PLAN.md`](docs/3F-PLAN.md) · example
[`config/3f.example.yaml`](config/3f.example.yaml). Signed offers default to a lifetime of twice the
discovery interval and cannot be configured to expire before the next discovery pass.

### RFQ Filler — `rfq-filler`

An externally-owned solver/executor for **[Symbiotic RFQ](https://symbiotic.fi)**, on top of per-vault
`LiquidLaneAdapter`s. It runs a `POST /quote` server that prices swaps for the RFQ backend and a poller
that fills the orders it is awarded, settling on-chain through the adapter.

It runs either in `external` mode (the open-source filler; quoting and filling scoped to the operator's
own adapters) or `internal` mode (Symbiotic-internal; adds the private discounts flow). The caller EOA
must be an authorized caller of the RFQ `Executor` (its `setCallers` allowlist, granted by the owner).
Design, config, and roadmap:
[`docs/RFQ-PLAN.md`](docs/RFQ-PLAN.md) · example
[`config/rfq.example.yaml`](config/rfq.example.yaml).

### RedStone OEV — `redstone-oev`

An off-chain bidder for **[RedStone Atom OEV](https://docs.redstone.finance/docs/oev)** auctions. When a
price update makes a **[Morpho Blue](https://morpho.org)** position liquidatable, RedStone runs a
sub-second WebSocket auction for the right to be the liquidator; this solver bids, and on winning, its
signed payload is bundled atomically with the price update and the liquidation.

On settlement it liquidates the position and exits the seized collateral through a single Symbiotic
`LiquidLaneAdapter`, realizing the spread and paying its bid. It signs and bids but never submits the
settlement transaction — RedStone's auctioneer does. The solver config owns the RedStone Executor,
LiquidLane adapter, and callback address; the selected strategy owns the callback-specific
`operationData`. Operators can set `maxBidWei` as a per-auction spend ceiling over any strategy; it is
required for the external `webhook` strategy and optional for the built-in `default`. Design, config,
and roadmap:
[`docs/OEV-PLAN.md`](docs/OEV-PLAN.md) · example
[`config/redstone-oev.example.yaml`](config/redstone-oev.example.yaml).

### Strategies

The solvers split protocol plumbing (reads, signing, submission — fixed) from the
**decision** — how to size, price, and select — which is a pluggable *strategy*, chosen in config:

- **`default`** — the built-in in-process strategy for that solver.
- **`webhook`** — delegates each decision to an **external HTTP service you run**: the solver sends it
  the raw facts as JSON and executes the plan it returns, so your service owns the logic.

This is the seam for customizing a solver without forking. Contract and trust model:
[`docs/strategy-plan.md`](docs/strategy-plan.md).

## Requirements

- Go (toolchain version pinned in [`go.mod`](./go.mod); auto-fetched by recent Go releases).
- For regenerating codegen: `make tools` (installs pinned `abigen`, `golangci-lint`). OpenAPI clients use
  the Java openapi-generator, downloaded on demand by `hack/openapi-generator-cli.sh` (needs a JRE).
  Its 7.12.0 JAR is verified with SHA-256
  `33e7dfa7a1f04d58405ee12ae19e2c6fc2a91497cf2e56fa68f1875a95cbf220` before execution.
- A reachable EVM RPC endpoint and a signing key (see Configuration).

## Quickstart

```bash
make build            # build ./bin/vault-solver
./bin/vault-solver version
make test             # go test -race -cover ./...
make lint             # golangci-lint
./bin/vault-solver run --config config/3f.example.yaml
```

The CLI is built with [Cobra](https://github.com/spf13/cobra); run `vault-solver --help` for the
command list (`run`, `version`). Debug logging is off by default; enable it with
`observability.debug: true` in config or the `--debug` flag (the flag wins):

```bash
./bin/vault-solver run --config config/3f.example.yaml --debug
```

## Configuration

Config is YAML with a two-stage decode: the framework reads `solver.name` to select the
implementation and hands the opaque `solver.config` block to that solver to type. Each solver has its
own fully annotated example under `config/` (see the *Example config* column above) — every field,
including the shared `chain`/`signer`/`txManager`/`observability` blocks, is documented inline there.
The `chain` block takes a primary `rpcUrl` plus optional `rpcFallbackUrls` — HTTP(S) endpoints tried
in order when the primary is unavailable. **Never commit a real key or live config** — keys are
supplied via env/file behind the `Signer` interface; `*.local.*` and `.env` are gitignored.

The `txManager` dispatcher serializes nonce allocation, signing, and initial broadcast; independent
trackers then require a canonical receipt plus the configured confirmation depth. The shared
`pendingIntervalMs` (default 120000), `feeBumpBps` (default 1250), and `maxReplacements` (default 3)
settings bound same-nonce, same-payload replacements. A positive `maxFeeGwei` is a hard ceiling and is
never exceeded by an initial transaction or replacement. See either annotated example for the exact
bounds.

All long-lived listeners and workers are supervised. An observability or RFQ listener failure is
process-fatal; cancellation shuts down the listeners and joins the transaction manager and solver
workers before the process returns.

## Code generation

Generated code is committed for hermetic builds; refresh from upstream on demand:

```bash
make refresh-abi FORGE_OUT=../rfq/out   # re-vendor contract ABIs from a Foundry build
make refresh-openapi                    # re-pull the live 3F OpenAPI spec
make refresh-rfq-openapi                # re-pull the RFQ backend OpenAPI spec
make generate                           # regenerate bindings + API client
make check-generated                    # regenerate from vendored inputs and reject drift
```

CI runs `make check-generated` only against committed interface artifacts. It never runs the live
`refresh-*` targets, so upstream changes enter the repository only through an explicit refresh.

## Contributing

Engineering conventions — the modular framework/integration boundary, config-driven configuration,
modern Go 1.26 style, the required test/lint/format gate, and secure-coding rules — are in
[`CLAUDE.md`](./CLAUDE.md) (`AGENTS.md` is a symlink to it). Every change must keep
`make format && make test && make lint` green and unit-test new logic.
