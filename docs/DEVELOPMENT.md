# Development workflow

[`CLAUDE.md`](../CLAUDE.md) is the normative working agreement. This document maps bounded changes to the
smallest useful context, verification command, and code-generation path. Start from the
[documentation map](README.md) rather than reading the whole repository.

## Bounded change loop

1. Run `git status --short --branch`; preserve unrelated work and record the baseline.
2. Resolve one owning subsystem through the [task routes](README.md#task-routes).
3. Read its contract, local source, tests, and example config before widening the search.
4. Establish a focused baseline for risky changes; distinguish pre-existing failures from regressions.
5. Edit the owning source. Never patch generated Go under `api/` by hand.
6. Run the focused check, inspect the complete diff, and update the canonical documentation surface.
7. Finish with the repository-wide format and verification gate.

Search `internal/`, `cmd/`, `config/`, and `docs/` first. Search `api/` only when tracing a generated type or
changing an external contract. Prefer definitions and references over broad text dumps; load only the sections
needed for the current decision.

## Verification

```bash
make tools                                      # install pinned tools in .tools/bin, once
make doctor                                     # diagnose local prerequisites
make verify-fast TARGET=./internal/txmanager    # uncached focused tests + lint
make verify-race TARGET=./internal/txmanager    # focused race tests
make format                                     # mutating formatter/autofix
make verify                                     # complete read-only repository gate
```

`make verify` checks formatting, compiles every package, runs uncached race tests with coverage, and runs the
final linter. The Makefile forces the exact Go toolchain declared by the repository. `make format` may mutate
files; inspect its diff before considering the change complete.

## Change map

| Change | Required context | Focused check |
|---|---|---|
| Common config, schema, solver composition, or example | [Architecture](ARCHITECTURE.md), affected examples | `make verify-race TARGET='./internal/config ./internal/solver ./cmd/vault-solver'` |
| CLI composition or process lifecycle | [Architecture](ARCHITECTURE.md) | `make verify-race TARGET=./cmd/vault-solver` |
| Chain reads or fallback transport | owning integration plan and reader call sites | `make verify-race TARGET=./internal/chain/...` |
| Signer implementation | `CLAUDE.md` security rules and call sites | `make verify-fast TARGET=./internal/signer` |
| Transaction lifecycle | [Transaction manager](TXMANAGER.md) | `make verify-race TARGET=./internal/txmanager` |
| Shared LiquidLane facts or economics | [LiquidLane conventions](LIQUIDLANE-CONVENTIONS.md) | `make verify-race TARGET=./internal/liquidlane/...` |
| One solver's behavior | corresponding [integration plan](README.md#integrations) and example config | `make verify-race TARGET=./internal/solvers/<package>/...` |
| Strategy contract or webhook wire shape | [Strategy contract](STRATEGIES.md), owning integration plan | affected strategy packages plus wire JSON tests |
| Observability or readiness | affected runtime contract and metric names | `make verify-race TARGET=./internal/observability` plus affected solver |
| Contract/API/GraphQL surface | code-generation table below and owning integration plan | regenerate, inspect the complete generated diff, then test every affected consumer |
| Documentation only | [documentation ownership](../CLAUDE.md#documentation-ownership-required) | link/path check, then the complete repository gate |

Use `verify-race` for shared state, goroutines, shutdown, caches, admission, or transaction handling. Use
`verify-fast` for bounded pure or sequential code. New logic still needs unit tests even when a broader package
suite already passes.

## Code generation

Generated files are committed; the vendored artifact is the contract of record. Refresh the artifact and
regenerate in the same change.

| Surface | Vendored input | Refresh | Generate |
|---|---|---|---|
| Contract bindings | `api/abi/*.json` | `make refresh-abi` | `make bindings` |
| 3F client | `openapi/3f-bf.openapi.json` | `make refresh-openapi` | `make refresh-3f-client` |
| RFQ client | `openapi/rfq-backend.openapi.json` | `make refresh-rfq-openapi` | `make refresh-rfq-client` |
| LI.FI client | `openapi/lifi-order.openapi.json` | `make refresh-lifi-openapi` | `make refresh-lifi-client` |
| UniswapX client | `openapi/uniswapx-service.openapi.json` | `make refresh-uniswapx-openapi` | `make refresh-uniswapx-client` |
| Morpho GraphQL | `api/graphql/morpho/` | `make refresh-morpho-graphql-schema` | `make refresh-morpho-graphql-client` |

`make generate` regenerates every committed binding and client from the already-vendored artifacts. Never use it
as a substitute for identifying the owning external surface, and never patch generated output to hide an
upstream mismatch.

## Configuration contract

`config/vault-solver.schema.json` provides structural editor completion. `vault-solver config validate` is the
semantic offline authority and exercises the generic config plus each integration-owned parser and selected
strategy parser. Every committed `*.example.yaml` must declare the schema and pass the offline validation test.

A config change is complete only when the typed parser, pure validator, schema, annotated example, tests, and
operator overview agree. Secret-bearing fields must remain `*Env` references; validation must not read the
secret value.

## Completion and handoff

Before handing work off:

1. inspect `git diff` and `git status --short` for accidental or unrelated changes;
2. report the behavior or contract changed and the exact files that own it;
3. report focused and full checks separately, including anything not run and why;
4. leave follow-up work in the owning plan only when it is durable live work.

Do not create a task report, refactor summary, or agent handoff file under `docs/`; Git history and the pull
request carry transient execution history.
