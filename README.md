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
- **`internal/solvers/<name>/`** — one self-contained package per integration
  (`bridgefacilitator/`, `rfq/`); all protocol-specific logic lives here.
- **`internal/{config,chain,signer,txmanager}`** — solver-agnostic infra: two-stage config, vault /
  Multicall3 reads, a pluggable signer, and a nonce-serialized transaction sender shared across solvers.
- **`api/`** — committed codegen: contract `bindings/` (abigen, grouped per integration) and protocol
  API clients (e.g. the 3F `threef/` client via the Java openapi-generator), each refreshable from upstream.

State is intentionally minimal: open positions, redemption readiness, and liquidity are read from
on-chain views and the relevant protocol API on each tick — no database. See
[`docs/3F-PLAN.md`](docs/3F-PLAN.md) §3.

## Solvers

Solvers are listed in config under `solvers:` — one or more, **at most one entry per solver type**.
Every solver in the process shares the chain client, signer, and the single nonce-serialized
`txManager`, so multiple solvers on one EOA never race on nonces — that shared sender is exactly why
running them together is safe. Each entry's `config` block is typed and validated by its own solver
(two-stage decode). Adding a solver touches **no** framework code — see the recipe in
[`CLAUDE.md`](./CLAUDE.md).

| `solver.name` | Integration | Status | Docs |
|---|---|---|---|
| `3f-bridge-facilitator` | 3F (Grunt) bridge-loan auctions | Live on Sepolia dev | [`docs/3F-PLAN.md`](docs/3F-PLAN.md) |
| `rfq-filler` | RFQ quoting + order filling | Implemented (quote · fill · discounts) | [`docs/RFQ-PLAN.md`](docs/RFQ-PLAN.md) |
| `redstone-oev` | RedStone OEV liquidations | Implemented (dry-run; Sepolia-proven) | [`docs/OEV-PLAN.md`](docs/OEV-PLAN.md) |

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

### RFQ Filler — `rfq-filler`

An externally-owned solver/executor for Symbiotic RFQ on top of per-vault `LiquidLaneAdapter`s. It is
both a request/response **quote server** and an order-filling **poller**:

- **Quote** — serves `POST /quote` (gated by an `x-rfq-shared-secret` header from the backend peer):
  it prices the requested swap directly off the adapter's on-chain `getAmountOut` (the oracle rate,
  quoted as-is), selects the best adapter legs across the inventory in the request,
  persists the chosen strategy by `quoteId`, and returns an `amountOut` (or `204` when it cannot
  quote — wrong chain, no whitelisted adapter, no matching asset, or no viable strategy). With
  `adapterWhitelistEnabled` (off by default), quoting and filling are restricted to the adapters of
  the configured `vaults` (enabling it without `vaults` entries fails at startup). The
  HTTP surface is **code-first OpenAPI 3.1**: request validation and the spec served at
  `/openapi.json` + `/docs` are generated from the same typed structs; `/health` is public.
- **Fill** — polls `GET /orders?filler=<executor>&orderStatus=open` every `pollIntervalMs`, then
  drives each awarded order through `queued → submitting → submitted → {filled | expired | failed}`,
  building `Executor.fill(Order, protocolSig, Swap[], DiscountSwapInput[], bytes)` and submitting it
  through the shared, nonce-serialized `txmanager` (an on-chain revert marks the order failed).
  Terminal status is reconciled back from the backend. Order discovery is **poll-only**.
- **Strategy recovery** — when the quote-time strategy isn't cached (e.g. after a restart), it rebuilds
  one from current on-chain state across the configured `vaults`, restricted to those the executor is
  authorized to fill through (adapter `marketMaker`, adapter `owner`, or delegated `isFiller`),
  plus any currently-offered discounts through whitelisted adapters.
- **Leg types** — **direct** legs (the public adapter rate) and **discount** legs (a signature-gated
  private rate resolved fresh from the backend's `/discounts` flow at fill time).

Reads are Multicall3-batched (a warm quote is a single `getAmountOut` multicall; `tokenIn` decimals
are cached). State is in-memory only — strategies, orders, attempts — and TTL-swept so it stays
bounded over long runs. The caller EOA must hold `CALLER_ROLE` on the `Executor`. The on-chain
`Executor` and `Reactor` live in the sibling `rfq` repo and the `LiquidLaneAdapter` in its
`core-mirror` submodule, all consumed via `api/bindings/rfq/`; the backend contract is pinned by a
vendored OpenAPI spec (`openapi/rfq-backend.openapi.json`). Config block: `backendUrl`,
`backendSharedSecretEnv`, `listenAddr`, `executor`, `reactor`, `pollIntervalMs`, `orderLimit`,
`adapterWhitelistEnabled`, `vaults` — see
[`config/rfq.hoodi.example.yaml`](config/rfq.hoodi.example.yaml).
Design, decisions, and the live TODO list: [`docs/RFQ-PLAN.md`](docs/RFQ-PLAN.md).

## Requirements

- Go (toolchain version pinned in [`go.mod`](./go.mod); auto-fetched by recent Go releases).
- For regenerating codegen: `make tools` (installs pinned `abigen`, `golangci-lint`). OpenAPI clients use
  the Java openapi-generator, downloaded on demand by `hack/openapi-generator-cli.sh` (needs a JRE).
- A reachable EVM RPC endpoint and a signing key (see Configuration).

## Quickstart

```bash
make build            # build ./bin/vault-solver
./bin/vault-solver version
make test             # go test -race -cover ./...
make lint             # golangci-lint
./bin/vault-solver run --config config/3f.sepolia.example.yaml
```

The CLI is built with [Cobra](https://github.com/spf13/cobra); run `vault-solver --help` for the
command list (`run`, `version`). Debug logging is off by default; enable it with
`observability.debug: true` in config or the `--debug` flag (the flag wins):

```bash
./bin/vault-solver run --config config/3f.sepolia.example.yaml --debug
```

## Configuration

Config is YAML with a two-stage decode: the framework reads `solver.name` to select the
implementation and hands the opaque `solver.config` block to that solver to type. A documented
example lives at `config/config.example.yaml` (per-instance vault selection, exposure caps,
intervals). The `chain` block takes a primary `rpcUrl` plus optional `rpcFallbackUrls` — HTTP(S)
endpoints tried in order when the primary is unavailable. **Never commit a real key or live config**
— keys are supplied via env/file behind the `Signer` interface; `*.local.*` and `.env` are gitignored.

## Code generation

Generated code is committed for hermetic builds; refresh from upstream on demand:

```bash
make refresh-abi FORGE_OUT=../rfq/out   # re-vendor contract ABIs from a Foundry build
make refresh-openapi                    # re-pull the live 3F OpenAPI spec
make refresh-rfq-openapi                # re-pull the RFQ backend OpenAPI spec
make generate                           # regenerate bindings + API client
```

## Contributing

Engineering conventions — the modular framework/integration boundary, config-driven configuration,
modern Go 1.26 style, the required test/lint/format gate, and secure-coding rules — are in
[`CLAUDE.md`](./CLAUDE.md) ([`AGENTS.md`](./AGENTS.md) points there). Every change must keep
`make format && make test && make lint` green and unit-test new logic.