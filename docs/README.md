# Documentation map

Use this index before opening the longer solver plans. `README.md` is the operator entry point;
`CLAUDE.md` is the contributor and agent contract.

## Shared architecture

| Subject | Source of truth | Primary code |
|---|---|---|
| Repository boundaries and dependency direction | [Architecture](ARCHITECTURE.md) | `cmd/`, `internal/solver`, `internal/solvers` |
| Development commands and change map | [Development](DEVELOPMENT.md) | `Makefile`, `.github/workflows/ci.yml` |
| Shared transaction lifecycle | [Transaction manager](TXMANAGER.md) | `internal/txmanager` |
| LiquidLane ownership and invariants | [LiquidLane conventions](LIQUIDLANE-CONVENTIONS.md) | `internal/liquidlane` |
| Strategy contracts | [Strategy architecture](strategy-plan.md) | `internal/*/strategies` |

## Integrations

| Runtime name | Package | Plan | Example config | Focused verification |
|---|---|---|---|---|
| `3f-bridge-facilitator` | `internal/solvers/bridgefacilitator` | [3F](3F-PLAN.md) | [`3f.example.yaml`](../config/3f.example.yaml) | `make verify-race TARGET=./internal/solvers/bridgefacilitator/...` |
| `rfq-filler` | `internal/solvers/rfq` | [RFQ](RFQ-PLAN.md) | [`rfq.example.yaml`](../config/rfq.example.yaml) | `make verify-race TARGET=./internal/solvers/rfq/...` |
| `redstone-oev` | `internal/solvers/redstoneoev` | [OEV](OEV-PLAN.md) | [`redstone-oev.example.yaml`](../config/redstone-oev.example.yaml) | `make verify-race TARGET=./internal/solvers/redstoneoev/...` |
| `lifi-samechain` | `internal/solvers/lifi` | [LI.FI](LIFI-PLAN.md) | [`lifi.example.yaml`](../config/lifi.example.yaml) | `make verify-race TARGET=./internal/solvers/lifi/...` |
| `uniswapx-filler` | `internal/solvers/uniswapx` | [UniswapX](UNISWAPX-PLAN.md) | [`uniswapx.example.yaml`](../config/uniswapx.example.yaml) | `make verify-race TARGET=./internal/solvers/uniswapx/...` |

Plans contain maintained design decisions, protocol facts, deployment prerequisites, and open work. Completed
implementation history belongs in Git history rather than the live open-work section.
