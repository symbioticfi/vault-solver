// Package redstoneoev implements the RedStone Atom OEV liquidation solver: it subscribes to OEV
// auctions over WebSocket, computes liquidatable Morpho Blue positions over our own independently-tracked
// at-risk set (Morpho API, or Sepolia testMonitor seeds; the auction frame supplies prices only), signs EXECUTOR_V6 bids,
// and replies with solve payloads that settle through an on-chain IOperationCallback contract (the single
// LiquidLane adapter). It self-registers via init(). See docs/OEV-PLAN.md.
package redstoneoev

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

// Name is the registry key that selects this solver from config.
const Name = "redstone-oev"

// minDeposit is the Executor's MIN_DEPOSIT (0.00001 ETH) — below this, settlement reverts (§6.2).
var minDeposit = big.NewInt(1e13)

// skipNoLegs is the bounded skip reason for "no liquidatable leg this auction" (a metric label).
const skipNoLegs = "no_legs"

// skipGasUnprofitable is the bounded skip reason for bundles whose loan profit does not cover estimated
// settlement gas plus the configured min-profit margin.
const skipGasUnprofitable = "gas_unprofitable"

// skipStaleEpoch is the bounded skip reason for missing or stale monitor snapshot epochs.
const skipStaleEpoch = "stale_epoch"

const (
	skipDepositLow      = "deposit_low"
	skipCallbackBalance = "callback_balance"
)

//nolint:gochecknoinits // self-registration with the solver framework is the intended plugin pattern.
func init() {
	solver.Register(Name, factory)
}

// Solver is the RedStone OEV bidding strategy.
type Solver struct {
	cfg     *Config
	deps    solver.Deps
	chainID *big.Int
	dryRun  bool // OEV_DRY_RUN: observe mode — sign + log would-bids, never send (env knob, not config)
	reader  *reader
	mon     monitorSource // Morpho market/position monitor — the single OEV opportunity source
	nonces  *nonceStore
	breaker *breaker
	metrics *metrics
	ws      *wsClient
	seen    *seenAuctions // de-dup of already-processed auction ids (WS-goroutine-only)
	log     logr.Logger

	state stateCache // cached executor accounting + callback balance, refreshed by the ops loop

	// resMu guards res: the per-bid reservation of payBid native and predicted gas debit committed by bids
	// already SENT but not yet reflected on-chain. buildBid debits these reservations from the cached callback
	// balance and Executor deposit so bids inside one ops-poll window cannot over-commit either funding pot.
	// pruneReservations frees a bid once it RESOLVES — its nonce fell below the on-chain nonce (submitted →
	// settled or reverted; the fresh read reflects it) or it aged past reservationTTL as a last-resort cleanup
	// for missed result frames — so fresh unresolved bids keep pinning headroom.
	resMu sync.Mutex
	res   []reservedBid
}

func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	apiKey := os.Getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		return nil, errors.Errorf("%s: ws api key env %q is empty", Name, cfg.APIKeyEnv)
	}
	// Dev/test knobs come from env (read at point of use), never config: the on-chain price basis and the
	// dry-run observe mode. Malformed values fail closed here.
	onchainPrice, err := onchainPriceForTestEnv()
	if err != nil {
		return nil, errors.Errorf("%s: %w", Name, err)
	}
	testMonitor, err := testMonitorEnv()
	if err != nil {
		return nil, errors.Errorf("%s: %w", Name, err)
	}
	dryRun, err := dryRunEnv()
	if err != nil {
		return nil, errors.Errorf("%s: %w", Name, err)
	}
	chainID := deps.Chain.ChainID()
	if !chainID.IsInt64() || chainID.Sign() <= 0 {
		return nil, errors.Errorf("%s: chain id %s out of supported range", Name, chainID)
	}
	log := deps.Log.WithName(Name)
	rdr := newReader(deps.Chain, log)
	var mon monitorSource
	if testMonitor {
		mon, err = newTestMonitor(rdr, log, cfg)
		if err != nil {
			return nil, errors.Errorf("%s: %w", Name, err)
		}
	} else {
		if cfg.MorphoAPIURL == "" {
			return nil, errors.Errorf("%s: morphoApiUrl is required unless %s=true", Name, envTestMonitor)
		}
		mon = newAPIMonitor(rdr, log, cfg, chainID.Int64())
	}
	if onchainPrice && !testMonitor {
		return nil, errors.Errorf("%s: %s requires %s=true", Name, envOnchainPrice, envTestMonitor)
	}

	var mx *metrics
	if deps.Metrics != nil {
		if mx, err = newMetrics(deps.Metrics.Registerer()); err != nil {
			return nil, err
		}
	}

	s := &Solver{
		cfg:     cfg,
		deps:    deps,
		chainID: chainID,
		dryRun:  dryRun,
		reader:  rdr,
		mon:     mon,
		nonces:  &nonceStore{},
		breaker: newBreaker(cfg.BreakerMaxFailures, cfg.BreakerWindow),
		metrics: mx,
		seen:    newSeenAuctions(maxSeenAuctions),
		log:     log,
	}
	topics := []string{"oev/liquidations", "oev/feeds", "oev/notify/" + strings.ToLower(cfg.Callback.Hex())}
	s.ws = newWSClient(wsConfig{URL: cfg.WSURL, APIKey: apiKey, Topics: topics}, log, s.handleMessage)
	return s, nil
}

// Name identifies the solver.
func (s *Solver) Name() string { return Name }

// Run warms the caches, starts the monitor + ops loops, and serves the WS auction stream until ctx
// is cancelled.
func (s *Solver) Run(ctx context.Context) error {
	s.log.Info("starting",
		"callback", s.cfg.Callback.Hex(), "executor", s.cfg.Executor.Hex(),
		"adapter", s.cfg.Adapter.Hex(), "monitor", s.mon.name(),
		"dryRun", s.dryRun, "signer", s.deps.Signer.Address().Hex())
	s.mon.refresh(ctx)  // seed market quotes before state; the gas predictor derives its collateral set from them
	s.refreshState(ctx) // seed nonce + deposit + callback balance before any bid

	// Start the background loops and join them on shutdown so no goroutine outlives Run (and races
	// deps teardown). The monitor runs its own market/position refresh loops; the WS client blocks
	// until ctx is cancelled; the ops loop keys off the same ctx.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.mon.run(ctx) }()
	go func() { defer wg.Done(); s.opsLoop(ctx) }()
	err := s.ws.Run(ctx)
	wg.Wait()
	return err
}

// opsLoop periodically refreshes the Executor accounting (nonce/deposit/locked) used for pre-bid
// checks and nonce reconciliation.
func (s *Solver) opsLoop(ctx context.Context) {
	t := time.NewTicker(s.cfg.OpsPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refreshState(ctx)
		}
	}
}

// refreshState reads the signer's Executor accounting and the callback's native balance into the cache,
// and reconciles the nonce high-water mark. Run by the ops loop so deposit/nonce/balance stay fresh.
//
// The Executor-state read is the load-bearing one: nonce reconciliation, reservation pruning, and the
// deposit-low alarm all derive from st alone. A transient BalanceAt failure must NOT skip that bookkeeping
// (it would leave reservations pinning headroom and the nonce high-water mark stale). So on a balance read
// failure we keep the previously-cached callback balance (don't overwrite with a bad value) but STILL run
// the executor-derived bookkeeping (applyExecutorState).
func (s *Solver) refreshState(ctx context.Context) {
	head, herr := s.latestHeadState(ctx)
	if herr != nil {
		s.log.Error(herr, "read block for state refresh failed; keeping cache")
		return
	}
	epoch := newReadEpoch(head.Number, time.Now())
	st, err := s.reader.ReadExecutorState(ctx, s.cfg.Executor, s.deps.Signer.Address())
	if err != nil {
		s.log.Error(err, "read executor state failed; keeping cache")
		return
	}
	// Callback native balance: on a read failure keep the last good cached value rather than overwriting it
	// (an absent cache stays absent — pre-bid funding then fails closed on state_unknown).
	bal, berr := s.deps.Chain.BalanceAt(ctx, s.cfg.Callback, nil)
	if berr != nil {
		s.log.Error(berr, "read callback balance failed; keeping last cached balance")
		if prev, ok := s.state.load(); ok {
			bal = prev.CallbackNative
		} else {
			bal = nil
		}
	}
	// Publish a usable state only with a callback balance in hand. Storing a nil balance would make load()
	// report ready, and the callback_balance gate's Sub(CallbackNative, …) would nil-deref and crash the WS
	// goroutine. With no balance (read failed AND no prior cache) keep the cache absent so pre-bid funding
	// fails closed on state_unknown — as the comment above intends. applyExecutorState still runs the
	// balance-independent bookkeeping (reservations/nonce/deposit) regardless.
	if bal != nil {
		rate := s.reader.ReadLoanEthRate(ctx, s.cfg.Adapter, s.cfg.LoanEthFeed, epoch.At)
		gasState, gerr := s.reader.ReadGasPredictorState(ctx, s.cfg.Adapter, quoteCollateralsFromSnapshot(s.mon.snapshot()))
		if gerr != nil {
			s.log.Error(gerr, "read gas predictor state failed; keeping last cached predictor state")
			if prev, ok := s.state.load(); ok {
				gasState = prev.Gas
			}
		}
		if !s.epochStillCurrent(ctx, epoch, "state") {
			return
		}
		s.state.store(cachedState{
			Exec: st, CallbackNative: bal, Rate: rate, Gas: gasState,
			GasLimit: head.GasLimit,
		})
	}
	s.applyExecutorState(st, bal, epoch.At)
}

type latestHeadState struct {
	Number   uint64
	GasLimit uint64
}

func (s *Solver) latestHeadState(ctx context.Context) (latestHeadState, error) {
	header, err := s.deps.Chain.HeaderByNumber(ctx, nil)
	if err == nil {
		if header == nil || header.Number == nil || !header.Number.IsUint64() {
			return latestHeadState{}, errors.New("latest header missing uint64 block number")
		}
		return latestHeadState{Number: header.Number.Uint64(), GasLimit: header.GasLimit}, nil
	}
	head, berr := s.deps.Chain.BlockNumber(ctx)
	if berr != nil {
		return latestHeadState{}, err
	}
	s.log.Error(err, "read latest header failed; using RedStone gas limit cap")
	return latestHeadState{Number: head}, nil
}

func (s *Solver) epochStillCurrent(ctx context.Context, epoch readEpoch, label string) bool {
	head, err := s.deps.Chain.BlockNumber(ctx)
	if err != nil {
		s.log.Error(err, "read block after "+label+" refresh failed; keeping cache")
		return false
	}
	if head != epoch.Block {
		s.log.V(1).Info("refresh crossed block boundary; keeping cache", "phase", label, "startBlock", epoch.Block, "endBlock", head)
		return false
	}
	return true
}

// applyExecutorState runs the bookkeeping derived purely from the Executor state read (nonce + deposit):
// reservation pruning, nonce high-water reconciliation, the balance gauges, and the deposit-low alarm.
// Split out of refreshState so it runs whenever ReadExecutorState succeeds — independent of the BalanceAt
// outcome. `bal` (the callback native, possibly the last cached value or nil) drives only the balance gauge.
// No I/O → directly unit-testable.
func (s *Solver) applyExecutorState(st ExecutorState, bal *big.Int, now time.Time) {
	s.pruneReservations(st.Nonce.Uint64(), now) // free bids the fresh read shows resolved (or aged out)
	s.nonces.reconcile(st.Nonce.Uint64())
	if bal != nil {
		s.metrics.balances(weiFloat(st.Deposit), weiFloat(bal))
	}
	// Deposit-drain alert: the gas pool drains on every settlement (even reverts; gas is debited from the
	// deposit post-settlement, §6.2). Below MIN_DEPOSIT settlement always reverts, so surface that floor
	// breach loudly; per-bid predicted-gas headroom is checked in buildBid.
	belowFloor := st.Deposit.Cmp(minDeposit) < 0
	s.metrics.depositBelowFloor(belowFloor)
	if belowFloor {
		s.log.Error(errors.New("executor deposit below MIN_DEPOSIT"),
			"bidding will skip until refueled (scripts/oev/oev-balance.sh topup-deposit)",
			"depositWei", st.Deposit, "minDepositWei", minDeposit)
	}
	s.log.V(1).Info("state", "nonce", st.Nonce, "depositWei", st.Deposit, "locked", st.Locked, "callbackWei", bal)
}

// handleMessage dispatches an inbound WS frame by op. Unknown/garbled frames are logged and dropped
// (the auctioneer is lenient and silent on bad input — §6.7).
func (s *Solver) handleMessage(ctx context.Context, raw []byte) {
	op, err := opName(raw)
	if err != nil {
		s.log.V(1).Error(err, "drop unparseable frame")
		return
	}
	switch op {
	case "auction":
		if isFeedAuction(raw) {
			s.log.V(1).Info("ignoring feed auction")
			return
		}
		s.handleAuction(raw)
	case "auction-result":
		var r AuctionResult
		if err := json.Unmarshal(raw, &r); err != nil {
			s.log.V(1).Error(err, "drop malformed frame", "op", op)
		} else {
			won := strings.EqualFold(r.Data.Liquidator, s.cfg.Callback.Hex())
			if won {
				s.metrics.won()
			} else {
				s.releaseReservationByAuction(r.ID)
			}
			s.log.Info("auction-result", "id", r.ID, "winner", r.Data.Liquidator, "bid", r.Data.Bid, "won", won)
		}
	case "liquidation-result":
		var r LiquidationResult
		if err := json.Unmarshal(raw, &r); err != nil {
			s.log.V(1).Error(err, "drop malformed frame", "op", op)
		} else {
			// This is the breaker's failure feed. The frame is delivered on both the broadcast oev/liquidations
			// and the callback-scoped oev/notify/<callback> subscription, so a result may belong to another
			// solver — gate on Data.Liquidator == our callback (same won-detection as auction-result) before
			// recording a failure, so a revert storm trips the breaker but other solvers' reverts never do.
			ours := strings.EqualFold(r.Data.Liquidator, s.cfg.Callback.Hex())
			pred, hasPred := s.reservationByAuction(r.ID)
			s.log.Info("liquidation-result", "id", r.ID, "success", r.Data.Success,
				"txHash", r.Data.TxHash, "error", r.Data.Error, "ours", ours,
				"predictedGas", pred.gasUnits, "predictedRoute", pred.gasRoutes)
			if ours {
				if hasPred && r.Data.TxHash != "" {
					go s.attributeSettlementGas(ctx, r.Data.TxHash, pred)
				}
				s.releaseReservationByAuction(r.ID)
				if !r.Data.Success {
					s.breaker.recordFailure(time.Now())
					s.metrics.failed()
				}
			}
		}
	case "blacklisted":
		var b Blacklisted
		_ = json.Unmarshal(raw, &b)
		s.breaker.blacklist() // actually halt bidding, not just log
		s.log.Error(errors.New("api key blacklisted"), "halting bidding", "msg", b.Data.Msg)
	default:
		s.log.V(1).Info("ignoring frame", "op", op)
	}
}

// bidDecision is the outcome of evaluating one auction: either a ready-to-send solve (skip == "") or
// a bounded skip reason (a metric label, never free-form/attacker-derived). gross is the bundle's Σ
// loan-token profit, carried for logging only.
type bidDecision struct {
	solve     SolveMessage
	legs      int
	gross     *big.Int
	bidNative *big.Int
	gasNative *big.Int
	nonce     uint64        // the bid's signed nonce, so handleAuction reserves headroom keyed on it
	positions []positionKey // the bundle's (market,borrower) legs, reserved so they aren't re-bid in-flight
	gas       gasPrediction
	skip      string
}

// handleAuction is the hot path: unmarshal, run the (testable, I/O-free) buildBid, emit metrics, and
// either send the solve or log the skip. The ~400ms auction budget is observed via the hotPath latency
// histogram.
func (s *Solver) handleAuction(raw []byte) {
	start := time.Now()
	var a AuctionMessage
	if err := json.Unmarshal(raw, &a); err != nil {
		s.log.V(1).Error(err, "drop malformed auction")
		return
	}
	s.metrics.auction()
	// Drop a duplicate delivery of the same auction (a reconnect re-subscribe can replay frames): bidding
	// twice would burn a second nonce and reserve a second headroom for one auction. An empty-id frame still
	// dedups — on a content hash (dedupKey) — so a replayed id-less frame can't slip past and double-bid.
	if s.seen.seen(a.dedupKey()) {
		s.metrics.skip("duplicate")
		s.log.V(1).Info("duplicate auction; already processed", "auction", a.ID)
		return
	}
	d := s.buildBid(a, time.Now)
	s.metrics.latency(time.Since(start))

	if d.skip != "" {
		s.metrics.skip(d.skip)
		s.log.V(1).Info("no bid", "auction", a.ID, "reason", d.skip)
		return
	}
	if s.dryRun {
		s.metrics.bid()
		s.log.Info("DRY-RUN would bid", "auction", a.ID, "legs", d.legs, "nonce", d.solve.Data.Nonce,
			"bidEth", d.solve.Data.Bid, "grossProfit", d.gross, "predictedGas", d.gas.Units,
			"predictedRoute", gasRoutesString(d.gas.Routes))
		return
	}
	// Don't send a bid that overran the auction's own deadline: the auctioneer rejects late solves, and a
	// sent-but-rejected bid would still reserve funding until the next refresh. Measure against the auction's
	// TRUE deadline — elapsed since the auctioneer EMITTED the frame (a.Timestamp) — so a late-DELIVERED frame
	// (WS transit) is dropped, not just one we were slow to process. Matters most with a remote signer whose
	// latency lands inside buildBid's SignBid.
	if a.TimeoutMs > 0 && tooLate(a.Timestamp, a.TimeoutMs, start, time.Now()) {
		s.metrics.skip("too_late")
		s.log.Info("bid not sent: auction deadline (since emit) exceeded",
			"auction", a.ID, "timeoutMs", a.TimeoutMs, "sinceEmitMs", sinceEmitMs(a.Timestamp, time.Now()),
			"localElapsedMs", time.Since(start).Milliseconds())
		return
	}
	if !s.ws.Send(marshal(d.solve)) {
		// The frame never left the process — don't count it as a bid or reserve its funding.
		s.metrics.skip("send_dropped")
		s.log.Info("bid NOT sent (ws buffer full)", "auction", a.ID, "nonce", d.solve.Data.Nonce)
		return
	}
	s.reserve(d.bidNative, d.gasNative, d.nonce, time.Now(), d.positions, a.ID, d.gas)
	s.metrics.bid()
	s.log.Info("bid sent", "auction", a.ID, "legs", d.legs, "nonce", d.solve.Data.Nonce,
		"bidEth", d.solve.Data.Bid, "grossProfit", d.gross, "predictedGas", d.gas.Units,
		"predictedRoute", gasRoutesString(d.gas.Routes))
}

func (s *Solver) attributeSettlementGas(ctx context.Context, txHash string, pred reservedBid) {
	if !common.IsHexHash(txHash) {
		s.log.Info("settlement gas attribution skipped: bad tx hash", "txHash", txHash)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	receipt, err := s.deps.Chain.TransactionReceipt(ctx, common.HexToHash(txHash))
	if err != nil {
		s.log.Error(err, "settlement gas attribution failed", "txHash", txHash)
		return
	}
	s.metrics.settlementGas(pred.gasUnits, receipt.GasUsed)
	s.log.Info("settlement gas", "auction", pred.auctionID, "txHash", txHash,
		"predictedGas", pred.gasUnits, "actualGas", receipt.GasUsed, "predictedRoute", pred.gasRoutes)
	logCallbackEvents(s.log, s.cfg.Callback, receipt)
}

// buildBid evaluates an auction end-to-end: pick candidates from the configured source, size
// liquidatable legs with the shared Morpho math, select the best after-cost bundle, run the O(1) pre-bid checks
// against cached state, and sign the EXECUTOR_V6 bid. It performs no I/O (reads only the in-memory
// snapshot/state caches and the local signer), so it is deterministic and unit-testable; nowFn is
// injected for breaker/accrual timing. A non-empty skip reason means no bid was built.
func (s *Solver) buildBid(a AuctionMessage, nowFn func() time.Time) bidDecision {
	now := nowFn()
	if tripped, _ := s.breaker.tripped(now); tripped {
		return bidDecision{skip: "breaker"}
	}
	// Snapshot in-flight bids ONCE: the committed (market,borrower) set filters the scored legs below, and
	// the reserved native debits the callback funding gate further down — all under a single resMu acquisition.
	inFlight := s.inFlightSnapshot()
	// Size legs off the monitor's snapshot. A stale cache contributes nothing (fail closed): one bid covers
	// the whole selected bundle.
	var scored []scoredLeg
	staleSkip := ""
	if ok, skip := s.fresh(a); !ok {
		staleSkip = skip
	} else {
		scored = s.scoredLegs(a, now)
	}
	scored, hadLegs := dropInFlightLegs(scored, inFlight.positions)
	if len(scored) == 0 {
		switch {
		case staleSkip != "":
			return bidDecision{skip: staleSkip} // the snapshot was stale (fail closed)
		case hadLegs:
			return bidDecision{skip: "in_flight"} // all candidates already committed by unresolved bids
		default:
			return bidDecision{skip: skipNoLegs}
		}
	}
	// Pre-bid checks (all O(1), from cached state).
	st, ok := s.state.load()
	if !ok {
		return bidDecision{skip: "state_unknown"}
	}
	rate := validRate(st.Rate)
	if rate == nil {
		s.log.Info("bid skipped: loan/ETH rate unavailable",
			"auction", a.ID, "scoredLegs", len(scored), "feedCount", len(a.Payload.Prices))
		return bidDecision{skip: skipGasUnprofitable}
	}
	gasPrice := new(big.Int).Set(s.cfg.MaxTxGasPrice)
	feedCount := len(a.Payload.Prices)
	b, skip := s.selectNetBundle(scored, rate, st.Gas, gasPrice, st.GasLimit, feedCount)
	if skip != "" {
		if skip == skipGasUnprofitable && len(b.legs) > 0 {
			s.logBundleEconomics(a.ID, "bid skipped: bundle is not profitable after gas and bid",
				b, rate, st.Gas, gasPrice, st.GasLimit, feedCount, len(scored))
		}
		return bidDecision{skip: skip, gross: b.grossLoan}
	}
	priced := s.priceBundle(b, rate, st.Gas, gasPrice, feedCount)

	if st.Exec.Locked {
		return bidDecision{skip: "signer_locked"}
	}
	if fundingSkip := s.fundingSkip(a, st, priced, inFlight, gasPrice); fundingSkip != "" {
		return bidDecision{skip: fundingSkip}
	}
	// Encode operationData only after the cheap gates pass — it's the bundle's ABI pack, needed solely as
	// SignBid's input, so defer it past state.load / signer_locked / deposit_low / callback_balance.
	auth := operationAuth{AuctionKey: auctionKeyHash(a), BidAmount: priced.bidNative, MinBundleProfit: priced.minBundleProfitLoan}
	authDigest, err := CallbackAuthDigest(s.chainID, s.cfg.Callback, s.cfg.Executor, auth, priced.callbackLegs)
	if err != nil {
		s.log.Error(err, "encode callback auth digest failed", "auction", a.ID)
		return bidDecision{skip: "encode_error"}
	}
	authSig, err := s.deps.Signer.SignHash(authDigest)
	if err != nil {
		s.log.Error(err, "sign callback auth failed", "auction", a.ID)
		return bidDecision{skip: "sign_error"}
	}
	opData, err := EncodeOperationData(auth, priced.callbackLegs, authSig)
	if err != nil {
		s.log.Error(err, "encode operationData failed", "auction", a.ID)
		return bidDecision{skip: "encode_error"}
	}
	nonce := s.nonces.next(st.Exec.Nonce.Uint64())
	sig, err := SignBid(s.deps.Signer, s.chainID, s.cfg.Callback, opData, priced.bidNative, big.NewInt(int64(nonce)), gasPrice)
	if err != nil {
		s.log.Error(err, "sign bid failed", "auction", a.ID)
		return bidDecision{skip: "sign_error"}
	}

	positions := make([]positionKey, len(b.legs))
	for i, leg := range b.legs {
		positions[i] = positionKey{market: leg.MarketId, borrower: leg.Borrower}
	}
	return bidDecision{
		legs:      len(b.legs),
		gross:     b.grossLoan,
		bidNative: priced.bidNative,
		gasNative: priced.gasNative,
		nonce:     nonce,
		positions: positions,
		gas:       priced.gas,
		solve: SolveMessage{
			Op: "solve", ID: a.ID,
			Data: SolveData{
				Bid:               weiToEthString(priced.bidNative),
				Nonce:             new(big.Int).SetUint64(nonce).String(),
				OperationCallback: s.cfg.Callback.Hex(),
				OperationData:     hexutil.Encode(opData),
				LiquidationSig:    hexutil.Encode(sig),
				MaxTxGasPrice:     gasPrice.String(),
				Borrowers:         b.borrowers,
			},
		},
	}
}

func (s *Solver) fundingSkip(a AuctionMessage, st cachedState, priced pricedBundle, inFlight inFlightState, gasPrice *big.Int) string {
	requiredDeposit := new(big.Int).Add(minDeposit, priced.gasNative)
	availableDeposit := new(big.Int).Sub(orZero(st.Exec.Deposit), inFlight.gasNative)
	if availableDeposit.Cmp(requiredDeposit) < 0 {
		s.log.Info("bid skipped: executor deposit cannot cover predicted gas",
			"auction", a.ID, "depositWei", st.Exec.Deposit, "reservedGasWei", inFlight.gasNative,
			"availableWei", availableDeposit, "requiredWei", requiredDeposit,
			"minDepositWei", minDeposit, "predictedGas", priced.gas.Units, "gasPriceWei", gasPrice)
		return skipDepositLow
	}

	availableCallback := new(big.Int).Sub(orZero(st.CallbackNative), inFlight.bidNative)
	if availableCallback.Cmp(priced.bidNative) < 0 {
		s.log.Info("bid skipped: callback balance cannot cover bid",
			"auction", a.ID, "callbackWei", st.CallbackNative, "reservedBidWei", inFlight.bidNative,
			"availableWei", availableCallback, "requiredWei", priced.bidNative)
		return skipCallbackBalance
	}
	return ""
}

func dropInFlightLegs(scored []scoredLeg, inFlight map[positionKey]bool) ([]scoredLeg, bool) {
	hadLegs := len(scored) > 0
	if len(inFlight) == 0 {
		return scored, hadLegs
	}
	kept := scored[:0]
	for _, sl := range scored {
		if !inFlight[positionKey{sl.leg.MarketId, sl.leg.Borrower}] {
			kept = append(kept, sl)
		}
	}
	return kept, hadLegs
}

func (s *Solver) logBundleEconomics(auctionID, msg string, b chosenBundle, rate *big.Int, gasState *gasPredictorState, gasPrice *big.Int, gasLimit uint64, feedCount, scoredLegs int) {
	gas := gasPredictionForBundleFeeds(b, gasState, feedCount)
	grossNative := loanToNative(b.grossLoan, rate)
	gasNative := gasCostNative(gas.Units, gasPrice)
	netNative := s.bundleNetNativeForFeeds(b, rate, gasState, gasPrice, feedCount)
	bidNative := s.bundleBidNative(b, rate)
	s.log.Info(msg,
		"auction", auctionID,
		"scoredLegs", scoredLegs,
		"selectedLegs", len(b.legs),
		"feedCount", feedCount,
		"grossLoan", b.grossLoan,
		"grossNative", grossNative,
		"gasUnits", gas.Units,
		"gasNative", gasNative,
		"gasPriceWei", gasPrice,
		"bidNative", bidNative,
		"minBundleProfitNative", s.minBundleProfitNative(bidNative),
		"netNative", netNative,
		"gasLimit", gasLimit,
		"usableGasLimit", usableBundleGasLimit(gasLimit),
		"routes", gasRoutesString(gas.Routes))
}

// tooLate reports whether the auction's deadline has already passed: the window is timeoutMs measured from
// the auctioneer EMIT time (emitMs, absolute epoch-ms — the same field clampTsAt trusts). Using emit (not
// frame-receipt) charges WS transit against the budget, so a stale frame doesn't pass the gate and send a
// doomed bid that needlessly reserves headroom. Clock-skew guard, exactly like clampTsAt: if emitMs is 0 or
// in the FUTURE (emit > now — a bogus/forward timestamp), don't trust it — fall back to the local
// frame-receipt measure (now − start). caller gates this on timeoutMs > 0.
func tooLate(emitMs int64, timeoutMs int, start, now time.Time) bool {
	window := time.Duration(timeoutMs) * time.Millisecond
	if emitMs <= 0 || emitMs > now.UnixMilli() { // no/forward emit timestamp → trust the local clock
		return now.Sub(start) > window
	}
	return now.UnixMilli()-emitMs > int64(timeoutMs)
}

// sinceEmitMs is the elapsed ms since the auctioneer emitted the frame (≤0 when emit is unset/forward),
// for the too_late log line.
func sinceEmitMs(emitMs int64, now time.Time) int64 {
	if emitMs <= 0 {
		return 0
	}
	return now.UnixMilli() - emitMs
}

// clampTsAt derives the accrual timestamp from the auction's (attacker-influenceable) timestamp,
// clamped to a sane window around the given clock so a bogus value can't skew interest accrual. A
// future timestamp is never trusted (accruing past `now` over-states debt and could flag a healthy
// position) — it clamps to `now`, which under-accrues vs the later settlement block (fail closed).
func clampTsAt(auctionMs int64, now time.Time) uint64 {
	nowSec := now.Unix()
	if auctionMs <= 0 {
		return uint64(nowSec)
	}
	ts := auctionMs / 1000
	const skew = 600 // tolerate up to 10 min of staleness in the past
	if ts < nowSec-skew || ts > nowSec {
		return uint64(nowSec)
	}
	return uint64(ts)
}

// weiToEthString formats wei as a decimal ether string exactly (the solve `bid` field, which must
// equal formatEther(bidWei) — §6.1): integer/fraction split, 18-digit fraction, trailing zeros
// trimmed.
func weiToEthString(wei *big.Int) string {
	q, r := new(big.Int).DivMod(wei, morpho.Wad, new(big.Int)) // wad = 1e18
	if r.Sign() == 0 {
		return q.String()
	}
	frac := r.String()
	for len(frac) < 18 {
		frac = "0" + frac
	}
	frac = strings.TrimRight(frac, "0")
	return q.String() + "." + frac
}

// weiFloat converts wei to a float64 for gauge reporting only (lossy at >2^53; fine for dashboards).
func weiFloat(n *big.Int) float64 {
	f, _ := new(big.Float).SetInt(n).Float64()
	return f
}

// cachedState is the atomically-swapped snapshot of the on-chain state needed for pre-bid checks:
// the signer's Executor accounting plus the callback contract's native balance (must cover the bid
// or payBid underpays). Written by the ops loop, read lock-free on the hot path.
type cachedState struct {
	Exec           ExecutorState
	CallbackNative *big.Int
	Rate           *big.Int
	Gas            *gasPredictorState
	GasLimit       uint64
}

type stateCache struct {
	p atomic.Pointer[cachedState]
}

func (s *stateCache) store(v cachedState) { s.p.Store(&v) }

func (s *stateCache) load() (cachedState, bool) {
	v := s.p.Load()
	if v == nil {
		return cachedState{}, false
	}
	return *v, true
}
