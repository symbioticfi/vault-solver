# RFQ Correctness and Bounded-State Hardening Implementation Plan

> **Public-port status:** This is the source-branch implementation record. The RFQ code and behavioral
> documentation were ported, but the private `.github/chart/**` deployment files referenced by literal
> steps below are intentionally absent from the public repository. Do not recreate or edit those paths;
> [`../../RFQ-PLAN.md`](../../RFQ-PLAN.md), the README, and `config/rfq.example.yaml` are authoritative.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make RFQ order execution bind to the exact signed order, fail closed on unsafe configuration and adapter state, keep the quote-plan cache amortized O(1), and characterize the RFQ trust boundaries with regression tests.

**Architecture:** Keep all behavior inside `internal/solvers/rfq` and its default strategy. The generated backend remains a transport boundary; handwritten code selects the exact requested row and then treats the ABI-decoded signed order as authoritative, while small injected read interfaces let tests exercise ABI-shaped Multicall results without changing production wiring. Cache cleanup remains opportunistic under the existing mutex, avoiding another goroutine or lifecycle obligation.

**Tech Stack:** Go 1.26.5, go-ethereum ABI types and generated abigen v2 bindings, generated RFQ OpenAPI client, `github.com/go-errors/errors`, Foundry-sourced ABIs, table-driven Go tests, race detector, golangci-lint.

## Global Constraints

- Execute this plan after the toolchain/generation, generic runtime-safety, and transaction-lifecycle changesets from `docs/superpowers/specs/2026-07-09-findings-2-20-hardening-design.md` have landed.
- Run every Go command with Go 1.26.5 (`GOTOOLCHAIN=go1.26.5`); the module language directive remains `go 1.26`.
- Preserve the generic-framework/integration boundary: every production change in this plan stays under `internal/solvers/rfq/`.
- Preserve the public YAML shape except for the intentional security tightening that rejects `executor: 0x0000000000000000000000000000000000000000`.
- Do not introduce a database, a background cache-cleanup goroutine, or a new dependency.
- Use `github.com/go-errors/errors` for new errors; do not use `fmt.Errorf`.
- Generated Go under `api/` is never hand-edited. This plan does not require an RFQ schema or generated-client change.
- Preserve transaction-lifecycle semantics established by the preceding changeset: an unresolved or ambiguously broadcast fill remains submitted for backend reconciliation and must not be re-armed as a definite pre-broadcast rejection.
- Finding 1 remains excluded: do not add workflow digest pins or container base-image digest pins.
- Do not deploy, push, or open a pull request while executing this plan.
- Every changed production behavior lands with its focused test and synchronized documentation in an independently reviewable Conventional Commit.

---

## File Map

- Modify `internal/solvers/rfq/backend.go`: select exactly one requested order ID from generated `/orders` responses.
- Modify `internal/solvers/rfq/backend_test.go`: characterize empty, reordered, missing, and duplicate order responses.
- Modify `internal/solvers/rfq/execution.go`: bind local identity, decode the signed order first, validate signed terms using `big.Int`, compare optional projections, and construct strategy/fill inputs only from the decoded order.
- Modify `internal/solvers/rfq/execution_test.go`: reject identity/projection mismatches and characterize the complete decoded-order-to-calldata path.
- Modify `internal/solvers/rfq/order_test.go`: unit-test signed-order validation, including deadlines outside `int64`.
- Modify `internal/solvers/rfq/config.go`: reject a zero executor address.
- Modify `internal/solvers/rfq/config_test.go`: pin zero-executor rejection.
- Modify `internal/solvers/rfq/chainreader.go`: inject narrow Multicall/decimals interfaces and require a successful, decodable `paused() == false` result.
- Create `internal/solvers/rfq/chainreader_test.go`: exercise ABI-shaped inventory and authorization success/failure matrices.
- Modify `internal/solvers/rfq/strategies/default/strategy.go`: schedule bounded periodic sweeps and lazily delete requested expired entries.
- Modify `internal/solvers/rfq/strategies/default/strategy_test.go`: pin sweep cadence, lazy deletion, and concurrent behavior.
- Modify `docs/RFQ-PLAN.md`: describe exact-order selection, signed-order authority, current adapter terminology, internal-only discounts, and bounded cache behavior.
- Modify `README.md`: keep the operator-facing internal-mode description consistent with the internal-only discounts endpoint.
- Modify `config/rfq.example.yaml`: replace the stale public-discounts claim.
- Modify `.github/chart/mainnet.yaml`: replace the stale public-discounts claim.
- Modify `.github/chart/sepolia.yaml`: replace the stale public-discounts claim.
- Modify `.github/chart/hoodi.yaml`: replace the stale public-discounts claim.

## Finding Coverage

- Finding 4: Tasks 1-2 bind the generated response to one requested row and bind execution/calldata to the decoded signed order.
- Finding 9: Tasks 3-4 reject a zero executor and fail closed when pause state is unavailable.
- Finding 15: Task 5 makes cache insertion amortized O(1) while retaining bounded expiry cleanup.
- Finding 18 (RFQ portion): Tasks 1, 2, and 4 characterize exact selection, ABI-decoded fill calldata, and ABI-shaped Multicall boundaries.
- Finding 20 (RFQ portion): Task 6 reconciles RFQ plan, README, example, and chart terminology with production behavior.

---

### Task 1: Select the Exact Requested Backend Order

**Files:**
- Modify: `internal/solvers/rfq/backend.go:112-134,196-201`
- Modify: `internal/solvers/rfq/backend_test.go`

**Interfaces:**
- Consumes: `[]backendOrder` from `ordersFromResponse` and the exact `orderID` already sent as the generated client's `orderId` query parameter.
- Produces: `func selectOrder(orders []backendOrder, orderID string) (*backendOrder, error)`; both `getExecutableOrder` and `getOrder` return only this exact match.

- [ ] **Step 1: Add the exact-selection table test**

Append this test to `internal/solvers/rfq/backend_test.go`:

```go
func TestSelectOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		orders  []backendOrder
		orderID string
		wantID  string
		wantErr string
	}{
		{name: "empty response", orderID: "wanted"},
		{
			name: "selects exact id regardless of order",
			orders: []backendOrder{
				{OrderID: "other", QuoteID: "q-other"},
				{OrderID: "wanted", QuoteID: "q-wanted"},
			},
			orderID: "wanted",
			wantID:  "wanted",
		},
		{
			name:    "nonempty response without requested id",
			orders:  []backendOrder{{OrderID: "other"}},
			orderID: "wanted",
			wantErr: `response for order "wanted" contained 1 non-matching row`,
		},
		{
			name: "duplicate requested id",
			orders: []backendOrder{
				{OrderID: "wanted", QuoteID: "q1"},
				{OrderID: "wanted", QuoteID: "q2"},
			},
			orderID: "wanted",
			wantErr: `response contained duplicate order "wanted"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := selectOrder(tc.orders, tc.orderID)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("order = %+v, want nil on ambiguity", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectOrder: %v", err)
			}
			if tc.wantID == "" {
				if got != nil {
					t.Fatalf("order = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.OrderID != tc.wantID {
				t.Fatalf("order = %+v, want id %q", got, tc.wantID)
			}
		})
	}
}
```

Add `strings` to the test imports.

- [ ] **Step 2: Run the test and observe the RED failure**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run '^TestSelectOrder$' -count=1
```

Expected: build failure containing `undefined: selectOrder`.

- [ ] **Step 3: Implement exact selection and route both lookups through it**

Replace `first` in `internal/solvers/rfq/backend.go` with:

```go
func selectOrder(orders []backendOrder, orderID string) (*backendOrder, error) {
	var match *backendOrder
	for i := range orders {
		if orders[i].OrderID != orderID {
			continue
		}
		if match != nil {
			return nil, errors.Errorf("response contained duplicate order %q", orderID)
		}
		match = &orders[i]
	}
	if match != nil || len(orders) == 0 {
		return match, nil
	}
	return nil, errors.Errorf(
		"response for order %q contained %d non-matching row(s)", orderID, len(orders))
}
```

Update the two callers without changing their HTTP query shapes:

```go
func (c *backendClient) getExecutableOrder(ctx context.Context, orderID, filler string) (*backendOrder, error) {
	req := c.api.RFQAPI.ApiV1OrdersGet(ctx).
		OrderId(orderID).
		Filler(filler).
		OrderStatus("open")
	resp, httpResp, err := req.Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("backend: get executable order: %w", err)
	}
	order, err := selectOrder(ordersFromResponse(resp), orderID)
	if err != nil {
		return nil, errors.Errorf("backend: get executable order: %w", err)
	}
	return order, nil
}

func (c *backendClient) getOrder(ctx context.Context, orderID string) (*backendOrder, error) {
	resp, httpResp, err := c.api.RFQAPI.ApiV1OrdersGet(ctx).OrderId(orderID).Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("backend: get order: %w", err)
	}
	order, err := selectOrder(ordersFromResponse(resp), orderID)
	if err != nil {
		return nil, errors.Errorf("backend: get order: %w", err)
	}
	return order, nil
}
```

- [ ] **Step 4: Run focused backend tests**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run '^(TestSelectOrder|TestBackendClient_)' -count=1
```

Expected: PASS. Existing path/query assertions must remain unchanged.

- [ ] **Step 5: Commit exact backend selection**

```bash
git add internal/solvers/rfq/backend.go internal/solvers/rfq/backend_test.go
git commit -m "fix(rfq): select backend orders by exact id"
```

---

### Task 2: Make the ABI-Decoded Signed Order Authoritative

**Files:**
- Modify: `internal/solvers/rfq/execution.go:34-42,132-225,445-503`
- Modify: `internal/solvers/rfq/execution_test.go`
- Modify: `internal/solvers/rfq/order_test.go`

**Interfaces:**
- Consumes: `executor.IReactorOrder` from `decodeOrder`, configured executor address, local `orderRecord`, and optional projections retained in `backendOrder`.
- Produces: `func validateSignedOrder(order executor.IReactorOrder, configuredExecutor common.Address, now time.Time) (common.Address, *big.Int, error)` and `func validateBackendProjection(projected backendOrder, order executor.IReactorOrder) error`.
- Produces: `executable` containing only the quote ID, encoded signed order, protocol signature, and a copy of the backend projection; strategy input and fill calldata use decoded order fields exclusively.

- [ ] **Step 1: Add pure signed-order validation tests**

Append the following to `internal/solvers/rfq/order_test.go` and add `strings` and `time` to its imports:

```go
func TestValidateSignedOrder(t *testing.T) {
	t.Parallel()
	executorAddr := common.HexToAddress("0x0000000000000000000000000000000000000010")
	now := time.Unix(1_000, 0)

	tests := []struct {
		name    string
		mutate  func(*executor.IReactorOrder)
		wantErr string
	}{
		{name: "valid"},
		{
			name: "different decoded filler",
			mutate: func(o *executor.IReactorOrder) {
				o.Filler = common.HexToAddress("0x00000000000000000000000000000000000000ff")
			},
			wantErr: "decoded order filler",
		},
		{
			name: "nil input amount",
			mutate: func(o *executor.IReactorOrder) {
				o.Request.AmountIn = nil
			},
			wantErr: "invalid input amount",
		},
		{
			name: "expired deadline",
			mutate: func(o *executor.IReactorOrder) {
				o.Request.Deadline = big.NewInt(now.Unix())
			},
			wantErr: "deadline has passed",
		},
		{
			name: "no outputs",
			mutate: func(o *executor.IReactorOrder) {
				o.Outputs = nil
			},
			wantErr: "no outputs",
		},
		{
			name: "mixed output tokens",
			mutate: func(o *executor.IReactorOrder) {
				o.Outputs = append(o.Outputs, executor.IReactorOutput{
					Token:     common.HexToAddress("0x00000000000000000000000000000000000000ee"),
					Amount:    big.NewInt(1),
					Recipient: o.Outputs[0].Recipient,
				})
			},
			wantErr: "multiple output tokens",
		},
		{
			name: "nil output amount",
			mutate: func(o *executor.IReactorOrder) {
				o.Outputs[0].Amount = nil
			},
			wantErr: "output 0 has invalid amount",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			order := sampleOrder()
			if tc.mutate != nil {
				tc.mutate(&order)
			}
			token, required, err := validateSignedOrder(order, executorAddr, now)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateSignedOrder: %v", err)
			}
			if token != tOut || required.Cmp(big.NewInt(900000)) != 0 {
				t.Fatalf("token/required = %s/%s, want %s/900000", token, required, tOut)
			}
		})
	}
}

func TestValidateSignedOrder_LargeUint256DeadlineDoesNotTruncate(t *testing.T) {
	t.Parallel()
	order := sampleOrder()
	order.Request.Deadline = new(big.Int).Lsh(big.NewInt(1), 70)

	_, _, err := validateSignedOrder(
		order,
		common.HexToAddress("0x0000000000000000000000000000000000000010"),
		time.Unix(1_000, 0),
	)
	if err != nil {
		t.Fatalf("large uint256 deadline rejected after narrowing: %v", err)
	}
}
```

- [ ] **Step 2: Run signed-order tests and observe the RED failure**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run '^TestValidateSignedOrder' -count=1
```

Expected: build failure containing `undefined: validateSignedOrder`.

- [ ] **Step 3: Implement exact signed-order validation**

Add this helper to `internal/solvers/rfq/execution.go` near the existing executable helpers:

```go
func validateSignedOrder(
	order executor.IReactorOrder,
	configuredExecutor common.Address,
	now time.Time,
) (common.Address, *big.Int, error) {
	if order.Filler != configuredExecutor {
		return common.Address{}, nil, errors.Errorf(
			"decoded order filler %s does not match configured executor %s",
			order.Filler.Hex(), configuredExecutor.Hex())
	}
	if order.Request.TokenIn == (common.Address{}) {
		return common.Address{}, nil, errors.New("decoded order has zero input token")
	}
	if order.Request.AmountIn == nil || order.Request.AmountIn.Sign() <= 0 {
		return common.Address{}, nil, errors.New("decoded order has invalid input amount")
	}
	if order.Request.Deadline == nil || order.Request.Deadline.Cmp(big.NewInt(now.Unix())) <= 0 {
		return common.Address{}, nil, errors.New("order deadline has passed")
	}
	if len(order.Outputs) == 0 {
		return common.Address{}, nil, errors.New("decoded order has no outputs")
	}

	token := order.Outputs[0].Token
	if token == (common.Address{}) {
		return common.Address{}, nil, errors.New("decoded order has zero output token")
	}
	required := new(big.Int)
	for i := range order.Outputs {
		out := order.Outputs[i]
		if out.Token != token {
			return common.Address{}, nil, errors.New("decoded order has multiple output tokens")
		}
		if out.Amount == nil || out.Amount.Sign() <= 0 {
			return common.Address{}, nil, errors.Errorf("decoded order output %d has invalid amount", i)
		}
		required.Add(required, out.Amount)
	}
	return token, required, nil
}
```

Do not call `Int64`, `Uint64`, `SetInt64`, or `SetUint64` on any decoded deadline or amount.

- [ ] **Step 4: Run signed-order tests and observe GREEN**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run '^TestValidateSignedOrder' -count=1
```

Expected: PASS, including the `2^70` deadline case.

- [ ] **Step 5: Add projection-binding and end-to-end calldata tests**

Add these helpers to `internal/solvers/rfq/execution_test.go`:

```go
type recordingFillStrategy struct {
	input types.FillInput
	plan  *types.FillPlan
}

func (s *recordingFillStrategy) DecideQuote(
	context.Context,
	types.QuoteInput,
) (types.QuoteOutput, error) {
	return types.QuoteOutput{}, nil
}

func (s *recordingFillStrategy) BuildFillPlan(
	_ context.Context,
	input types.FillInput,
) (*types.FillPlan, error) {
	s.input = input
	return s.plan, nil
}

func setExecutableOrder(t *testing.T, be *fakeBackend, order executor.IReactorOrder) {
	t.Helper()
	encoded, err := orderTupleArgs.Pack(order)
	if err != nil {
		t.Fatalf("pack order: %v", err)
	}
	be.executable.EncodedOrder = strPtr(hexutil.Encode(encoded))
}

type decodedFill struct {
	order             executor.IReactorOrder
	protocolSignature []byte
	swaps             []executor.IReactorSwapInput
	discountSwaps     []executor.IReactorDiscountSwapInput
	executorData      []byte
}

func unpackSentFill(t *testing.T, data []byte) decodedFill {
	t.Helper()
	if len(data) < 4 {
		t.Fatal("fill calldata is missing")
	}
	method, err := executorABI.MethodById(data[:4])
	if err != nil {
		t.Fatalf("find fill method: %v", err)
	}
	values, err := method.Inputs.Unpack(data[4:])
	if err != nil {
		t.Fatalf("unpack fill calldata: %v", err)
	}
	return decodedFill{
		order: *abi.ConvertType(
			values[0], new(executor.IReactorOrder),
		).(*executor.IReactorOrder),
		protocolSignature: *abi.ConvertType(values[1], new([]byte)).(*[]byte),
		swaps: *abi.ConvertType(
			values[2], new([]executor.IReactorSwapInput),
		).(*[]executor.IReactorSwapInput),
		discountSwaps: *abi.ConvertType(
			values[3], new([]executor.IReactorDiscountSwapInput),
		).(*[]executor.IReactorDiscountSwapInput),
		executorData: *abi.ConvertType(values[4], new([]byte)).(*[]byte),
	}
}
```

Add `bytes`, `github.com/ethereum/go-ethereum/accounts/abi`, and `github.com/symbioticfi/vault-solver/api/bindings/rfq/executor` to the test imports, then add:

```go
func TestExecution_UsesSignedOrderTermsAndCalldata(t *testing.T) {
	st, be := fillFixtures(t)
	// Simulate an executable row that omits optional redundant projections. The signed tuple remains complete.
	be.executable.Filler = nil
	be.executable.Deadline = nil
	be.executable.Outputs = nil

	txm := &fakeTxm{result: txmanager.Result{Hash: common.HexToHash("0xdead")}}
	e := newExec(t, st, be, txm)
	recording := &recordingFillStrategy{plan: baseFillPlan()}
	e.strategy = recording

	e.syncOnce(t.Context())

	if recording.input.TokenIn != tIn || recording.input.TokenOut != tOut {
		t.Fatalf("strategy tokens = %s/%s, want %s/%s", recording.input.TokenIn, recording.input.TokenOut, tIn, tOut)
	}
	if recording.input.AmountIn.Cmp(big.NewInt(1_000000000000000000)) != 0 ||
		recording.input.RequiredAmountOut.Cmp(big.NewInt(900000)) != 0 {
		t.Fatalf("strategy amounts = %s/%s", recording.input.AmountIn, recording.input.RequiredAmountOut)
	}

	sent := unpackSentFill(t, txm.lastData)
	sentOrder := sent.order
	wantOrder := sampleOrder()
	if sentOrder.Filler != wantOrder.Filler ||
		sentOrder.Request.TokenIn != wantOrder.Request.TokenIn ||
		sentOrder.Request.AmountIn.Cmp(wantOrder.Request.AmountIn) != 0 ||
		len(sentOrder.Outputs) != 1 ||
		sentOrder.Outputs[0].Amount.Cmp(wantOrder.Outputs[0].Amount) != 0 {
		t.Fatalf("sent signed order = %+v, want %+v", sentOrder, wantOrder)
	}
	if !bytes.Equal(sent.protocolSignature, []byte{0xab, 0xcd}) {
		t.Fatalf("protocol signature = %x, want abcd", sent.protocolSignature)
	}
	if len(sent.swaps) != 1 || sent.swaps[0].Adapter != vlt ||
		sent.swaps[0].Swap.TokenIn != wantOrder.Request.TokenIn ||
		sent.swaps[0].Swap.AmountIn.Cmp(wantOrder.Request.AmountIn) != 0 ||
		sent.swaps[0].Swap.AmountOut.Cmp(wantOrder.Outputs[0].Amount) != 0 {
		t.Fatalf("direct swaps = %+v, want one signed-order-bound leg", sent.swaps)
	}
	if len(sent.discountSwaps) != 0 || !bytes.Equal(sent.executorData, emptyExecutorData) {
		t.Fatalf("discount swaps/executor data = %+v/%x", sent.discountSwaps, sent.executorData)
	}
}

func TestExecution_RejectsBackendProjectionMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*backendOrder)
		wantErr string
	}{
		{
			name: "filler",
			mutate: func(bo *backendOrder) {
				bo.Filler = strPtr("0x00000000000000000000000000000000000000ff")
			},
			wantErr: "backend filler does not match decoded order",
		},
		{
			name: "output amount",
			mutate: func(bo *backendOrder) {
				bo.Outputs[0].Amount = "899999"
			},
			wantErr: "backend output 0 does not match decoded order",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			tc.mutate(be.executable)
			txm := &fakeTxm{}
			e := newExec(t, st, be, txm)

			e.syncOnce(t.Context())

			rec := st.order("o1")
			if rec == nil || rec.Status != statusFailed || !strings.Contains(rec.LastError, tc.wantErr) {
				t.Fatalf("record = %+v, want failed with %q", rec, tc.wantErr)
			}
			if txm.lastData != nil {
				t.Fatal("projection mismatch must fail before transaction submission")
			}
		})
	}
}

func TestExecution_RejectsDecodedFillerMismatch(t *testing.T) {
	t.Parallel()
	st, be := fillFixtures(t)
	order := sampleOrder()
	order.Filler = common.HexToAddress("0x00000000000000000000000000000000000000ff")
	setExecutableOrder(t, be, order)
	// Remove the projection so the rejection is demonstrably based on the signed tuple itself.
	be.executable.Filler = nil
	be.executable.Outputs = nil
	txm := &fakeTxm{}
	e := newExec(t, st, be, txm)

	e.syncOnce(t.Context())

	rec := st.order("o1")
	if rec == nil || rec.Status != statusFailed ||
		!strings.Contains(rec.LastError, "decoded order filler") {
		t.Fatalf("record = %+v, want decoded-filler failure", rec)
	}
	if txm.lastData != nil {
		t.Fatal("decoded filler mismatch must fail before transaction submission")
	}
}

func TestExecution_RejectsLocalIdentityMismatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*backendOrder)
	}{
		{name: "order id", mutate: func(bo *backendOrder) { bo.OrderID = "different" }},
		{name: "quote id", mutate: func(bo *backendOrder) { bo.QuoteID = "different" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			tc.mutate(be.executable)
			txm := &fakeTxm{}
			e := newExec(t, st, be, txm)
			e.syncOnce(t.Context())
			if txm.lastData != nil {
				t.Fatal("identity mismatch must fail before transaction submission")
			}
		})
	}
}
```

If the preceding transaction-lifecycle changeset changed the fields needed to construct a successful `txmanager.Result`, use that changeset's confirmed-result constructor/fields in the two happy-path tests; do not weaken the assertions or bypass `txSender.Send`.

- [ ] **Step 6: Run the new execution tests and observe RED behavior**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run '^TestExecution_(UsesSignedOrderTermsAndCalldata|RejectsBackendProjectionMismatch|RejectsDecodedFillerMismatch|RejectsLocalIdentityMismatch)$' -count=1
```

Expected before implementation: at least `UsesSignedOrderTermsAndCalldata` fails because `executableFromBackend` requires omitted projections; mismatch tests either submit calldata or do not record the expected fail-closed reason.

- [ ] **Step 7: Retain projections only for equality checks**

Change `executable` in `internal/solvers/rfq/execution.go` to:

```go
type executable struct {
	quoteID      string
	encodedOrder []byte
	signature    []byte
	projected    backendOrder
}
```

Replace `executableFromBackend` with:

```go
func executableFromBackend(bo *backendOrder) (*executable, error) {
	if bo.EncodedOrder == nil || bo.ProtocolSignature == nil {
		return nil, errors.New("executable order payload incomplete")
	}
	encoded, err := hexutil.Decode(*bo.EncodedOrder)
	if err != nil {
		return nil, errors.Errorf("decode encodedOrder: %w", err)
	}
	sig, err := hexutil.Decode(*bo.ProtocolSignature)
	if err != nil {
		return nil, errors.Errorf("decode protocolSignature: %w", err)
	}
	return &executable{
		quoteID:      bo.QuoteID,
		encodedOrder: encoded,
		signature:    sig,
		projected:    *bo,
	}, nil
}
```

Add this equality checker beside `validateSignedOrder`:

```go
func validateBackendProjection(projected backendOrder, order executor.IReactorOrder) error {
	if projected.Filler != nil {
		if !common.IsHexAddress(*projected.Filler) ||
			common.HexToAddress(*projected.Filler) != order.Filler {
			return errors.New("backend filler does not match decoded order")
		}
	}
	if projected.Outputs == nil {
		return nil
	}
	if len(projected.Outputs) != len(order.Outputs) {
		return errors.New("backend outputs do not match decoded order")
	}
	for i := range projected.Outputs {
		got := projected.Outputs[i]
		want := order.Outputs[i]
		amount, ok := new(big.Int).SetString(got.Amount, 10)
		if !ok || amount.Sign() < 0 ||
			!common.IsHexAddress(got.Token) || common.HexToAddress(got.Token) != want.Token ||
			!common.IsHexAddress(got.Recipient) || common.HexToAddress(got.Recipient) != want.Recipient ||
			want.Amount == nil || amount.Cmp(want.Amount) != 0 {
			return errors.Errorf("backend output %d does not match decoded order", i)
		}
	}
	return nil
}
```

Do not compare `backendOrder.Deadline` to the decoded deadline: the generated projection is narrower than the signed `uint256` and is not authoritative. The decoded deadline is validated entirely as `*big.Int`.

- [ ] **Step 8: Bind the fetched row to the local record**

Update `resolveExecutable` before calling `executableFromBackend`:

```go
func (e *executionService) resolveExecutable(ctx context.Context, local *orderRecord) (*executable, error) {
	bo, err := e.backend.getExecutableOrder(ctx, local.OrderID, lowerAddr(e.executor))
	if err != nil {
		return nil, err
	}
	if bo == nil {
		return nil, nil
	}
	if bo.OrderID != local.OrderID {
		return nil, errors.Errorf("backend returned order %q for requested order %q", bo.OrderID, local.OrderID)
	}
	if bo.QuoteID != local.QuoteID {
		return nil, errors.Errorf(
			"backend returned quote %q for local quote %q", bo.QuoteID, local.QuoteID)
	}
	return executableFromBackend(bo)
}
```

- [ ] **Step 9: Reorder submission around the decoded signed order**

In `submitOrder`, delete the pre-decode `exec.filler` check and replace the deadline/output block with:

```go
	order, err := decodeOrder(exec.encodedOrder)
	if err != nil {
		e.fail(orderID, "decode order: "+err.Error())
		return
	}
	outputToken, required, err := validateSignedOrder(order, e.executor, e.now())
	if err != nil {
		e.fail(orderID, err.Error())
		return
	}
	if err := validateBackendProjection(exec.projected, order); err != nil {
		e.fail(orderID, err.Error())
		return
	}
```

Delete `singleOutputToken([]backendOut)` and `sumOutputs([]backendOut)`. Keep `order.Request.TokenIn`, `order.Request.AmountIn`, `order.Outputs`, and `order.Request.Deadline` as the only sources used by `buildFillPlan`, `directSwaps`, and `encodeFill`.

- [ ] **Step 10: Run the RFQ execution and ABI tests**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run '^(TestValidateSignedOrder|TestExecution_|TestDecodeOrder_RoundTrip|TestEncodeFill_)' -count=1
```

Expected: PASS. The final calldata test must decode the exact signed order, and every mismatch test must assert that no transaction was sent.

- [ ] **Step 11: Commit signed-order authority**

```bash
git add internal/solvers/rfq/execution.go internal/solvers/rfq/execution_test.go internal/solvers/rfq/order_test.go
git commit -m "fix(rfq): bind fills to the signed order"
```

---

### Task 3: Reject a Zero Executor Address

**Files:**
- Modify: `internal/solvers/rfq/config.go:104-120`
- Modify: `internal/solvers/rfq/config_test.go:214-239`

**Interfaces:**
- Consumes: the existing shared `parse.NonZeroAddress(value, field)` helper.
- Produces: unchanged `Config.Executor common.Address`, now guaranteed nonzero after `parseConfig` succeeds.

- [ ] **Step 1: Add the zero-executor regression case**

Add this entry to the `TestParseConfig_Errors` table in `internal/solvers/rfq/config_test.go`:

```go
		"zero executor": `
backendUrl: https://x
backendSharedSecretEnv: S
executor: "0x0000000000000000000000000000000000000000"
`,
```

- [ ] **Step 2: Run the config test and observe RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run '^TestParseConfig_Errors$/zero_executor$' -count=1
```

Expected: FAIL with `expected an error for "zero executor"` because `parse.Address` currently accepts the zero address.

- [ ] **Step 3: Use the nonzero parser**

Change only this line in `parseConfig`:

```go
	executor, err := parse.NonZeroAddress(raw.Executor, "executor")
```

Do not make `reactor` newly required or nonzero in this task; it is optional and unused by this finding.

- [ ] **Step 4: Run all RFQ config tests**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run '^TestParseConfig_' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit config hardening**

```bash
git add internal/solvers/rfq/config.go internal/solvers/rfq/config_test.go
git commit -m "fix(rfq): reject a zero executor address"
```

---

### Task 4: Fail Closed on Pause Reads and Characterize RFQ Multicalls

**Files:**
- Modify: `internal/solvers/rfq/chainreader.go:24-39,56-111`
- Create: `internal/solvers/rfq/chainreader_test.go`

**Interfaces:**
- Consumes: `chain.Call`, `chain.CallResult`, generated `LiquidLaneAdapter` Pack/Unpack helpers, and `chain.Decimals.Get`.
- Produces: `type multicallClient interface { Multicall(context.Context, []chain.Call) ([]chain.CallResult, error) }` and `type decimalsReader interface { Get(context.Context, common.Address) (int, error) }`; `*chain.Client` and `*chain.Decimals` satisfy these in production.
- Produces: inventory inclusion only when `paused()` succeeds, decodes, and returns false; authorization remains market-maker, owner, or delegated filler, with all failed/malformed reads excluded.

- [ ] **Step 1: Create ABI-shaped test doubles and a happy-path boundary test**

Create `internal/solvers/rfq/chainreader_test.go` with:

```go
package rfq

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	llbinding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

type fakeMulticallClient struct {
	responses [][]chain.CallResult
	calls     [][]chain.Call
}

func (f *fakeMulticallClient) Multicall(
	_ context.Context,
	calls []chain.Call,
) ([]chain.CallResult, error) {
	f.calls = append(f.calls, append([]chain.Call(nil), calls...))
	if len(f.responses) == 0 {
		return nil, nil
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

type fakeDecimalsReader struct {
	decimals int
	err      error
}

func (f fakeDecimalsReader) Get(context.Context, common.Address) (int, error) {
	return f.decimals, f.err
}

func adapterResult(t *testing.T, method string, values ...any) chain.CallResult {
	t.Helper()
	parsed, err := llbinding.LiquidLaneAdapterMetaData.ParseABI()
	if err != nil {
		t.Fatalf("parse LiquidLaneAdapter ABI: %v", err)
	}
	m, ok := parsed.Methods[method]
	if !ok {
		t.Fatalf("LiquidLaneAdapter ABI has no method %q", method)
	}
	data, err := m.Outputs.Pack(values...)
	if err != nil {
		t.Fatalf("pack %s output: %v", method, err)
	}
	return chain.CallResult{Success: true, ReturnData: data}
}

func inventoryResults(t *testing.T, paused chain.CallResult) []chain.CallResult {
	t.Helper()
	return []chain.CallResult{
		paused,
		adapterResult(t, "getMaxAssets", big.NewInt(1_000_000)),
		adapterResult(t, "getMaxRate", big.NewInt(1_000_000_000_000_000_000)),
	}
}

func TestReadVaultInventories_ABIBoundary(t *testing.T) {
	t.Parallel()
	adapterAddr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	asset := common.HexToAddress("0x0000000000000000000000000000000000000022")
	tokenIn := common.HexToAddress("0x0000000000000000000000000000000000000033")
	mc := &fakeMulticallClient{responses: [][]chain.CallResult{
		inventoryResults(t, adapterResult(t, "paused", false)),
	}}
	r := &reader{chain: mc, dec: fakeDecimalsReader{decimals: 6}, log: logr.Discard()}

	got, err := r.readVaultInventories(t.Context(), tokenIn, []recoveryVault{{
		Adapter: adapterAddr,
		Asset:   asset,
	}})
	if err != nil {
		t.Fatalf("readVaultInventories: %v", err)
	}
	if len(got) != 1 || got[0].Adapter != adapterAddr || got[0].Asset != asset ||
		got[0].AssetDecimals != 6 || got[0].MaxAssets.Cmp(big.NewInt(1_000_000)) != 0 {
		t.Fatalf("inventory = %+v", got)
	}
	if len(mc.calls) != 1 || len(mc.calls[0]) != readsPerAdapter {
		t.Fatalf("multicall layout = %+v", mc.calls)
	}
	wantData := [][]byte{
		llAdapter.PackPaused(),
		llAdapter.PackGetMaxAssets(tokenIn),
		llAdapter.PackGetMaxRate(tokenIn),
	}
	for i, call := range mc.calls[0] {
		if call.Target != adapterAddr || !call.AllowFailure || string(call.Data) != string(wantData[i]) {
			t.Fatalf("call %d = %+v, want target %s and selector %x", i, call, adapterAddr, wantData[i])
		}
	}
}
```

- [ ] **Step 2: Run the boundary test and observe the seam RED failure**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run '^TestReadVaultInventories_ABIBoundary$' -count=1
```

Expected: build failure because `*fakeMulticallClient` and `fakeDecimalsReader` cannot be assigned to the current concrete `*chain.Client` and `*chain.Decimals` fields.

- [ ] **Step 3: Introduce only the narrow read interfaces**

In `internal/solvers/rfq/chainreader.go`, define:

```go
type multicallClient interface {
	Multicall(context.Context, []chain.Call) ([]chain.CallResult, error)
}

type decimalsReader interface {
	Get(context.Context, common.Address) (int, error)
}
```

Change the reader fields while keeping production construction unchanged:

```go
type reader struct {
	chain multicallClient
	log   logr.Logger
	dec   decimalsReader
}

func newReader(c *chain.Client, log logr.Logger) *reader {
	return &reader{chain: c, log: log, dec: chain.NewDecimals(c)}
}
```

No solver or generic-chain API changes are needed.

- [ ] **Step 4: Run the happy-path boundary test and observe GREEN**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run '^TestReadVaultInventories_ABIBoundary$' -count=1
```

Expected: PASS, proving the exact call count, order, selectors, target, `AllowFailure`, and ABI output decoding.

- [ ] **Step 5: Add the fail-closed pause matrix**

Append:

```go
func TestReadVaultInventories_PauseReadFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		paused chain.CallResult
		want   int
	}{
		{name: "unpaused", paused: adapterResult(t, "paused", false), want: 1},
		{name: "paused", paused: adapterResult(t, "paused", true)},
		{name: "pause read reverted", paused: chain.CallResult{Success: false}},
		{name: "pause read malformed", paused: chain.CallResult{Success: true, ReturnData: []byte{0x01}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mc := &fakeMulticallClient{responses: [][]chain.CallResult{inventoryResults(t, tc.paused)}}
			r := &reader{chain: mc, dec: fakeDecimalsReader{decimals: 6}, log: logr.Discard()}
			got, err := r.readVaultInventories(t.Context(), tIn, []recoveryVault{{Adapter: vlt, Asset: tOut}})
			if err != nil {
				t.Fatalf("readVaultInventories: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("inventories = %d, want %d", len(got), tc.want)
			}
		})
	}
}

func TestReadVaultInventories_MaxAssetsAndRateFailClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(*testing.T, []chain.CallResult)
		decimalsErr error
	}{
		{name: "max assets reverted", mutate: func(_ *testing.T, r []chain.CallResult) { r[1] = chain.CallResult{Success: false} }},
		{name: "rate reverted", mutate: func(_ *testing.T, r []chain.CallResult) { r[2] = chain.CallResult{Success: false} }},
		{name: "max assets malformed", mutate: func(_ *testing.T, r []chain.CallResult) { r[1].ReturnData = []byte{0x01} }},
		{name: "rate malformed", mutate: func(_ *testing.T, r []chain.CallResult) { r[2].ReturnData = []byte{0x01} }},
		{name: "zero max assets", mutate: func(t *testing.T, r []chain.CallResult) { r[1] = adapterResult(t, "getMaxAssets", new(big.Int)) }},
		{name: "zero rate", mutate: func(t *testing.T, r []chain.CallResult) { r[2] = adapterResult(t, "getMaxRate", new(big.Int)) }},
		{name: "decimals unavailable", decimalsErr: errors.New("decimals unavailable")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			results := inventoryResults(t, adapterResult(t, "paused", false))
			if tc.mutate != nil {
				tc.mutate(t, results)
			}
			mc := &fakeMulticallClient{responses: [][]chain.CallResult{results}}
			r := &reader{
				chain: mc,
				dec:   fakeDecimalsReader{decimals: 6, err: tc.decimalsErr},
				log:   logr.Discard(),
			}
			got, err := r.readVaultInventories(t.Context(), tIn, []recoveryVault{{Adapter: vlt, Asset: tOut}})
			if err != nil {
				t.Fatalf("readVaultInventories: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("inventories = %+v, want none", got)
			}
		})
	}
}

func TestReadVaultInventories_RejectsWrongResultCount(t *testing.T) {
	t.Parallel()
	mc := &fakeMulticallClient{responses: [][]chain.CallResult{{
		adapterResult(t, "paused", false),
	}}}
	r := &reader{chain: mc, dec: fakeDecimalsReader{decimals: 6}, log: logr.Discard()}
	_, err := r.readVaultInventories(t.Context(), tIn, []recoveryVault{{Adapter: vlt, Asset: tOut}})
	if err == nil || !strings.Contains(err.Error(), "got 1 results, want 3") {
		t.Fatalf("error = %v, want result-count mismatch", err)
	}
}
```

Add `strings` and `github.com/go-errors/errors` to `chainreader_test.go` imports.

- [ ] **Step 6: Run the pause matrix and observe RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run '^TestReadVaultInventories_PauseReadFailsClosed$' -count=1
```

Expected: FAIL for `pause_read_reverted` and `pause_read_malformed`; current code includes the adapter when pause state is unknown.

- [ ] **Step 7: Require a successful, decoded unpaused state**

Replace the pause/max/rate prelude inside `readVaultInventories` with:

```go
		paused, maxA, mr := res[base], res[base+1], res[base+2]
		if !paused.Success || !maxA.Success || !mr.Success {
			continue
		}
		isPaused, pauseErr := llAdapter.UnpackPaused(paused.ReturnData)
		if pauseErr != nil || isPaused {
			continue
		}
		maxAssets, maxErr := llAdapter.UnpackGetMaxAssets(maxA.ReturnData)
		maxRate, rateErr := llAdapter.UnpackGetMaxRate(mr.ReturnData)
		if maxErr != nil || rateErr != nil {
			continue
		}
```

Keep the existing positive-value and decimals checks directly after this block.

- [ ] **Step 8: Add authorization boundary characterization**

Append a table that drives `readPermissionedVaultInventories` through its exact three-batch maximum. Use this complete test:

```go
func TestReadPermissionedVaultInventories_AuthorizationBoundary(t *testing.T) {
	t.Parallel()
	executorAddr := common.HexToAddress("0x0000000000000000000000000000000000000044")
	marketMaker := common.HexToAddress("0x0000000000000000000000000000000000000055")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000066")

	tests := []struct {
		name        string
		marketMaker common.Address
		owner       common.Address
		delegated   *bool
		want        int
	}{
		{name: "market maker is executor", marketMaker: executorAddr, owner: owner, want: 1},
		{name: "owner is executor", marketMaker: marketMaker, owner: executorAddr, want: 1},
		{name: "delegated filler", marketMaker: marketMaker, owner: owner, delegated: boolPtr(true), want: 1},
		{name: "not delegated", marketMaker: marketMaker, owner: owner, delegated: boolPtr(false)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			responses := [][]chain.CallResult{
				inventoryResults(t, adapterResult(t, "paused", false)),
				{
					adapterResult(t, "marketMaker", tc.marketMaker),
					adapterResult(t, "owner", tc.owner),
				},
			}
			if tc.delegated != nil {
				responses = append(responses, []chain.CallResult{adapterResult(t, "isFiller", *tc.delegated)})
			}
			mc := &fakeMulticallClient{responses: responses}
			r := &reader{chain: mc, dec: fakeDecimalsReader{decimals: 6}, log: logr.Discard()}
			got, err := r.readPermissionedVaultInventories(
				t.Context(), executorAddr, tIn, []recoveryVault{{Adapter: vlt, Asset: tOut}})
			if err != nil {
				t.Fatalf("readPermissionedVaultInventories: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("inventories = %d, want %d", len(got), tc.want)
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }
```

Append the malformed/reverted authorization matrix as a separate test so each ABI boundary has an explicit expected result:

```go
func TestReadPermissionedVaultInventories_AuthorizationReadFailsClosed(t *testing.T) {
	t.Parallel()
	executorAddr := common.HexToAddress("0x0000000000000000000000000000000000000044")
	marketMaker := common.HexToAddress("0x0000000000000000000000000000000000000055")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000066")

	tests := []struct {
		name       string
		auth       []chain.CallResult
		delegation []chain.CallResult
	}{
		{
			name: "market maker reverted",
			auth: []chain.CallResult{
				{Success: false},
				adapterResult(t, "owner", owner),
			},
		},
		{
			name: "owner malformed",
			auth: []chain.CallResult{
				adapterResult(t, "marketMaker", marketMaker),
				{Success: true, ReturnData: []byte{0x01}},
			},
		},
		{
			name: "delegation reverted",
			auth: []chain.CallResult{
				adapterResult(t, "marketMaker", marketMaker),
				adapterResult(t, "owner", owner),
			},
			delegation: []chain.CallResult{{Success: false}},
		},
		{
			name: "delegation malformed",
			auth: []chain.CallResult{
				adapterResult(t, "marketMaker", marketMaker),
				adapterResult(t, "owner", owner),
			},
			delegation: []chain.CallResult{{Success: true, ReturnData: []byte{0x01}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			responses := [][]chain.CallResult{
				inventoryResults(t, adapterResult(t, "paused", false)),
				tc.auth,
			}
			if tc.delegation != nil {
				responses = append(responses, tc.delegation)
			}
			mc := &fakeMulticallClient{responses: responses}
			r := &reader{chain: mc, dec: fakeDecimalsReader{decimals: 6}, log: logr.Discard()}
			got, err := r.readPermissionedVaultInventories(
				t.Context(), executorAddr, tIn, []recoveryVault{{Adapter: vlt, Asset: tOut}})
			if err != nil {
				t.Fatalf("readPermissionedVaultInventories: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("inventories = %+v, want none when authorization is unknown", got)
			}
		})
	}
}
```

- [ ] **Step 9: Run chain-reader tests under the race detector**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test -race ./internal/solvers/rfq -run '^(TestReadVaultInventories_|TestReadPermissionedVaultInventories_)' -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit fail-closed reads and boundary tests**

```bash
git add internal/solvers/rfq/chainreader.go internal/solvers/rfq/chainreader_test.go
git commit -m "fix(rfq): fail closed on unknown adapter pause state"
```

---

### Task 5: Amortize Fill-Plan Cache Eviction

**Files:**
- Modify: `internal/solvers/rfq/strategies/default/strategy.go:20-35,362-389`
- Modify: `internal/solvers/rfq/strategies/default/strategy_test.go`

**Interfaces:**
- Consumes: existing injectable `Strategy.now`, `fillPlanTTL`, `plans`, and `mu`.
- Produces: `fillPlanSweepInterval = time.Minute`, mutex-guarded `Strategy.nextSweep time.Time`, and `func (s *Strategy) sweepExpiredLocked(now time.Time)`; `remember` is O(1) between scheduled sweeps and `cached` lazily removes its requested expired key.

- [ ] **Step 1: Add deterministic cadence and lazy-deletion tests**

Append these tests to `internal/solvers/rfq/strategies/default/strategy_test.go`:

```go
func cachePlan() *types.FillPlan {
	return &types.FillPlan{
		QuoteID:         "q",
		TokenIn:         tIn,
		TokenOut:        tOut,
		AmountIn:        big.NewInt(1),
		QuotedAmountOut: big.NewInt(1),
	}
}

func TestRememberAmortizesExpiredPlanSweep(t *testing.T) {
	t.Parallel()
	now := time.Unix(10_000, 0)
	s := New(fakePricing{})
	s.now = func() time.Time { return now }
	s.nextSweep = now.Add(fillPlanSweepInterval)
	s.plans["stale"] = cachedFillPlan{
		plan:      cachePlan(),
		createdAt: now.Add(-fillPlanTTL - time.Second),
	}

	s.remember("fresh-before-sweep", cachePlan())
	if _, ok := s.plans["stale"]; !ok {
		t.Fatal("remember scanned the full map before nextSweep")
	}

	now = now.Add(fillPlanSweepInterval)
	s.remember("fresh-at-sweep", cachePlan())
	if _, ok := s.plans["stale"]; ok {
		t.Fatal("scheduled sweep retained an expired plan")
	}
	if _, ok := s.plans["fresh-before-sweep"]; !ok {
		t.Fatal("scheduled sweep removed a live plan")
	}
}

func TestCachedLazilyDeletesRequestedExpiredPlan(t *testing.T) {
	t.Parallel()
	now := time.Unix(10_000, 0)
	s := New(fakePricing{})
	s.now = func() time.Time { return now }
	s.nextSweep = now.Add(time.Hour)
	s.plans["expired"] = cachedFillPlan{
		plan:      cachePlan(),
		createdAt: now.Add(-fillPlanTTL - time.Second),
	}

	got := s.cached(types.FillInput{
		QuoteID:  "expired",
		TokenIn:  tIn,
		TokenOut: tOut,
		AmountIn: big.NewInt(1),
	})
	if got != nil {
		t.Fatalf("cached expired plan = %+v, want nil", got)
	}
	if _, ok := s.plans["expired"]; ok {
		t.Fatal("requested expired plan was not deleted")
	}
}
```

- [ ] **Step 2: Run the cache tests and observe RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq/strategies/default -run '^(TestRememberAmortizesExpiredPlanSweep|TestCachedLazilyDeletesRequestedExpiredPlan)$' -count=1
```

Expected: build failure for undefined `fillPlanSweepInterval`/`nextSweep`; after only adding names, the first test fails because every `remember` still scans and the second fails because `cached` leaves the expired key in the map.

- [ ] **Step 3: Implement bounded scheduled sweeps**

Change the constants and `Strategy` state to:

```go
const (
	fillPlanTTL           = 3 * time.Hour
	fillPlanSweepInterval = time.Minute
)

type Strategy struct {
	pricing types.Pricing
	now     func() time.Time

	mu        sync.Mutex
	plans     map[string]cachedFillPlan
	nextSweep time.Time
}
```

Replace `remember` and `cached`, and add the locked helper:

```go
func (s *Strategy) remember(quoteID string, plan *types.FillPlan) {
	if quoteID == "" || plan == nil {
		return
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepExpiredLocked(now)
	s.plans[quoteID] = cachedFillPlan{plan: clonePlan(plan), createdAt: now}
}

func (s *Strategy) sweepExpiredLocked(now time.Time) {
	if !s.nextSweep.IsZero() && now.Before(s.nextSweep) {
		return
	}
	for id, cached := range s.plans {
		if now.Sub(cached.createdAt) > fillPlanTTL {
			delete(s.plans, id)
		}
	}
	s.nextSweep = now.Add(fillPlanSweepInterval)
}

func (s *Strategy) cached(input types.FillInput) *types.FillPlan {
	now := s.now()
	s.mu.Lock()
	cached, ok := s.plans[input.QuoteID]
	if ok && now.Sub(cached.createdAt) > fillPlanTTL {
		delete(s.plans, input.QuoteID)
		ok = false
	}
	s.mu.Unlock()
	if !ok {
		return nil
	}
	plan := clonePlan(cached.plan)
	if err := validateCachedPlan(input, plan); err != nil {
		return nil
	}
	return plan
}
```

Call `s.now()` once per operation as shown so tests and expiration decisions cannot observe two clock values.

- [ ] **Step 4: Add a concurrent cache characterization test**

Append:

```go
func TestFillPlanCacheConcurrentRememberAndLookup(t *testing.T) {
	t.Parallel()
	s := New(fakePricing{})
	plan := cachePlan()
	input := types.FillInput{
		QuoteID:  "shared",
		TokenIn:  tIn,
		TokenOut: tOut,
		AmountIn: big.NewInt(1),
	}

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 100 {
				s.remember("shared", plan)
			}
		}()
		go func() {
			defer wg.Done()
			for range 100 {
				_ = s.cached(input)
			}
		}()
	}
	wg.Wait()
}
```

Add `sync` to the test imports.

- [ ] **Step 5: Run default-strategy tests with the race detector**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test -race ./internal/solvers/rfq/strategies/default -count=1
```

Expected: PASS with no race report.

- [ ] **Step 6: Record the amortized hot-path benchmark**

Append this non-threshold benchmark; it records behavior without a timing-based flaky assertion:

```go
func BenchmarkRememberFillPlanBetweenSweeps(b *testing.B) {
	s := New(fakePricing{})
	now := time.Unix(10_000, 0)
	s.now = func() time.Time { return now }
	s.nextSweep = now.Add(time.Hour)
	for i := range 100_000 {
		id := strconv.Itoa(i)
		s.plans[id] = cachedFillPlan{plan: cachePlan(), createdAt: now}
	}
	plan := cachePlan()
	b.ResetTimer()
	for range b.N {
		s.remember("hot", plan)
	}
}
```

Add `strconv` to the test imports, then run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq/strategies/default -run '^$' -bench '^BenchmarkRememberFillPlanBetweenSweeps$' -benchmem -count=3
```

Expected: three benchmark samples complete successfully. Save the output in the implementation handoff; do not add a machine-dependent time threshold to the test.

- [ ] **Step 7: Commit amortized eviction**

```bash
git add internal/solvers/rfq/strategies/default/strategy.go internal/solvers/rfq/strategies/default/strategy_test.go
git commit -m "perf(rfq): amortize fill plan eviction"
```

---

### Task 6: Reconcile RFQ Operator and Maintainer Documentation

**Files:**
- Modify: `docs/RFQ-PLAN.md`
- Modify: `README.md:59-70`
- Modify: `config/rfq.example.yaml:53-58`
- Modify: `.github/chart/mainnet.yaml:43-46`
- Modify: `.github/chart/sepolia.yaml:39-41`
- Modify: `.github/chart/hoodi.yaml:41-43`

**Interfaces:**
- Consumes: behavior completed in Tasks 1-5 and existing `Config.quoteScopesToAdapters`, `Config.restrictsToAdapters`, and `Config.usesDiscounts` semantics.
- Produces: one consistent operator contract: external mode skips the internal-only discounts API and scopes quote/execution to configured adapters; internal mode may use the internal-only discounts API, quote scoping follows configured adapters, and execution may recover through any backend-advertised discount adapter.

- [ ] **Step 1: Add a documentation contradiction check**

Run this command before editing:

```bash
rg -n 'applies a discount|Swap.*vault.*slot|public discounts|public discounts flow|in-memory strategies/orders/attempts|full functional parity' \
  README.md docs/RFQ-PLAN.md config/rfq.example.yaml \
  .github/chart/mainnet.yaml .github/chart/sepolia.yaml .github/chart/hoodi.yaml
```

Expected: matches in `docs/RFQ-PLAN.md`, `config/rfq.example.yaml`, and all three chart profiles. Record the exact list so the GREEN check can prove each stale claim disappeared.

- [ ] **Step 2: Update the RFQ plan's execution and state descriptions**

Make these exact semantic replacements in `docs/RFQ-PLAN.md`:

```markdown
- **HTTP server** — `POST /quote` (backend fans out a swap request carrying the candidate per-adapter
  inventory snapshot in `adapters[]`; the selected strategy prices it, selects direct and eligible
  signature-gated discount legs, caches the default strategy's fill plan by `quoteId`, and returns an
  `amountOut`), `GET /health`, and the code-first OpenAPI surface (`/openapi.json`, `/openapi.yaml`,
  `/docs`).

- **Execution** — selects exactly one backend row matching the requested `orderId`, decodes its signed
  `encodedOrder`, and treats that tuple as authoritative for filler, input, amount, deadline, and
  outputs. Optional backend filler/output projections must agree. It then builds
  `Executor.fill(Order, protocolSig, Swap[], DiscountSwapInput[], bytes)`; each direct `SwapInput`
  carries the selected LiquidLane `adapter` explicitly.

- **State** — in-memory only: the default strategy's fill plans (by `quoteId`), order records (state
  machine), and attempt counts. Expired fill plans are lazily removed on lookup and swept at a bounded
  cadence; terminal orders retain their existing three-hour eviction.
```

Update the component map's `store.go` row so it says `orders/attempts`; identify the default strategy as the owner of the fill-plan cache.

- [ ] **Step 3: Make solver-mode terminology match production behavior**

Replace the internal-mode paragraph in `docs/RFQ-PLAN.md` with:

```markdown
- **`internal`**: may call the backend's **internal-only discounts API** (`GET`/`POST /discounts`).
  Configured `adapters` scope quoting when non-empty, but execution is not adapter-restricted so
  discount-driven recovery can use any backend-advertised adapter. Configured adapters remain optional
  extra permissioned recovery inventory.
```

In the parity/hardening section, replace the absolute `full functional parity` claim with a bounded statement that lists the deliberate Go hardenings:

```markdown
**Status:** the pricing, ABI encoding, backend endpoints, and recovery read set track the current TS
filler, while the Go service deliberately fails closed at additional trust boundaries. It requires an
exact `orderId` match, binds fill terms to the ABI-decoded signed order, rejects unknown pause state,
validates strategy/order terms, and bounds in-memory cache retention.
```

Ensure the plan no longer claims a quote discount is applied or that a direct swap has a `vault` field.

- [ ] **Step 4: Update operator-facing wording**

Use this wording in `README.md`'s RFQ section:

```markdown
It runs either in `external` mode (the open-source filler; quoting and filling scoped to the operator's
own adapters, with no discounts API access) or `internal` mode (Symbiotic-internal; may use the
backend's internal-only discounts API). The caller EOA must be an authorized caller of the RFQ
`Executor` (its `setCallers` allowlist, granted by the owner).
```

In `config/rfq.example.yaml`, use:

```yaml
      #   internal — Symbiotic-internal: may use the backend's internal-only discounts API; configured
      #              adapters scope quoting when non-empty but do not restrict discount-driven filling.
```

In `.github/chart/mainnet.yaml`, `.github/chart/sepolia.yaml`, and `.github/chart/hoodi.yaml`, replace every `public discounts` phrase with `the internal-only discounts API`. Preserve all deployed values and addresses.

- [ ] **Step 5: Run the documentation contradiction check and observe GREEN**

Run:

```bash
if rg -n 'applies a discount|Swap.*vault.*slot|public discounts|public discounts flow|in-memory strategies/orders/attempts|full functional parity' \
  README.md docs/RFQ-PLAN.md config/rfq.example.yaml \
  .github/chart/mainnet.yaml .github/chart/sepolia.yaml .github/chart/hoodi.yaml; then
  exit 1
fi
```

Expected: exit 0 with no matches.

- [ ] **Step 6: Commit synchronized RFQ documentation**

```bash
git add README.md docs/RFQ-PLAN.md config/rfq.example.yaml \
  .github/chart/mainnet.yaml .github/chart/sepolia.yaml .github/chart/hoodi.yaml
git commit -m "docs(rfq): reconcile execution and discount behavior"
```

---

### Task 7: Run the RFQ and Repository Verification Gates

**Files:**
- Verify only: all files changed by Tasks 1-6

**Interfaces:**
- Consumes: completed RFQ commits and the preceding repository-wide changesets.
- Produces: evidence that focused RFQ behavior, race safety, full repository tests, lint, build, and deterministic generation all pass on Go 1.26.5.

- [ ] **Step 1: Run focused RFQ tests with race detection and coverage**

```bash
GOTOOLCHAIN=go1.26.5 go test -race -cover ./internal/solvers/rfq/... -count=1
```

Expected: every RFQ and RFQ-strategy package reports `ok`; no race report; coverage percentages are printed.

- [ ] **Step 2: Run autofix formatting/lint and inspect its diff**

```bash
GOTOOLCHAIN=go1.26.5 golangci-lint run --fix
git diff --check
git status --short
```

Expected: lint exits 0, `git diff --check` prints nothing, and status contains only intentional implementation/doc changes. If autofix changed a file, review the hunk and amend the commit that owns that file; do not create an unrelated formatting commit.

- [ ] **Step 3: Build every package**

```bash
GOTOOLCHAIN=go1.26.5 go build ./...
```

Expected: exit 0 with no output.

- [ ] **Step 4: Run the full repository race/coverage suite**

```bash
GOTOOLCHAIN=go1.26.5 go test -race -cover ./... -count=1
```

Expected: every package reports `ok` (or `[no test files]`), with no race report and no failure.

- [ ] **Step 5: Run lint without autofix**

```bash
GOTOOLCHAIN=go1.26.5 golangci-lint run
```

Expected: exit 0 and zero issues.

- [ ] **Step 6: Verify deterministic generated code**

```bash
GOTOOLCHAIN=go1.26.5 make check-generated
git status --short
```

Expected: `make check-generated` exits 0. It must not report drift in generated bindings/clients; status must contain no newly generated changes.

- [ ] **Step 7: Re-run the stale-documentation assertion**

```bash
if rg -n 'applies a discount|Swap.*vault.*slot|public discounts|public discounts flow|in-memory strategies/orders/attempts|full functional parity' \
  README.md docs/RFQ-PLAN.md config/rfq.example.yaml \
  .github/chart/mainnet.yaml .github/chart/sepolia.yaml .github/chart/hoodi.yaml; then
  exit 1
fi
```

Expected: exit 0 with no matches.

- [ ] **Step 8: Review commit scope and final diff**

```bash
git log --oneline --decorate -7
git diff HEAD~6..HEAD --stat
git diff HEAD~6..HEAD -- \
  internal/solvers/rfq \
  README.md docs/RFQ-PLAN.md config/rfq.example.yaml \
  .github/chart/mainnet.yaml .github/chart/sepolia.yaml .github/chart/hoodi.yaml
```

Expected: the diff contains only the RFQ implementation, its tests, and synchronized RFQ documentation described in this plan. Confirm no generated Go, generic framework package, deployment value, secret, or finding-1 pin was added.

---

## Completion Criteria

- `/orders` lookup returns only one exact requested `orderId`; a nonempty nonmatching response and duplicate matches return errors.
- The local `orderId` and `quoteId` match the executable row before decode.
- The decoded signed order is the only source for filler, input token, input amount, deadline, outputs, strategy requirements, swaps, and final fill calldata.
- Optional backend filler and output projections are equality checks only; mismatch stops before transaction submission.
- Decoded deadlines and amounts are validated as `big.Int`, including a deadline larger than `int64`.
- Zero executor config is rejected.
- Reverted or malformed `paused()` reads drop the adapter.
- ABI-shaped tests pin inventory and authorization Multicall layouts and fail-closed outcomes.
- Normal quote insertion does not scan the three-hour cache map; scheduled sweep and lazy lookup deletion keep it bounded.
- Concurrent cache tests pass with the race detector.
- RFQ docs, examples, and chart comments consistently describe adapter fields and the internal-only discounts API.
- Focused and full Go 1.26.5 format, build, race/coverage test, lint, and generated-code drift gates all pass.
