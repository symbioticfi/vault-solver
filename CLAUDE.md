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
4. Decode your own config from the deferred `solver.config` YAML node — no framework edits.

If you find yourself adding an `if integration == "3f"` branch or a protocol-specific field in the
generic layer, stop — the abstraction is wrong. Generalize the mechanism instead of special-casing.

## Configuration: always file-driven

- **All configuration comes from the YAML config file** (and, at runtime, from the upstream API).
  Do not hardcode addresses, URLs, chain IDs, rates, intervals, or limits in Go. The bot is started
  with `run --config <path>`; there is no other source of operational config.
- The generic layer decodes only `chain`, `signer`, `txManager`, `observability`, and `solver.name`,
  and keeps `solver.config` as an opaque `yaml.Node` (two-stage decode). Each solver decodes that node
  into its own typed, **validated** struct in `parseConfig`.
- **Prefer values from the upstream source over constants.** When the 3F API (or any integration's
  source of truth) provides a value — e.g. the EIP-712 domain `name`/`version`/`chainId` — read it
  from there; use an in-code constant only as a *fallback* when the source omits it, and say so.
- Non-secret env interpolation (`${VAR}`) into the YAML is supported via `os.ExpandEnv`. **Secrets are
  never interpolated into the config struct.** They are referenced by env-var *name* (`keyEnv`,
  `passphraseEnv`, `apiKeyEnv`) and read with `os.Getenv` at point of use, so dumping/logging config
  can never leak them. New secret-bearing config must follow the same `*Env` indirection.

## Go style (modern Go 1.26)

- Toolchain is pinned: module declares `go 1.26`, builds run `GOTOOLCHAIN=go1.26.3`. Match it.
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
GOTOOLCHAIN=go1.26.3 golangci-lint run --fix   # make format — formats + lints + autofixes
GOTOOLCHAIN=go1.26.3 go build ./...
GOTOOLCHAIN=go1.26.3 go test -race -cover ./...  # make test
GOTOOLCHAIN=go1.26.3 golangci-lint run            # make lint — must report 0 issues
```

- **Unit-test all new logic.** Pure logic (pricing/sizing, EIP-712 digests, config parsing/validation)
  must have table-driven tests. EIP-712 signing has golden + `apitypes` parity tests — keep them green
  and extend them when you touch the digest. HTTP/on-chain paths should be tested against an
  `httptest` server / a simulated or forked chain backend.
- Generated code (`api/bindings`, `api/threef`) is committed for hermetic builds; regenerate via
  `make refresh-abi && make bindings` / `make refresh-openapi && make openapi-client`, never hand-edit.

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

## Quick reference

- Run gate: `make format && make test && make lint && go build ./...`
- Add an integration: new `internal/solvers/<name>/` + `solver.Register` in `init()` + bindings under
  `api/bindings/<name>/` + a `solver.config` block. No framework changes.
- Config is king: if it varies by deployment, it belongs in the YAML, not in code.
