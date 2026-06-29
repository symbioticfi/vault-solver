# 3F Multi-Adapter (adapter-as-facilitator, signed payloads) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the `bridgefacilitator` (3F) solver from a single registered facilitator (API key + offer-address) into a keyless, **multi-adapter** solver that maintains offers on behalf of any adapter whose **EIP-1271 signer** is our key, selecting the single best-fit adapter per auction.

**Architecture:** The adapter contract *is* the 3F facilitator (registered by its vault creator, with our signer set as its EIP-1271 signer). We authenticate purely with signatures: offer **creation** carries the EIP-712 `Offer` signature (`maker = adapter`, verified on-chain via the adapter's `isValidSignature`); offer **listing** uses an EIP-712 `GetOffers` signature in an `Authorization: Bearer` header against `maker=<adapter>`. One solver serves a config whitelist of adapters; per auction it picks the single adapter that can fund the largest amount within its on-chain exposure caps (1 adapter per offer, no aggregation).

**Tech Stack:** Go 1.26, `go-errors`, `logr`, generated `api/threef` OpenAPI client, `api/bindings/3f/*` abigen bindings, `internal/{chain,signer,txmanager}` shared infra. Tests: `go test -race`, table-driven + golden EIP-712 + httptest.

## Global Constraints

- Toolchain pinned: `GOTOOLCHAIN=go1.26.4`. Match it.
- Errors: `github.com/go-errors/errors` (`errors.Errorf("...: %w")`, `errors.New`) — never `fmt.Errorf` (`forbidigo` enforces).
- Logging: `logr.Logger`, structured key/values; `V(1)` for debug. Never log a secret/signature except a documented `V(1)` line.
- Solvers never send transactions directly — build calldata, hand to the shared `txmanager`.
- All config comes from YAML; secrets via `*Env` indirection read with `os.Getenv` at point of use. No hardcoded addresses/URLs.
- Generated code (`api/threef`, `api/bindings/**`) is never hand-edited; regenerate via `make`.
- Gate (must stay green): `GOTOOLCHAIN=go1.26.4 golangci-lint run --fix && go build ./... && go test -race -cover ./... && golangci-lint run`.
- Keep `docs/3F-PLAN.md` in sync in the same change (CLAUDE.md plan rule).
- Commits: author `oxsteins`, **no** `Co-Authored-By` trailer.

---

## Verified ground truth (read before starting)

- **3F API (`openapi/3f-bf.openapi.json`):** `POST /v1/offer` — `x-api-key` *optional* → create is signed-only (the `Offer` EIP-712 signature in the DTO). `GET /v1/offer` — params `maker` (required), `chainId?`, `deadline?`, header `Authorization?` ("`Bearer <signature>` for EIP-712 authenticated requests; `deadline` required when using it"). So **listing by `maker=<adapter>` = `Authorization: Bearer <GetOffers sig>` + `deadline`**.
- **EIP-712 pattern:** `internal/solvers/bridgefacilitator/eip712.go`. `APIKeyDigest` (lines 124-134) is the template for `GetOffersDigest`: grunt domain `{name:"grunt-api", version:"1", chainId:1}` (no `verifyingContract`), `keccak256(0x1901 ‖ domainSeparator ‖ structHash)`. `OfferDigest` (maker=adapter) already exists and is unchanged.
- **Exposure is on-chain per adapter** (`chainreader.go` `exposureState`): `limitOf`, `totalAssets`, `outstandingPrincipal`, `activeRequests`, `withdrawable`, `perRequestMaxCollateral`, `totalMaxCollateral`, `minRequestYieldBps`, `maxConcurrentLoans`. `sizeOffer` (`sizer.go:24-47`) applies caps in order. **No config exposure.**
- **Current solver loop** (`solver.go`): `Run` (3 tickers) → `onboard` (REMOVE) → `discoverAndOffer`→`offerForTarget` (single Target) → `redeemAll` → `reconcile`. `resolveTarget` resolves the single adapter's vault/collateral.
- **Decisions (locked):** adapter selection = **most fundable** (largest `sizeOffer` result; rate is fixed per auction so this maximizes expected return); `GetOffers` type = **scaffold + golden-test vs the live API** (like `TestAPIKeyDigest_MatchesLiveAcceptedSignature`); **verify the EIP-1271 signer on-chain at startup** (skip/warn adapters where our signer isn't authorized); the in-progress uncommitted bf edits are stashed — start from the committed base.

---

## File structure

| File | Change |
|---|---|
| `config.go` | `adapter string` → `adapters []string`; `Config.Target` → `Config.Targets []Target`; require ≥1 adapter. (`apiKeyEnv` removed later, in Task 3, when the client that reads it is rewritten — keeps every commit green.) |
| `config_test.go` | adapters-list parsing, empty-list + zero-address rejection |
| `eip712.go` | add `GetOffersDigest(maker, deadline)` + `getOffersTypeString` |
| `eip712_test.go` | `GetOffers` golden hash + live-API parity test |
| `apiclient.go` | drop key-gen/`ensureKey`/`withAuth` retry/`offerAddress`/`setOfferAddress`; `listOffers(adapter)` → Bearer `GetOffers` sig; constructor drops `fallbackKey`/`facilitator` |
| `chainreader.go` | add `authorizedSigner(adapter)` read (EIP-1271 signer); exposure read already per-adapter |
| `offercache.go` | dedup key `(adapter, auctionID)` not `auctionID` |
| `selection.go` (new) | `selectBestAdapter` — most-fundable pick |
| `solver.go` | remove `onboard`/`ensureOfferAddress`; `resolveTargets` (all adapters) + startup signer verification; multi-adapter discover/redeem/reconcile loops |
| `config/3f.sepolia.example.yaml` | `adapters:` list; drop `apiKeyEnv` |
| `docs/3F-PLAN.md` | already updated; sync any deltas (§8 phases) |

The adapter binding methods (`vault()`, `vault.asset()`, exposure getters, and the **authorized-signer getter**) live under `api/bindings/3f/adapter` — confirm exact names there before writing the on-chain reads (Task 4).

---

## Task 1: Config — `adapters[]` list

**Files:**
- Modify: `internal/solvers/bridgefacilitator/config.go`
- Test: `internal/solvers/bridgefacilitator/config_test.go`

**Interfaces:**
- Produces: `Config.Targets []Target` (each `Target{Adapter}` with `Vault`/`Collateral` resolved later); `parseConfig(yaml.Node) (*Config, error)` requires ≥1 non-zero adapter; `Config.Target` is replaced by `Config.Targets`. **`Config.APIKeyEnv` stays for now** (removed in Task 3).

- [ ] **Step 1: Write the failing test** — append to `config_test.go`:

```go
func TestParseConfig_AdaptersList(t *testing.T) {
	cfg, err := parse(t, minimalConfig+"adapters:\n  - \"0x0000000000000000000000000000000000000042\"\n  - \"0x0000000000000000000000000000000000000043\"\n")
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.Targets) != 2 ||
		cfg.Targets[0].Adapter != common.HexToAddress("0x0000000000000000000000000000000000000042") ||
		cfg.Targets[1].Adapter != common.HexToAddress("0x0000000000000000000000000000000000000043") {
		t.Fatalf("targets = %+v", cfg.Targets)
	}
}

func TestParseConfig_RejectsEmptyAndZeroAdapters(t *testing.T) {
	if _, err := parse(t, minimalConfig); err == nil {
		t.Fatal("expected an error when no adapters are configured")
	}
	if _, err := parse(t, minimalConfig+"adapters:\n  - \"0x0000000000000000000000000000000000000000\"\n"); err == nil {
		t.Fatal("expected an error for a zero adapter address")
	}
}
```

Check `config_test.go` for the existing `parse` helper and `minimalConfig` const; reuse them. `minimalConfig` must be reduced to just `apiBaseUrl` (no `adapter`).

- [ ] **Step 2: Run to verify it fails**

Run: `GOTOOLCHAIN=go1.26.4 go test -run TestParseConfig_Adapters ./internal/solvers/bridgefacilitator/`
Expected: FAIL (compile error: `cfg.Targets` undefined).

- [ ] **Step 3: Edit `config.go`** —
  - `rawConfig`: replace `Adapter string \`yaml:"adapter"\`` with `Adapters []string \`yaml:"adapters"\``. **Keep** `APIKeyEnv` (removed in Task 3).
  - `Config`: replace `Target Target` with `Targets []Target`. **Keep** `APIKeyEnv`.
  - Replace `parseTarget` with:

```go
func parseTargets(raw rawConfig) ([]Target, error) {
	if len(raw.Adapters) == 0 {
		return nil, errors.New("at least one adapters entry is required")
	}
	targets := make([]Target, 0, len(raw.Adapters))
	for i, a := range raw.Adapters {
		adapter, err := parseNonZeroAddress(a, "adapters["+strconv.Itoa(i)+"]")
		if err != nil {
			return nil, err
		}
		targets = append(targets, Target{Adapter: adapter})
	}
	return targets, nil
}
```

  - In `parseConfig`: replace `target, err := parseTarget(raw)` with `targets, err := parseTargets(raw)` and set `Targets: targets` (keep `APIKeyEnv: raw.APIKeyEnv`). Add `"strconv"` import.

- [ ] **Step 4: Run to verify it passes**

Run: `GOTOOLCHAIN=go1.26.4 go test -run TestParseConfig ./internal/solvers/bridgefacilitator/`
Expected: PASS (other tests/call sites referencing `cfg.Target` fail to compile — fix them to `cfg.Targets[0]`; the package must build. `APIKeyEnv` is untouched here, so the API-client/onboarding wiring still compiles.)

- [ ] **Step 5: Commit**

```bash
git add internal/solvers/bridgefacilitator/config.go internal/solvers/bridgefacilitator/config_test.go internal/solvers/bridgefacilitator/solver.go
git commit -m "feat(3f): config takes an adapters list"
```

---

## Task 2: `GetOffers` EIP-712 digest (scaffold + golden)

**Files:**
- Modify: `internal/solvers/bridgefacilitator/eip712.go`
- Test: `internal/solvers/bridgefacilitator/eip712_test.go`

**Interfaces:**
- Produces: `GetOffersDigest(maker common.Address, deadline *big.Int) common.Hash`.

- [ ] **Step 1: Write the failing golden test** — append to `eip712_test.go`:

```go
func TestGetOffersDigest_Golden(t *testing.T) {
	maker := common.HexToAddress("0x0000000000000000000000000000000000000042")
	got := GetOffersDigest(maker, big.NewInt(4102444800)).Hex()
	// GOLDEN: recompute once with the apitypes cross-check (Step 3a) and paste the value here.
	want := "0x0000000000000000000000000000000000000000000000000000000000000000"
	if got != want {
		t.Fatalf("digest = %s, want %s", got, want)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `GOTOOLCHAIN=go1.26.4 go test -run TestGetOffersDigest ./internal/solvers/bridgefacilitator/`
Expected: FAIL (`GetOffersDigest` undefined).

- [ ] **Step 3: Implement** — append to `eip712.go` (modeled on `APIKeyDigest`):

```go
// getOffersTypeHash is the EIP-712 type the maker signs to list its offers via the Authorization
// header. SCAFFOLD: the exact field set is verified against the live 3F API in
// TestGetOffersDigest_MatchesLiveAcceptedSignature; adjust the type string if the API rejects it.
var getOffersTypeHash = crypto.Keccak256Hash([]byte("GetOffers(address maker,uint256 deadline)"))

// GetOffersDigest computes the EIP-712 digest signed for an authenticated GET /v1/offer (maker=adapter).
// Same grunt-api domain as APIKeyDigest (name/version/chainId=1, no verifyingContract).
func GetOffersDigest(maker common.Address, deadline *big.Int) common.Hash {
	ds := crypto.Keccak256Hash(
		apiKeyDomainTypeHash.Bytes(),
		crypto.Keccak256([]byte(apiKeyDomainName)),
		crypto.Keccak256([]byte(apiKeyDomainVersion)),
		word(big.NewInt(apiKeyDomainChainID).Bytes()),
	)
	sh := crypto.Keccak256Hash(getOffersTypeHash.Bytes(), word(maker.Bytes()), word(deadline.Bytes()))
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, ds.Bytes(), sh.Bytes())
}
```

- [ ] **Step 3a: Pin the golden** — add an apitypes cross-check (mirror `TestOfferDigest_MatchesApitypes`): build the same digest via `signer/core/apitypes`, assert it equals `GetOffersDigest(...)`, and copy the value into `want` in Step 1's test. Also add a **live-API parity** test guarded behind the same env flag the existing `TestAPIKeyDigest_MatchesLiveAcceptedSignature` uses (a correctly-formed sig is accepted; an unauthorized maker returns 403, not a signature error) — this is how the scaffolded type string is verified.

- [ ] **Step 4: Run to verify it passes**

Run: `GOTOOLCHAIN=go1.26.4 go test -run TestGetOffersDigest ./internal/solvers/bridgefacilitator/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/solvers/bridgefacilitator/eip712.go internal/solvers/bridgefacilitator/eip712_test.go
git commit -m "feat(3f): add GetOffers EIP-712 digest for signed offer listing"
```

---

## Task 3: API client — signed `listOffers(adapter)`, drop key + offer-address

**Files:**
- Modify: `internal/solvers/bridgefacilitator/apiclient.go`, `internal/solvers/bridgefacilitator/config.go` (remove `APIKeyEnv` now — deferred from Task 1), `internal/solvers/bridgefacilitator/solver.go` (factory: drop the `os.Getenv(cfg.APIKeyEnv)` wiring into `newAPIClient`)
- Test: `internal/solvers/bridgefacilitator/liveauth_test.go` (or a new httptest in `apiclient_test.go`)

**Interfaces:**
- Consumes: `GetOffersDigest` (Task 2); `signer.Signer.SignHash` (65-byte `[R‖S‖V]`).
- Produces: `(*apiClient).listOffers(ctx, adapter common.Address) ([]threef.OfferDto, error)`; `newAPIClient` no longer takes/needs an API key or facilitator address.

- [ ] **Step 1: Write the failing test** — httptest asserting the request carries `maker=<adapter>`, a `deadline`, and an `Authorization: Bearer 0x...` header, and NO `x-api-key`:

```go
func TestAPIClient_ListOffers_SignedPerAdapter(t *testing.T) {
	var gotMaker, gotAuth, gotKey, gotDeadline string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMaker = r.URL.Query().Get("maker")
		gotDeadline = r.URL.Query().Get("deadline")
		gotAuth = r.Header.Get("Authorization")
		gotKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	adapter := common.HexToAddress("0x0000000000000000000000000000000000000042")
	ac := newAPIClient(srv.URL, fakeSigner{}, 5*time.Second, logr.Discard())
	if _, err := ac.listOffers(context.Background(), adapter); err != nil {
		t.Fatalf("listOffers: %v", err)
	}
	if gotMaker != lowerAddr(adapter) || gotDeadline == "" ||
		!strings.HasPrefix(gotAuth, "Bearer 0x") || gotKey != "" {
		t.Fatalf("maker=%q deadline=%q auth=%q key=%q", gotMaker, gotDeadline, gotAuth, gotKey)
	}
}
```

Add a `fakeSigner` test double (`Address()`, `SignHash([]byte)→65 bytes`, `SignTx` unused) if one isn't already in the package's test helpers — check `eip712_test.go`/existing tests first.

- [ ] **Step 2: Run to verify it fails**

Run: `GOTOOLCHAIN=go1.26.4 go test -run TestAPIClient_ListOffers ./internal/solvers/bridgefacilitator/`
Expected: FAIL (signature mismatch / `newAPIClient` arity).

- [ ] **Step 3: Implement** —
  - **Remove `APIKeyEnv`** (deferred from Task 1): delete it from `rawConfig`, `Config`, and the `parseConfig` literal in `config.go`; in `solver.go`'s factory, drop the `os.Getenv(cfg.APIKeyEnv)` read and stop passing a key into `newAPIClient`.
  - In `apiclient.go`: **delete** `fallbackKey`, `apiKey`, `lastGenerate`, `facilitator` from the `apiClient` struct; delete `ensureKey`, `refreshKey`, `generate`, `withAuth`, `offerAddress`, `setOfferAddress`, `keyRegenCooldown`. Keep `sgnr signer.Signer`, `log`, the generated client `c`.
  - Update `newAPIClient(baseURL string, sgnr signer.Signer, timeout time.Duration, log logr.Logger) *apiClient`.
  - Rewrite `listOffers` to sign + use the `Authorization`/`deadline` builder params (confirm the generated builder method names on `OfferControllerGetV1` — the spec has `Authorization` + `deadline` + `maker`, so the builder should expose `.Authorization(...)`, `.Deadline(...)`, `.Maker(...)`):

```go
const getOffersDeadlineWindow = 5 * time.Minute

func (ac *apiClient) listOffers(ctx context.Context, adapter common.Address) ([]threef.OfferDto, error) {
	deadline := big.NewInt(time.Now().Add(getOffersDeadlineWindow).Unix())
	sig, err := ac.sgnr.SignHash(GetOffersDigest(adapter, deadline))
	if err != nil {
		return nil, errors.Errorf("3f api: sign GetOffers: %w", err)
	}
	o, httpResp, e := ac.c.OfferAPI.OfferControllerGetV1(ctx).
		Maker(lowerAddr(adapter)).
		Deadline(deadline.String()).
		Authorization("Bearer 0x" + common.Bytes2Hex(sig)).
		Execute()
	closeResp(httpResp)
	if e != nil {
		_, err := handleApiError("3f api: list offers", httpResp, e)
		return nil, errors.Errorf("3f api: list offers: %w", err)
	}
	return o, nil
}
```

  - `createOffer` already has no `x-api-key` in scope after the key removal — confirm its builder chain doesn't call `.XApiKey(...)`.

- [ ] **Step 4: Run to verify it passes**

Run: `GOTOOLCHAIN=go1.26.4 go test -run TestAPIClient ./internal/solvers/bridgefacilitator/`
Expected: PASS. The package will not fully build until `solver.go` stops calling the deleted onboarding funcs — fix those call sites in Task 4/8 (or temporarily stub `onboard` to a no-op to keep the build green between commits; remove it in Task 8).

- [ ] **Step 5: Commit**

```bash
git add internal/solvers/bridgefacilitator/apiclient.go internal/solvers/bridgefacilitator/*_test.go
git commit -m "feat(3f): list offers per-adapter via signed Authorization header"
```

---

## Task 4: Startup — resolve all adapters + verify EIP-1271 signer on-chain

**Files:**
- Modify: `internal/solvers/bridgefacilitator/solver.go`, `internal/solvers/bridgefacilitator/chainreader.go`
- Test: `internal/solvers/bridgefacilitator/chainreader_test.go`

**Interfaces:**
- Consumes: the adapter binding's `vault()`, vault `asset()`, and the **authorized-signer getter** (confirm name in `api/bindings/3f/adapter`).
- Produces: `(*reader).resolveTargets(ctx, []Target) ([]Target, error)` (fills `Vault`/`Collateral`); `(*reader).authorizedSigner(ctx, adapter) (common.Address, error)`; `Solver` startup filters `Targets` to those whose `authorizedSigner == deps.Signer.Address()`, warning on the rest.

- [ ] **Step 1: Write the failing test** — table test for `authorizedSigner` over a Multicall3 fake backend (mirror existing `chainreader_test.go` setup): given an adapter whose on-chain signer == X, `authorizedSigner` returns X.

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement** —
  - `chainreader.go`: add `authorizedSigner(ctx, adapter)` packing the binding's signer-getter call through `chain.Multicall` (single call) and decoding via the generated `Unpack`. Generalize the existing single-adapter vault/collateral resolution into `resolveTargets(ctx, targets)` (one Multicall for all adapters' `vault()`, then one for all `vault.asset()`).
  - `solver.go`: replace `resolveTarget` with a startup block that calls `resolveTargets`, then `authorizedSigner` per adapter; drop any adapter where the signer ≠ `deps.Signer.Address()` with `log.Info("skipping adapter: solver is not its EIP-1271 signer", "adapter", a, "want", ourAddr, "got", onchain)`. If zero remain → return a startup error.

- [ ] **Step 4: Run to verify it passes.**

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(3f): resolve all adapters at startup and verify EIP-1271 signer on-chain"
```

---

## Task 5: Best-adapter selection (most fundable)

**Files:**
- Create: `internal/solvers/bridgefacilitator/selection.go`
- Test: `internal/solvers/bridgefacilitator/selection_test.go`

**Interfaces:**
- Consumes: `sizeOffer(...)` (`sizer.go`) and the per-adapter `exposureState` (`chainreader.go`).
- Produces: `selectBestAdapter(candidates []adapterSizing) (adapterSizing, bool)` where `adapterSizing` pairs a `Target` with its `sizeOffer` result for the auction; returns the max-sized candidate (`false` if none size > 0).

- [ ] **Step 1: Write the failing test** — three candidates with sized amounts 100, 300, 200 → returns the 300 one; all-zero → `ok == false`; tie → deterministic (first by config order).

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement** `selectBestAdapter` — iterate, track the max `sized` (`*big.Int`), break ties by lowest index (config order). Pure function, no I/O.

- [ ] **Step 4: Run to verify it passes.**

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(3f): select the most-fundable adapter per auction"
```

---

## Task 6: Multi-adapter offer loop + per-(adapter,auction) dedup

**Files:**
- Modify: `internal/solvers/bridgefacilitator/solver.go`, `internal/solvers/bridgefacilitator/offercache.go`
- Test: `internal/solvers/bridgefacilitator/offercache_test.go`, `solver_test.go`

**Interfaces:**
- Consumes: `selectBestAdapter` (Task 5), `listOffers(ctx, adapter)` (Task 3), `buildSignedOffer(..., maker=adapter, ...)` (unchanged), `createOffer` (unchanged).
- Produces: `(*offerTracker)` keyed by `(adapter, auctionID)`; `discoverAndOffer` that, per open auction, reads each candidate adapter's exposure, sizes, selects best, signs `maker=best.Adapter`, submits, and records dedup under `(best.Adapter, auctionID)`.

- [ ] **Step 1: Write the failing test** — `offercache_test.go`: an offer recorded for `(adapterA, auction1)` does NOT suppress an offer for `(adapterB, auction1)`, and DOES suppress a re-offer for `(adapterA, auction1)` until expiry.

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement** —
  - `offercache.go`: change the `expiry` map key from `auctionID` to a `struct{ adapter common.Address; auction string }` (or `adapter.Hex()+"|"+auctionID`). Update `record`/`isLive`/rebuild signatures to take the adapter.
  - `solver.go`: rewrite `discoverAndOffer`/`offerForTarget` into: list auctions once; for each open auction matching *any* candidate's `Collateral`, gather `adapterSizing` for the matching candidates (read each adapter's exposure — reuse the existing multicall per adapter), `selectBestAdapter`, skip if none or below `minRequestYieldBps`, `buildSignedOffer(maker=best.Adapter)`, `createOffer`, record dedup. Rebuild the dedup cache at startup by calling `listOffers` **per adapter** (Task 3) and recording `(adapter, auctionID)` for each live offer.

- [ ] **Step 4: Run to verify it passes** (`solver_test.go` with fakes for the API + chain reader).

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(3f): per-auction adapter selection, offers keyed by (adapter, auction)"
```

---

## Task 7: Multi-adapter redeem + reconcile

**Files:**
- Modify: `internal/solvers/bridgefacilitator/solver.go`, `internal/solvers/bridgefacilitator/redeemer.go`

**Interfaces:**
- Produces: `redeemAll` / `reconcile` iterate over all resolved `Targets` (each adapter's `activeRequests`→`canWithdraw`→`redeem`; each adapter's health snapshot).

- [ ] **Step 1: Write the failing test** — `redeemer` with two adapters, each with one ready request, packs two `redeem` calldatas (one per adapter) handed to the fake txmanager.

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Implement** — wrap the existing single-adapter `redeemAll`/`reconcile` bodies in a `for _, tgt := range s.cfg.Targets` loop; the per-adapter reads/packing are unchanged. The redeem batch cap (`RedeemBatchSize`) applies per adapter per tick.

- [ ] **Step 4: Run to verify it passes.**

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(3f): redeem and reconcile across all configured adapters"
```

---

## Task 8: Remove onboarding; config example + docs

**Files:**
- Modify: `internal/solvers/bridgefacilitator/solver.go` (delete `onboard`, `ensureOfferAddress`), `config/3f.sepolia.example.yaml`, `docs/3F-PLAN.md`, `README.md`

**Interfaces:** none new — pure removal + docs.

- [ ] **Step 1: Delete** `onboard` and `ensureOfferAddress` from `solver.go` and their call in `Run`. Replace the startup `onboard(ctx)` call with the Task-4 resolve+verify block and a `rebuildOfferCache` that lists per adapter (Task 6).
- [ ] **Step 2: Update** `config/3f.sepolia.example.yaml` — replace `adapter:`/`apiKeyEnv:` with an `adapters:` list; note onboarding (deploy + 3F register + set EIP-1271 signer) is the vault creator's job.
- [ ] **Step 3: Sync** `docs/3F-PLAN.md` §8 — mark Phase 5 items done; `README.md` 3F section — adapters list, no API key/offer-address.
- [ ] **Step 4: Run the full gate**

Run: `GOTOOLCHAIN=go1.26.4 golangci-lint run --fix && go build ./... && go test -race -cover ./... && golangci-lint run`
Expected: all green, `bridgefacilitator` coverage not regressed.

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(3f): drop API-key/offer-address onboarding; update config + docs"
```

---

## Self-review notes (gaps to confirm during implementation)

- **`GetOffers` type string + `Authorization` format** — scaffolded as `GetOffers(address maker,uint256 deadline)` + `Bearer 0x<65-byte sig>`. Verify against the live 3F dev API (Task 2 Step 3a). If the API expects a different field set (e.g. includes `chainId`) or a non-`Bearer` scheme, adjust the type string / header in Tasks 2-3 only.
- **Generated builder method names** (`OfferControllerGetV1(...).Authorization/.Deadline/.Maker`) — confirm in `api/threef`; if the spec param names differ, regenerate is not needed (they're already in the vendored spec), just use the actual method names.
- **Adapter authorized-signer getter** — confirm the exact view name in `api/bindings/3f/adapter` (Task 4); if the adapter exposes only `isValidSignature`, verify by signing a probe digest and checking the magic value instead of reading a getter.
- **No aggregation** — one adapter per offer is enforced by `selectBestAdapter` returning a single candidate; do not sum across adapters.
