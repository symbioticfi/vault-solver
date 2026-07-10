# OEV Hardening Implementation Plan

> **Public-port status:** This is the source-branch implementation record, not an executable checklist
> for the current public tree. The public port preserves the audit invariants while keeping generic WS,
> Executor, and adapter-snapshot ownership in `internal/solvers/redstoneoev/` and Morpho/IRM/bundle logic
> in `internal/solvers/redstoneoev/strategies/default/`. Receipt attribution was intentionally removed;
> shutdown instead joins the public async auction-decision workers. Literal root-OEV paths and
> receipt-attribution steps below are superseded by [`../../OEV-PLAN.md`](../../OEV-PLAN.md).

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the RedStone OEV solver's WebSocket boundary, cache freshness, result handling, bundle search, and Morpho accrual inputs while adding money-boundary characterization tests and synchronized operator/maintainer documentation.

**Architecture:** Keep every protocol decision inside `internal/solvers/redstoneoev`; consume the generic pinned-block Multicall primitive delivered by the preceding runtime plan. Background refreshes publish immutable snapshots with independently aged components, WebSocket processing remains single-reader and bounded, Morpho fee/rate data is read coherently at the API-selected block, and the hot-path beam retains only its best 64 lightweight trials before materialization.

**Tech Stack:** Go 1.26.5, go-ethereum/abigen v2 bindings, Gorilla WebSocket, `math/big`, atomic snapshots, Multicall3, Prometheus test helpers, Foundry-derived ABI fixtures, standard Go benchmarks.

## Global Constraints

- Finding 1 remains out of scope; do not add workflow digest pins or container image digests.
- Use `GOTOOLCHAIN=go1.26.5` for every Go command; the preceding toolchain plan must already have updated repository pins.
- Consume this exact generic interface from the preceding chain plan: `func (c *chain.Client) MulticallAt(ctx context.Context, calls []chain.Call, blockNumber *big.Int) ([]chain.CallResult, error)`.
- `MulticallAt` with a non-nil block must issue one `eth_call` at that block; this plan must not recreate Multicall3 packing inside the OEV package.
- Generated Go under `api/` is read-only. The existing `api/bindings/oev/morpho` and `api/bindings/oev/irm` v2 bindings already provide `PackMarket`, `UnpackMarket`, `PackBorrowRateView`, and `UnpackBorrowRateView`.
- Public YAML stays backward compatible except that unsafe remote `ws://` URLs, credential-bearing URLs, malformed URLs, and unsupported schemes now fail startup validation.
- All protocol-specific code remains under `internal/solvers/redstoneoev`; only the already-approved generic `chain.MulticallAt` dependency is consumed.
- Use `github.com/go-errors/errors` for new runtime errors and `logr.Logger` for structured logs.
- Keep the hot path free of RPC/HTTP I/O. All Morpho, feed, executor, balance, and gas-predictor reads remain in monitor/ops refreshes.
- Missing, reverted, undecodable, or stale money-facing state fails closed; never replace a failed non-zero IRM read with a zero rate.
- Update `docs/OEV-PLAN.md` and operator-facing configuration text in the same commit as the behavior they describe.
- Do not deploy, push, or open a pull request while executing this plan.

## File Responsibility Map

| File | Responsibility in this plan |
|---|---|
| `internal/solvers/redstoneoev/config.go` | Validate secure/loopback WebSocket URLs before factory construction |
| `internal/solvers/redstoneoev/config_test.go` | Pin accepted and rejected WebSocket URL classes |
| `internal/solvers/redstoneoev/wsclient.go` | Apply the fixed inbound-frame byte limit before subscription/read startup |
| `internal/solvers/redstoneoev/wsintegration_test.go` | Exercise the real Gorilla client against normal and oversized local frames |
| `internal/solvers/redstoneoev/solver.go` | Merge per-component ops state, gate stale components, deduplicate settlement results, and supervise attribution workers |
| `internal/solvers/redstoneoev/solver_test.go` | Pin freshness, duplicate-result, breaker, reservation, metrics, and worker-join behavior |
| `internal/solvers/redstoneoev/wsmessages.go` | Derive deterministic liquidation-result identities |
| `internal/solvers/redstoneoev/reservations.go` | Provide separate bounded auction and liquidation-result seen sets |
| `internal/solvers/redstoneoev/chainreader.go` | Read exact Morpho market tuples and IRM rates at one pinned block |
| `internal/solvers/redstoneoev/chainreader_test.go` | Pin exact market/IRM tuple conversion and failure behavior |
| `internal/solvers/redstoneoev/chainreader_boundary_test.go` | Record OEV's actual `MulticallAt` batches, block tag, selectors, and decoded outputs |
| `internal/solvers/redstoneoev/monitor.go` | Resolve Morpho, enrich API discovery with pinned on-chain state, and use the real block header timestamp |
| `internal/solvers/redstoneoev/monitor_test.go` | Pin API-block selection, market intersection, header time, and no-zero-rate fallback |
| `internal/solvers/redstoneoev/testmonitor.go` | Give the Sepolia seeded monitor the same exact fee/rate path |
| `internal/morpho/math_test.go` | Add independently calculated non-zero-fee accrual/debt vectors |
| `internal/solvers/redstoneoev/bundle.go` | Probe lightweight trials, retain a stable top-64 heap, and materialize only retained states |
| `internal/solvers/redstoneoev/bundle_benchmark_test.go` | Record runtime and allocation behavior at realistic candidate/depth combinations |
| `docs/OEV-PLAN.md` | Describe the implemented security, concurrency, source-of-truth, and complexity guarantees |
| `config/redstone-oev.example.yaml` | Tell operators that plaintext WebSocket is loopback-only |
| `README.md` | Surface the RedStone production WSS requirement without adding internal design detail |

---

### Task 1: Enforce a Secure, Size-Bounded WebSocket Boundary

**Files:**
- Modify: `internal/solvers/redstoneoev/config.go:127-143`
- Modify: `internal/solvers/redstoneoev/config_test.go:63-312`
- Modify: `internal/solvers/redstoneoev/wsclient.go:15-180`
- Modify: `internal/solvers/redstoneoev/wsintegration_test.go:1-76`
- Modify: `config/redstone-oev.example.yaml:26-31`
- Modify: `README.md:72-83`
- Modify: `docs/OEV-PLAN.md:435-456`

**Interfaces:**
- Consumes: `rawWS.URL string` from strict YAML decoding and Gorilla's `(*websocket.Conn).SetReadLimit(limit int64)`.
- Produces: `func validateWSURL(raw string) error` and `const maxWSMessageBytes int64 = 1 << 20`.
- Preserves: `Config.WSURL string`, `newWSClient`, reconnect behavior, and the local `httptest` WebSocket path.

- [ ] **Step 1: Add failing URL-security tests**

Add a focused table test to `config_test.go` that calls the production parser, not a standalone URL helper:

```go
func TestParseConfigWebSocketURLSecurity(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "production wss", url: "wss://oev.example/ws"},
		{name: "localhost ws", url: "ws://localhost:8080/ws"},
		{name: "ipv4 loopback ws", url: "ws://127.0.0.1:8080/ws"},
		{name: "ipv6 loopback ws", url: "ws://[::1]:8080/ws"},
		{name: "remote plaintext", url: "ws://oev.example/ws", wantErr: true},
		{name: "credentials", url: "wss://user:pass@oev.example/ws", wantErr: true},
		{name: "missing host", url: "wss:///ws", wantErr: true},
		{name: "http scheme", url: "https://oev.example/ws", wantErr: true},
		{name: "relative", url: "/ws", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			y := strings.Replace(validCfg, "wss://dev-rwa-sepolia.oev.a.redstone.finance", tc.url, 1)
			_, err := decodeCfg(t, y)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseConfig error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run the URL test and confirm RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/redstoneoev -run '^TestParseConfigWebSocketURLSecurity$' -count=1
```

Expected: FAIL because `parseConfig` currently accepts remote plaintext, credential-bearing, and unsupported WebSocket URLs.

- [ ] **Step 3: Implement minimal URL validation**

Import `net`, `net/url`, and `strings`, then call this helper immediately after the existing empty-URL check:

```go
func validateWSURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return errors.Errorf("ws.url must be an absolute ws/wss URL with a host, got %q", raw)
	}
	if u.User != nil {
		return errors.New("ws.url must not contain credentials")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "wss" {
		return nil
	}
	if scheme != "ws" {
		return errors.Errorf("ws.url scheme must be wss, got %q", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return errors.New("ws.url may use plaintext ws only for localhost or a loopback IP")
	}
	return nil
}
```

In `parseConfig`:

```go
if err := validateWSURL(raw.WS.URL); err != nil {
	return nil, err
}
```

- [ ] **Step 4: Add an oversized-frame integration test**

Append a real-client test to `wsintegration_test.go`. The server must send a text frame one byte above the production limit on each connection; the client must reconnect without invoking `onMsg`:

```go
func TestWSIntegrationRejectsOversizedFrame(t *testing.T) {
	var connections atomic.Int32
	var delivered atomic.Int32
	reconnected := make(chan struct{}, 1)
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if connections.Add(1) >= 2 {
			select {
			case reconnected <- struct{}{}:
			default:
			}
		}
		_ = conn.WriteMessage(websocket.TextMessage, make([]byte, maxWSMessageBytes+1))
	}))
	defer srv.Close()

	client := newWSClient(wsConfig{
		URL:            "ws" + strings.TrimPrefix(srv.URL, "http"),
		APIKey:         "test",
		Topics:        []string{"oev/liquidations"},
		BackoffInitial: time.Millisecond,
		BackoffMax:     5 * time.Millisecond,
	}, logr.Discard(), func(context.Context, []byte) {
		delivered.Add(1)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.Run(ctx) }()

	select {
	case <-reconnected:
	case <-ctx.Done():
		t.Fatal("client did not reconnect after an oversized frame")
	}
	if got := delivered.Load(); got != 0 {
		t.Fatalf("oversized frames delivered = %d, want 0", got)
	}
	cancel()
	<-done
}
```

- [ ] **Step 5: Run the oversized-frame test and confirm RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/redstoneoev -run '^TestWSIntegrationRejectsOversizedFrame$' -count=1
```

Expected: FAIL because `maxWSMessageBytes` is undefined and the client has no `SetReadLimit` call.

- [ ] **Step 6: Apply the fixed read limit before subscriptions and pumps**

Add the package constant and insert the limit immediately after a successful dial, before queue flushing or subscription writes:

```go
const maxWSMessageBytes int64 = 1 << 20

if err != nil {
	return errors.Errorf("dial websocket: %w", err)
}
conn.SetReadLimit(maxWSMessageBytes)
w.log.Info("connected")
```

Keep the existing subscription and pump/join body directly after this insertion. Remove the full URL from connection logs/errors.

- [ ] **Step 7: Update operator and maintainer documentation**

In `config/redstone-oev.example.yaml`, state that production requires `wss://` and `ws://` is accepted only for local loopback testing. In the README's RedStone paragraph, add one sentence saying its authenticated auction stream requires WSS in production. In `docs/OEV-PLAN.md` section 6.1, record the 1 MiB frame limit and loopback-only plaintext exception.

- [ ] **Step 8: Run focused transport tests and confirm GREEN**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test -race ./internal/solvers/redstoneoev -run 'Test(ParseConfigWebSocketURLSecurity|WSIntegration)' -count=1
```

Expected: PASS; the existing reconnect hygiene test and both new security tests pass under the race detector.

- [ ] **Step 9: Commit the transport boundary**

```bash
git add internal/solvers/redstoneoev/config.go internal/solvers/redstoneoev/config_test.go internal/solvers/redstoneoev/wsclient.go internal/solvers/redstoneoev/wsintegration_test.go config/redstone-oev.example.yaml README.md docs/OEV-PLAN.md
git commit -m "fix(oev): harden websocket transport"
```

---

### Task 2: Track Every Ops-State Component's Freshness Independently

**Files:**
- Modify: `internal/solvers/redstoneoev/solver.go:196-307,470-510,755-779`
- Modify: `internal/solvers/redstoneoev/solver_test.go:29-227,768-793`
- Modify: `docs/OEV-PLAN.md:217-243`

**Interfaces:**
- Consumes: `Config.MaxStateAge`, `snapshot.updatedAt`, existing `latestHeadState`, `ReadExecutorState`, `BalanceAt`, `ReadLoanEthRate`, and `ReadGasPredictorState`.
- Produces: `type stateFreshness`, `type cachedStateUpdate`, and `func mergeCachedState(prev cachedState, update cachedStateUpdate) cachedState`.
- Preserves: one atomic `stateCache` swap, executor bookkeeping after every successful executor read, and `skipStaleState` as the bounded metric label.

- [ ] **Step 1: Replace the aggregate-age test with a component matrix that initially fails to compile**

Update the seeded state to stamp every component, then replace `TestBuildBidStaleStateGate` with a matrix over exact stamp names:

```go
func freshStateTimes(at time.Time) stateFreshness {
	return stateFreshness{
		Executor:        at,
		CallbackBalance: at,
		LoanEthRate:     at,
		GasPredictor:   at,
		HeadGasLimit:   at,
	}
}

func TestBuildBidStaleStateGateByComponent(t *testing.T) {
	base := auctionClock()()
	now := base.Add(defaultMaxStateAge + time.Second)
	tests := []struct {
		name string
		age  func(*cachedState)
	}{
		{name: "executor", age: func(st *cachedState) { st.Fresh.Executor = base }},
		{name: "callback balance", age: func(st *cachedState) { st.Fresh.CallbackBalance = base }},
		{name: "loan eth rate", age: func(st *cachedState) { st.Fresh.LoanEthRate = base }},
		{name: "gas predictor", age: func(st *cachedState) { st.Fresh.GasPredictor = base }},
		{name: "head gas limit", age: func(st *cachedState) { st.Fresh.HeadGasLimit = base }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := seededSolver(t)
			snap := *snapshotOf(t, s)
			snap.updatedAt = now
			storeSnapshot(t, s, &snap)
			st, _ := s.state.load()
			st.Fresh = freshStateTimes(now)
			tc.age(&st)
			s.state.store(st)
			if got := s.buildBid(decodeAuction(t), func() time.Time { return now }).skip; got != skipStaleState {
				t.Fatalf("skip = %q, want %q", got, skipStaleState)
			}
		})
	}
}
```

- [ ] **Step 2: Add failing merge tests that distinguish reused values from fresh values**

```go
func TestMergeCachedStateDoesNotRefreshFailedComponents(t *testing.T) {
	oldAt := time.Unix(100, 0)
	newAt := time.Unix(200, 0)
	prev := cachedState{
		Exec:           ExecutorState{Nonce: big.NewInt(1), Deposit: big.NewInt(2)},
		CallbackNative: big.NewInt(3),
		Rate:           big.NewInt(4),
		Gas:            &gasPredictorState{FreeAssets: big.NewInt(5)},
		GasLimit:       6,
		Fresh:          freshStateTimes(oldAt),
	}
	newExec := ExecutorState{Nonce: big.NewInt(7), Deposit: big.NewInt(8)}
	newRate := big.NewInt(9)
	got := mergeCachedState(prev, cachedStateUpdate{
		At:       newAt,
		Executor: &newExec,
		Rate:     newRate,
	})
	if got.Exec.Nonce.Uint64() != 7 || !got.Fresh.Executor.Equal(newAt) {
		t.Fatalf("executor was not refreshed: %+v", got)
	}
	if got.Rate.Cmp(newRate) != 0 || !got.Fresh.LoanEthRate.Equal(newAt) {
		t.Fatalf("rate was not refreshed: %+v", got)
	}
	if got.CallbackNative.Cmp(big.NewInt(3)) != 0 || !got.Fresh.CallbackBalance.Equal(oldAt) {
		t.Fatalf("failed balance read changed value or age: %+v", got)
	}
	if got.Gas.FreeAssets.Cmp(big.NewInt(5)) != 0 || !got.Fresh.GasPredictor.Equal(oldAt) {
		t.Fatalf("failed predictor read changed value or age: %+v", got)
	}
	if got.GasLimit != 6 || !got.Fresh.HeadGasLimit.Equal(oldAt) {
		t.Fatalf("failed header read changed gas limit or age: %+v", got)
	}
}
```

Add `TestRefreshStateEpochCrossingStillAppliesExecutorBookkeeping`. Seed one reservation and nonce
state, make `ReadExecutorState` succeed with a nonce that resolves them, then make the ending block
check cross the starting epoch. Assert reservation pruning and nonce reconciliation still occur while
the coherent cached snapshot is not published. A later component failure may not discard successful
executor bookkeeping.

- [ ] **Step 3: Run freshness tests and confirm RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/redstoneoev -run 'Test(BuildBidStaleStateGateByComponent|MergeCachedStateDoesNotRefreshFailedComponents|RefreshStateEpochCrossingStillAppliesExecutorBookkeeping)' -count=1
```

Expected: FAIL to compile with undefined `stateFreshness`, `cachedStateUpdate`, `Fresh`, and `mergeCachedState`.

- [ ] **Step 4: Add the independent freshness model and merge helper**

Replace `cachedState.UpdatedAt` with:

```go
type stateFreshness struct {
	Executor        time.Time
	CallbackBalance time.Time
	LoanEthRate     time.Time
	GasPredictor   time.Time
	HeadGasLimit   time.Time
}

type cachedState struct {
	Exec           ExecutorState
	CallbackNative *big.Int
	Rate           *big.Int
	Gas            *gasPredictorState
	GasLimit       uint64
	Fresh          stateFreshness
}

type cachedStateUpdate struct {
	At              time.Time
	Executor        *ExecutorState
	CallbackNative  *big.Int
	Rate            *big.Int
	Gas             *gasPredictorState
	HeadGasLimit    *uint64
}

func mergeCachedState(prev cachedState, update cachedStateUpdate) cachedState {
	next := prev
	if update.Executor != nil {
		next.Exec = *update.Executor
		next.Fresh.Executor = update.At
	}
	if update.CallbackNative != nil {
		next.CallbackNative = update.CallbackNative
		next.Fresh.CallbackBalance = update.At
	}
	if update.Rate != nil {
		next.Rate = update.Rate
		next.Fresh.LoanEthRate = update.At
	}
	if update.Gas != nil {
		next.Gas = update.Gas
		next.Fresh.GasPredictor = update.At
	}
	if update.HeadGasLimit != nil {
		next.GasLimit = *update.HeadGasLimit
		next.Fresh.HeadGasLimit = update.At
	}
	return next
}
```

Nil update fields mean that component's read failed or was unavailable; they retain both value and timestamp.

- [ ] **Step 5: Make header fallback explicitly report an unavailable gas-limit component**

Extend the existing head result without treating the RedStone cap fallback as fresh chain data:

```go
type latestHeadState struct {
	Number        uint64
	GasLimit      uint64
	HasGasLimit   bool
}
```

Return `HasGasLimit: true` only from a valid `HeaderByNumber` response. If only `BlockNumber` succeeds, return the number with `HasGasLimit: false`; the refresh may update executor state but must retain the previous gas-limit value and age.

- [ ] **Step 6: Apply executor bookkeeping immediately; gate only snapshot publication**

Refactor `refreshState` around one update value:

```go
prev, _ := s.state.load()
update := cachedStateUpdate{At: epoch.At, Executor: &st}
if head.HasGasLimit {
	gasLimit := head.GasLimit
	update.HeadGasLimit = &gasLimit
}
if berr == nil {
	update.CallbackNative = bal
	s.applyExecutorState(st, bal, epoch.At)
	// Balance metrics may use this successful read.

} else {
	s.applyExecutorState(st, nil, epoch.At)
}
if rate := s.reader.ReadLoanEthRate(ctx, s.cfg.Adapter, s.cfg.LoanEthFeed, epoch.At); rate != nil {
	update.Rate = rate
}
if gasState, gerr := s.reader.ReadGasPredictorState(ctx, s.cfg.Adapter, quoteCollateralsFromSnapshot(s.mon.snapshot())); gerr == nil && gasState != nil {
	update.Gas = gasState
} else if gerr != nil {
	s.log.Error(gerr, "read gas predictor state failed; keeping last cached predictor state")
}
if !s.epochStillCurrent(ctx, epoch, "state") {
	return
}
next := mergeCachedState(prev, update)
s.state.store(next)
```

Place the `applyExecutorState` call immediately after the callback-balance attempt, before rate,
predictor, or ending-epoch reads. Keep the existing early return only when executor state itself
fails. A failed callback balance, rate, predictor, header detail, or ending block-stability check no
longer prevents successful reservation pruning, nonce reconciliation, or deposit-floor bookkeeping;
only publication of the coherent cached snapshot is gated by `epochStillCurrent`.

- [ ] **Step 7: Gate and log every required stamp**

Use a fixed list, avoiding free-form metric labels:

```go
func staleOpsComponents(st cachedState, ok bool, now time.Time, maxAge time.Duration) []any {
	components := []struct {
		name    string
		at      time.Time
		present bool
	}{
		{name: "executor", at: st.Fresh.Executor, present: st.Exec.Nonce != nil && st.Exec.Deposit != nil},
		{name: "callbackBalance", at: st.Fresh.CallbackBalance, present: st.CallbackNative != nil},
		{name: "loanEthRate", at: st.Fresh.LoanEthRate, present: st.Rate != nil},
		{name: "gasPredictor", at: st.Fresh.GasPredictor, present: st.Gas != nil},
		{name: "headGasLimit", at: st.Fresh.HeadGasLimit, present: st.GasLimit > 0},
	}
	if !ok {
		for i := range components {
			components[i].present = false
		}
	}
	fields := make([]any, 0, len(components)*2)
	for _, component := range components {
		if !component.present || component.at.IsZero() || now.Sub(component.at) > maxAge {
			at := component.at
			if !component.present {
				at = time.Time{}
			}
			fields = append(fields, component.name+"Age", cacheAge(at, now))
		}
	}
	return fields
}
```

Call this helper from `staleStateGate` in addition to the monitor snapshot check. Retain a monitor-stale subtest so the existing snapshot-age guarantee remains covered.

- [ ] **Step 8: Update all test fixtures to stamp all components**

In `seededSolver`, use `Fresh: freshStateTimes(auctionClock()())`. In `setSnapshotBlockTime`, assign `freshStateTimes(time.Now())`. Retain `TestApplyExecutorStateRunsWithoutBalance` to prove executor bookkeeping is independent of callback-balance freshness.

- [ ] **Step 9: Run the OEV state suite and confirm GREEN**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test -race ./internal/solvers/redstoneoev -run 'Test(BuildBidStaleStateGateByComponent|MergeCachedStateDoesNotRefreshFailedComponents|ApplyExecutorStateRunsWithoutBalance|BuildBidHappyPath)' -count=1
```

Expected: PASS; each stale component independently produces `stale_state`, and failed reads retain their original ages.

- [ ] **Step 10: Synchronize the cache-concurrency documentation**

Rewrite `docs/OEV-PLAN.md` section 3.3 so it names the five ops stamps, states that partial refreshes merge successful values only, and explains that executor bookkeeping still runs after other component failures. Remove the claim that one `cachedState.updatedAt` represents all ops values.

- [ ] **Step 11: Commit independent freshness**

```bash
git add internal/solvers/redstoneoev/solver.go internal/solvers/redstoneoev/solver_test.go docs/OEV-PLAN.md
git commit -m "fix(oev): track component freshness independently"
```

---

### Task 3: Deduplicate Liquidation Results Before Side Effects

**Files:**
- Modify: `internal/solvers/redstoneoev/wsmessages.go:49-81`
- Modify: `internal/solvers/redstoneoev/reservations.go:121-149`
- Modify: `internal/solvers/redstoneoev/solver.go:60-84,138-149,161-179,338-361`
- Modify: `internal/solvers/redstoneoev/solver_test.go:742-766,1498-1558`
- Modify: `docs/OEV-PLAN.md:116-119,141-160,435-456`

**Interfaces:**
- Consumes: the single WS read goroutine, `LiquidationResult`, raw frame bytes, reservation lookup/release, breaker, metrics, and receipt attribution.
- Produces: `func (r LiquidationResult) dedupKey(raw []byte) string`, a generic bounded `seenKeys`, separate `seenAuctions` and `seenResults` solver fields, and supervised settlement-attribution work.
- Preserves: auction dedup semantics, maximum cache size 1,024, and callback-address ownership checks.

- [ ] **Step 1: Add failing identity and bounded-cache tests**

Add table coverage for the precedence required by the design:

```go
func TestLiquidationResultDedupKey(t *testing.T) {
	withID := LiquidationResult{ID: "auction-1", Data: LiquidationResultData{TxHash: common.Hash{1}.Hex()}}
	if got := withID.dedupKey([]byte(`{"different":"body"}`)); got != "id:auction-1" {
		t.Fatalf("id key = %q", got)
	}
	withHash := LiquidationResult{Data: LiquidationResultData{TxHash: "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}
	if got := withHash.dedupKey([]byte(`{"body":1}`)); got != "tx:0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("tx key = %q", got)
	}
	raw := []byte(`{"op":"liquidation-result","data":{"success":false}}`)
	want := "frame:" + crypto.Keccak256Hash(raw).Hex()
	if got := (LiquidationResult{}).dedupKey(raw); got != want {
		t.Fatalf("frame key = %q, want %q", got, want)
	}
}
```

Rename the existing cache test to `TestSeenKeys` and keep its capacity/oldest-eviction assertions.

- [ ] **Step 2: Add a failing duplicate-side-effect regression test**

Use `maxFailures=2`: one failed result delivered twice must not trip the breaker, but a distinct second result must:

```go
func TestLiquidationResultDuplicateHasOneSideEffect(t *testing.T) {
	s, _ := seededSolver(t)
	s.breaker = newBreaker(2, time.Hour)
	frame := func(id string) []byte {
		return marshal(LiquidationResult{
			Op: "liquidation-result",
			ID: id,
			Data: LiquidationResultData{
				Success: false,
				Liquidator: s.cfg.Callback.Hex(),
				TxHash: common.HexToHash("0x1234").Hex(),
			},
		})
	}
	s.handleMessage(context.Background(), frame("same"))
	s.handleMessage(context.Background(), frame("same"))
	if tripped, _ := s.breaker.tripped(time.Now()); tripped {
		t.Fatal("duplicate result counted twice")
	}
	s.handleMessage(context.Background(), frame("distinct"))
	if tripped, _ := s.breaker.tripped(time.Now()); !tripped {
		t.Fatal("two distinct failures must trip the breaker")
	}
}
```

Also update `TestLiquidationResultFeedsBreaker` so the three intentional failures use IDs `failure-0`, `failure-1`, and `failure-2`; repeated identical IDs no longer represent distinct settlements.

- [ ] **Step 3: Run result tests and confirm RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/redstoneoev -run 'Test(LiquidationResultDedupKey|LiquidationResultDuplicateHasOneSideEffect|SeenKeys|LiquidationResultFeedsBreaker)' -count=1
```

Expected: FAIL to compile because `LiquidationResult.dedupKey` and `seenKeys` do not exist; without dedup the duplicate frame trips the breaker.

- [ ] **Step 4: Generalize the bounded set and add a separate result cache**

Replace the auction-specific container with:

```go
const maxSeenMessages = 1024

type seenKeys struct {
	set   map[string]struct{}
	order []string
	cap   int
}

func newSeenKeys(capacity int) *seenKeys {
	return &seenKeys{set: make(map[string]struct{}, capacity), cap: capacity}
}

func (s *seenKeys) seen(key string) bool {
	if _, ok := s.set[key]; ok {
		return true
	}
	if len(s.order) >= s.cap {
		delete(s.set, s.order[0])
		s.order = s.order[1:]
	}
	s.set[key] = struct{}{}
	s.order = append(s.order, key)
	return false
}
```

The solver fields become:

```go
	seenAuctions *seenKeys
	seenResults  *seenKeys
```

Initialize both in `factory` and `seededSolver`; auction handling continues to call `s.seenAuctions.seen(key)`.

- [ ] **Step 5: Implement result identity and drop duplicates before all effects**

```go
func (r LiquidationResult) dedupKey(raw []byte) string {
	if r.ID != "" {
		return "id:" + r.ID
	}
	if common.IsHexHash(r.Data.TxHash) {
		return "tx:" + strings.ToLower(r.Data.TxHash)
	}
	return "frame:" + crypto.Keccak256Hash(raw).Hex()
}
```

Immediately after successful JSON decode in the `liquidation-result` branch:

```go
key := r.dedupKey(raw)
if s.seenResults.seen(key) {
	s.log.V(1).Info("duplicate liquidation result; already processed", "result", key)
	return
}
```

This check must precede `reservationByAuction`, the info log, attribution launch, reservation release, breaker mutation, and metrics.

- [ ] **Step 6: Preserve the already-supervised receipt-attribution owner**

The transaction-supervision plan runs first and already provides `attributionWG`, `attributeFn`,
`launchSettlementAttribution`, the `runCtx` child context, and the mandatory read-pump-before-`Wait`
ordering. Do not redeclare or bypass any of them here. Keep the liquidation-result branch calling:

```go
s.launchSettlementAttribution(ctx, r.Data.TxHash, pred)
```

The duplicate check must precede that existing call, so only the first delivery increments the wait
group. Here `ctx` is the `runCtx` passed through the WebSocket callback. Extend the duplicate test to
count `attributeFn` invocations and assert exactly one. In `Run`,
preserve cancellation of `runCtx`, joining monitor/ops, and `attributionWG.Wait()` only after
`s.ws.Run(runCtx)` has returned and joined its read pump.

- [ ] **Step 7: Run duplicate, lifecycle, and race tests and confirm GREEN**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test -race ./internal/solvers/redstoneoev -run 'Test(LiquidationResult|SeenKeys|FullAuctionLifecycle)' -count=1
```

Expected: PASS; duplicate topic delivery has one breaker/metric/log/attribution path, distinct failures retain existing breaker behavior, and the race detector reports no wait-group misuse.

- [ ] **Step 8: Update result-processing documentation**

In `docs/OEV-PLAN.md`, document separate bounded auction/result caches, result-key precedence, duplicate suppression before side effects, and the fact that `Run` joins receipt-attribution workers during shutdown.

- [ ] **Step 9: Commit duplicate suppression**

```bash
git add internal/solvers/redstoneoev/wsmessages.go internal/solvers/redstoneoev/reservations.go internal/solvers/redstoneoev/solver.go internal/solvers/redstoneoev/solver_test.go docs/OEV-PLAN.md
git commit -m "fix(oev): deduplicate liquidation results"
```

---

### Task 4: Read Coherent Morpho Fee and Borrow Rate at the API Block

**Files:**
- Create: `internal/solvers/redstoneoev/chainreader_boundary_test.go`
- Modify: `internal/solvers/redstoneoev/chainreader.go:17-43,125-153,186-233,379-424`
- Modify: `internal/solvers/redstoneoev/chainreader_test.go:1-263`
- Modify: `internal/solvers/redstoneoev/monitor.go:132-297`
- Modify: `internal/solvers/redstoneoev/monitor_test.go:106-221`
- Modify: `internal/solvers/redstoneoev/testmonitor.go:87-187,227-244`
- Modify: `internal/morpho/math_test.go:28-49`
- Modify: `docs/OEV-PLAN.md:100-110,135-151,188-215,225-243,497-509`

**Interfaces:**
- Consumes: `func (c *chain.Client) MulticallAt(ctx context.Context, calls []chain.Call, blockNumber *big.Int) ([]chain.CallResult, error)`, callback binding `PackMORPHO`/`UnpackMORPHO`, Morpho `PackMarket`/`UnpackMarket`, and IRM `PackBorrowRateView`/`UnpackBorrowRateView`.
- Produces: `type multicaller`, `func (r *reader) ReadMarketStatesAt(ctx context.Context, morphoAddr common.Address, params map[common.Hash]abiMarketParams, blockNumber *big.Int) (map[common.Hash]morpho.MarketState, error)`, and exact tuple-conversion helpers.
- Preserves: API discovery and at-risk-position enumeration, market-ID re-derivation, immutable atomic snapshots, and no I/O on the auction hot path.

- [ ] **Step 1: Add a recording Multicall boundary fixture and failing exact-state test**

Create `chainreader_boundary_test.go` with a fake implementing the exact generic calls. It records every batch and returns pre-encoded v2 binding outputs:

```go
type recordingMulticaller struct {
	batches  [][]chain.Call
	blocks   []*big.Int
	results  [][]chain.CallResult
	err      error
}

func (r *recordingMulticaller) Multicall(context.Context, []chain.Call) ([]chain.CallResult, error) {
	return nil, errors.New("unexpected latest-block multicall")
}

func (r *recordingMulticaller) MulticallAt(_ context.Context, calls []chain.Call, block *big.Int) ([]chain.CallResult, error) {
	r.batches = append(r.batches, slices.Clone(calls))
	r.blocks = append(r.blocks, new(big.Int).Set(block))
	if r.err != nil {
		return nil, r.err
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
}
```

The main characterization test uses one non-zero IRM market and one zero-IRM market:

```go
func TestReadMarketStatesAtPinsBlockAndDecodesFeeRate(t *testing.T) {
	block := big.NewInt(123)
	morphoAddr := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	nonzeroIRM := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	marketA := common.HexToHash("0x01")
	marketB := common.HexToHash("0x02")
	params := map[common.Hash]abiMarketParams{
		marketA: {Irm: nonzeroIRM, Lltv: mustBig("860000000000000000")},
		marketB: {Irm: common.Address{}, Lltv: mustBig("770000000000000000")},
	}
	stateA := morphobinding.MarketOutput{
		TotalSupplyAssets: big.NewInt(1000), TotalSupplyShares: big.NewInt(900),
		TotalBorrowAssets: big.NewInt(500), TotalBorrowShares: big.NewInt(450),
		LastUpdate: big.NewInt(100), Fee: mustBig("100000000000000000"),
	}
	stateB := morphobinding.MarketOutput{
		TotalSupplyAssets: big.NewInt(2000), TotalSupplyShares: big.NewInt(1800),
		TotalBorrowAssets: big.NewInt(0), TotalBorrowShares: big.NewInt(0),
		LastUpdate: big.NewInt(101), Fee: big.NewInt(0),
	}
	fake := &recordingMulticaller{results: [][]chain.CallResult{
		{
			{Success: true, ReturnData: packOut(t, morphoABI, "market", stateA.TotalSupplyAssets, stateA.TotalSupplyShares, stateA.TotalBorrowAssets, stateA.TotalBorrowShares, stateA.LastUpdate, stateA.Fee)},
			{Success: true, ReturnData: packOut(t, morphoABI, "market", stateB.TotalSupplyAssets, stateB.TotalSupplyShares, stateB.TotalBorrowAssets, stateB.TotalBorrowShares, stateB.LastUpdate, stateB.Fee)},
		},
		{{Success: true, ReturnData: packOut(t, irmABI, "borrowRateView", big.NewInt(182418302))}},
	}}
	r := &reader{calls: fake, log: logr.Discard()}
	got, err := r.ReadMarketStatesAt(t.Context(), morphoAddr, params, block)
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.blocks) != 2 || fake.blocks[0].Cmp(block) != 0 || fake.blocks[1].Cmp(block) != 0 {
		t.Fatalf("blocks = %v, want two calls at %s", fake.blocks, block)
	}
	if got[marketA].Fee.Cmp(stateA.Fee) != 0 || got[marketA].BorrowRatePerSec.Cmp(big.NewInt(182418302)) != 0 {
		t.Fatalf("market A state = %+v", got[marketA])
	}
	if got[marketB].BorrowRatePerSec.Sign() != 0 {
		t.Fatalf("zero IRM rate = %s, want 0", got[marketB].BorrowRatePerSec)
	}
	if len(fake.batches[0]) != 2 || fake.batches[0][0].Target != morphoAddr || fake.batches[0][1].Target != morphoAddr {
		t.Fatalf("market batch = %+v", fake.batches[0])
	}
	if !bytes.Equal(fake.batches[0][0].Data, morphoB.PackMarket(marketA)) ||
		!bytes.Equal(fake.batches[0][1].Data, morphoB.PackMarket(marketB)) {
		t.Fatalf("market selectors/order = %x / %x", fake.batches[0][0].Data, fake.batches[0][1].Data)
	}
	if len(fake.batches[1]) != 1 || fake.batches[1][0].Target != nonzeroIRM {
		t.Fatalf("IRM batch = %+v", fake.batches[1])
	}
	expectedIRMCall := irmB.PackBorrowRateView(irmParams(params[marketA]), irmMarket(got[marketA]))
	if recorded := fake.batches[1][0].Data; !bytes.Equal(recorded, expectedIRMCall) {
		t.Fatalf("borrowRateView calldata = %x, want %x", recorded, expectedIRMCall)
	}
}
```

Define `morphoABI` and `irmABI` from the committed v2 binding metadata with the existing `mustParseABI` helper. The byte-equality assertion pins total assets/shares, `lastUpdate`, fee, market params, and LLTV without relying on generated anonymous tuple reflection.

- [ ] **Step 2: Add a failing partial-read test and exact tuple assertion**

Add a complete non-zero-IRM failure case:

```go
func TestReadMarketStatesAtDropsFailedNonzeroIRM(t *testing.T) {
	block := big.NewInt(123)
	marketID := common.HexToHash("0x01")
	morphoAddr := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	params := map[common.Hash]abiMarketParams{
		marketID: {
			Irm: common.HexToAddress("0x00000000000000000000000000000000000000a1"),
			Lltv: mustBig("860000000000000000"),
		},
	}
	marketResult := chain.CallResult{Success: true, ReturnData: packOut(
		t, morphoABI, "market",
		big.NewInt(1000), big.NewInt(900), big.NewInt(500), big.NewInt(450),
		big.NewInt(100), mustBig("100000000000000000"),
	)}
	fake := &recordingMulticaller{results: [][]chain.CallResult{
		{marketResult},
		{{Success: false}},
	}}
	r := &reader{calls: fake, log: logr.Discard()}
	got, err := r.ReadMarketStatesAt(t.Context(), morphoAddr, params, block)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[marketID]; ok {
		t.Fatal("market with reverted non-zero IRM was retained with a zero-rate fallback")
	}
}
```

Add `TestReadMarketStatesAtDropsUninitializedZeroMarket`: return a successful all-zero `market(id)`
tuple for a configured non-zero IRM and assert the market is omitted and no IRM batch is issued.
Morpho uses `lastUpdate == 0` as the definitive uninitialized-market signal; an all-zero tuple is not a
valid empty market.

- [ ] **Step 3: Run reader tests and confirm RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/redstoneoev -run '^TestReadMarketStatesAt' -count=1
```

Expected: FAIL to compile because `reader.calls`, `multicaller`, and `ReadMarketStatesAt` do not exist.

- [ ] **Step 4: Add the narrow Multicall seam and exact binding instances**

Import the existing IRM binding and define:

```go
var irmB = irmbinding.NewAdaptiveCurveIrm()

type multicaller interface {
	Multicall(ctx context.Context, calls []chain.Call) ([]chain.CallResult, error)
	MulticallAt(ctx context.Context, calls []chain.Call, blockNumber *big.Int) ([]chain.CallResult, error)
}

type reader struct {
	chain *chain.Client
	calls multicaller
	log logr.Logger
	decimals *chain.Decimals
	mu sync.Mutex
	adapterLoan map[common.Address]common.Address
	redeemColl map[common.Address][]common.Address
}
```

`newReader` sets both `chain: c` and `calls: c`. Route every existing `r.chain.Multicall` call in `chainreader.go` through `r.calls.Multicall`; keep `r.chain` for headers, balances, receipts, and `chain.Decimals`. This seam permits exact OEV call-vector characterization without reimplementing Multicall3.

- [ ] **Step 5: Implement exact market decode and IRM tuple conversion**

```go
func decodeMarketState(data []byte, params abiMarketParams) (morpho.MarketState, bool) {
	out, err := morphoB.UnpackMarket(data)
	if err != nil || out.TotalSupplyAssets == nil || out.TotalSupplyShares == nil ||
	out.TotalBorrowAssets == nil || out.TotalBorrowShares == nil || out.LastUpdate == nil ||
	out.Fee == nil || params.Lltv == nil || !out.LastUpdate.IsUint64() || out.LastUpdate.Sign() <= 0 {
		return morpho.MarketState{}, false
	}
	return morpho.MarketState{
		TotalSupplyAssets: out.TotalSupplyAssets,
		TotalSupplyShares: out.TotalSupplyShares,
		TotalBorrowAssets: out.TotalBorrowAssets,
		TotalBorrowShares: out.TotalBorrowShares,
		LastUpdate: out.LastUpdate.Uint64(),
		Fee: out.Fee,
		Lltv: params.Lltv,
	}, true
}

func irmParams(params abiMarketParams) irmbinding.Struct0 {
	return irmbinding.Struct0{
		LoanToken: params.LoanToken,
		CollateralToken: params.CollateralToken,
		Oracle: params.Oracle,
		Irm: params.Irm,
		Lltv: params.Lltv,
	}
}

func irmMarket(state morpho.MarketState) irmbinding.Struct1 {
	return irmbinding.Struct1{
		TotalSupplyAssets: state.TotalSupplyAssets,
		TotalSupplyShares: state.TotalSupplyShares,
		TotalBorrowAssets: state.TotalBorrowAssets,
		TotalBorrowShares: state.TotalBorrowShares,
		LastUpdate: new(big.Int).SetUint64(state.LastUpdate),
		Fee: state.Fee,
	}
}
```

- [ ] **Step 6: Implement the two pinned batches and fail-closed filtering**

`ReadMarketStatesAt` must:

1. reject nil/negative block numbers and a zero Morpho address;
2. sort market IDs for deterministic call order;
3. issue `market(id)` calls to Morpho with `r.calls.MulticallAt(ctx, calls, blockNumber)`;
4. decode successful exact tuples and skip failed/invalid/uninitialized (`lastUpdate == 0`) markets;
5. assign a real zero rate only when `params.Irm == common.Address{}`;
6. issue `borrowRateView(params, exactState)` only for retained non-zero IRMs at the same block;
7. omit any market whose non-zero IRM call fails or cannot decode; and
8. return RPC-level errors with operation context.

Require each result vector to match its slot vector. Skip the second batch entirely when every retained market has a zero IRM.

Use this concrete result assembly shape:

```go
type rateSlot struct {
	id common.Hash
}

states := make(map[common.Hash]morpho.MarketState, len(ids))
var rateCalls []chain.Call
var rateSlots []rateSlot
for i, id := range ids {
	if i >= len(marketResults) || !marketResults[i].Success {
		continue
	}
	state, ok := decodeMarketState(marketResults[i].ReturnData, params[id])
	if !ok {
		continue
	}
	if params[id].Irm == (common.Address{}) {
		state.BorrowRatePerSec = new(big.Int)
		states[id] = state
		continue
	}
	rateSlots = append(rateSlots, rateSlot{id: id})
	rateCalls = append(rateCalls, chain.Call{
		Target: params[id].Irm,
		AllowFailure: true,
		Data: irmB.PackBorrowRateView(irmParams(params[id]), irmMarket(state)),
	})
	states[id] = state
}
```

After the rate batch, delete a non-zero-IRM state unless its corresponding result succeeds and decodes to a non-nil rate.

- [ ] **Step 7: Make adapter discovery return the callback's Morpho deployment**

Add `morpho common.Address` to `adapterSnapshot`. In `readAdapterSnapshot`, resolve it via `callbackB.PackMORPHO()`/`UnpackMORPHO` and reject a zero/malformed result. This replaces the duplicate callback lookup in `testMonitor` and gives production API mode the authoritative Morpho address without a YAML address.

- [ ] **Step 8: Add failing monitor tests for header time and market intersection**

Refactor `apiMarketSnapshot` tests so `state.timestamp` remains the Morpho market `LastUpdate`, while a separate pinned header supplies snapshot `blockTime`. Add a pure intersection test:

```go
func TestAPIMarketSnapshotAppliesPinnedStates(t *testing.T) {
	marketA := common.HexToHash("0x01")
	marketB := common.HexToHash("0x02")
	snap := apiMarketSnapshot{
		markets: map[common.Hash]MarketInfo{marketA: {}, marketB: {}},
		prices: map[common.Hash]*big.Int{marketA: big.NewInt(1), marketB: big.NewInt(2)},
		params: map[common.Hash]abiMarketParams{marketA: {}, marketB: {}},
		serve: map[common.Hash]bool{marketA: true, marketB: true},
	}
	snap.applyPinnedStates(map[common.Hash]morpho.MarketState{
		marketA: {Fee: big.NewInt(3), BorrowRatePerSec: big.NewInt(4)},
	})
	if len(snap.markets) != 1 || snap.markets[marketA].State.BorrowRatePerSec.Cmp(big.NewInt(4)) != 0 {
		t.Fatalf("applied markets = %+v", snap.markets)
	}
	if _, ok := snap.params[marketB]; ok {
		t.Fatal("market without pinned accrual state was not removed")
	}
}
```

Implement the intersection on all parallel snapshot maps:

```go
func (s *apiMarketSnapshot) applyPinnedStates(states map[common.Hash]morpho.MarketState) {
	for id, info := range s.markets {
		state, ok := states[id]
		if !ok {
			delete(s.markets, id)
			delete(s.prices, id)
			delete(s.params, id)
			delete(s.serve, id)
			continue
		}
		info.State = state
		s.markets[id] = info
	}
}
```

- [ ] **Step 9: Enrich API snapshots before quote/position reads**

In `apiMonitor.refresh`, after selecting one API block:

```go
blockNumber := new(big.Int).SetUint64(apiSnap.block)
header, err := m.reader.chain.HeaderByNumber(ctx, blockNumber)
if err != nil || header == nil || header.Number == nil || header.Number.Cmp(blockNumber) != 0 {
	m.log.Error(err, "pinned Morpho block header unreadable; keeping cache", "block", apiSnap.block)
	return
}
states, err := m.reader.ReadMarketStatesAt(ctx, adapter.morpho, apiSnap.params, blockNumber)
if err != nil {
	m.log.Error(err, "pinned Morpho state refresh failed; keeping cache", "block", apiSnap.block)
	return
}
apiSnap.applyPinnedStates(states)
if len(apiSnap.markets) == 0 {
	m.log.V(1).Info("pinned Morpho refresh returned no usable markets", "block", apiSnap.block)
	return
}
```

Store `blockTime: header.Time`. `marketInfoFromAPI` continues parsing `state.timestamp` only into `MarketState.LastUpdate`; remove `apiMarketView.blockTime` and never copy market last-update time into snapshot epoch time.

- [ ] **Step 10: Give the test monitor the same exact rate path**

Pass the starting header number into `readMarkets`. Use `ReadMarketStatesAt` for tuples/rates and one pinned oracle batch:

```go
func (m *testMonitor) readMarkets(
	ctx context.Context,
	morphoAddr common.Address,
	params map[common.Hash]abiMarketParams,
	blockNumber *big.Int,
) (map[common.Hash]MarketInfo, map[common.Hash]*big.Int, error) {
	states, err := m.reader.ReadMarketStatesAt(ctx, morphoAddr, params, blockNumber)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]common.Hash, 0, len(states))
	for id := range states {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, common.Hash.Cmp)
	calls := make([]chain.Call, len(ids))
	for i, id := range ids {
		calls[i] = chain.Call{Target: params[id].Oracle, AllowFailure: true, Data: oracleB.PackPrice()}
	}
	results, err := m.reader.calls.MulticallAt(ctx, calls, blockNumber)
	if err != nil {
		return nil, nil, err
	}
	if len(results) != len(calls) {
		return nil, nil, errors.Errorf("testMonitor prices: got %d results, want %d", len(results), len(calls))
	}
	markets := make(map[common.Hash]MarketInfo, len(ids))
	prices := make(map[common.Hash]*big.Int, len(ids))
	for i, id := range ids {
		if !results[i].Success {
			continue
		}
		price, unpackErr := oracleB.UnpackPrice(results[i].ReturnData)
		if unpackErr != nil || price == nil || price.Sign() <= 0 {
			continue
		}
		markets[id] = MarketInfo{Params: params[id], State: states[id]}
		prices[id] = price
	}
	return markets, prices, nil
}
```

Call it as `m.readMarkets(ctx, adapter.morpho, want, header.Number)`. Retain the ending-header equality check. Remove `decodeTestMarketState`; the shared `decodeMarketState` is now the only exact market tuple decoder.

- [ ] **Step 11: Add independent non-zero-fee accrual vectors**

Extend `internal/morpho/math_test.go` with fixed expected values calculated from Morpho's Solidity formula, including fee-share minting:

```go
func TestAccruedMarketStateWithFeeVector(t *testing.T) {
	market := MarketState{
		TotalSupplyAssets: mustBig("1000000000000"),
		TotalSupplyShares: mustBig("1000000000000"),
		TotalBorrowAssets: mustBig("500000000000"),
		TotalBorrowShares: mustBig("500000000000"),
		LastUpdate: 1_000,
		Fee: mustBig("100000000000000000"),
		Lltv: mustBig("860000000000000000"),
		BorrowRatePerSec: mustBig("1000000000000"),
	}
	got := AccruedMarketState(market, 1_100)
	if got.TotalBorrowAssets.Cmp(mustBig("500050002500")) != 0 {
		t.Fatalf("borrow assets = %s", got.TotalBorrowAssets)
	}
	if got.TotalSupplyAssets.Cmp(mustBig("1000050002500")) != 0 {
		t.Fatalf("supply assets = %s", got.TotalSupplyAssets)
	}
	if got.TotalSupplyShares.Cmp(mustBig("1000005000029")) != 0 {
		t.Fatalf("supply shares = %s", got.TotalSupplyShares)
	}
	debt := BorrowedAssetsAt(
		PositionState{BorrowShares: mustBig("250000000000")},
		got.TotalBorrowAssets,
		got.TotalBorrowShares,
	)
	if debt.Cmp(mustBig("250024501202")) != 0 {
		t.Fatalf("borrower debt = %s", debt)
	}
}
```

The fixed constants follow Morpho's three-term Taylor growth, downward WAD multiplication, fee multiplication, virtual-shares `ToSharesDown`, and upward borrower `ToAssetsUp` in that order.

- [ ] **Step 12: Run pinned-state, monitor, and accrual tests and confirm GREEN**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test -race ./internal/solvers/redstoneoev ./internal/morpho -run 'Test(ReadMarketStatesAt|APIMarketSnapshot|MarketInfoFromAPI|AccruedMarketState)' -count=1
```

Expected: PASS; both batches use block 123 in the boundary fixture, failed non-zero IRMs disappear, zero IRM is exactly zero, API last-update remains distinct from header time, and accrual includes the exact fee.

- [ ] **Step 13: Reconcile Morpho source-of-truth documentation**

Update `docs/OEV-PLAN.md` to state:

- GraphQL discovers markets and at-risk positions;
- callback `MORPHO()` identifies the deployment;
- exact market totals/fee and IRM rate are read at the API-selected block;
- the block header supplies `snapshot.blockTime` while GraphQL `state.timestamp` remains `LastUpdate`;
- failed non-zero IRM reads exclude the market; and
- the hot path only consumes the completed immutable snapshot.

Remove the stale assertions that production reads all Morpho state only from GraphQL and that only the test monitor calls Morpho on-chain.

- [ ] **Step 14: Commit coherent accrual inputs and boundary characterization**

```bash
git add internal/solvers/redstoneoev/chainreader.go internal/solvers/redstoneoev/chainreader_test.go internal/solvers/redstoneoev/chainreader_boundary_test.go internal/solvers/redstoneoev/monitor.go internal/solvers/redstoneoev/monitor_test.go internal/solvers/redstoneoev/testmonitor.go internal/morpho/math_test.go docs/OEV-PLAN.md
git commit -m "fix(oev): read coherent Morpho accrual state"
```

---

### Task 5: Bound the Beam Frontier Before Deep Materialization

**Files:**
- Modify: `internal/solvers/redstoneoev/bundle.go:99-233,262-340`
- Modify: `internal/solvers/redstoneoev/solver_test.go:979-1437`
- Create: `internal/solvers/redstoneoev/bundle_benchmark_test.go`
- Modify: `docs/OEV-PLAN.md:327-352`

**Interfaces:**
- Consumes: existing `searchBundle`, deterministic `sortedScoredLegs`, replay sizing, gas-fit predicate, and score function.
- Produces: `type bundleTrial`, `type bundleTrialHeap`, `func keepBundleTrial`, `func materializeBundleTrial`, and test-only `bundleSearchStats` through `searchBundleWithStats`.
- Preserves: width 64, full candidate scanning, score ordering, earlier-sequence tie preference, gas-derived depth, shared collateral budgets, sequential same-market replay, and existing selected bundles.

- [ ] **Step 1: Add the benchmark harness and record the pre-change baseline**

Before editing `bundle.go`, create `bundle_benchmark_test.go` with the benchmark that will be reused after the change:

```go
func BenchmarkBundleSearch(b *testing.B) {
	tests := []struct {
		name string
		candidates int
		depth int
	}{
		{name: "N100_D2", candidates: 100, depth: 2},
		{name: "N1000_D2", candidates: 1000, depth: 2},
		{name: "N1000_D8", candidates: 1000, depth: 8},
		{name: "N10000_D2", candidates: 10000, depth: 2},
	}
	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			s := &Solver{cfg: &Config{}, log: logr.Discard()}
			legs := make([]scoredLeg, tc.candidates)
			for i := range legs {
				legs[i] = scoredFor(byte(i%255+1), big.NewInt(int64(tc.candidates-i+1)))
				legs[i].Borrower = common.BigToAddress(big.NewInt(int64(i + 1)))
			}
			usable := fixedGasUnits(defaultPriceUpdateFeeds) + gasFirstAcquireLeg
			if tc.depth > 1 {
				usable += uint64(tc.depth-1) * gasAdditionalAcquireLeg
			}
			gasState := &gasPredictorState{
				FreeAssets: big.NewInt(0),
				Withdrawable: big.NewInt(0),
				Acquire: map[common.Address]*big.Int{{}: new(big.Int).SetUint64(^uint64(0))},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, _ = s.searchBundle(legs, gasState, headerGasLimitForUsable(usable), defaultPriceUpdateFeeds, func(bundle chosenBundle) *big.Int {
					return new(big.Int).Set(bundle.grossLoan)
				})
			}
		})
	}
}
```

Run it against the existing full-frontier implementation and retain the terminal output with the task notes:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/redstoneoev -run '^$' -bench '^BenchmarkBundleSearch$' -benchmem -count=3
```

Expected: all four sub-benchmarks run against the pre-change search and report `ns/op`, `B/op`, and `allocs/op`.

- [ ] **Step 2: Add failing stable-top-K and materialization-bound tests**

Add test-only stats and exercise a candidate set much wider than 64:

```go
func TestSearchBundleMaterializesOnlyBoundedFrontier(t *testing.T) {
	s := &Solver{cfg: &Config{}, log: logr.Discard()}
	legs := make([]scoredLeg, 1000)
	for i := range legs {
		legs[i] = scoredFor(byte(i%255+1), big.NewInt(int64(1000-i)))
		legs[i].Borrower = common.BigToAddress(big.NewInt(int64(i + 1)))
	}
	depthGas := fixedGasUnits(defaultPriceUpdateFeeds) + gasFirstAcquireLeg + gasAdditionalAcquireLeg
	stats := &bundleSearchStats{}
	_, ok := s.searchBundleWithStats(
		legs,
		&gasPredictorState{FreeAssets: big.NewInt(0), Withdrawable: big.NewInt(0), Acquire: map[common.Address]*big.Int{{}: big.NewInt(1_000_000)}},
		headerGasLimitForUsable(depthGas),
		defaultPriceUpdateFeeds,
		func(b chosenBundle) *big.Int { return new(big.Int).Set(b.grossLoan) },
		stats,
	)
	if !ok {
		t.Fatal("search returned no bundle")
	}
	if max := netBundleBeamWidth * 2; stats.materialized > max {
		t.Fatalf("materialized states = %d, want <= %d", stats.materialized, max)
	}
	if max := netBundleBeamWidth + 1; stats.probeLegBuffers > max {
		t.Fatalf("probe leg buffers = %d, want <= %d for depth two", stats.probeLegBuffers, max)
	}
}

func TestBundleTrialHeapKeepsEarlierEqualScore(t *testing.T) {
	h := &bundleTrialHeap{}
	for seq := uint64(0); seq < netBundleBeamWidth+10; seq++ {
		keepBundleTrial(h, bundleTrial{score: big.NewInt(1), grossLoan: big.NewInt(1), seq: seq})
	}
	for _, trial := range *h {
		if trial.seq >= netBundleBeamWidth {
			t.Fatalf("late equal-score trial retained: seq=%d", trial.seq)
		}
	}
}
```

- [ ] **Step 3: Run beam tests and confirm RED**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/redstoneoev -run 'Test(SearchBundleMaterializesOnlyBoundedFrontier|BundleTrialHeapKeepsEarlierEqualScore)' -count=1
```

Expected: FAIL to compile with undefined heap, stats, and `searchBundleWithStats` types/functions.

- [ ] **Step 4: Introduce lightweight trial descriptors and stable heap ordering**

Import `container/heap` and define:

```go
type bundleTrial struct {
	parent bundleSearchState
	next replayedScoredLeg
	idx int
	grossLoan *big.Int
	score *big.Int
	seq uint64
}

type bundleTrialHeap []bundleTrial

func (h bundleTrialHeap) Len() int { return len(h) }
func (h bundleTrialHeap) Less(i, j int) bool {
	if cmp := h[i].score.Cmp(h[j].score); cmp != 0 {
		return cmp < 0
	}
	return h[i].seq > h[j].seq
}
func (h bundleTrialHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *bundleTrialHeap) Push(v any) { *h = append(*h, v.(bundleTrial)) }
func (h *bundleTrialHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

func trialBetter(a, b bundleTrial) bool {
	if cmp := a.score.Cmp(b.score); cmp != 0 {
		return cmp > 0
	}
	return a.seq < b.seq
}

func freezeBundleTrial(trial bundleTrial) bundleTrial {
	trial.grossLoan = new(big.Int).Set(trial.grossLoan)
	trial.score = new(big.Int).Set(trial.score)
	return trial
}

func keepBundleTrial(h *bundleTrialHeap, trial bundleTrial) {
	if h.Len() < netBundleBeamWidth {
		heap.Push(h, freezeBundleTrial(trial))
		return
	}
	if trialBetter(trial, (*h)[0]) {
		heap.Pop(h)
		heap.Push(h, freezeBundleTrial(trial))
	}
}
```

The freeze is mandatory because probe score/gross values are reusable scratch objects. Only trials
that actually enter the 64-wide heap receive owned `big.Int` copies.

- [ ] **Step 5: Separate replay deltas from full market-map clones**

Change `replayedScoredLeg` to carry only the affected replay delta:

```go
type replayedScoredLeg struct {
	scored scoredLeg
	marketID common.Hash
	marketInfo MarketInfo
	marketState morpho.MarketState
	borrower common.Address
	position morpho.PositionState
}
```

For static legs, return only `scored`. For replay legs, replace the existing `nextMarket` construction with the exact delta:

```go
return replayedScoredLeg{
	scored: nextLeg,
	marketID: id,
	marketInfo: ms.info,
	marketState: replay.Market,
	borrower: cand.Borrower,
	position: replay.Position,
}, true
```

`marketInfo` is an immutable shallow reference during probing; `marketState` is the replay result
already required to score the candidate. Do not clone its big integers, every market, or every
previously touched borrower until the descriptor survives the heap.

- [ ] **Step 6: Build a shallow score bundle, then deep-materialize retained trials only**

Allocate one reusable candidate-leg buffer and one reusable gross value per parent beam state, not per
candidate. `scoreFn` and `bundleFitsGasLimit` consume the candidate synchronously and must not retain
the scratch slice:

```go
func probeBundle(parent chosenBundle, next scoredLeg, legs []bundleLeg, gross *big.Int) chosenBundle {
	legs[len(parent.legs)] = next.bundleLeg
	gross.Add(parent.grossLoan, next.profit)
	return chosenBundle{legs: legs, grossLoan: gross}
}
```

Materialization performs copy-on-write only after a trial survives top 64:

```go
func materializeBundleTrial(trial bundleTrial) bundleSearchState {
	bundle := cloneChosenBundle(trial.parent.bundle)
	appendScoredLeg(&bundle, trial.next.scored)
	bundle.grossLoan.Set(trial.grossLoan)
	next := bundleSearchState{
		bundle: bundle,
		consumed: cloneCollateralBudget(trial.parent.consumed),
		markets: maps.Clone(trial.parent.markets),
		used: cloneUsed(trial.parent.used),
		score: new(big.Int).Set(trial.score),
	}
	next.used[trial.idx] = true
	commitCollateralBudget(next.consumed, trial.next.scored)
	if trial.next.marketID != (common.Hash{}) {
		previous := trial.parent.markets[trial.next.marketID]
		positions := maps.Clone(previous.positions)
		if positions == nil {
			positions = make(map[common.Address]morpho.PositionState)
		}
		positions[trial.next.borrower] = clonePositionState(trial.next.position)
		info := trial.next.marketInfo
		info.State = cloneMarketState(trial.next.marketState)
		next.markets[trial.next.marketID] = bundleMarketState{
			info: info,
			positions: positions,
		}
	}
	return next
}

func cloneChosenBundle(bundle chosenBundle) chosenBundle {
	return chosenBundle{
		legs: cloneBundleLegs(bundle.legs),
		grossLoan: new(big.Int).Set(bundle.grossLoan),
	}
}
```

- [ ] **Step 7: Replace full frontier construction with bounded probing**

Keep `searchBundle` as the production-compatible wrapper:

```go
type bundleSearchStats struct {
	materialized   int
	probeLegBuffers int
}

func (s *Solver) searchBundle(scored []scoredLeg, gasState *gasPredictorState, gasLimit uint64, feedCount int, scoreFn func(chosenBundle) *big.Int) (bundleSearchState, bool) {
	return s.searchBundleWithStats(scored, gasState, gasLimit, feedCount, scoreFn, nil)
}
```

At each depth, iterate all current beam states and all candidates, skip used/budget/replay/gas failures, assign a monotonically increasing `seq`, and pass the descriptor to `keepBundleTrial`. After the scan:

1. copy the at-most-64 heap entries;
2. sort them by score descending then sequence ascending;
3. materialize them in that order;
4. increment `stats.materialized` for each materialized state;
5. update `best` from the first state only when its score is strictly greater; and
6. continue to the next gas-derived depth.

Use this loop shape so probing precedes materialization:

```go
seq := uint64(0)
for depth := 0; depth < maxDepth && depth < len(group); depth++ {
	frontier := &bundleTrialHeap{}
	for _, state := range beam {
		probeLegs := make([]bundleLeg, len(state.bundle.legs)+1)
		copy(probeLegs, state.bundle.legs)
		probeGross := new(big.Int)
		if stats != nil {
			stats.probeLegBuffers++
		}
		for i, scored := range group {
			if state.used[i] {
				continue
			}
			next, ok := s.replayScoredLeg(scored, state.markets)
			if !ok || !fitsCollateralBudget(state.consumed, next.scored) {
				continue
			}
			candidate := probeBundle(state.bundle, next.scored, probeLegs, probeGross)
			if !bundleFitsGasLimit(candidate, gasState, gasLimit, feedCount) {
				continue
			}
			keepBundleTrial(frontier, bundleTrial{
				parent: state,
				next: next,
				idx: i,
				grossLoan: candidate.grossLoan,
				score: scoreFn(candidate),
				seq: seq,
			})
			seq++
		}
	}
	if frontier.Len() == 0 {
		break
	}
	trials := slices.Clone(*frontier)
	slices.SortFunc(trials, func(a, b bundleTrial) int {
		return cmp.Or(b.score.Cmp(a.score), cmp.Compare(a.seq, b.seq))
	})
	nextBeam := make([]bundleSearchState, len(trials))
	for i, trial := range trials {
		nextBeam[i] = materializeBundleTrial(trial)
		if stats != nil {
			stats.materialized++
		}
	}
	if len(best.bundle.legs) == 0 || nextBeam[0].score.Cmp(best.score) > 0 {
		best = nextBeam[0]
	}
	beam = nextBeam
}
```

Never pre-truncate `sortedScoredLegs`; the existing “searches past gross-only candidate window” regression must stay green.

- [ ] **Step 8: Run all bundle parity tests and confirm GREEN**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test -race ./internal/solvers/redstoneoev -run 'Test(Bundle|SelectBundle|SelectNetBundle|SearchBundle)' -count=1
```

Expected: PASS, including same-market sequential replay, lower-gross net winners, non-monotonic score, candidate-after-512, deterministic equal-profit order, and the new materialization bound.

- [ ] **Step 9: Smoke-test the unchanged benchmark harness after the refactor**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/redstoneoev -run '^$' -bench '^BenchmarkBundleSearch/N100_D2$' -benchmem -count=1
```

Expected: the benchmark compiles against the unchanged `searchBundle` production signature and reports allocations for the bounded implementation.

- [ ] **Step 10: Record post-change benchmark output**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/redstoneoev -run '^$' -bench '^BenchmarkBundleSearch$' -benchmem -count=3
```

Expected: all four sub-benchmarks complete. Compare `allocs/op` and `B/op` against the Step 1 baseline; the correctness gate is the explicit materialization bound, not a machine-dependent nanosecond threshold.

- [ ] **Step 11: Update complexity documentation**

Replace the `O(W*N)` transient-state/full-sort claim in `docs/OEV-PLAN.md` section 4.3. State that
each depth scans up to `W*N` probes, allocates one reusable candidate-leg buffer per parent state,
copies score/gross ownership only when a descriptor enters the `W=64` heap, retains that heap in
`O(log W)` per accepted comparison, sorts at most 64 descriptors, and deep-materializes at most `W`
states. Keep the full candidate scan and gas-derived depth discussion.

- [ ] **Step 12: Commit the bounded frontier**

```bash
git add internal/solvers/redstoneoev/bundle.go internal/solvers/redstoneoev/solver_test.go internal/solvers/redstoneoev/bundle_benchmark_test.go docs/OEV-PLAN.md
git commit -m "perf(oev): bound beam frontier materialization"
```

---

### Task 6: Complete OEV Characterization, Documentation Audit, and Verification

**Files:**
- Modify: `internal/solvers/redstoneoev/chainreader_boundary_test.go`
- Modify: `internal/solvers/redstoneoev/solver_test.go`
- Modify: `docs/OEV-PLAN.md`
- Modify: `README.md`
- Modify: `config/redstone-oev.example.yaml`

**Interfaces:**
- Consumes: all behavior delivered by Tasks 1-5, the prior generic `MulticallAt`, and the repository's Go 1.26.5/check-generated gates.
- Produces: a complete OEV money/trust-boundary regression suite and reconciled public/internal documentation.
- Preserves: live/fork suites behind their existing build tags; default CI remains hermetic.

- [ ] **Step 1: Add a complete failure matrix to the pinned Multicall characterization**

Extend `chainreader_boundary_test.go` with concrete market/rate results and per-case batch sequences:

```go
func TestReadMarketStatesAtFailureMatrix(t *testing.T) {
	marketID := common.HexToHash("0x01")
	morphoAddr := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	params := map[common.Hash]abiMarketParams{
		marketID: {
			Irm: common.HexToAddress("0x00000000000000000000000000000000000000a1"),
			Lltv: mustBig("860000000000000000"),
		},
	}
	validMarket := chain.CallResult{Success: true, ReturnData: packOut(
		t, morphoABI, "market",
		big.NewInt(1000), big.NewInt(900), big.NewInt(500), big.NewInt(450),
		big.NewInt(100), mustBig("100000000000000000"),
	)}
	validRate := chain.CallResult{Success: true, ReturnData: packOut(t, irmABI, "borrowRateView", big.NewInt(182418302))}
	tests := []struct {
		name       string
		results    [][]chain.CallResult
		wantMarket bool
	}{
		{name: "market reverted", results: [][]chain.CallResult{{{Success: false}}}},
		{name: "market malformed", results: [][]chain.CallResult{{{Success: true, ReturnData: []byte{1}}}}},
		{name: "rate reverted", results: [][]chain.CallResult{{validMarket}, {{Success: false}}}},
		{name: "rate malformed", results: [][]chain.CallResult{{validMarket}, {{Success: true, ReturnData: []byte{1}}}}},
		{name: "all valid", results: [][]chain.CallResult{{validMarket}, {validRate}}, wantMarket: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &recordingMulticaller{results: tc.results}
			r := &reader{calls: fake, log: logr.Discard()}
			got, err := r.ReadMarketStatesAt(t.Context(), morphoAddr, params, big.NewInt(123))
			if err != nil {
				t.Fatal(err)
			}
			_, retained := got[marketID]
			if retained != tc.wantMarket {
				t.Fatalf("retained = %v, want %v", retained, tc.wantMarket)
			}
			for _, block := range fake.blocks {
				if block == nil || block.Cmp(big.NewInt(123)) != 0 {
					t.Fatalf("multicall block = %v, want 123", block)
				}
			}
		})
	}

	rpcErr := errors.New("rpc unavailable")
	fake := &recordingMulticaller{err: rpcErr}
	r := &reader{calls: fake, log: logr.Discard()}
	got, err := r.ReadMarketStatesAt(t.Context(), morphoAddr, params, big.NewInt(123))
	if err == nil || got != nil {
		t.Fatalf("RPC failure = (%v, %v), want nil map and error", got, err)
	}
}
```

The malformed/reverted cases omit the market, RPC-level failure returns no partial map, and every attempted batch uses the same non-nil block.

- [ ] **Step 2: Add one integrated duplicate-result metrics test**

Construct a Prometheus registry through the existing metrics helper, attach it to a seeded solver, and assert the counter directly:

```go
func TestLiquidationResultDuplicateIncrementsMetricOnce(t *testing.T) {
	s, _ := seededSolver(t)
	registry := prometheus.NewRegistry()
	mx, err := newMetrics(registry)
	if err != nil {
		t.Fatal(err)
	}
	s.metrics = mx
	frame := func(id string) []byte {
		return marshal(LiquidationResult{
			Op: "liquidation-result",
			ID: id,
			Data: LiquidationResultData{
				Success: false,
				Liquidator: s.cfg.Callback.Hex(),
			},
		})
	}
	s.handleMessage(t.Context(), frame("same"))
	s.handleMessage(t.Context(), frame("same"))
	if got := testutil.ToFloat64(mx.failedLiq); got != 1 {
		t.Fatalf("failed metric after duplicate = %v, want 1", got)
	}
	s.handleMessage(t.Context(), frame("distinct"))
	if got := testutil.ToFloat64(mx.failedLiq); got != 2 {
		t.Fatalf("failed metric after distinct result = %v, want 2", got)
	}
}
```

- [ ] **Step 3: Run the complete hermetic OEV suite**

Run:

```bash
GOTOOLCHAIN=go1.26.5 go test -race -cover ./internal/solvers/redstoneoev ./internal/morpho -count=1
```

Expected: PASS with no race reports. Coverage output is informational, but every new branch from Tasks 1-5 must be exercised by a named regression test.

- [ ] **Step 4: Audit stale documentation claims**

Run:

```bash
rg -n 'all Morpho data from|only path that reads Morpho|cachedState.*updatedAt|sorts at most W\*N|O\(W\*N\)|ws://dev-rwa|ws\.url' docs/OEV-PLAN.md README.md config/redstone-oev.example.yaml
```

Expected: no stale claims that production Morpho state is GraphQL-only, one ops stamp covers every value, the beam materializes/sorts `W*N` states, or remote plaintext WS is valid. Remaining `ws.url` matches must describe WSS production use and loopback-only plaintext testing.

- [ ] **Step 5: Reconcile the live refinements section**

In `docs/OEV-PLAN.md` section 10, remove completed gaps for WebSocket security, aggregate freshness, duplicate result processing, zero accrual inputs, or unbounded frontier materialization if present. Keep unrelated live calibration/refinement items unchanged. Add no speculative future subsystem.

- [ ] **Step 6: Run formatting and focused static checks**

Run:

```bash
GOTOOLCHAIN=go1.26.5 golangci-lint run --fix
git diff --check
GOTOOLCHAIN=go1.26.5 go test -race ./internal/solvers/redstoneoev ./internal/morpho -count=1
```

Expected: lint autofix exits 0, `git diff --check` emits nothing, and focused race tests pass after formatting.

- [ ] **Step 7: Run the repository verification gate**

Run each command separately and inspect its fresh output:

```bash
GOTOOLCHAIN=go1.26.5 go build ./...
GOTOOLCHAIN=go1.26.5 go test -race -cover ./...
GOTOOLCHAIN=go1.26.5 golangci-lint run
GOTOOLCHAIN=go1.26.5 make check-generated
```

Expected: build succeeds, all hermetic tests pass with no race report, lint reports zero issues, and regeneration leaves no generated-code diff. If `make check-generated` changes generated files, stop and diagnose the prior generation/toolchain plan rather than committing unrelated generated output in this OEV changeset.

- [ ] **Step 8: Review the final OEV-only diff**

Run:

```bash
git status --short
git diff --stat HEAD~5
git log -5 --oneline
```

Expected: only OEV source/tests, shared Morpho math tests, and synchronized OEV docs/config/README files are present across the five implementation commits; no generated file, deployment manifest, or unrelated solver source appears.

- [ ] **Step 9: Commit final characterization/doc corrections only if Step 1, 2, or 5 changed files**

```bash
git add internal/solvers/redstoneoev/chainreader_boundary_test.go internal/solvers/redstoneoev/solver_test.go docs/OEV-PLAN.md README.md config/redstone-oev.example.yaml
git diff --cached --quiet
```

Expected: exit 1 when the final task has staged changes. In that case, run:

```bash
git commit -m "test(oev): characterize hardened boundaries"
```

If `git diff --cached --quiet` exits 0, do not create an empty commit.
