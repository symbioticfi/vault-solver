# Documentation map

This is the contributor and coding-agent entry point. [`CLAUDE.md`](../CLAUDE.md) owns the normative
engineering rules; the root [`README.md`](../README.md) owns operator-visible behavior.

## Load context in this order

1. Read `CLAUDE.md` and preserve the current worktree state.
2. Select the smallest matching route below.
3. Read the linked contract, then inspect the affected package's source, tests, and example config.
4. Widen to another shared contract or integration plan only when the dependency crosses that boundary.

Do not preload every integration plan or scan generated `api/` packages for an ordinary source change. For an
external-contract change, start from the vendored ABI, OpenAPI specification, or GraphQL schema and use the
code-generation route in [Development](DEVELOPMENT.md).

## Sources of truth

| Concern | Canonical source |
|---|---|
| Engineering, security, testing, and documentation rules | [`CLAUDE.md`](../CLAUDE.md) |
| Operator-visible capabilities, runtime requirements, and configuration overview | [`README.md`](../README.md) |
| Exact example configuration and structural schema | `config/*.example.yaml`, `config/vault-solver.schema.json` |
| Semantic configuration validation | `vault-solver config validate` and integration-owned config parsers |
| Repository boundaries and composition | [Architecture](ARCHITECTURE.md) |
| Shared subsystem invariants | the shared contracts listed below |
| Integration design, protocol facts, deployment prerequisites, and live open work | the corresponding integration plan |
| Development commands and generated-code ownership | [Development](DEVELOPMENT.md) and `Makefile` |
| External wire/contract surface | vendored artifacts under `api/abi/`, `openapi/`, and `api/graphql/` |

If two surfaces disagree, treat that as a defect: establish the implemented behavior from code, tests, and the
executable contract, then update the canonical document in the same change. Do not preserve contradictory
summaries. Vendored artifacts pin what this build consumes; live addresses, chain support, API limits, and
onboarding policy remain deployment-time facts and must be revalidated upstream rather than copied as
"current" repository state.

## Task routes

| Change | Read first | Primary code or artifact |
|---|---|---|
| CLI composition, solver registration, or common config | [Architecture](ARCHITECTURE.md), [Development](DEVELOPMENT.md) | `cmd/vault-solver`, `internal/{app,config}` |
| Project architecture or cross-cutting solver/LiquidLane design | [Architecture](ARCHITECTURE.md), [Strategy contract](STRATEGIES.md) | `internal/{capacity,liquidlane,solvers}`, `cmd/vault-solver` |
| Chain reads or RPC fallback | [Architecture](ARCHITECTURE.md), owning integration plan | `internal/chain`, affected reader |
| Transaction admission, fees, nonce, replacement, or shutdown | [Transaction manager](TXMANAGER.md) | `internal/txmanager` |
| LiquidLane reads, route identity, capacity, gas, or discounts | [LiquidLane conventions](LIQUIDLANE-CONVENTIONS.md) | `internal/liquidlane/...` |
| Planner interface, default policy, or webhook decision | [Strategy contract](STRATEGIES.md), owning integration plan | owning solver root, `internal/{liquidlane/planning,webhook}` |
| One solver's protocol behavior | corresponding integration plan and example config | `internal/solvers/<name>` |
| ABI, OpenAPI, GraphQL, or generated client | [Development code generation](DEVELOPMENT.md#code-generation), owning integration plan | vendored artifact first, generated `api/` consumer second |
| Operator-visible behavior or configuration | root [`README.md`](../README.md), example config | `cmd/`, `config/`, affected solver |

## Shared contracts

| Subject | Contract | Primary code |
|---|---|---|
| Dependency direction and startup composition | [Architecture](ARCHITECTURE.md) | `cmd/vault-solver`, `internal/app`, `internal/solvers` |
| Development workflow and verification | [Development](DEVELOPMENT.md) | `Makefile`, `.github/workflows/ci.yml` |
| Transaction lifecycle | [Transaction manager](TXMANAGER.md) | `internal/txmanager` |
| LiquidLane ownership and invariants | [LiquidLane conventions](LIQUIDLANE-CONVENTIONS.md) | `internal/liquidlane` |
| Solver-local decision boundary | [Strategy contract](STRATEGIES.md) | solver-local `planner_*.go`, `internal/liquidlane/planning`, `internal/webhook` |

## Integrations

| Runtime name | Package | Plan | Example config |
|---|---|---|---|
| `3f-bridge-facilitator` | `internal/solvers/bridgefacilitator` | [3F](3F-PLAN.md) | [`3f.example.yaml`](../config/3f.example.yaml) |
| `rfq-filler` | `internal/solvers/rfq` | [RFQ](RFQ-PLAN.md) | [`rfq.example.yaml`](../config/rfq.example.yaml) |
| `redstone-oev` | `internal/solvers/redstoneoev` | [RedStone OEV](OEV-PLAN.md) | [`redstone-oev.example.yaml`](../config/redstone-oev.example.yaml) |
| `lifi-samechain` | `internal/solvers/lifi` | [LI.FI](LIFI-PLAN.md) | [`lifi.example.yaml`](../config/lifi.example.yaml) |
| `uniswapx-filler` | `internal/solvers/uniswapx` | [UniswapX](UNISWAPX-PLAN.md) | [`uniswapx.example.yaml`](../config/uniswapx.example.yaml) |

Plans describe the current integration contract, durable verified evidence, implementation status, and live
open work. Task handoffs, generated summaries, and refactor reports belong in Git history or the pull request,
not in a parallel documentation tree.
When adding, renaming, or removing a durable document, update this map in the same change.
