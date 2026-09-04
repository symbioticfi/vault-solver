package rfq

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	liquidplanning "github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// txSender sends a transaction and blocks until its receipt (the shared txmanager). A revert is
// reported as Result.Err, so callers only check Err.
type txSender interface {
	Send(ctx context.Context, req txmanager.Request) txmanager.Result
}

// orderBackend is the backend order surface execution needs (satisfied by *backendClient).
type orderBackend interface {
	listOpenOrders(ctx context.Context, filler string, limit int) ([]backendOrder, error)
	getExecutableOrder(ctx context.Context, orderID, filler string) (*backendOrder, error)
	getOrder(ctx context.Context, orderID string) (*backendOrder, error)
	Resolve(ctx context.Context, discountID string) (*discounts.Resolved, error)
	ListDiscounts(ctx context.Context) (*discounts.List, error)
}

// executionService polls the backend for open orders and fills them via the Executor. Its single
// goroutine runs poll cycles and per-order work serially, including blocking transaction submission.
type executionService struct {
	chainID          int64
	executor         common.Address
	orderLimit       int
	vaults           []recoveryVault
	whitelist        adapterWhitelist // nil disables adapter filtering
	tokenPolicy      tokenpolicy.Policy
	discountsEnabled bool // false (external solver) skips the backend discounts API entirely
	backend          orderBackend
	store            *store
	reader           fillReader
	planner          Planner
	txm              txSender
	capacity         *capacity.Book
	metrics          *rfqMetrics
	log              logr.Logger
	now              func() time.Time
}

// fillReader is the on-chain surface used to assemble fill-time planner inputs.
type fillReader interface {
	quoteCandidateReader
	latestBlockTime(ctx context.Context) (time.Time, error)
	readPermissionedVaultInventories(
		ctx context.Context, executor, tokenIn common.Address, vaults []recoveryVault,
	) ([]liquidlane.Inventory, error)
	// resolveVaults returns the config entries with Vault/Asset resolved from the adapter at startup
	// (config carries only adapter addresses).
	resolveVaults(ctx context.Context, vaults []recoveryVault) ([]recoveryVault, error)
	setQuoteAdapters(resolved []recoveryVault)
	validateDirectAuthorization(ctx context.Context, executor common.Address, vaults []recoveryVault) error
}

func (e *executionService) run(ctx context.Context, interval time.Duration) {
	e.syncOnce(ctx)
	t := time.NewTicker(interval)
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

// syncOnce polls open orders, then advances every active order's state machine.
func (e *executionService) syncOnce(ctx context.Context) {
	timer := observability.StartOperation(e.metrics.operation())
	if err := e.pollOpenOrders(ctx); err != nil {
		e.log.Error(err, "poll open orders")
		timer.Finish(ctx, observability.ExternalOperationError)
	} else {
		timer.Finish(ctx, observability.ExternalOperationSuccess)
		e.metrics.pollSucceeded()
	}
	for _, o := range e.store.activeOrders() {
		e.handleOrder(ctx, o)
	}
	e.store.sweep() // evict stale terminal orders so the maps stay bounded
	e.metrics.orders(e.store.activeOrders(), e.now())
}

func (e *executionService) pollOpenOrders(ctx context.Context) error {
	orders, err := e.backend.listOpenOrders(ctx, lowerAddr(e.executor), e.orderLimit)
	if err != nil {
		return err
	}
	for i := range orders {
		o := &orders[i]
		if e.store.upsertQueued(o.OrderID) {
			e.metrics.won()
		}
	}
	if len(orders) > 0 {
		e.log.V(1).Info("polled open orders", "count", len(orders))
	}
	return nil
}

func (e *executionService) handleOrder(ctx context.Context, o *orderRecord) {
	if o.Status == statusQueued {
		e.submitOrder(ctx, o.OrderID)
		return
	}
	if o.Status == statusSubmitted {
		e.reconcileTerminalStatus(ctx, o.OrderID)
	}
}

func (e *executionService) submitOrder(ctx context.Context, orderID string) {
	backendOrder, err := e.backend.getExecutableOrder(ctx, orderID, lowerAddr(e.executor))
	if err != nil {
		e.log.Error(err, "resolve executable order", "orderId", orderID)
		return
	}
	if backendOrder == nil {
		e.reconcileTerminalStatus(ctx, orderID)
		return
	}
	payload, err := executableFromBackend(backendOrder)
	if err != nil {
		e.log.Error(err, "resolve executable order", "orderId", orderID)
		return
	}
	exec, err := prepareExecutable(payload, e.executor)
	if err != nil {
		e.fail(orderID, "validate order: "+err.Error())
		return
	}
	chainObservedAt := e.now()
	chainTime, err := e.reader.latestBlockTime(ctx)
	if err != nil {
		e.log.Error(err, "read chain time", "orderId", orderID)
		return
	}
	if !exec.deadline.After(chainTime) {
		e.fail(orderID, "order deadline has passed")
		return
	}

	selected, plannedAgainst, err := e.buildFillPlan(
		ctx, exec.quoteID, exec.order, exec.outputToken, exec.required,
	)
	if err != nil || selected == nil {
		e.fail(orderID, "planner fill plan: "+errString(err))
		return
	}

	swaps := directSwaps(selected, exec.order.Request.TokenIn, e.executor)
	discountSwaps, discountValidUntil, err := e.buildDiscountSwapInputs(
		ctx, selected, exec.order.Request.TokenIn, chainTime,
	)
	if err != nil {
		// The backend swapping the adapter under a quoted leg must never be filled as-is: fail the
		// order instead of submitting. While the backend still lists the order open, the next poll
		// re-arms it and re-resolves the discount, so a transient mis-resolution self-heals without
		// ever sending a tx through the wrong adapter (mirrors the TS filler's lifecycle).
		if errors.Is(err, errDiscountAdapterMismatch) || errors.Is(err, errDiscountsDisabled) {
			e.fail(orderID, err.Error())
			return
		}
		// A discount resolve is a live backend call; treat its failure as transient (leave the order
		// queued and retry next cycle) rather than terminal. Once the order is no longer open
		// the executable lookup returns nil and reconciliation marks it expired/filled.
		e.log.Error(err, "resolve discounts (will retry)", "orderId", orderID)
		return
	}
	calldata, err := encodeFill(exec.order, exec.signature, swaps, discountSwaps, emptyExecutorData)
	if err != nil {
		e.fail(orderID, "encode fill: "+err.Error())
		return
	}
	deadline := rfqFillDeadline(exec.deadline, discountValidUntil)
	cancelAt, ok := liquidlane.CancellationDeadline(deadline, chainTime, chainObservedAt, e.now())
	if !ok {
		e.fail(orderID, "fill execution deadline elapsed before submission")
		return
	}
	requested, valid := liquidplanning.FillRouteReservations(selected.Routes)
	if !valid {
		e.fail(orderID, "fill plan has invalid capacity reservations")
		return
	}

	// Funds-moving side effects start only after the complete plan has been validated.
	lease, err := e.capacity.Acquire(
		capacity.NewOwner(Name, orderID),
		requested,
		capacity.Limits(plannedAgainst, requested),
	)
	if err != nil {
		e.log.V(1).Info("fill admission raced with another solver; will replan", "orderId", orderID, "error", err.Error())
		return
	}
	defer lease.Release()

	res := e.txm.Send(ctx, txmanager.Request{
		To: e.executor, Data: calldata, CancelAt: cancelAt, Label: "rfq-fill",
	})
	attempt := e.store.recordAttempt(orderID)
	if res.Err != nil {
		e.metrics.fillFailed(res.NotAdmitted)
		e.log.Error(res.Err, "fill failed", "orderId", orderID, "attempt", attempt, "tx", res.Hash.Hex())
		e.fail(orderID, res.Err.Error())
		return
	}
	if e.metrics != nil {
		planned := new(big.Int)
		for _, route := range selected.Routes {
			planned.Add(planned, route.ExpectedAmountOut)
		}
		e.metrics.fill.Observe(res.Receipt, exec.order.Request.TokenIn, exec.order.Request.AmountIn,
			exec.outputToken, exec.required, liquidlane.PlannedSurplus(planned, exec.required))
	}
	e.log.Info("filled order", "orderId", orderID, "quoteId", exec.quoteID, "tx", res.Hash.Hex())
	e.store.markStatus(orderID, statusSubmitted, res.Hash, "")
	e.reconcileTerminalStatus(ctx, orderID)
}

func (e *executionService) reconcileTerminalStatus(ctx context.Context, orderID string) {
	bo, err := e.backend.getOrder(ctx, orderID)
	if err != nil {
		e.log.Error(err, "reconcile: get order", "orderId", orderID)
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
	case "open":
		// still open; leave as-is for the next cycle
	default:
		e.store.markStatus(orderID, statusFailed, txHash, "backend terminal status "+bo.OrderStatus)
	}
}

// buildFillPlan gives the trusted planner the awarded order terms plus current solver inputs. The
// planner owns route economics; the solver assembles the fresh snapshot and enforces solver-owned
// structural constraints on the returned plan.
func (e *executionService) buildFillPlan(
	ctx context.Context,
	requestID string,
	order executor.IReactorOrder,
	outputToken common.Address,
	required *big.Int,
) (*liquidlane.Plan, liquidlane.CapacityReservations, error) {
	// Direct inventories are filtered to adapters this executor is authorized to fill through. Skipped
	// when no candidate vaults are configured (a discount-only solver), leaving discount legs only.
	inv := make([]liquidlane.Inventory, 0, len(e.vaults)+1)
	if len(e.vaults) > 0 {
		direct, derr := e.reader.readPermissionedVaultInventories(ctx, e.executor, order.Request.TokenIn, e.vaults)
		if derr != nil {
			return nil, nil, derr
		}
		inv = append(inv, direct...)
	}
	// Discount inventories use the internal-only discounts API; external solvers skip it (adapters alone).
	if e.discountsEnabled {
		inv = append(inv, e.discountInventories(ctx, order.Request.TokenIn, inv)...)
	}
	req := quoteRequestFacts{
		RequestID: requestID, QuoteID: requestID,
		TokenIn: order.Request.TokenIn, TokenOut: outputToken, Amount: order.Request.AmountIn,
	}
	requireSingleRoute := e.tokenPolicy.RequiresSingleRoute(req.TokenIn)
	var candidates []liquidlane.QuoteCandidate
	reservations := e.capacity.Snapshot()
	if len(inv) > 0 {
		var err error
		candidates, err = e.reader.readQuoteCandidates(
			ctx, inv, req.TokenIn, req.TokenOut, req.Amount, reservations,
		)
		if err != nil {
			return nil, nil, errors.Errorf("fill: read LiquidLane candidates: %w", err)
		}
	}
	input := newQuoteInput(e.chainID, e.executor, req, candidates, required, requireSingleRoute, e.now())
	plan, err := e.planner.BuildFillPlan(ctx, input)
	if err != nil || plan == nil {
		return plan, reservations, err
	}
	if err := validateSingleRoute(input.RequireSingleRoute, len(plan.Routes)); err != nil {
		return nil, reservations, errors.Errorf("fill: planner: %w", err)
	}
	return plan, reservations, nil
}

// buildDiscountSwapInputs resolves each discount leg's fresh signed discount from the backend and
// encodes it into the Executor's DiscountSwapInput. Direct-only strategies return nil.
func (e *executionService) buildDiscountSwapInputs(
	ctx context.Context,
	selected *liquidlane.Plan,
	tokenIn common.Address,
	chainTime time.Time,
) ([]executor.IReactorDiscountSwapInput, time.Time, error) {
	var out []executor.IReactorDiscountSwapInput
	var validUntil time.Time
	for _, leg := range selected.Routes {
		if leg.DiscountID == nil {
			continue
		}
		// Defensive: external solvers never produce discount legs; fail closed (see errDiscountsDisabled).
		if !e.discountsEnabled {
			return nil, time.Time{}, errors.Errorf("%w: leg %s", errDiscountsDisabled, leg.DiscountID.Hex())
		}
		resolved, err := e.backend.Resolve(ctx, leg.DiscountID.Hex())
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
			Adapter:    leg.Adapter, TokenIn: tokenIn,
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
	ctx context.Context, tokenIn common.Address, direct []liquidlane.Inventory,
) []liquidlane.Inventory {
	resp, err := e.backend.ListDiscounts(ctx)
	if err != nil {
		e.log.Error(err, "fill: list discounts")
		return nil
	}
	seen := make(map[common.Address]bool, len(direct))
	for _, d := range direct {
		seen[d.Adapter] = true
	}
	now := e.now()
	var out []liquidlane.Inventory
	offers, issues := discounts.LiveOffers(resp, now)
	for _, issue := range issues {
		e.log.V(1).Info(
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
// planner leg it was quoted for. The leg's adapter was whitelist-filtered at selection time, so a
// mismatch means the backend swapped the adapter under us — never fill through it.
var errDiscountAdapterMismatch = errors.New("resolved discount adapter does not match the planner leg adapter")

// errDiscountsDisabled marks a discount leg seen while discounts are off (external solver). Terminal —
// fail the order, no tx. Defensive: the external profile never advertises discount candidates.
var errDiscountsDisabled = errors.New("discount leg present but discounts are disabled")

// toDiscountSwapInput converts a resolved signed discount + its planner leg into the Executor input.
func toDiscountSwapInput(
	parsed *discounts.Signed, leg liquidlane.PlanLeg, recipient common.Address,
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

func (e *executionService) fail(orderID, msg string) {
	e.log.Info("order failed", "orderId", orderID, "reason", msg)
	e.store.markStatus(orderID, statusFailed, common.Hash{}, msg)
}

func errString(err error) string {
	if err == nil {
		return "not available"
	}
	return err.Error()
}

// isHash32 reports whether s is a 0x-prefixed, well-formed 32-byte hash.
func isHash32(s string) bool {
	b, err := hexutil.Decode(s)
	return err == nil && len(b) == 32
}
