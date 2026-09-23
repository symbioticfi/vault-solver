package rfq

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// txSender sends a transaction and blocks until the shared txmanager reports its typed outcome.
type txSender interface {
	Send(ctx context.Context, req txmanager.Request) txmanager.Result
}

// orderBackend is the backend order surface execution needs (satisfied by *backendClient).
type orderBackend interface {
	listOpenOrders(ctx context.Context, filler string, limit int) ([]backendOrder, error)
	getExecutableOrder(ctx context.Context, orderID, filler string) (*backendOrder, error)
	getOrder(ctx context.Context, orderID string) (*backendOrder, error)
	resolveDiscount(ctx context.Context, discountID string) (*resolveDiscountResponse, error)
	listDiscounts(ctx context.Context) (*discountsResponse, error)
}

// executable is the resolved, typed payload needed to build a fill (from the backend executable view).
type executable struct {
	quoteID      string
	encodedOrder []byte
	signature    []byte
	deadline     int64
	filler       common.Address
	outputs      []backendOut
}

// executionService polls the backend for open orders and fills them via the Executor. The poll loop
// reserves each won order's liquidity and reconciles submitted ones; a single submitter sends fills
// in award order, because the shared nonce lane admits one at a time. Per-order work is guarded by an
// in-flight set so the two goroutines never handle the same order at once.
type executionService struct {
	chainID                int64
	executor               common.Address
	orderLimit             int
	maxCancellationRetries int
	pollInterval           time.Duration
	vaults                 []recoveryVault
	whitelist              adapterWhitelist // nil disables adapter filtering
	tokenPolicy            tokenpolicy.Policy
	discountsEnabled       bool // false (external solver) skips the backend discounts API entirely
	backend                orderBackend
	store                  *store
	reader                 fillReader
	strategy               types.Strategy
	strategyName           string // registry key, reported as the strategy.name span attribute
	txm                    txSender
	metrics                *rfqMetrics
	orderPollObserver      *observability.OperationObserver
	// links is shared with the server: it holds the span context of each quote this process served,
	// so a fill can link back to it (spec §12). nil disables linking.
	links *observability.SpanLinks
	log   logr.Logger
	now   func() time.Time

	inflightMu sync.Mutex
	inflight   map[string]bool
	// planningMu serializes the reservation snapshot, fresh reads and plan so two won orders
	// cannot both claim the same free capacity. Quotes read the ledger without it.
	planningMu sync.Mutex
	submitWake chan struct{}
	// sending is set while the submitter handles orders. The poll loop reserves won orders itself
	// only then; an idle submitter reserves them through its own plan on the wake that follows.
	sending atomic.Bool
}

// fillReader is the on-chain surface used to assemble fill-time strategy inputs.
type fillReader interface {
	quoteCandidateReader
	latestBlock(ctx context.Context) (uint64, time.Time, error)
	readPermissionedVaultInventories(
		ctx context.Context, executor, tokenIn common.Address, vaults []recoveryVault,
	) ([]solverInventory, error)
	// resolveVaults returns the config entries with Vault/Asset resolved from the adapter at startup
	// (config carries only adapter addresses).
	resolveVaults(ctx context.Context, vaults []recoveryVault) ([]recoveryVault, error)
	setQuoteAdapters(resolved []recoveryVault)
	validateDirectAuthorization(ctx context.Context, executor common.Address, vaults []recoveryVault) error
}

func (e *executionService) run(ctx context.Context) {
	submitterDone := make(chan struct{})
	go func() {
		defer close(submitterDone)
		e.submitLoop(ctx)
	}()
	defer func() { <-submitterDone }()
	e.syncOnce(ctx)
	t := time.NewTicker(e.pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.syncOnce(ctx)
		}
	}
}

// syncOnce polls open orders, reserves newly won ones and reconciles submitted ones, then wakes the
// submitter. It roots the fill trace for this cycle: the quote that produced an order lives in the
// backend's trace, not here. It never waits on a transaction.
func (e *executionService) syncOnce(ctx context.Context) {
	ctx, end := tracer.Start(ctx, "rfq.execution.sync")
	var err error
	defer func() { end(err) }()

	err = e.pollOpenOrders(ctx)
	if err != nil {
		backendErrorLogger(observability.Log(ctx), err).Error(err, "poll open orders")
	}
	for _, o := range e.store.activeOrders() {
		switch {
		case !o.Status.awaitsSubmission():
			e.handleOrder(ctx, o)
		case e.expireQueued(o):
		case e.sending.Load():
			e.reserveWon(ctx, o)
		}
	}
	e.store.sweep() // evict stale terminal orders so the maps stay bounded
	e.wakeSubmitter()
}

// submitLoop sends won orders oldest first, one pass per wake. A pass may block for a whole
// transaction lifecycle; polling and reservation continue meanwhile.
func (e *executionService) submitLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.submitWake:
		}
		e.sending.Store(true)
		for _, o := range e.store.ordersAwaitingSubmission() {
			if ctx.Err() != nil {
				e.sending.Store(false)
				return
			}
			e.handleOrder(ctx, o)
		}
		e.sending.Store(false)
	}
}

func (e *executionService) wakeSubmitter() {
	select {
	case e.submitWake <- struct{}{}:
	default:
	}
}

// expireUnsigned expires an order whose recorded deadline has passed while no transaction was signed
// for it. A confirmed cancellation leaves no unresolved fill, so retry waiting and subsequent unsigned
// preparation are bounded when backend views disappear; submitted/unknown inclusion keeps tracking.
// The caller owns the order through the in-flight set, so a fill in Send is never expired.
func (e *executionService) expireUnsigned(o *orderRecord) bool {
	unsignedRetry := o.Status == statusRetryWaiting || o.Status == statusQueued || o.Status == statusSubmitting
	if !unsignedRetry || o.RetryDeadline.IsZero() || e.now().Before(o.RetryDeadline) {
		return false
	}
	e.store.markStatus(o.OrderID, statusExpired, common.Hash{}, "order deadline has passed")
	return true
}

// expireQueued applies the deadline bound from the poll loop, so a queued order does not keep its
// reservation past its deadline while the submitter is busy with another fill. An order the
// submitter owns is left to it.
func (e *executionService) expireQueued(o *orderRecord) bool {
	if !e.acquire(o.OrderID) {
		return false
	}
	defer e.release(o.OrderID)
	if o = e.store.order(o.OrderID); o == nil || !o.Status.awaitsSubmission() {
		return false
	}
	return e.expireUnsigned(o)
}

// reserveWon holds a won order's liquidity as soon as it is polled, before its turn on the
// transaction lane. It records no outcome: submission plans again and owns every failure.
func (e *executionService) reserveWon(ctx context.Context, o *orderRecord) {
	if e.store.reserved(o.OrderID) || e.isInflight(o.OrderID) {
		return
	}
	exec, err := e.resolveExecutable(ctx, o)
	if err != nil || exec == nil {
		return
	}
	order, err := decodeOrder(exec.encodedOrder)
	if err != nil {
		return
	}
	outputToken, required, err := executableOrderTerms(exec, order, e.executor)
	if err != nil {
		return
	}
	chainObservedAt := e.now()
	// A failed header read still reserves: block zero subtracts every other order, and submission
	// records the deadline bound later.
	chainBlock, chainTime, err := e.reader.latestBlock(ctx)
	if err == nil {
		e.boundByOrderDeadline(o.OrderID, order, chainTime, chainObservedAt)
	}
	if _, err := e.buildFillPlan(ctx, o.OrderID, exec, order, outputToken, required, chainBlock); err != nil {
		observability.Log(ctx).V(1).Info("won order not reserved yet; submission plans again",
			"orderId", o.OrderID, "quoteId", o.QuoteID, "err", err.Error())
	}
}

func (e *executionService) pollOpenOrders(ctx context.Context) (err error) {
	ctx, end := tracer.Start(ctx, "rfq.execution.poll")
	defer func() { end(err) }()
	timer := observability.StartOperation(e.orderPollObserver)
	defer func() {
		outcome := observability.ExternalOperationSuccess
		if err != nil {
			outcome = observability.ExternalOperationError
		}
		timer.Finish(ctx, outcome)
	}()
	orders, err := e.backend.listOpenOrders(ctx, lowerAddr(e.executor), e.orderLimit)
	if err != nil {
		return err
	}
	for i := range orders {
		o := &orders[i]
		if e.store.upsertQueued(queuedOrder{OrderID: o.OrderID, QuoteID: o.QuoteID}) {
			e.metrics.observeWin()
		}
	}
	if len(orders) > 0 {
		observability.Log(ctx).V(1).Info("polled open orders", "count", len(orders))
	}
	if e.metrics != nil {
		e.metrics.observeOrderPoll(e.now())
	}
	return nil
}

func (e *executionService) handleOrder(ctx context.Context, o *orderRecord) {
	if !e.acquire(o.OrderID) {
		return
	}
	defer e.release(o.OrderID)
	// The caller's snapshot can predate a transaction the other goroutine just finished.
	if o = e.store.order(o.OrderID); o == nil {
		return
	}

	// The narrowed base logger, never a trace-stamped one: Log stamps each stage's own span.
	ctx = observability.WithLogger(ctx, e.log.WithValues("orderId", o.OrderID, "quoteId", o.QuoteID))
	ctx, end, _ := tracer.StartLinkedKey(ctx, e.links, o.QuoteID, "rfq.order",
		observability.AttrOrderID.String(o.OrderID),
		observability.AttrQuoteID.String(o.QuoteID),
	)
	defer end(nil) // each stage records its own failure; terminal skips are declined events here

	if e.expireUnsigned(o) {
		return
	}
	switch o.Status {
	case statusQueued, statusSubmitting:
		e.submitOrder(ctx, o.OrderID)
	case statusSubmitted, statusRetryWaiting:
		e.reconcileTerminalStatus(ctx, o.OrderID)
	case statusFilled, statusExpired, statusFailed:
		// terminal — nothing to do
	}
}

func (e *executionService) submitOrder(ctx context.Context, orderID string) {
	e.store.markStatus(orderID, statusSubmitting, common.Hash{}, "")
	local := e.store.order(orderID)
	if local == nil {
		return
	}

	exec, err := e.resolveExecutable(ctx, local)
	if err != nil {
		backendErrorLogger(observability.Log(ctx), err).Error(err, "resolve executable order")
		return // transient; retried next cycle
	}
	if exec == nil {
		observability.Decline(ctx, "fill_skipped", "order is no longer executable")
		e.reconcileTerminalStatus(ctx, orderID)
		return
	}
	order, err := decodeOrder(exec.encodedOrder)
	if err != nil {
		e.fail(ctx, orderID, "decode order: "+err.Error())
		return
	}
	outputToken, required, err := executableOrderTerms(exec, order, e.executor)
	if err != nil {
		e.fail(ctx, orderID, "validate order: "+err.Error())
		return
	}
	chainObservedAt := e.now()
	chainBlock, chainTime, err := e.reader.latestBlock(ctx)
	if err != nil {
		observability.Log(ctx).Error(err, "read chain time")
		return
	}
	orderDeadline := time.Unix(order.Request.Deadline.Int64(), 0)
	if !orderDeadline.After(chainTime) {
		// Skip an already-expired order rather than spend gas on a fill the Reactor will revert.
		e.fail(ctx, orderID, "order deadline has passed")
		return
	}
	e.boundByOrderDeadline(orderID, order, chainTime, chainObservedAt)

	selected, err := e.buildFillPlan(ctx, orderID, exec, order, outputToken, required, chainBlock)
	if err != nil || selected == nil {
		e.fail(ctx, orderID, "strategy fill plan: "+errString(err))
		return
	}
	traceAdapter(ctx, selected.Legs)

	calldata, discountValidUntil, ok := e.buildFillCalldata(ctx, orderID, exec, order, selected, chainTime)
	if !ok {
		return
	}
	deadline := rfqFillDeadline(orderDeadline, discountValidUntil)
	cancelAt, ok := liquidlane.CancellationDeadline(deadline, chainTime, chainObservedAt, e.now())
	if !ok {
		e.fail(ctx, orderID, "fill execution deadline elapsed before submission")
		return
	}

	res, sendErr := e.sendFill(ctx, txmanager.Request{
		Solver: Name,
		To:     e.executor, Data: calldata, CancelAt: cancelAt, Label: "rfq-fill",
	})
	attempt := e.store.recordAttempt(orderID)
	outcome := res.Outcome
	if !outcome.Included() {
		if e.metrics != nil {
			fillOutcome := liquidlane.FillOutcomeFailure
			if res.NotAdmitted {
				fillOutcome = liquidlane.FillOutcomeNotAdmitted
			}
			e.metrics.fillAmounts.ObserveOutcome(fillOutcome)
		}
		status := statusFailed
		if outcome == txmanager.OutcomeTrackingStopped || outcome == txmanager.OutcomeCancelledUnconfirmed {
			// Inclusion is unknown; reconcile the backend without sending another transaction.
			status = statusSubmitted
		}
		retryAt := e.now().Add(e.pollInterval)
		retryDeadline, stillValid := liquidlane.CancellationDeadline(orderDeadline, chainTime, chainObservedAt, retryAt)
		// Scheduled before any terminal status so a retrying order keeps its reservation.
		if ctx.Err() == nil && stillValid && outcome == txmanager.OutcomeCancelled &&
			res.Receipt != nil && res.Receipt.Status == ethtypes.ReceiptStatusSuccessful &&
			e.store.scheduleCancellationRetry(orderID, e.maxCancellationRetries, retryAt, retryDeadline, res.Hash, sendErr.Error()) {
			observability.Log(ctx).Info("fill cancelled; retry scheduled", "attempt", attempt,
				"tx", res.Hash.Hex(), "retryAt", retryAt)
			return
		}
		e.store.markStatus(orderID, status, res.Hash, sendErr.Error())
		observability.Log(ctx).Error(sendErr, "fill failed", "attempt", attempt, "tx", res.Hash.Hex(),
			"outcome", outcome)
		return
	}
	if outcome == txmanager.OutcomeConfirmed {
		observability.Log(ctx).Info("filled order", "tx", res.Hash.Hex())
		if res.Receipt != nil && res.Receipt.BlockNumber != nil {
			e.store.markIncluded(orderID, res.Receipt.BlockNumber.Uint64())
		}
	} else {
		observability.Log(ctx).Error(res.Err, "fill included but confirmation wait failed",
			"attempt", attempt, "tx", res.Hash.Hex())
	}
	if e.metrics != nil {
		e.metrics.fillAmounts.Observe(
			res.Receipt,
			order.Request.TokenIn,
			order.Request.AmountIn,
			outputToken,
			required,
			liquidlane.PlannedSurplus(selected.QuotedAmountOut, required),
		)
	}
	e.store.markStatus(orderID, statusSubmitted, res.Hash, "")
	e.reconcileTerminalStatus(ctx, orderID)
}

// boundByOrderDeadline records the order's chain deadline as the local bound on unsigned work, so a
// won order the backend stops reporting cannot hold its reservation past the point a fill could land.
func (e *executionService) boundByOrderDeadline(
	orderID string, order executor.IReactorOrder, chainTime, chainObservedAt time.Time,
) {
	orderDeadline := time.Unix(order.Request.Deadline.Int64(), 0)
	if deadline, ok := liquidlane.CancellationDeadline(orderDeadline, chainTime, chainObservedAt, e.now()); ok {
		e.store.boundUnsignedWork(orderID, deadline)
	}
}

// buildFillCalldata resolves the plan's discount legs and encodes the Executor fill. It reports
// whether submission may proceed, having already recorded the order outcome when it may not.
func (e *executionService) buildFillCalldata(
	ctx context.Context,
	orderID string,
	exec *executable,
	order executor.IReactorOrder,
	selected *fillPlan,
	chainTime time.Time,
) (calldata []byte, discountValidUntil time.Time, ok bool) {
	// buildCtx is the stage; ctx stays the order span, so a terminal skip is declined there rather
	// than on a stage span this function has already ended.
	buildCtx, end := tracer.Start(ctx, "rfq.order.build")
	var err error
	defer func() { end(err) }()

	swaps := directSwaps(selected, order.Request.TokenIn, e.executor)
	discountSwaps, discountValidUntil, err := e.buildDiscountSwapInputs(buildCtx, selected, chainTime)
	if err != nil {
		// The backend swapping the adapter under a quoted leg must never be filled as-is: fail the
		// order instead of submitting. While the backend still lists the order open, the next poll
		// re-arms it and re-resolves the discount, so a transient mis-resolution self-heals without
		// ever sending a tx through the wrong adapter (mirrors the TS filler's lifecycle).
		if errors.Is(err, errDiscountAdapterMismatch) || errors.Is(err, errDiscountsDisabled) {
			e.fail(ctx, orderID, err.Error())
			err = nil // failing closed on a backend-side swap is an expected skip, not a stage error
			return nil, time.Time{}, false
		}
		// A discount resolve is a live backend call; treat its failure as transient (leave the order
		// in submitting and retry next cycle) rather than terminal. Once the order is no longer open
		// the executable lookup returns nil and reconciliation marks it expired/filled.
		backendErrorLogger(observability.Log(ctx), err).Error(err, "resolve discounts (will retry)")
		return nil, time.Time{}, false
	}
	calldata, err = encodeFill(order, exec.signature, swaps, discountSwaps, emptyExecutorData)
	if err != nil {
		e.fail(ctx, orderID, "encode fill: "+err.Error())
		return nil, time.Time{}, false
	}
	return calldata, discountValidUntil, true
}

// sendFill submits the fill as the rfq.order.submit stage and reports the transaction on both that
// stage and the order span it belongs to. The returned error is the failure the outcome carries,
// synthesized when the manager reports a non-inclusive outcome without one.
func (e *executionService) sendFill(
	ctx context.Context, req txmanager.Request,
) (res txmanager.Result, err error) {
	submitCtx, end := tracer.Start(ctx, "rfq.order.submit")
	defer func() { end(err) }()

	res = e.txm.Send(submitCtx, req)
	txmanager.RecordResult(submitCtx, res) // the stage
	txmanager.RecordResult(ctx, res)       // the order span it belongs to
	err = res.Err                          // included-but-unconfirmed still carries the wait failure
	if err == nil && !res.Outcome.Included() {
		err = errors.Errorf("unknown transaction outcome %q", res.Outcome)
	}
	return res, err
}

// resolveExecutable returns the executable payload for a polled order from the backend.
func (e *executionService) resolveExecutable(ctx context.Context, local *orderRecord) (exec *executable, err error) {
	ctx, end := tracer.Start(ctx, "rfq.order.resolve")
	defer func() { end(err) }()

	bo, err := e.backend.getExecutableOrder(ctx, local.OrderID, lowerAddr(e.executor))
	if err != nil {
		return nil, err
	}
	if bo == nil {
		return nil, nil
	}
	return executableFromBackend(bo)
}

func (e *executionService) reconcileTerminalStatus(ctx context.Context, orderID string) {
	ctx, end := tracer.Start(ctx, "rfq.order.report")
	var err error
	defer func() { end(err) }()

	var bo *backendOrder
	bo, err = e.backend.getOrder(ctx, orderID)
	if err != nil {
		backendErrorLogger(observability.Log(ctx), err).Error(err, "reconcile: get order")
		return
	}
	if bo == nil {
		return
	}
	txHash := common.Hash{}
	// HexToHash silently zero-pads/truncates malformed input, so only accept a well-formed 32-byte
	// hash from the backend; otherwise leave it zero rather than record a garbage reference.
	if bo.TxHash != nil && isHash32(*bo.TxHash) {
		txHash = common.HexToHash(*bo.TxHash)
	}
	switch bo.OrderStatus {
	case "filled":
		e.store.markStatus(orderID, statusFilled, txHash, "")
	case "expired":
		e.store.markStatus(orderID, statusExpired, txHash, "")
	case backendOrderStatusOpen:
		// still open; leave as-is for the next cycle
	case "error", "cancelled", "unverified", "insufficient-funds":
		observability.Decline(ctx, "fill_failed", "backend terminal status "+bo.OrderStatus)
		e.store.markStatus(orderID, statusFailed, txHash, "backend terminal status "+bo.OrderStatus)
	default:
		// The client tolerates a dropped or renamed field, so "" or a new value reaches here. Marking
		// it failed would re-arm the order and re-submit a fill the backend may still consider live.
		err = errUnknownOrderStatus
		observability.Log(ctx).Error(err, "reconcile: retaining order", "status", bo.OrderStatus)
	}
}

var errUnknownOrderStatus = errors.New("unrecognized backend order status")

// buildFillPlan gives the trusted strategy the awarded order terms plus current solver inputs, then
// replaces the order's reservation with the plan. The strategy owns route economics; the solver
// assembles the fresh snapshot, subtracts every other won order and enforces solver-owned structural
// constraints on the returned plan.
func (e *executionService) buildFillPlan(
	ctx context.Context,
	orderID string,
	exec *executable,
	order executor.IReactorOrder,
	outputToken common.Address,
	required *big.Int,
	snapshotBlock uint64,
) (plan *fillPlan, err error) {
	ctx, end := tracer.Start(ctx, "rfq.order.plan", observability.AttrStrategy.String(e.strategyName))
	defer func() { end(err) }()
	e.planningMu.Lock()
	defer e.planningMu.Unlock()
	pending := func(inventory []solverInventory) liquidlane.CapacityReservations {
		return e.store.pendingReservations(orderID, inventory)
	}

	// Direct inventories are filtered to adapters this executor is authorized to fill through. Skipped
	// when no candidate vaults are configured (a discount-only solver), leaving discount legs only.
	inv := make([]solverInventory, 0, len(e.vaults)+1)
	if len(e.vaults) > 0 {
		direct, derr := e.reader.readPermissionedVaultInventories(ctx, e.executor, order.Request.TokenIn, e.vaults)
		if derr != nil {
			return nil, derr
		}
		// Read after the header, so the state reflects at least snapshotBlock.
		for index := range direct {
			direct[index].BlockNumber = snapshotBlock
		}
		inv = append(inv, direct...)
	}
	// Discount inventories use the internal-only discounts API; external solvers skip it (adapters alone).
	if e.discountsEnabled {
		inv = append(inv, e.discountInventories(ctx, order.Request.TokenIn, inv)...)
	}
	req := strategyRequest{
		RequestID: exec.quoteID, QuoteID: exec.quoteID,
		TokenIn: order.Request.TokenIn, TokenOut: outputToken, Amount: order.Request.AmountIn,
	}
	requireSingleRoute := e.tokenPolicy.RequiresSingleRoute(req.TokenIn)
	var candidates []liquidlane.QuoteCandidate
	if len(inv) > 0 {
		candidates, err = e.reader.readQuoteCandidates(ctx, inv, req.TokenIn, req.TokenOut, req.Amount, pending)
		if err != nil {
			return nil, errors.Errorf("fill: read LiquidLane candidates: %w", err)
		}
	}
	input := newFillInput(e.chainID, e.executor, req, candidates, required, requireSingleRoute, e.now())
	plan, err = e.strategy.BuildFillPlan(ctx, input)
	if err != nil || plan == nil {
		return plan, err
	}
	if verr := validateSingleRoute(input.RequireSingleRoute, len(plan.Legs)); verr != nil {
		return nil, errors.Errorf("fill: strategy: %w", verr)
	}
	reservations, err := fillPlanReservations(plan, candidates)
	if err != nil {
		return nil, err
	}
	if !e.store.reserve(orderID, reservations) {
		return nil, errors.New("fill: order is no longer active")
	}
	return plan, nil
}

// buildDiscountSwapInputs resolves each discount leg's fresh signed discount from the backend and
// encodes it into the Executor's DiscountSwapInput. Direct-only strategies return nil.
func (e *executionService) buildDiscountSwapInputs(
	ctx context.Context,
	selected *fillPlan,
	chainTime time.Time,
) ([]executor.IReactorDiscountSwapInput, time.Time, error) {
	var out []executor.IReactorDiscountSwapInput
	var validUntil time.Time
	for _, leg := range selected.Legs {
		if leg.DiscountID == nil {
			continue
		}
		// Defensive: external solvers never produce discount legs; fail closed (see errDiscountsDisabled).
		if !e.discountsEnabled {
			return nil, time.Time{}, errors.Errorf("%w: leg %s", errDiscountsDisabled, leg.DiscountID.Hex())
		}
		resolved, err := e.backend.resolveDiscount(ctx, leg.DiscountID.Hex())
		if err != nil {
			return nil, time.Time{}, errors.Errorf("resolve discount %s: %w", leg.DiscountID.Hex(), err)
		}
		parsed, err := discounts.ParseSigned(resolved)
		if err != nil {
			return nil, time.Time{}, errors.Errorf("discount: %w", err)
		}
		if parsed.Adapter != leg.Adapter {
			return nil, time.Time{}, errors.Errorf(
				"%w: resolved %s, leg %s", errDiscountAdapterMismatch, parsed.Adapter.Hex(), leg.Adapter.Hex(),
			)
		}
		if err := discounts.ValidateSelection(parsed, discounts.Selection{
			DiscountID: *leg.DiscountID,
			Adapter:    leg.Adapter, TokenIn: selected.TokenIn,
		}, chainTime); err != nil {
			return nil, time.Time{}, errors.Errorf("discount: %w", err)
		}
		dsi, err := toDiscountSwapInput(parsed, leg, e.executor)
		if err != nil {
			return nil, time.Time{}, err
		}
		out = append(out, dsi)
		deadline := discounts.ValidUntil(parsed)
		if validUntil.IsZero() || deadline.Before(validUntil) {
			validUntil = deadline
		}
	}
	return out, validUntil, nil
}

func rfqFillDeadline(orderDeadline, discountValidUntil time.Time) time.Time {
	if !discountValidUntil.IsZero() && discountValidUntil.Before(orderDeadline) {
		return discountValidUntil
	}
	return orderDeadline
}

// discountInventories fetches offered discounts and turns those redeemable
// against tokenIn into discount-leg candidates: keep discounts whose adapter is whitelisted, whose
// tokenToRedeem == tokenIn, and whose adapter is not already permissioned. Solver-side candidate
// normalization later filters collateral to the order's tokenOut.
func (e *executionService) discountInventories(
	ctx context.Context, tokenIn common.Address, direct []solverInventory,
) []solverInventory {
	resp, err := e.backend.listDiscounts(ctx)
	if err != nil {
		backendErrorLogger(observability.Log(ctx), err).Error(err, "fill: list discounts")
		return nil
	}
	seen := make(map[common.Address]bool, len(direct))
	for _, d := range direct {
		seen[d.Adapter] = true
	}
	now := e.now()
	var out []solverInventory
	offers, issues := discounts.LiveOffers(resp, now)
	for _, issue := range issues {
		observability.Log(ctx).V(1).Info(
			"recover: skip invalid discount", "discountId", issue.DiscountID, "error", issue.Err.Error(),
		)
	}
	for _, offer := range offers {
		if offer.TokenToRedeem != tokenIn {
			continue
		}
		adapter := offer.Adapter
		if !e.whitelist.allows(adapter) {
			continue
		}
		if seen[adapter] {
			continue
		}
		route := liquidlane.NewRoute(
			e.chainID, adapter, common.Address{}, tokenIn, offer.Collateral, 0, offer.CollateralDecimals,
		)
		// The discounts API does not expose the backing vault. Keep unknown adapters in independent
		// capacity domains instead of making address(0) look like one shared vault.
		route.CapacityID = liquidlane.CapacityID(route.ID)
		out = append(out, liquidlane.DiscountInventory(
			route, offer.MaxAssets, offer.MaxRate, offer.DiscountID, time.Unix(offer.Deadline, 0),
		))
	}
	return out
}

// errDiscountAdapterMismatch marks a backend-resolved discount whose adapter differs from the
// strategy leg it was quoted for. The leg's adapter was whitelist-filtered at selection time, so a
// mismatch means the backend swapped the adapter under us — never fill through it.
var errDiscountAdapterMismatch = errors.New("resolved discount adapter does not match the strategy leg adapter")

// errDiscountsDisabled marks a discount leg seen while discounts are off (external solver). Terminal —
// fail the order, no tx. Defensive: the external profile never advertises discount candidates.
var errDiscountsDisabled = errors.New("discount leg present but discounts are disabled")

// toDiscountSwapInput converts a resolved signed discount + its strategy leg into the Executor input.
func toDiscountSwapInput(
	parsed *discounts.Signed, leg fillLeg, recipient common.Address,
) (executor.IReactorDiscountSwapInput, error) {
	if parsed == nil {
		return executor.IReactorDiscountSwapInput{}, errors.New("discount: resolved discount is nil")
	}
	// Mirrors buildDiscountSwapInputs in discounts.ts: the outer adapter comes from the resolved
	// discount's adapter, the inner Discount no longer carries the vault field, and the input drops
	// amountOut.
	return executor.IReactorDiscountSwapInput{
		Adapter: parsed.Adapter,
		DiscountSwap: executor.ILiquidLaneAdapterDiscountSwap{
			Discount: executor.ILiquidLaneAdapterDiscount{
				TokenToRedeem: parsed.Terms.TokenToRedeem,
				Discount:      parsed.Terms.Discount, Signer: parsed.Terms.Signer, Protocol: parsed.Terms.Protocol,
				Nonce: parsed.Terms.Nonce, Deadline: parsed.Terms.Deadline,
			},
			SignerSignature:  parsed.SignerSignature,
			ProtocolDeadline: parsed.ProtocolDeadline,
		},
		ProtocolSignature: parsed.ProtocolSignature,
		Recipient:         recipient,
		AmountIn:          new(big.Int).Set(leg.AmountIn),
	}, nil
}

// fail records a terminal skip: expected, so it declines the span rather than erroring it, the same
// way it logs at Info rather than Error.
func (e *executionService) fail(ctx context.Context, orderID, msg string) {
	observability.Decline(ctx, "fill_failed", msg)
	observability.Log(ctx).Info("order failed", "reason", msg)
	e.store.markStatus(orderID, statusFailed, common.Hash{}, msg)
}

func errString(err error) string {
	if err == nil {
		return "not available"
	}
	return err.Error()
}

func (e *executionService) acquire(orderID string) bool {
	e.inflightMu.Lock()
	defer e.inflightMu.Unlock()
	if e.inflight[orderID] {
		return false
	}
	e.inflight[orderID] = true
	return true
}

func (e *executionService) release(orderID string) {
	e.inflightMu.Lock()
	defer e.inflightMu.Unlock()
	delete(e.inflight, orderID)
}

func (e *executionService) isInflight(orderID string) bool {
	e.inflightMu.Lock()
	defer e.inflightMu.Unlock()
	return e.inflight[orderID]
}

/* ───────── executable helpers ───────── */

func executableFromBackend(bo *backendOrder) (*executable, error) {
	if bo.EncodedOrder == nil || bo.ProtocolSignature == nil || bo.Deadline == nil || bo.Filler == nil {
		return nil, errors.New("executable order payload incomplete")
	}
	if !common.IsHexAddress(*bo.Filler) {
		return nil, errors.Errorf("invalid filler %q", *bo.Filler)
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
		deadline:     *bo.Deadline,
		filler:       common.HexToAddress(*bo.Filler),
		outputs:      bo.Outputs,
	}, nil
}

// isHash32 reports whether s is a 0x-prefixed, well-formed 32-byte hash.
func isHash32(s string) bool {
	b, err := hexutil.Decode(s)
	return err == nil && len(b) == 32
}

func executableOrderTerms(
	exec *executable,
	order executor.IReactorOrder,
	expectedFiller common.Address,
) (common.Address, *big.Int, error) {
	if order.Filler != expectedFiller {
		return common.Address{}, nil, errors.New("signed order assigns a different filler")
	}
	if exec.filler != order.Filler {
		return common.Address{}, nil, errors.New("backend filler does not match signed order")
	}
	if order.Request.TokenIn == (common.Address{}) || order.Request.AmountIn == nil || order.Request.AmountIn.Sign() <= 0 {
		return common.Address{}, nil, errors.New("signed order has invalid input")
	}
	if order.Request.Deadline == nil || !order.Request.Deadline.IsInt64() ||
		order.Request.Deadline.Sign() <= 0 {
		return common.Address{}, nil, errors.New("signed order has invalid deadline")
	}
	if exec.deadline != order.Request.Deadline.Int64() {
		return common.Address{}, nil, errors.New("backend deadline does not match signed order")
	}
	token, ok := singleOrderOutputToken(order.Outputs)
	if !ok {
		return common.Address{}, nil, errors.New("only single output-token orders are supported")
	}
	required, err := sumOrderOutputs(order.Outputs)
	if err != nil {
		return common.Address{}, nil, err
	}
	if err := matchBackendOutputs(exec.outputs, order.Outputs); err != nil {
		return common.Address{}, nil, err
	}
	return token, required, nil
}

func singleOrderOutputToken(outputs []executor.IReactorOutput) (common.Address, bool) {
	if len(outputs) == 0 {
		return common.Address{}, false
	}
	token := outputs[0].Token
	for _, o := range outputs {
		if o.Token != token {
			return common.Address{}, false
		}
	}
	if token == (common.Address{}) {
		return common.Address{}, false
	}
	return token, true
}

func sumOrderOutputs(outputs []executor.IReactorOutput) (*big.Int, error) {
	total := new(big.Int)
	for i, output := range outputs {
		if output.Amount == nil || output.Amount.Sign() <= 0 {
			return nil, errors.Errorf("signed order output %d has invalid amount", i)
		}
		if output.Recipient == (common.Address{}) {
			return nil, errors.Errorf("signed order output %d has invalid recipient", i)
		}
		total.Add(total, output.Amount)
	}
	return total, nil
}

func matchBackendOutputs(backend []backendOut, signed []executor.IReactorOutput) error {
	if len(backend) != len(signed) {
		return errors.New("backend outputs do not match signed order")
	}
	for i, output := range backend {
		if !common.IsHexAddress(output.Token) || !common.IsHexAddress(output.Recipient) {
			return errors.Errorf("backend output %d has invalid address", i)
		}
		amount, ok := new(big.Int).SetString(output.Amount, 10)
		if !ok || amount.Sign() <= 0 {
			return errors.Errorf("backend output %d has invalid amount", i)
		}
		if common.HexToAddress(output.Token) != signed[i].Token ||
			common.HexToAddress(output.Recipient) != signed[i].Recipient ||
			amount.Cmp(signed[i].Amount) != 0 {
			return errors.Errorf("backend output %d does not match signed order", i)
		}
	}
	return nil
}
