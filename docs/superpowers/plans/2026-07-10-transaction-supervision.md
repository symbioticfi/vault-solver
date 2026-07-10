# Transaction Lifecycle and Worker Supervision Implementation Plan

> **Public-port status:** This is the source-branch implementation record. Generic transaction and
> server supervision was ported, but source-only receipt-attribution workers and private deployment
> chart paths are not part of the public architecture. OEV shutdown joins its async auction-decision
> workers, and public deployment manifests remain outside this repository; use the live subsystem plans
> and README for current paths and ownership.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Make every submitted transaction outcome explicit and canonically confirmed, keep later nonces moving while earlier receipts are pending, supervise every long-lived worker, and characterize the production signer boundary.

**Architecture:** A single txmanager dispatcher remains the only nonce allocator and initial broadcaster, while one manager-owned tracker per admitted or ambiguous logical transaction handles canonical receipts and bounded same-nonce replacements. A shared HTTP-server runner and root errgroup make listener failures fatal and shutdown joinable; RFQ, 3F, and OEV consume the new lifecycle without treating ambiguity as a safe retry.

**Tech Stack:** Go 1.26.5, go-ethereum transaction/receipt primitives, github.com/go-errors/errors, logr, golang.org/x/sync/errgroup, net/http, Foundry-generated bindings already committed in the repository.

## Global Constraints

- Use Go 1.26.5 for every command in this plan; keep the language directive at go 1.26.
- Finding 1 is out of scope: do not pin reusable workflows or container base-image digests.
- Do not add a database; transaction and redemption suppression remain in memory and reconcile against authoritative chain/backend state.
- Keep the generic/integration boundary intact: transaction and HTTP lifecycle mechanisms stay under internal/{txmanager,httpserver,observability}; protocol-specific reactions stay under internal/solvers/<name>.
- Generated Go under api/ is never hand-edited.
- All operational settings come from YAML; replacement limits are not environment variables or hidden constants.
- Use github.com/go-errors/errors rather than fmt.Errorf, and logr rather than a concrete logger outside cmd wiring.
- Preserve the existing invariant that caller cancellation after enqueue cannot report “not sent” while a transaction may land.
- Result.SafeToRetry is true only before possible admission; neither a revert nor ambiguity is inferred from Err alone.
- Update user-facing examples and architecture plans in the same implementation series as behavior.
- Do not deploy, push, or open a pull request as part of this plan.

---

## File Map

Create:

- internal/txmanager/tracker.go — one logical nonce’s receipt, canonicality, replacement, and final-result state machine.
- internal/txmanager/tracker_test.go — canonical receipt, transient RPC, reorg, replacement, deadline, and shutdown tests.
- internal/httpserver/server.go — generic joinable ListenAndServe plus bounded graceful shutdown.
- internal/httpserver/server_test.go — clean cancellation, occupied-listener failure, and shutdown-error tests.
- internal/observability/observability_test.go — observability wrapper propagation and readiness handler coverage.
- internal/solvers/bridgefacilitator/redeemer_test.go — unresolved redemption suppression and authoritative resync tests.
- internal/signer/local_test.go — production hex key, encrypted keystore, recovery, transaction sender, redaction, and race characterization.

Modify:

- internal/config/config.go — validated txManager.pendingIntervalMs, feeBumpBps, and maxReplacements defaults.
- internal/config/config_test.go — replacement-policy default and invalid-boundary tests.
- config/3f.example.yaml — documented replacement policy.
- config/rfq.example.yaml — documented replacement policy.
- internal/txmanager/txmanager.go — typed outcomes, dispatcher-only nonce ownership, async tracker ownership, and joinable Start.
- internal/txmanager/txmanager_test.go — dispatcher, ambiguity, nonce, second-broadcast, and manager-stop tests.
- cmd/vault-solver/run.go — replacement-policy wiring and one root errgroup for observability, txmanager, and solvers.
- internal/observability/observability.go — return listener/shutdown errors through the generic server runner.
- internal/solvers/rfq/execution.go — state-aware transaction consumption.
- internal/solvers/rfq/execution_test.go — confirmed/reverted/rejected/unresolved matrices.
- internal/solvers/rfq/solver.go — join quote listener and order poller.
- internal/solvers/rfq/solver_test.go — listener failure cancels and joins the poller.
- internal/solvers/redstoneoev/solver.go — child context and joined settlement-attribution workers.
- internal/solvers/redstoneoev/solver_test.go — attribution worker join test.
- internal/solvers/bridgefacilitator/solver.go — in-memory unresolved-redemption set.
- internal/solvers/bridgefacilitator/redeemer.go — suppress ambiguous batches until an authoritative scan changes state.
- README.md — accurate transaction/supervision summary and corrected 3F chart filename.
- CLAUDE.md — document dispatcher/tracker concurrency rather than whole-lifecycle serialization.
- docs/3F-PLAN.md — exact result states, policy fields, and redemption ambiguity behavior.
- docs/RFQ-PLAN.md — state-aware fill handling and joined listener/poller.
- docs/OEV-PLAN.md — joined settlement attribution.

No api/ generated file changes are part of this plan.

---

### Task 1: Add the bounded replacement policy to generic configuration

**Files:**

- Modify: internal/config/config.go:65-90,120-162
- Modify: internal/config/config_test.go
- Modify: config/3f.example.yaml:28-31
- Modify: config/rfq.example.yaml:26-29

**Interfaces:**

- Consumes: Config.Load(path string) (*Config, error)
- Produces: TxManagerConfig.PendingIntervalMs int, TxManagerConfig.FeeBumpBps uint64, TxManagerConfig.MaxReplacements uint64
- Produces defaults: DefaultPendingIntervalMs = 120000, DefaultFeeBumpBps = 1250, DefaultMaxReplacements = 3
- Validation: pendingIntervalMs must be positive and at most 86400000 (24h) after defaults;
  feeBumpBps must be 1000..10000 inclusive; maxReplacements must be 1..10 inclusive; the complete
  tracking duration must fit in `time.Duration` before runtime conversion.
- Semantics: total tracking bound is (maxReplacements + 1) × pendingIntervalMs; zero means omitted and is replaced by the default.

- [ ] **Step 1: Write failing table tests for defaults and bounds**

Add these assertions to internal/config/config_test.go, using validConfig as the base fixture:

    func TestLoad_TxManagerReplacementDefaults(t *testing.T) {
        cfg, err := Load(writeTemp(t, validConfig))
        if err != nil {
            t.Fatalf("Load: %v", err)
        }
        if cfg.TxManager.PendingIntervalMs != DefaultPendingIntervalMs {
            t.Fatalf("pendingIntervalMs = %d, want %d", cfg.TxManager.PendingIntervalMs, DefaultPendingIntervalMs)
        }
        if cfg.TxManager.FeeBumpBps != DefaultFeeBumpBps {
            t.Fatalf("feeBumpBps = %d, want %d", cfg.TxManager.FeeBumpBps, DefaultFeeBumpBps)
        }
        if cfg.TxManager.MaxReplacements != DefaultMaxReplacements {
            t.Fatalf("maxReplacements = %d, want %d", cfg.TxManager.MaxReplacements, DefaultMaxReplacements)
        }
    }

    func TestLoad_RejectsInvalidTxManagerReplacementPolicy(t *testing.T) {
        cases := map[string]string{
            "negative pending interval": "pendingIntervalMs: -1",
            "pending interval above 24 hours": "pendingIntervalMs: 86400001",
            "pending interval duration overflow": "pendingIntervalMs: 9223372036854775807",
            "fee bump below client replacement floor": "feeBumpBps: 999",
            "fee bump above one hundred percent": "feeBumpBps: 10001",
            "too many replacements": "maxReplacements: 11",
        }
        for name, policy := range cases {
            t.Run(name, func(t *testing.T) {
                body := strings.Replace(validConfig, "signer:", "txManager:\n  "+policy+"\nsigner:", 1)
                if _, err := Load(writeTemp(t, body)); err == nil {
                    t.Fatalf("expected %s to be rejected", policy)
                }
            })
        }
    }

Add `strings` to the test imports. Add `time` to `internal/config/config.go` for overflow-safe duration
validation.

- [ ] **Step 2: Run the focused RED test**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/config -run 'TestLoad_(TxManagerReplacementDefaults|RejectsInvalidTxManagerReplacementPolicy)$' -count=1

Expected: FAIL to compile because the new fields and constants do not exist.

- [ ] **Step 3: Add fields, defaults, and exact validation**

Extend TxManagerConfig:

    type TxManagerConfig struct {
        Confirmations     uint64  `yaml:"confirmations"`
        MaxFeeGwei        float64 `yaml:"maxFeeGwei"`
        TipGwei           float64 `yaml:"tipGwei"`
        PendingIntervalMs int     `yaml:"pendingIntervalMs"`
        FeeBumpBps        uint64  `yaml:"feeBumpBps"`
        MaxReplacements   uint64  `yaml:"maxReplacements"`
    }

Add constants:

    const (
        DefaultConfirmations      = 2
        DefaultPendingIntervalMs  = 120_000
        DefaultFeeBumpBps         = 1_250
        DefaultMaxReplacements    = 3
        maxPendingIntervalMs      = 86_400_000
        maxConfiguredReplacements = 10
    )

Extend applyDefaults:

    if c.TxManager.PendingIntervalMs == 0 {
        c.TxManager.PendingIntervalMs = DefaultPendingIntervalMs
    }
    if c.TxManager.FeeBumpBps == 0 {
        c.TxManager.FeeBumpBps = DefaultFeeBumpBps
    }
    if c.TxManager.MaxReplacements == 0 {
        c.TxManager.MaxReplacements = DefaultMaxReplacements
    }

Add a TxManagerConfig.validate method and call it from Config.Validate before signer validation:

    func (t TxManagerConfig) validate() error {
        if t.PendingIntervalMs <= 0 || t.PendingIntervalMs > maxPendingIntervalMs {
            return errors.Errorf("txManager.pendingIntervalMs must be between 1 and %d, got %d",
                maxPendingIntervalMs, t.PendingIntervalMs)
        }
        if t.FeeBumpBps < 1_000 || t.FeeBumpBps > 10_000 {
            return errors.Errorf("txManager.feeBumpBps must be between 1000 and 10000, got %d", t.FeeBumpBps)
        }
        if t.MaxReplacements == 0 || t.MaxReplacements > maxConfiguredReplacements {
            return errors.Errorf("txManager.maxReplacements must be between 1 and %d, got %d",
                maxConfiguredReplacements, t.MaxReplacements)
        }
        interval := time.Duration(t.PendingIntervalMs) * time.Millisecond
        windows := time.Duration(t.MaxReplacements + 1)
        const maxDuration = time.Duration(1<<63 - 1)
        if interval <= 0 || interval > maxDuration/windows {
            return errors.New("txManager replacement tracking duration overflows time.Duration")
        }
        return nil
    }

- [ ] **Step 4: Document the policy in both transaction-sending examples**

Add beneath confirmations in config/3f.example.yaml and config/rfq.example.yaml:

      pendingIntervalMs: 120000         # replace a still-pending nonce after 2m; default 120000
      feeBumpBps: 1250                  # raise tip + max fee by 12.5% per same-nonce replacement
      maxReplacements: 3                # unresolved after 4 pending windows total; allowed 1..10

Keep `maxFeeGwei` documented as a hard ceiling that replacements never exceed, and document the
24-hour upper bound on one pending interval.

- [ ] **Step 5: Run focused GREEN tests and the package suite**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/config -run 'TestLoad_(TxManagerReplacementDefaults|RejectsInvalidTxManagerReplacementPolicy)$' -count=1
    GOTOOLCHAIN=go1.26.5 go test ./internal/config -count=1

Expected: PASS.

- [ ] **Step 6: Commit the configuration unit**

    git add internal/config/config.go internal/config/config_test.go config/3f.example.yaml config/rfq.example.yaml
    git commit -m "feat(txmanager): configure bounded replacement policy"

---

### Task 2: Replace blocking send/receipt handling with an explicit supervised state machine

**Files:**

- Modify: internal/txmanager/txmanager.go
- Modify: internal/txmanager/txmanager_test.go
- Create: internal/txmanager/tracker.go
- Create: internal/txmanager/tracker_test.go

**Interfaces:**

- Consumes: Backend as currently defined; HeaderByNumber, TransactionReceipt, and BlockNumber already provide the required canonicality surface.
- Produces: func (m *Manager) Start(ctx context.Context) error
- Preserves: func (m *Manager) Send(ctx context.Context, req Request) Result
- Produces states: StateNotBroadcast, StateRejected, StateBroadcastUnknown, StatePending, StateConfirmed, StateReverted, StateUnresolved.
- Produces: func (r Result) SafeToRetry() bool.
- Result.Hash is the canonical receipt hash when final, otherwise the newest signed attempt hash.
- Result.Hashes is ordered oldest to newest and contains every signed same-nonce attempt.
- Result.Err is nil only for StateConfirmed. Consumers must branch on State, not Err.

- [ ] **Step 1: Add RED tests for the public result contract**

Add to internal/txmanager/txmanager_test.go:

    func TestResultSafeToRetry(t *testing.T) {
        cases := map[State]bool{
            StateNotBroadcast:      true,
            StateRejected:          true,
            StateBroadcastUnknown: false,
            StatePending:          false,
            StateConfirmed:        false,
            StateReverted:         false,
            StateUnresolved:       false,
        }
        for state, want := range cases {
            if got := (Result{State: state}).SafeToRetry(); got != want {
                t.Errorf("state %q SafeToRetry = %v, want %v", state, got, want)
            }
        }
    }

    func TestSend_CancelBeforeEnqueueIsNotBroadcast(t *testing.T) {
        m := New(newMockBackend(), mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
        ctx, cancel := context.WithCancel(context.Background())
        cancel()
        res := m.Send(ctx, Request{To: common.HexToAddress("0x1")})
        if res.State != StateNotBroadcast || !res.SafeToRetry() || res.Hash != (common.Hash{}) {
            t.Fatalf("result = %+v, want safe not_broadcast", res)
        }
    }

- [ ] **Step 2: Add RED tests for ambiguous admission and dispatcher progress**

Extend mockBackend with synchronized, fully-defined test controls:

    type mockBackend struct {
        // keep the current mutex, fee, nonce, gas, head, send-error, sent, and receipt fields
        sentCh      chan *types.Transaction
        heldNonces map[uint64]bool
        receiptErrs []error
        headers     map[uint64]*types.Header
        pendingNonces []uint64
    }

Initialize sentCh with capacity 32, heldNonces, and headers in newMockBackend. SendTransaction records
every admitted transaction on sentCh. Unless its nonce is held, it creates a receipt whose BlockHash
equals headerFor(head).Hash(). TransactionReceipt consumes receiptErrs in order before consulting the
receipt map. HeaderByNumber returns headerFor(head) for nil and headerFor(number.Uint64()) otherwise.
PendingNonceAt consumes `pendingNonces` in order when non-empty and otherwise returns the existing
mock nonce.
Use this deterministic header helper:

    func (b *mockBackend) headerFor(number uint64) *types.Header {
        if header := b.headers[number]; header != nil {
            return types.CopyHeader(header)
        }
        return &types.Header{
            Number: new(big.Int).SetUint64(number),
            BaseFee: new(big.Int).Set(b.baseFee),
            Extra: []byte{byte(number), byte(number >> 8)},
        }
    }

releaseNonce finds the most recent sent transaction with the requested nonce, removes the hold, and
publishes its canonical successful receipt:

    func (b *mockBackend) releaseNonce(nonce uint64) {
        b.mu.Lock()
        defer b.mu.Unlock()
        delete(b.heldNonces, nonce)
        for i := len(b.sent) - 1; i >= 0; i-- {
            tx := b.sent[i]
            if tx.Nonce() == nonce {
                header := b.headerFor(b.head)
                b.receipts[tx.Hash()] = &types.Receipt{
                    Status: types.ReceiptStatusSuccessful, TxHash: tx.Hash(),
                    BlockNumber: new(big.Int).SetUint64(b.head), BlockHash: header.Hash(),
                }
                return
            }
        }
    }

Add:

    func TestSend_AmbiguousBroadcastReturnsSignedHashAndCommitsNonce(t *testing.T) {
        b := newMockBackend()
        b.sendErrs = []error{
            context.DeadlineExceeded, // initial broadcast
            context.DeadlineExceeded, // identical re-broadcast
            context.DeadlineExceeded, // first and only fee-bumped replacement
        }
        m, cancel, done := newTestManager(t, b, Config{
            PollInterval: time.Millisecond, PendingInterval: 5 * time.Millisecond,
            FeeBumpBps: 1_250, MaxReplacements: 1,
        })
        defer func() { cancel(); <-done }()

        res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
        if res.State != StateUnresolved || res.Hash == (common.Hash{}) || len(res.Hashes) == 0 {
            t.Fatalf("ambiguous result = %+v", res)
        }
        if res.SafeToRetry() {
            t.Fatal("ambiguous broadcast must never be safe to retry")
        }

        next := m.Send(context.Background(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000})
        if next.Nonce != res.Nonce+1 {
            t.Fatalf("next nonce = %d, want %d", next.Nonce, res.Nonce+1)
        }
    }

    func TestSend_SecondNonceBroadcastsWhileFirstIsPending(t *testing.T) {
        b := newMockBackend()
        b.heldNonces[7] = true
        m, cancel, done := newTestManager(t, b, Config{
            PollInterval: time.Millisecond, PendingInterval: time.Second,
            FeeBumpBps: 1_250, MaxReplacements: 1,
        })
        defer func() { cancel(); <-done }()

        first := make(chan Result, 1)
        go func() {
            first <- m.Send(context.Background(), Request{To: common.HexToAddress("0xa"), GasLimit: 21_000})
        }()
        if tx := <-b.sentCh; tx.Nonce() != 7 {
            t.Fatalf("first broadcast nonce = %d, want 7", tx.Nonce())
        }

        second := make(chan Result, 1)
        go func() {
            second <- m.Send(context.Background(), Request{To: common.HexToAddress("0xb"), GasLimit: 21_000})
        }()
        select {
        case tx := <-b.sentCh:
            if tx.Nonce() != 8 {
                t.Fatalf("second broadcast nonce = %d, want 8", tx.Nonce())
            }
        case <-time.After(time.Second):
            t.Fatal("nonce 8 was head-of-line blocked by nonce 7 receipt tracking")
        }

        b.releaseNonce(7)
        <-first
        <-second
    }

    func TestSend_AmbiguousNonceFloorSurvivesRegressedPendingNonce(t *testing.T) {
        // First seed is 7. The nonce-7 broadcast returns ambiguous "nonce too low" and is committed.
        // The next seed deliberately regresses to 6, as a stale fallback can do.
        b := newMockBackend()
        b.pendingNonces = []uint64{7, 6}
        b.sendErrs = []error{
            errors.New("nonce too low"), // initial broadcast: ambiguous and invalidates the seed
            context.DeadlineExceeded,   // identical re-broadcast remains ambiguous
            context.DeadlineExceeded,   // sole fee-bumped replacement remains ambiguous
            // The next logical transaction gets the default nil result and a canonical receipt.
        }
        m, cancel, done := newTestManager(t, b, Config{
            PollInterval: time.Millisecond, PendingInterval: 5 * time.Millisecond,
            FeeBumpBps: 1_250, MaxReplacements: 1,
        })
        defer func() { cancel(); <-done }()

        first := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
        if first.State != StateUnresolved || first.Nonce != 7 {
            t.Fatalf("first result = %+v, want unresolved nonce 7", first)
        }
        second := m.Send(context.Background(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000})
        if second.State != StateConfirmed || second.Nonce != 8 {
            t.Fatalf("second result = %+v, want confirmed at committed floor 8", second)
        }
    }

Replace newTestManager with this backward-compatible variadic helper so existing callers may omit an
override while new lifecycle tests provide short policy intervals:

    func newTestManager(t *testing.T, b Backend, overrides ...Config) (*Manager, context.CancelFunc, <-chan error) {
        t.Helper()
        cfg := Config{PollInterval: time.Millisecond}
        if len(overrides) == 1 {
            cfg = overrides[0]
            if cfg.PollInterval == 0 {
                cfg.PollInterval = time.Millisecond
            }
        }
        if len(overrides) > 1 {
            t.Fatal("newTestManager accepts at most one Config override")
        }
        m := New(b, mustSigner(t), big.NewInt(11155111), cfg, logr.Discard())
        ctx, cancel := context.WithCancel(context.Background())
        done := make(chan error, 1)
        go func() { done <- m.Start(ctx) }()
        return m, cancel, done
    }

- [ ] **Step 3: Run the dispatcher RED tests**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/txmanager -run 'Test(ResultSafeToRetry|Send_CancelBeforeEnqueueIsNotBroadcast|Send_AmbiguousBroadcastReturnsSignedHashAndCommitsNonce|Send_SecondNonceBroadcastsWhileFirstIsPending|Send_AmbiguousNonceFloorSurvivesRegressedPendingNonce)$' -count=1

Expected: FAIL to compile because State, expanded Result, replacement Config fields, and Start’s error return do not exist.

- [ ] **Step 4: Define exact states, results, sentinels, and internal tracked transaction**

In internal/txmanager/txmanager.go define:

    type State string

    const (
        StateNotBroadcast     State = "not_broadcast"
        StateRejected         State = "rejected"
        StateBroadcastUnknown State = "broadcast_unknown"
        StatePending          State = "pending"
        StateConfirmed        State = "confirmed"
        StateReverted         State = "reverted"
        StateUnresolved       State = "unresolved"
    )

    var (
        ErrManagerStopped = errors.New("txmanager stopped")
        ErrUnresolved     = errors.New("transaction outcome unresolved")
    )

    type Result struct {
        State   State
        Nonce   uint64
        Hash    common.Hash
        Hashes  []common.Hash
        Receipt *types.Receipt
        Err     error
    }

    func (r Result) SafeToRetry() bool {
        return r.State == StateNotBroadcast || r.State == StateRejected
    }

Extend Config:

    type Config struct {
        Confirmations   uint64
        MaxFeeGwei      float64
        TipGwei         float64
        PollInterval    time.Duration
        PendingInterval time.Duration
        FeeBumpBps      uint64
        MaxReplacements uint64
    }

New supplies the same defaults as config.Load for direct unit construction and adds done:

    type Manager struct {
        backend Backend
        signer  signer.Signer
        chainID *big.Int
        cfg     Config
        log     logr.Logger
        queue   chan job
        done    chan struct{}
        mu        sync.Mutex
        nonce     uint64
        nonceInit bool
        nonceFloor    uint64
        nonceFloorSet bool
        nonceExhausted bool
    }

    func New(
        backend Backend,
        s signer.Signer,
        chainID *big.Int,
        cfg Config,
        log logr.Logger,
    ) *Manager {
        if cfg.PollInterval <= 0 {
            cfg.PollInterval = defaultPollInterval
        }
        if cfg.PendingInterval <= 0 {
            cfg.PendingInterval = 2 * time.Minute
        }
        if cfg.FeeBumpBps == 0 {
            cfg.FeeBumpBps = 1_250
        }
        if cfg.MaxReplacements == 0 {
            cfg.MaxReplacements = 3
        }
        return &Manager{
            backend: backend, signer: s, chainID: new(big.Int).Set(chainID), cfg: cfg,
            log: log.WithName("txmanager"), queue: make(chan job), done: make(chan struct{}),
        }
    }

In tracker.go define:

    type trackedTx struct {
        req          Request
        nonce        uint64
        state        State
        attempts     []*types.Transaction
        admissionErr error
    }

    func (t *trackedTx) hashes() []common.Hash {
        out := make([]common.Hash, len(t.attempts))
        for i, tx := range t.attempts {
            out[i] = tx.Hash()
        }
        return out
    }

- [ ] **Step 5: Implement dispatcher-only Start, Send, and broadcast classification**

Start must serialize prepare/sign/initial broadcast, then spawn tracking without waiting for a receipt:

    func (m *Manager) Start(ctx context.Context) error {
        m.log.Info("started", "from", m.signer.Address().Hex())
        var trackers sync.WaitGroup
        defer func() {
            trackers.Wait()
            close(m.done)
            m.log.Info("stopped")
        }()

        for {
            select {
            case <-ctx.Done():
                return nil
            case j := <-m.queue:
                tracked, immediate := m.dispatch(ctx, j.req)
                if immediate != nil {
                    j.res <- *immediate
                    continue
                }
                trackers.Add(1)
                go func(tracked *trackedTx, result chan<- Result) {
                    defer trackers.Done()
                    result <- m.track(ctx, tracked)
                }(tracked, j.res)
            }
        }
    }

Send must retain the post-enqueue invariant and avoid hanging after manager shutdown:

    func (m *Manager) Send(ctx context.Context, req Request) Result {
        res := make(chan Result, 1)
        select {
        case m.queue <- job{req: req, res: res}:
        case <-ctx.Done():
            return Result{State: StateNotBroadcast, Err: ctx.Err()}
        case <-m.done:
            return Result{State: StateNotBroadcast, Err: ErrManagerStopped}
        }
        return <-res
    }

Implement classifyBroadcastError with three internal classes. The deterministic rejection allowlist is exactly:

- insufficient funds
- intrinsic gas too low
- invalid sender
- max fee per gas less than block base fee
- max priority fee per gas higher than max fee per gas
- transaction type not supported

nil and “already known” are admitted. Every other error, including timeout, EOF, nonce too low, and replacement transaction underpriced, is ambiguous.

dispatch must:

1. Return StateRejected for fee, estimation, nonce-read, construction, or signing failure.
2. Compute signed.Hash before SendTransaction.
3. Return StateRejected with Hash and Hashes for an allowlisted deterministic RPC rejection, without committing the nonce.
4. Commit the nonce for admitted or ambiguous results.
5. On every admitted/ambiguous broadcast, advance a persistent committed floor to `nonce+1` (or
   mark the nonce space exhausted at `math.MaxUint64`). A later seed is
   `max(PendingNonceAt, nonceFloor)`; a stale/fallback RPC must never make committed nonces reusable.
6. Invalidate nonceInit after an ambiguous “nonce too low” so only a future logical request re-seeds
   above that floor; never replay current calldata at a new nonce.
7. Return trackedTx with StatePending after admitted send or StateBroadcastUnknown after ambiguous send.

Keep the floor separate from the currently seeded candidate. A deterministic pre-admission rejection
does not advance it. Add a small `seedNonce` helper that applies the floor after `PendingNonceAt`, and
make `commitNonce` overflow-safe.

Use this exact immediate-result helper:

    func rejectedResult(nonce uint64, signed *types.Transaction, err error) Result {
        result := Result{State: StateRejected, Nonce: nonce, Err: err}
        if signed != nil {
            result.Hash = signed.Hash()
            result.Hashes = []common.Hash{signed.Hash()}
        }
        return result
    }

- [ ] **Step 6: Add RED canonicality and replacement tests**

Create internal/txmanager/tracker_test.go with these synchronized cases:

    func TestTrack_TransientReceiptErrorRetries(t *testing.T)
    func TestTrack_ReceiptDisappearsBeforeConfirmation(t *testing.T)
    func TestTrack_BlockHashMismatchIsNotCanonical(t *testing.T)
    func TestTrack_RevertWaitsForCanonicalConfirmations(t *testing.T)
    func TestTrack_ReplacementPreservesPayloadAndBumpsFees(t *testing.T)
    func TestTrack_ExplicitFeeCapPreventsBumpAndReturnsUnresolved(t *testing.T)
    func TestStart_CancellationReturnsUnresolvedAndJoinsTrackers(t *testing.T)
    func TestSend_AfterManagerStopsReturnsNotBroadcast(t *testing.T)

The replacement assertion must compare every invariant:

    if replacement.Nonce() != original.Nonce() ||
        replacement.To() == nil || *replacement.To() != *original.To() ||
        replacement.Value().Cmp(original.Value()) != 0 ||
        !bytes.Equal(replacement.Data(), original.Data()) ||
        replacement.Gas() != original.Gas() ||
        replacement.ChainId().Cmp(original.ChainId()) != 0 {
        t.Fatal("replacement changed logical transaction payload")
    }
    if replacement.GasTipCapCmp(original) <= 0 || replacement.GasFeeCapCmp(original) <= 0 {
        t.Fatal("replacement did not monotonically increase both EIP-1559 fee fields")
    }

The reorg test must first expose a receipt whose block hash matches the canonical header but lacks enough confirmations, then return NotFound, advance the head, and assert no result is delivered. Only a newly included receipt with a matching header may complete.

- [ ] **Step 7: Run the tracker RED tests**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/txmanager -run 'TestTrack_|TestStart_Cancellation|TestSend_AfterManagerStops' -count=1

Expected: FAIL because track, canonical re-fetching, fee bumping, and bounded unresolved completion are not implemented.

- [ ] **Step 8: Implement canonical polling and bounded same-nonce replacement**

Implement tracker.go with these exact rules:

- Poll every Config.PollInterval.
- On every poll, query TransactionReceipt for every attempt hash; never cache a receipt across polls.
- Treat ethereum.NotFound and every other receipt/header/head error as transient; retain the latest error for ErrUnresolved context and continue.
- For a receipt, fetch HeaderByNumber(receipt.BlockNumber) and require header.Hash() == receipt.BlockHash.
- Require BlockNumber >= receipt.BlockNumber + Confirmations; guard uint64 overflow by comparing big.Int values.
- Return StateConfirmed with nil Err only for a successful canonical receipt.
- Return StateReverted with the canonical receipt and a wrapped error only after the same confirmation rule.
- At each PendingInterval boundary, create at most MaxReplacements replacements.
- After the final replacement’s PendingInterval expires, return StateUnresolved.
- When ctx ends after possible admission, return StateUnresolved and wrap ctx.Err with ErrUnresolved.
- A rejected replacement never changes an old attempt’s eligibility; continue polling existing hashes until the bound.

Use ceiling arithmetic so a positive fee always increases by at least one wei:

    func bumpedFee(old *big.Int, bumpBps uint64) *big.Int {
        numerator := new(big.Int).Mul(old, new(big.Int).SetUint64(bumpBps))
        delta := new(big.Int).Quo(
            new(big.Int).Add(numerator, big.NewInt(9_999)),
            big.NewInt(10_000),
        )
        if delta.Sign() == 0 {
            delta.SetInt64(1)
        }
        return new(big.Int).Add(old, delta)
    }

Build replacement payloads only from the previous signed transaction:

    func replacementTx(previous *types.Transaction, tip, fee *big.Int) *types.Transaction {
        to := previous.To()
        return types.NewTx(&types.DynamicFeeTx{
            ChainID: previous.ChainId(),
            Nonce: previous.Nonce(),
            GasTipCap: tip,
            GasFeeCap: fee,
            Gas: previous.Gas(),
            To: to,
            Value: previous.Value(),
            Data: previous.Data(),
            AccessList: previous.AccessList(),
        })
    }

If MaxFeeGwei is explicit, convert it once to wei and reject a bump when:

    nextTip.Cmp(capWei) > 0 || nextFee.Cmp(previous.GasFeeCap()) <= 0

Clamp nextFee to capWei before that comparison. Return StateUnresolved with an error wrapping ErrUnresolved; do not silently exceed the cap.

Before the first fee-changing replacement of a StateBroadcastUnknown transaction, re-send the identical first signed transaction once. Whether that re-send says admitted, already known, or ambiguous, keep tracking its existing hash. A deterministic rejection of the re-broadcast also leaves the original ambiguous hash eligible.

- [ ] **Step 9: Update old tests to assert states and canonical block hashes**

Every successful fixture must return a receipt with BlockHash equal to the mock header hash. Replace old Err-only assertions:

    if res.State != StateConfirmed || res.Err != nil {
        t.Fatalf("result = %+v, want confirmed", res)
    }

Replace TestSend_NonceTooLowResyncsAndRetries with a fail-closed characterization:

    func TestSend_NonceTooLowIsAmbiguousAndNeverReplaysAtNewNonce(t *testing.T)

Assert only one signed logical transaction is created for that Send, StateUnresolved is returned,
SafeToRetry is false, and the next separate Send re-seeds from
`max(PendingNonceAt, committed nonce floor)`.

- [ ] **Step 10: Run txmanager GREEN tests repeatedly and under race**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/txmanager -count=20
    GOTOOLCHAIN=go1.26.5 go test -race ./internal/txmanager -count=1

Expected: PASS with no flakes and no race report.

- [ ] **Step 11: Commit the state-machine unit**

    git add internal/txmanager/txmanager.go internal/txmanager/txmanager_test.go internal/txmanager/tracker.go internal/txmanager/tracker_test.go
    git commit -m "fix(txmanager): supervise canonical transaction lifecycles"

---

### Task 3: Migrate RFQ and 3F consumers away from Err-based retry inference

**Files:**

- Modify: internal/solvers/rfq/execution.go:203-212
- Modify: internal/solvers/rfq/execution_test.go
- Modify: internal/solvers/bridgefacilitator/solver.go:39-75
- Modify: internal/solvers/bridgefacilitator/redeemer.go
- Create: internal/solvers/bridgefacilitator/redeemer_test.go
- Modify: docs/3F-PLAN.md
- Modify: docs/RFQ-PLAN.md

**Interfaces:**

- Consumes: txmanager.Result.State, Hash, Hashes, Err, SafeToRetry.
- RFQ mapping: confirmed → submitted then backend reconcile; unresolved → submitted then backend reconcile; reverted/rejected/not_broadcast → failed.
- 3F mapping: confirmed → finalized log; unresolved → suppress every request in that batch; reverted/rejected/not_broadcast → log definite failure and allow a future authoritative scan to retry.
- Produces: redeemKey{adapter, request common.Address} and Solver.pendingRedemptions map[redeemKey]struct{}.

- [ ] **Step 1: Add RED RFQ outcome-matrix tests**

Update every fake success to use StateConfirmed. Add:

    func TestExecution_UnresolvedSubmissionIsNeverRearmed(t *testing.T) {
        st, be := fillFixtures(t)
        hash := common.HexToHash("0xdead")
        txm := &fakeTxm{result: txmanager.Result{
            State: txmanager.StateUnresolved, Hash: hash, Hashes: []common.Hash{hash},
            Err: txmanager.ErrUnresolved,
        }}
        be.order.OrderStatus = "open"
        e := newExec(t, st, be, txm)

        e.syncOnce(context.Background())
        rec := st.order("o1")
        if rec == nil || rec.Status != statusSubmitted || rec.TxHash != hash {
            t.Fatalf("record = %+v, want submitted unresolved transaction", rec)
        }

        firstData := append([]byte(nil), txm.lastData...)
        e.syncOnce(context.Background())
        if !bytes.Equal(txm.lastData, firstData) {
            t.Fatal("unresolved order was submitted a second time")
        }
    }

    func TestExecution_DefiniteTransactionOutcomesFail(t *testing.T) {
        for _, state := range []txmanager.State{
            txmanager.StateNotBroadcast, txmanager.StateRejected, txmanager.StateReverted,
        } {
            t.Run(string(state), func(t *testing.T) {
                st, be := fillFixtures(t)
                txm := &fakeTxm{result: txmanager.Result{State: state, Err: errors.New("definite failure")}}
                e := newExec(t, st, be, txm)
                e.syncOnce(context.Background())
                if rec := st.order("o1"); rec == nil || rec.Status != statusFailed {
                    t.Fatalf("record = %+v, want failed", rec)
                }
            })
        }
    }

Add bytes to imports.

- [ ] **Step 2: Run RFQ RED tests**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq -run 'TestExecution_(UnresolvedSubmissionIsNeverRearmed|DefiniteTransactionOutcomesFail)$' -count=1

Expected: FAIL because execution.go currently marks every Err result failed.

- [ ] **Step 3: Implement the exact RFQ state switch**

Replace the Err-only branch with:

    switch res.State {
    case txmanager.StateConfirmed:
        e.log.Info("fill transaction confirmed", "orderId", orderID, "quoteId", exec.quoteID, "tx", res.Hash.Hex())
        e.store.markStatus(orderID, statusSubmitted, res.Hash, "")
        e.reconcileTerminalStatus(ctx, orderID)
    case txmanager.StateUnresolved:
        e.log.Error(res.Err, "fill transaction unresolved; reconciling without retry",
            "orderId", orderID, "attempt", attempt, "tx", res.Hash.Hex(), "nonce", res.Nonce)
        e.store.markStatus(orderID, statusSubmitted, res.Hash, res.Err.Error())
        e.reconcileTerminalStatus(ctx, orderID)
    case txmanager.StateNotBroadcast, txmanager.StateRejected, txmanager.StateReverted:
        e.log.Error(res.Err, "fill transaction failed definitively",
            "orderId", orderID, "attempt", attempt, "tx", res.Hash.Hex(), "state", res.State)
        e.fail(orderID, res.Err.Error())
    default:
        err := errors.Errorf("unexpected txmanager state %q", res.State)
        e.log.Error(err, "fill transaction state invalid", "orderId", orderID)
        e.store.markStatus(orderID, statusSubmitted, res.Hash, err.Error())
    }

Do not call fail for StateBroadcastUnknown or StatePending if a future internal change accidentally returns one; the conservative default is submitted/reconcile, never retry.

- [ ] **Step 4: Add RED tests for 3F unresolved redemption suppression**

Create redeemer_test.go around pure pending-set helpers:

    func TestPendingRedemptions_SuppressUntilAuthoritativeAbsence(t *testing.T) {
        adapter := common.HexToAddress("0xa")
        r1 := common.HexToAddress("0x1")
        r2 := common.HexToAddress("0x2")
        s := &Solver{pendingRedemptions: make(map[redeemKey]struct{})}

        s.recordPendingRedemptions(adapter, []common.Address{r1, r2})
        got := s.filterPendingRedemptions(adapter, []common.Address{r1, r2})
        if len(got) != 0 {
            t.Fatalf("unresolved requests were offered again: %v", got)
        }

        // An authoritative scan no longer containing r1 clears only r1.
        s.reconcilePendingRedemptions(adapter, []common.Address{r2})
        got = s.filterPendingRedemptions(adapter, []common.Address{r1, r2})
        if len(got) != 1 || got[0] != r1 {
            t.Fatalf("filtered = %v, want only %s retryable after authoritative absence", got, r1)
        }
    }

    func TestRedeemResult_UnresolvedRecordsWholeBatch(t *testing.T) {
        adapter := common.HexToAddress("0xa")
        batch := []common.Address{common.HexToAddress("0x1"), common.HexToAddress("0x2")}
        s := &Solver{pendingRedemptions: make(map[redeemKey]struct{}), log: logr.Discard()}
        s.handleRedeemResult(adapter, batch, txmanager.Result{
            State: txmanager.StateUnresolved, Err: txmanager.ErrUnresolved,
        })
        if got := s.filterPendingRedemptions(adapter, batch); len(got) != 0 {
            t.Fatalf("unresolved batch not suppressed: %v", got)
        }
    }

- [ ] **Step 5: Run 3F RED tests**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/bridgefacilitator -run 'Test(PendingRedemptions|RedeemResult_)' -count=1

Expected: FAIL to compile because redeemKey and pending-set helpers do not exist.

- [ ] **Step 6: Implement single-Run-goroutine redemption suppression**

Add to Solver:

    type redeemKey struct {
        adapter common.Address
        request common.Address
    }

Add this field to the existing Solver type without moving or renaming its other fields:

    pendingRedemptions map[redeemKey]struct{}

Initialize the map in factory. It needs no mutex: discover, redeem, and reconcile all execute on the one Solver.Run goroutine; txmanager.Send returns its result to that same goroutine.

Implement:

    func (s *Solver) recordPendingRedemptions(adapter common.Address, requests []common.Address) {
        for _, request := range requests {
            s.pendingRedemptions[redeemKey{adapter: adapter, request: request}] = struct{}{}
        }
    }

    func (s *Solver) reconcilePendingRedemptions(adapter common.Address, ready []common.Address) {
        present := make(map[common.Address]struct{}, len(ready))
        for _, request := range ready {
            present[request] = struct{}{}
        }
        for key := range s.pendingRedemptions {
            if key.adapter == adapter {
                if _, ok := present[key.request]; !ok {
                    delete(s.pendingRedemptions, key)
                }
            }
        }
    }

    func (s *Solver) filterPendingRedemptions(adapter common.Address, ready []common.Address) []common.Address {
        out := make([]common.Address, 0, len(ready))
        for _, request := range ready {
            if _, pending := s.pendingRedemptions[redeemKey{adapter: adapter, request: request}]; !pending {
                out = append(out, request)
            }
        }
        return out
    }

After readyToRedeem succeeds, reconcile first and filter second. If nothing remains, return. handleRedeemResult uses:

    switch res.State {
    case txmanager.StateConfirmed:
        s.log.Info("finalized ready requests", "count", len(batch), "tx", res.Hash.Hex())
    case txmanager.StateUnresolved:
        s.recordPendingRedemptions(adapter, batch)
        s.log.Error(res.Err, "redeem transaction unresolved; suppressing batch until chain resync",
            "requests", len(batch), "tx", res.Hash.Hex(), "nonce", res.Nonce)
    case txmanager.StateNotBroadcast, txmanager.StateRejected, txmanager.StateReverted:
        s.log.Error(res.Err, "redeem transaction failed definitively",
            "requests", len(batch), "tx", res.Hash.Hex(), "state", res.State)
    default:
        s.recordPendingRedemptions(adapter, batch)
        s.log.Error(errors.Errorf("unexpected txmanager state %q", res.State),
            "redeem transaction state invalid; suppressing conservatively", "requests", len(batch))
    }

- [ ] **Step 7: Update solver plans beside the behavior**

In docs/3F-PLAN.md replace TxResult{Hash, Receipt, Err} and whole-lifecycle serialization with the exact seven states, SafeToRetry rule, dispatcher/tracker split, canonical receipt check, replacement bounds, and pending redemption set.

In docs/RFQ-PLAN.md state:

- confirmed fills enter submitted and reconcile;
- unresolved fills also enter submitted and never re-arm from local failure;
- reverted/rejected/not_broadcast fills enter failed;
- txmanager ambiguity is never inferred from Err.

- [ ] **Step 8: Run both solver suites GREEN**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/rfq ./internal/solvers/bridgefacilitator -count=1
    GOTOOLCHAIN=go1.26.5 go test -race ./internal/solvers/rfq ./internal/solvers/bridgefacilitator -count=1

Expected: PASS.

- [ ] **Step 9: Commit consumer migration**

    git add internal/solvers/rfq/execution.go internal/solvers/rfq/execution_test.go \
      internal/solvers/bridgefacilitator/solver.go internal/solvers/bridgefacilitator/redeemer.go \
      internal/solvers/bridgefacilitator/redeemer_test.go docs/3F-PLAN.md docs/RFQ-PLAN.md
    git commit -m "fix(solvers): reconcile unresolved transaction outcomes"

---

### Task 4: Supervise HTTP listeners, txmanager, and solver workers

**Files:**

- Create: internal/httpserver/server.go
- Create: internal/httpserver/server_test.go
- Modify: internal/observability/observability.go:82-130
- Create: internal/observability/observability_test.go
- Modify: cmd/vault-solver/run.go:66-122
- Modify: internal/solvers/rfq/execution.go:76-88
- Modify: internal/solvers/rfq/solver.go:124-176
- Modify: internal/solvers/rfq/solver_test.go

**Interfaces:**

- Produces: func httpserver.ServeUntil(ctx context.Context, srv *http.Server, shutdownTimeout time.Duration) error
- Produces: func observability.ServeUntil(ctx context.Context, srv *http.Server) error
- Consumes: func (*txmanager.Manager).Start(context.Context) error
- RFQ execution loop becomes func (e *executionService) run(ctx context.Context, interval time.Duration) error and returns nil on cancellation.

- [ ] **Step 1: Write RED tests for the joinable HTTP runner**

Create internal/httpserver/server_test.go:

    func TestServeUntil_CancellationIsClean(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        cancel()
        srv := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
        if err := ServeUntil(ctx, srv, time.Second); err != nil {
            t.Fatalf("ServeUntil: %v", err)
        }
    }

    func TestServeUntil_OccupiedAddressIsFatal(t *testing.T) {
        ln, err := net.Listen("tcp", "127.0.0.1:0")
        if err != nil {
            t.Fatal(err)
        }
        defer ln.Close()
        srv := &http.Server{Addr: ln.Addr().String(), Handler: http.NewServeMux()}
        err = ServeUntil(context.Background(), srv, time.Second)
        if err == nil || !strings.Contains(err.Error(), "listen") {
            t.Fatalf("error = %v, want listener failure", err)
        }
    }

Also add `TestServeUntil_ShutdownDeadlineForcesCloseAndJoins`: start a request whose handler remains
active past a short shutdown timeout, cancel the parent context, and assert `ServeUntil` returns the
shutdown deadline error rather than blocking on the listener child. Release the handler during test
cleanup so the test itself leaves no goroutine behind.

- [ ] **Step 2: Run HTTP runner RED tests**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/httpserver -count=1

Expected: FAIL because the package and ServeUntil do not exist.

- [ ] **Step 3: Implement ServeUntil with one joined listener child**

Create server.go:

    package httpserver

    import (
        "context"
        "net"
        "net/http"
        "time"

        "github.com/go-errors/errors"
    )

    func ServeUntil(ctx context.Context, srv *http.Server, shutdownTimeout time.Duration) error {
        listener, err := net.Listen("tcp", srv.Addr)
        if err != nil {
            return errors.Errorf("listen %q: %w", srv.Addr, err)
        }
        serveErr := make(chan error, 1)
        go func() { serveErr <- srv.Serve(listener) }()

        select {
        case err := <-serveErr:
            if err == nil || errors.Is(err, http.ErrServerClosed) {
                return nil
            }
            return errors.Errorf("serve %q: %w", srv.Addr, err)
        case <-ctx.Done():
        }

        shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
        shutdownErr := srv.Shutdown(shutdownCtx)
        cancel()
        if shutdownErr != nil {
            // A timed-out graceful shutdown can leave Serve running. Force it closed before join.
            _ = srv.Close()
        }
        err := <-serveErr
        if err != nil && !errors.Is(err, http.ErrServerClosed) {
            return errors.Errorf("serve %q: %w", srv.Addr, err)
        }
        if shutdownErr != nil {
            return errors.Errorf("shutdown %q: %w", srv.Addr, shutdownErr)
        }
        return nil
    }

Always receive `serveErr` after `Shutdown`, even when graceful shutdown fails. The `Close` fallback is
required first: a timed-out `Shutdown` can otherwise leave `Serve` running and deadlock the join.

- [ ] **Step 4: Make observability errors return to the caller**

Replace its internal goroutine/select with:

    const shutdownTimeout = 5 * time.Second

    func ServeUntil(ctx context.Context, srv *http.Server) error {
        if err := httpserver.ServeUntil(ctx, srv, shutdownTimeout); err != nil {
            return errors.Errorf("observability server: %w", err)
        }
        return nil
    }

Use github.com/go-errors/errors, remove logr from this function, and add a wrapper test showing an occupied address returns an error containing “observability server”.

- [ ] **Step 5: Add RED RFQ listener-failure supervision test**

In solver_test.go bind an address first, build an RFQ Solver with a fake backend, empty active store, non-nil server, and no adapters, then call Run:

    func TestRun_ListenerFailureCancelsAndJoinsPoller(t *testing.T) {
        ln, err := net.Listen("tcp", "127.0.0.1:0")
        if err != nil {
            t.Fatal(err)
        }
        defer ln.Close()

        st := newStore(time.Now)
        exec := &executionService{
            orderLimit: 1, backend: &fakeBackend{}, store: st,
            inflight: make(map[string]bool), log: logr.Discard(),
        }
        s := &Solver{
            cfg: &Config{ListenAddr: ln.Addr().String(), PollInterval: time.Hour},
            server: &server{sharedSecret: "test", quotes: &quoteService{}, log: logr.Discard()},
            exec: exec, log: logr.Discard(),
        }
        err = s.Run(context.Background())
        if err == nil || !strings.Contains(err.Error(), "quote server") {
            t.Fatalf("Run error = %v, want fatal quote listener error", err)
        }
    }

The empty quoteService is safe because this test never sends a /quote request; route registration does
not dereference it.

- [ ] **Step 6: Run supervision RED tests**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/httpserver ./internal/observability ./internal/solvers/rfq \
      -run 'Test(ServeUntil_|Run_ListenerFailure)' -count=1

Expected: HTTP helper tests pass after Step 3; observability/RFQ tests fail until their callers return and join errors.

- [ ] **Step 7: Put the RFQ listener and poller in one errgroup**

Make executionService.run return nil when ctx is done:

    case <-ctx.Done():
        return nil

In Solver.Run:

    g, gctx := errgroup.WithContext(ctx)
    g.Go(func() error {
        if err := httpserver.ServeUntil(gctx, httpSrv, 5*time.Second); err != nil {
            return errors.Errorf("rfq: quote server: %w", err)
        }
        return nil
    })
    g.Go(func() error {
        return s.exec.run(gctx, s.cfg.PollInterval)
    })
    if err := g.Wait(); err != nil {
        return err
    }
    return ctx.Err()

Remove the unjoined errCh/listener goroutine, unjoined poller goroutine, and manual Shutdown block.

- [ ] **Step 8: Put observability, txmanager, and all solvers in the root errgroup**

After all dependencies and solvers have been constructed:

    g, gctx := errgroup.WithContext(ctx)
    g.Go(func() error { return observability.ServeUntil(gctx, httpSrv) })
    g.Go(func() error { return txm.Start(gctx) })
    for _, slv := range solvers {
        slv := slv
        g.Go(func() error { return solver.Run(gctx, slv, log) })
    }
    health.SetReady(true)
    defer health.SetReady(false)
    return g.Wait()

Delete both earlier bare go calls. Probes now become live immediately before readiness rather than during dependency construction; update the comment accordingly. An observability bind error must be returned by g.Wait and cancel txmanager/solvers.

Wire Task 1’s policy:

    PendingInterval: time.Duration(cfg.TxManager.PendingIntervalMs) * time.Millisecond,
    FeeBumpBps: cfg.TxManager.FeeBumpBps,
    MaxReplacements: cfg.TxManager.MaxReplacements,

- [ ] **Step 9: Run GREEN lifecycle tests**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/httpserver ./internal/observability ./internal/solvers/rfq ./cmd/vault-solver -count=1
    GOTOOLCHAIN=go1.26.5 go test -race ./internal/httpserver ./internal/observability ./internal/solvers/rfq ./internal/txmanager -count=1

Expected: PASS; no goroutine or race failures.

- [ ] **Step 10: Commit shared supervision**

    git add internal/httpserver/server.go internal/httpserver/server_test.go \
      internal/observability/observability.go internal/observability/observability_test.go \
      cmd/vault-solver/run.go internal/solvers/rfq/execution.go \
      internal/solvers/rfq/solver.go internal/solvers/rfq/solver_test.go
    git commit -m "fix(runtime): supervise servers and transaction workers"

---

### Task 5: Join OEV settlement-attribution work

**Files:**

- Modify: internal/solvers/redstoneoev/solver.go:59-84,159-179,338-355
- Modify: internal/solvers/redstoneoev/solver_test.go
- Modify: docs/OEV-PLAN.md:91-95

**Interfaces:**

- Produces: Solver.attributionWG sync.WaitGroup.
- Produces: func (s *Solver) launchSettlementAttribution(ctx context.Context, txHash string, pred reservedBid).
- Preserves: func (s *Solver) attributeSettlementGas(context.Context, string, reservedBid).

- [ ] **Step 1: Write a RED test that proves attribution is joinable**

Add a narrow test seam to Solver:

    attributeFn func(context.Context, string, reservedBid)

The factory assigns s.attributeFn = s.attributeSettlementGas after construction.

Add:

    func TestSettlementAttributionWorkerIsJoined(t *testing.T) {
        entered := make(chan struct{})
        release := make(chan struct{})
        s := &Solver{}
        s.attributeFn = func(context.Context, string, reservedBid) {
            close(entered)
            <-release
        }
        s.launchSettlementAttribution(context.Background(), "0x01", reservedBid{})
        <-entered

        joined := make(chan struct{})
        go func() {
            s.attributionWG.Wait()
            close(joined)
        }()
        select {
        case <-joined:
            t.Fatal("Wait returned while attribution was still running")
        default:
        }
        close(release)
        select {
        case <-joined:
        case <-time.After(time.Second):
            t.Fatal("attribution worker was not joined")
        }
    }

- [ ] **Step 2: Run the OEV RED test**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/redstoneoev -run TestSettlementAttributionWorkerIsJoined -count=1

Expected: FAIL to compile because attributionWG, attributeFn, and launchSettlementAttribution do not exist.

- [ ] **Step 3: Implement launch and cancellation-safe Run ownership**

Add:

    attributionWG sync.WaitGroup
    attributeFn    func(context.Context, string, reservedBid)

Implement:

    func (s *Solver) launchSettlementAttribution(ctx context.Context, txHash string, pred reservedBid) {
        s.attributionWG.Add(1)
        go func() {
            defer s.attributionWG.Done()
            s.attributeFn(ctx, txHash, pred)
        }()
    }

Replace the bare go call in handleMessage. In Run create a child context and cancel it whenever ws.Run returns:

    runCtx, cancel := context.WithCancel(ctx)
    var wg sync.WaitGroup
    wg.Add(2)
    go func() { defer wg.Done(); s.mon.run(runCtx) }()
    go func() { defer wg.Done(); s.opsLoop(runCtx) }()
    err := s.ws.Run(runCtx)
    cancel()
    wg.Wait()
    // ws.Run joins its read pump before returning, so no later handleMessage can call Add here.
    s.attributionWG.Wait()
    return err

This Add/Wait ordering is mandatory: never begin attributionWG.Wait before ws.Run has joined the WebSocket read pump.

- [ ] **Step 4: Update OEV architecture documentation**

Change docs/OEV-PLAN.md to state that Run owns and joins monitor, ops, WebSocket read/write pumps, and every bounded settlement-attribution receipt read. Mention the ordering guarantee that prevents WaitGroup Add racing with Wait.

- [ ] **Step 5: Run OEV GREEN and race tests**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/solvers/redstoneoev -run 'TestSettlementAttributionWorkerIsJoined|Test.*Liquidation' -count=1
    GOTOOLCHAIN=go1.26.5 go test -race ./internal/solvers/redstoneoev -count=1

Expected: PASS with no WaitGroup misuse or race report.

- [ ] **Step 6: Commit OEV worker ownership**

    git add internal/solvers/redstoneoev/solver.go internal/solvers/redstoneoev/solver_test.go docs/OEV-PLAN.md
    git commit -m "fix(oev): join settlement attribution workers"

---

### Task 6: Characterize the production signer boundary

**Files:**

- Create: internal/signer/local_test.go
- Verify only: internal/signer/local.go
- Verify only: internal/signer/signer.go

**Interfaces:**

- Consumes: FromConfig, NewFromHexKey, NewFromKeystore, SignHash, SignTx.
- Establishes: 65-byte R||S||V with V in 27..28, correct recovered address, EIP-155 sender binding, concurrent safety, and redacted secret failures.
- Production code should remain unchanged unless a characterization exposes an actual contract violation.

- [ ] **Step 1: Record the missing characterization as RED**

Run:

    test -f internal/signer/local_test.go

Expected: exit status 1 because the production signer has no direct test file.

- [ ] **Step 2: Add hex/env, recovery, and transaction-sender tests**

Create local_test.go with the known Anvil key and expected address:

    const localTestKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
    var localTestAddress = common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")

    func TestFromConfig_HexEnvironmentAndHashRecovery(t *testing.T) {
        t.Setenv("TEST_SIGNER_KEY", "  0x"+localTestKey+"  ")
        s, err := FromConfig(config.SignerConfig{KeyEnv: "TEST_SIGNER_KEY"})
        if err != nil {
            t.Fatal(err)
        }
        if s.Address() != localTestAddress {
            t.Fatalf("address = %s, want %s", s.Address(), localTestAddress)
        }
        digest := crypto.Keccak256Hash([]byte("vault-solver signer characterization"))
        sig, err := s.SignHash(digest)
        if err != nil {
            t.Fatal(err)
        }
        if len(sig) != 65 || (sig[64] != 27 && sig[64] != 28) {
            t.Fatalf("signature shape = len %d V %d", len(sig), sig[64])
        }
        recovery := append([]byte(nil), sig...)
        recovery[64] -= 27
        pub, err := crypto.SigToPub(digest.Bytes(), recovery)
        if err != nil {
            t.Fatal(err)
        }
        if got := crypto.PubkeyToAddress(*pub); got != localTestAddress {
            t.Fatalf("recovered = %s, want %s", got, localTestAddress)
        }
    }

    func TestLocalSignTx_BindsEIP155SenderAndChain(t *testing.T) {
        s, err := NewFromHexKey(localTestKey)
        if err != nil {
            t.Fatal(err)
        }
        chainID := big.NewInt(11155111)
        tx := types.NewTransaction(7, common.HexToAddress("0x1234"), big.NewInt(5), 21_000, big.NewInt(1e9), []byte{1, 2})
        signed, err := s.SignTx(tx, chainID)
        if err != nil {
            t.Fatal(err)
        }
        sender, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
        if err != nil {
            t.Fatal(err)
        }
        if sender != localTestAddress || signed.ChainId().Cmp(chainID) != 0 {
            t.Fatalf("sender/chain = %s/%s, want %s/%s", sender, signed.ChainId(), localTestAddress, chainID)
        }
    }

- [ ] **Step 3: Add encrypted-keystore and redaction tests**

Use crypto.HexToECDSA, keystore.EncryptKey with LightScryptN/LightScryptP, and t.TempDir:

    func TestFromConfig_EncryptedKeystore(t *testing.T) {
        key, err := crypto.HexToECDSA(localTestKey)
        if err != nil {
            t.Fatal(err)
        }
        encrypted, err := keystore.EncryptKey(
            &keystore.Key{Address: localTestAddress, PrivateKey: key},
            "correct horse battery staple", keystore.LightScryptN, keystore.LightScryptP,
        )
        if err != nil {
            t.Fatal(err)
        }
        path := filepath.Join(t.TempDir(), "key.json")
        if err := os.WriteFile(path, encrypted, 0o600); err != nil {
            t.Fatal(err)
        }
        t.Setenv("TEST_KEYSTORE_PASSWORD", "correct horse battery staple")
        s, err := FromConfig(config.SignerConfig{
            KeystorePath: path, PassphraseEnv: "TEST_KEYSTORE_PASSWORD",
        })
        if err != nil || s.Address() != localTestAddress {
            t.Fatalf("signer/error = %v/%v", s, err)
        }

        t.Setenv("TEST_KEYSTORE_PASSWORD", "SENSITIVE-WRONG-PASSPHRASE")
        _, err = FromConfig(config.SignerConfig{
            KeystorePath: path, PassphraseEnv: "TEST_KEYSTORE_PASSWORD",
        })
        if err == nil || strings.Contains(err.Error(), "SENSITIVE-WRONG-PASSPHRASE") {
            t.Fatalf("wrong-passphrase error leaked secret: %v", err)
        }
    }

    func TestNewFromHexKey_DoesNotEchoMalformedSecret(t *testing.T) {
        const secret = "SENSITIVE-not-a-private-key"
        _, err := NewFromHexKey(secret)
        if err == nil || strings.Contains(err.Error(), secret) {
            t.Fatalf("malformed-key error leaked secret: %v", err)
        }
    }

- [ ] **Step 4: Add a concurrent SignHash/SignTx race characterization**

Use 32 goroutines × 100 iterations, one immutable signer, a buffered error channel, and one WaitGroup. Each iteration signs a unique digest and a legacy transaction; collect any error without calling testing.T from worker goroutines:

    func TestLocalSigner_ConcurrentUse(t *testing.T) {
        s, err := NewFromHexKey(localTestKey)
        if err != nil {
            t.Fatal(err)
        }
        const workers, iterations = 32, 100
        errs := make(chan error, workers*iterations*2)
        var wg sync.WaitGroup
        for worker := 0; worker < workers; worker++ {
            worker := worker
            wg.Add(1)
            go func() {
                defer wg.Done()
                for i := 0; i < iterations; i++ {
                    digest := crypto.Keccak256Hash([]byte(strconv.Itoa(worker)), []byte(strconv.Itoa(i)))
                    if _, err := s.SignHash(digest); err != nil {
                        errs <- err
                    }
                    tx := types.NewTransaction(uint64(worker*iterations+i), localTestAddress, big.NewInt(0), 21_000, big.NewInt(1), nil)
                    if _, err := s.SignTx(tx, big.NewInt(1)); err != nil {
                        errs <- err
                    }
                }
            }()
        }
        wg.Wait()
        close(errs)
        for err := range errs {
            t.Error(err)
        }
    }

- [ ] **Step 5: Run signer GREEN tests under race**

Run:

    GOTOOLCHAIN=go1.26.5 go test ./internal/signer -count=1
    GOTOOLCHAIN=go1.26.5 go test -race ./internal/signer -count=1

Expected: PASS without production changes and without a race report. If a test fails, stop and diagnose the signer contract before editing local.go.

- [ ] **Step 6: Commit signer characterization**

    git add internal/signer/local_test.go
    git commit -m "test(signer): characterize production signing paths"

---

### Task 7: Reconcile runtime documentation and run the complete gate

**Files:**

- Modify: README.md
- Modify: CLAUDE.md
- Review/finish: docs/3F-PLAN.md
- Review/finish: docs/RFQ-PLAN.md
- Review/finish: docs/OEV-PLAN.md
- Review/finish: config/3f.example.yaml
- Review/finish: config/rfq.example.yaml

**Interfaces:**

- Documents the exact Task 1/2/3/4/5 behavior; introduces no new runtime interface.
- Corrects README’s 3F deployment values path to .github/chart/vault-solver-3f-sepolia.yaml.

- [ ] **Step 1: Run a stale-claim RED scan**

Run:

    rg -n 'TxResult\\{Hash, Receipt, Err\\}|one worker goroutine drains|go observability\\.ServeUntil|go txm\\.Start|go s\\.exec\\.run|go s\\.attributeSettlementGas|\\.github/chart/3f-sepolia\\.yaml' README.md CLAUDE.md docs internal cmd

Expected before reconciliation: at least the stale TxResult/worker wording and wrong chart filename match.

- [ ] **Step 2: Update README and CLAUDE with exact operator semantics**

README must say:

- txmanager serializes nonce allocation and initial broadcast, not receipt waiting;
- canonical confirmation/replacement trackers continue independently;
- pendingIntervalMs, feeBumpBps, and maxReplacements bound same-nonce replacement;
- explicit maxFeeGwei is never exceeded;
- observability/listener failure is process-fatal and shutdown joins workers;
- the 3F chart path is .github/chart/vault-solver-3f-sepolia.yaml.

CLAUDE’s concurrency section must say:

- the dispatcher alone allocates/commits nonces;
- trackers may poll and broadcast same-nonce replacements concurrently;
- a solver must branch on Result.State/SafeToRetry, never Err alone;
- every new goroutine must be owned and joined by its component’s Run/Start.

- [ ] **Step 3: Finish plan reconciliation**

docs/3F-PLAN.md must contain the exact seven state values, replacement defaults, canonical block-hash check, unresolved redemption suppression, and current test count without claiming a whole-lifecycle single worker.

docs/RFQ-PLAN.md must describe listener/poller errgroup ownership and unresolved→submitted reconciliation.

docs/OEV-PLAN.md must name settlement-attribution receipt reads among joined workers.

Remove completed transaction/supervision gaps from each live deferred-work section rather than leaving contradictory prose.

- [ ] **Step 4: Run the stale-claim GREEN scan**

Run:

    ! rg -n 'TxResult\\{Hash, Receipt, Err\\}|go observability\\.ServeUntil|go txm\\.Start|go s\\.exec\\.run|go s\\.attributeSettlementGas|\\.github/chart/3f-sepolia\\.yaml' README.md CLAUDE.md docs internal cmd

Expected: exit status 0 from the leading negation, with no matches printed.

- [ ] **Step 5: Format and run focused suites one last time**

Run:

    GOTOOLCHAIN=go1.26.5 golangci-lint run --fix
    GOTOOLCHAIN=go1.26.5 go test -race ./internal/txmanager ./internal/httpserver ./internal/observability \
      ./internal/signer ./internal/solvers/rfq ./internal/solvers/bridgefacilitator ./internal/solvers/redstoneoev -count=1

Expected: formatter/linter reports no unfixable issue; all focused packages PASS under race.

- [ ] **Step 6: Run the repository completion gate**

Run in this exact order:

    GOTOOLCHAIN=go1.26.5 go build ./...
    GOTOOLCHAIN=go1.26.5 go test -race -cover ./...
    GOTOOLCHAIN=go1.26.5 golangci-lint run
    GOTOOLCHAIN=go1.26.5 make check-generated
    git diff --check

Expected:

- go build exits 0;
- every package test passes and emits coverage without a race report;
- golangci-lint reports 0 issues;
- check-generated reports no committed generated-code drift;
- git diff --check emits nothing.

- [ ] **Step 7: Inspect scope and transaction invariants**

Run:

    git status --short
    git diff --stat HEAD~7
    rg -n 'res\\.Err != nil' internal/solvers internal/txmanager
    rg -n 'State(Unresolved|Confirmed|Reverted|Rejected|NotBroadcast)|SafeToRetry' internal/solvers

Expected:

- no api/ generated files changed;
- no finding-1 workflow/base-image pinning appeared;
- no transaction-sending solver uses only res.Err != nil to decide retry safety;
- RFQ and 3F explicitly name the relevant result states;
- all new long-lived goroutines have a visible WaitGroup or errgroup owner.

- [ ] **Step 8: Commit final documentation reconciliation**

    git add README.md CLAUDE.md docs/3F-PLAN.md docs/RFQ-PLAN.md docs/OEV-PLAN.md \
      config/3f.example.yaml config/rfq.example.yaml
    git commit -m "docs(runtime): document transaction supervision"

---

## Completion Criteria

- A transport-timeout broadcast returns a nonzero signed hash, consumes the nonce locally, and is never marked safe to retry.
- One pending receipt cannot prevent a later nonce from being signed and broadcast.
- Every receipt is re-fetched and checked against the canonical header until the configured confirmation depth.
- Transient receipt/header/head failures remain inside the bounded tracker instead of becoming a solver retry.
- Same-nonce replacements preserve all payload fields, monotonically increase both fee fields, and respect explicit maxFeeGwei.
- Exhausted/cap-blocked/canceled admitted work returns StateUnresolved and never reuses that nonce for different calldata.
- RFQ records unresolved work as submitted and reconciles; 3F suppresses unresolved request batches until authoritative state changes.
- Observability bind errors cancel the process; RFQ joins listener and poller; txmanager joins trackers; OEV joins attribution reads.
- Production signer tests recover the expected hash signer and EIP-155 transaction sender in both key-loading modes and pass under race.
- All documentation describes the implemented state names, config keys, ownership model, and deployment chart path exactly.
- The complete Go 1.26.5 build, race/coverage, lint, generated-drift, and diff checks pass.
