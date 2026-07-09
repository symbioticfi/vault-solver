# 3F Solver Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the default offer-coverage gap, make 3F rates and signature identifiers exact, support optional salted EIP-712 domains, and characterize the full auction/offer/redemption boundaries.

**Architecture:** The vendored OpenAPI document remains the wire contract and generated code is refreshed from it. All transport floats are normalized once at the handwritten boundary into integer tenth-basis-points; strategies and signatures use exact integers. Offer lifetime is derived from typed configuration and one injected clock.

**Tech Stack:** Go 1.26.5, generated 3F OpenAPI client, go-ethereum EIP-712/ABI types, Multicall3, `httptest`, `big.Int`/`big.Rat`.

## Global Constraints

- Finding 1 remains out of scope.
- Never hand-edit `api/threef`; update `openapi/3f-bf.openapi.json` and run `make refresh-3f-client`.
- Preserve the upstream numeric JSON wire form for `maxRate`; only webhook strategy JSON changes from a number to an exact decimal string.
- No money-facing calculation may use `float32`, `float64`, or `big.Float` after boundary normalization.
- Unsalted EIP-712 output must remain byte-for-byte compatible.
- Use injected clocks in tests; do not sleep.
- Follow strict TDD for handwritten behavior and commit docs with the behavior they describe.

---

### Task 1: Make Offer Lifetime Cover Discovery

**Files:**
- Modify: `internal/solvers/bridgefacilitator/config.go`
- Modify: `internal/solvers/bridgefacilitator/config_test.go`
- Modify: `internal/solvers/bridgefacilitator/solver.go`
- Modify: `internal/solvers/bridgefacilitator/offer.go`
- Modify: `internal/solvers/bridgefacilitator/strategy_test.go`
- Modify: `config/3f.example.yaml`
- Modify: `README.md`
- Modify: `docs/3F-PLAN.md`

**Interfaces:**
- Produces: `Intervals.OfferTTL time.Duration` from YAML `intervals.offerTTL`.
- Produces: omitted TTL defaults to `2 * Discover`; explicit TTL must be at least `Discover`.
- Produces: `Solver.now func() time.Time`, initialized to `time.Now`.

- [ ] **Step 1: Add failing configuration tests**

Add these cases:

```go
func TestParseConfigOfferTTL(t *testing.T) {
	cfg := mustParse(t, oneTarget+"intervals:\n  discover: 20m\n")
	if cfg.Intervals.OfferTTL != 40*time.Minute {
		t.Fatalf("offer TTL = %s, want 40m", cfg.Intervals.OfferTTL)
	}

	cfg = mustParse(t, oneTarget+"intervals:\n  discover: 20m\n  offerTTL: 45m\n")
	if cfg.Intervals.OfferTTL != 45*time.Minute { t.Fatalf("offer TTL = %s", cfg.Intervals.OfferTTL) }

	if _, err := parse(t, oneTarget+"intervals:\n  discover: 20m\n  offerTTL: 19m\n"); err == nil {
		t.Fatal("expected offerTTL shorter than discover to fail")
	}
}
```

Add a signed-offer test with `s.now = func() time.Time { return time.Unix(1_000, 0) }` and `OfferTTL: 45*time.Minute`; assert DTO expiration is `3700` and the signed digest uses the same value.

- [ ] **Step 2: Run RED**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test ./internal/solvers/bridgefacilitator -run 'TestParseConfigOfferTTL|TestBuildSignedOffer.*TTL' -v
```

Expected: `OfferTTL` and `Solver.now` do not exist; the fixed 30-minute expiration fails.

- [ ] **Step 3: Parse and validate dynamic TTL**

Add `OfferTTL string` to `rawIntervals` and `OfferTTL time.Duration` to `Intervals`. After parsing `discover`:

```go
if discover > time.Duration(math.MaxInt64/2) {
	return nil, errors.New("intervals.discover is too large to derive offerTTL")
}
offerTTL, err := cfgparse.Duration(raw.Intervals.OfferTTL, 2*discover, "intervals.offerTTL")
if err != nil { return nil, err }
if offerTTL < discover {
	return nil, errors.New("intervals.offerTTL must be >= intervals.discover")
}
```

Store it in the returned `Intervals`.

- [ ] **Step 4: Inject one clock and use configured lifetime**

Add `now func() time.Time` to `Solver`, set it in `factory`, and replace solver-local wall-clock reads used for offer expiry/cache snapshots with `s.now()`. In `buildSignedOffer`:

```go
now := s.now()
expiration := big.NewInt(now.Add(s.cfg.Intervals.OfferTTL).Unix())
```

Use that one `expiration` for the signed `Offer`, DTO, and later cache record. Remove the fixed `offerTTL` constant.

- [ ] **Step 5: Run GREEN and update operator docs**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race ./internal/solvers/bridgefacilitator -run 'TestParseConfigOfferTTL|TestBuildSignedOffer.*TTL|TestOffer' -v
```

Expected: default, override, rejection, and signed expiration pass.

Add `offerTTL` to the annotated example and explain the dynamic default/invariant in README and the 3F plan.

- [ ] **Step 6: Commit**

```bash
git add internal/solvers/bridgefacilitator/config.go internal/solvers/bridgefacilitator/config_test.go internal/solvers/bridgefacilitator/solver.go internal/solvers/bridgefacilitator/offer.go internal/solvers/bridgefacilitator/strategy_test.go config/3f.example.yaml README.md docs/3F-PLAN.md
git commit -m "fix(3f): align offer lifetime with discovery"
```

### Task 2: Correct the Vendored 3F Schema and Regenerate

**Files:**
- Modify: `openapi/3f-bf.openapi.json`
- Regenerate: `api/threef/*.go`
- Modify: `internal/solvers/bridgefacilitator/apiclient.go`
- Modify: `internal/solvers/bridgefacilitator/apiclient_test.go`
- Modify: `internal/solvers/bridgefacilitator/auctionview.go`
- Modify: `internal/solvers/bridgefacilitator/offer.go`
- Modify: `internal/solvers/bridgefacilitator/solver.go`
- Modify: `internal/solvers/bridgefacilitator/strategy_test.go`

**Interfaces:**
- Produces: `AuctionEip712DomainDto.Salt` as nullable string.
- Produces: `maxRate` as generated `float64` with `multipleOf: 0.1`.
- Produces: signature-bearing chain/auction/offer IDs as generated `int64`.

- [ ] **Step 1: Add a failing generated-client JSON fixture**

Before changing the schema, add a test that unmarshals:

```json
{
  "id": 9007199254740993,
  "requestId": "0x0000000000000000000000000000000000000010",
  "amountRequested": "1000000000",
  "solve_start_time": null,
  "maxRate": 50.5,
  "status": "open",
  "asset": null,
  "depositAsset": null,
  "vault": null,
  "settlement": null,
  "direction": null,
  "eip712Domain": {
    "name": "SuperstateRequest",
    "version": "1",
    "chainId": 9007199254740993,
    "salt": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  }
}
```

Assert the ID and chain ID are exactly `9007199254740993` (beyond float64's exact-integer range),
max rate is `50.5`, and salt is present.

- [ ] **Step 2: Run RED**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test ./internal/solvers/bridgefacilitator -run TestGeneratedAuctionExactFields -v
```

Expected: current generated types are `float32` and reject unknown `salt`.

- [ ] **Step 3: Update the vendored schema**

Use these exact property shapes:

```json
"chainId": {"type":"integer","format":"int64","nullable":true,"minimum":1},
"salt": {
  "type":"string",
  "nullable":true,
  "pattern":"^0x[0-9a-fA-F]{64}$",
  "description":"Optional EIP-712 domain salt as bytes32"
},
"maxRate": {
  "type":"number",
  "format":"double",
  "multipleOf":0.1,
  "nullable":true
}
```

Change every semantic ID/chain surface below from generic `number` to `integer` with `format: int64`.
Retain each field's existing description, nullability, required status, and example:

```text
GenerateFacilitatorApiKeyDto.chainId
CreateOfferDto.chainId
CreateOfferDto.auctionId
CreateOfferResponseDto.id
CancelOfferDto.offerId
CancelOfferDto.chainId
CancelOfferResponseDto.id
OfferDto.id
OfferDto.auctionId
AuctionEip712DomainDto.chainId
AuctionDto.id
GET /v1/offer query chainId
GET /v1/offer/{id} path id
GET /v1/offer/{id} query chainId
GET /v1/auction/{id} path id
```

Do not leave a float-backed alternate route to any offer, auction, or chain identity.

- [ ] **Step 4: Regenerate and migrate handwritten call sites**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local make refresh-3f-client
```

Update constructors/setters/tests to use `int64` and generated `NullableFloat64`; remove lossy
`float32(...)` conversions. Add an `httptest` request-builder test that sends
`9007199254740993` through `/v1/offer?chainId=...`, `/v1/offer/{id}?chainId=...`, and
`/v1/auction/{id}` and asserts the captured query/path digits are unchanged. Do not edit generated
files after regeneration.

- [ ] **Step 5: Run GREEN and generation drift check**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race ./internal/solvers/bridgefacilitator/... -run 'TestGenerated(AuctionExactFields|RequestIDs)|Test.*Auction|Test.*Offer' -v
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local make check-generated
```

Expected: exact-field and generated request-builder fixtures preserve values beyond 2^53, existing 3F
tests pass, and regeneration is clean.

- [ ] **Step 6: Commit**

```bash
git add openapi/3f-bf.openapi.json api/threef internal/solvers/bridgefacilitator
git commit -m "fix(3f): preserve exact auction signature fields"
```

### Task 3: Normalize Max Rate to Exact Tenth-Basis-Points

**Files:**
- Modify: `internal/solvers/bridgefacilitator/auctionview.go`
- Modify: `internal/solvers/bridgefacilitator/strategy.go`
- Modify: `internal/solvers/bridgefacilitator/strategy_test.go`
- Modify: `internal/solvers/bridgefacilitator/strategies/types/types.go`
- Modify: `internal/solvers/bridgefacilitator/strategies/types/math.go`
- Modify: `internal/solvers/bridgefacilitator/strategies/types/math_test.go`
- Modify: `internal/solvers/bridgefacilitator/strategies/types/wire_json.go`
- Modify: `internal/solvers/bridgefacilitator/strategies/types/wire_json_test.go`
- Modify: `internal/solvers/bridgefacilitator/strategies/default/strategy.go`
- Modify: `internal/solvers/bridgefacilitator/strategies/default/strategy_test.go`
- Modify: `docs/strategy-plan.md`
- Modify: `docs/3F-PLAN.md`

**Interfaces:**
- Replaces: `AuctionSnapshot.MaxRateBps float64` with `MaxRateDeciBps *big.Int`.
- Produces: webhook JSON `"maxRateBps":"50.5"`.
- Produces: `ExpectedReturn(principal, maxRateDeciBps *big.Int) *big.Int` using denominator `100_000`.

- [ ] **Step 1: Add failing exact conversion and arithmetic tests**

Cover `50.1`, `50.5`, invalid `50.55`, NaN/Inf/negative, and a principal larger than 2^53:

```go
func TestExpectedReturnUsesExactDeciBps(t *testing.T) {
	principal := mustBig(t, "900719925474099300000")
	got := ExpectedReturn(principal, big.NewInt(501))
	want := new(big.Int).Quo(new(big.Int).Mul(principal, big.NewInt(501)), big.NewInt(100_000))
	if got.Cmp(want) != 0 { t.Fatalf("return = %s, want %s", got, want) }
}
```

Add a wire test requiring `"maxRateBps":"50.5"`, not a JSON number, and an eligibility edge where `499` deci-bps fails a 50-bps floor while `500` passes.

- [ ] **Step 2: Run RED**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test ./internal/solvers/bridgefacilitator/... -run 'Test.*(DeciBps|Exact|RateWire|YieldFloor)' -v
```

Expected: old float fields/functions fail compilation or produce the old numeric wire shape.

- [ ] **Step 3: Normalize the generated double once**

Replace `maxRateBps` with:

```go
func (a auctionView) maxRateDeciBps() (*big.Int, bool) {
	r, ok := a.dto.GetMaxRateOk()
	if !ok || r == nil { return nil, false }
	text := strconv.FormatFloat(*r, 'f', -1, 64)
	rate, ok := new(big.Rat).SetString(text)
	if !ok || rate.Sign() < 0 { return nil, false }
	rate.Mul(rate, big.NewRat(10, 1))
	if rate.Denom().Cmp(big.NewInt(1)) != 0 { return nil, false }
	return new(big.Int).Set(rate.Num()), true
}
```

Carry `*big.Int` through `buildAuctionSnapshot` and `AuctionSnapshot`, cloning at ownership boundaries.

- [ ] **Step 4: Replace float money math**

Implement:

```go
var rateDenominatorDeciBps = big.NewInt(100_000)

func ExpectedReturn(principal, rateDeciBps *big.Int) *big.Int {
	if principal == nil || rateDeciBps == nil { return new(big.Int) }
	return new(big.Int).Quo(new(big.Int).Mul(principal, rateDeciBps), rateDenominatorDeciBps)
}
```

Eligibility becomes:

```go
floor := new(big.Int).Mul(st.snapshot.MinYieldBps, big.NewInt(10))
if auction.MaxRateDeciBps.Cmp(floor) < 0 { continue }
```

Remove `RateDenominatorBps`, `BpsToFloat`, and all `big.Float` use from the 3F strategy path.

- [ ] **Step 5: Emit an exact decimal string to webhooks**

Use:

```go
func formatDeciBps(n *big.Int) string {
	if n == nil { return "" }
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(n, big.NewInt(10), r)
	if r.Sign() == 0 { return q.String() }
	return q.String() + "." + r.String()
}
```

Change the wire field to `MaxRateBps string` and populate it with this helper.

- [ ] **Step 6: Run GREEN, scan for floats, and commit**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race ./internal/solvers/bridgefacilitator/... -v
! rg -n 'MaxRateBps\s+float|big\.Float|BpsToFloat' internal/solvers/bridgefacilitator
git add internal/solvers/bridgefacilitator docs/strategy-plan.md docs/3F-PLAN.md
git commit -m "fix(3f): calculate offers with exact rates"
```

Expected: exact boundary, math, strategy, and wire tests pass; no money-facing float remains after normalization.

### Task 4: Support Salted and Unsalted Offer Domains

**Files:**
- Modify: `internal/solvers/bridgefacilitator/eip712.go`
- Modify: `internal/solvers/bridgefacilitator/eip712_test.go`
- Modify: `internal/solvers/bridgefacilitator/offer.go`
- Modify: `internal/solvers/bridgefacilitator/strategy_test.go`

**Interfaces:**
- Produces: `type OfferDomain struct { Name, Version string; ChainID *big.Int; VerifyingContract common.Address; Salt *common.Hash }`.
- Replaces: positional `OfferDigest` parameters with `OfferDigest(offer Offer, domain OfferDomain) common.Hash`.

- [ ] **Step 1: Add failing salted parity and validation tests**

Keep the current unsalted golden unchanged. Add independent salted and unsalted `apitypes` parity
cases whose chain ID is beyond float64's exact-integer range:

```go
salt := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
domain := OfferDomain{
	Name: "request-8185", Version: "1", ChainID: big.NewInt(9_007_199_254_740_993),
	VerifyingContract: request, Salt: &salt,
}
got := OfferDigest(offer, domain)
```

Build the independent salted `apitypes.TypedDataDomain` with `Salt: salt.Hex()` and assert equal
digest. Repeat with `Salt: nil` at the same large chain ID for unsalted parity. Add offer-building
cases for omitted/null salt (unsalted), valid salt, and malformed/31-byte salt rejected before signer
invocation.

- [ ] **Step 2: Run RED**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test ./internal/solvers/bridgefacilitator -run 'TestOfferDigest.*Salt|TestBuildSignedOffer.*Salt' -v
```

Expected: `OfferDomain` and generated salt accessors are unused/not supported, so tests fail.

- [ ] **Step 3: Implement conditional domain type hashes**

Define:

```go
const unsaltedDomainType = "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"
const saltedDomainType = "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)"

type OfferDomain struct {
	Name string
	Version string
	ChainID *big.Int
	VerifyingContract common.Address
	Salt *common.Hash
}
```

`domainSeparator` selects the matching type hash, appends name/version/chain/address, and appends the salt word only when non-nil. Refactor all callers to the value object.

- [ ] **Step 4: Parse the generated optional salt fail-closed**

In offer-domain construction:

```go
var salt *common.Hash
if raw, ok := domain.GetSaltOk(); ok && raw != nil {
	b, err := hexutil.Decode(*raw)
	if err != nil || len(b) != common.HashLength {
		return threef.CreateOfferDto{}, errors.Errorf("auction %v: invalid EIP-712 domain salt", auction.Id)
	}
	h := common.BytesToHash(b)
	salt = &h
}
```

Build and pass the complete value:

```go
offerDomain := OfferDomain{
	Name:              *domainName,
	Version:           domainVersion,
	ChainID:           chainID,
	VerifyingContract: offer.Request,
	Salt:              salt,
}
digest := OfferDigest(signedOffer, offerDomain)
```

- [ ] **Step 5: Run GREEN and commit**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race ./internal/solvers/bridgefacilitator -run 'TestOfferDigest|TestBuildSignedOffer' -v
git add internal/solvers/bridgefacilitator/eip712.go internal/solvers/bridgefacilitator/eip712_test.go internal/solvers/bridgefacilitator/offer.go internal/solvers/bridgefacilitator/strategy_test.go
git commit -m "fix(3f): sign salted EIP-712 offer domains"
```

Expected: unsalted golden remains identical; salted parity and malformed-salt rejection pass.

### Task 5: Characterize Full Offer and Redemption Boundaries

**Files:**
- Create: `internal/solvers/bridgefacilitator/fullpath_test.go`
- Modify: `internal/solvers/bridgefacilitator/chainreader_test.go`
- Modify: `internal/solvers/bridgefacilitator/apiclient_test.go`
- Modify: `docs/3F-PLAN.md`

**Interfaces:**
- Consumes: real generated 3F client/bindings, exact rates/salt, configured TTL, shared transaction-result state machine.
- Produces: hermetic auction → Multicall → strategy → signed POST and redeem-read → calldata characterization tests.

- [ ] **Step 1: Add an end-to-end offer-path characterization test**

Build one `httptest.Server` that:

1. serves `GET /v1/auction?domain=true` with one open auction at `50.5` bps and a salt;
2. captures `POST /v1/offer`;
3. rejects any maker that is not lowercase.

Build a JSON-RPC test server that decodes real Multicall3 `aggregate3` calldata and returns encoded `fundable`, limits, signer, vault, and collateral results through generated bindings. Run one solver discovery pass with a fixed clock and a production local signer. Assert the captured DTO has:

```go
if got.Amount != "1000000000" || got.ExpectedReturn != "5050000" {
	t.Fatalf("offer amounts = %s/%s", got.Amount, got.ExpectedReturn)
}
if got.Expiration != strconv.FormatInt(fixedNow.Add(cfg.Intervals.OfferTTL).Unix(), 10) {
	t.Fatalf("expiration = %s", got.Expiration)
}
```

Recover/verify the signature against `OfferDigest` with the captured values and salted domain. Assert the offer tracker remains empty on a forced HTTP 500 and records exactly one entry after 2xx.

- [ ] **Step 2: Run the integrated characterization**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test ./internal/solvers/bridgefacilitator -run TestFullOfferPath -v
```

Expected: PASS because Tasks 1-4 already delivered exact rate, salt, TTL, and tracker behavior. This is
a GREEN boundary characterization, not a second RED transition. If it exposes an integration defect,
add the smallest regression assertion at the owning helper first, then correct that helper.

- [ ] **Step 3: Keep the characterization on production paths**

Use the existing `discoverAndOffer`, real `apiClient`, `reader`, generated bindings, and signer. Do
not add test-only production methods, a new discovery seam, or duplicate business logic. Keep all
JSON-RPC response construction in `_test.go` helpers.

- [ ] **Step 4: Characterize redemption calldata and batching**

Add a test whose RPC responses drive `requestsLength`, `requests(i)`, and `canWithdraw`, with more ready requests than `RedeemBatchSize`. Run the real redemption path through a real txmanager fake backend and decode the submitted adapter multicall. Assert:

```go
if len(decodedFinalizeCalls) != cfg.RedeemBatchSize {
	t.Fatalf("finalize calls = %d, want %d", len(decodedFinalizeCalls), cfg.RedeemBatchSize)
}
```

Assert request IDs/order match the readable prefix and an ambiguous/unresolved transaction result is suppressed until on-chain reconciliation, per the transaction-supervision plan.

- [ ] **Step 5: Run GREEN**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race ./internal/solvers/bridgefacilitator -run 'TestFullOfferPath|TestRedeem.*Boundary|TestChainReader' -v
```

Expected: full offer and redeem boundary tests pass without network access.

- [ ] **Step 6: Update 3F architecture/TODO documentation and commit**

Update `docs/3F-PLAN.md` to describe exact deci-bps, optional salt, configured offer TTL, generated-client response limits, explicit transaction outcomes, and the new characterization coverage. Mark only completed selected findings done.

```bash
git add internal/solvers/bridgefacilitator/fullpath_test.go internal/solvers/bridgefacilitator/chainreader_test.go internal/solvers/bridgefacilitator/apiclient_test.go docs/3F-PLAN.md
git commit -m "test(3f): characterize offer and redemption paths"
```

### Task 6: Verify 3F Hardening

**Files:**
- Verify only.

**Interfaces:**
- Produces: a clean, generated, exact 3F implementation ready for whole-branch review.

- [ ] **Step 1: Run 3F packages, generation, build, and lint**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local golangci-lint run --fix
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race -cover ./internal/solvers/bridgefacilitator/...
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local make check-generated
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go build ./...
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local golangci-lint run
```

Expected: every command exits 0 and generated output is clean.

- [ ] **Step 2: Scan the exactness and scope invariants**

```bash
! rg -n 'MaxRateBps\s+float|NullableFloat32|big\.Float|BpsToFloat' internal/solvers/bridgefacilitator
git status --short
```

Expected: no money-facing float artifacts in handwritten 3F code and no uncommitted changes.
