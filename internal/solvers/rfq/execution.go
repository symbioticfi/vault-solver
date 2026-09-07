package rfq

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/parse"
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
	discounts.Provider
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

// executionService has one owner: the poll goroutine. It resolves, plans and waits for
// each fill before advancing another order; HTTP quoting never mutates execution state.
type executionService struct {
	chainID           int64
	executor          common.Address
	orderLimit        int
	vaults            []recoveryVault
	whitelist         adapterWhitelist // nil disables adapter filtering
	tokenPolicy       tokenpolicy.Policy
	discountsEnabled  bool // false (external solver) skips the backend discounts API entirely
	backend           orderBackend
	store             *store
	reader            fillReader
	strategy          types.Strategy
	txm               txSender
	metrics           *rfqMetrics
	orderPollObserver *observability.OperationObserver
	log               logr.Logger
	now               func() time.Time
}

// fillReader is the on-chain surface used to assemble fill-time strategy inputs.
type fillReader interface {
	quoteCandidateReader
	latestBlockTime(ctx context.Context) (time.Time, error)
	readPermissionedVaultInventories(
		ctx context.Context, executor, tokenIn common.Address, vaults []recoveryVault,
	) ([]solverInventory, error)
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
	if err := e.pollOpenOrders(ctx); err != nil {
		e.log.Error(err, "poll open orders")
	}
	for _, o := range e.store.activeOrders() {
		if ctx.Err() != nil {
			break
		}
		e.handleOrder(ctx, o)
	}
	e.store.sweep() // evict stale terminal orders so the maps stay bounded
}

func (e *executionService) pollOpenOrders(ctx context.Context) error {
	observation := observability.StartOperation(e.orderPollObserver)
	outcome := observability.ExternalOperationError
	defer func() { observation.Finish(ctx, outcome) }()
	listing, err := e.backend.listOpenOrders(ctx, lowerAddr(e.executor), e.orderLimit)
	if err != nil {
		return err
	}
	for _, order := range listing {
		fresh := e.store.upsertQueued(order.OrderID)
		if fresh {
			e.metrics.observeWin()
		}
	}
	e.metrics.observeOrderPoll(e.now())
	outcome = observability.ExternalOperationSuccess
	if len(listing) != 0 {
		e.log.V(1).Info("polled open orders", "count", len(listing))
	}
	return nil
}

func (e *executionService) handleOrder(ctx context.Context, o orderRecord) {
	switch o.Status {
	case statusQueued, statusSubmitting:
		e.submitOrder(ctx, o.OrderID)
	case statusSubmitted:
		e.reconcileTerminalStatus(ctx, o.OrderID)
	case statusFilled, statusExpired, statusFailed:
		// terminal — nothing to do
	}
}

// preparedFill carries the exact signed terms used for both execution and metrics.
// It is local to one attempt and is never reused after a backend refresh.
type preparedFill struct {
	request                     txmanager.Request
	quoteID                     string
	tokenIn, tokenOut           common.Address
	amountIn, required, surplus *big.Int
}

type rejectedFillError struct{ error }

func rejectFill(message string, err error) error {
	if err == nil {
		return &rejectedFillError{errors.New(message)}
	}
	return &rejectedFillError{errors.Errorf("%s: %w", message, err)}
}

func (e *executionService) submitOrder(ctx context.Context, orderID string) {
	e.store.markStatus(orderID, statusSubmitting, common.Hash{}, "")
	payload, err := e.resolveExecutable(ctx, orderID)
	if err != nil {
		e.log.Error(err, "resolve executable order", "orderId", orderID)
		return
	}
	if payload == nil {
		e.reconcileTerminalStatus(ctx, orderID)
		return
	}
	fill, err := e.prepareFill(ctx, payload)
	if err != nil {
		var rejected *rejectedFillError
		if errors.As(err, &rejected) {
			e.fail(orderID, err.Error())
		} else if ctx.Err() == nil {
			e.log.Error(err, "prepare fill (will retry)", "orderId", orderID)
		}
		return
	}
	// Send owns admitted work even after ctx cancellation. Bookkeeping below must
	// always run; only subsequent reconciliation depends on the caller context.
	result := e.txm.Send(ctx, fill.request)
	attempt := e.store.recordAttempt(orderID)
	if !result.Outcome.Included() {
		outcome := liquidlane.FillOutcomeFailure
		if result.NotAdmitted {
			outcome = liquidlane.FillOutcomeNotAdmitted
		}
		if e.metrics != nil {
			e.metrics.fillAmounts.ObserveOutcome(outcome)
		}
		err := result.Err
		if err == nil {
			err = errors.Errorf("unknown transaction outcome %q", result.Outcome)
		}
		e.log.Error(err, "fill failed", "orderId", orderID, "attempt", attempt, "tx", result.Hash.Hex())
		e.fail(orderID, err.Error())
		return
	}
	e.store.markStatus(orderID, statusSubmitted, result.Hash, "")
	if e.metrics != nil {
		e.metrics.fillAmounts.Observe(result.Receipt, fill.tokenIn, fill.amountIn, fill.tokenOut, fill.required, fill.surplus)
	}
	if result.Outcome == txmanager.OutcomeConfirmed {
		e.log.Info("filled order", "orderId", orderID, "quoteId", fill.quoteID, "tx", result.Hash.Hex())
	} else {
		e.log.Error(result.Err, "fill included but confirmation wait failed", "orderId", orderID, "tx", result.Hash.Hex())
	}
	e.reconcileTerminalStatus(ctx, orderID)
}

func (e *executionService) prepareFill(ctx context.Context, payload *executable) (*preparedFill, error) {
	order, err := decodeOrder(payload.encodedOrder)
	if err != nil {
		return nil, rejectFill("decode order", err)
	}
	output, required, err := executableOrderTerms(payload, order, e.executor)
	if err != nil {
		return nil, rejectFill("validate order", err)
	}
	// Anchor before the RPC, so transport latency consumes executable lifetime.
	observedAt := e.now()
	chainTime, err := e.reader.latestBlockTime(ctx)
	if err != nil {
		return nil, errors.Errorf("read chain time: %w", err)
	}
	deadline := time.Unix(order.Request.Deadline.Int64(), 0)
	if !deadline.After(chainTime) {
		return nil, rejectFill("order deadline has passed", nil)
	}
	plan, err := e.buildFillPlan(ctx, payload, order, output, required)
	if err != nil || plan == nil {
		return nil, rejectFill("strategy fill plan: "+errString(err), nil)
	}
	discountSwaps, discountDeadline, err := e.buildDiscountSwapInputs(ctx, plan, chainTime)
	if err != nil {
		if errors.Is(err, errDiscountAdapterMismatch) || errors.Is(err, errDiscountsDisabled) {
			return nil, rejectFill("resolve discounts", err)
		}
		return nil, errors.Errorf("resolve discounts: %w", err)
	}
	data, err := encodeFill(order, payload.signature, directSwaps(plan, order.Request.TokenIn, e.executor), discountSwaps, emptyExecutorData)
	if err != nil {
		return nil, rejectFill("encode fill", err)
	}
	cancelAt, ok := liquidlane.CancellationDeadline(rfqFillDeadline(deadline, discountDeadline), chainTime, observedAt, e.now())
	if !ok {
		return nil, rejectFill("fill execution deadline elapsed before submission", nil)
	}
	return &preparedFill{
		request: txmanager.Request{Solver: Name, To: e.executor, Data: data, CancelAt: cancelAt, Label: "rfq-fill"},
		quoteID: payload.quoteID, tokenIn: order.Request.TokenIn, tokenOut: output,
		amountIn: order.Request.AmountIn, required: required, surplus: liquidlane.PlannedSurplus(plan.QuotedAmountOut, required),
	}, nil
}

// resolveExecutable returns the executable payload for a polled order from the backend.
func (e *executionService) resolveExecutable(ctx context.Context, orderID string) (*executable, error) {
	bo, err := e.backend.getExecutableOrder(ctx, orderID, lowerAddr(e.executor))
	if err != nil {
		return nil, err
	}
	if bo == nil {
		return nil, nil
	}
	return executableFromBackend(bo)
}

func (e *executionService) reconcileTerminalStatus(ctx context.Context, orderID string) {
	order, err := e.backend.getOrder(ctx, orderID)
	if err != nil {
		e.log.Error(err, "reconcile: get order", "orderId", orderID)
		return
	}
	if order == nil || order.OrderStatus == backendOrderStatusOpen {
		return
	}
	var status orderStatus
	reason := ""
	switch order.OrderStatus {
	case "filled":
		status = statusFilled
	case "expired":
		status = statusExpired
	case "error", "cancelled", "unverified", "insufficient-funds":
		status, reason = statusFailed, "backend terminal status "+order.OrderStatus
	default:
		// Unknown or dropped status must retain the local admission: otherwise it could submit twice.
		e.log.Error(errUnknownOrderStatus, "reconcile: retaining order", "orderId", orderID, "status", order.OrderStatus)
		return
	}
	hash := common.Hash{}
	if order.TxHash != nil {
		// Malformed backend hashes never become padded or truncated transaction references.
		if parsed, parseErr := parse.Hash(*order.TxHash, "txHash"); parseErr == nil {
			hash = parsed
		}
	}
	e.store.markStatus(orderID, status, hash, reason)
}

var errUnknownOrderStatus = errors.New("unrecognized backend order status")

// buildFillPlan gives the trusted strategy the awarded order terms plus current solver inputs. The
// strategy owns route economics; the solver assembles the fresh snapshot and enforces solver-owned
// structural constraints on the returned plan.
func (e *executionService) buildFillPlan(ctx context.Context, payload *executable, order executor.IReactorOrder,
	output common.Address, required *big.Int) (*fillPlan, error) {
	inventories, err := e.fillInventories(ctx, order.Request.TokenIn)
	if err != nil {
		return nil, err
	}
	request := strategyRequest{RequestID: payload.quoteID, QuoteID: payload.quoteID,
		TokenIn: order.Request.TokenIn, TokenOut: output, Amount: order.Request.AmountIn}
	input := newQuoteInput(e.chainID, e.executor, request, nil, required,
		e.tokenPolicy.RequiresSingleRoute(request.TokenIn), e.now())
	if len(inventories) != 0 {
		input.Candidates, err = e.reader.readQuoteCandidates(ctx, inventories, request.TokenIn, request.TokenOut, request.Amount)
		if err != nil {
			return nil, errors.Errorf("fill: read LiquidLane candidates: %w", err)
		}
	}
	input.Now = e.now()
	plan, err := e.strategy.BuildFillPlan(ctx, input)
	if err != nil || plan == nil {
		return plan, err
	}
	if err := validateSingleRoute(input.RequireSingleRoute, len(plan.Legs)); err != nil {
		return nil, errors.Errorf("fill: strategy: %w", err)
	}
	return plan, nil
}

func (e *executionService) fillInventories(ctx context.Context, token common.Address) ([]solverInventory, error) {
	var inventories []solverInventory
	if len(e.vaults) != 0 {
		direct, err := e.reader.readPermissionedVaultInventories(ctx, e.executor, token, e.vaults)
		if err != nil {
			return nil, errors.Errorf("fill: permissioned inventories: %w", err)
		}
		inventories = direct
	}
	// External mode has no access to the internal discounts surface. Direct inventories take precedence.
	if e.discountsEnabled {
		inventories = append(inventories, e.discountInventories(ctx, token, inventories)...)
	}
	return inventories, nil
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
	var out []solverInventory
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
			Discount:         executor.ILiquidLaneAdapterDiscount(parsed.Terms.Clone()),
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

/* ───────── executable helpers ───────── */

func executableFromBackend(order *backendOrder) (*executable, error) {
	if order == nil || order.EncodedOrder == nil || order.ProtocolSignature == nil || order.Deadline == nil || order.Filler == nil {
		return nil, errors.New("executable order payload incomplete")
	}
	filler, err := parse.Address(*order.Filler, "filler")
	if err != nil {
		return nil, err
	}
	payload := &executable{quoteID: order.QuoteID, filler: filler, deadline: *order.Deadline,
		outputs: append([]backendOut(nil), order.Outputs...)}
	for _, field := range []struct {
		name, encoded string
		target        *[]byte
	}{
		{"encodedOrder", *order.EncodedOrder, &payload.encodedOrder},
		{"protocolSignature", *order.ProtocolSignature, &payload.signature},
	} {
		decoded, err := hexutil.Decode(field.encoded)
		if err != nil {
			return nil, errors.Errorf("decode %s: %w", field.name, err)
		}
		*field.target = decoded
	}
	return payload, nil
}

func executableOrderTerms(payload *executable, order executor.IReactorOrder, filler common.Address) (common.Address, *big.Int, error) {
	request := order.Request
	switch {
	case order.Filler != filler:
		return common.Address{}, nil, errors.New("signed order assigns a different filler")
	case payload.filler != order.Filler:
		return common.Address{}, nil, errors.New("backend filler does not match signed order")
	case request.TokenIn == (common.Address{}) || request.AmountIn == nil || request.AmountIn.Sign() <= 0:
		return common.Address{}, nil, errors.New("signed order has invalid input")
	case request.Deadline == nil || !request.Deadline.IsInt64() || request.Deadline.Sign() <= 0:
		return common.Address{}, nil, errors.New("signed order has invalid deadline")
	case payload.deadline != request.Deadline.Int64():
		return common.Address{}, nil, errors.New("backend deadline does not match signed order")
	}
	if len(order.Outputs) == 0 {
		return common.Address{}, nil, errors.New("only single output-token orders are supported")
	}
	if len(order.Outputs) != len(payload.outputs) {
		return common.Address{}, nil, errors.New("backend outputs do not match signed order")
	}
	token, total := order.Outputs[0].Token, new(big.Int)
	for index, output := range order.Outputs {
		if token == (common.Address{}) || output.Token != token {
			return common.Address{}, nil, errors.New("only single output-token orders are supported")
		}
		if output.Amount == nil || output.Amount.Sign() <= 0 {
			return common.Address{}, nil, errors.Errorf("signed order output %d has invalid amount", index)
		}
		if output.Recipient == (common.Address{}) {
			return common.Address{}, nil, errors.Errorf("signed order output %d has invalid recipient", index)
		}
		backend := payload.outputs[index]
		backendToken, tokenErr := parse.Address(backend.Token, "token")
		recipient, recipientErr := parse.Address(backend.Recipient, "recipient")
		amount, amountErr := parseUint256(backend.Amount, "amount")
		if tokenErr != nil || recipientErr != nil {
			return common.Address{}, nil, errors.Errorf("backend output %d has invalid address", index)
		}
		if amountErr != nil || amount.Sign() <= 0 {
			return common.Address{}, nil, errors.Errorf("backend output %d has invalid amount", index)
		}
		if backendToken != token || recipient != output.Recipient || amount.Cmp(output.Amount) != 0 {
			return common.Address{}, nil, errors.Errorf("backend output %d does not match signed order", index)
		}
		total.Add(total, output.Amount)
	}
	return token, total, nil
}
