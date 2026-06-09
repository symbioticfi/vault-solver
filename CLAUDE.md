# CLAUDE.md — working agreement for this repo

This file is the source of truth for how code is written here. Read it before making changes.
It applies to AI agents and humans alike. `AGENTS.md` points here.

## Purpose

`vault-solver` is a Go service that monitors a configured selection of **Symbiotic vaults** and runs a
pluggable **solver** strategy against them. A "solver" is an off-chain integration with some external
protocol that sources or prices liquidity on top of a Symbiotic vault adapter.

The first implementation is the **3F (Grunt) Bridge Facilitator**. The repository is explicitly
structured so additional integrations — **RFQ, Redstone/OEV, and others** — can be added *without
touching the generic framework*. Keeping that boundary clean is the single most important design goal.

See `PLAN.md` for the architecture, decisions, and the live TODO list (§10).

## The modularity rule (most important)

Two layers, and code lives in exactly one:

- **Generic framework** (integration-agnostic, shared by every solver):
  `internal/{config,chain,signer,txmanager,solver,observability,version}` and `cmd/`.
  Nothing here may know about 3F, RFQ, Redstone, or any specific protocol.
- **Integration packages** (fully self-contained): `internal/solvers/<name>/`
  (today `bridgefacilitator/`). All protocol-specific logic, types, ABIs usage, pricing, and config
  live here.

To add a new integration (e.g. `rfq`):
1. Create `internal/solvers/rfq/` implementing `solver.Solver` (`Name()`, `Run(ctx)`), with a
   `Factory(raw yaml.Node, deps solver.Deps) (Solver, error)`.
2. Self-register in `init()` via `solver.Register(Name, factory)`; blank-import the package from `main`.
3. Put generated bindings under `api/bindings/<name>/...` (the existing 3F bindings are under
   `api/bindings/3f/`; shared Symbiotic core stays in `api/bindings/vaultv2/`).
4. Decode your own config from the deferred `solvers[].config` YAML node — no framework edits.

If you find yourself adding an `if integration == "3f"` branch or a protocol-specific field in the
generic layer, stop — the abstraction is wrong. Generalize the mechanism instead of special-casing.

## Configuration: always file-driven

- **All configuration comes from the YAML config file** (and, at runtime, from the upstream API).
  Do not hardcode addresses, URLs, chain IDs, rates, intervals, or limits in Go. The bot is started
  with `run --config <path>`; there is no other source of operational config.
- Config lists solvers under `solvers:` (one or more, at most one per type). The generic layer decodes
  only `chain`, `signer`, `txManager`, `observability`, and each `solvers[].name`, and keeps each
  `solvers[].config` as an opaque `yaml.Node` (two-stage decode); each solver decodes that node into
  its own typed, **validated** struct in `parseConfig`. All solvers run in one process and share the
  chain client, signer, and the single nonce-serialized `txManager` — which is why multiple solvers on
  one EOA never race on nonces.
- **Prefer values from the upstream source over constants.** When the 3F API (or any integration's
  source of truth) provides a value — e.g. the EIP-712 domain `name`/`version`/`chainId` — read it
  from there; use an in-code constant only as a *fallback* when the source omits it, and say so.
- Non-secret env interpolation (`${VAR}`) into the YAML is supported via `os.ExpandEnv`. **Secrets are
  never interpolated into the config struct.** They are referenced by env-var *name* (`keyEnv`,
  `passphraseEnv`, `apiKeyEnv`) and read with `os.Getenv` at point of use, so dumping/logging config
  can never leak them. New secret-bearing config must follow the same `*Env` indirection.

## Go style (modern Go 1.26)

- Toolchain is pinned: module declares `go 1.26`, builds run `GOTOOLCHAIN=go1.26.4`. Match it.
- **Errors:** use `github.com/go-errors/errors` — `errors.Errorf("...: %w", err)` (NOT `fmt.Errorf`;
  `forbidigo` enforces this) and `errors.New` for sentinels. Wrap with `%w` and add context at each
  boundary; compare with `errors.Is`/`errors.As`. Return errors, don't log-and-continue silently —
  a swallowed error is a bug. `panic` only for genuine programmer errors (e.g. a `mustPack` of a
  static, known-good ABI call), never for runtime/IO failures.
- **Logging:** `logr.Logger` everywhere (backed by zap, wired only in `main`). Info level for
  operational events; `V(1)` for debug detail. Structured key/values, not formatted strings.
- **Context:** thread `context.Context` through all I/O (RPC, HTTP, tx). Respect cancellation; never
  `context.Background()` deep in a call path.
- **Concurrency:** shared on-chain sending goes through the single `txmanager` (nonce-serialized) —
  solvers build calldata and submit a request, they never send transactions directly and never race
  on nonces. Document the goroutine/locking model of any new shared state (see the `apiClient`
  "single Run goroutine" note).
- Keep functions at one altitude, prefer small pure helpers (they're the easily-tested seams),
  table-driven tests, and accept interfaces / return concrete types. Run `golangci-lint` (below) and
  fix findings rather than suppressing them; a `//nolint` must be specific and carry an explanation
  (`nolintlint` requires both) — and is a last resort, not a shortcut.

## Testing, linting, formatting — required for every change

Nothing merges red. Before considering a change done, all of these must pass:

```
GOTOOLCHAIN=go1.26.4 golangci-lint run --fix   # make format — formats + lints + autofixes
GOTOOLCHAIN=go1.26.4 go build ./...
GOTOOLCHAIN=go1.26.4 go test -race -cover ./...  # make test
GOTOOLCHAIN=go1.26.4 golangci-lint run            # make lint — must report 0 issues
```

- **Unit-test all new logic.** Pure logic (pricing/sizing, EIP-712 digests, config parsing/validation)
  must have table-driven tests. EIP-712 signing has golden + `apitypes` parity tests — keep them green
  and extend them when you touch the digest. HTTP/on-chain paths should be tested against an
  `httptest` server / a simulated or forked chain backend.
- Generated code (`api/bindings/**`, `api/threef`, `api/rfqbackend`) is committed for hermetic builds;
  regenerate via the `make` targets, never hand-edit (see **Code generation** below).

## Code generation: vendor the source, then generate

We never hand-write the boilerplate for talking to a contract or a third-party HTTP API. Instead we
**vendor the upstream interface artifact into the repo and generate typed Go from it.** This keeps the
bot's view of an external surface honest (it comes from the source of truth, not a hand-transcription
that silently drifts), keeps the build hermetic (generated code is committed, so a clean checkout
builds with no network/toolchain surprises), and turns an upstream change into a reviewable diff.

Two instances of the same pattern — **vendor → generate → commit, regenerated only via `make`:**

- **Contract bindings (ABI → abigen).** Vendor the ABI JSON under `api/abi/` (from a `forge build`
  out-dir; `make refresh-abi` extracts `.abi` from the build artifacts of `ABIS`/`CORE_MIRROR_ABIS`),
  then `make bindings` runs `abigen` per contract into `api/bindings/<group>/` (one package per leaf
  dir so shared ABI structs don't collide). An ABI that can't be sourced from a build (e.g. Multicall3,
  or a minimal hand-pruned `UniversalDelegator` whose full ABI has an abigen-hostile overload) is
  hand-vendored into `api/abi/` with a comment saying why — still generated from, never hand-bound.
- **API clients (OpenAPI spec → openapi-generator).** Vendor the spec under `openapi/` (`make
  refresh-*-openapi` pulls it), then `make refresh-{3f,rfq}-client` runs the **Java openapi-generator**
  (via `hack/openapi-generator-cli.sh`, which downloads the pinned jar on demand — needs a JRE) into
  `api/<client>/`. `OPENAPI_GENERATOR_VERSION` is pinned and is the **floor**: it must ingest the spec
  (e.g. 7.12.0 for an OpenAPI 3.1 spec with numeric `exclusiveMinimum` / `type:[…,null]` unions, which
  `oapi-codegen`/kin-openapi and `ogen` reject). The recipe strips the generator's non-package cruft
  (its `go.mod`/docs/test/etc.), keeping only the Go client so it joins the main module.

Rules for both: the vendored artifact (ABI/spec) is the **contract of record** — when upstream changes,
re-vendor + regenerate in the same change rather than patching generated Go. The integration code wraps
the generated client/binding behind a thin adapter so generated types (nullable pointers, response
wrappers) stay contained at the boundary and don't leak into solver logic. Reach for this pattern
**whenever a new integration needs to call a contract or a typed HTTP API** — add the `make` target and
commit the generated output; don't hand-roll request/response structs or `abi.Pack` calls.

## Security

Write defensively; this bot holds a signing key and moves funds.

- **Never commit or log secrets.** Private keys / API keys come from env-var-named config fields and
  are read at point of use. `.gitignore` and `.dockerignore` exclude `.env*`, `*.local.*`, key files —
  keep it that way. The only place a secret may appear in logs is a deliberate `V(1)` debug line, and
  that must be documented as such (and disabled for production).
- **Validate all external input** — config, API responses, and on-chain reads. Treat nullable API
  fields as nullable (the generated types use pointers for a reason); never deref without a nil check.
  Bound anything that drives funds (per-request / sleeve / concurrency caps) and fail closed.
- **Keys stay behind the `Signer` interface** so a KMS/remote signer can replace the local key without
  touching call sites. The offer-signing key and the tx-sending EOA are distinct roles.
- Honor protocol/API quirks already encoded here (e.g. the 3F API requires **lowercase** address
  strings; the EIP-712 domain pins `chainId = 1`). Don't regress them.
- Build/ship the hardened image: `CGO_ENABLED=0` static binary on **distroless nonroot** (`deploy/`).
  No shell, no root, minimal surface.
- Prefer the standard library and already-vendored deps; adding a dependency is a deliberate decision
  (supply-chain surface). Run `make tidy` and keep `go.sum` honest.

## Keep the plan in sync — required

`PLAN.md` (and the per-solver plans under `docs/`, e.g. `docs/RFQ-PLAN.md`) are the source of truth for
the high-level architecture, design decisions, and the live TODO list. They are not write-once docs.

- **Whenever you change the high-level architecture or a design decision** — a new layer or boundary,
  a changed data flow, a new/removed integration, an interface or external-contract change, a
  deliberate deviation from an upstream reference — **update the relevant plan in the same change.**
- **Whenever the TODO work changes** — an item is started, finished, dropped, or added — **update the
  TODO list (§10 of `PLAN.md` / the solver plan)** so it always reflects reality.
- A code change that alters architecture/design but leaves the plan stale is **incomplete**. If a
  change is purely local (a bug fix, a refactor with no design impact), no plan update is needed —
  use judgement, but err toward recording anything a future reader would be surprised to discover.

## Quick reference

- Run gate: `make format && make test && make lint && go build ./...`
- Add an integration: new `internal/solvers/<name>/` + `solver.Register` in `init()` + bindings under
  `api/bindings/<name>/` + a `solvers[]` entry. No framework changes.
- Config is king: if it varies by deployment, it belongs in the YAML, not in code.
- Keep the plan current: architecture/design or TODO changes must update `PLAN.md` / `docs/*-PLAN.md`
  in the same change.
