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
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// txSender sends a transaction through the shared txmanager and returns its explicit lifecycle
// outcome. Callers branch on Result.State; Err alone does not establish retry safety.
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
	projected    backendOrder
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
	discountsEnabled bool             // false (external solver) skips the backend discounts API entirely
	backend          orderBackend
	store            *store
	reader           recoveryReader
	strategy         types.Strategy
	txm              txSender
	log              logr.Logger
	now              func() time.Time

	inflightMu sync.Mutex
	inflight   map[string]bool
}

// recoveryReader is the on-chain surface used to assemble fill-time strategy inputs.
type recoveryReader interface {
	readPermissionedVaultInventories(
		ctx context.Context, executor, tokenIn common.Address, vaults []recoveryVault,
	) ([]solverInventory, error)
	// resolveVaults returns the config entries with Vault/Asset resolved from the adapter at startup
	// (config carries only adapter addresses).
	resolveVaults(ctx context.Context, vaults []recoveryVault) ([]recoveryVault, error)
}

func (e *executionService) run(ctx context.Context, interval time.Duration) error {
	e.syncOnce(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
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
	outputToken, required, err := validateSignedOrder(order, e.executor, e.now())
	if err != nil {
		e.fail(orderID, err.Error())
		return
	}
	if err := validateBackendProjection(exec.projected, order); err != nil {
		e.fail(orderID, err.Error())
		return
	}

	selected, err := e.buildFillPlan(ctx, exec, order, outputToken, required)
	if err != nil || selected == nil {
		e.fail(orderID, "strategy fill plan: "+errString(err))
		return
	}

	swaps := directSwaps(selected, order.Request.TokenIn, e.executor)
	discountSwaps, err := e.buildDiscountSwapInputs(ctx, selected)
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

	res := e.txm.Send(ctx, txmanager.Request{To: e.executor, Data: calldata, Label: "rfq-fill"})
	attempt := e.store.recordAttempt(orderID)
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
	case txmanager.StateBroadcastUnknown, txmanager.StatePending:
		fallthrough
	default:
		err := errors.Errorf("unexpected txmanager state %q", res.State)
		e.log.Error(err, "fill transaction state invalid; reconciling without retry", "orderId", orderID)
		e.store.markStatus(orderID, statusSubmitted, res.Hash, err.Error())
		e.reconcileTerminalStatus(ctx, orderID)
	}
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
	if bo.OrderID != local.OrderID {
		return nil, errors.Errorf("backend returned order %q for requested order %q", bo.OrderID, local.OrderID)
	}
	if bo.QuoteID != local.QuoteID {
		return nil, errors.Errorf(
			"backend returned quote %q for local quote %q", bo.QuoteID, local.QuoteID)
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
	case backendStatusOpen:
		// still open; leave as-is for the next cycle
	default:
		e.store.markStatus(orderID, statusFailed, txHash, "backend terminal status "+bo.OrderStatus)
	}
}

// buildFillPlan gives the trusted strategy the awarded order terms plus current solver inputs. The
// strategy owns cached quote lookup and recovery; solver only assembles the snapshot and executes the
// returned plan.
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
	input := newFillInput(e.chainID, e.executor, req, inv, required, e.now())
	return e.strategy.BuildFillPlan(ctx, input)
}

// buildDiscountSwapInputs resolves each discount leg's fresh signed discount from the backend and
// encodes it into the Executor's DiscountSwapInput. Direct-only strategies return nil.
func (e *executionService) buildDiscountSwapInputs(
	ctx context.Context, selected *fillPlan,
) ([]executor.IReactorDiscountSwapInput, error) {
	var out []executor.IReactorDiscountSwapInput
	for _, leg := range selected.Legs {
		if leg.DiscountID == nil {
			continue
		}
		// Defensive: external solvers never produce discount legs; fail closed (see errDiscountsDisabled).
		if !e.discountsEnabled {
			return nil, errors.Errorf("%w: leg %s", errDiscountsDisabled, leg.DiscountID.Hex())
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
// against tokenIn into discount-leg candidates: keep discounts whose adapter is whitelisted, whose
// tokenToRedeem == tokenIn, and whose adapter is not already permissioned (the asset==tokenOut check
// is left to the evaluator).
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
		if !e.whitelist.allows(adapter) {
			continue
		}
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

// errDiscountAdapterMismatch marks a backend-resolved discount whose adapter differs from the
// strategy leg it was quoted for. The leg's adapter was whitelist-filtered at selection time, so a
// mismatch means the backend swapped the adapter under us — never fill through it.
var errDiscountAdapterMismatch = errors.New("resolved discount adapter does not match the strategy leg adapter")

// errDiscountsDisabled marks a discount leg seen while discounts are off (external solver). Terminal —
// fail the order, no tx. Defensive: a restart (needed to change config) wipes the cache, so it's unreachable.
var errDiscountsDisabled = errors.New("discount leg present but discounts are disabled")

// toDiscountSwapInput converts a resolved signed discount + its strategy leg into the Executor input.
func toDiscountSwapInput(
	r *resolveDiscountResponse, leg fillLeg, recipient common.Address,
) (executor.IReactorDiscountSwapInput, error) {
	d := r.Discount
	for _, a := range []string{d.Adapter, d.TokenToRedeem, d.Signer, d.Protocol} {
		if !common.IsHexAddress(a) {
			return executor.IReactorDiscountSwapInput{}, errors.Errorf("discount: invalid address %q", a)
		}
	}
	if common.HexToAddress(d.Adapter) != leg.Adapter {
		return executor.IReactorDiscountSwapInput{}, errors.Errorf(
			"%w: resolved %s, leg %s", errDiscountAdapterMismatch, d.Adapter, leg.Adapter.Hex())
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

func validateSignedOrder(
	order executor.IReactorOrder,
	configuredExecutor common.Address,
	now time.Time,
) (common.Address, *big.Int, error) {
	if order.Filler != configuredExecutor {
		return common.Address{}, nil, errors.Errorf(
			"decoded order filler %s does not match configured executor %s",
			order.Filler.Hex(), configuredExecutor.Hex())
	}
	if order.Request.TokenIn == (common.Address{}) {
		return common.Address{}, nil, errors.New("decoded order has zero input token")
	}
	if order.Request.AmountIn == nil || order.Request.AmountIn.Sign() <= 0 {
		return common.Address{}, nil, errors.New("decoded order has invalid input amount")
	}
	if order.Request.Deadline == nil || order.Request.Deadline.Cmp(big.NewInt(now.Unix())) <= 0 {
		return common.Address{}, nil, errors.New("order deadline has passed")
	}
	if len(order.Outputs) == 0 {
		return common.Address{}, nil, errors.New("decoded order has no outputs")
	}

	token := order.Outputs[0].Token
	if token == (common.Address{}) {
		return common.Address{}, nil, errors.New("decoded order has zero output token")
	}
	required := new(big.Int)
	for i := range order.Outputs {
		out := order.Outputs[i]
		if out.Token != token {
			return common.Address{}, nil, errors.New("decoded order has multiple output tokens")
		}
		if out.Amount == nil || out.Amount.Sign() <= 0 {
			return common.Address{}, nil, errors.Errorf("decoded order output %d has invalid amount", i)
		}
		required.Add(required, out.Amount)
	}
	return token, required, nil
}

func validateBackendProjection(projected backendOrder, order executor.IReactorOrder) error {
	if projected.Filler != nil {
		if !common.IsHexAddress(*projected.Filler) ||
			common.HexToAddress(*projected.Filler) != order.Filler {
			return errors.New("backend filler does not match decoded order")
		}
	}
	if projected.Outputs == nil {
		return nil
	}
	if len(projected.Outputs) != len(order.Outputs) {
		return errors.New("backend outputs do not match decoded order")
	}
	for i := range projected.Outputs {
		got := projected.Outputs[i]
		want := order.Outputs[i]
		amount, ok := new(big.Int).SetString(got.Amount, 10)
		if !ok || amount.Sign() < 0 ||
			!common.IsHexAddress(got.Token) || common.HexToAddress(got.Token) != want.Token ||
			!common.IsHexAddress(got.Recipient) || common.HexToAddress(got.Recipient) != want.Recipient ||
			want.Amount == nil || amount.Cmp(want.Amount) != 0 {
			return errors.Errorf("backend output %d does not match decoded order", i)
		}
	}
	return nil
}

func executableFromBackend(bo *backendOrder) (*executable, error) {
	if bo.EncodedOrder == nil || bo.ProtocolSignature == nil {
		return nil, errors.New("executable order payload incomplete")
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
		projected:    *bo,
	}, nil
}

// isHash32 reports whether s is a 0x-prefixed, well-formed 32-byte hash.
func isHash32(s string) bool {
	b, err := hexutil.Decode(s)
	return err == nil && len(b) == 32
}
