# 3F Bridge Facilitator

> **Role:** maintained integration contract for `3f-bridge-facilitator`: protocol scope, design decisions,
> deployment prerequisites, and live open work. Runtime behavior is summarized in the root README; shared
> boundaries live in [Architecture](ARCHITECTURE.md).
>
> **Code/config:** `internal/solvers/bridgefacilitator` · [`config/3f.example.yaml`](../config/3f.example.yaml)

The integration remains self-contained and shares only the generic framework with other solvers.

---

## 1. Scope

- **In scope:** the off-chain Go bot, serving **multiple `ThreeFAdapter`s** — auction
  discovery, **per-auction multi-adapter coverage**, offer pricing/sizing/signing (signed payloads),
  on-chain reads for liquidity, position reconciliation, and redemption.
- **Out of scope:** the on-chain `ThreeFAdapter` (Solidity, consumed via generated ABI
  bindings) **and its 3F onboarding**. In the new model each adapter is deployed and registered with 3F
  **as a facilitator by its own vault creator**, who then sets this solver's signer as the adapter's
  **EIP-1271 signer**. The bot registers nothing with 3F and holds no API key.
- **Target networks:** 3F Sepolia dev (`chainId 11155111`) and Ethereum mainnet (`chainId 1`).

---

## 2. Confirmed decisions

| Topic | Decision |
|---|---|
| Adapter scope | One solver serves adapters from exactly one source: an explicit `adapters` list when present, otherwise a dynamic set discovered from a configured on-chain `IAdapterFactory`. Factory enumeration has a hard 2,000-entity limit and returns an error above it. The snapshot is refreshed before every auction-discovery pass; either source is filtered by `offerSigner`, non-zero vault, and non-zero asset. Per auction it can cover the **full requested amount** with one or more single-adapter offers; the default strategy does this most-fundable first, stopping once covered. **1 adapter per offer, no aggregation within an offer** (a single offer is never split across adapters). |
| Persistence | **Stateless + periodic on-chain resync.** No DB. Open requests come from enumerating `adapter.requests(i)` (per adapter); redemption readiness from `canWithdraw()`; auctions/offers from the 3F API. Optional live-log subscription is a latency optimization only, never on the critical path. |
| Key management | Env/file private key behind a pluggable **`Signer`** interface (KMS/remote-signer can be added later without touching call sites). This key is the **EIP-1271 signer every served adapter trusts** (each adapter's owner sets it on-chain): it signs offers with `maker = adapter`, and the adapter's `isValidSignature` authorizes them. The same EOA is the tx-sender for `multicall(finalizeRequest…)` (via the shared `txmanager`). |
| Multi-solver shape | 3F logic fully encapsulated in its own package; the command's immutable descriptor selects its validator and factory from config. A **shared `txmanager`** owns on-chain sending so solvers never race on nonces. |

---

## 3. Why view-only monitoring is sufficient

Everything the bot needs is reachable from view functions + the 3F API:

| Need | Source | Type |
|---|---|---|
| Open request count | `adapter.requestsLength()` (single read) | on-chain view |
| Open request set | `adapter.requests(i)` enumerated `0..requestsLength()-1` | on-chain view |
| Per-request valuation | folded into `adapter.totalAssets()` (values each request's PT/YT live) | on-chain view |
| Funding headroom | `adapter.getMaxAssets()` by default, or the configured liquidity lens's cross-adapter estimate | on-chain view |
| Redeem trigger (loan ready) | `IVaultController(request).canWithdraw()` across the enumerated `requests(i)` | on-chain view |
| Offer won / consumed | next `requests(i)` resync | on-chain view |
| Auction discovery + offer status | `GET /v1/auction`, `GET /v1/offer` | 3F API (off-chain) |
| Realized loss/gain per loan | `FinalizeRequest` log parsed from the bot's **own** `multicall(finalizeRequest…)` receipt | self-emitted |

Trade-off accepted: view-only loses *latency* (learn of consume/repay on the next
poll tick) and *historical analytics*. Neither matters for 3F — funding pull time is
known hours ahead, repayments land 24h–30d+ out, and there is no sub-second deadline.
No historical event indexer, no DB.

---

## 4. Source map

| Area | Owner |
|---|---|
| Runtime, config parsing, API adapter, reconciliation, signing, and redemption | `internal/solvers/bridgefacilitator` |
| Solver-local strategy contract and built-ins | `internal/solvers/bridgefacilitator/strategies` |
| 3F contract bindings | `api/bindings/3f/...` |
| Adapter-factory binding | `api/bindings/adapterfactory` |
| Generated 3F HTTP client | `api/threef` |
| External contracts of record | `api/abi/*.json`, `openapi/3f-bf.openapi.json` |
| Exact runtime configuration | [`config/3f.example.yaml`](../config/3f.example.yaml) |

---

## 5. Framework integration

The package exports its runtime factory and pure config validator; the command binds them to the runtime name.
Generic composition and startup rules live in [Architecture](ARCHITECTURE.md); nonce, fee, replacement, and
shutdown behavior live in [Transaction manager](TXMANAGER.md).

3F-specific integration rules are:

- offer signing and transaction sending are distinct protocol roles, currently backed by the same framework
  signer/EOA;
- redemptions submit generated calldata through the shared transaction manager rather than sending directly;
- an occupied or conflicted transaction lane pauses new offers before planning and again before submission;
- offer reconciliation and redemption continue while new commitments are paused.

---

## 6. Configuration & per-offer adapter selection

The framework selects the integration by `solvers[].name` and passes `solvers[].config` through for strict,
integration-owned decoding. The exact fields and defaults live only in the annotated
[`config/3f.example.yaml`](../config/3f.example.yaml); the semantics below explain the non-obvious adapter
selection and offer rules.

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
instance, or a server-side cancellation — and it never double-offers an auction it already covers. Each successful listing is authoritative and includes our just-submitted offers, so any pair absent from the
fresh response is dropped (no local record is retained between passes). Funding headroom comes from the adapter's `getMaxAssets()` by default; when `liquidityLens` is configured,
the lens supplies the stricter cross-adapter deallocation-cascade estimate. Both paths remain on-chain facts,
not strategy guesses. Concurrency is the contract's
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
       Fundable      uint256 // selected adapter/lens funding headroom
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
   offer is priced at the adapter's `minYieldPerRequest` floor plus
   `ceil(principal / max(1, minAssetsPerRequest))` base units. `Request.consume()` floors the pro-rata
   return before the adapter checks yield; this margin contributes at least one whole return unit at the
   smallest principal the adapter accepts, so every larger partial consumption also clears the floor.
   A floor-exact offer lacks that guarantee (mainnet tx `0xc637…8386` failed one unit short). If the
   auction's max rate cannot fit the required margin, the pair is skipped rather than clamped to an
   unsafe full-offer yield. Consequently, a zero `minAssetsPerRequest` with a positive yield floor is
   normally infeasible under realistic auction caps because the adapter permits a one-unit consume.
   Before signing, the submission loop applies the same full and minimum-partial validation to default
   and webhook outputs.
   Capacity is `min(maxAssetsPerRequest, fundable − committed)` gated by the
   concurrency and `minAssetsPerRequest` limits; `maxAssetsPerRequest` is an always-active ceiling (`0`
   means no capacity). A `webhook` strategy posts the same JSON input to an external decider; big
   integers are decimal strings and unknown response fields are rejected.
4. **Side effects** — the solver treats the strategy as trusted and does not replay its allocation or
   capacity decisions. It still rejects amounts below `minAssetsPerRequest` and yields that can fail at
   either the full or minimum partial principal. It uses the `auctionId` to recover the raw auction
   EIP-712 domain, signs the returned execution offer, submits `createOffer`, and records the live-offer
   cache only after a successful submit. Strategy output cannot set nonce or signature.

---

## 7. External contracts

The vendored OpenAPI specification and ABIs are the external contracts of record. This integration consumes
`ThreeFAdapter`, `IAdapterFactory`, `IRequest`, `IVaultController`, `IWhitelist`, and `IVaultV2`; generated Go
remains committed. Use the canonical refresh/generate commands in
[Development](DEVELOPMENT.md#code-generation), then inspect and test the 3F consumers.

---

## 8. Implementation status

The signed-payload, dynamic/static multi-adapter, strategy, reconciliation, and redemption paths are
implemented and covered by unit, HTTP, RPC-decoder, EIP-712 parity, and opt-in live tests. A Sepolia offer and
redemption flow has been exercised. Durable gaps and external confirmations remain in §9–§10; completed phase
history belongs in Git.

---

## 9. External confirmations

- **Signed-payload API contract** — confirm with 3F the exact request shape for creating *and listing*
  offers without an API key: how a list request is authenticated/scoped to an adapter (the signed payload),
  and that 3F verifies offer creation via the adapter's EIP-1271 `isValidSignature`.
- Mainnet `RequestWhitelist` address and prod API base URL — operational onboarding inputs supplied by
  3F; the bot discovers adapter instances from their factory and does not configure the whitelist itself.

---

## 10. Deferred work

Tracked TODOs and known gaps — each a scoped follow-up; none block release.

**Deferred features / known gaps:**
- **Multi-maker offers.** An auction's ask is covered by **multiple single-adapter offers** (most-fundable first, sized to the uncovered remainder), but a **single** offer is still funded by one adapter. Splitting one offer across several makers (true aggregation) is deferred — needs multi-maker offer support on-chain.
- **Re-pricing live offers on rising yield.** An auction's `maxRate` can climb over time, so an auction infeasible now (below an adapter's `minYieldPerRequest`) becomes feasible later — handled, since infeasible auctions are never negatively cached and each pass re-evaluates. But a live offer placed at an earlier, lower rate is **not** re-priced upward while it stays live (dedup by `(adapter, auction)`); capturing the higher rate would need cancel/replace (depends on `OfferControllerCancelV1`, below).
- **Custom offer pricing/scoring.** The default local strategy bids at the lowest partial-safe yield
  permitted by the adapter floor and auction cap, and sizes by selected adapter/lens headroom plus adapter
  per-request limits. Operators that need spread, risk-adjusted target rate, time-in-auction, or
  competing-offer logic should replace it with a local custom strategy or the built-in `webhook`
  strategy. The strategy returns principal and expected return; the solver signs and submits it after
  the minimum-partial yield backstop.
- **Offer cancellation.** `OfferControllerCancelV1` not wired — needs offer-id↔auction state.

**Testing:**
- **Integration coverage.** `bridgefacilitator` statement coverage is ~66%. API auction reads, exposure
  snapshots, signed DTO construction, and redeemable-request scanning have httptest/RPC-backed tests.
  The complete `discoverAndOffer` and redemption-transaction orchestration still need a simulated or
  forked backend.
- **Bridge-facilitator metrics.** RFQ, RedStone, and UniswapX already use the shared metrics registerer;
  add 3F collectors for offers sent/won, exposure, locked vs realized assets, and redemptions.
