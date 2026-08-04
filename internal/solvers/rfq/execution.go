package rfq

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
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

// executionService polls the backend for open orders and fills them via the Executor. It runs in its
// own goroutine; per-order work is guarded by an in-flight set so overlapping poll cycles never
// double-submit the same order.
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
	strategy         types.Strategy
	txm              txSender
	log              logr.Logger
	now              func() time.Time

	inflightMu sync.Mutex
	inflight   map[string]bool
}

// fillReader is the on-chain surface used to assemble fill-time strategy inputs.
type fillReader interface {
	quoteCandidateReader
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
		e.handleOrder(ctx, o)
	}
	e.store.sweep() // evict stale terminal orders so the maps stay bounded
}

func (e *executionService) pollOpenOrders(ctx context.Context) error {
	orders, err := e.backend.listOpenOrders(ctx, lowerAddr(e.executor), e.orderLimit)
	if err != nil {
		return err
	}
	for i := range orders {
		o := &orders[i]
		e.store.upsertQueued(queuedOrder{OrderID: o.OrderID, QuoteID: o.QuoteID})
	}
	if len(orders) > 0 {
		e.log.V(1).Info("polled open orders", "count", len(orders))
	}
	return nil
}

func (e *executionService) handleOrder(ctx context.Context, o *orderRecord) {
	if !e.acquire(o.OrderID) {
		return
	}
	defer e.release(o.OrderID)

	switch o.Status {
	case statusQueued, statusSubmitting:
		e.submitOrder(ctx, o.OrderID)
	case statusSubmitted:
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
		e.log.Error(err, "resolve executable order", "orderId", orderID)
		return // transient; retried next cycle
	}
	if exec == nil {
		e.reconcileTerminalStatus(ctx, orderID)
		return
	}
	order, err := decodeOrder(exec.encodedOrder)
	if err != nil {
		e.fail(orderID, "decode order: "+err.Error())
		return
	}
	outputToken, required, err := executableOrderTerms(exec, order, e.executor)
	if err != nil {
		e.fail(orderID, "validate order: "+err.Error())
		return
	}
	if dl := order.Request.Deadline; dl == nil || dl.Int64() <= e.now().Unix() {
		// Skip an already-expired order rather than spend gas on a fill the Reactor will revert.
		e.fail(orderID, "order deadline has passed")
		return
	}

	selected, err := e.buildFillPlan(ctx, exec, order, outputToken, required)
	if err != nil || selected == nil {
		e.fail(orderID, "strategy fill plan: "+errString(err))
		return
	}

	swaps := directSwaps(selected, order.Request.TokenIn, e.executor)
	discountSwaps, discountValidUntil, err := e.buildDiscountSwapInputs(ctx, selected)
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
		// in submitting and retry next cycle) rather than terminal. Once the order is no longer open
		// the executable lookup returns nil and reconciliation marks it expired/filled.
		e.log.Error(err, "resolve discounts (will retry)", "orderId", orderID)
		return
	}
	calldata, err := encodeFill(order, exec.signature, swaps, discountSwaps, emptyExecutorData)
	if err != nil {
		e.fail(orderID, "encode fill: "+err.Error())
		return
	}

	cancelAt := time.Unix(order.Request.Deadline.Int64(), 0)
	if !discountValidUntil.IsZero() && discountValidUntil.Before(cancelAt) {
		cancelAt = discountValidUntil
	}
	res := e.txm.Send(ctx, txmanager.Request{
		To: e.executor, Data: calldata, CancelAt: cancelAt, Label: "rfq-fill",
	})
	attempt := e.store.recordAttempt(orderID)
	if res.Err != nil {
		e.log.Error(res.Err, "fill failed", "orderId", orderID, "attempt", attempt, "tx", res.Hash.Hex())
		e.fail(orderID, res.Err.Error())
		return
	}
	e.log.Info("filled order", "orderId", orderID, "quoteId", exec.quoteID, "tx", res.Hash.Hex())
	e.store.markStatus(orderID, statusSubmitted, res.Hash, "")
	e.reconcileTerminalStatus(ctx, orderID)
}

// resolveExecutable returns the executable payload for a polled order from the backend.
func (e *executionService) resolveExecutable(ctx context.Context, local *orderRecord) (*executable, error) {
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

// buildFillPlan gives the trusted strategy the awarded order terms plus current solver inputs. The
// strategy owns route economics; the solver assembles the fresh snapshot and enforces solver-owned
// structural constraints on the returned plan.
func (e *executionService) buildFillPlan(
	ctx context.Context,
	exec *executable,
	order executor.IReactorOrder,
	outputToken common.Address,
	required *big.Int,
) (*fillPlan, error) {
	// Direct inventories are filtered to adapters this executor is authorized to fill through. Skipped
	// when no candidate vaults are configured (a discount-only solver), leaving discount legs only.
	inv := make([]solverInventory, 0, len(e.vaults)+1)
	if len(e.vaults) > 0 {
		direct, derr := e.reader.readPermissionedVaultInventories(ctx, e.executor, order.Request.TokenIn, e.vaults)
		if derr != nil {
			return nil, derr
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
		var err error
		candidates, err = e.reader.readQuoteCandidates(ctx, inv, req.TokenIn, req.TokenOut, req.Amount)
		if err != nil {
			return nil, errors.Errorf("fill: read LiquidLane candidates: %w", err)
		}
	}
	input := newFillInput(e.chainID, e.executor, req, candidates, required, requireSingleRoute, e.now())
	plan, err := e.strategy.BuildFillPlan(ctx, input)
	if err != nil || plan == nil {
		return plan, err
	}
	if err := validateSingleRoute(input.RequireSingleRoute, len(plan.Legs)); err != nil {
		return nil, errors.Errorf("fill: strategy: %w", err)
	}
	return plan, nil
}

// buildDiscountSwapInputs resolves each discount leg's fresh signed discount from the backend and
// encodes it into the Executor's DiscountSwapInput. Direct-only strategies return nil.
func (e *executionService) buildDiscountSwapInputs(
	ctx context.Context, selected *fillPlan,
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
		}, e.now()); err != nil {
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

// discountInventories fetches offered discounts and turns those redeemable
// against tokenIn into discount-leg candidates: keep discounts whose adapter is whitelisted, whose
// tokenToRedeem == tokenIn, and whose adapter is not already permissioned. Solver-side candidate
// normalization later filters collateral to the order's tokenOut.
func (e *executionService) discountInventories(
	ctx context.Context, tokenIn common.Address, direct []solverInventory,
) []solverInventory {
	resp, err := e.backend.listDiscounts(ctx)
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
