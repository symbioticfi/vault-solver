# Development

`CLAUDE.md` is the normative working agreement. This document is the short task and command map.

## Before editing

1. Run `git status --short --branch` and preserve unrelated work.
2. Find the subsystem in [the documentation map](README.md).
3. Do not edit generated files under `api/`; update the vendored contract and regenerate them.
4. Use focused checks while iterating, then the complete gate before finishing.
5. Update a solver plan for architecture, external-contract, or open-work changes; update root `README.md`
   only for operator-visible behavior or configuration.

## Verification

```bash
make tools                                      # pinned tools in .tools/bin, once
make doctor                                     # diagnose local prerequisites
make verify-fast TARGET=./internal/txmanager    # uncached unit tests + lint
make verify-race TARGET=./internal/txmanager    # concurrency-sensitive package
make format                                     # mutating formatter/autofix
make verify                                     # complete read-only gate
```

`make verify` checks formatting, compiles every package, runs uncached race tests with coverage, and runs the
final linter. The Makefile forces the repository's exact Go toolchain.

## Change map

| Change | Read first | Focused check |
|---|---|---|
| Generic config or registry | [Architecture](ARCHITECTURE.md) | `make verify-race TARGET='./internal/config ./internal/solver ./cmd/vault-solver'` |
| Chain reads/fallback | relevant package comments and solver plan | `make verify-race TARGET=./internal/chain/...` |
| Transaction lifecycle | [Transaction manager](TXMANAGER.md) | `make verify-race TARGET=./internal/txmanager` |
| Shared LiquidLane facts/economics | [LiquidLane conventions](LIQUIDLANE-CONVENTIONS.md) | `make verify-race TARGET=./internal/liquidlane/...` |
| Solver behavior | corresponding solver plan and example config | solver command from [docs index](README.md) |
| Strategy wire contract | [Strategy architecture](strategy-plan.md) | affected strategy package plus wire JSON tests |
| Contract/API surface | codegen table below | regenerate, inspect the complete diff, then affected consumers |

## Code generation

Generated files are committed; the vendored artifact is the contract of record.

| Surface | Vendored input | Refresh/generate command |
|---|---|---|
| Contract bindings | `api/abi/*.json` | `make refresh-abi`, then `make bindings` |
| 3F client | `openapi/3f-bf.openapi.json` | `make refresh-openapi refresh-3f-client` |
| RFQ client | `openapi/rfq-backend.openapi.json` | `make refresh-rfq-openapi refresh-rfq-client` |
| LI.FI client | `openapi/lifi-order.openapi.json` | `make refresh-lifi-openapi refresh-lifi-client` |
| UniswapX client | `openapi/uniswapx-service.openapi.json` | `make refresh-uniswapx-openapi refresh-uniswapx-client` |
| Morpho GraphQL | `api/graphql/morpho/` | `make refresh-morpho-graphql-schema refresh-morpho-graphql-client` |

Never patch generated Go to compensate for an upstream mismatch. Update the artifact and regenerate in the same
change so call-site breakage remains reviewable.
