# Per-Leg SWAP Authorization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make RFQ `BUILD` emit solver-signed calldata for direct legs and the exact resolved signed-discount calldata for discount legs while preserving confirmation order, floors, capacity checks, and idempotency.

**Architecture:** Keep authorization selection local to the existing RFQ swap service and derive it from persisted `FillLeg.DiscountID`. Direct legs continue through the EIP-712 signer/domain path; discount legs resolve the shared `discounts.Provider`, validate the fresh payload against the exact persisted route and build deadline, and pack the generated LiquidLane `swap(DiscountSwap,bytes,address,uint256)` binding. The response and store remain unchanged and cache the complete opaque call list.

**Tech Stack:** Go 1.26.5, Huma v2, go-ethereum ABI/crypto, generated abigen v2 LiquidLane bindings, the shared `internal/liquidlane/discounts` package, and standard-library synchronization.

## Global Constraints

- Authorization is selected per persisted leg: `DiscountID == nil` means direct `SignedSwap`; non-nil means resolved `DiscountSwap`.
- Keep the authenticated v2 `DISCOVERY`/`CONFIRM`/`BUILD` wire schema unchanged.
- Preserve the exact confirmed adapter, route, input split, output token, liquidity domain, call order, per-leg floor, and aggregate floor.
- Only direct legs require framework-signer adapter authorization and an EIP-712 domain.
- Direct nonce derivation uses the leg's index in the complete mixed plan.
- Both resolved discount deadlines must be strictly later than the selected BUILD deadline.
- Discount calldata must contain the exact resolved terms and signatures, configured Router recipient, and persisted input amount.
- Check the nonce carried by every resolved discount for prior invalidation.
- A failed mixed build returns no partial call list; successful and concurrent retries return the byte-identical cached payload without re-resolving or re-signing.
- Provider transport failures and malformed resolved payloads map to `502`; stale identity, route, deadline, minimum-discount, output-floor, capacity, or nonce state maps to `409`.
- Accept the existing discount ABI's bearer-authorization limitation; do not add selector decoding or new cryptographic claims.
- Do not change the backend, Router, generated bindings, public response type, legacy order fill path, quote selection, or solver selection.

---

### Task 1: Wire the Discount Provider and Partition Direct Authorization

**Files:**
- Modify: `internal/solvers/rfq/swap.go`
- Modify: `internal/solvers/rfq/solver.go`
- Test: `internal/solvers/rfq/swap_test.go`
- Test: `internal/solvers/rfq/solver_test.go`

**Interfaces:**
- Consumes: `discounts.Provider` from `internal/liquidlane/discounts` and persisted `fillPlan.Legs[].DiscountID`.
- Produces: `swapService.discounts discounts.Provider` and `directPlanAdapters(plan *fillPlan) []common.Address` for CONFIRM and BUILD authorization.

- [ ] **Step 1: Write failing tests for direct-only authorization and provider wiring**

Add `TestDirectPlanAdaptersExcludesDiscountLegs` with one direct and one discount leg. Assert the result contains only the direct adapter:

```go
got := directPlanAdapters(&fillPlan{Legs: []fillLeg{
	{Adapter: directAdapter},
	{Adapter: discountAdapter, DiscountID: &discountID},
}})
if len(got) != 1 || got[0] != directAdapter {
	t.Fatalf("directPlanAdapters = %v, want [%s]", got, directAdapter.Hex())
}
```

Extend `TestFactoryWiresSwapOnlyWhenEnabledAndSharesBackend` to assert the swap service receives the exact shared discounts client:

```go
if swaps.discounts != backend.discounts {
	t.Fatal("swap service does not share the backend discount provider")
}
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run 'TestFactoryWiresSwapOnlyWhenEnabledAndSharesBackend|TestDirectPlanAdaptersExcludesDiscountLegs' -count=1
```

Expected: build failure because `swapService` has no discount provider and `directPlanAdapters` does not exist.

- [ ] **Step 3: Add provider wiring and direct-adapter selection**

Add the shared provider to `swapService`:

```go
discounts discounts.Provider
```

Wire the existing backend-owned client in `buildServicesWithSwap`:

```go
discounts: backend.discounts,
```

Replace the all-leg helper with:

```go
func directPlanAdapters(plan *fillPlan) []common.Address {
	if plan == nil {
		return nil
	}
	out := make([]common.Address, 0, len(plan.Legs))
	for _, leg := range plan.Legs {
		if leg.DiscountID == nil {
			out = append(out, leg.Adapter)
		}
	}
	return out
}
```

Use `directPlanAdapters` in CONFIRM and BUILD. Do not alter startup validation of configured adapters.

- [ ] **Step 4: Run the focused tests and verify GREEN**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run 'TestFactoryWiresSwapOnlyWhenEnabledAndSharesBackend|TestDirectPlanAdaptersExcludesDiscountLegs' -count=1
```

Expected: PASS for provider identity and direct-adapter partitioning.

- [ ] **Step 5: Commit the wiring boundary**

```bash
git add internal/solvers/rfq/swap.go internal/solvers/rfq/solver.go internal/solvers/rfq/swap_test.go internal/solvers/rfq/solver_test.go
git commit -m "refactor(rfq): select swap authorization per leg"
```

---

### Task 2: Pack Exact DiscountSwap Calldata

**Files:**
- Modify: `internal/solvers/rfq/swap_signing.go`
- Test: `internal/solvers/rfq/swap_signing_test.go`

**Interfaces:**
- Consumes: `*discounts.Signed`, Router `common.Address`, and persisted `*big.Int` amount.
- Produces: `packDiscountSwapCall(signed *discounts.Signed, recipient common.Address, amountIn *big.Int) ([]byte, error)`.

- [ ] **Step 1: Write a failing ABI-boundary test**

Add `TestPackDiscountSwapCallUsesExactResolvedPayload` with independently chosen terms and signatures. Decode `swap0` through the generated ABI and assert selector `0x8fa5c671`, every inner term, both signatures, Router, and amount:

```go
data, err := packDiscountSwapCall(signed, common.HexToAddress(testRouter), big.NewInt(100))
if err != nil {
	t.Fatalf("packDiscountSwapCall: %v", err)
}
if got := common.Bytes2Hex(data[:4]); got != "8fa5c671" {
	t.Fatalf("selector = 0x%s", got)
}
values, err := parsed.Methods["swap0"].Inputs.Unpack(data[4:])
if err != nil {
	t.Fatal(err)
}
decoded := *abi.ConvertType(values[0], new(adapter.ILiquidLaneAdapterDiscountSwap)).(*adapter.ILiquidLaneAdapterDiscountSwap)
```

The production change this catches is packing the wrong overload, replacing signed terms, losing a signature, or changing the outer recipient/amount.

- [ ] **Step 2: Run the packing test and verify RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run TestPackDiscountSwapCallUsesExactResolvedPayload -count=1
```

Expected: build failure because `packDiscountSwapCall` does not exist.

- [ ] **Step 3: Implement the generated-binding mapper**

Map the shared parsed payload directly into the generated adapter structs:

```go
func packDiscountSwapCall(
	signed *discounts.Signed,
	recipient common.Address,
	amountIn *big.Int,
) ([]byte, error) {
	if signed == nil {
		return nil, errors.New("discount swap is nil")
	}
	value := adapter.ILiquidLaneAdapterDiscountSwap{
		Discount: adapter.ILiquidLaneAdapterDiscount{
			TokenToRedeem: signed.Terms.TokenToRedeem,
			Discount: signed.Terms.Discount,
			Signer: signed.Terms.Signer,
			Protocol: signed.Terms.Protocol,
			Nonce: signed.Terms.Nonce,
			Deadline: signed.Terms.Deadline,
		},
		SignerSignature: signed.SignerSignature,
		ProtocolDeadline: signed.ProtocolDeadline,
	}
	data, err := swapAdapterBinding.TryPackSwap0(
		value, signed.ProtocolSignature, recipient, liquidlane.CloneBig(amountIn),
	)
	if err != nil {
		return nil, errors.Errorf("pack discount swap: %w", err)
	}
	return data, nil
}
```

Use only the generated binding; do not hand-pack ABI strings or edit generated code.

- [ ] **Step 4: Run signing tests and verify GREEN**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run 'TestPack(Signed|Discount)SwapCall' -count=1
```

Expected: PASS for both selectors and exact payload decoding.

- [ ] **Step 5: Commit the calldata mapper**

```bash
git add internal/solvers/rfq/swap_signing.go internal/solvers/rfq/swap_signing_test.go
git commit -m "feat(rfq): encode resolved discount swap calls"
```

---

### Task 3: Build Mixed Direct and Discount Calls Atomically

**Files:**
- Modify: `internal/solvers/rfq/swap.go`
- Test: `internal/solvers/rfq/swap_test.go`

**Interfaces:**
- Consumes: provider from Task 1, packer from Task 2, `discounts.ParseSigned`, `discounts.ValidateSigned`, and `swapStateReader.readUsedNonces`.
- Produces: one immutable `swapBuildPayload` whose calls retain the persisted order and whose per-leg authorization matches `DiscountID`.

- [ ] **Step 1: Replace the signed-fallback test with a failing exact-discount lifecycle test**

Rename `TestSwapBuildSignsDiscountSelectedLegOnPersistedAdapter` to `TestSwapBuildReturnsResolvedDiscountCall`. Give the service a fake provider that returns a complete `discounts.Resolved` with discount ID and adapter matching the selected leg, `tokenToRedeem == tokenIn`, zero discount for the existing 200-unit floor, and deadlines `1021`/`1022` for BUILD deadline `1020`.

Assert:

```go
if call.Data[:10] != "0x8fa5c671" {
	t.Fatalf("discount response = %+v", response)
}
if signer.calls != 0 || provider.resolveCalls != 1 || state.nonceReads != 1 {
	t.Fatalf("discount dependencies: signer=%d resolve=%d nonce=%d", signer.calls, provider.resolveCalls, state.nonceReads)
}
```

Decode the call and assert it contains the exact resolved signed terms, Router, and persisted amount. Retry the same BUILD and assert byte identity plus unchanged dependency counts.

- [ ] **Step 2: Add a failing mixed-plan order and nonce test**

Persist a two-leg confirmation directly in the store: discount leg at index 0 and direct leg at index 1, with distinct adapters/routes and physical quotes. Return a resolved discount for leg 0 and valid domains only for leg 1.

Assert call selectors remain `[0x8fa5c671, 0x9a4568b6]`, authorization receives only the direct adapter, and the decoded direct nonce equals:

```go
signedSwapNonce(buildID, record.ChainID, directLeg.Adapter, record.TokenIn, 1)
```

The production changes this catches are sorting/grouping calls, applying direct auth to the discount leg, or deriving the direct nonce from the filtered direct-leg index `0`.

- [ ] **Step 3: Add failing table tests for discount failure mapping**

Create `TestSwapBuildRejectsStaleOrMalformedResolvedDiscount` with literal fixtures covering:

- provider error → `502`;
- malformed address/signature/nonce payload → `502`;
- mismatched discount ID, adapter, or token → `409`;
- either signed deadline equal to or earlier than BUILD deadline → `409`;
- discount below the current adapter minimum → `409`;
- discounted current output below the persisted leg floor → `409`;
- resolved discount nonce already invalidated → `409`.

For every case assert response is nil, the exact `swapServiceError.status`, signer call count is zero, and the build cache remains empty/retryable.

- [ ] **Step 4: Add a failing concurrent retry test for discount resolution**

Block the fake provider's first `Resolve` call, start two BUILD requests with the same build ID and different transport request IDs, release the provider, and assert both responses carry identical calldata while `resolveCalls == 1`, `fillReads == 1`, and `nonceReads == 1`.

- [ ] **Step 5: Run lifecycle tests and verify RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run 'TestSwapBuild(ReturnsResolvedDiscountCall|PreservesMixedAuthorizationOrder|RejectsStaleOrMalformedResolvedDiscount|ConcurrentDiscountRetries)' -count=1
```

Expected: FAIL because BUILD still converts every leg to `SignedSwap`, requires domains for every adapter, and never resolves discounts.

- [ ] **Step 6: Resolve and validate each discount leg before nonce checks**

During the first uncached BUILD, allocate one per-leg resolved result:

```go
type swapBuildLeg struct {
	amountOut *big.Int
	nonce     *big.Int
	discount  *discounts.Signed
}
```

For a direct leg, retain the current fresh-output-floor check and deterministic nonce using the full persisted index. For a discount leg:

1. Call `s.discounts.Resolve(ctx, leg.DiscountID.Hex())`; map call errors to `502`.
2. Call `discounts.ParseSigned`; map malformed payload errors to `502`.
3. Call `discounts.ValidateSigned` with the exact ID, adapter, input token, output token, input amount, persisted leg floor, fresh physical quote, and `request.Deadline`; map validation errors to `409`.
4. Store its calculated output, signed nonce, and parsed payload.

Use the calculated outputs for shared-capacity and aggregate-floor checks. Build one nonce check per persisted leg: deterministic for direct legs and resolved for discount legs. Map used discount nonces to `409` with a discount-specific sanitized message.

- [ ] **Step 7: Encode calls in persisted order**

For each `record.Plan.Legs[i]`:

```go
if leg.DiscountID != nil {
	data, packErr = packDiscountSwapCall(builtLegs[i].discount, s.router, leg.AmountIn)
} else {
	domain, exists := domains[leg.Adapter]
	if !exists {
		return nil, swapError(http.StatusConflict, "confirmed swap adapter domain disappeared", nil)
	}
	value := adapter.ILiquidLaneAdapterSignedSwap{
		Recipient: s.router, TokenIn: record.TokenIn, AmountIn: liquidlane.CloneBig(leg.AmountIn),
		AmountOut: liquidlane.CloneBig(builtLegs[i].amountOut), Caller: s.router,
		Signer: s.signer.Address(), Nonce: builtLegs[i].nonce,
		Deadline: big.NewInt(request.Deadline.Unix()),
	}
	data, packErr = packSignedSwapCall(s.signer, domain, value)
}
```

Map either packing failure to `502`, then build the existing opaque response entry. Do not cache or return until every leg, nonce, capacity, and aggregate floor has passed and the deadline is rechecked.

- [ ] **Step 8: Run lifecycle tests and verify GREEN**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run 'TestSwap|TestPack' -count=1
```

Expected: PASS with direct-only behavior unchanged and discount/mixed behavior covered.

- [ ] **Step 9: Run the RFQ package under the race detector**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test -race ./internal/solvers/rfq -count=1
```

Expected: PASS with no races, especially across the build lease and fake provider synchronization.

- [ ] **Step 10: Commit the per-leg build behavior**

```bash
git add internal/solvers/rfq/swap.go internal/solvers/rfq/swap_test.go
git commit -m "feat(rfq): preserve per-leg swap authorization"
```

---

### Task 4: Synchronize Operator and Maintainer Documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/RFQ-PLAN.md`
- Modify: `docs/superpowers/specs/2026-08-03-user-directed-swap-calldata-design.md`
- Modify: `docs/superpowers/plans/2026-08-03-user-directed-swap-calldata.md`
- Modify: `config/rfq.example.yaml`
- Test: `internal/solvers/rfq/docs_test.go`

**Interfaces:**
- Consumes: final per-leg behavior from Task 3.
- Produces: consistent operator guidance and historical design records that no longer claim discount-selected legs are converted to `SignedSwap`.

- [ ] **Step 1: Update the documentation contract test first**

Replace the stale phrase assertion `"Private discount calldata is never"` with assertions that the README explains both selectors and per-leg semantics:

```go
"0x9a4568b6", "0x8fa5c671", "resolved signed discount", "per leg"
```

- [ ] **Step 2: Run the docs test and verify RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run TestRFQDocumentationContract -count=1
```

Expected: FAIL because the current README describes signed-only output.

- [ ] **Step 3: Update all user-directed swap descriptions**

Document that direct legs use selector `0x9a4568b6`, discount legs resolve the persisted discount ID and use selector `0x8fa5c671`, mixed responses preserve order, and successful retries reuse cached opaque calldata. Record the accepted bearer-authorization limitation without claiming the Router or backend cryptographically binds the discount recipient/amount.

In the 2026-08-03 design and implementation plan, label signed-only statements as superseded by `2026-08-04-per-leg-swap-authorization-design.md` rather than erasing the historical decision. Update `config/rfq.example.yaml` comments to match current runtime behavior.

- [ ] **Step 4: Run docs and RFQ tests and verify GREEN**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit documentation synchronization**

```bash
git add README.md docs/RFQ-PLAN.md docs/superpowers/specs/2026-08-03-user-directed-swap-calldata-design.md docs/superpowers/plans/2026-08-03-user-directed-swap-calldata.md config/rfq.example.yaml internal/solvers/rfq/docs_test.go
git commit -m "docs(rfq): describe per-leg swap calldata"
```

---

### Task 5: Full Verification and PR Update

**Files:**
- Verify: all files changed by Tasks 1–4.

**Interfaces:**
- Consumes: the complete feature branch.
- Produces: a clean, pushed PR branch with current verification evidence.

- [ ] **Step 1: Format and autofix through the repository gate**

Run:

```bash
GOTOOLCHAIN=go1.26.5 golangci-lint run --fix
```

Expected: exit 0. Review any autofix and keep only changes belonging to this feature.

- [ ] **Step 2: Build all packages**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go build ./...
```

Expected: exit 0.

- [ ] **Step 3: Run full race and coverage tests**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test -race -cover ./...
```

Expected: exit 0 with no race report.

- [ ] **Step 4: Run the final lint gate**

Run:

```bash
GOTOOLCHAIN=go1.26.5 golangci-lint run
```

Expected: exit 0 with zero issues.

- [ ] **Step 5: Inspect the final diff and history**

Run:

```bash
git diff --check origin/stage...HEAD
git status --short --branch
git log --oneline --decorate -8
```

Expected: no whitespace errors, no uncommitted files, and only scoped per-leg implementation commits.

- [ ] **Step 6: Push and inspect PR #116**

Run:

```bash
git push origin codex/swap-calldata
gh pr view 116 --json number,title,url,baseRefName,headRefName,state,isDraft,statusCheckRollup
```

Expected: open ready PR from `codex/swap-calldata` to `stage`, containing the per-leg implementation and current local verification evidence.
