# vault-solver — Implementation Plan (v0)

> A Go service that monitors a configured selection of Symbiotic vaults and runs a
> pluggable **solver** strategy against them. The first (and currently only) solver
> implementation is the **3F Bridge Facilitator** off-chain bot. The repository is
> structured so additional solver implementations can be added later without
> touching the generic framework.

This document is the source of truth for the build. It captures the agreed scope,
architecture, and decisions. See `3F_BRIDGE_FACILITATOR_INTEGRATION.md` (sibling
repo root) §4 for the functional blueprint of the 3F solver.

---

## 1. Scope

- **In scope:** the off-chain Go bot — auction discovery, offer pricing/sizing/signing,
  on-chain reads for liquidity, position reconciliation, and redemption.
- **Out of scope:** the on-chain `BridgeFacilitatorAdapter` (Solidity). It lives in a
  separate repo and is consumed here only via generated ABI bindings.
- **First target network:** 3F Sepolia dev (`chainId 11155111`), which has a live
  deployment and a public-readable dev API. Mainnet config slots in later.

---

## 2. Confirmed decisions

| Topic | Decision |
|---|---|
| Language / toolchain | **Go 1.26** (module declares `go 1.26`; toolchain auto-fetch) |
| Logging | **`logr.Logger`** interface throughout, backed by **zap** via `zapr`; only `main` wires the backend |
| Metrics | Prometheus (`/metrics`); `logr` keeps the logging dependency swappable |
| License | **BSL-1.1** (matches the contract side; Licensor `GPRP`, Change License GPLv2+) |
| Contract bindings | **abigen over vendored ABIs** in `api/abi/` (ABIs copied from `forge build` output, not hand-curated). `make refresh-abi` re-vendors from a Foundry `out/` dir; build stays hermetic off the committed ABIs. |
| API client | **oapi-codegen** over a vendored OpenAPI snapshot in `openapi/`. `make refresh-openapi` re-pulls the live spec. |
| Persistence | **Stateless + periodic on-chain resync.** No DB. Open positions come from `adapter.activeRequests()`; redemption readiness from `canWithdraw()`; auctions/offers from the 3F API. Optional live-log subscription is a latency optimization only, never on the critical path. |
| Key management | Env/file private key behind a pluggable **`Signer`** interface (KMS/remote-signer can be added later without touching call sites). |
| Multi-solver shape | 3F logic fully encapsulated in its own package; `main` initializes one solver today. A name→factory **registry** selects the impl from config. A **shared `txmanager`** owns on-chain sending so solvers never race on nonces. |

---

## 3. Why view-only monitoring is sufficient

Everything the bot needs is reachable from view functions + the 3F API:

| Need | Source | Type |
|---|---|---|
| Open position set | `adapter.activeRequests()` | on-chain view |
| Per-position detail (principal, ytExpected, openedAt) | `adapter.positions(request)` | on-chain view |
| Realized / recallable principal | `realizedPrincipal()`, `deallocatable()`, `skimmable()` | on-chain view |
| Redeem trigger (loan ready) | `IVaultController(request).canWithdraw()` across `activeRequests()` | on-chain view |
| Offer won / consumed | next `activeRequests()` resync | on-chain view |
| Auction discovery + offer status | `GET /v1/auction`, `GET /v1/offer` | 3F API (off-chain) |
| Realized loss/gain per loan | `PositionRedeemed` log parsed from the bot's **own** `redeem` receipt | self-emitted |

Trade-off accepted: view-only loses *latency* (learn of consume/repay on the next
poll tick) and *historical analytics*. Neither matters for 3F — funding pull time is
known hours ahead, repayments land 24h–30d+ out, and there is no sub-second deadline.
No historical event indexer, no DB.

---

## 4. Repository layout

```
vault-solver/
├── cmd/vault-solver/main.go         # bootstrap: config → chain → signer → txmanager → init solver → run
├── internal/
│   ├── config/                    # env + YAML loader; per-instance VAULT SELECTION; two-stage solver decode
│   ├── chain/                     # GENERIC eth client primitives (Dial, ChainID). Solver-specific
│   │   │                          # reads (e.g. vault/adapter liquidity) live in the owning solver.
│   ├── signer/                    # Signer interface + local (env/file key) impl  ← pluggable
│   ├── txmanager/                 # SHARED nonce-serialized tx sender  ← shared infra
│   ├── solver/                    # generic Solver interface + registry + engine (solver-agnostic)
│   ├── solvers/bridgefacilitator/ # ALL 3F-specific logic, encapsulated
│   │   ├── solver.go  config.go  apiclient.go  auctionview.go  sizer.go
│   │   ├── offer.go   eip712.go  chainreader.go  redeemer.go
│   ├── observability/             # logr+zap setup, prometheus, /healthz /readyz
│   └── version/
├── api/
│   ├── abi/                       # vendored *.abi.json (copied from forge build)
│   ├── bindings/                  # abigen output (committed), grouped per integration:
│   │   ├── 3f/{adapter,request,vaultcontroller,whitelist}/  # 3F-specific (future: rfq/, oev/)
│   │   └── vaultv2/               # shared Symbiotic core, reused by every integration
│   └── threef/                    # oapi-codegen output (committed)
├── openapi/3f-bf.openapi.json     # vendored OpenAPI snapshot
├── config/{config.example.yaml,sepolia.yaml}
├── deploy/{Dockerfile,docker-compose.yml}
├── .github/workflows/ci.yml
├── .golangci.yml  Makefile  go.mod  README.md  LICENSE  .gitignore  PLAN.md
```

---

## 5. Shared infrastructure

### 5.1 `txmanager` — nonce-serialized sender

A single service owns the on-chain sending EOA. One worker goroutine drains a queue
of `TxRequest{To, Data, Value, GasLimit?, Label}`; for each it tracks the nonce
locally (seeded from the pending nonce, monotonic), sets EIP-1559 fees, signs via the
`Signer`, sends, waits for the receipt, and handles `nonce too low` / stuck-tx bump +
resync. Solvers **never** send directly — they build calldata (packed via the abigen
ABI, e.g. `adapter.Pack("redeem", requests)`) and hand it to the txmanager, receiving
a `TxResult{Hash, Receipt, Err}`. Serializing through one worker eliminates
parallel-nonce races across solvers.

> The **offerSigner** (EIP-712 offer signing, off-chain, gasless) and the **tx-sending
> EOA** are distinct roles behind the same `Signer` interface, possibly the same key.
> txmanager owns only the on-chain nonce.

### 5.2 `solver` — generic interface + registry

```go
type Deps struct {
    Chain     *chain.Client
    TxManager *txmanager.Manager
    Signer    signer.Signer
    Log       logr.Logger
    Metrics   *observability.Metrics
}

type Solver interface {
    Name() string
    Run(ctx context.Context) error
}

type Factory func(raw yaml.Node, deps Deps) (Solver, error)
```

A `registry` maps name→`Factory`. The 3F package self-registers in `init()`; `main`
blank-imports it (`_ ".../solvers/bridgefacilitator"`) — the only line referencing 3F.
Adding a future solver is a register + config switch, no framework edit.

---

## 6. Configuration

Two-stage decode keeps solver config encapsulated. The generic layer reads only
`solver.name` to pick the impl and keeps `solver.config` as a deferred `yaml.Node`;
the chosen solver decodes it into its own typed struct.

```yaml
chain: { rpcUrl, chainId, wsUrl? }
signer: { keyEnv: SOLVER_PRIVATE_KEY }     # or keystorePath + passphraseEnv
txManager: { confirmations: 2, maxFeeGwei, tipGwei }

solver:
  name: 3f-bridge-facilitator               # ← registry key: selects the impl
  config:                                    # ← opaque to framework; typed by the 3F package
    apiBaseUrl: https://bf.dev.gcp.3f.xyz
    # Single vault+adapter pair: 3F registers exactly one offer-address per facilitator.
    vault:   "0x…"
    adapter: "0x…"                           # BridgeFacilitatorAdapter (single-vault by construction)
    exposure: { perRequestMaxUsdc: "…", totalSleeveMaxUsdc: "…", maxConcurrentLoans: 10 }
    intervals: { discover: 1h, redeemPoll: 5m, reconcile: 15m }
```

---

## 7. Make-driven codegen

| Target | Action |
|---|---|
| `make tools` | install pinned `abigen`, `oapi-codegen`, `golangci-lint` |
| `make refresh-abi` | copy ABIs from `FORGE_OUT=` (jq-extract `.abi` from `out/*.json`) into `api/abi/` — manual |
| `make refresh-openapi` | curl the live spec → `openapi/` — manual |
| `make bindings` | `abigen` over `api/abi/*.json` → `api/bindings/*.go` |
| `make openapi-client` | `oapi-codegen` over the vendored spec → `api/threef/client.gen.go` |
| `make generate` | bindings + openapi-client |
| `make lint` / `test` / `build` / `docker` | golangci-lint; `go test -race -cover ./...`; build; image |

Generated code is committed (hermetic build); refresh targets regenerate from upstream
on demand. ABIs required: `BridgeFacilitatorAdapter`, `IRequest`/`IVaultController`,
`IWhitelist`, `IVaultV2`.

---

## 8. Build phases

Prerequisite (done). **`BridgeFacilitatorAdapter` contract** — built in the `rfq` repo (`src/3f/`), 25 tests, ABI vendored here. This is what the bot binds against.

0. **(done)** Scaffold + tooling — module, layout, Makefile, `.golangci.yml`, CI, BSL LICENSE, README, version pkg.
1. **(done)** Codegen pipeline — ABIs vendored from `../rfq/out`; OpenAPI snapshot; `bindings` (one pkg/contract) + `openapi-client`; committed.
2. **(done)** Core infra (solver-agnostic) — config (two-stage decode), chain primitives, signer, **txmanager (+5 tests)**, solver interface/registry/engine, observability, graceful shutdown.
3. **(done)** 3F solver (encapsulated) — API client (x-api-key auctions/offers), sizer (fundable-liquidity + curator exposure caps; Request authorization is the on-chain 3F whitelist), EIP-712 offer signing **+ golden-hash + apitypes parity test**, reconcile + redeemer (poll `canWithdraw` → pack `redeem` → txmanager), exposure / no-over-commit guards. Deltas tracked in §10.
4. **(done)** Packaging + verification — README/config docs; Sepolia-dev e2e (offers won + redeemed live); multi-stage non-root distroless Dockerfile + compose (`deploy/`, ~20 MB static CGO-free image).

---

## 9. Open items to confirm during implementation

- Mainnet `RequestWhitelist` address and prod API base URL — supplied by 3F when prod lands.
- Go module path (`github.com/symbioticfi/vault-solver` placeholder) — adjust to the real org.

---

## 10. Pending / deferred items (post-Phase-3)

Tracked TODOs and known gaps — each a scoped follow-up; none block release.

**Deferred features / known gaps:**
- **Move exposure / risk params on-chain.** Today the caps (`perRequestMaxUsdc`, `totalSleeveMaxUsdc`, `maxConcurrentLoans`, `minReturnBps`) live in the bot config and are enforced only off-chain in `sizeOffer` — a buggy or rogue bot could exceed them. Hoisting them into the `BridgeFacilitatorAdapter` (e.g. owner-set caps re-checked in `onRequestConsumed`, mirroring the removed `requestMetadata` budget but at the adapter level) makes the limits trust-minimized and curator-governed; the bot's config caps then become a redundant client-side guard. Needs a contract change in the `rfq` repo + binding regen; the bot reads the on-chain caps instead of (or in addition to) config.
- **Offer pricing is naive.** The bot bids at the auction's current `maxRate`, then caps to exposure + fundable liquidity — it models no spread, risk-adjusted target rate, time-in-auction, or competing offers. A real quoting strategy (e.g. the RFQ solver's strategy logic) is the main follow-up; `buildSignedOffer` is the seam to extend, and `MinReturnBps` is the only knob today.
- **API key logged at debug (`V(1)`).** Convenience for out-of-band replay; it is a secret in logs — disable or scrub before production.
- **Offer cancellation.** `OfferControllerCancelV1` not wired — needs offer-id↔auction state. Note `offerTTL` (30m) < `discover` (1h) leaves a no-offer gap each cycle; consider `offerTTL` ≥ the discover interval (dedup prevents redundant re-offers).
- **WS live-log subscription** (`chain.wsUrl`) — config field present but unused; the poll-based reconcile/redeem path is sufficient for v0.

**Testing:**
- **Integration coverage.** `bridgefacilitator` unit coverage is ~16% — pure logic (EIP-712 golden+parity, sizer caps, config) is covered; the HTTP/on-chain paths (apiclient, chainreader, redeemer, Run loop) need an httptest-backed API mock + a simulated/forked chain backend.
- **Solver-agnostic metrics seam.** `solver.Deps.Metrics` (the `Registerer()` extension point) is wired but no solver registers collectors yet; add bridge-facilitator metrics (offers sent/won, exposure, locked vs realized, redemptions) and they'll verify the seam.
