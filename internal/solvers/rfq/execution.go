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
	chainID    int64
	executor   common.Address
	orderLimit int
	vaults     []recoveryVault
	backend    orderBackend
	store      *store
	reader     *reader
	txm        txSender
	log        logr.Logger
	now        func() time.Time

	inflightMu sync.Mutex
	inflight   map[string]bool
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
	e.store.sweep() // evict stale strategies/terminal orders so the maps stay bounded
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
	if exec.filler != e.executor {
		e.fail(orderID, "backend assigned a different filler")
		return
	}

	selected := e.store.strategy(exec.quoteID)
	if selected == nil {
		if selected, err = e.recoverStrategy(ctx, exec); err != nil || selected == nil {
			e.fail(orderID, "missing strategy for quoteId "+exec.quoteID)
			return
		}
	}

	order, err := decodeOrder(exec.encodedOrder)
	if err != nil {
		e.fail(orderID, "decode order: "+err.Error())
		return
	}
	if dl := order.Request.Deadline; dl == nil || dl.Int64() <= e.now().Unix() {
		// Skip an already-expired order rather than spend gas on a fill the Reactor will revert.
		e.fail(orderID, "order deadline has passed")
		return
	}
	outputToken, ok := singleOutputToken(exec.outputs)
	if !ok {
		e.fail(orderID, "only single output-token orders are supported")
		return
	}
	required, err := sumOutputs(exec.outputs)
	if err != nil {
		e.fail(orderID, "sum outputs: "+err.Error())
		return
	}
	// The strategy is looked up by the backend-supplied quoteId, so bind it to the awarded order's
	// own terms before filling: a reused/mismatched quoteId must not let us fill on stale pricing.
	if selected.Asset != outputToken {
		e.fail(orderID, "strategy asset does not match order output token")
		return
	}
	if selected.TokenIn != order.Request.TokenIn || selected.TokenOut != outputToken ||
		selected.AmountIn.Cmp(order.Request.AmountIn) != 0 {
		e.fail(orderID, "stored strategy does not match the awarded order (tokenIn/tokenOut/amountIn)")
		return
	}
	if selected.QuotedAmountOut.Cmp(required) < 0 {
		e.fail(orderID, "stored strategy output is below the required order output")
		return
	}

	swaps := directSwaps(selected, order.Request.TokenIn, e.executor)
	discountSwaps, err := e.buildDiscountSwapInputs(ctx, selected)
	if err != nil {
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

	res := e.txm.Send(ctx, txmanager.Request{To: e.executor, Data: calldata, Label: "rfq-fill"})
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

// recoverStrategy rebuilds a strategy from current on-chain state when the quote-time strategy is
// not cached (e.g. after a restart). Requires a configured candidate vault universe.
func (e *executionService) recoverStrategy(ctx context.Context, exec *executable) (*strategyRecord, error) {
	if len(e.vaults) == 0 {
		return nil, nil
	}
	order, err := decodeOrder(exec.encodedOrder)
	if err != nil {
		return nil, err
	}
	outputToken, ok := singleOutputToken(exec.outputs)
	if !ok {
		return nil, nil
	}
	required, err := sumOutputs(exec.outputs)
	if err != nil {
		return nil, err
	}
	// Direct inventories are filtered to adapters this executor is authorized to fill through.
	inv, err := e.reader.readPermissionedVaultInventories(ctx, e.executor, order.Request.TokenIn, e.vaults)
	if err != nil {
		return nil, err
	}
	inv = append(inv, e.discountInventories(ctx, order.Request.TokenIn, inv)...)
	if len(inv) == 0 {
		return nil, nil
	}
	tokenInDecimals, err := e.reader.tokenDecimals(ctx, order.Request.TokenIn)
	if err != nil {
		return nil, err
	}
	matching := matchingInventories(inv, outputToken)
	oracle, err := e.reader.amountsOut(ctx, order.Request.TokenIn, matching, order.Request.AmountIn)
	if err != nil {
		return nil, err
	}
	req := strategyRequest{
		RequestID: exec.quoteID, QuoteID: exec.quoteID,
		TokenIn: order.Request.TokenIn, TokenOut: outputToken, Amount: order.Request.AmountIn,
	}
	best := selectBestStrategy(req, inv, tokenInDecimals, oracle, e.now())
	if best == nil || best.QuotedAmountOut.Cmp(required) < 0 {
		return nil, nil
	}
	e.store.putStrategy(best)
	return best, nil
}

// buildDiscountSwapInputs resolves each discount leg's fresh signed discount from the backend and
// encodes it into the Executor's DiscountSwapInput. Direct-only strategies return nil.
func (e *executionService) buildDiscountSwapInputs(
	ctx context.Context, selected *strategyRecord,
) ([]executor.IReactorDiscountSwapInput, error) {
	var out []executor.IReactorDiscountSwapInput
	for _, leg := range selected.Legs {
		if leg.DiscountID == nil {
			continue
		}
		resolved, err := e.backend.resolveDiscount(ctx, leg.DiscountID.Hex())
		if err != nil {
			return nil, errors.Errorf("resolve discount %s: %w", leg.DiscountID.Hex(), err)
		}
		dsi, err := toDiscountSwapInput(resolved, leg, e.executor)
		if err != nil {
			return nil, err
		}
		out = append(out, dsi)
	}
	return out, nil
}

// discountInventories fetches offered discounts and, for strategy recovery, turns those redeemable
// against tokenIn into discount-leg candidates: keep discounts whose tokenToRedeem == tokenIn and
// whose adapter is not already permissioned (the asset==tokenOut check is left to the evaluator).
func (e *executionService) discountInventories(
	ctx context.Context, tokenIn common.Address, direct []solverInventory,
) []solverInventory {
	resp, err := e.backend.listDiscounts(ctx)
	if err != nil {
		e.log.Error(err, "recover: list discounts")
		return nil
	}
	seen := make(map[common.Address]bool, len(direct))
	for _, d := range direct {
		seen[d.Adapter] = true
	}
	var out []solverInventory
	for _, d := range resp.Discounts {
		if !common.IsHexAddress(d.Adapter) || !common.IsHexAddress(d.TokenToRedeem) || !common.IsHexAddress(d.Collateral) {
			continue
		}
		if common.HexToAddress(d.TokenToRedeem) != tokenIn {
			continue
		}
		adapter := common.HexToAddress(d.Adapter)
		if seen[adapter] {
			continue
		}
		maxOut, ok1 := new(big.Int).SetString(d.MaxAssets, 10)
		maxRate, ok2 := new(big.Int).SetString(d.MaxRate, 10)
		if !ok1 || !ok2 {
			continue
		}
		h := common.HexToHash(d.DiscountID)
		out = append(out, solverInventory{
			Adapter: adapter, Asset: common.HexToAddress(d.Collateral), AssetDecimals: d.CollateralDecimals,
			MaxAssets: maxOut, MaxRate: maxRate, DiscountID: &h,
		})
	}
	return out
}

// toDiscountSwapInput converts a resolved signed discount + its strategy leg into the Executor input.
func toDiscountSwapInput(
	r *resolveDiscountResponse, leg strategyLeg, recipient common.Address,
) (executor.IReactorDiscountSwapInput, error) {
	d := r.Discount
	for _, a := range []string{d.Adapter, d.TokenToRedeem, d.Signer, d.Protocol} {
		if !common.IsHexAddress(a) {
			return executor.IReactorDiscountSwapInput{}, errors.Errorf("discount: invalid address %q", a)
		}
	}
	discount, ok := new(big.Int).SetString(d.Discount, 10)
	if !ok {
		return executor.IReactorDiscountSwapInput{}, errors.Errorf("discount: invalid amount %q", d.Discount)
	}
	nonce, err := hexutil.DecodeBig(d.Nonce)
	if err != nil {
		return executor.IReactorDiscountSwapInput{}, errors.Errorf("discount: invalid nonce %q: %w", d.Nonce, err)
	}
	signerSig, err := hexutil.Decode(r.SignerSignature)
	if err != nil {
		return executor.IReactorDiscountSwapInput{}, errors.Errorf("discount: signerSignature: %w", err)
	}
	protocolSig, err := hexutil.Decode(r.ProtocolSignature)
	if err != nil {
		return executor.IReactorDiscountSwapInput{}, errors.Errorf("discount: protocolSignature: %w", err)
	}
	// Mirrors buildDiscountSwapInputs in discounts.ts: the outer adapter comes from the resolved
	// discount's adapter, the inner Discount no longer carries the vault field, and the input drops
	// amountOut.
	return executor.IReactorDiscountSwapInput{
		Adapter: common.HexToAddress(d.Adapter),
		DiscountSwap: executor.ILiquidLaneAdapterDiscountSwap{
			Discount: executor.ILiquidLaneAdapterDiscount{
				TokenToRedeem: common.HexToAddress(d.TokenToRedeem),
				Discount:      discount, Signer: common.HexToAddress(d.Signer), Protocol: common.HexToAddress(d.Protocol),
				Nonce: nonce, Deadline: big.NewInt(d.Deadline),
			},
			SignerSignature:  signerSig,
			ProtocolDeadline: big.NewInt(r.ProtocolDeadline),
		},
		ProtocolSignature: protocolSig,
		Recipient:         recipient,
		AmountIn:          new(big.Int).Set(leg.AmountIn),
	}, nil
}

func (e *executionService) fail(orderID, msg string) {
	e.log.Info("order failed", "orderId", orderID, "reason", msg)
	e.store.markStatus(orderID, statusFailed, common.Hash{}, msg)
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

func singleOutputToken(outputs []backendOut) (common.Address, bool) {
	if len(outputs) == 0 {
		return common.Address{}, false
	}
	token := outputs[0].Token
	for _, o := range outputs {
		if o.Token != token {
			return common.Address{}, false
		}
	}
	if !common.IsHexAddress(token) {
		return common.Address{}, false
	}
	return common.HexToAddress(token), true
}

func sumOutputs(outputs []backendOut) (*big.Int, error) {
	total := new(big.Int)
	for _, o := range outputs {
		amt, ok := new(big.Int).SetString(o.Amount, 10)
		if !ok {
			return nil, errors.Errorf("invalid output amount %q", o.Amount)
		}
		total.Add(total, amt)
	}
	return total, nil
}
