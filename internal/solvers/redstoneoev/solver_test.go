package redstoneoev

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solver"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

// seedAdapter is the LiquidLane adapter stamped into the seeded market, so tests can assert it flows
// snapshot → leg → operationData.
var seedAdapter = common.HexToAddress("0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b")
var seedCallback = common.HexToAddress("0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1")
var seedCollateral = common.HexToAddress("0x0000000000000000000000000000000000000c01")
var seedBidWei = mustBig("500000000000000")

const (
	seedOracleHex         = "0xfED5bC312C7139743bc3ab21Ef92f5AeB353339D"
	seedHealthyPrice      = "5000000000000000000000000000"
	seedLiquidatablePrice = "1550000000000000000000000000"
)

func newQuote(maxRate string, maxAssets *big.Int) defaultstrategy.AdapterQuote {
	return defaultstrategy.AdapterQuote{
		MaxRate: mustBig(maxRate), MaxAssets: maxAssets,
		LoanScale: exp10(6), CollScale: exp10(18),
	}
}

// seededSolver wires a Solver that does no chain/WS I/O: a monitor whose snapshot is pre-populated
// (RedStone source), a stateCache with healthy accounting, and an in-memory signer — exactly the
// surface buildBid reads. nowFn drives accrual/breaker timing deterministically.
func seededSolver(t *testing.T) (*Solver, *testSigner) {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	sgnr := &testSigner{key: key, addr: crypto.PubkeyToAddress(key.PublicKey)}

	id := common.HexToHash("0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5")
	oracle := common.HexToAddress(seedOracleHex)

	seed := defaultstrategy.SnapshotSeed{
		Markets: map[common.Hash]defaultstrategy.MarketInfo{
			id: {Params: defaultstrategy.MarketParams{Oracle: oracle, CollateralToken: seedCollateral, Lltv: mustBig("860000000000000000")}, State: goldenMarket()},
		},
		// Cached API/test state price. The hot path still evaluates candidates at the auction frame price.
		Prices: map[common.Hash]*big.Int{id: mustBig(seedLiquidatablePrice)},
		Quotes: map[common.Hash]defaultstrategy.AdapterQuote{
			// The single adapter's quote: sells the RWA at ~$1780 (≈1% under the auctioned $1800.9); ample liquidity.
			id: newQuote("1780000000000000000000", mustBig("100000000000")),
		},
		// Independently-tracked at-risk positions — the SOLE candidate source
		// now that the frame's pushed positions are no longer consumed. Both fixture borrowers are seeded so
		// workerCandidates surfaces them, evaluated at the auction frame price. The captured frame still
		// carries these same positions, but they're ignored: candidates come from snap.Positions.
		Positions: map[common.Hash]map[common.Address]morpho.PositionState{
			id: {
				// 0x629d… — goldenBorrower (1.0 TCOL, borrowShares 1685600000000000).
				common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE"): goldenBorrower(),
				// 0x378a… — the frame's second borrower.
				common.HexToAddress("0x378a49c640fd9eea888a6a553caae441e2fdebc6"): {
					BorrowShares: mustBig("1582399974653062"), Collateral: mustBig("1000000000000000000"),
				},
			},
		},
		Block:     100,
		BlockTime: 1781243340,
		UpdatedAt: auctionClock()(),
	}

	cfg := &Config{
		Executor:            common.HexToAddress("0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD"),
		Adapter:             seedAdapter,
		Callback:            seedCallback,
		MaxTxGasPrice:       big.NewInt(1_000_000_000),
		ExecutorStateMaxAge: defaultExecutorStateMaxAge,
	}

	s := &Solver{
		cfg:     cfg,
		chainID: big.NewInt(11155111),
		nonces:  &nonceStore{},
		breaker: newBreaker(3, time.Hour),
		seen:    newSeenAuctions(maxSeenAuctions),
		log:     logr.Discard(),
		deps:    solver.Deps{Signer: sgnr},
		// Disconnected WS client: Send just buffers into its channel, which tests drain to capture solves.
		ws: newWSClient(wsConfig{URL: "wss://test", APIKey: "k", Topics: []string{"t"}}, logr.Discard(), func(context.Context, []byte) {}),
	}
	strategyCfg := defaultstrategy.ConfigForTest(defaultstrategy.Config{
		BidWei:      seedBidWei,
		MaxStateAge: defaultExecutorStateMaxAge,
		Sizing:      defaultstrategy.SizingParams{AllowFullLiquidation: true, SwapHaircutBps: 0},
	})
	s.strategy = defaultstrategy.NewWithSnapshotForTest(strategyCfg, seedAdapter, seedCallback, seed, logr.Discard(), sgnr)
	seedDefaultDecisionState(t, s, mustBig("2500000000"))
	// Healthy accounting: deposit clears MIN_DEPOSIT.
	s.state.store(cachedState{
		Exec:      ExecutorState{Nonce: big.NewInt(7), Deposit: mustBig("100000000000000000"), Locked: false},
		Adapter:   seedAdapterSnapshot(),
		GasLimit:  2_000_000,
		UpdatedAt: auctionClock()(),
	})
	return s, sgnr
}

func seedAdapterSnapshot() types.AdapterSnapshot {
	return types.AdapterSnapshot{
		Address:      seedAdapter,
		Vault:        common.HexToAddress("0x0000000000000000000000000000000000000a10"),
		Loan:         common.HexToAddress("0x0000000000000000000000000000000000000a11"),
		LoanDecimals: 6,
		FreeAssets:   mustBig("100000000000"),
		Withdrawable: mustBig("100000000000"),
		Redeemable: []types.RedeemableSnapshot{{
			Asset:          seedCollateral,
			Decimals:       18,
			MaxRate:        mustBig("1780000000000000000000"),
			MaxAssets:      mustBig("100000000000"),
			AcquireBalance: big.NewInt(0),
		}},
		Filler: true,
	}
}

func seedDefaultDecisionState(t testFataler, s *Solver, rate *big.Int) {
	t.Helper()
	seedDefaultDecisionStateAt(t, s, rate, auctionClock()())
}

func seedDefaultDecisionStateAt(t testFataler, s *Solver, rate *big.Int, at time.Time) {
	t.Helper()
	seedDefaultDecisionStateWithCallbackBalance(t, s, rate, mustBig("1000000000000000000"), at)
}

func seedDefaultDecisionStateWithCallbackBalance(t testFataler, s *Solver, rate, callbackNative *big.Int, at time.Time) {
	t.Helper()
	defaultStrategyOf(t, s).StoreDecisionStateForTest(rate, callbackNative, at)
}

type testFataler interface {
	Helper()
	Fatalf(format string, args ...any)
}

func snapshotOf(t testFataler, s *Solver) *defaultstrategy.SnapshotSeed {
	t.Helper()
	snap := defaultStrategyOf(t, s).SnapshotForTest()
	return &snap
}

func storeSnapshot(t testFataler, s *Solver, snap *defaultstrategy.SnapshotSeed) {
	t.Helper()
	defaultStrategyOf(t, s).StoreSnapshotForTest(*snap)
}

func defaultStrategyOf(t testFataler, s *Solver) *defaultstrategy.Strategy {
	t.Helper()
	strategy, ok := s.strategy.(*defaultstrategy.Strategy)
	if ok {
		return strategy
	}
	t.Fatalf("unexpected strategy type %T", s.strategy)
	return nil
}

type recordingBidStrategy struct {
	called bool
	input  types.BidInput
}

func (s *recordingBidStrategy) Run(context.Context) {}

func (s *recordingBidStrategy) DecideBid(_ context.Context, input types.BidInput) (types.BidOutput, error) {
	s.called = true
	s.input = input
	return types.BidOutput{
		Decision:      types.DecisionBid,
		BidAmount:     big.NewInt(1),
		OperationData: []byte{0x01},
	}, nil
}

type blockingBidStrategy struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingBidStrategy) Run(context.Context) {}

func (s *blockingBidStrategy) DecideBid(ctx context.Context, _ types.BidInput) (types.BidOutput, error) {
	select {
	case s.started <- struct{}{}:
	default:
	}
	select {
	case <-s.release:
		return types.BidOutput{Decision: types.DecisionSkip, Reason: "released"}, nil
	case <-ctx.Done():
		return types.BidOutput{}, ctx.Err()
	}
}

// setSnapshotBlockTime re-stamps the cached snapshot (and the ops state) for tests that drive
// handleMessage with wall-clock time: blockTime tracks the frame and both updatedAt stamps move to now
// so the stale-state gate sees freshly-refreshed caches.
func setSnapshotBlockTime(t *testing.T, s *Solver, tsMs int64) {
	t.Helper()
	now := time.Now()
	snap := *snapshotOf(t, s)
	snap.BlockTime = uint64(tsMs / 1000)
	snap.UpdatedAt = now
	storeSnapshot(t, s, &snap)
	if st, ok := s.state.load(); ok {
		st.UpdatedAt = now
		s.state.store(st)
	}
	seedDefaultDecisionStateAt(t, s, mustBig("2500000000"), now)
}

// auctionClock returns a clock within ±600s of the captured auction's timestamp, so clampTsAt keeps
// the auction timestamp (deterministic accrual) instead of falling back to wall-clock.
func auctionClock() func() time.Time { return func() time.Time { return time.Unix(1781243340, 0) } }

// decodeAuction parses the captured live auction frame (the fixture every bid test starts from).
func decodeAuction(t *testing.T) AuctionMessage {
	t.Helper()
	var a AuctionMessage
	if err := json.Unmarshal([]byte(capturedAuction), &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func setAuctionPrice(a *AuctionMessage, price string) {
	a.Payload.Prices = map[string]string{seedOracleHex: price}
}

// TestBuildBidStaleStateGate pins cache ownership: stale solver Executor accounting fails closed before
// calling the strategy, while default-strategy cache staleness remains a strategy skip.
func TestBuildBidStaleStateGate(t *testing.T) {
	base := auctionClock()()
	pastMax := func() time.Time { return base.Add(defaultExecutorStateMaxAge + time.Second) }

	t.Run("both caches stale", func(t *testing.T) {
		s, _ := seededSolver(t)
		if d := s.buildBid(t.Context(), decodeAuction(t), pastMax); d.skip != skipExecutorStateStale {
			t.Fatalf("skip = %q, want %q", d.skip, skipExecutorStateStale)
		}
	})
	t.Run("strategy state stale, executor fresh", func(t *testing.T) {
		s, _ := seededSolver(t)
		st, _ := s.state.load()
		st.UpdatedAt = pastMax()
		s.state.store(st)
		seedDefaultDecisionStateAt(t, s, mustBig("2500000000"), base)
		if d := s.buildBid(t.Context(), decodeAuction(t), pastMax); d.skip != types.SkipReasonStaleState {
			t.Fatalf("skip = %q, want %q", d.skip, types.SkipReasonStaleState)
		}
	})
	t.Run("executor state stale", func(t *testing.T) {
		s, _ := seededSolver(t)
		st, _ := s.state.load()
		st.UpdatedAt = base
		s.state.store(st)
		if d := s.buildBid(t.Context(), decodeAuction(t), pastMax); d.skip != skipExecutorStateStale {
			t.Fatalf("skip = %q, want %q", d.skip, skipExecutorStateStale)
		}
	})
	t.Run("fresh caches pass the gate", func(t *testing.T) {
		s, _ := seededSolver(t)
		if d := s.buildBid(t.Context(), decodeAuction(t), auctionClock()); d.skip == skipExecutorStateStale {
			t.Fatalf("fresh caches must not trip executor_state_stale")
		}
	})
}

func TestBuildBidHappyPath(t *testing.T) {
	s, sgnr := seededSolver(t)
	a := decodeAuction(t)

	d := s.buildBid(t.Context(), a, auctionClock())
	if d.skip != "" {
		t.Fatalf("expected a bid, got skip %q", d.skip)
	}
	if d.solve.Data.Bid != "0.0005" {
		t.Fatalf("bid = %q, want 0.0005 (flat BidWei)", d.solve.Data.Bid)
	}
	if d.solve.Data.Nonce != "8" { // on-chain 7, next is strictly greater
		t.Fatalf("nonce = %q, want 8", d.solve.Data.Nonce)
	}
	// Full sign path: the LiquidationSig must recover to our signer over the EXECUTOR_V6 digest the
	// Executor verifies (keccak(opData) bound into the digest, EIP-191 wrapped).
	opData, err := hexutil.Decode(d.solve.Data.OperationData)
	if err != nil {
		t.Fatal(err)
	}
	if len(opData) == 0 {
		t.Fatal("operationData must be non-empty")
	}
	if got := recoverSolveSigner(t, s, d.solve.Data); got != sgnr.addr {
		t.Fatalf("recovered %s, want signer %s", got, sgnr.addr)
	}
}

func TestBuildBidLetsStrategyOwnDecisionState(t *testing.T) {
	s, _ := seededSolver(t)
	strategy := &recordingBidStrategy{}
	s.strategy = strategy

	d := s.buildBid(t.Context(), decodeAuction(t), auctionClock())
	if d.skip != "" {
		t.Fatalf("expected strategy bid from execution-only solver state, got skip %q", d.skip)
	}
	if !strategy.called {
		t.Fatal("strategy was not called")
	}
	if strategy.input.Adapter.Address != seedAdapter || strategy.input.Adapter.Loan == (common.Address{}) ||
		strategy.input.Adapter.LoanDecimals != 6 || len(strategy.input.Adapter.Redeemable) != 1 ||
		strategy.input.Adapter.Redeemable[0].Decimals != 18 || strategy.input.Adapter.Redeemable[0].MaxRate == nil ||
		strategy.input.Adapter.Redeemable[0].MaxAssets == nil || !strategy.input.Adapter.Filler {
		t.Fatalf("adapter snapshot was not passed to strategy: %+v", strategy.input.Adapter)
	}
	if strategy.input.Context.Callback != seedCallback {
		t.Fatalf("callback = %s, want %s", strategy.input.Context.Callback.Hex(), seedCallback.Hex())
	}
	if strategy.input.Context.ExecutorMinDeposit.Cmp(minDeposit) != 0 {
		t.Fatalf("executor minimum deposit = %s, want %s", strategy.input.Context.ExecutorMinDeposit, minDeposit)
	}
}

func TestDefaultStrategyUsesBidInputAdapterSnapshot(t *testing.T) {
	s, _ := seededSolver(t)
	st, ok := s.state.load()
	if !ok {
		t.Fatal("missing seeded state")
	}
	st.Adapter.Redeemable[0].MaxRate = nil
	s.state.store(st)

	d := s.buildBid(t.Context(), decodeAuction(t), auctionClock())
	if d.skip != types.SkipReasonNoLegs {
		t.Fatalf("skip = %q, want %q when input adapter quote is unusable", d.skip, types.SkipReasonNoLegs)
	}
}

func TestBuildBidGasProfitabilityGate(t *testing.T) {
	a := decodeAuction(t)

	t.Run("net below min skips gas_unprofitable", func(t *testing.T) {
		s, _ := seededSolver(t)
		s.cfg.MaxTxGasPrice = mustBig("1000000000000000000")
		if d := s.buildBid(t.Context(), a, auctionClock()); d.skip != types.SkipReasonGasUnprofitable {
			t.Fatalf("skip = %q, want %q", d.skip, types.SkipReasonGasUnprofitable)
		}
	})

	t.Run("dry-run without rate skips because callback auth needs loan profit floors", func(t *testing.T) {
		s, _ := seededSolver(t)
		s.dryRun = true
		seedDefaultDecisionState(t, s, nil)
		if d := s.buildBid(t.Context(), a, auctionClock()); d.skip != types.SkipReasonGasUnprofitable {
			t.Fatalf("dry-run no-rate path should skip %q, got %q", types.SkipReasonGasUnprofitable, d.skip)
		}
	})
}

func TestBuildBidSignsConfiguredGasPriceCap(t *testing.T) {
	s, _ := seededSolver(t)
	s.cfg.MaxTxGasPrice = big.NewInt(1_000_000_000)

	d := s.buildBid(t.Context(), decodeAuction(t), auctionClock())
	if d.skip != "" {
		t.Fatalf("expected bid, got skip %q", d.skip)
	}
	if d.solve.Data.MaxTxGasPrice != s.cfg.MaxTxGasPrice.String() {
		t.Fatalf("maxTxGasPrice = %q, want configured cap %s", d.solve.Data.MaxTxGasPrice, s.cfg.MaxTxGasPrice)
	}
	if got := recoverSolveSigner(t, s, d.solve.Data); got != s.deps.Signer.Address() {
		t.Fatalf("recovered %s, want signer %s", got, s.deps.Signer.Address())
	}
}

func TestFactoryRejectsLiveBiddingWithoutRateSource(t *testing.T) {
	var node yaml.Node
	raw := wsline + addrs + "strategy:\n  name: default\n  config:\n    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.0005\"}\n"
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatal(err)
	}
	cfg, err := parseConfig(node)
	if err == nil {
		_, err = defaultstrategy.ParseConfig(cfg.Strategy.Config)
	}
	if err == nil {
		t.Fatal("expected default strategy construction to reject config without loanEthFeed")
	}
}

// TestBuildBidPriceSource proves buildBid trusts the auction frame price: a healthy frame skips, while a
// liquidatable frame drives a full sized bid.
func TestBuildBidPriceSource(t *testing.T) {
	cases := []struct {
		name      string
		framePx   string
		wantSkip  string
		wantSized bool // for the bidding case, assert a full leg was sized
	}{
		{"healthy $5000 frame skips", seedHealthyPrice, "no_legs", false},
		{"liquidatable $1550 frame bids", seedLiquidatablePrice, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := decodeAuction(t)
			setAuctionPrice(&a, tc.framePx)
			s, _ := seededSolver(t)
			d := s.buildBid(t.Context(), a, auctionClock())
			if d.skip != tc.wantSkip {
				t.Fatalf("skip = %q, want %q", d.skip, tc.wantSkip)
			}
			if tc.wantSized {
				opData, err := hexutil.Decode(d.solve.Data.OperationData)
				if err != nil {
					t.Fatal(err)
				}
				if len(opData) == 0 {
					t.Fatal("expected non-empty operationData")
				}
			}
		})
	}
}

func TestBuildBidLetsStrategyOwnCallbackFunding(t *testing.T) {
	s, _ := seededSolver(t)
	seedDefaultDecisionStateWithCallbackBalance(t, s, mustBig("2500000000"), big.NewInt(1), auctionClock()())

	if d := s.buildBid(t.Context(), decodeAuction(t), auctionClock()); d.skip != "callback_balance" {
		t.Fatalf("callback balance is strategy-owned; skip = %q, want callback_balance", d.skip)
	}
}

func TestBuildBidLetsDefaultStrategyOwnDepositGasHeadroom(t *testing.T) {
	s, _ := seededSolver(t)
	st, ok := s.state.load()
	if !ok {
		t.Fatal("missing cached state")
	}
	st.Exec.Deposit = new(big.Int).Add(minDeposit, big.NewInt(1))
	s.state.store(st)

	if d := s.buildBid(t.Context(), decodeAuction(t), auctionClock()); d.skip != "deposit_low" {
		t.Fatalf("deposit gas headroom is default-strategy-owned; skip = %q, want deposit_low", d.skip)
	}
}

func TestBuildBidSkipsSolverEnvelopeBeforeStrategy(t *testing.T) {
	tests := []struct {
		name  string
		state ExecutorState
		want  string
	}{
		{
			name:  "signer locked",
			state: ExecutorState{Nonce: big.NewInt(7), Deposit: mustBig("100000000000000000"), Locked: true},
			want:  "signer_locked",
		},
		{
			name:  "deposit below floor",
			state: ExecutorState{Nonce: big.NewInt(7), Deposit: big.NewInt(1), Locked: false},
			want:  "deposit_low",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := seededSolver(t)
			st, _ := s.state.load()
			st.Exec = tc.state
			s.state.store(st)

			strategy := &recordingBidStrategy{}
			s.strategy = strategy
			if d := s.buildBid(t.Context(), decodeAuction(t), auctionClock()); d.skip != tc.want {
				t.Fatalf("skip = %q, want %q", d.skip, tc.want)
			}
			if strategy.called {
				t.Fatal("strategy should not be called after solver-owned envelope skip")
			}
		})
	}
}

// TestBuildBidReservesPendingPosition pins that the default strategy, rather than the solver, keeps the
// selected Morpho position unavailable while a bid is unresolved.
func TestBuildBidReservesPendingPosition(t *testing.T) {
	s, _ := seededSolver(t)
	a := decodeAuction(t)

	d1 := s.buildBid(t.Context(), a, auctionClock())
	if d1.skip != "" {
		t.Fatalf("first bid should succeed, got skip %q", d1.skip)
	}
	s.reserve(d1.nonce, time.Unix(1781243340, 0), seedCallback, a.ID)

	if d2 := s.buildBid(t.Context(), a, auctionClock()); d2.skip != types.SkipReasonNoLegs {
		t.Fatalf("the reserved position must not be selected twice, got %q", d2.skip)
	}

	// Once the bid resolves, the strategy keeps its accounting reservation until callback balance has been
	// refreshed after resolution. This prevents bidding against a balance spent by a winning settlement.
	s.pruneReservations(d1.nonce, time.Unix(1781243340, 0))
	if d3 := s.buildBid(t.Context(), a, auctionClock()); d3.skip != types.SkipReasonNoLegs {
		t.Fatalf("resolved position should remain reserved until balance refresh, got %q", d3.skip)
	}
	seedDefaultDecisionStateAt(t, s, mustBig("2500000000"), auctionClock()().Add(time.Second))
	if d4 := s.buildBid(t.Context(), a, auctionClock()); d4.skip != "" {
		t.Fatalf("after callback balance refresh the strategy should be allowed again, got %q", d4.skip)
	}
}

// TestPruneReservations pins reservation lifecycle: a bid whose nonce fell below the on-chain nonce
// (submitted → settled/reverted) or that aged past reservationTTL is freed, while a recent still-pending
// bid remains visible to strategies as a pending auction.
func TestPruneReservations(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1781243340, 0)
	s.reserve(8, now, seedCallback, "auction-8")
	s.reserve(10, now, seedCallback, "auction-10")
	s.reserve(12, now.Add(-time.Hour), seedCallback, "auction-old")

	// nonce 10 frees 8 (below) AND 10 (settlement sets the on-chain nonce to the consumed bid's nonce, so
	// nonce == r.nonce must release it — the F1 fix: `<=`, not `<`); 12 is freed by age (> TTL) → none left.
	s.pruneReservations(10, now)
	if inFlight := s.inFlightSnapshot(); len(inFlight.pending) != 0 {
		t.Fatalf("all reservations should be freed, got pending=%v", inFlight.pending)
	}

	s.reserve(11, now, seedCallback, "auction-11")
	s.pruneReservations(10, now)
	if inFlight := s.inFlightSnapshot(); len(inFlight.pending) != 1 || inFlight.pending[0].ID != "auction-11" {
		t.Fatalf("a recent pending bid should be kept, got pending=%v", inFlight.pending)
	}

	// A bid is freed exactly when the on-chain nonce reaches its nonce (== r.nonce), not only when it passes.
	s.pruneReservations(11, now)
	if inFlight := s.inFlightSnapshot(); len(inFlight.pending) != 0 {
		t.Fatalf("bid with nonce == on-chain nonce should be freed at settlement, got pending=%v", inFlight.pending)
	}
}

func TestWonReservationSurvivesDelayedSettlement(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1781243340, 0)
	s.reserve(8, now, seedCallback, "auction-won")
	s.markReservationWon("auction-won")

	s.pruneReservations(7, now.Add(2*time.Minute))
	if inFlight := s.inFlightSnapshot(); len(inFlight.pending) != 1 || !inFlight.pending[0].Won {
		t.Fatalf("won bid must stay reserved while settlement is delayed, pending=%v", inFlight.pending)
	}
}

func TestAuctionResultReleasesLostBidReservation(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1781243340, 0)
	s.reserve(8, now, seedCallback, "auction-lost")

	s.handleMessage(t.Context(), []byte(`{
		"op":"auction-result",
		"id":"auction-lost",
		"data":{"bid":"0.0005","liquidator":"0x1111111111111111111111111111111111111111"}
	}`))
	if inFlight := s.inFlightSnapshot(); len(inFlight.pending) != 0 {
		t.Fatalf("lost auction must release reservation, pending=%v", inFlight.pending)
	}

	s.reserve(9, now, seedCallback, "auction-won")
	s.handleMessage(t.Context(), []byte(`{
		"op":"auction-result",
		"id":"auction-won",
		"data":{"bid":"0.0005","liquidator":"`+`0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1`+`"}
	}`))
	if inFlight := s.inFlightSnapshot(); len(inFlight.pending) != 1 || inFlight.pending[0].ID != "auction-won" {
		t.Fatalf("won auction must stay reserved until liquidation result/nonce, pending=%v", inFlight.pending)
	}
}

func TestLiquidationResultReleasesOurReservation(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1781243340, 0)
	s.reserve(8, now, seedCallback, "auction-ours")

	s.handleMessage(t.Context(), []byte(`{
		"op":"liquidation-result",
		"id":"auction-ours",
		"data":{"success":true,"txHash":"","liquidator":"`+`0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1`+`","error":""}
	}`))
	if inFlight := s.inFlightSnapshot(); len(inFlight.pending) != 0 {
		t.Fatalf("our liquidation result must release reservation, pending=%v", inFlight.pending)
	}

	s.reserve(9, now, seedCallback, "auction-other")
	s.handleMessage(t.Context(), []byte(`{
		"op":"liquidation-result",
		"id":"auction-other",
		"data":{"success":true,"txHash":"","liquidator":"0x1111111111111111111111111111111111111111","error":""}
	}`))
	if inFlight := s.inFlightSnapshot(); len(inFlight.pending) != 1 || inFlight.pending[0].ID != "auction-other" {
		t.Fatalf("other solver liquidation result must not release our reservation, pending=%v", inFlight.pending)
	}
}

func TestLiquidationResultRequestsStateRefresh(t *testing.T) {
	s, _ := seededSolver(t)
	s.stateRefreshCh = make(chan struct{}, 1)
	s.handleMessage(t.Context(), []byte(`{
		"op":"liquidation-result",
		"id":"auction-ours",
		"data":{"success":true,"txHash":"","liquidator":"`+`0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1`+`","error":""}
	}`))
	select {
	case <-s.stateRefreshCh:
	default:
		t.Fatal("liquidation result did not request solver state refresh")
	}
}

func TestApplyExecutorStatePrunesReservations(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1781243340, 0)

	// A sent bid (nonce 8), plus a stale local nonce high-water mark (5).
	s.reserve(8, now, seedCallback, "auction-8")
	s.nonces.reconcile(5)
	if inFlight := s.inFlightSnapshot(); len(inFlight.pending) == 0 {
		t.Fatal("precondition: the reservation should be present")
	}

	// On-chain nonce advanced to 9 (the bid settled).
	st := ExecutorState{Nonce: big.NewInt(9), Deposit: mustBig("100000000000000000"), Locked: false}
	s.applyExecutorState(st, now)

	// pruneReservations ran: nonce 8 <= 9 → the reservation is freed.
	if inFlight := s.inFlightSnapshot(); len(inFlight.pending) != 0 {
		t.Fatalf("pruneReservations must run from executor state; pending=%v", inFlight.pending)
	}
	// nonces.reconcile ran: the next nonce is strictly above the on-chain 9.
	if got := s.nonces.next(0); got != 10 {
		t.Fatalf("nonces.reconcile must run despite a failed balance read; next nonce = %d, want 10", got)
	}
}

// TestFullAuctionLifecycle drives the whole inbound-frame flow through handleMessage: an auction frame
// produces a signed solve on the wire, then tripping the breaker via its REAL input (recorded settlement
// failures, the same path the WS liquidation-result handler feeds) makes buildBid skip "breaker" so a fresh
// auction is dropped (no solve sent). (The WS-frame → recordFailure path is covered by
// TestLiquidationResultFeedsBreaker.)
func TestFullAuctionLifecycle(t *testing.T) {
	s, sgnr := seededSolver(t)

	// 1) Auction → a solve is sent on the wire. Stamp the frame as freshly emitted so the too_late gate
	// doesn't drop the captured fixture's long-past emit time.
	fresh := decodeAuction(t)
	setAuctionPrice(&fresh, seedLiquidatablePrice)
	fresh.Timestamp = time.Now().UnixMilli()
	setSnapshotBlockTime(t, s, fresh.Timestamp)
	s.handleMessage(t.Context(), marshal(fresh))
	frame := waitSend(t, s)
	if frame == nil {
		t.Fatal("expected a solve to be sent for a liquidatable auction")
	}
	var solve SolveMessage
	if err := json.Unmarshal(frame, &solve); err != nil {
		t.Fatal(err)
	}
	if solve.Op != "solve" || solve.ID != "6382e936-c915-496a-bb3e-fa3b4ccc3a8d" ||
		solve.Data.OperationCallback == "" || solve.Data.OperationData == "" || solve.Data.LiquidationSig == "" {
		t.Fatalf("bad solve: %+v", solve.Data)
	}
	// The signature recovers to our signer (full sign path through handleMessage).
	if got := recoverSolveSigner(t, s, solve.Data); got != sgnr.addr {
		t.Fatalf("solve signature does not recover to signer: got %s", got)
	}

	// 2) Trip the breaker through its REAL input — recorded settlement failures (maxFailures=3 within the
	// window), the same recordFailure path the WS liquidation-result handler feeds. Record at wall-clock now,
	// since the hot path (handleAuction → buildBid) evaluates the breaker with time.Now. After this, tripped.
	now := time.Now()
	for i := 0; i < 3; i++ {
		s.breaker.recordFailure(now)
	}
	if tripped, _ := s.breaker.tripped(now); !tripped {
		t.Fatal("breaker should be tripped after 3 recorded failures within the window")
	}
	// buildBid (evaluated at the same wall clock as the hot path) must short-circuit to skip "breaker".
	if d := s.buildBid(t.Context(), decodeAuction(t), time.Now); d.skip != "breaker" {
		t.Fatalf("tripped breaker must skip the bid, got skip %q", d.skip)
	}

	// 3) A fresh auction (new id so dedup can't mask it) is dropped by the breaker — nothing sent.
	a := decodeAuction(t)
	a.ID = "9999aaaa-0000-1111-2222-333344445555"
	a.Timestamp = time.Now().UnixMilli()
	setSnapshotBlockTime(t, s, a.Timestamp)
	s.handleMessage(t.Context(), marshal(a))
	expectNoSend(t, s, "breaker tripped")
}

// drainSend returns the next buffered outbound frame, or nil if none is queued.
func drainSend(s *Solver) []byte {
	select {
	case f := <-s.ws.send:
		return f
	default:
		return nil
	}
}

func waitSend(t *testing.T, s *Solver) []byte {
	t.Helper()
	select {
	case f := <-s.ws.send:
		return f
	case <-time.After(time.Second):
		return nil
	}
}

func expectNoSend(t *testing.T, s *Solver, why string) {
	t.Helper()
	select {
	case f := <-s.ws.send:
		t.Fatalf("%s: expected no solve, got one: %s", why, f)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestFeedAuctionDoesNotBuildLiquidationBid(t *testing.T) {
	s, _ := seededSolver(t)
	raw := []byte(`{
	  "op":"auction","id":"feed-auction",
	  "timestamp":1726058300000,"durationMs":400,
	  "payload":{"ETH":"250000000000","BTC":"6000000000000","USDC":"99878787"}
	}`)
	s.handleMessage(t.Context(), raw)
	if frame := drainSend(s); frame != nil {
		t.Fatalf("feed auction must not produce a liquidation solve: %s", frame)
	}
}

func TestHandleMessageDispatchesAuctionBidAsync(t *testing.T) {
	s, _ := seededSolver(t)
	blocking := &blockingBidStrategy{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	defer close(blocking.release)

	a := decodeAuction(t)
	setAuctionPrice(&a, seedLiquidatablePrice)
	a.Timestamp = time.Now().UnixMilli()
	setSnapshotBlockTime(t, s, a.Timestamp)
	s.strategy = blocking

	done := make(chan struct{})
	go func() {
		s.handleMessage(t.Context(), marshal(a))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("handleMessage blocked behind bid strategy")
	}
	select {
	case <-blocking.started:
	case <-time.After(time.Second):
		t.Fatal("bid strategy was not called")
	}
}

// TestRedstoneClosedPositionNotBid proves we bid off our own tracked on-chain state, not the frame's
// pushed positions: even though the captured frame lists the borrower as deeply underwater, our cached
// position shows it fully closed (zero debt/collateral), so buildBid computes it non-liquidatable and
// does not bid. (The frame's pushed positions are ignored entirely — candidates come from snap.Positions.)
func TestRedstoneClosedPositionNotBid(t *testing.T) {
	s, _ := seededSolver(t)
	id := common.HexToHash("0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5")
	borrower := common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE")

	// Build a FRESH snapshot whose tracked set is a single CLOSED position and store it (the loaded snapshot is
	// immutable once stored — mutating its maps would write through the atomic). The frame still lists it as
	// liquidatable, but candidates come from snap.Positions.
	cur := snapshotOf(t, s)
	fresh := *cur
	fresh.Positions = map[common.Hash]map[common.Address]morpho.PositionState{
		id: {borrower: {BorrowShares: big.NewInt(0), Collateral: big.NewInt(0)}},
	}
	storeSnapshot(t, s, &fresh)

	if d := s.buildBid(t.Context(), decodeAuction(t), auctionClock()); d.skip != "no_legs" {
		t.Fatalf("a closed tracked position is not liquidatable → no_legs, got %q", d.skip)
	}
}

// TestDryRunSuppressesSend pins the OEV_DRY_RUN observe mode: a profitable auction is fully evaluated
// (counted as a would-bid via metrics.bid()) but NO solve is sent on the wire — the operator can watch the
// bot's decisions against a live feed without funding or competing.
func TestDryRunSuppressesSend(t *testing.T) {
	s, _ := seededSolver(t)
	s.dryRun = true

	// Real metrics on a fresh registry so we can read the would-bid counter back.
	reg := prometheus.NewRegistry()
	m, err := newMetrics(reg, defaultStrategyName)
	if err != nil {
		t.Fatalf("newMetrics: %v", err)
	}
	s.metrics = m

	a := decodeAuction(t)
	setAuctionPrice(&a, seedLiquidatablePrice)
	a.Timestamp = time.Now().UnixMilli() // freshly emitted so the too_late gate doesn't drop it
	setSnapshotBlockTime(t, s, a.Timestamp)
	s.handleAuctionWithContext(t.Context(), marshal(a))

	if f := drainSend(s); f != nil {
		t.Fatalf("dry-run must not send a solve, got %s", f)
	}
	if got := testutil.ToFloat64(m.bids); got != 1 {
		t.Fatalf("oev_bids_total = %v, want 1 (dry-run still counts the would-bid)", got)
	}
}

func TestMetricsCarryStrategyLabel(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := newMetrics(reg, "webhook")
	if err != nil {
		t.Fatalf("newMetrics: %v", err)
	}
	m.skip("strategy_skip")
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "oev_skips_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			hasReason := false
			hasStrategy := false
			for _, label := range metric.GetLabel() {
				switch label.GetName() {
				case "reason":
					hasReason = label.GetValue() == "strategy_skip"
				case "strategy":
					hasStrategy = label.GetValue() == "webhook"
				}
			}
			if hasReason && hasStrategy {
				return
			}
		}
	}
	t.Fatal("oev_skips_total missing strategy label")
}

// TestHandleAuctionEmptyIdDropped pins the auction identity invariant: RedStone auctions must carry an id.
// Without it we cannot safely correlate solve/result frames, so the frame is ignored before bid building.
func TestHandleAuctionEmptyIdDropped(t *testing.T) {
	s, _ := seededSolver(t)

	a := decodeAuction(t)
	setAuctionPrice(&a, seedLiquidatablePrice)
	a.ID = ""                            // the frame carries no id
	a.Timestamp = time.Now().UnixMilli() // freshly emitted so the too_late gate doesn't drop it
	setSnapshotBlockTime(t, s, a.Timestamp)

	if f := drainSend(s); f != nil {
		t.Fatalf("precondition: send channel should be empty, got %s", f)
	}
	s.handleAuctionWithContext(t.Context(), marshal(a))
	if f := drainSend(s); f != nil {
		t.Fatalf("empty-id auction must be ignored, got solve %s", f)
	}
}

// TestDedupKey pins that only RedStone's auction id is a valid dedup key.
func TestDedupKey(t *testing.T) {
	withID := AuctionMessage{ID: "abc"}
	if got := withID.dedupKey(); got != "id:abc" {
		t.Fatalf("present id must be the key, got %q", got)
	}
	if got := (AuctionMessage{}).dedupKey(); got != "" {
		t.Fatalf("empty id must not produce a synthetic key, got %q", got)
	}
}

// TestBuildBidStaleEpoch pins the fail-closed epoch gate: a non-empty snapshot must be block-tagged and
// close enough to the auction timestamp that a stuck API cache cannot keep bidding indefinitely.
func TestBuildBidStaleEpoch(t *testing.T) {
	a := decodeAuction(t)
	now := auctionClock()
	s, _ := seededSolver(t)

	fresh := *snapshotOf(t, s)
	fresh.Block, fresh.BlockTime = 0, 0
	storeSnapshot(t, s, &fresh)
	if d := s.buildBid(t.Context(), a, now); d.skip != types.SkipReasonStaleEpoch {
		t.Fatalf("untagged snapshot must skip %s, got %q", types.SkipReasonStaleEpoch, d.skip)
	}

	fresh.Block, fresh.BlockTime = 123, uint64(a.Timestamp/1000)
	storeSnapshot(t, s, &fresh)
	if d := s.buildBid(t.Context(), a, now); d.skip != "" {
		t.Fatalf("current tagged snapshot should bid, got skip %q", d.skip)
	}

	fresh.BlockTime = uint64(a.Timestamp/1000) - uint64(time.Hour/time.Second)
	storeSnapshot(t, s, &fresh)
	if d := s.buildBid(t.Context(), a, now); d.skip != types.SkipReasonStaleEpoch {
		t.Fatalf("old tagged snapshot must skip %s, got %q", types.SkipReasonStaleEpoch, d.skip)
	}
}

// TestSeenAuctions pins the bounded de-dup: first sight is new, repeats are seen, and the oldest id is
// evicted past cap (so a long-evicted id reads as new again).
func TestSeenAuctions(t *testing.T) {
	s := newSeenAuctions(2)
	if s.seen("a") {
		t.Fatal("first sight of a should be new")
	}
	if !s.seen("a") {
		t.Fatal("repeat of a should be seen")
	}
	_ = s.seen("b")  // [a, b]
	if s.seen("c") { // cap 2 → evict a → [b, c]
		t.Fatal("c is new")
	}
	if s.seen("a") {
		t.Fatal("a was evicted past cap; should read as new again")
	}
}

// TestLiquidationResultFeedsBreaker pins the WS-driven failure breaker: a liquidation-result frame for OUR
// callback with success:false records exactly one breaker failure (and trips at maxFailures); a success:true
// frame, and a failure for ANOTHER liquidator, record none. This is the sole breaker-failure feed now that
// the on-chain event scan is gone.
func TestLiquidationResultFeedsBreaker(t *testing.T) {
	frame := func(liquidator string, success bool) []byte {
		return marshal(LiquidationResult{
			Op: "liquidation-result", ID: "a",
			Data: LiquidationResultData{Success: success, Liquidator: liquidator, TxHash: "0x1"},
		})
	}
	now := time.Now()

	t.Run("success:false for our callback records a failure and trips at maxFailures", func(t *testing.T) {
		s, _ := seededSolver(t) // breaker maxFailures = 3
		for i := 0; i < 3; i++ {
			s.handleMessage(t.Context(), frame(seedCallback.Hex(), false))
		}
		if tripped, _ := s.breaker.tripped(now); !tripped {
			t.Fatal("3 failed liquidation-result frames for our callback must trip the breaker")
		}
	})

	t.Run("success:true records none", func(t *testing.T) {
		s, _ := seededSolver(t)
		for i := 0; i < 5; i++ {
			s.handleMessage(t.Context(), frame(seedCallback.Hex(), true))
		}
		if tripped, _ := s.breaker.tripped(now); tripped {
			t.Fatal("successful liquidation-result frames must not trip the breaker")
		}
	})

	t.Run("a failure for another liquidator records none", func(t *testing.T) {
		s, _ := seededSolver(t)
		other := common.HexToAddress("0x2222222222222222222222222222222222222222").Hex()
		for i := 0; i < 5; i++ {
			s.handleMessage(t.Context(), frame(other, false))
		}
		if tripped, _ := s.breaker.tripped(now); tripped {
			t.Fatal("another solver's failed liquidations must not trip our breaker")
		}
	})
}

// TestTooLate pins that the auction window is measured from the auctioneer emit time. A late-delivered
// frame is dropped; a bogus/future emit timestamp falls back to local elapsed time.
func TestTooLate(t *testing.T) {
	now := time.Unix(1781243340, 0)
	const timeoutMs = 500
	emit := func(deltaMs int64) int64 { return now.UnixMilli() + deltaMs }

	cases := []struct {
		name    string
		emitMs  int64
		start   time.Time // local frame-receipt time
		wantBad bool
	}{
		// Emitted (timeoutMs + slack) ago → past the deadline since emit, even though we just received it.
		{"late-delivered frame (emit + slack ago)", emit(-(timeoutMs + 100)), now, true},
		// Emitted exactly at the window edge → not yet too late (strictly greater trips it).
		{"emit exactly at the window edge", emit(-timeoutMs), now, false},
		// Fresh frame, emitted just now and just received → in budget.
		{"fresh frame", emit(-10), now, false},
		// Emit unset (0): trust the local clock — a slow local path (start long ago) is too late.
		{"no emit ts, slow local path", 0, now.Add(-time.Duration(timeoutMs+100) * time.Millisecond), true},
		{"no emit ts, fast local path", 0, now.Add(-10 * time.Millisecond), false},
		// Forward emit timestamp (clock skew / bogus): fall back to the local clock, don't trust emit.
		{"future emit ts falls back to local (fast)", emit(5000), now.Add(-10 * time.Millisecond), false},
		{"future emit ts falls back to local (slow)", emit(5000), now.Add(-time.Duration(timeoutMs+100) * time.Millisecond), true},
	}
	for _, c := range cases {
		if got := tooLate(c.emitMs, timeoutMs, c.start, now); got != c.wantBad {
			t.Errorf("%s: tooLate(emit=%d, start=%v) = %v, want %v", c.name, c.emitMs, c.start, got, c.wantBad)
		}
	}
}

func TestAuctionBidContextDeadline(t *testing.T) {
	start := time.Unix(100, 0)
	a := AuctionMessage{Timestamp: start.Add(-100 * time.Millisecond).UnixMilli(), TimeoutMs: 500}

	ctx, cancel := auctionBidContext(t.Context(), a, start)
	defer cancel()
	got, ok := ctx.Deadline()
	if !ok {
		t.Fatal("bid context missing deadline")
	}
	want := time.UnixMilli(a.Timestamp).Add(time.Duration(a.TimeoutMs)*time.Millisecond - bidDecisionDeadlineMargin)
	if !got.Equal(want) {
		t.Fatalf("deadline = %s, want %s", got, want)
	}

	future := AuctionMessage{Timestamp: start.Add(time.Second).UnixMilli(), TimeoutMs: 500}
	ctx, cancel = auctionBidContext(t.Context(), future, start)
	defer cancel()
	got, ok = ctx.Deadline()
	if !ok {
		t.Fatal("fallback bid context missing deadline")
	}
	want = start.Add(time.Duration(future.TimeoutMs)*time.Millisecond - bidDecisionDeadlineMargin)
	if !got.Equal(want) {
		t.Fatalf("fallback deadline = %s, want %s", got, want)
	}
}

func TestBuildBidSkips(t *testing.T) {
	clock := auctionClock()
	healthy := mustBig("100000000000000000000000000000000000000000000")

	stateWith := func(s *Solver, deposit *big.Int, locked bool) cachedState {
		st, _ := s.state.load()
		st.Exec = ExecutorState{Nonce: big.NewInt(7), Deposit: deposit, Locked: locked}
		return st
	}
	tests := []struct {
		name          string
		mut           func(*Solver)
		priceOverride *big.Int // if set, re-prices the auction oracle so the position is healthy
		want          string
	}{
		{name: "breaker", mut: func(s *Solver) { s.breaker.blacklist() }, want: "breaker"},
		{name: "signer_locked", mut: func(s *Solver) {
			s.state.store(stateWith(s, mustBig("100000000000000000"), true))
		}, want: "signer_locked"},
		{name: "deposit_low", mut: func(s *Solver) {
			s.state.store(stateWith(s, big.NewInt(1), false)) // below MIN_DEPOSIT (1e13)
		}, want: "deposit_low"},
		{name: "bid_cap", mut: func(s *Solver) {
			s.cfg.MaxBidWei = big.NewInt(1)
		}, want: "bid_cap"},
		{name: "callback_balance", mut: func(s *Solver) {
			seedDefaultDecisionStateWithCallbackBalance(t, s, mustBig("2500000000"), big.NewInt(1), clock())
		}, want: "callback_balance"},
		{name: "no_legs_when_healthy", priceOverride: healthy, want: "no_legs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frame := decodeAuction(t)
			s, _ := seededSolver(t)
			if tc.mut != nil {
				tc.mut(s)
			}
			if tc.priceOverride != nil {
				frame.Payload.Prices = map[string]string{"0xfED5bC312C7139743bc3ab21Ef92f5AeB353339D": tc.priceOverride.String()}
			}
			if d := s.buildBid(t.Context(), frame, clock); d.skip != tc.want {
				t.Fatalf("skip = %q, want %q", d.skip, tc.want)
			}
		})
	}
}
