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
- **Target networks:** 3F Sepolia dev (`chainId 11155111`) and Ethereum mainnet (`chainId 1`).

---

## 2. Confirmed decisions

| Topic | Decision |
|---|---|
| Language / toolchain | **Go 1.26** (module declares `go 1.26`; toolchain auto-fetch) |
| Logging | **`logr.Logger`** interface throughout, backed by **zap** via `zapr`; only `main` wires the backend |
| Metrics | Prometheus (`/metrics`); `logr` keeps the logging dependency swappable |
| License | **MIT** (see [`LICENSE`](../LICENSE)) |
| Contract bindings | **abigen over vendored ABIs** in `api/abi/` (ABIs copied from `forge build` output, not hand-curated). `make refresh-abi` re-vendors from a Foundry `out/` dir; build stays hermetic off the committed ABIs. |
| API client | **openapi-generator (Java)** over a vendored OpenAPI snapshot in `openapi/`. `make refresh-openapi` re-pulls the live spec. |
| Adapter scope | One solver serves adapters from exactly one source: an explicit `adapters` list when present, otherwise a dynamic set discovered from a configured on-chain `IAdapterFactory`. Factory enumeration has a hard 2,000-entity limit and returns an error above it. The snapshot is refreshed before every auction-discovery pass; either source is filtered by `offerSigner`, non-zero vault, and non-zero asset. Per auction it can cover the **full requested amount** with one or more single-adapter offers; the default strategy does this most-fundable first, stopping once covered. **1 adapter per offer, no aggregation within an offer** (a single offer is never split across adapters). |
| Persistence | **Stateless + periodic on-chain resync.** No DB. Open requests come from enumerating `adapter.requests(i)` (per adapter); redemption readiness from `canWithdraw()`; auctions/offers from the 3F API. Optional live-log subscription is a latency optimization only, never on the critical path. |
| Key management | Env/file private key behind a pluggable **`Signer`** interface (KMS/remote-signer can be added later without touching call sites). This key is the **EIP-1271 signer every served adapter trusts** (each adapter's owner sets it on-chain): it signs offers with `maker = adapter`, and the adapter's `isValidSignature` authorizes them. The same EOA is the tx-sender for `multicall(finalizeRequest…)` (via the shared `txmanager`). |
| Multi-solver shape | 3F logic fully encapsulated in its own package; a name→factory **registry** selects the impl from config. A **shared `txmanager`** owns on-chain sending so solvers never race on nonces. |

---

## 3. Why view-only monitoring is sufficient

Everything the bot needs is reachable from view functions + the 3F API:

| Need | Source | Type |
|---|---|---|
| Open request count | `adapter.requestsLength()` (single read) | on-chain view |
| Open request set | `adapter.requests(i)` enumerated `0..requestsLength()-1` | on-chain view |
| Per-request valuation | folded into `adapter.totalAssets()` (values each request's PT/YT live) | on-chain view |
| Funding headroom | `adapter.getMaxAssets()` (min(limitOf − totalAssets, withdrawable), 0 if sweep pending) | on-chain view |
| Redeem trigger (loan ready) | `IVaultController(request).canWithdraw()` across the enumerated `requests(i)` | on-chain view |
| Offer won / consumed | next `requests(i)` resync | on-chain view |
| Auction discovery + offer status | `GET /v1/auction`, `GET /v1/offer` | 3F API (off-chain) |
| Realized loss/gain per loan | `FinalizeRequest` log parsed from the bot's **own** `multicall(finalizeRequest…)` receipt | self-emitted |

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
│   │   ├── solver.go  config.go  apiclient.go  auctionview.go  offercache.go
│   │   ├── offer.go   eip712.go  chainreader.go  redeemer.go  strategy.go
│   │   ├── strategies/            # pluggable decision layer:
│   │   │   ├── registry.go        #   package strategies — registry/factory
│   │   │   ├── types/             #   OfferInput/OfferOutput + Strategy interface
│   │   │   ├── default/           #   in-process default strategy (owns sizing/selection)
│   │   │   └── webhook/           #   external-decider adapter
│   ├── observability/             # logr+zap setup, prometheus, /healthz /readyz
│   └── version/
├── api/
│   ├── abi/                       # vendored *.abi.json (copied from forge build)
│   ├── bindings/                  # abigen output (committed), grouped per integration:
│   │   ├── 3f/{adapter,request,vaultcontroller,whitelist}/  # 3F-specific
│   │   ├── adapterfactory/         # shared IAdapterFactory registry surface
│   │   └── vaultv2/                # shared Symbiotic core, reused by every integration
│   └── threef/                    # openapi-generator (Java) output (committed)
├── openapi/3f-bf.openapi.json     # vendored OpenAPI snapshot
├── config/{3f,rfq,redstone-oev}.example.yaml   # one annotated example per solver
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
ABI, e.g. `adapter.PackMulticall(finalizeRequest…)`) and hand it to the txmanager, receiving
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

solvers:
  - name: 3f-bridge-facilitator             # ← registry key: selects the impl
    config:                                  # ← opaque to framework; typed by the 3F package
      apiBaseUrl: https://bf.dev.gcp.3f.xyz
      strategy:
        name: default                        # default local strategy, or webhook
        config: {}
      adapterFactory: "0x…factory"             # used when adapters is omitted; max 2,000 entities
      # adapters:                               # optional exclusive override; factory is not queried
      #   - "0x…adapterA"
      redeemBatchSize: 10                     # optional (default 10)
      httpTimeout: 30s                        # optional
      intervals: { discover: 5m, redeemPoll: 5m, reconcile: 15m }
```

`apiKeyEnv` and the single `adapter`/`vault`/`exposure` keys are **gone**: there is no API key, and each
adapter's **vault + collateral are resolved on-chain** (`adapter.vault()` / `vault.asset()`) and its
**per-request caps are read on-chain** (`minYieldPerRequest` — kept in exact ppm for the offer floor
check and sent as ppm on the webhook wire (the webhook derives bps itself if it needs it);
`minAssetsPerRequest`; `maxAssetsPerRequest` — set via `setLimitsPerRequest`) — config carries the
adapter factory plus an optional exclusive adapter list. When `adapters` is present, the solver resolves
only those entries and does not query the factory. When it is omitted, the solver reads
`totalEntities()` + `entity(i)` for every registry entry on startup and before each discovery pass, then
resolves every candidate. A reported count above the hard 2,000-entity limit returns an error. A
factory-backed deployment may start with a successful empty snapshot and keep running; an
explicit-list configuration still fails startup if none of its adapters validate. A later whole-refresh
RPC failure preserves the last-known-good snapshot; a successful refresh replaces it, so signer changes
remove and can later re-add an adapter. Before every offer pass the solver reconciles its live-offer
cache against the 3F API (per adapter): it re-lists each adapter's live offers and replaces that
adapter's cache wholesale, so coverage reflects offers made out of band — a manual re-offer, a second
instance, or a server-side cancellation — and it never double-offers an auction it already covers. The
1-2 minute poll is authoritative and always surfaces our own just-submitted offers, so any pair not in
the fresh listing is gone and dropped (no local record is kept between passes). Funding headroom is the adapter's own
`getMaxAssets()` (it folds in the delegator's per-adapter `limitOf`, the vault's `withdrawable`, and
any pending sweep), so the bot reads no separate sleeve cap. Concurrency is the contract's
`MAX_REQUESTS` constant (50), mirrored as a bot const. A signed offer's `expiration` is anchored to the
auction's `solve_start_time` plus a configurable `offerExpiryBuffer` (default 2h), never earlier than
`now + buffer`, so the offer stays valid across the whole solve window regardless of when it is signed
(not a fixed TTL from wall-clock).

### Per-auction adapter coverage and strategy split

Each discover tick lists open auctions (public, unauthenticated), then for each auction covers its
**full requested amount** with one or more single-adapter offers, in a single pass:

1. **Solver-owned snapshot** — the solver lists auctions, reconciles its live-offer cache against the
   API and reads each configured adapter's liquidity/exposure in Multicall, prunes the cache, and builds
   a compact strategy input.
   The input contains only raw facts — adapter snapshots (liquidity and on-chain caps), normalized
   auction snapshots, and the live offers the solver already holds. It does not include raw generated
   API DTOs, and the solver computes no capacity, joins, or candidate scoring.
2. **No solver-side decisions** — the solver does not size offers, join adapters to auctions, filter
   eligibility, or rank anything. It provides raw adapter caps and auction facts; the strategy owns
   every decision (sizing, collateral/live-offer/min-yield eligibility, ordering, and cross-auction
   budget accounting) built from those facts.
3. **Strategy decision** — the strategy interface is `DecideOffers(ctx, OfferInput) (OfferOutput, error)`,
   where `OfferOutput` wraps the returned `[]OfferExecution` (`type OfferOutput struct { Offers []OfferExecution }`).
   The selected strategy receives:

   ```go
   type OfferInput struct {
       Now        time
       Adapters   []AdapterSnapshot
       Auctions   []AuctionSnapshot
       LiveOffers []LiveOffer
   }

   type AdapterSnapshot struct {
       ID            string
       Adapter       address
       Vault         address
       Collateral    address
       Fundable      uint256 // getMaxAssets()
       OpenCount     int     // requestsLength()
       MaxAssets     uint256 // maxAssetsPerRequest, 0 = reject-all
       MinAssets     uint256 // minAssetsPerRequest, 0 = disabled
       MinYieldPpm   uint256 // minYieldPerRequest in ppm — exact on-chain floor (also sent on the webhook wire)
       MaxConcurrent int      // MAX_REQUESTS
   }

   type LiveOffer struct {
       AdapterID string
       AuctionID int64
   }
   ```

   It returns ordered execution offers:

   ```go
   type OfferExecution struct {
       AuctionID      int64
       Request        address
       Maker          address
       Principal      uint256
       ExpectedReturn uint256
       Reason         string  // optional, strategy-supplied context
   }
   ```

   The default local strategy: process auctions in API order, filter adapter eligibility (collateral
   match, no live offer for the pair, the auction max rate can reach the adapter's `minYieldPerRequest`),
   compute each adapter's capacity from its raw caps, rank by available capacity (largest first), clamp
   each offer to the still-uncovered remainder, and track local adapter commitments across the pass. Each
   offer is **priced at the adapter's `minYieldPerRequest` floor** (the most competitive rate it allows),
   rounded up so the realised yield always clears the on-chain floor, and skipped if that floor rate
   exceeds the auction's max rate for the sized principal. The submission loop re-checks every strategy's
   offers (default and webhook) against the exact `minYieldPerRequest` before signing, so no path posts a
   sub-floor offer the fill would revert.
   Capacity is `min(min(getMaxAssets, maxAssetsPerRequest), fundable − committed)` gated by the
   concurrency and `minAssetsPerRequest` limits; `maxAssetsPerRequest` is an always-active ceiling (`0`
   means no capacity). A `webhook` strategy posts the same JSON input to an external decider; big
   integers are decimal strings and unknown response fields are rejected.
4. **Side effects** — the solver treats the strategy as trusted. It does not replay or revalidate the
   returned execution offers against caps. It uses the `auctionId` to recover the raw auction EIP-712 domain,
   signs the returned execution offer, submits `createOffer`, and records the live-offer cache only
   after a successful submit. Strategy output cannot set nonce or signature.

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
on demand. ABIs required: `ThreeFAdapter` and `IAdapterFactory` (from core-mirror),
`IRequest`/`IVaultController`, `IWhitelist`, `IVaultV2`.

---

## 8. Build phases

Prerequisite (done). **`ThreeFAdapter` contract** — core-mirror's `src/contracts/adapters/ThreeFAdapter.sol`, ABI vendored here from the core-mirror Foundry build. This is what the bot binds against (it replaced rfq's `BridgeFacilitatorAdapter`).

0. **(done)** Scaffold + tooling — module, layout, Makefile, `.golangci.yml`, CI, README, version pkg. (LICENSE not yet added.)
1. **(done)** Codegen pipeline — ABIs vendored from `../rfq/out`; OpenAPI snapshot; `bindings` (one pkg/contract) + `openapi-client`; committed.
2. **(done)** Core infra (solver-agnostic) — config (two-stage decode), chain primitives, signer, **txmanager (+5 tests)**, solver interface/registry/engine, observability, graceful shutdown.
3. **(done)** 3F solver (encapsulated) — signed-payload API client, offer sizing (now owned by the strategy layer: `getMaxAssets` headroom + per-request caps; Request authorization is the on-chain 3F whitelist), EIP-712 offer signing **+ golden-hash + apitypes parity test**, reconcile + redeemer (poll `canWithdraw` over `requests(0..requestsLength()-1)` → `multicall(finalizeRequest…)` → txmanager), exposure / no-over-commit guards. Deltas tracked in §10.
4. **(done)** Packaging + verification — README/config docs; Sepolia-dev e2e (offers won + redeemed live); multi-stage non-root distroless Dockerfile + compose (`deploy/`, ~20 MB static CGO-free image).
5. **(done) Adapter-as-facilitator + signed payloads + multi-adapter.** The new model (§1, §2, §6),
   implemented across the `bridgefacilitator` package:
   - **Dropped the API key + offer-address.** `listOffers` is now a per-adapter **signed** query (EIP-712
     `GetOffers` in an `Authorization: Bearer` header); `createOffer` sends no `x-api-key`. Removed the
     key-gen, `apiKeyEnv`, and the `ensureOfferAddress`/`setOfferAddress` onboarding. Onboarding (deploy
     adapter → register with 3F → set this signer as EIP-1271 signer) is the vault creator's job.
   - **Dynamic adapter sources + ERC-1271 offer-signer check.** When `adapters[]` is present it is the
     exclusive source; otherwise `adapterFactory` is enumerated at startup and each discovery tick, with
     a hard 2,000-entity limit that returns an error for a larger reported count. Every candidate's
     vault/collateral is re-resolved, and offer-signer authorization is validated via the adapter's own
     ERC-1271 `isValidSignature` (**not** an address match): a one-time payload is signed with the solver
     key at startup and validated against each adapter, so an adapter is kept iff its `offerSigner`
     authorizes our key — our EOA *or* an EIP-1271 contract signer; the probe is reusable across ticks.
     Successful snapshots replace the active set, whole-refresh failures retain the last-known-good set,
     and a factory-backed deployment may validly idle with zero eligible adapters. Explicit-list startup
     retains its fail-closed behavior. The live-offer cache is reconciled against the API before every
     offer pass, so out-of-band offers (manual re-offer, another instance, server-side cancellation)
     count toward coverage and are never double-offered.
   - **Per-auction multi-adapter coverage** (§6): cover each auction's full requested amount with one or
     more single-adapter offers through the configured trusted strategy; uncovered remainder retries
     next pass. Offer dedup, coverage, exposure, redeem, and reconcile all run per adapter.
   - Tests: strategy registry/default selection, default strategy eligibility/sizing, webhook wire shape, per-(adapter,auction) dedup, `liveCoverage`, `reconcileAdapter` wholesale replace (API-authoritative), signed `listOffers` httptest, `resolveAdapters` (incl. the ERC-1271 `isValidSignature` offer-signer probe + unauthorized-drop)
     Multicall round-trip, EIP-712 `GetOffers` golden + apitypes cross-check. The `GetOffers` type string
     and the signer's live-API acceptance are pinned by env-guarded live tests (§9).

---

## 9. Open items to confirm during implementation

- **Signed-payload API contract** — confirm with 3F the exact request shape for creating *and listing*
  offers without an API key: how a list request is authenticated/scoped to an adapter (the signed payload),
  and that 3F verifies offer creation via the adapter's EIP-1271 `isValidSignature`.
- Filtering to adapters our key is authorized to sign for is settled: the adapter's ERC-1271
  `isValidSignature` probe (§8), reused across factory-discovery ticks.
- Mainnet `RequestWhitelist` address and prod API base URL — operational onboarding inputs supplied by
  3F; the bot discovers adapter instances from their factory and does not configure the whitelist itself.
- Go module path (`github.com/symbioticfi/vault-solver` placeholder) — adjust to the real org.

---

## 10. Pending / deferred items (post-Phase-3)

Tracked TODOs and known gaps — each a scoped follow-up; none block release.

**Deferred features / known gaps:**
- **(done) Exposure / risk params are on-chain.** The per-request caps (`minYieldPerRequest` in ppm, `minAssetsPerRequest`, `maxAssetsPerRequest`) live on the `ThreeFAdapter` and are read per-adapter via Multicall each discover tick (`chainreader.go`); the bot no longer carries config exposure caps. Funding headroom is the adapter's own `getMaxAssets()` (folds in the delegator `limitOf`, vault `withdrawable`, and pending sweep), and the concurrency cap is the contract's `MAX_REQUESTS` constant — neither is a separate adapter read. Trust-minimized + curator-governed, as planned.
- **Multi-maker offers.** An auction's ask is covered by **multiple single-adapter offers** (most-fundable first, sized to the uncovered remainder), but a **single** offer is still funded by one adapter. Splitting one offer across several makers (true aggregation) is deferred — needs multi-maker offer support on-chain.
- **Re-pricing live offers on rising yield.** An auction's `maxRate` can climb over time, so an auction infeasible now (below an adapter's `minYieldPerRequest`) becomes feasible later — handled, since infeasible auctions are never negatively cached and each pass re-evaluates. But a live offer placed at an earlier, lower rate is **not** re-priced upward while it stays live (dedup by `(adapter, auction)`); capturing the higher rate would need cancel/replace (depends on `OfferControllerCancelV1`, below).
- **(done) Dynamic adapter discovery.** When `adapters` is omitted, every entry in the configured
  `IAdapterFactory` is enumerated at startup and before each discovery pass, subject to a hard 2,000-entry
  limit that errors above it. When `adapters` is present, only that explicit list is used. Either source
  is filtered to adapters whose non-zero vault/asset resolve and that authorize this solver's signer via
  the adapter's ERC-1271 `isValidSignature` (**not** an address match; EOA or contract signer).
- **Custom offer pricing/scoring.** The default local strategy bids at the auction's current `maxRate`
  and sizes by `getMaxAssets` headroom plus adapter per-request limits. Operators that need spread,
  risk-adjusted target rate, time-in-auction, or competing-offer logic should replace it with a local
  custom strategy or the built-in `webhook` strategy. The strategy returns principal and expected
  return; the solver only signs and submits the returned offer.
- **Offer cancellation.** `OfferControllerCancelV1` not wired — needs offer-id↔auction state.
- **WS live-log subscription** (`chain.wsUrl`) — config field present but unused; the poll-based reconcile/redeem path is sufficient for v0.

**Testing:**
- **Integration coverage.** `bridgefacilitator` unit coverage is ~16% — pure logic (EIP-712 golden+parity, default-strategy capacity/caps, config) is covered; the HTTP/on-chain paths (apiclient, chainreader, redeemer, Run loop) need an httptest-backed API mock + a simulated/forked chain backend.
- **Solver-agnostic metrics seam.** `solver.Deps.Metrics` (the `Registerer()` extension point) is wired but no solver registers collectors yet; add bridge-facilitator metrics (offers sent/won, exposure, locked vs realized, redemptions) and they'll verify the seam.
