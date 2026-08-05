# User-Directed Swap Calldata Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an authenticated RFQ `POST /swap` v2 discovery/confirmation/build lifecycle that returns immutable, Router-bound LiquidLane adapter calldata without broadcasting transactions.

**Implementation status:** Complete. The signed-only authorization result documented in historical Task 9
was superseded on 2026-08-04 by `2026-08-04-per-leg-swap-authorization.md`.

**Architecture:** Add a swap service beside the existing quote and execution services. It parses one phase-tagged Huma contract, prices backend-supplied sample grids with the existing strategy, stores short-lived discovery and immutable confirmation records, then rebuilds only the persisted allocation. Direct legs become solver-signed `SignedSwap` calls; discount-selected legs resolve and return their exact signed `DiscountSwap` payload. A dedicated chain-read seam validates Router bytecode, direct-adapter EIP-712 domains, signer authorization, and nonce availability; the existing `/quote`, order poller, Executor fill path, and transaction manager remain untouched.

**Tech Stack:** Go 1.26.5, Huma v2, go-ethereum ABI/crypto/apitypes, generated abigen v2 LiquidLane bindings, existing LiquidLane strategy/discount packages, Prometheus, and standard-library concurrency primitives.

## Global Constraints

- Implement only same-chain, exact-input, single-output-token swaps with distinct nonzero ERC-20 token addresses.
- The wire protocol is exactly `"v2"`; the only phases are `DISCOVERY`, `CONFIRM`, and `BUILD` on authenticated `POST /swap`.
- Preserve every JSON field name from the approved spec, including `requestId`, `discoveryRequestId`, `quoteId`, `solverQuoteId`, `buildId`, `sampleAmountsIn`, `minAmountOut`, `liquidityDomains`, and `validUntil`.
- Encode every amount as a base-10 `uint256` string; encode timestamps as Unix seconds that fit `uint48`.
- Normalize every response address, capacity domain, and calldata hex string to lowercase.
- `liquidityDomain` is the canonical `liquidlane.CapacityID`, not an adapter address; backend dedupe keys are `(chainId, CapacityID)`.
- `CONFIRM` persists the exact ordered candidate allocation; `BUILD` may improve a leg's output but must not change its candidate, adapter, input split, domain, token, order, or confirmed per-leg floor.
- Treat CONFIRM `deadline` as a requested maximum and cap `validUntil` at the earliest local validity; BUILD chooses one exact unexpired deadline at or before that cap.
- Set `SignedSwap.recipient` and `SignedSwap.caller` to the configured Router, use the existing framework signer, and set its deadline to the chosen BUILD deadline.
- Derive direct nonces exactly from `buildId`, chain, adapter, input token, and zero-based persisted call index; never substitute another nonce after a collision or use.
- Select authorization per persisted leg: direct legs emit fresh `SignedSwap` calldata; discount-selected
  legs resolve the exact persisted discount ID and emit `DiscountSwap` calldata without substitution.
- Exclude transport-only `requestId` from BUILD idempotency, cache immutable payload only, and rebuild the response envelope for each retry.
- Reject swap requests or plans that could produce more than 64 calls.
- `DISCOVERY` creates no authorization or liquidity reservation; confirmation records are bounded in-memory state and become unknown after restart.
- The user-directed path must never call `TxManager`, Executor, Reactor, or any transaction-send API.
- Cross-solver capacity-domain rejection, Router transaction assembly, token transfers, aggregate output
  enforcement, submission, and monitoring remain RFQ-backend responsibilities outside this solver repository.
- Do not edit generated files under `api/bindings`; use `TryPackSwap1` for direct legs and `TryPackSwap0`
  for discount legs from the existing adapter binding.
- Keep `swapEnabled` false by default. Disabled deployments must preserve existing `/quote` and order-execution behavior.
- Follow repository conventions: absolute `internal/...` imports between packages, same-package relative files, `go-errors/errors` wrapping, `logr` logging, strict YAML decode, no raw secrets/calldata/signatures in errors or metric labels.
- Use Go 1.26.5 for every build and test gate.

---

## File map

### Create

- `internal/solvers/rfq/swap_apitypes.go` — exact v2 phase-tagged request/response types and phase-aware parsing.
- `internal/solvers/rfq/swap_apitypes_test.go` — wire-name, normalization, and phase-validation contract tests.
- `internal/solvers/rfq/swap_store.go` — bounded discovery/confirmation/build-idempotency state with deep copies.
- `internal/solvers/rfq/swap_store_test.go` — expiry, capacity, mutation isolation, and concurrent-build tests.
- `internal/solvers/rfq/swap_signing.go` — EIP-712 domain model, `SignedSwap` digest, deterministic nonce, and direct/discount adapter calldata encoders.
- `internal/solvers/rfq/swap_signing_test.go` — independent apitypes cross-checks, golden nonce/selector tests, and signature recovery.
- `internal/solvers/rfq/swap_chainreader.go` — Router-code, EIP-5267 domain, signer-authorization, and used-nonce reads.
- `internal/solvers/rfq/swap_chainreader_test.go` — multicall packing/unpacking and fail-closed validation tests.
- `internal/solvers/rfq/swap.go` — discovery, confirmation, exact-allocation refresh, and per-leg build orchestration.
- `internal/solvers/rfq/swap_test.go` — service-level lifecycle, allocation immutability, mixed direct/discount selection, and no-broadcast tests.
- `internal/solvers/rfq/docs_test.go` — source-level contract for swap operator documentation.

### Modify

- `go.mod`, `go.sum` — promote the already-resolved `github.com/google/uuid v1.6.0` module to a direct dependency.
- `internal/solvers/rfq/config.go` — add `swapEnabled`, configured `router`, and `swapQuoteTtlMs`.
- `internal/solvers/rfq/config_test.go` — pin disabled defaults and enabled validation.
- `internal/solvers/rfq/strategies/types/types.go` — retain candidate, route, capacity, and validity identity on validated fill legs.
- `internal/solvers/rfq/strategies/fillplan.go` — copy those identities from the selected candidate.
- `internal/solvers/rfq/strategies/fillplan_test.go` — prove the validated plan retains immutable route metadata.
- `internal/solvers/rfq/strategies/default/liquidlane.go` — populate the same metadata in fill-time default plans.
- `internal/solvers/rfq/strategies/default/strategy_test.go` — pin default-plan metadata.
- `internal/solvers/rfq/server.go` — conditionally register and authenticate `POST /swap`, then map typed phase errors.
- `internal/solvers/rfq/server_test.go` — HTTP/OpenAPI phase contract, auth, status, and body-limit tests.
- `internal/solvers/rfq/metrics.go` — add bounded phase/outcome instrumentation without unbounded labels.
- `internal/solvers/rfq/metrics_test.go` — pin swap metric names and labels.
- `internal/solvers/rfq/solver.go` — wire signer/backend/store/reader into the swap service and perform startup validation before listening.
- `internal/solvers/rfq/solver_test.go` — pin enabled/disabled wiring, startup checks, and legacy execution coexistence.
- `internal/solvers/rfq/test_helpers_test.go` — reusable signer, swap request, candidate, and backend fixtures.
- `config/rfq.example.yaml` — document the disabled-by-default switch, Router, and 30-second quote TTL.
- `README.md` — describe the three private phases and Router execution responsibility.
- `docs/RFQ-PLAN.md` — record the v2 calldata lifecycle and its non-broadcasting boundary.

### Create for documentation verification

- `internal/solvers/rfq/docs_test.go` — source-level operator-documentation contract.

### Explicitly unchanged

- `api/bindings/liquidlane/adapter/LiquidLaneAdapter.go` and all other generated bindings.
- `internal/solvers/rfq/order.go` and the existing Executor calldata shape.
- `internal/txmanager/` and framework signer configuration.
- Database schemas and migrations.

---

### Task 1: Parse and validate swap configuration

**Files:**
- Modify: `internal/solvers/rfq/config.go`
- Test: `internal/solvers/rfq/config_test.go`

**Interfaces:**
- Consumes: existing `parseConfig(yaml.Node) (*Config, error)` and `parse.NonZeroAddress`.
- Produces: `Config.SwapEnabled bool`, `Config.Router common.Address`, `Config.SwapQuoteTTL time.Duration`, and `defaultSwapQuoteTTL = 30 * time.Second` for solver wiring in Task 10.

- [ ] **Step 1: Write failing default and enabled-setting tests**

Add these tests to `internal/solvers/rfq/config_test.go`:

```go
func TestParseConfigSwapDefaults(t *testing.T) {
	cfg, err := parseCfg(t, minimalConfig+oneAdapter)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.SwapEnabled {
		t.Fatal("swapEnabled = true, want false")
	}
	if cfg.Router != (common.Address{}) {
		t.Fatalf("router = %s, want zero address", cfg.Router.Hex())
	}
	if cfg.SwapQuoteTTL != 30*time.Second {
		t.Fatalf("swapQuoteTTL = %s, want 30s", cfg.SwapQuoteTTL)
	}
}

func TestParseConfigSwapEnabled(t *testing.T) {
	cfg, err := parseCfg(t, minimalConfig+oneAdapter+`
swapEnabled: true
router: "0x0000000000000000000000000000000000000055"
swapQuoteTtlMs: 45000
`)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !cfg.SwapEnabled {
		t.Fatal("swapEnabled = false, want true")
	}
	if cfg.Router != common.HexToAddress("0x0000000000000000000000000000000000000055") {
		t.Fatalf("router = %s", cfg.Router.Hex())
	}
	if cfg.SwapQuoteTTL != 45*time.Second {
		t.Fatalf("swapQuoteTTL = %s, want 45s", cfg.SwapQuoteTTL)
	}
}
```

- [ ] **Step 2: Run the focused tests and verify they fail**

Run:

```bash
go test ./internal/solvers/rfq -run 'TestParseConfigSwap(Default|Enabled)' -count=1
```

Expected: compilation fails because the three `Config` fields do not exist.

- [ ] **Step 3: Add failing validation cases**

Add this table test:

```go
func TestParseConfigSwapRejectsInvalidSettings(t *testing.T) {
	cases := map[string]string{
		"enabled without router": minimalConfig + oneAdapter + "swapEnabled: true\n",
		"enabled with zero router": minimalConfig + oneAdapter + `
swapEnabled: true
router: "0x0000000000000000000000000000000000000000"
`,
		"invalid router": minimalConfig + oneAdapter + `
swapEnabled: true
router: "not-an-address"
`,
		"negative ttl": minimalConfig + oneAdapter + "swapQuoteTtlMs: -1\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCfg(t, body); err == nil {
				t.Fatal("expected swap config error")
			}
		})
	}
}

func TestParseConfigSwapAllowsDynamicInternalAdapters(t *testing.T) {
	cfg, err := parseCfg(t, minimalConfig+`
solverMode: internal
swapEnabled: true
router: "0x0000000000000000000000000000000000000055"
`)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.Adapters) != 0 || !cfg.SwapEnabled {
		t.Fatalf("config = %+v, want enabled dynamic-adapter mode", cfg)
	}
}
```

- [ ] **Step 4: Implement the minimal configuration fields and checks**

Add these fields to the existing `rawConfig` and `Config` declarations, then add the default:

```go
// rawConfig fields
	SwapEnabled    bool   `yaml:"swapEnabled"`
	Router         string `yaml:"router"`
	SwapQuoteTTLms int   `yaml:"swapQuoteTtlMs"`

// Config fields
	SwapEnabled  bool
	Router       common.Address
	SwapQuoteTTL time.Duration

const defaultSwapQuoteTTL = 30 * time.Second
```

In `parseConfig`, initialize `SwapEnabled` and `SwapQuoteTTL`, reject a negative override, apply a positive
millisecond override, parse a supplied Router with `parse.NonZeroAddress`, and require a Router when enabled:

```go
	cfg.SwapEnabled = raw.SwapEnabled
	cfg.SwapQuoteTTL = defaultSwapQuoteTTL
	if raw.SwapQuoteTTLms < 0 {
		return nil, errors.New("swapQuoteTtlMs must be non-negative")
	}
	if raw.SwapQuoteTTLms > 0 {
		cfg.SwapQuoteTTL = time.Duration(raw.SwapQuoteTTLms) * time.Millisecond
	}
	if raw.Router != "" {
		cfg.Router, err = parse.NonZeroAddress(raw.Router, "router")
		if err != nil {
			return nil, err
		}
	}
	if cfg.SwapEnabled && cfg.Router == (common.Address{}) {
		return nil, errors.New("router is required when swapEnabled is true")
	}
```

Keep the existing external-mode adapter requirement unchanged; internal mode may enable swaps with no
static adapter list because confirmation validates backend-supplied adapters dynamically.

- [ ] **Step 5: Run configuration tests**

Run:

```bash
go test ./internal/solvers/rfq -run 'TestParseConfig' -count=1
```

Expected: PASS, including strict unknown-key coverage.

- [ ] **Step 6: Commit the configuration slice**

```bash
git add internal/solvers/rfq/config.go internal/solvers/rfq/config_test.go
git commit -m "feat(rfq): configure user-directed swaps"
```

---

### Task 2: Define and parse the exact v2 wire contract

**Files:**
- Create: `internal/solvers/rfq/swap_apitypes.go`
- Create: `internal/solvers/rfq/swap_apitypes_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: `quoteAdapter.parse`, `parseUint256`, `liquidlane.CapacityID`, and configured chain/Router values.
- Produces: `swapRequest.parse(chainID int64, configuredRouter common.Address) (*parsedSwapRequest, error)`, `swapResponse`, `swapPointResponse`, and `swapCallResponse`.

- [ ] **Step 1: Promote UUID and write failing contract tests**

Run `go get github.com/google/uuid@v1.6.0`, then create these tests:

```go
func TestSwapRequestParseDiscovery(t *testing.T)
func TestSwapRequestParseConfirm(t *testing.T)
func TestSwapRequestParseBuild(t *testing.T)
func TestSwapRequestParseRejectsPhaseMismatches(t *testing.T)
func TestSwapRequestParseRejectsInvalidUUIDAmountDeadlineAndDomain(t *testing.T)
func TestSwapResponseJSONPinsV2FieldNamesAndLowercaseValues(t *testing.T)
```

Use these canonical fixtures:

```go
const (
	testDiscoveryRequestID = "8fcc1d0d-246d-4e8e-9620-13c76857b31a"
	testConfirmRequestID   = "2ac09473-0c50-4db0-ad22-9417522f3ca2"
	testBuildRequestID     = "5e56f7c0-3840-4545-a8ca-e942ce3f3d71"
	testQuoteID            = "92b1be9d-25c1-4eca-80d1-fd1338ab57d2"
	testSolverQuoteID      = "ed972bed-60a9-499e-ab25-0d4d09b4aa5a"
	testBuildID            = "7423df2b-957b-47d5-acbc-21c3bd8a614e"
)
```

Assert discovery accepts only 1–32 strictly increasing, duplicate-free positive samples; confirm requires
`discoveryRequestId`, exact amounts, deadline, and adapters; build requires `solverQuoteId`, `buildId`,
exact amounts/deadline, domains, and configured Router while rejecting adapters and samples. All phases
require protocol `v2`, canonical UUIDs, local chain, nonzero swapper/tokens, and distinct tokens.

- [ ] **Step 2: Verify the tests fail**

```bash
go test ./internal/solvers/rfq -run 'TestSwap(Request|Response)' -count=1
```

Expected: compilation fails because the swap wire types do not exist.

- [ ] **Step 3: Create the exact wire structs**

```go
type swapPhase string

const (
	swapProtocolV2     = "v2"
	swapPhaseDiscovery swapPhase = "DISCOVERY"
	swapPhaseConfirm   swapPhase = "CONFIRM"
	swapPhaseBuild     swapPhase = "BUILD"
	maxDiscoverySamples          = 32
)

type swapRequest struct {
	Protocol string `json:"protocol"`
	Phase swapPhase `json:"phase"`
	RequestID string `json:"requestId"`
	DiscoveryRequestID *string `json:"discoveryRequestId,omitempty"`
	QuoteID string `json:"quoteId"`
	SolverQuoteID *string `json:"solverQuoteId,omitempty"`
	BuildID *string `json:"buildId,omitempty"`
	ChainID int64 `json:"chainId"`
	Swapper string `json:"swapper"`
	TokenIn string `json:"tokenIn"`
	TokenOut string `json:"tokenOut"`
	SampleAmountsIn []string `json:"sampleAmountsIn,omitempty"`
	AmountIn *string `json:"amountIn,omitempty"`
	MinAmountOut *string `json:"minAmountOut,omitempty"`
	Deadline *int64 `json:"deadline,omitempty"`
	Adapters []quoteAdapter `json:"adapters,omitempty"`
	LiquidityDomains []string `json:"liquidityDomains,omitempty"`
	Router *string `json:"router,omitempty"`
}

type swapPointResponse struct {
	AmountIn string `json:"amountIn"`
	AmountOut string `json:"amountOut"`
	LiquidityDomains []string `json:"liquidityDomains"`
}

type swapCallResponse struct {
	To string `json:"to"`
	Data string `json:"data"`
	AmountIn string `json:"amountIn"`
	AmountOut string `json:"amountOut"`
	TokenOut string `json:"tokenOut"`
	LiquidityDomain string `json:"liquidityDomain"`
	ValidUntil int64 `json:"validUntil"`
}
```

Define `swapResponse` with the spec's exact echo fields plus `Points []swapPointResponse` and
`Calls []swapCallResponse`, using `omitempty` only for fields absent in a phase. Define:

```go
type parsedSwapRequest struct {
	Protocol string
	Phase swapPhase
	RequestID uuid.UUID
	DiscoveryRequestID uuid.UUID
	QuoteID uuid.UUID
	SolverQuoteID uuid.UUID
	BuildID uuid.UUID
	ChainID int64
	Swapper common.Address
	TokenIn common.Address
	TokenOut common.Address
	SampleAmountsIn []*big.Int
	AmountIn *big.Int
	MinAmountOut *big.Int
	Deadline time.Time
	Inventory []solverInventory
	LiquidityDomains []liquidlane.CapacityID
	Router common.Address
}
```

- [ ] **Step 4: Implement phase-aware parsing**

Implement these helpers:

```go
func (r *swapRequest) parse(chainID int64, configuredRouter common.Address) (*parsedSwapRequest, error)
func parseCanonicalUUID(raw, field string) (uuid.UUID, error)
func parseSampleAmounts(raw []string) ([]*big.Int, error)
func parseCapacityDomains(raw []string, chainID int64) ([]liquidlane.CapacityID, error)
func parseSwapDeadline(raw *int64, field string) (time.Time, error)
```

`parseCapacityDomains` accepts only lowercase
`capacity:<chainId>:<20-byte vault>:<20-byte output token>`, checks the request chain, sorts the set, and
rejects input duplicates. `parseCanonicalUUID` requires `uuid.Parse(raw).String() == raw`. Parse adapters
with `quoteAdapter.parse(index, chainID, tokenIn)`. Reject every nonempty field not admitted by its phase.

- [ ] **Step 5: Run and commit**

```bash
gofmt -w internal/solvers/rfq/swap_apitypes.go internal/solvers/rfq/swap_apitypes_test.go
go test ./internal/solvers/rfq -run 'TestSwap(Request|Response)' -count=1
git add go.mod go.sum internal/solvers/rfq/swap_apitypes.go internal/solvers/rfq/swap_apitypes_test.go
git commit -m "feat(rfq): define swap v2 wire contract"
```

Expected: PASS, and UUID v1.6.0 is a direct dependency.

---

### Task 3: Preserve candidate and capacity identity in validated plans

**Files:**
- Modify: `internal/solvers/rfq/strategies/types/types.go`
- Modify: `internal/solvers/rfq/strategies/fillplan.go`
- Test: `internal/solvers/rfq/strategies/fillplan_test.go`
- Modify: `internal/solvers/rfq/strategies/default/liquidlane.go`
- Test: `internal/solvers/rfq/strategies/default/strategy_test.go`

**Interfaces:**
- Consumes: `liquidlane.QuoteCandidate`.
- Produces: each `types.FillLeg` retains `CandidateID liquidlane.CandidateID`, `Route liquidlane.Route`, and `ValidUntil time.Time`.

- [ ] **Step 1: Write failing metadata tests**

Extend both the shared fill-plan and default-strategy fill tests:

```go
leg := plan.Legs[0]
if leg.CandidateID != candidate.ID || leg.Route != candidate.Route ||
	!leg.ValidUntil.Equal(candidate.ValidUntil) {
	t.Fatalf("leg identity = %+v, want candidate %+v", leg, candidate)
}
```

Add `TestFillPlanFromQuotePreservesSharedCapacityID`: select two adapters backed by one vault/output
token and assert both legs retain the same nonempty `Route.CapacityID`.

- [ ] **Step 2: Verify the tests fail**

```bash
go test ./internal/solvers/rfq/strategies/... -run 'Test(FillPlanFromQuote|Strategy.*Fill)' -count=1
```

Expected: compilation fails on the three absent fields.

- [ ] **Step 3: Add and populate metadata**

Add before the existing `FillLeg` execution fields:

```go
CandidateID liquidlane.CandidateID
Route       liquidlane.Route
ValidUntil  time.Time
```

In `FillPlanFromQuote`, set:

```go
CandidateID: candidate.ID,
Route:       candidate.Route,
ValidUntil:  candidate.ValidUntil,
Adapter:     candidate.Route.Adapter,
```

In the default strategy's `buildFillPlan`, set the same fields from `source`. Keep `Adapter` for
legacy Executor consumers, and do not reject older hand-built execution fixtures that omit metadata.

- [ ] **Step 4: Run and commit**

```bash
gofmt -w internal/solvers/rfq/strategies/types/types.go internal/solvers/rfq/strategies/fillplan.go internal/solvers/rfq/strategies/fillplan_test.go internal/solvers/rfq/strategies/default/liquidlane.go internal/solvers/rfq/strategies/default/strategy_test.go
go test ./internal/solvers/rfq/strategies/... ./internal/solvers/rfq -count=1
git add internal/solvers/rfq/strategies/types/types.go internal/solvers/rfq/strategies/fillplan.go internal/solvers/rfq/strategies/fillplan_test.go internal/solvers/rfq/strategies/default/liquidlane.go internal/solvers/rfq/strategies/default/strategy_test.go
git commit -m "feat(rfq): retain swap allocation identity"
```

Expected: PASS with unchanged legacy fill behavior.

---

### Task 4: Add bounded discovery and confirmation state

**Files:**
- Create: `internal/solvers/rfq/swap_store.go`
- Create: `internal/solvers/rfq/swap_store_test.go`

**Interfaces:**
- Consumes: UUIDs, `fillPlan`, `CapacityID` sets, and `swapResponse`.
- Produces: `newSwapStore(now func() time.Time) *swapStore`, discovery/confirmation accessors, and per-confirmation `buildLease`.

- [ ] **Step 1: Write failing lifecycle and race tests**

Create:

```go
func TestSwapStoreDiscoveryIsDeepCopiedAndExpires(t *testing.T)
func TestSwapStoreConfirmationIsDeepCopiedAndExpires(t *testing.T)
func TestSwapStoreRejectsWhenTenThousandLiveRecordsExist(t *testing.T)
func TestSwapStoreBuildLeaseCachesIdenticalResponse(t *testing.T)
func TestSwapStoreBuildLeaseRejectsSecondBuildIDAndFingerprintDrift(t *testing.T)
func TestSwapStoreBuildLeaseSerializesConcurrentIdenticalBuilds(t *testing.T)
```

The concurrency test holds the first lease, proves a second identical acquire blocks, completes the first,
then asserts the second receives a deep-copied cache and does not execute a second build.

- [ ] **Step 2: Verify the tests fail**

```bash
go test ./internal/solvers/rfq -run 'TestSwapStore' -count=1
```

Expected: compilation fails because the store does not exist.

- [ ] **Step 3: Create exact record types**

```go
const maxSwapRecords = 10_000

type discoveryPointRecord struct {
	AmountIn  *big.Int
	AmountOut *big.Int
	Domains   []liquidlane.CapacityID
}

type discoveryRecord struct {
	RequestID uuid.UUID
	QuoteID uuid.UUID
	ChainID int64
	Swapper common.Address
	TokenIn common.Address
	TokenOut common.Address
	Points map[string]discoveryPointRecord
	ExpiresAt time.Time
}

type confirmationRecord struct {
	SolverQuoteID uuid.UUID
	DiscoveryRequestID uuid.UUID
	QuoteID uuid.UUID
	ChainID int64
	Swapper common.Address
	TokenIn common.Address
	TokenOut common.Address
	AmountIn *big.Int
	AmountOut *big.Int
	PublicDeadline time.Time
	ValidUntil time.Time
	Domains []liquidlane.CapacityID
	Plan *fillPlan

	buildMu sync.Mutex
	buildID uuid.UUID
	buildFingerprint common.Hash
	built *swapResponse
}
```

`swapStore` has one mutex, discovery and confirmation maps, and an injected clock. Deep-clone every
`big.Int`, hash pointer, plan leg, domain slice, point map, response, and calls slice at map boundaries.

- [ ] **Step 4: Implement bounded operations and build leases**

Implement:

```go
func newSwapStore(now func() time.Time) *swapStore
func (s *swapStore) putDiscovery(record discoveryRecord) error
func (s *swapStore) discovery(id uuid.UUID) (*discoveryRecord, error)
func (s *swapStore) putConfirmation(record confirmationRecord) error
func (s *swapStore) confirmation(id uuid.UUID) (*confirmationRecord, error)
func (s *swapStore) acquireBuild(id, buildID uuid.UUID, fingerprint common.Hash) (*buildLease, error)
func (s *swapStore) sweep()

func (l *buildLease) Record() *confirmationRecord
func (l *buildLease) Cached() *swapResponse
func (l *buildLease) Complete(response *swapResponse)
func (l *buildLease) Release()
```

Use sentinel errors `errSwapRecordNotFound`, `errSwapRecordExpired`, `errSwapStoreFull`, and
`errSwapBuildConflict`. Count both maps toward 10,000; sweep expired entries before rejecting a put.
`acquireBuild` locks only the selected record, rechecks expiry, binds the first attempted build ID and
fingerprint, and caches only successful responses. A failed build remains retryable only with the same ID
and fingerprint. `Release` unlocks exactly once.

- [ ] **Step 5: Run race tests and commit**

```bash
gofmt -w internal/solvers/rfq/swap_store.go internal/solvers/rfq/swap_store_test.go
go test -race ./internal/solvers/rfq -run 'TestSwapStore' -count=1
git add internal/solvers/rfq/swap_store.go internal/solvers/rfq/swap_store_test.go
git commit -m "feat(rfq): store immutable swap confirmations"
```

Expected: PASS with no races or mutable aliases.

---

### Task 5: Encode deterministic Router-bound adapter calls

**Files:**
- Create: `internal/solvers/rfq/swap_signing.go`
- Create: `internal/solvers/rfq/swap_signing_test.go`

**Interfaces:**
- Consumes: framework `signer.Signer`, generated LiquidLane adapter bindings, and UUID build IDs.
- Produces: deterministic nonce, EIP-712 digest, and signed-swap calldata helpers.

- [ ] **Step 1: Write independent digest, nonce, and calldata tests**

Add tests which derive expected hashes with `github.com/ethereum/go-ethereum/signer/core/apitypes`, not
the production helpers:

```go
func TestSignedSwapDigestMatchesEIP712Reference(t *testing.T)
func TestSignedSwapNonceMatchesGoldenVector(t *testing.T)
func TestPackSignedSwapCallUsesBindingSelectorAndFrameworkSigner(t *testing.T)
func TestPackSignedSwapCallPropagatesSigningFailure(t *testing.T)
```

Pin these protocol constants in the tests:

```go
const signedSwapTypeHashHex = "0xffdb5fa9b52456ef5eb17369ab4182fbbad380b6a348af67c04fc73924d9bc77"
const swapNonceTypeHashHex = "0x6011c54012434af706b36d8a23df1173ae1c8af38fa0f0dfb634193b013f6571"
const swapNonceGoldenHex = "0x60e388e6f57a7f56b02157a0756e27a89e584029632d086f9a5c3fc1c47ab643"
```

The nonce golden input is build ID `7423df2b-957b-47d5-acbc-21c3bd8a614e`, chain ID `1`, adapter
`0x3333333333333333333333333333333333333333`, token input
`0x1111111111111111111111111111111111111111`, and call index `0`. ABI-unpack the generated call
payload and assert every tuple field and signature byte. Assert signed selector `0x9a4568b6`.

- [ ] **Step 2: Verify the tests fail**

```bash
go test ./internal/solvers/rfq -run 'Test(SignedSwap|PackSignedSwap)' -count=1
```

Expected: compilation fails because the signing helpers do not exist.

- [ ] **Step 3: Implement the exact signing surface**

```go
type swapDomain struct {
	Name              string
	Version           string
	ChainID           *big.Int
	VerifyingContract common.Address
}

func signedSwapDigest(domain swapDomain, value adapter.ILiquidLaneAdapterSignedSwap) (common.Hash, error)
func signedSwapNonce(buildID uuid.UUID, chainID int64, adapterAddress, tokenIn common.Address, callIndex int) *big.Int
func packSignedSwapCall(
	frameworkSigner signer.Signer,
	domain swapDomain,
	value adapter.ILiquidLaneAdapterSignedSwap,
) ([]byte, error)
```

Hash `SignedSwap(address recipient,address tokenIn,uint256 amountIn,uint256 amountOut,address caller,address signer,uint256 nonce,uint48 deadline)` using the standard EIP-712 domain
`EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)`. Compute the nonce as
`keccak256(abi.encode(keccak256("VaultSolverSwapNonce(bytes16 buildId,uint256 chainId,address adapter,address tokenIn,uint256 callIndex)"), buildId, chainId, adapter, tokenIn, callIndex))`.
Use `adapter.NewLiquidLaneAdapter()` only as the generated ABI packer; do not edit the binding. Require a
65-byte framework signature whose `V` is 27 or 28.

- [ ] **Step 4: Run focused tests and commit**

```bash
gofmt -w internal/solvers/rfq/swap_signing.go internal/solvers/rfq/swap_signing_test.go
go test ./internal/solvers/rfq -run 'Test(SignedSwap|PackSignedSwap)' -count=1
git add internal/solvers/rfq/swap_signing.go internal/solvers/rfq/swap_signing_test.go
git commit -m "feat(rfq): encode signed swap adapter calls"
```

Expected: PASS, with hashes matching the independent reference implementation.

---

### Task 6: Read adapter domains, authorization, freshness, and nonce state

**Files:**
- Create: `internal/solvers/rfq/swap_chainreader.go`
- Create: `internal/solvers/rfq/swap_chainreader_test.go`
- Modify: `internal/solvers/rfq/chainreader.go`
- Modify: `internal/solvers/rfq/chainreader_test.go`

**Interfaces:**
- Consumes: shared `liquidlane.Reader`, chain multicall client, framework signer address, routes, and nonces.
- Produces: `swapStateReader` for startup/CONFIRM/BUILD validation without duplicating LiquidLane reads.

- [ ] **Step 1: Write failing adapter-state tests**

Create deterministic multicall fakes and these tests:

```go
func TestSwapOnchainReaderValidateRouterRequiresDeployedCode(t *testing.T)
func TestSwapOnchainReaderValidateAdaptersAcceptsOwnerMarketMakerOrFiller(t *testing.T)
func TestSwapOnchainReaderValidateAdaptersRejectsUnauthorizedSigner(t *testing.T)
func TestSwapOnchainReaderValidateAdaptersRejectsWrongChainOrVerifyingContract(t *testing.T)
func TestSwapOnchainReaderValidateAdaptersRejectsUnsupportedDomainShape(t *testing.T)
func TestSwapOnchainReaderReadFillQuoteRequiresExactRouteAndAmount(t *testing.T)
func TestSwapOnchainReaderReadUsedNoncesFailsClosed(t *testing.T)
func TestReaderResolvesDynamicAdapterMetadataWithoutMutatingStaticCache(t *testing.T)
```

Cover domain `fields != 0x0f`, nonzero salt, nonempty extensions, empty name/version, failed calls,
malformed results, a used nonce, and transport failure. Verify one batched authorization/domain read per
deduplicated adapter set.

- [ ] **Step 2: Verify the tests fail**

```bash
go test ./internal/solvers/rfq -run 'Test(SwapOnchainReader|ReaderResolvesDynamic)' -count=1
```

Expected: compilation fails because the swap reader does not exist.

- [ ] **Step 3: Add the narrow read interface and implementation**

```go
type swapNonceCheck struct {
	Adapter common.Address
	TokenIn common.Address
	Nonce   *big.Int
}

type swapStateReader interface {
	validateRouter(context.Context, common.Address) error
	validateAdapters(context.Context, []common.Address, common.Address) (map[common.Address]swapDomain, error)
	readFillQuote(context.Context, liquidlane.Route, *big.Int) (liquidlane.FillQuote, error)
	readUsedNonces(context.Context, []swapNonceCheck) ([]bool, error)
}
```

Implement it as `swapOnchainReader` backed by the existing chain client and `*liquidlane.Reader`:

- `validateRouter` calls `CodeAt` and rejects zero-length bytecode.
- `validateAdapters` deduplicates addresses, calls `ReadAuth`, requires `Authorized` for the framework
  signer, batches generated `PackEip712Domain` calls, and returns only domains with fields `0x0f`, exact
  configured chain ID, exact adapter verifying contract, zero salt, empty extensions, and nonempty name
  and version.
- `readFillQuote` delegates to `ReadFillQuotes` with one exact route/amount and rejects zero, extra, or
  identity-mismatched results.
- `readUsedNonces` batches generated `PackIsUsedNonce(tokenIn, nonce)` calls and treats every transport,
  per-call, or decode failure as an error.

Add `swapState swapStateReader` to `reader`; construct it from the same chain client and the same
`liquidlane.Reader`. Keep `quoteAdapters` as the immutable startup cache, while dynamic adapters are
resolved into a request-local metadata map by the existing `ResolveAdapters` path.

- [ ] **Step 4: Run focused tests and commit**

```bash
gofmt -w internal/solvers/rfq/swap_chainreader.go internal/solvers/rfq/swap_chainreader_test.go internal/solvers/rfq/chainreader.go internal/solvers/rfq/chainreader_test.go
go test ./internal/solvers/rfq -run 'Test(SwapOnchainReader|ReaderResolvesDynamic)' -count=1
git add internal/solvers/rfq/swap_chainreader.go internal/solvers/rfq/swap_chainreader_test.go internal/solvers/rfq/chainreader.go internal/solvers/rfq/chainreader_test.go
git commit -m "feat(rfq): validate swap adapter state"
```

Expected: PASS; every security-sensitive read fails closed.

---

### Task 7: Implement DISCOVERY and CONFIRM with immutable allocations

**Files:**
- Create: `internal/solvers/rfq/swap.go`
- Create: `internal/solvers/rfq/swap_test.go`
- Modify: `internal/solvers/rfq/test_helpers_test.go`

**Interfaces:**
- Consumes: parsed phase requests, existing candidate reader, configured strategy, signer/domain reader,
  adapter policy, token policy, quote TTL, and `swapStore`.
- Produces: `swapService.swap`, advisory discovery points, and stored confirmations.

- [ ] **Step 1: Write failing DISCOVERY tests**

Add table-driven tests:

```go
func TestSwapDiscoveryReturnsOnlyFullyCoveredRequestedSamples(t *testing.T)
func TestSwapDiscoveryReturnsEmptyPointsForNoLiquidity(t *testing.T)
func TestSwapDiscoveryRejectsDuplicateAdapterTargets(t *testing.T)
func TestSwapDiscoveryUsesCanonicalUniqueSortedCapacityDomains(t *testing.T)
func TestSwapDiscoverySkipsDiscountInventoryInExternalMode(t *testing.T)
func TestSwapDiscoveryPropagatesDependencyFailure(t *testing.T)
func TestSwapDiscoveryAppliesChainTokenMinimumAndAdapterPolicies(t *testing.T)
```

Assert one coherent candidate read at the largest requested sample and one strategy decision per sample;
explicitly reject a
plan whose leg inputs do not sum to the sample or whose leg exceeds its source candidate's
`MaxAmountIn`. Assert no discovery record contains a signature, nonce, or executable calldata.

- [ ] **Step 2: Write failing CONFIRM tests**

```go
func TestSwapConfirmStoresExactOrderedPlanAndReturnsFloor(t *testing.T)
func TestSwapConfirmReturnsNoContentWhenExactInputCannotBeCovered(t *testing.T)
func TestSwapConfirmRequiresReturnedDiscoveryPointAndExactFloor(t *testing.T)
func TestSwapConfirmRequiresExactTupleAndCapacityDomainSet(t *testing.T)
func TestSwapConfirmCapsValidityAtTTLDeadlineAndAuthorization(t *testing.T)
func TestSwapConfirmValidatesDynamicAdapterDomainAndSigner(t *testing.T)
func TestSwapConfirmRejectsDuplicateAdapterTarget(t *testing.T)
func TestSwapConfirmRejectsExpiredDeadlineWithoutStoring(t *testing.T)
```

Inspect the stored record to prove it retains candidate ID, full route, adapter, split input, per-leg
confirmed output, discount ID, capacity ID, validity, and original order. Assert a 204 outcome stores
nothing.

- [ ] **Step 3: Verify the phase tests fail**

```bash
go test ./internal/solvers/rfq -run 'TestSwap(Discovery|Confirm)' -count=1
```

Expected: compilation fails because `swapService` does not exist.

- [ ] **Step 4: Implement service construction and error taxonomy**

```go
type swapService struct {
	chainID          int64
	executor         common.Address
	router           common.Address
	quoteTTL         time.Duration
	whitelist        adapterWhitelist
	tokenPolicy      tokenpolicy.Policy
	minAmountsIn     map[common.Address]*big.Int
	discountsEnabled bool
	reader           quoteCandidateReader
	state            swapStateReader
	strategy         types.Strategy
	store            *swapStore
	now              func() time.Time
	newID            func() uuid.UUID
	log              logr.Logger
}

func (s *swapService) swap(ctx context.Context, request *swapRequest) (*swapResponse, error)
func (s *swapService) discover(ctx context.Context, request *parsedDiscoveryRequest) (*swapResponse, error)
func (s *swapService) confirm(ctx context.Context, request *parsedConfirmRequest) (*swapResponse, error)
```

Use a private error carrying exact HTTP status and a stable public message. Map parse/policy/mismatch
errors to 400, missing records to 404, expired records/deadlines to 410, stale/domain conflicts to 409,
capacity exhaustion to 429, and unexpected read/strategy/signing dependencies to 502. Return a typed
`errSwapNoContent` only for a valid CONFIRM with insufficient exact coverage.

- [ ] **Step 5: Implement DISCOVERY**

Read current candidates once at the largest sample, remove discount candidates in external mode, then
run `strategy.DecideQuote` for every requested amount against clones of that same coherent candidate
snapshot and recover each plan with `FillPlanFromQuote`. Keep the point only when the plan
is fully covered, every selected candidate still bounds its leg, adapter targets are unique, and the
domain set is canonical and unique. Sort domain strings lexicographically for deterministic comparison.
Store the complete discovery tuple and returned points until `now + swapQuoteTTL`; return 200 with an
empty point list when none are coverable.

- [ ] **Step 6: Implement CONFIRM**

Load the discovery by `discoveryRequestId`; require exact quote/chain/swapper/tokens, require `amountIn`
to identify a returned point, and require `minAmountOut` to equal its output. Re-read and re-decide that
exact size. Return no-content if it is not fully coverable. Otherwise require the fresh output to meet
the requested floor and the fresh unique capacity-domain set to equal the discovery set.

Validate all selected adapter domains and signer authorization before storing. Treat the request deadline
as a maximum and set `validUntil` to the minimum of that maximum, `now + swapQuoteTTL`, and every nonzero
selected candidate `ValidUntil`; shorten a longer maximum rather than rejecting it. Reject only if the
result is not in the future or does not fit `uint48`. Generate `solverQuoteId`, deep-store the
ordered exact plan and the response floor, and return the exact CONFIRM wire response.

- [ ] **Step 7: Run service tests and commit**

```bash
gofmt -w internal/solvers/rfq/swap.go internal/solvers/rfq/swap_test.go internal/solvers/rfq/test_helpers_test.go
go test -race ./internal/solvers/rfq -run 'TestSwap(Discovery|Confirm)' -count=1
git add internal/solvers/rfq/swap.go internal/solvers/rfq/swap_test.go internal/solvers/rfq/test_helpers_test.go
git commit -m "feat(rfq): discover and confirm swap allocations"
```

Expected: PASS with exact point/floor/domain behavior and no state races.

---

### Task 8: Build the exact confirmed allocation idempotently

**Files:**
- Modify: `internal/solvers/rfq/swap.go`
- Modify: `internal/solvers/rfq/swap_test.go`

**Interfaces:**
- Consumes: stored `confirmationRecord`, exact physical fill quotes, adapter domains, deterministic
  nonces, signer, and `buildLease`.
- Produces: all-or-nothing ordered `calls` for the configured Router; never a transaction.

- [ ] **Step 1: Write failing exact-allocation and stale-state tests**

```go
func TestSwapBuildReturnsSignedCallsForExactConfirmedAllocation(t *testing.T)
func TestSwapBuildPreservesOrderSplitAdaptersAndCandidateIdentity(t *testing.T)
func TestSwapBuildAllowsImprovedOutputButNotLegBelowConfirmedFloor(t *testing.T)
func TestSwapBuildRejectsChangedTupleFloorDeadlineDomainsOrRouter(t *testing.T)
func TestSwapBuildRejectsExpiredConfirmation(t *testing.T)
func TestSwapBuildRejectsMissingChangedOrDuplicateAdapter(t *testing.T)
func TestSwapBuildAccountsCombinedCapacityByCapacityID(t *testing.T)
func TestSwapBuildRejectsUsedDeterministicNonce(t *testing.T)
func TestSwapBuildReturnsNoPartialCallsWhenOneLegFails(t *testing.T)
func TestSwapBuildNeverCallsTransactionManager(t *testing.T)
```

Decode each direct call with the generated ABI. Assert `recipient == router`, `caller == router`,
`signer == framework signer`, input/output/nonce/deadline match the response, and the recovered signer
is authorized for that adapter. Assert the aggregate input and output equations exactly.

- [ ] **Step 2: Write failing idempotency and concurrency tests**

```go
func TestSwapBuildRetryWithFreshRequestIDEchoesNewEnvelope(t *testing.T)
func TestSwapBuildConcurrentFreshRequestIDsShareImmutablePayload(t *testing.T)
func TestSwapBuildRejectsSecondBuildID(t *testing.T)
func TestSwapBuildRejectsSameBuildIDWithDifferentNormalizedRequest(t *testing.T)
func TestSwapBuildFailedAttemptRetriesOnlySameBuildIDAndFingerprint(t *testing.T)
```

Count physical reads and signer calls. The successful concurrent case must perform each once. Exclude
transport-only `requestId` from the fingerprint, but normalize addresses, decimal amounts, UUIDs, and
sorted domains before hashing every economic field. Cache only immutable calls/signatures and reconstruct
each response envelope with the current request ID.

- [ ] **Step 3: Verify BUILD tests fail**

```bash
go test ./internal/solvers/rfq -run 'TestSwapBuild' -count=1
```

Expected: BUILD is not implemented or does not satisfy the assertions.

- [ ] **Step 4: Implement request binding and build lease acquisition**

```go
func (s *swapService) build(ctx context.Context, request *parsedBuildRequest) (*swapResponse, error)
func buildFingerprint(request *parsedBuildRequest) common.Hash
```

Require exact stored quote ID, solver quote ID, chain, swapper, tokens, input, confirmed output floor,
sorted domain set, and configured Router. Require `now < deadline <= confirmation.validUntil`; the first
build fingerprint freezes that chosen deadline. Acquire the per-confirmation lease before any fresh read
or signature. Rebuild an envelope around a deep-copied cached payload; otherwise defer `Release` and call
`Complete` only after every leg succeeds and the chosen deadline is rechecked.

- [ ] **Step 5: Revalidate and encode direct legs**

Revalidate every adapter's EIP-712 domain and signer authorization. For each persisted leg in order,
read the exact persisted route at its exact input. Do not call the strategy and do not substitute a
candidate. Require route identity, adapter, token pair, capacity ID, and split input to remain exact;
require fresh output to meet that leg's confirmed output floor.

For routes sharing a capacity ID, sum the planned fresh output consumption and compare it with the
lowest fresh `MaxAssets` reported by those physical reads. Reject duplicate adapter targets even when
their capacity IDs match. Derive each nonce from the zero-based persisted call index, batch
`isUsedNonce`, and reject any used nonce without trying another value.

Build `adapter.ILiquidLaneAdapterSignedSwap` with Router recipient/caller, common token input, leg input,
fresh conservative output, framework signer, deterministic nonce, and chosen BUILD deadline. Sign
and pack every call, then construct the response from those same values. Verify sums and top-level floor
and recheck the deadline before caching. The service has no `txSender` dependency and never broadcasts.

- [ ] **Step 6: Run BUILD tests and commit**

```bash
gofmt -w internal/solvers/rfq/swap.go internal/solvers/rfq/swap_test.go
go test -race ./internal/solvers/rfq -run 'TestSwapBuild' -count=1
git add internal/solvers/rfq/swap.go internal/solvers/rfq/swap_test.go
git commit -m "feat(rfq): build idempotent swap calldata"
```

Expected: PASS; retry payloads are byte-identical under fresh request envelopes and stale allocations emit no calldata.

---

### Task 9: Sign discount-selected legs without exposing private payloads

> **Superseded result:** This historical task produced the signed-only behavior at initial delivery.
> `2026-08-04-per-leg-swap-authorization.md` replaces its resulting behavior with exact resolved
> `DiscountSwap` calldata for discount-selected legs.

**Files:**
- Modify: `internal/solvers/rfq/swap.go`
- Modify: `internal/solvers/rfq/swap_test.go`

**Interfaces:**
- Consumes: persisted candidate identity and a fresh physical quote for the same route.
- Produces: only `SignedSwap` calldata on the persisted adapter.

- [x] **Step 1: Write the failing signed-only discount-selection test**

```go
func TestSwapBuildSignsDiscountSelectedLegOnPersistedAdapter(t *testing.T)
```

Confirm from discount inventory without configuring any discount resolver. BUILD must succeed with
selector `0x9a4568b6`, preserve adapter/route/split/domain/order, use the original call index for its
deterministic nonce, and create the Router-bound adapter signature.

- [x] **Step 2: Remove the private discount BUILD surface**

Remove discount resolution, `TryPackSwap0`, and discount calldata tests from the user-directed path.
Re-read the same physical route and require its fresh conservative output to meet the persisted leg
floor; otherwise return a stale-confirmation conflict. Legacy awarded-order discount execution remains
unchanged.

- [x] **Step 3: Run signed-only BUILD tests**

```bash
go test ./internal/solvers/rfq -run 'TestSwapBuildSignsDiscountSelectedLegOnPersistedAdapter' -count=1
```

Expected: PASS with only selector `0x9a4568b6` and no discount-backend dependency.

---

### Task 10: Mount authenticated `/swap`, metrics, and startup validation

**Files:**
- Modify: `internal/solvers/rfq/server.go`
- Modify: `internal/solvers/rfq/server_test.go`
- Modify: `internal/solvers/rfq/metrics.go`
- Modify: `internal/solvers/rfq/metrics_test.go`
- Modify: `internal/solvers/rfq/solver.go`
- Modify: `internal/solvers/rfq/solver_test.go`

**Interfaces:**
- Consumes: parsed config, framework signer, shared backend client, RFQ middleware, and `swapService`.
- Produces: conditional authenticated `POST /swap`, OpenAPI schema, bounded metrics, and fail-fast startup.

- [ ] **Step 1: Write failing HTTP contract tests**

```go
func TestServerSwapIsAbsentWhenDisabled(t *testing.T)
func TestServerSwapRequiresSharedSecret(t *testing.T)
func TestServerSwapRoutesAllThreePhases(t *testing.T)
func TestServerSwapReturnsNoContentWithoutBody(t *testing.T)
func TestServerSwapMapsBadRequestNotFoundConflictGoneTooManyAndBadGateway(t *testing.T)
func TestServerSwapRejectsOversizeAndMalformedBodies(t *testing.T)
func TestServerSwapOpenAPIPinsProtocolPhaseAndWireNames(t *testing.T)
```

Assert the OpenAPI JSON and response bodies use exactly `protocol`, `phase`, `requestId`,
`discoveryRequestId`, `quoteId`, `solverQuoteId`, `buildId`, `chainId`, `swapper`, `router`, `tokenIn`,
`tokenOut`, `sampleAmountsIn`, `adapters`, `points`, `amountIn`, `minAmountOut`, `amountOut`,
`liquidityDomains`, `validUntil`, `calls`, and singular call field `liquidityDomain`. Assert no nested
validity object and no native-value field.

- [ ] **Step 2: Write failing wiring and startup tests**

```go
func TestFactoryWiresSwapOnlyWhenEnabled(t *testing.T)
func TestFactorySharesBackendAndPassesFrameworkSigner(t *testing.T)
func TestSolverRunValidatesRouterAndStaticAdaptersBeforeListening(t *testing.T)
func TestSolverRunAllowsInternalModeWithoutStaticAdapters(t *testing.T)
func TestSolverRunFailsOnUnauthorizedSignerOrInvalidDomain(t *testing.T)
func TestHTTPMetricsRecordsSwapRouteAndBoundedPhaseOutcome(t *testing.T)
```

Prove Router validation precedes listener startup; static configured adapters are validated once at
startup, while internal dynamic adapters remain request-time validated and are revalidated at BUILD.

- [ ] **Step 3: Verify integration tests fail**

```bash
go test ./internal/solvers/rfq -run 'Test(ServerSwap|FactoryWiresSwap|FactorySharesBackend|SolverRunValidatesRouter|SolverRunAllowsInternal|SolverRunFails|HTTPMetricsRecordsSwap)' -count=1
```

Expected: `/swap` is absent and swap wiring does not exist.

- [ ] **Step 4: Register the endpoint conditionally**

Add `swaps *swapService` to `server`. When it is nil, do not register `/swap`. When present, register
`POST /swap` with the existing 1 MiB limit, timeout, recovery, request-ID, logging, metrics, and
constant-time shared-secret authentication. Parse phase-specific payloads manually through Task 2's
parser so wrong/missing fields return 400. Translate `errSwapNoContent` to a header-only 204 and the
service error taxonomy to the documented statuses without exposing wrapped causes.

- [ ] **Step 5: Wire one backend, signer, store, and startup checks**

Create one `backendClient` in `factory` and pass it to both execution and swap services. Pass
`deps.Signer` only to the swap service; leave the legacy transaction manager only on execution. Use the
same configured strategy, reader, token policy, minimum amounts, and adapter scope as quoting. Create a
separate `swapStore`, and populate the server's swap field only when `cfg.SwapEnabled`.

In `Solver.Run`, before starting the HTTP listener, validate configured Router bytecode and validate all
static configured adapter domains/authorization when swaps are enabled. Preserve existing adapter/vault
resolution and external direct-fill authorization. Internal mode may start with no static adapters;
request-local adapters are validated during CONFIRM and every selected adapter is revalidated during
BUILD.

- [ ] **Step 6: Add bounded observability**

Add `/swap` to HTTP route normalization. Add one counter labeled only by `phase` and `outcome`, where
phase is `discovery`, `confirm`, `build`, or `invalid`, and outcome is `success`, `no_content`,
`bad_request`, `forbidden`, `not_found`, `conflict`, `expired`, `store_full`, or `dependency_error`.
Never label with IDs, addresses, domains, or amounts.

- [ ] **Step 7: Run integration tests and commit**

```bash
gofmt -w internal/solvers/rfq/server.go internal/solvers/rfq/server_test.go internal/solvers/rfq/metrics.go internal/solvers/rfq/metrics_test.go internal/solvers/rfq/solver.go internal/solvers/rfq/solver_test.go
go test -race ./internal/solvers/rfq -run 'Test(ServerSwap|FactoryWiresSwap|FactorySharesBackend|SolverRunValidatesRouter|SolverRunAllowsInternal|SolverRunFails|HTTPMetricsRecordsSwap)' -count=1
git add internal/solvers/rfq/server.go internal/solvers/rfq/server_test.go internal/solvers/rfq/metrics.go internal/solvers/rfq/metrics_test.go internal/solvers/rfq/solver.go internal/solvers/rfq/solver_test.go
git commit -m "feat(rfq): expose authenticated swap calldata API"
```

Expected: PASS; disabled deployments expose no route and enabled deployments fail fast on unsafe static
configuration.

---

### Task 11: Document the feature and run the complete safety gate

**Files:**
- Modify: `config/rfq.example.yaml`
- Modify: `README.md`
- Modify: `docs/RFQ-PLAN.md`
- Create: `internal/solvers/rfq/docs_test.go`

**Interfaces:**
- Consumes: finalized configuration and HTTP contract.
- Produces: operator-facing enablement, security, lifecycle, and restart behavior documentation.

- [ ] **Step 1: Write the documentation contract before prose**

Add a focused source test if no existing documentation test covers these files:

```go
func TestSwapDocumentationPinsConfigurationAndSecurityContract(t *testing.T) {
	checks := map[string][]string{
		"../../../config/rfq.example.yaml": {"swapEnabled", "router", "swapQuoteTtlMs", "30000"},
		"../../../README.md": {"POST /swap", "DISCOVERY", "CONFIRM", "BUILD", "never broadcasts", "transport-only"},
		"../../../docs/RFQ-PLAN.md": {"in-memory", "0x9a4568b6", "0x8fa5c671", "resolved signed discount", "restart"},
	}
	for path, required := range checks {
		body, err := os.ReadFile(path)
		if err != nil { t.Fatalf("read %s: %v", path, err) }
		for _, needle := range required {
			if !bytes.Contains(body, []byte(needle)) {
				t.Errorf("%s missing %q", path, needle)
			}
		}
	}
}
```

Assert documentation contains `swapEnabled`, `router`, `swapQuoteTtlMs`, default `30000`, authenticated
`POST /swap`, all three uppercase phase names, no transaction broadcast, in-memory confirmation expiry,
restart invalidation, Router transfer-before-call behavior, zero native value, and aggregate output-floor
enforcement.

- [ ] **Step 2: Verify the documentation test fails**

```bash
go test ./internal/solvers/rfq -run TestSwapDocumentationPinsConfigurationAndSecurityContract -count=1
```

Expected: FAIL because the operator contract is not documented.

- [ ] **Step 3: Update operator documentation**

In `config/rfq.example.yaml`, add disabled defaults:

```yaml
swapEnabled: false
router: ""
swapQuoteTtlMs: 30000
```

In `README.md`, document opt-in configuration, startup requirements, shared-secret authentication, and
the discovery/confirm/build lifecycle. In `docs/RFQ-PLAN.md`, record the allocation invariants,
deterministic nonce and fresh-request-envelope retry rule, per-leg direct/discount selectors, dynamic-adapter validation, and the fact
that the solver signs calldata but neither transfers Router inputs nor broadcasts a transaction.

- [ ] **Step 4: Run documentation and package tests**

```bash
gofmt -w internal/solvers/rfq
go test ./internal/solvers/rfq -count=1
git add config/rfq.example.yaml README.md docs/RFQ-PLAN.md internal/solvers/rfq
git commit -m "docs(rfq): document swap calldata protocol"
```

Expected: PASS with no unrelated generated-file changes.

- [ ] **Step 5: Run formatting, static checks, full tests, and race tests**

```bash
gofmt -w internal/solvers/rfq internal/solvers/rfq/strategies
GOTOOLCHAIN=go1.26.5 go build ./...
GOTOOLCHAIN=go1.26.5 go test -race -cover ./...
GOTOOLCHAIN=go1.26.5 golangci-lint run
git diff --check
git status --short
```

Expected: every command passes; status contains only intentional implementation/documentation files.

- [ ] **Step 6: Audit the final diff against protocol invariants**

```bash
rg -n '0x9a4568b6|VaultSolverSwapNonce|transport-only|swapEnabled|swapQuoteTtlMs|POST /swap' internal/solvers/rfq README.md docs/RFQ-PLAN.md config/rfq.example.yaml
rg -n 'TODO|FIXME|TBD|placeholder|similar to' internal/solvers/rfq README.md docs/RFQ-PLAN.md config/rfq.example.yaml
git diff --stat
git log --oneline --max-count=12
```

Expected: protocol constants and operational controls are present; the placeholder scan has no hits in
new material; generated bindings, transaction-manager code, contracts, migrations, and unrelated solver
paths remain unchanged.
