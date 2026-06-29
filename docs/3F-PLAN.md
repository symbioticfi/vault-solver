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

- **In scope:** the off-chain Go bot, serving **multiple `BridgeFacilitatorAdapter`s** — auction
  discovery, **per-auction multi-adapter coverage**, offer pricing/sizing/signing (signed payloads),
  on-chain reads for liquidity, position reconciliation, and redemption.
- **Out of scope:** the on-chain `BridgeFacilitatorAdapter` (Solidity, consumed via generated ABI
  bindings) **and its 3F onboarding**. In the new model each adapter is deployed and registered with 3F
  **as a facilitator by its own vault creator**, who then sets this solver's signer as the adapter's
  **EIP-1271 signer**. The bot registers nothing with 3F and holds no API key.
- **First target network:** 3F Sepolia dev (`chainId 11155111`). Mainnet config slots in later.

---

## 2. Confirmed decisions

| Topic | Decision |
|---|---|
| Language / toolchain | **Go 1.26** (module declares `go 1.26`; toolchain auto-fetch) |
| Logging | **`logr.Logger`** interface throughout, backed by **zap** via `zapr`; only `main` wires the backend |
| Metrics | Prometheus (`/metrics`); `logr` keeps the logging dependency swappable |
| License | _TBD — not yet added_ |
| Contract bindings | **abigen over vendored ABIs** in `api/abi/` (ABIs copied from `forge build` output, not hand-curated). `make refresh-abi` re-vendors from a Foundry `out/` dir; build stays hermetic off the committed ABIs. |
| API client | **openapi-generator (Java)** over a vendored OpenAPI snapshot in `openapi/`. `make refresh-openapi` re-pulls the live spec. |
| 3F auth | **Signed payloads — no API key, no offer-address.** Offer create + list authenticate via the EIP-712 offer signature, which 3F verifies through the adapter's **EIP-1271 `isValidSignature`** (the adapter trusts this solver's signer). Auction listing is public. Replaces the old self-generated `x-api-key` + per-facilitator offer-address registration. |
| Adapter scope | One solver serves a **set of adapters** (config whitelist now; a dynamic "list public 3F adapters" API later). Per auction it covers the **full requested amount** with one or more single-adapter offers (most-fundable first), stopping once covered — **1 adapter per offer, no aggregation within an offer** (a single offer is never split across adapters). |
| Persistence | **Stateless + periodic on-chain resync.** No DB. Open positions come from `adapter.activeRequests()` (per adapter); redemption readiness from `canWithdraw()`; auctions/offers from the 3F API. Optional live-log subscription is a latency optimization only, never on the critical path. |
| Key management | Env/file private key behind a pluggable **`Signer`** interface (KMS/remote-signer can be added later without touching call sites). This key is the **EIP-1271 signer every served adapter trusts** (each adapter's owner sets it on-chain): it signs offers with `maker = adapter`, and the adapter's `isValidSignature` authorizes them. The same EOA is the tx-sender for `redeem` (via the shared `txmanager`). |
| Multi-solver shape | 3F logic fully encapsulated in its own package; a name→factory **registry** selects the impl from config. A **shared `txmanager`** owns on-chain sending so solvers never race on nonces. |

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
│   └── threef/                    # openapi-generator (Java) output (committed)
├── openapi/3f-bf.openapi.json     # vendored OpenAPI snapshot
├── config/{config.example.yaml,3f.sepolia.example.yaml}
├── deploy/{Dockerfile,docker-compose.yml}
├── .github/workflows/ci.yml
├── .golangci.yml  Makefile  go.mod  README.md  .gitignore
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

## 6. Configuration & per-offer adapter selection

Two-stage decode keeps solver config encapsulated. The generic layer reads only
`solver.name` to pick the impl and keeps `solver.config` as a deferred `yaml.Node`;
the chosen solver decodes it into its own typed struct.

```yaml
chain: { rpcUrl, chainId, rpcFallbackUrls?, wsUrl? }   # rpcFallbackUrls: HTTP(S), tried on primary failure
signer: { keyEnv: SOLVER_PRIVATE_KEY }     # the EIP-1271 signer every served adapter trusts
txManager: { confirmations: 2, maxFeeGwei, tipGwei }

solver:
  name: 3f-bridge-facilitator               # ← registry key: selects the impl
  config:                                    # ← opaque to framework; typed by the 3F package
    apiBaseUrl: https://bf.dev.gcp.3f.xyz
    # The adapters this solver maintains offers for. Each must already be registered with 3F as a
    # facilitator by its vault creator, with this solver's signer set as the adapter's EIP-1271 signer.
    # A config whitelist for now; a dynamic "list public 3F adapters" API replaces it later.
    adapters:
      - "0x…adapterA"
      - "0x…adapterB"
    redeemBatchSize: 10                       # optional (default 10)
    httpTimeout: 30s                          # optional
    intervals: { discover: 1h, redeemPoll: 5m, reconcile: 15m }
```

`apiKeyEnv` and the single `adapter`/`vault`/`exposure` keys are **gone**: there is no API key, and each
adapter's **vault + collateral are resolved on-chain** (`adapter.vault()` / `vault.asset()`) and its
**exposure caps are read on-chain** (`perRequestMaxCollateral`, `maxConcurrentLoans`, `minRequestYield` —
ppm, converted to bps by the reader) — config carries only the adapter addresses. The sleeve cap is the
delegator's per-adapter `limitOf` (folded into `fundable`), so the adapter has no separate one.

### Per-auction adapter coverage (the new multi-adapter core)

Each discover tick lists open auctions (public, unauthenticated), then for each auction covers its
**full requested amount** with one or more single-adapter offers, in a single pass:

1. **Candidates** — the configured adapters that are **eligible to bid**: collateral (`vault.asset()`)
   matches the auction's `depositAsset`, no live offer of ours already covers them on this auction, and
   the auction's `maxRate` clears the adapter's on-chain return floor (`minRequestYield`). The floor is
   a selection filter, not a late signing-time check — a floor-failing adapter never competes.
2. **Capacity** — read each candidate's exposure/liquidity in one Multicall (`fundable`,
   `outstandingPrincipal`, open-loan count, the on-chain caps), then `sizeOffer()` it to its
   **capacity** — the max principal it can fund (per-request → fundable → concurrency),
   independent of the ask.
3. **Select offers** — `remaining = amountRequested − liveCoverage(auction)` (coverage already held from
   this and prior passes, summed across adapters). If `remaining ≤ 0` the auction is already fully
   covered → skip it (no duplicate offers). Otherwise `selectOffers` ranks the candidates by capacity
   (largest first) and, in one shot, assigns each the principal it will offer — `min(capacity,
   still-uncovered)` — until `remaining` is filled or candidates run out. Each adapter offers at most
   once. **1 adapter per offer, no aggregation within an offer** — a single offer is never split across
   adapters, but an auction's ask may be covered by several single-adapter offers; any uncovered
   remainder (insufficient capacity, or a build/submit failure) is retried next pass. A future
   min-amount exposure param would be enforced here.
4. **Sign + submit** — each offer is built with `maker = chosen adapter`, the EIP-712 digest signed with
   the solver signer, and `createOffer`d as a **signed payload** (3F authorizes via the adapter's
   EIP-1271). Dedup is keyed by **(adapter, auction)** and offers carry their **principal**, so coverage
   is tracked across adapters and passes.

---

## 7. Make-driven codegen

| Target | Action |
|---|---|
| `make tools` | install pinned `abigen`, `golangci-lint` (OpenAPI uses the Java openapi-generator, downloaded on demand) |
| `make refresh-abi` | copy ABIs from `FORGE_OUT=` (jq-extract `.abi` from `out/*.json`) into `api/abi/` — manual |
| `make refresh-openapi` | curl the live spec → `openapi/` — manual |
| `make bindings` | `abigen` over `api/abi/*.json` → `api/bindings/*.go` |
| `make openapi-client` | Java openapi-generator over the vendored spec → `api/threef/` |
| `make generate` | bindings + openapi-client |
| `make lint` / `test` / `build` / `docker` | golangci-lint; `go test -race -cover ./...`; build; image |

Generated code is committed (hermetic build); refresh targets regenerate from upstream
on demand. ABIs required: `ThreeFAdapter` (from core-mirror), `IRequest`/`IVaultController`,
`IWhitelist`, `IVaultV2`.

---

## 8. Build phases

Prerequisite (done). **`ThreeFAdapter` contract** — core-mirror's `src/contracts/adapters/ThreeFAdapter.sol`, ABI vendored here from the core-mirror Foundry build. This is what the bot binds against (it replaced rfq's `BridgeFacilitatorAdapter`).

0. **(done)** Scaffold + tooling — module, layout, Makefile, `.golangci.yml`, CI, README, version pkg. (LICENSE not yet added.)
1. **(done)** Codegen pipeline — ABIs vendored from `../rfq/out`; OpenAPI snapshot; `bindings` (one pkg/contract) + `openapi-client`; committed.
2. **(done)** Core infra (solver-agnostic) — config (two-stage decode), chain primitives, signer, **txmanager (+5 tests)**, solver interface/registry/engine, observability, graceful shutdown.
3. **(done)** 3F solver (encapsulated) — API client (x-api-key auctions/offers), sizer (fundable-liquidity + curator exposure caps; Request authorization is the on-chain 3F whitelist), EIP-712 offer signing **+ golden-hash + apitypes parity test**, reconcile + redeemer (poll `canWithdraw` → pack `redeem` → txmanager), exposure / no-over-commit guards. Deltas tracked in §10.
4. **(done)** Packaging + verification — README/config docs; Sepolia-dev e2e (offers won + redeemed live); multi-stage non-root distroless Dockerfile + compose (`deploy/`, ~20 MB static CGO-free image).
5. **(done) Adapter-as-facilitator + signed payloads + multi-adapter.** The new model (§1, §2, §6),
   implemented across the `bridgefacilitator` package:
   - **Dropped the API key + offer-address.** `listOffers` is now a per-adapter **signed** query (EIP-712
     `GetOffers` in an `Authorization: Bearer` header); `createOffer` sends no `x-api-key`. Removed the
     key-gen, `apiKeyEnv`, and the `ensureOfferAddress`/`setOfferAddress` onboarding. Onboarding (deploy
     adapter → register with 3F → set this signer as EIP-1271 signer) is the vault creator's job.
   - **`adapter` → `adapters[]`.** Config whitelist; each adapter's vault/collateral resolved once at
     startup; on-chain EIP-1271 signer check drops any adapter this solver isn't authorised for
     (fail-closed; zero remaining → startup shutdown). **No redeem-only mode** — with ≥1 matching adapter
     the bot runs offers + redeems for the matched set; with none it shuts down.
   - **Per-auction multi-adapter coverage** (§6): cover each auction's full requested amount with one or
     more single-adapter offers (most-fundable first) in a single pass, gated on live coverage so a
     fully-covered auction is never re-offered; uncovered remainder retries next pass. Offer dedup,
     coverage, exposure, redeem, and reconcile all run per adapter.
   - Tests: `selectBestAdapter`, per-(adapter,auction) dedup, `liveCoverage`, signed `listOffers` httptest, `authorizedSigner`
     Multicall round-trip, EIP-712 `GetOffers` golden + apitypes cross-check. The `GetOffers` type string
     and the signer's live-API acceptance are pinned by env-guarded live tests (§9).

---

## 9. Open items to confirm during implementation

- **Signed-payload API contract** — confirm with 3F the exact request shape for creating *and listing*
  offers without an API key: how a list request is authenticated/scoped to an adapter (the signed payload),
  and that 3F verifies offer creation via the adapter's EIP-1271 `isValidSignature`.
- **Dynamic "list public 3F adapters" API** — the endpoint that replaces the config whitelist (what
  marks an adapter public/eligible, and how we filter to ones our signer is the EIP-1271 signer for).
- Mainnet `RequestWhitelist` address and prod API base URL — supplied by 3F when prod lands.
- Go module path (`github.com/symbioticfi/vault-solver` placeholder) — adjust to the real org.

---

## 10. Pending / deferred items (post-Phase-3)

Tracked TODOs and known gaps — each a scoped follow-up; none block release.

**Deferred features / known gaps:**
- **(done) Exposure / risk params are on-chain.** The caps (`perRequestMaxCollateral`, `maxConcurrentLoans`, `minRequestYield` in ppm) live on the `ThreeFAdapter` and are read per-adapter via Multicall each discover tick (`chainreader.go`); the bot no longer carries config exposure caps. The total-sleeve cap is the delegator's per-adapter `limitOf` (folded into `fundable`), not a separate adapter cap. Trust-minimized + curator-governed, as planned.
- **Multi-maker offers.** An auction's ask is covered by **multiple single-adapter offers** (most-fundable first, sized to the uncovered remainder), but a **single** offer is still funded by one adapter. Splitting one offer across several makers (true aggregation) is deferred — needs multi-maker offer support on-chain.
- **Re-pricing live offers on rising yield.** An auction's `maxRate` can climb over time, so an auction infeasible now (below an adapter's `minRequestYield`) becomes feasible later — handled, since infeasible auctions are never negatively cached and each pass re-evaluates. But a live offer placed at an earlier, lower rate is **not** re-priced upward while it stays live (dedup by `(adapter, auction)`); capturing the higher rate would need cancel/replace (depends on `OfferControllerCancelV1`, below).
- **Dynamic adapter discovery.** The adapter set is a config whitelist; the dynamic "list public 3F adapters" API (§9) replaces it later, filtered to adapters our signer is the EIP-1271 signer for.
- **Offer pricing is naive.** The bot bids at the auction's current `maxRate`, then caps to exposure + fundable liquidity — it models no spread, risk-adjusted target rate, time-in-auction, or competing offers. A real quoting strategy (e.g. the RFQ solver's strategy logic) is the main follow-up; `buildSignedOffer` is the seam to extend, and the adapter's on-chain `minRequestYield` is the only floor today.
- **Offer cancellation.** `OfferControllerCancelV1` not wired — needs offer-id↔auction state. Note `offerTTL` (30m) < `discover` (1h) leaves a no-offer gap each cycle; consider `offerTTL` ≥ the discover interval (dedup prevents redundant re-offers).
- **WS live-log subscription** (`chain.wsUrl`) — config field present but unused; the poll-based reconcile/redeem path is sufficient for v0.

**Testing:**
- **Integration coverage.** `bridgefacilitator` unit coverage is ~16% — pure logic (EIP-712 golden+parity, sizer caps, config) is covered; the HTTP/on-chain paths (apiclient, chainreader, redeemer, Run loop) need an httptest-backed API mock + a simulated/forked chain backend.
- **Solver-agnostic metrics seam.** `solver.Deps.Metrics` (the `Registerer()` extension point) is wired but no solver registers collectors yet; add bridge-facilitator metrics (offers sent/won, exposure, locked vs realized, redemptions) and they'll verify the seam.
