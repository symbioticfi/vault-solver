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
)

// seedAdapter is the LiquidLane adapter stamped into the seeded market, so tests can assert it flows
// snapshot → leg → operationData.
var seedAdapter = common.HexToAddress("0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b")

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
	oracle := common.HexToAddress("0xfED5bC312C7139743bc3ab21Ef92f5AeB353339D")

	mon := &apiMonitor{log: logr.Discard()}
	mon.snap.Store(&snapshot{
		markets: map[common.Hash]MarketInfo{
			id: {Params: abiMarketParams{Oracle: oracle, Lltv: mustBig("860000000000000000")}, State: goldenMarket()},
		},
		// Cached on-chain oracle price ($1550) — used by testMonitor.
		prices: map[common.Hash]*big.Int{id: mustBig("1550000000000000000000000000")},
		quotes: map[common.Hash]AdapterQuote{
			// The single adapter's quote: sells the RWA at ~$1780 (≈1% under the auctioned $1800.9); ample liquidity.
			id: newQuote("1780000000000000000000", mustBig("100000000000")),
		},
		// Independently-tracked at-risk positions — the SOLE candidate source
		// now that the frame's pushed positions are no longer consumed. Both fixture borrowers are seeded so
		// workerCandidates surfaces them, evaluated at the frame/onchain price. The captured frame still
		// carries these same positions, but they're ignored: candidates come from snap.positions.
		positions: map[common.Hash]map[common.Address]morpho.PositionState{
			id: {
				// 0x629d… — goldenBorrower (1.0 TCOL, borrowShares 1685600000000000).
				common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE"): goldenBorrower(),
				// 0x378a… — the frame's second borrower.
				common.HexToAddress("0x378a49c640fd9eea888a6a553caae441e2fdebc6"): {
					BorrowShares: mustBig("1582399974653062"), Collateral: mustBig("1000000000000000000"),
				},
			},
		},
		block:     100,
		blockTime: 1781243340,
	})

	cfg := &Config{
		Executor:      common.HexToAddress("0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD"),
		Callback:      common.HexToAddress("0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1"),
		Adapter:       seedAdapter,
		BidWei:        mustBig("500000000000000"), // 0.0005 ETH flat bid
		MaxTxGasPrice: big.NewInt(1_000_000_000),
		Sizing:        SizingParams{AllowFullLiquidation: true, SwapHaircutBps: 0},
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
	s.mon = mon
	// Healthy accounting: deposit clears MIN_DEPOSIT; callback covers the bid.
	s.state.store(cachedState{
		Exec:           ExecutorState{Nonce: big.NewInt(7), Deposit: mustBig("100000000000000000"), Locked: false},
		CallbackNative: mustBig("1000000000000000000"),
		Rate:           mustBig("2500000000"), // 2500e6 loan base units per ETH
		GasLimit:       redstoneExecutorMaxGasUnits,
		Gas: &gasPredictorState{
			FreeAssets:   mustBig("100000000000"),
			Withdrawable: mustBig("100000000000"),
			Acquire:      map[common.Address]*big.Int{},
		},
	})
	return s, sgnr
}

type testFataler interface {
	Helper()
	Fatalf(format string, args ...any)
}

func snapshotOf(t testFataler, s *Solver) *snapshot {
	t.Helper()
	return s.mon.snapshot()
}

func storeSnapshot(t testFataler, s *Solver, snap *snapshot) {
	t.Helper()
	switch m := s.mon.(type) {
	case *apiMonitor:
		m.snap.Store(snap)
	case *testMonitor:
		m.snap.Store(snap)
	default:
		t.Fatalf("unexpected monitor type %T", s.mon)
	}
}

func useOnchainTestMonitor(t *testing.T, s *Solver) {
	t.Helper()
	mon := &testMonitor{log: logr.Discard()}
	mon.snap.Store(snapshotOf(t, s))
	s.mon = mon
}

func setSnapshotBlockTime(t *testing.T, s *Solver, tsMs int64) {
	t.Helper()
	snap := *snapshotOf(t, s)
	snap.blockTime = uint64(tsMs / 1000)
	storeSnapshot(t, s, &snap)
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

func recoverCallbackAuthSigner(t *testing.T, s *Solver, op operationData) common.Address {
	t.Helper()
	legs := make([]LiquidationLeg, len(op.Legs))
	for i, leg := range op.Legs {
		legs[i] = LiquidationLeg{
			MarketId:       leg.MarketId,
			Borrower:       leg.Borrower,
			MaxSeizeAssets: leg.MaxSeizeAssets,
			MinProfit:      leg.MinProfit,
		}
	}
	digest, err := CallbackAuthDigest(s.chainID, s.cfg.Callback, s.cfg.Executor, op.Auth, legs)
	if err != nil {
		t.Fatalf("callback auth digest: %v", err)
	}
	sig := append([]byte(nil), op.AuthSig...)
	if len(sig) != 65 {
		t.Fatalf("callback auth signature len = %d, want 65", len(sig))
	}
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	pub, err := crypto.SigToPub(digest.Bytes(), sig)
	if err != nil {
		t.Fatalf("recover callback auth: %v", err)
	}
	return crypto.PubkeyToAddress(*pub)
}

func TestBuildBidHappyPath(t *testing.T) {
	s, sgnr := seededSolver(t)
	a := decodeAuction(t)

	d := s.buildBid(a, auctionClock())
	if d.skip != "" {
		t.Fatalf("expected a bid, got skip %q", d.skip)
	}
	if d.legs != 2 {
		t.Fatalf("legs = %d, want both profitable same-market borrowers", d.legs)
	}
	if len(d.solve.Data.Borrowers) != 2 {
		t.Fatalf("borrowers = %v, want 2", d.solve.Data.Borrowers)
	}
	if d.solve.Data.Bid != "0.0005" {
		t.Fatalf("bid = %q, want 0.0005 (flat BidWei)", d.solve.Data.Bid)
	}
	if d.solve.Data.Nonce != "8" { // on-chain 7, next is strictly greater
		t.Fatalf("nonce = %q, want 8", d.solve.Data.Nonce)
	}
	// Flat-bid path: gross carries the bundle's Σ loan-token profit (logging only); the bid is the flat BidWei.
	if d.gross == nil || d.gross.Sign() <= 0 {
		t.Fatalf("gross profit = %v, want > 0", d.gross)
	}

	// Full sign path: the LiquidationSig must recover to our signer over the EXECUTOR_V6 digest the
	// Executor verifies (keccak(opData) bound into the digest, EIP-191 wrapped).
	opData, err := hexutil.Decode(d.solve.Data.OperationData)
	if err != nil {
		t.Fatal(err)
	}
	op, err := decodeOperationData(opData)
	if err != nil {
		t.Fatalf("decode operationData: %v", err)
	}
	if op.Auth.AuctionKey != auctionKeyHash(a) || op.Auth.BidAmount.Cmp(s.cfg.BidWei) != 0 || op.Auth.MinBundleProfit.Sign() <= 0 {
		t.Fatalf("bad operation auth: %+v", op.Auth)
	}
	if len(op.Legs) != 2 || op.Legs[0].MaxSeizeAssets.Sign() <= 0 || op.Legs[0].MinProfit.Sign() <= 0 {
		t.Fatalf("encoded leg must carry maxSeizeAssets and minProfit, got %+v", op.Legs)
	}
	st, _ := s.state.load()
	wantBundleFloor := nativeToLoan(new(big.Int).Add(d.gasNative, d.bidNative), st.Rate)
	if op.Auth.MinBundleProfit.Cmp(wantBundleFloor) != 0 {
		t.Fatalf("minBundleProfit = %s, want %s", op.Auth.MinBundleProfit, wantBundleFloor)
	}
	for i, leg := range op.Legs {
		route := d.gas.Routes[i]
		wantLegFloor := nativeToLoan(gasCostNative(gasUnitsForRoute(route), s.cfg.MaxTxGasPrice), st.Rate)
		if leg.MinProfit.Cmp(wantLegFloor) != 0 {
			t.Fatalf("leg %d minProfit = %s, want %s for route %s", i, leg.MinProfit, wantLegFloor, route)
		}
	}
	if got := recoverCallbackAuthSigner(t, s, op); got != sgnr.addr {
		t.Fatalf("callback auth recovered %s, want signer %s", got, sgnr.addr)
	}
	if got := recoverSolveSigner(t, s, d.solve.Data); got != sgnr.addr {
		t.Fatalf("recovered %s, want signer %s", got, sgnr.addr)
	}
}

func TestBuildBidAllowsReplayedSameMarketBundle(t *testing.T) {
	s, _ := seededSolver(t)
	a := decodeAuction(t)

	d := s.buildBid(a, auctionClock())
	if d.skip != "" {
		t.Fatalf("expected a bid, got skip %q", d.skip)
	}
	opData, err := hexutil.Decode(d.solve.Data.OperationData)
	if err != nil {
		t.Fatal(err)
	}
	op, err := decodeOperationData(opData)
	if err != nil {
		t.Fatalf("decode operationData: %v", err)
	}
	legs := op.Legs
	if len(legs) != 2 {
		t.Fatalf("encoded %d legs, want two same-market legs after replay", len(legs))
	}
	if legs[0].MarketId != legs[1].MarketId {
		t.Fatalf("fixture should select two borrowers from one market, got %s and %s", legs[0].MarketId, legs[1].MarketId)
	}
	state := morpho.AccruedMarketState(goldenMarket(), uint64(a.Timestamp/1000))
	positions := map[common.Address]morpho.PositionState{
		common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE"): goldenBorrower(),
		common.HexToAddress("0x378a49c640fd9eea888a6a553caae441e2fdebc6"): {
			BorrowShares: mustBig("1582399974653062"), Collateral: mustBig("1000000000000000000"),
		},
	}
	for _, leg := range legs {
		pos, ok := positions[leg.Borrower]
		if !ok {
			t.Fatalf("unexpected borrower %s", leg.Borrower)
		}
		replay, ok := morpho.ApplySeizeLiquidation(state, pos, leg.MaxSeizeAssets, mustBig("1550000000000000000000000000"))
		if !ok {
			t.Fatalf("encoded leg for %s does not replay against current simulated state", leg.Borrower)
		}
		state = replay.Market
		positions[leg.Borrower] = replay.Position
	}
}

func TestBuildBidCapsBundleByCachedGasLimit(t *testing.T) {
	s, _ := seededSolver(t)
	st, _ := s.state.load()
	oneUnknownLeg := fixedGasUnits(defaultPriceUpdateFeeds) + gasFirstUnknownLeg
	st.GasLimit = headerGasLimitForUsable(oneUnknownLeg)
	s.state.store(st)

	d := s.buildBid(decodeAuction(t), auctionClock())
	if d.skip != "" {
		t.Fatalf("expected one gas-fit bid, got skip %q", d.skip)
	}
	if d.legs != 1 {
		t.Fatalf("legs = %d, want only one leg to fit cached gas limit", d.legs)
	}
	if d.gas.Units > usableBundleGasLimit(st.GasLimit) {
		t.Fatalf("predicted gas %d exceeds usable limit %d", d.gas.Units, usableBundleGasLimit(st.GasLimit))
	}
}

func TestComposeLoanPerEth(t *testing.T) {
	cases := []struct {
		name                             string
		ethUsd, loanUsd                  *big.Int
		ethFeedDec, loanFeedDec, loanDec int
		want                             string
	}{
		{"USDC at 2500, 8-dec feeds, 6-dec loan", mustBig("250000000000"), mustBig("100000000"), 8, 8, 6, "2500000000"},
		{"18-dec loan", mustBig("250000000000"), mustBig("100000000"), 8, 8, 18, "2500000000000000000000"},
		{"mixed feed decimals", mustBig("2500000000000000000000"), mustBig("100000000"), 18, 8, 6, "2500000000"},
		{"zero loan price", mustBig("250000000000"), big.NewInt(0), 8, 8, 6, ""},
		{"negative answer", big.NewInt(-1), mustBig("100000000"), 8, 8, 6, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := composeLoanPerEth(c.ethUsd, c.loanUsd, c.ethFeedDec, c.loanFeedDec, c.loanDec)
			if c.want == "" {
				if got != nil {
					t.Fatalf("want nil, got %s", got)
				}
				return
			}
			if got == nil || got.String() != c.want {
				t.Fatalf("got %v, want %s", got, c.want)
			}
		})
	}
}

func TestRateForUsesCachedOracleAndConversions(t *testing.T) {
	s, _ := seededSolver(t)
	st, _ := s.state.load()
	st.Rate = nil
	if got := s.rate(st); got != nil {
		t.Fatalf("no cached oracle rate should fail closed, got %v", got)
	}

	st.Rate = mustBig("2500000000")
	if got := s.rate(st); got == nil || got.String() != "2500000000" {
		t.Fatalf("oracle rate present → preferred over config, got %v", got)
	}

	if got := loanToNative(mustBig("2500000000"), mustBig("2500000000")); got.Cmp(morpho.Wad) != 0 {
		t.Fatalf("2500e6 loan at 2500e6/ETH = %s native units, want 1 ETH", got)
	}
	if got := loanToNative(mustBig("1"), nil); got.Sign() != 0 {
		t.Fatalf("nil rate should convert to 0, got %s", got)
	}
	if got := nativeToLoan(morpho.Wad, mustBig("2500000000")); got.String() != "2500000000" {
		t.Fatalf("1 native at 2500e6/native = %s loan units", got)
	}
}

func selectedBundleForTest(t *testing.T, s *Solver, a AuctionMessage) chosenBundle {
	t.Helper()
	scored := s.scoredLegs(a, auctionClock()())
	if len(scored) == 0 {
		t.Fatal("precondition: expected scored legs")
	}
	st, _ := s.state.load()
	b, skip := s.selectBundleWithGas(scored, st.Gas, st.GasLimit, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("precondition: selectBundle skip %q", skip)
	}
	return b
}

func TestBuildBidGasProfitabilityGate(t *testing.T) {
	a := decodeAuction(t)

	t.Run("net below min skips gas_unprofitable", func(t *testing.T) {
		s, _ := seededSolver(t)
		s.cfg.MaxTxGasPrice = mustBig("1000000000000000000")
		if d := s.buildBid(a, auctionClock()); d.skip != skipGasUnprofitable {
			t.Fatalf("skip = %q, want %q", d.skip, skipGasUnprofitable)
		}
	})

	t.Run("exact boundary passes", func(t *testing.T) {
		s, _ := seededSolver(t)
		b := selectedBundleForTest(t, s, a)
		st, _ := s.state.load()
		gasUnits := gasPredictionForBundle(b, st.Gas).Units
		if b.grossLoan.Cmp(new(big.Int).SetUint64(gasUnits)) <= 0 {
			t.Fatalf("test fixture cannot form exact gas boundary: gross=%s gasUnits=%d", b.grossLoan, gasUnits)
		}
		s.cfg.BidWei = new(big.Int).Sub(b.grossLoan, new(big.Int).SetUint64(gasUnits))
		s.cfg.MaxTxGasPrice = big.NewInt(1)
		st.Rate = morpho.Wad
		s.state.store(st)
		if d := s.buildBid(a, auctionClock()); d.skip != "" {
			t.Fatalf("exact after-cost boundary should pass, got skip %q", d.skip)
		}
	})

	t.Run("one wei below boundary skips", func(t *testing.T) {
		s, _ := seededSolver(t)
		b := selectedBundleForTest(t, s, a)
		st, _ := s.state.load()
		gasUnits := gasPredictionForBundle(b, st.Gas).Units
		if b.grossLoan.Cmp(new(big.Int).SetUint64(gasUnits)) <= 0 {
			t.Fatalf("test fixture cannot form gas boundary: gross=%s gasUnits=%d", b.grossLoan, gasUnits)
		}
		s.cfg.BidWei = new(big.Int).Sub(b.grossLoan, new(big.Int).SetUint64(gasUnits))
		s.cfg.BidWei.Add(s.cfg.BidWei, big.NewInt(1))
		s.cfg.MaxTxGasPrice = big.NewInt(1)
		st.Rate = morpho.Wad
		s.state.store(st)
		if d := s.buildBid(a, auctionClock()); d.skip != skipGasUnprofitable {
			t.Fatalf("skip = %q, want %q", d.skip, skipGasUnprofitable)
		}
	})

	t.Run("dry-run without rate skips because callback auth needs loan profit floors", func(t *testing.T) {
		s, _ := seededSolver(t)
		s.dryRun = true
		st, _ := s.state.load()
		st.Rate = nil
		s.state.store(st)
		if d := s.buildBid(a, auctionClock()); d.skip != skipGasUnprofitable {
			t.Fatalf("dry-run no-rate path should skip %q, got %q", skipGasUnprofitable, d.skip)
		}
	})
}

func TestBuildBidSignsConfiguredGasPriceCap(t *testing.T) {
	s, _ := seededSolver(t)
	s.cfg.MaxTxGasPrice = big.NewInt(1_000_000_000)

	d := s.buildBid(decodeAuction(t), auctionClock())
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
	t.Setenv("K", "k")
	t.Setenv(envDryRun, "false")

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(wsline+addrs+api+okBid), &node); err != nil {
		t.Fatal(err)
	}
	_, err := factory(node, solver.Deps{})
	if err == nil {
		t.Fatal("expected live factory to reject config without loanEthFeed")
	}
}

// TestBuildBidPriceSource proves the price-source switch through buildBid: the test-only on-chain path sizes
// against the cached on-chain price (ignoring a healthy frame → §6.6 dev-settlement fix), while the
// production auctioned path trusts the frame — a healthy frame skips, and a liquidatable frame drives a
// full SIZED bid (the otherwise-untested mainnet sizing path, since the dev testbed only ever runs the
// on-chain test flag). (Monitor-level marketPrice resolution is covered by TestMarketPriceSource.)
func TestBuildBidPriceSource(t *testing.T) {
	const feed = "0xfED5bC312C7139743bc3ab21Ef92f5AeB353339D"
	const px5000 = "5000000000000000000000000000" // healthy
	const px1550 = "1550000000000000000000000000" // the golden position is liquidatable here
	cases := []struct {
		name        string
		onchainTest bool
		framePx     string
		wantSkip    string
		wantSized   bool // for the bidding case, assert a full leg was sized
	}{
		{"onchain test flag bids against cached $1550 despite a healthy $5000 frame", true, px5000, "", false},
		{"auctioned trusts the healthy $5000 frame → no_legs", false, px5000, "no_legs", false},
		{"auctioned sizes a full bid at a liquidatable $1550 frame", false, px1550, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := decodeAuction(t)
			a.Payload.Prices = map[string]string{feed: tc.framePx}
			s, _ := seededSolver(t)
			if tc.onchainTest {
				useOnchainTestMonitor(t, s)
			}
			d := s.buildBid(a, auctionClock())
			if d.skip != tc.wantSkip {
				t.Fatalf("skip = %q, want %q", d.skip, tc.wantSkip)
			}
			if tc.wantSized && (d.legs < 1 || len(d.positions) < 1) {
				t.Fatalf("expected ≥1 sized leg, got legs=%d positions=%d", d.legs, len(d.positions))
			}
		})
	}
}

// TestBuildBidReservesInFlightFunding checks that a sent bid's payBid native is debited from the cached
// headroom so a second auction in the same window can't double-spend it — and that clearing the
// reservation (as refreshState does after a fresh on-chain read) re-opens the headroom.
func TestBuildBidReservesInFlightFunding(t *testing.T) {
	s, _ := seededSolver(t)
	a := decodeAuction(t)
	// Tighten the callback to exactly one bid's worth of native: a second in-flight bid must be blocked.
	st, _ := s.state.load()
	st.CallbackNative = new(big.Int).Set(s.cfg.BidWei)
	s.state.store(st)

	d1 := s.buildBid(a, auctionClock())
	if d1.skip != "" {
		t.Fatalf("first bid should succeed, got skip %q", d1.skip)
	}
	s.reserve(d1.bidNative, nil, d1.nonce, time.Unix(1781243340, 0), nil, "", gasPrediction{})

	if d2 := s.buildBid(a, auctionClock()); d2.skip != skipCallbackBalance {
		t.Fatalf("second in-flight bid should skip callback_balance (native already committed), got %q", d2.skip)
	}

	// A fresh on-chain read whose nonce REACHED d1's (it settled: the Executor sets the nonce to the
	// consumed bid's nonce) frees the reservation → headroom re-opens. Uses == d1.nonce, not +1, to pin
	// that settlement (on-chain nonce == bid nonce) is what releases it.
	s.pruneReservations(d1.nonce, time.Unix(1781243340, 0))
	if d3 := s.buildBid(a, auctionClock()); d3.skip != "" {
		t.Fatalf("after the bid resolved a bid should be allowed again, got skip %q", d3.skip)
	}
}

func TestBuildBidChecksDepositGasHeadroom(t *testing.T) {
	s, _ := seededSolver(t)
	a := decodeAuction(t)

	probe := s.buildBid(a, auctionClock())
	if probe.skip != "" {
		t.Fatalf("fixture should bid before tightening deposit, got skip %q", probe.skip)
	}
	required := new(big.Int).Add(minDeposit, probe.gasNative)
	st, _ := s.state.load()
	st.Exec.Deposit = new(big.Int).Sub(required, big.NewInt(1))
	s.state.store(st)

	if d := s.buildBid(a, auctionClock()); d.skip != "deposit_low" {
		t.Fatalf("deposit below predicted gas headroom should skip deposit_low, got %q", d.skip)
	}
}

func TestBuildBidReservesInFlightGasFunding(t *testing.T) {
	s, _ := seededSolver(t)
	a := decodeAuction(t)

	probe := s.buildBid(a, auctionClock())
	if probe.skip != "" {
		t.Fatalf("fixture should bid before tightening deposit, got skip %q", probe.skip)
	}
	st, _ := s.state.load()
	st.Exec.Deposit = new(big.Int).Add(minDeposit, probe.gasNative)
	s.state.store(st)

	d1 := s.buildBid(a, auctionClock())
	if d1.skip != "" {
		t.Fatalf("first bid should fit exactly one gas reservation, got skip %q", d1.skip)
	}
	s.reserve(d1.bidNative, d1.gasNative, d1.nonce, time.Unix(1781243340, 0), nil, "", d1.gas)

	if d2 := s.buildBid(a, auctionClock()); d2.skip != skipDepositLow {
		t.Fatalf("second bid should skip deposit_low because gas is already reserved, got %q", d2.skip)
	}

	s.pruneReservations(d1.nonce, time.Unix(1781243340, 0))
	if d3 := s.buildBid(a, auctionClock()); d3.skip != "" {
		t.Fatalf("after gas reservation clears the bid should fit again, got skip %q", d3.skip)
	}
}

// TestBuildBidSkipsInFlightPosition pins that a second rapid auction for an in-flight position is skipped
// instead of re-bid against the still-stale snapshot.
func TestBuildBidSkipsInFlightPosition(t *testing.T) {
	s, _ := seededSolver(t)
	onlyBorrower := common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE")
	snap := *snapshotOf(t, s)
	for market, positions := range snap.positions {
		for borrower := range positions {
			if borrower != onlyBorrower {
				delete(positions, borrower)
			}
		}
		snap.positions[market] = positions
	}
	storeSnapshot(t, s, &snap)
	a := decodeAuction(t)

	d1 := s.buildBid(a, auctionClock())
	if d1.skip != "" || len(d1.positions) == 0 {
		t.Fatalf("first bid should succeed with reserved positions, got skip %q positions %d", d1.skip, len(d1.positions))
	}
	s.reserve(d1.bidNative, nil, d1.nonce, time.Unix(1781243340, 0), d1.positions, "", gasPrediction{})

	if d2 := s.buildBid(a, auctionClock()); d2.skip != "in_flight" {
		t.Fatalf("a second auction for the same in-flight position(s) must skip in_flight, got %q", d2.skip)
	}

	// Once the bid resolves (the on-chain nonce REACHES the bid's nonce — settlement sets it to exactly
	// the consumed nonce), the positions free and become biddable again. Uses == d1.nonce, not +1.
	s.pruneReservations(d1.nonce, time.Unix(1781243340, 0))
	if d3 := s.buildBid(a, auctionClock()); d3.skip != "" {
		t.Fatalf("after the in-flight bid resolved the position should be biddable again, got %q", d3.skip)
	}
}

// TestPruneReservations pins the precise headroom release: a bid whose nonce fell below the on-chain nonce
// (submitted → settled/reverted) or that aged past reservationTTL (lost its auction) is freed, while a
// recent still-pending bid keeps its reservation. (A7).
func TestPruneReservations(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1781243340, 0)
	s.reserve(big.NewInt(100), big.NewInt(10), 8, now, nil, "", gasPrediction{})
	s.reserve(big.NewInt(200), big.NewInt(20), 10, now, nil, "", gasPrediction{})
	s.reserve(big.NewInt(300), big.NewInt(30), 12, now.Add(-time.Hour), nil, "", gasPrediction{})

	// nonce 10 frees 8 (below) AND 10 (settlement sets the on-chain nonce to the consumed bid's nonce, so
	// nonce == r.nonce must release it — the F1 fix: `<=`, not `<`); 12 is freed by age (> TTL) → none left.
	s.pruneReservations(10, now)
	if inFlight := s.inFlightSnapshot(); inFlight.bidNative.Sign() != 0 || inFlight.gasNative.Sign() != 0 {
		t.Fatalf("all reservations should be freed, got bid=%s gas=%s", inFlight.bidNative, inFlight.gasNative)
	}

	s.reserve(big.NewInt(500), big.NewInt(50), 11, now, nil, "", gasPrediction{})
	s.pruneReservations(10, now)
	if inFlight := s.inFlightSnapshot(); inFlight.bidNative.String() != "500" || inFlight.gasNative.String() != "50" {
		t.Fatalf("a recent pending bid should be kept, got bid=%s gas=%s", inFlight.bidNative, inFlight.gasNative)
	}

	// A bid is freed exactly when the on-chain nonce reaches its nonce (== r.nonce), not only when it passes.
	s.pruneReservations(11, now)
	if inFlight := s.inFlightSnapshot(); inFlight.bidNative.Sign() != 0 || inFlight.gasNative.Sign() != 0 {
		t.Fatalf("bid with nonce == on-chain nonce should be freed at settlement, got bid=%s gas=%s", inFlight.bidNative, inFlight.gasNative)
	}
}

func TestWonReservationSurvivesDelayedSettlement(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1781243340, 0)
	pos := []positionKey{{market: common.Hash{1}, borrower: common.Address{2}}}
	s.reserve(big.NewInt(100), big.NewInt(10), 8, now, pos, "auction-won", gasPrediction{})

	s.pruneReservations(7, now.Add(2*time.Minute))
	if inFlight := s.inFlightSnapshot(); inFlight.bidNative.String() != "100" || len(inFlight.positions) != 1 {
		t.Fatalf("won bid must stay reserved while settlement is delayed, bid=%s positions=%d", inFlight.bidNative, len(inFlight.positions))
	}
}

func TestReservationByAuctionCarriesGasPrediction(t *testing.T) {
	s, _ := seededSolver(t)
	pred := gasPrediction{Units: 350_000, Routes: []gasRoute{gasRouteAcquire}}
	s.reserve(big.NewInt(100), big.NewInt(50), 8, time.Unix(1781243340, 0), nil, "auction-1", pred)

	got, ok := s.reservationByAuction("auction-1")
	if !ok {
		t.Fatal("reservationByAuction did not find sent bid")
	}
	if got.gasUnits != pred.Units || got.gasRoutes != "acquire" {
		t.Fatalf("attribution = gas %d routes %q", got.gasUnits, got.gasRoutes)
	}
}

func TestAuctionResultReleasesLostBidReservation(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1781243340, 0)
	pos := []positionKey{{market: common.Hash{1}, borrower: common.Address{2}}}
	s.reserve(big.NewInt(100), big.NewInt(10), 8, now, pos, "auction-lost", gasPrediction{})

	s.handleMessage(context.Background(), []byte(`{
		"op":"auction-result",
		"id":"auction-lost",
		"data":{"bid":"0.0005","liquidator":"0x1111111111111111111111111111111111111111"}
	}`))
	if inFlight := s.inFlightSnapshot(); inFlight.bidNative.Sign() != 0 || inFlight.gasNative.Sign() != 0 || len(inFlight.positions) != 0 {
		t.Fatalf("lost auction must release reservation, bid=%s gas=%s inflight=%v", inFlight.bidNative, inFlight.gasNative, inFlight.positions)
	}

	s.reserve(big.NewInt(200), big.NewInt(20), 9, now, pos, "auction-won", gasPrediction{})
	s.handleMessage(context.Background(), []byte(`{
		"op":"auction-result",
		"id":"auction-won",
		"data":{"bid":"0.0005","liquidator":"`+`0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1`+`"}
	}`))
	if inFlight := s.inFlightSnapshot(); inFlight.bidNative.String() != "200" || inFlight.gasNative.String() != "20" {
		t.Fatalf("won auction must stay reserved until liquidation result/nonce, bid=%s gas=%s", inFlight.bidNative, inFlight.gasNative)
	}
}

func TestLiquidationResultReleasesOurReservation(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1781243340, 0)
	pos := []positionKey{{market: common.Hash{1}, borrower: common.Address{2}}}
	s.reserve(big.NewInt(100), big.NewInt(10), 8, now, pos, "auction-ours", gasPrediction{})

	s.handleMessage(context.Background(), []byte(`{
		"op":"liquidation-result",
		"id":"auction-ours",
		"data":{"success":true,"txHash":"","liquidator":"`+`0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1`+`","error":""}
	}`))
	if inFlight := s.inFlightSnapshot(); inFlight.bidNative.Sign() != 0 || inFlight.gasNative.Sign() != 0 || len(inFlight.positions) != 0 {
		t.Fatalf("our liquidation result must release reservation, bid=%s gas=%s inflight=%v", inFlight.bidNative, inFlight.gasNative, inFlight.positions)
	}

	s.reserve(big.NewInt(200), big.NewInt(20), 9, now, pos, "auction-other", gasPrediction{})
	s.handleMessage(context.Background(), []byte(`{
		"op":"liquidation-result",
		"id":"auction-other",
		"data":{"success":true,"txHash":"","liquidator":"0x1111111111111111111111111111111111111111","error":""}
	}`))
	if inFlight := s.inFlightSnapshot(); inFlight.bidNative.String() != "200" || inFlight.gasNative.String() != "20" {
		t.Fatalf("other solver liquidation result must not release our reservation, bid=%s gas=%s", inFlight.bidNative, inFlight.gasNative)
	}
}

// TestApplyExecutorStateRunsWithoutBalance pins that Executor-state bookkeeping still runs when only the
// callback balance read failed. A transient BalanceAt error must not strand reservations or stale nonces.
func TestApplyExecutorStateRunsWithoutBalance(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1781243340, 0)

	// A sent bid (nonce 8) pinning headroom, plus a stale local nonce high-water mark (5).
	s.reserve(big.NewInt(100), big.NewInt(10), 8, now, []positionKey{{market: common.Hash{1}, borrower: common.Address{2}}}, "", gasPrediction{})
	s.nonces.reconcile(5)
	if inFlight := s.inFlightSnapshot(); inFlight.bidNative.Sign() == 0 || inFlight.gasNative.Sign() == 0 {
		t.Fatal("precondition: the reservation should be present")
	}

	// On-chain nonce advanced to 9 (the bid settled). Run with bal=nil — the balance-read-failure path.
	st := ExecutorState{Nonce: big.NewInt(9), Deposit: mustBig("100000000000000000"), Locked: false}
	s.applyExecutorState(st, nil, now)

	// pruneReservations ran: nonce 8 <= 9 → the reservation is freed.
	if inFlight := s.inFlightSnapshot(); inFlight.bidNative.Sign() != 0 || inFlight.gasNative.Sign() != 0 {
		t.Fatalf("pruneReservations must run despite a failed balance read; bid=%s gas=%s", inFlight.bidNative, inFlight.gasNative)
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
	useOnchainTestMonitor(t, s) // size against the cached $1550 (the dev settlement price)

	// 1) Auction → a solve is sent on the wire. Stamp the frame as freshly emitted so the too_late gate
	// doesn't drop the captured fixture's long-past emit time.
	fresh := decodeAuction(t)
	fresh.Timestamp = time.Now().UnixMilli()
	setSnapshotBlockTime(t, s, fresh.Timestamp)
	s.handleMessage(context.Background(), marshal(fresh))
	frame := drainSend(s)
	if frame == nil {
		t.Fatal("expected a solve to be sent for a liquidatable auction")
	}
	var solve SolveMessage
	if err := json.Unmarshal(frame, &solve); err != nil {
		t.Fatal(err)
	}
	if solve.Op != "solve" || solve.ID != "6382e936-c915-496a-bb3e-fa3b4ccc3a8d" || len(solve.Data.Borrowers) != 2 {
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
	if d := s.buildBid(decodeAuction(t), time.Now); d.skip != "breaker" {
		t.Fatalf("tripped breaker must skip the bid, got skip %q", d.skip)
	}

	// 3) A fresh auction (new id so dedup can't mask it) is dropped by the breaker — nothing sent.
	a := decodeAuction(t)
	a.ID = "9999aaaa-0000-1111-2222-333344445555"
	a.Timestamp = time.Now().UnixMilli()
	setSnapshotBlockTime(t, s, a.Timestamp)
	s.handleMessage(context.Background(), marshal(a))
	if extra := drainSend(s); extra != nil {
		t.Fatalf("breaker tripped — expected no solve, got one: %s", extra)
	}
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

func TestFeedAuctionDoesNotBuildLiquidationBid(t *testing.T) {
	s, _ := seededSolver(t)
	raw := []byte(`{
	  "op":"auction","id":"feed-auction",
	  "timestamp":1726058300000,"durationMs":400,
	  "payload":{"ETH":"250000000000","BTC":"6000000000000","USDC":"99878787"}
	}`)
	s.handleMessage(context.Background(), raw)
	if frame := drainSend(s); frame != nil {
		t.Fatalf("feed auction must not produce a liquidation solve: %s", frame)
	}
}

// TestRedstoneClosedPositionNotBid proves we bid off our own tracked on-chain state, not the frame's
// pushed positions: even though the captured frame lists the borrower as deeply underwater, our cached
// position shows it fully closed (zero debt/collateral), so buildBid computes it non-liquidatable and
// does not bid. (The frame's pushed positions are ignored entirely — candidates come from snap.positions.)
func TestRedstoneClosedPositionNotBid(t *testing.T) {
	s, _ := seededSolver(t)
	id := common.HexToHash("0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5")
	borrower := common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE")

	// Build a FRESH snapshot whose tracked set is a single CLOSED position and store it (the loaded snapshot is
	// immutable once stored — mutating its maps would write through the atomic). The frame still lists it as
	// liquidatable, but candidates come from snap.positions.
	cur := snapshotOf(t, s)
	fresh := *cur
	fresh.positions = map[common.Hash]map[common.Address]morpho.PositionState{
		id: {borrower: {BorrowShares: big.NewInt(0), Collateral: big.NewInt(0)}},
	}
	storeSnapshot(t, s, &fresh)

	if d := s.buildBid(decodeAuction(t), auctionClock()); d.skip != "no_legs" {
		t.Fatalf("a closed tracked position is not liquidatable → no_legs, got %q", d.skip)
	}
}

// TestDryRunSuppressesSend pins the OEV_DRY_RUN observe mode: a profitable auction is fully evaluated
// (counted as a would-bid via metrics.bid()) but NO solve is sent on the wire — the operator can watch the
// bot's decisions against a live feed without funding or competing.
func TestDryRunSuppressesSend(t *testing.T) {
	s, _ := seededSolver(t)
	s.dryRun = true
	useOnchainTestMonitor(t, s) // size against the cached $1550

	// Real metrics on a fresh registry so we can read the would-bid counter back.
	reg := prometheus.NewRegistry()
	m, err := newMetrics(reg)
	if err != nil {
		t.Fatalf("newMetrics: %v", err)
	}
	s.metrics = m

	a := decodeAuction(t)
	a.Timestamp = time.Now().UnixMilli() // freshly emitted so the too_late gate doesn't drop it
	setSnapshotBlockTime(t, s, a.Timestamp)
	s.handleAuction(marshal(a))

	if f := drainSend(s); f != nil {
		t.Fatalf("dry-run must not send a solve, got %s", f)
	}
	if got := testutil.ToFloat64(m.bids); got != 1 {
		t.Fatalf("oev_bids_total = %v, want 1 (dry-run still counts the would-bid)", got)
	}
}

// TestHandleAuctionEmptyIdDedup is the regression for review F8: an empty-id frame must still be deduped on
// a content hash, so a replayed id-less frame can't be processed twice (a second nonce + a double bid). The
// first delivery sends a solve; an identical replay is dropped.
func TestHandleAuctionEmptyIdDedup(t *testing.T) {
	s, _ := seededSolver(t)
	useOnchainTestMonitor(t, s) // size against the cached $1550

	a := decodeAuction(t)
	a.ID = ""                            // the frame carries no id
	a.Timestamp = time.Now().UnixMilli() // freshly emitted so the too_late gate doesn't drop it
	setSnapshotBlockTime(t, s, a.Timestamp)
	raw := marshal(a)

	s.handleAuction(raw)
	if drainSend(s) == nil {
		t.Fatal("first empty-id auction should produce a solve")
	}
	s.handleAuction(raw) // identical replay
	if f := drainSend(s); f != nil {
		t.Fatalf("a replayed empty-id frame must be deduped (no second solve), got %s", f)
	}
}

// TestDedupKey pins the F8 key derivation: a present id is authoritative; an empty id derives a stable
// content hash that matches across identical frames and differs when prices differ.
func TestDedupKey(t *testing.T) {
	withID := AuctionMessage{ID: "abc"}
	if got := withID.dedupKey(); got != "id:abc" {
		t.Fatalf("present id must be the key, got %q", got)
	}
	base := AuctionMessage{Payload: AuctionPayload{
		Prices: map[string]string{"0xoracleA": "100", "0xoracleB": "200"},
	}}
	// Same content (prices map order is irrelevant — sorted) → same key.
	same := AuctionMessage{Payload: AuctionPayload{
		Prices: map[string]string{"0xoracleB": "200", "0xoracleA": "100"},
	}}
	if base.dedupKey() != same.dedupKey() {
		t.Fatal("identical empty-id frames must hash to the same key (order-independent)")
	}
	// A different price → a different key (not falsely deduped).
	diff := base
	diff.Payload.Prices = map[string]string{"0xoracleA": "101", "0xoracleB": "200"}
	if base.dedupKey() == diff.dedupKey() {
		t.Fatal("frames with different prices must not share a dedup key")
	}
	if got := base.dedupKey(); len(got) < 5 || got[:5] != "hash:" {
		t.Fatalf("empty-id key must be a content hash, got %q", got)
	}
}

// tokenA is the single loan token used by the bundling tests (the seeded adapter's loan token).
var tokenA = common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48") // USDC-like, 6dp

// scoredFor builds a minimal single-swap scoredLeg with the given borrower nonce and profit (loan units).
func scoredFor(borrowerByte byte, profit *big.Int) scoredLeg {
	var b common.Address
	b[19] = borrowerByte
	return scoredLeg{
		leg:    LiquidationLeg{Borrower: b, MarketId: common.Hash{}, SwapAmountOut: profit},
		profit: profit,
	}
}

func headerGasLimitForUsable(usable uint64) uint64 {
	return (usable*10_000 + bundleGasLimitSafetyBps - 1) / bundleGasLimitSafetyBps
}

func TestBundleSearchBounds(t *testing.T) {
	t.Run("candidate window keeps only top gross candidates", func(t *testing.T) {
		scored := make([]scoredLeg, 0, maxBundleSearchCandidates+100)
		for i := maxBundleSearchCandidates + 100; i > 0; i-- {
			scored = append(scored, scoredLeg{
				leg:    LiquidationLeg{Borrower: common.BigToAddress(big.NewInt(int64(i)))},
				profit: big.NewInt(int64(i)),
			})
		}

		got := bundleSearchCandidates(scored)
		if len(got) != maxBundleSearchCandidates {
			t.Fatalf("candidate window = %d, want %d", len(got), maxBundleSearchCandidates)
		}
		if got[0].profit.Int64() != int64(maxBundleSearchCandidates+100) || got[len(got)-1].profit.Int64() != 101 {
			t.Fatalf("candidate window did not keep top gross range: first=%s last=%s", got[0].profit, got[len(got)-1].profit)
		}
	})

	t.Run("depth follows usable gas", func(t *testing.T) {
		if got := bundleSearchDepth(1, defaultPriceUpdateFeeds); got != 0 {
			t.Fatalf("depth below fixed gas = %d, want 0", got)
		}
		usable := fixedGasUnits(defaultPriceUpdateFeeds) + gasFirstAcquireLeg + gasAdditionalAcquireLeg
		if got := bundleSearchDepth(headerGasLimitForUsable(usable), defaultPriceUpdateFeeds); got != 2 {
			t.Fatalf("depth = %d, want 2", got)
		}
	})
}

// TestSelectBundleSingleToken exercises the flat-bid selection: every scored leg is already expected-positive
// in sizeLeg, so selectBundle ranks by gross loan profit desc, keeps adding improving gas-fit legs, sums
// grossLoan, and only skips (no_legs) when the scored set is empty.
func TestSelectBundleSingleToken(t *testing.T) {
	newSolver := func(cfg *Config) *Solver {
		return &Solver{cfg: cfg, log: logr.Discard()}
	}

	t.Run("bundles all profitable legs into one bid, grossLoan summed", func(t *testing.T) {
		s := newSolver(&Config{})
		gasState := &gasPredictorState{
			FreeAssets:   big.NewInt(0),
			Withdrawable: big.NewInt(0),
			Acquire:      map[common.Address]*big.Int{{}: mustBig("100000000000")},
		}
		b, skip := s.selectBundleWithGas([]scoredLeg{
			scoredFor(1, mustBig("60000000")),
			scoredFor(2, mustBig("30000000")),
			scoredFor(3, mustBig("9000000")),
		}, gasState, redstoneExecutorMaxGasUnits, defaultPriceUpdateFeeds)
		if skip != "" {
			t.Fatalf("unexpected skip %q", skip)
		}
		if len(b.legs) != 3 || b.grossLoan.String() != "99000000" { // 60+30+9
			t.Fatalf("legs=%d grossLoan=%s, want 3 / 99000000", len(b.legs), b.grossLoan)
		}
	})

	t.Run("header gas limit caps the group, keeping the most profitable gas-fit subset", func(t *testing.T) {
		s := newSolver(&Config{})
		twoAcquireLegs := fixedGasUnits(defaultPriceUpdateFeeds) + gasFirstAcquireLeg + gasAdditionalAcquireLeg
		gasState := &gasPredictorState{
			FreeAssets:   big.NewInt(0),
			Withdrawable: big.NewInt(0),
			Acquire:      map[common.Address]*big.Int{{}: mustBig("100000000")},
		}
		b, skip := s.selectBundleWithGas([]scoredLeg{
			scoredFor(1, mustBig("10000000")),
			scoredFor(2, mustBig("30000000")),
			scoredFor(3, mustBig("20000000")),
		}, gasState, headerGasLimitForUsable(twoAcquireLegs), defaultPriceUpdateFeeds)
		if skip != "" {
			t.Fatalf("unexpected skip %q", skip)
		}
		if len(b.legs) != 2 || b.grossLoan.String() != "50000000" { // top two: 30 + 20
			t.Fatalf("legs=%d gross=%s, want 2 / 50000000", len(b.legs), b.grossLoan)
		}
	})

	t.Run("empty scored set → no_legs", func(t *testing.T) {
		s := newSolver(&Config{})
		if _, skip := s.selectBundle(nil); skip != "no_legs" {
			t.Fatalf("skip = %q, want no_legs", skip)
		}
	})

	t.Run("equal-profit legs ordered deterministically (borrower tie-break)", func(t *testing.T) {
		s := newSolver(&Config{})
		// Equal profit + zero marketId, so the deterministic tie-break is the borrower byte (ascending) —
		// the same signed bundle regardless of candidate iteration order.
		gasState := &gasPredictorState{
			FreeAssets:   big.NewInt(0),
			Withdrawable: big.NewInt(0),
			Acquire:      map[common.Address]*big.Int{{}: mustBig("30000000")},
		}
		b, skip := s.selectBundleWithGas([]scoredLeg{
			scoredFor(3, mustBig("10000000")),
			scoredFor(1, mustBig("10000000")),
			scoredFor(2, mustBig("10000000")),
		}, gasState, redstoneExecutorMaxGasUnits, defaultPriceUpdateFeeds)
		if skip != "" {
			t.Fatalf("unexpected skip %q", skip)
		}
		if len(b.legs) != 3 || b.legs[0].Borrower[19] != 1 || b.legs[1].Borrower[19] != 2 || b.legs[2].Borrower[19] != 3 {
			t.Fatalf("borrower order = %d,%d,%d, want 1,2,3 (deterministic tie-break)",
				b.legs[0].Borrower[19], b.legs[1].Borrower[19], b.legs[2].Borrower[19])
		}
	})
}

// TestSelectBundlePerCollateralBudget pins the shared-liquidity cap: legs seizing the same collateral can't
// jointly over-commit that collateral's getMaxAssets (scoredFor sets SwapAmountOut = profit), so the bundle
// won't revert with InsufficientAllocate on settlement. A leg on a different collateral is unaffected.
func TestSelectBundlePerCollateralBudget(t *testing.T) {
	s := &Solver{cfg: &Config{}, log: logr.Discard()}
	collA := common.HexToAddress("0x00000000000000000000000000000000000000ca")
	collB := common.HexToAddress("0x00000000000000000000000000000000000000cb")
	withColl := func(byteID byte, profit int64, c common.Address, maxA int64) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit)) // SwapAmountOut == profit
		sl.collateral = c
		sl.maxAssets = big.NewInt(maxA)
		return sl
	}
	// collA budget 100: leg#1 (60) fits; leg#2 (60) would push it to 120>100 → skipped. collB leg#3 fits.
	gasState := &gasPredictorState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{collA: big.NewInt(100), collB: big.NewInt(100)},
	}
	b, skip := s.selectBundleWithGas([]scoredLeg{
		withColl(1, 60, collA, 100),
		withColl(2, 60, collA, 100),
		withColl(3, 10, collB, 100),
	}, gasState, redstoneExecutorMaxGasUnits, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("unexpected skip %q", skip)
	}
	got := map[byte]bool{}
	for _, l := range b.legs {
		got[l.Borrower[19]] = true
	}
	if len(b.legs) != 2 || !got[1] || got[2] || !got[3] {
		t.Fatalf("included borrowers = %v (legs=%d), want {1,3} — the over-committing same-collateral leg dropped",
			got, len(b.legs))
	}
	if b.grossLoan.String() != "70" { // 60 (leg#1) + 10 (leg#3); leg#2 excluded
		t.Fatalf("grossLoan = %s, want 70", b.grossLoan)
	}
}

func TestSelectBundleAllowsSameMarketStaticLegs(t *testing.T) {
	s := &Solver{cfg: &Config{}, log: logr.Discard()}
	marketA := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	marketB := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	withMarket := func(byteID byte, profit int64, market common.Hash) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit))
		sl.leg.MarketId = market
		return sl
	}

	gasState := &gasPredictorState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{{}: big.NewInt(150)},
	}
	b, skip := s.selectBundleWithGas([]scoredLeg{
		withMarket(1, 60, marketA),
		withMarket(2, 50, marketA),
		withMarket(3, 40, marketB),
	}, gasState, redstoneExecutorMaxGasUnits, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("unexpected skip %q", skip)
	}
	got := map[byte]bool{}
	for _, leg := range b.legs {
		got[leg.Borrower[19]] = true
	}
	if len(b.legs) != 3 || !got[1] || !got[2] || !got[3] {
		t.Fatalf("selected borrowers = %v (legs=%d), want both same-market static legs plus other market", got, len(b.legs))
	}
}

func TestSelectBundleReplaysSameMarketSources(t *testing.T) {
	s := &Solver{
		cfg: &Config{
			Sizing: SizingParams{AllowFullLiquidation: true, SwapHaircutBps: 0},
		},
		log: logr.Discard(),
	}
	market := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	coll := common.HexToAddress("0x00000000000000000000000000000000000000c0")
	info := MarketInfo{
		Params: abiMarketParams{LoanToken: tokenA, CollateralToken: coll, Lltv: mustBig("500000000000000000")},
		State: morpho.MarketState{
			TotalSupplyAssets: mustBig("5000000000"),
			TotalSupplyShares: mustBig("5000000000"),
			TotalBorrowAssets: mustBig("3000000000"),
			TotalBorrowShares: mustBig("3000000000"),
			Lltv:              mustBig("500000000000000000"),
			Fee:               big.NewInt(0),
			BorrowRatePerSec:  big.NewInt(0),
		},
	}
	price := mustBig("1000000000000000000000000000")
	quote := newQuote("1200000000000000000000", nil)
	replayable := func(byteID byte) scoredLeg {
		var borrower common.Address
		borrower[19] = byteID
		pos := morpho.PositionState{BorrowShares: mustBig("1200000000"), Collateral: mustBig("1000000000000000000")}
		cand := Candidate{MarketID: market, Borrower: borrower, Market: info, Position: pos}
		leg, _, ok := sizeLeg(cand, price, quote, info.State.TotalBorrowAssets, s.cfg.Sizing)
		if !ok {
			t.Fatal("fixture should size")
		}
		leg.MaxSeizeAssets = big.NewInt(1) // stale/bogus: selection must ignore and recompute from source
		leg.SwapAmountOut = big.NewInt(1)  // stale/bogus
		return scoredLeg{
			leg:        leg,
			profit:     mustBig("999999999999999999"),
			collateral: coll,
			source:     evalItem{cand: cand, price: price, quote: quote, accrued: info.State.TotalBorrowAssets},
			replay:     true,
		}
	}

	gasState := &gasPredictorState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{coll: mustBig("10000000000000000000000")},
	}
	b, skip := s.selectBundleWithGas([]scoredLeg{replayable(1), replayable(2)}, gasState, redstoneExecutorMaxGasUnits, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("unexpected skip %q", skip)
	}
	if len(b.legs) != 2 {
		t.Fatalf("selected %d same-market replayed legs, want 2", len(b.legs))
	}
	if b.legs[0].MaxSeizeAssets.Cmp(big.NewInt(1)) == 0 || b.legs[0].SwapAmountOut.Cmp(big.NewInt(1)) == 0 {
		t.Fatalf("selected stale precomputed leg instead of replaying source: %+v", b.legs[0])
	}
	if b.grossLoan.Cmp(mustBig("999999999999999999")) >= 0 {
		t.Fatalf("grossLoan used stale bogus profit: %s", b.grossLoan)
	}
	if _, ok := morpho.ApplySeizeLiquidation(info.State, replayable(1).source.cand.Position, b.legs[0].MaxSeizeAssets, price); !ok {
		t.Fatal("first replayed leg should apply to initial market state")
	}
}

func TestSelectNetBundleAvoidsGrossBestGasFalseSkip(t *testing.T) {
	collHigh := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	collLow := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	withColl := func(byteID byte, profit int64, c common.Address) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit))
		sl.collateral = c
		return sl
	}
	s := &Solver{
		cfg: &Config{
			MaxTxGasPrice: big.NewInt(1),
			Sizing:        SizingParams{},
		},
		log: logr.Discard(),
	}
	gasState := &gasPredictorState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{collLow: big.NewInt(1_000_000)},
	}
	b, skip := s.selectNetBundle([]scoredLeg{
		withColl(1, 640_000, collHigh), // gross-best, but unknown route is net-negative even as a marginal leg
		withColl(2, 600_000, collLow),  // lower gross, acquire route clears fixed + acquire gas
	}, morpho.Wad, gasState, big.NewInt(1), 0, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("lower-gross passing route should be selected, got skip %q", skip)
	}
	if len(b.legs) != 1 || b.legs[0].Borrower[19] != 2 {
		t.Fatalf("selected borrowers = %+v, want only lower-gross acquire leg", b.legs)
	}
	if got := s.bundleNetNative(b, morpho.Wad, gasState, big.NewInt(1)); got.Cmp(big.NewInt(1)) < 0 {
		t.Fatalf("selected bundle net = %s, want >= min margin", got)
	}
}

func TestSelectNetBundleAllowsSameMarketStaticLegs(t *testing.T) {
	market := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	collA := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	collB := common.HexToAddress("0x00000000000000000000000000000000000000b2")
	withMarket := func(byteID byte, profit int64, c common.Address) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit))
		sl.leg.MarketId = market
		sl.collateral = c
		return sl
	}
	s := &Solver{
		cfg: &Config{
			MaxTxGasPrice: big.NewInt(1),
			Sizing:        SizingParams{},
		},
		log: logr.Discard(),
	}
	gasState := &gasPredictorState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire: map[common.Address]*big.Int{
			collA: big.NewInt(700_000),
			collB: big.NewInt(700_000),
		},
	}
	b, skip := s.selectNetBundle([]scoredLeg{
		withMarket(1, 700_000, collA),
		withMarket(2, 700_000, collB),
	}, morpho.Wad, gasState, big.NewInt(1), 0, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("unexpected skip %q", skip)
	}
	if len(b.legs) != 2 {
		t.Fatalf("selected %d same-market legs, want 2", len(b.legs))
	}
}

func TestSelectNetBundleSharesBaseGasAcrossLegs(t *testing.T) {
	collA := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	collB := common.HexToAddress("0x00000000000000000000000000000000000000b2")
	withColl := func(byteID byte, profit int64, c common.Address) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit))
		sl.collateral = c
		return sl
	}
	s := &Solver{
		cfg: &Config{
			MaxTxGasPrice: big.NewInt(1),
			Sizing:        SizingParams{},
		},
		log: logr.Discard(),
	}
	gasState := &gasPredictorState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire: map[common.Address]*big.Int{
			collA: big.NewInt(590_000),
			collB: big.NewInt(590_000),
		},
	}
	b, skip := s.selectNetBundle([]scoredLeg{
		withColl(1, 590_000, collA), // singleton cannot cover fixed + acquire gas
		withColl(2, 590_000, collB), // together shares fixed gas and clears the bundle gate
	}, morpho.Wad, gasState, big.NewInt(1), 0, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("combined bundle should share base gas and pass, got skip %q", skip)
	}
	if len(b.legs) != 2 {
		t.Fatalf("selected %d legs, want 2", len(b.legs))
	}
	if got := s.bundleNetNative(b, morpho.Wad, gasState, big.NewInt(1)); got.Cmp(big.NewInt(1)) < 0 {
		t.Fatalf("selected bundle net = %s, want >= min margin", got)
	}
}

func TestSelectNetBundleSearchesPastGreedyBudgetTrap(t *testing.T) {
	coll := common.HexToAddress("0x00000000000000000000000000000000000000cc")
	withColl := func(byteID byte, profit int64) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit))
		sl.collateral = coll
		sl.maxAssets = big.NewInt(1_240_000)
		return sl
	}
	s := &Solver{
		cfg: &Config{
			BidWei:        big.NewInt(0),
			MaxTxGasPrice: big.NewInt(1),
			Sizing:        SizingParams{},
		},
		log: logr.Discard(),
	}
	gasState := &gasPredictorState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{coll: big.NewInt(1_400_000)},
	}
	b, skip := s.selectNetBundle([]scoredLeg{
		withColl(1, 700_000), // gross-best consumes too much shared budget to pair with either 500k leg
		withColl(2, 620_000),
		withColl(3, 620_000),
	}, morpho.Wad, gasState, big.NewInt(1), 0, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("expected lower-gross pair to pass, got skip %q", skip)
	}
	got := map[byte]bool{}
	for _, leg := range b.legs {
		got[leg.Borrower[19]] = true
	}
	if len(b.legs) != 2 || got[1] || !got[2] || !got[3] {
		t.Fatalf("selected borrowers = %v (legs=%d), want {2,3}", got, len(b.legs))
	}
	if gotNet := s.bundleNetNative(b, morpho.Wad, gasState, big.NewInt(1)); gotNet.Cmp(big.NewInt(1)) < 0 {
		t.Fatalf("selected bundle net = %s, want >= min margin", gotNet)
	}
}

func TestSearchBundleDoesNotRequireMonotonicScore(t *testing.T) {
	s := &Solver{cfg: &Config{}, log: logr.Discard()}
	legs := []scoredLeg{
		scoredFor(1, big.NewInt(1)),
		scoredFor(2, big.NewInt(1)),
	}
	scoreFn := func(b chosenBundle) *big.Int {
		if len(b.legs) < 2 {
			return big.NewInt(-1)
		}
		return big.NewInt(10)
	}

	gasState := &gasPredictorState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{{}: big.NewInt(2)},
	}
	best, ok := s.searchBundle(legs, gasState, redstoneExecutorMaxGasUnits, defaultPriceUpdateFeeds, scoreFn)
	if !ok {
		t.Fatal("search should keep temporary negative states when a deeper bundle can become profitable")
	}
	if len(best.bundle.legs) != 2 {
		t.Fatalf("selected %d legs, want 2", len(best.bundle.legs))
	}
}

func TestBundleBidNativeUsesProfitShareFloor(t *testing.T) {
	b := chosenBundle{grossLoan: big.NewInt(1_000)}
	s := &Solver{
		cfg: &Config{
			BidWei:               big.NewInt(100),
			TotalBundleProfitBps: 2_000,
		},
		log: logr.Discard(),
	}
	if got := s.bundleBidNative(b, morpho.Wad); got.Cmp(big.NewInt(200)) != 0 {
		t.Fatalf("bid = %s, want 20%% of gross native", got)
	}
	s.cfg.TotalBundleProfitBps = 500
	if got := s.bundleBidNative(b, morpho.Wad); got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("bid = %s, want minimal bid floor", got)
	}
}

// TestBuildBidStaleEpoch pins the fail-closed epoch gate: a non-empty snapshot must be block-tagged and
// close enough to the auction timestamp that a stuck API cache cannot keep bidding indefinitely.
func TestBuildBidStaleEpoch(t *testing.T) {
	a := decodeAuction(t)
	now := auctionClock()
	s, _ := seededSolver(t)

	fresh := *snapshotOf(t, s)
	fresh.block, fresh.blockTime = 0, 0
	storeSnapshot(t, s, &fresh)
	if d := s.buildBid(a, now); d.skip != skipStaleEpoch {
		t.Fatalf("untagged snapshot must skip %s, got %q", skipStaleEpoch, d.skip)
	}

	fresh.block, fresh.blockTime = 123, uint64(a.Timestamp/1000)
	storeSnapshot(t, s, &fresh)
	if d := s.buildBid(a, now); d.skip != "" {
		t.Fatalf("current tagged snapshot should bid, got skip %q", d.skip)
	}

	fresh.blockTime = uint64(a.Timestamp/1000) - uint64(snapshotMaxAuctionLag/time.Second) - 1
	storeSnapshot(t, s, &fresh)
	if d := s.buildBid(a, now); d.skip != skipStaleEpoch {
		t.Fatalf("old tagged snapshot must skip %s, got %q", skipStaleEpoch, d.skip)
	}
}

func TestLegResultCode(t *testing.T) {
	code := new(big.Int).Lsh(big.NewInt(0xdeadbeef), 224)
	code.Or(code, new(big.Int).Lsh(big.NewInt(42), 16))
	code.Or(code, new(big.Int).Lsh(big.NewInt(3), 8))
	code.Or(code, big.NewInt(7))

	got := legResultCode(code)
	if got.index != 42 || got.status != 3 || got.reason != 7 || got.selector != "0xdeadbeef" {
		t.Fatalf("decoded code = (%d,%d,%d,%q), want (42,3,7,0xdeadbeef)", got.index, got.status, got.reason, got.selector)
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
			s.handleMessage(context.Background(), frame(s.cfg.Callback.Hex(), false))
		}
		if tripped, _ := s.breaker.tripped(now); !tripped {
			t.Fatal("3 failed liquidation-result frames for our callback must trip the breaker")
		}
	})

	t.Run("success:true records none", func(t *testing.T) {
		s, _ := seededSolver(t)
		for i := 0; i < 5; i++ {
			s.handleMessage(context.Background(), frame(s.cfg.Callback.Hex(), true))
		}
		if tripped, _ := s.breaker.tripped(now); tripped {
			t.Fatal("successful liquidation-result frames must not trip the breaker")
		}
	})

	t.Run("a failure for another liquidator records none", func(t *testing.T) {
		s, _ := seededSolver(t)
		other := common.HexToAddress("0x2222222222222222222222222222222222222222").Hex()
		for i := 0; i < 5; i++ {
			s.handleMessage(context.Background(), frame(other, false))
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

func TestBuildBidSkips(t *testing.T) {
	clock := auctionClock()
	healthy := mustBig("100000000000000000000000000000000000000000000")

	stateWith := func(s *Solver, deposit, callback *big.Int, locked bool) cachedState {
		st, _ := s.state.load()
		st.Exec = ExecutorState{Nonce: big.NewInt(7), Deposit: deposit, Locked: locked}
		st.CallbackNative = callback
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
			s.state.store(stateWith(s, mustBig("100000000000000000"), mustBig("1000000000000000000"), true))
		}, want: "signer_locked"},
		{name: "deposit_low", mut: func(s *Solver) {
			s.state.store(stateWith(s, big.NewInt(1), mustBig("1000000000000000000"), false)) // below MIN_DEPOSIT (1e13)
		}, want: "deposit_low"},
		{name: "callback_balance", mut: func(s *Solver) {
			s.state.store(stateWith(s, mustBig("100000000000000000"), big.NewInt(1), false))
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
			if d := s.buildBid(frame, clock); d.skip != tc.want {
				t.Fatalf("skip = %q, want %q", d.skip, tc.want)
			}
		})
	}
}
