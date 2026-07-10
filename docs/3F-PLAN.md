# vault-solver — Implementation Plan (v0)

> A Go service that monitors a configured selection of Symbiotic vaults and runs a
> pluggable **solver** strategy against them. The **3F Bridge Facilitator** off-chain
> bot is one integration alongside RFQ and Redstone/OEV. Each integration remains
> self-contained so it can evolve without changing the generic framework.

This document is the source of truth for the 3F solver's agreed scope, architecture,
decisions, and live follow-up list.

---

## 1. Scope

- **In scope:** the off-chain Go bot, serving **multiple `ThreeFAdapter`s** — auction
  discovery, **per-auction multi-adapter coverage**, offer pricing/sizing/signing (signed payloads),
  on-chain reads for liquidity, position reconciliation, and redemption.
- **Out of scope:** the on-chain `ThreeFAdapter` (Solidity, consumed via generated ABI
  bindings) **and its 3F onboarding**. In the new model each adapter is deployed and registered with 3F
  **as a facilitator by its own vault creator**, who then sets this solver's signer as the adapter's
  **EIP-1271 signer**. The bot registers nothing with 3F and holds no API key.
- **First target network:** 3F Sepolia dev (`chainId 11155111`). Mainnet config slots in later.

---

## 2. Confirmed decisions

| Topic | Decision |
|---|---|
| Language / toolchain | **Go 1.26.5** (module declares `go 1.26`; CI and local gates pin `GOTOOLCHAIN=go1.26.5`) |
| Logging | **`logr.Logger`** interface throughout, backed by **zap** via `zapr`; only `main` wires the backend |
| Metrics | Prometheus (`/metrics`); `logr` keeps the logging dependency swappable |
| License | _TBD — not yet added_ |
| Contract bindings | **abigen over vendored ABIs** in `api/abi/` (ABIs copied from `forge build` output, not hand-curated). `make refresh-abi` re-vendors from a Foundry `out/` dir; build stays hermetic off the committed ABIs. |
| API client | **openapi-generator (Java)** over a vendored OpenAPI snapshot in `openapi/`. `make refresh-openapi` re-pulls the live spec. |
| Adapter scope | One solver serves a **set of adapters** (config whitelist now; a dynamic "list public 3F adapters" API later). Per auction it can cover the **full requested amount** with one or more single-adapter offers; the default strategy does this most-fundable first, stopping once covered. **1 adapter per offer, no aggregation within an offer** (a single offer is never split across adapters). |
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
│   ├── txmanager/                 # SHARED nonce dispatcher + concurrent receipt trackers
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
│   │   ├── rfq/  oev/                                      # other integration-specific bindings
│   │   └── vaultv2/               # shared Symbiotic core, reused by every integration
│   └── threef/                    # openapi-generator (Java) output (committed)
├── openapi/3f-bf.openapi.json     # vendored OpenAPI snapshot
├── config/{3f,rfq,redstone-oev}.example.yaml   # one annotated example per solver
├── deploy/{Dockerfile,docker-compose.yml}
├── .github/workflows/ci.yml
├── .golangci.yml  Makefile  go.mod  README.md  .gitignore
```

---

## 5. Shared infrastructure

### 5.1 `txmanager` — serialized dispatcher, concurrent trackers

A single service owns the on-chain sending EOA. Its dispatcher serializes nonce seeding/allocation
plus original-attempt construction, signing, and initial broadcast for
`Request{To, Data, Value, GasLimit?, Label}`. Once an initial broadcast is admitted or ambiguous, an
independent tracker owns that logical transaction, so an earlier pending nonce does not block the
dispatcher from signing and broadcasting later nonces. The committed nonce floor prevents an
admitted/ambiguous nonce from being reused even when an RPC pending-nonce response is stale. Solvers
**never** send directly; they build calldata with generated ABI helpers and hand it to txmanager.

Each tracker re-reads receipts for every same-nonce attempt and accepts one only when its block hash
matches the canonical header and the configured confirmation depth has elapsed. Trackers construct,
sign, and broadcast bounded same-nonce, same-payload EIP-1559 fee replacements concurrently:
`pendingIntervalMs` (default
120000 ms) bounds each attempt window, `feeBumpBps` (default 1250) controls each increase,
`maxReplacements` (default 3) bounds the attempt count, and `maxFeeGwei` is a hard cap. The result state
is exactly one of `not_broadcast`, `rejected`,
`broadcast_unknown`, `pending`, `confirmed`, `reverted`, or `unresolved`, accompanied as applicable by
`Nonce`, the newest `Hash`, all `Hashes`, a canonical `Receipt`, and `Err`. `SafeToRetry()` is true only
for `not_broadcast` and `rejected`; consumers branch on `State`, never infer ambiguity or retry safety
from `Err`.

The 3F redeemer treats `confirmed` as complete. `unresolved` and any unexpected/intermediate state
conservatively record every request in the submitted batch in a per-`(adapter, request)` pending set;
those requests are suppressed until a later successful authoritative `readyToRedeem` scan proves that
they are no longer ready or no longer active. A failed or undecodable per-request `canWithdraw`
sub-call remains unknown and preserves only that request's pending key; known-ready requests from the
same scan still proceed through filtering and batching. Every successful scan, including an empty one,
reconciles its known results before filtering, while a whole-scan error preserves every pending key for
that adapter. `not_broadcast`, `rejected`, and `reverted` are definite outcomes and are not suppressed,
so a later authoritative scan may make them eligible again. The map is owned only by the single
`Solver.Run` goroutine and needs no mutex; adapter-qualified keys prevent one adapter's scan from
clearing another's suppression.

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
Adding another solver follows the same register + config pattern, with no generic framework edit.

---

## 6. Configuration & per-offer adapter selection

Two-stage decode keeps solver config encapsulated. The generic layer reads only
`solver.name` to pick the impl and keeps `solver.config` as a deferred `yaml.Node`;
the chosen solver decodes it into its own typed struct.

```yaml
chain: { rpcUrl, chainId, rpcFallbackUrls?, writeRpcUrl? } # all distinct endpoints are chain-ID preflighted
signer: { keyEnv: SOLVER_PRIVATE_KEY }     # the EIP-1271 signer every served adapter trusts
txManager: { confirmations: 2, maxFeeGwei, tipGwei, pendingIntervalMs: 120000, feeBumpBps: 1250, maxReplacements: 3 }

solvers:
  - name: 3f-bridge-facilitator             # ← registry key: selects the impl
    config:                                  # ← opaque to framework; typed by the 3F package
      apiBaseUrl: https://bf.dev.gcp.3f.xyz
      strategy:
        name: default                        # default local strategy, or webhook
        config: {}
      # The adapters this solver maintains offers for. Each must already be registered with 3F as a
      # facilitator by its vault creator, with this solver's signer set as the adapter's EIP-1271 signer.
      # A config whitelist for now; a dynamic "list public 3F adapters" API replaces it later.
      adapters:
        - "0x…adapterA"
        - "0x…adapterB"
      redeemBatchSize: 10                     # optional (default 10)
      httpTimeout: 30s                        # optional
      intervals: { discover: 1h, offerTTL: 2h, redeemPoll: 5m, reconcile: 15m }
```

`intervals.offerTTL` defaults to twice `discover` and must be at least `discover`, so the default
schedule never sets a signed expiration earlier than the next discovery pass. Fractional durations
remain valid; offer construction rounds the expiration upward when converting to Unix seconds, never
shortening the configured lifetime. One injected solver clock drives the signed expiration, DTO
expiration, and live-offer cache snapshots.

The vendored 3F schema represents auction, offer, and chain identities as `int64`, preserving exact
signature inputs beyond JavaScript's 2^53 boundary. Request-contract domains may carry an optional
bytes32 salt; the solver validates and includes it in `OfferDigest`, while unsalted domains retain the
original digest. Generated-client responses are bounded to 8 MiB and every request is covered by the
configured `httpTimeout`, preventing a large or stalled API response from blocking redemption scans.

At startup, the generic chain layer preflights every configured read endpoint (primary and fallback)
plus any distinct write endpoint against `chainId`. Diagnostics identify endpoints only by safe
origin labels (`scheme://host`), never by userinfo, path, query, or fragment.

`apiKeyEnv` and the single `adapter`/`vault`/`exposure` keys are **gone**: there is no API key, and each
adapter's **vault + collateral are resolved on-chain** (`adapter.vault()` / `vault.asset()`) and its
**per-request caps are read on-chain** (`minYieldPerRequest` — ppm, converted to bps by the reader;
`minAssetsPerRequest`; `maxAssetsPerRequest` — set via `setLimitsPerRequest`) — config carries only the
adapter addresses. Funding headroom is the adapter's own `getMaxAssets()` (it folds in the delegator's
per-adapter `limitOf`, the vault's `withdrawable`, and any pending sweep), so the bot reads no separate
sleeve cap. Concurrency is the contract's `MAX_REQUESTS` constant (50), mirrored as a bot const.

### Per-auction adapter coverage and strategy split

Each discover tick lists open auctions (public, unauthenticated), then for each auction covers its
**full requested amount** with one or more single-adapter offers, in a single pass:

1. **Solver-owned snapshot** — the solver lists auctions, reads each configured adapter's
   liquidity/exposure in Multicall, prunes its live-offer cache, and builds a compact strategy input.
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
       MinYieldBps   uint256 // minYieldPerRequest converted from ppm to bps
       MaxConcurrent int      // MAX_REQUESTS
   }

   type AuctionSnapshot struct {
       // Other auction identity and amount fields omitted here for brevity.
       MaxRateDeciBps uint256 // exact count of tenth-basis-points
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

   The generated API retains its numeric `maxRate` field, but the solver normalizes that `float64`
   exactly once into `MaxRateDeciBps`. Missing, negative, non-finite, or finer-than-one-tenth values
   are rejected before the strategy boundary. The default local strategy preserves the current
   behavior: process auctions in API order, filter adapter eligibility (collateral match, no live
   offer for the pair, `MaxRateDeciBps >= MinYieldBps * 10`),
   compute each adapter's capacity from its raw caps, rank by available capacity (largest first), clamp
   each offer to the still-uncovered remainder, and track local adapter commitments across the pass.
   Capacity is `min(min(getMaxAssets, maxAssetsPerRequest), fundable − committed)` gated by the
   concurrency and `minAssetsPerRequest` limits; `maxAssetsPerRequest` is an always-active ceiling (`0`
   means no capacity). Expected return is the exact, round-down calculation
   `principal * MaxRateDeciBps / 100_000`. A `webhook` strategy posts the same JSON input to an
   external decider; big integers are decimal strings, `maxRateBps` is an exact decimal string such as
   `"50.5"`, and unknown response fields are rejected.
4. **Side effects** — the solver treats the strategy as trusted. It does not replay or revalidate the
   returned execution offers against caps. It uses the `auctionId` to recover the raw auction EIP-712 domain,
   validates and includes its optional salt, signs the returned execution offer, submits `createOffer`,
   and records the live-offer cache only after a successful submit. Strategy output cannot set nonce or
   signature.

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

Prerequisite (done). **`ThreeFAdapter` contract** — core-mirror's
`src/contracts/adapters/ThreeFAdapter.sol`, with its ABI vendored from the core-mirror Foundry build.
This is the active contract the bot binds against.

0. **(done)** Scaffold + tooling — module, layout, Makefile, `.golangci.yml`, CI, README, version pkg. (LICENSE not yet added.)
1. **(done)** Codegen pipeline — ABIs vendored from `../rfq/out`; OpenAPI snapshot; `bindings` (one pkg/contract) + `openapi-client`; committed.
2. **(done)** Core infra (solver-agnostic) — config (two-stage decode), chain primitives, signer,
   **txmanager (36 top-level tests)**, solver interface/registry/engine, observability, and supervised
   graceful shutdown.
3. **(done)** 3F solver (encapsulated) — signed-payload API client, offer sizing (now owned by the strategy layer: `getMaxAssets` headroom + per-request caps; Request authorization is the on-chain 3F whitelist), EIP-712 offer signing **+ golden-hash + apitypes parity test**, reconcile + redeemer (poll `canWithdraw` over `requests(0..requestsLength()-1)` → `multicall(finalizeRequest…)` → txmanager), exposure / no-over-commit guards. Deltas tracked in §10.
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
     more single-adapter offers through the configured trusted strategy; uncovered remainder retries
     next pass. Offer dedup, coverage, exposure, redeem, and reconcile all run per adapter.
   - Tests: strategy registry/default selection, exact deci-bps conversion/arithmetic and yield-floor
     boundaries, default strategy eligibility/sizing, webhook wire shape, per-(adapter,auction) dedup,
     `liveCoverage`, signed `listOffers` httptest, `authorizedSigner`
     Multicall round-trip, EIP-712 `GetOffers` golden + apitypes cross-check, and hermetic production-path
     characterization from auction API + Multicall snapshot through salted offer submission, plus
     request enumeration through bounded redemption calldata and unresolved-outcome reconciliation.
     The `GetOffers` type string and the signer's live-API acceptance are pinned by env-guarded live
     tests (§9).

---

## 9. Open items to confirm during implementation

- **Dynamic "list public 3F adapters" API** — the endpoint that lets deployed solvers discover their
  adapter set without config-pinned addresses (what marks an adapter public/eligible, and how we filter
  to ones our signer is the EIP-1271 signer for).
- Mainnet `RequestWhitelist` address and prod API base URL — supplied by 3F when prod lands.
- Go module path (`github.com/symbioticfi/vault-solver` placeholder) — adjust to the real org.

---

## 10. Pending / deferred items (post-Phase-3)

Tracked TODOs and known gaps — each a scoped follow-up; none block release.

**Deferred features / known gaps:**
- **(done) Exposure / risk params are on-chain.** The per-request caps (`minYieldPerRequest` in ppm, `minAssetsPerRequest`, `maxAssetsPerRequest`) live on the `ThreeFAdapter` and are read per-adapter via Multicall each discover tick (`chainreader.go`); the bot no longer carries config exposure caps. Funding headroom is the adapter's own `getMaxAssets()` (folds in the delegator `limitOf`, vault `withdrawable`, and pending sweep), and the concurrency cap is the contract's `MAX_REQUESTS` constant — neither is a separate adapter read. Trust-minimized + curator-governed, as planned.
- **(done) Auction rates are exact after ingress.** The upstream numeric `maxRate` remains generated
  as `float64`, then is validated and normalized once to integer tenth-basis-points. Eligibility,
  expected-return arithmetic, and webhook transport use only the exact integer/decimal-string form.
- **Multi-maker offers.** An auction's ask is covered by **multiple single-adapter offers** (most-fundable first, sized to the uncovered remainder), but a **single** offer is still funded by one adapter. Splitting one offer across several makers (true aggregation) is deferred — needs multi-maker offer support on-chain.
- **Re-pricing live offers on rising yield.** An auction's `maxRate` can climb over time, so an auction infeasible now (below an adapter's `minYieldPerRequest`) becomes feasible later — handled, since infeasible auctions are never negatively cached and each pass re-evaluates. But a live offer placed at an earlier, lower rate is **not** re-priced upward while it stays live (dedup by `(adapter, auction)`); capturing the higher rate would need cancel/replace (depends on `OfferControllerCancelV1`, below).
- **Dynamic adapter discovery.** The adapter set is a config whitelist; the dynamic "list public 3F adapters" API (§9) replaces it later, filtered to adapters our signer is the EIP-1271 signer for.
- **Custom offer pricing/scoring.** The default local strategy bids at the auction's current `maxRate`
  and sizes by `getMaxAssets` headroom plus adapter per-request limits. Operators that need spread,
  risk-adjusted target rate, time-in-auction, or competing-offer logic should replace it with a local
  custom strategy or the built-in `webhook` strategy. The strategy returns principal and expected
  return; the solver only signs and submits the returned offer.
- **Offer cancellation.** `OfferControllerCancelV1` not wired — needs offer-id↔auction state. The
  configured offer lifetime already covers discovery, and dedup prevents redundant live re-offers.
**Testing:**
- **(done) Offer/redemption boundary coverage.** Hermetic tests exercise the production generated API
  client, decoded Multicall reads, default strategy, local signer, successful/failed offer tracking,
  real txmanager submission, bounded `finalizeRequest` ordering, and unresolved-result suppression
  through authoritative on-chain reconciliation.
- **Long-running integration coverage.** The complete ticker-driven `Run` lifecycle and live/forked
  chain behavior remain candidates for deployment-level tests; the money-moving offer and redemption
  boundaries no longer depend on test-only production seams.
- **Solver-agnostic metrics seam.** `solver.Deps.Metrics` (the `Registerer()` extension point) is wired but no solver registers collectors yet; add bridge-facilitator metrics (offers sent/won, exposure, locked vs realized, redemptions) and they'll verify the seam.
