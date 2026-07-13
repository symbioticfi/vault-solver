package rfq

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type fakeBackend struct {
	open         []backendOrder
	executable   *backendOrder
	order        *backendOrder
	discount     *resolveDiscountResponse
	discounts    *discountsResponse
	resolveCalls int
	listCalls    int
}

func (f *fakeBackend) listOpenOrders(context.Context, string, int) ([]backendOrder, error) {
	return f.open, nil
}
func (f *fakeBackend) getExecutableOrder(context.Context, string, string) (*backendOrder, error) {
	return f.executable, nil
}
func (f *fakeBackend) getOrder(context.Context, string) (*backendOrder, error) { return f.order, nil }

func (f *fakeBackend) resolveDiscount(context.Context, string) (*resolveDiscountResponse, error) {
	f.resolveCalls++
	return f.discount, nil
}

func (f *fakeBackend) listDiscounts(context.Context) (*discountsResponse, error) {
	f.listCalls++
	if f.discounts == nil {
		return &discountsResponse{}, nil
	}
	return f.discounts, nil
}

// fakeRecoveryReader is the solver-owned on-chain surface used to assemble fill-time inputs.
// readPermissionedVaultInventories is only invoked when vaults are configured.
type fakeRecoveryReader struct {
	permInv []solverInventory
	permErr error
}

func (f *fakeRecoveryReader) readPermissionedVaultInventories(
	context.Context, common.Address, common.Address, []recoveryVault,
) ([]solverInventory, error) {
	return f.permInv, f.permErr
}

func (f *fakeRecoveryReader) resolveVaults(_ context.Context, vaults []recoveryVault) ([]recoveryVault, error) {
	return vaults, nil
}

type fakeTxm struct {
	lastData []byte
	result   txmanager.Result
}

func (f *fakeTxm) Send(_ context.Context, req txmanager.Request) txmanager.Result {
	f.lastData = req.Data
	return f.result
}

func strPtr(s string) *string { return &s }
func i64Ptr(i int64) *int64   { return &i }

// newExec builds an internal-solver service (discountsEnabled: true); the external path is covered by
// the TestExecution_DiscountsDisabled* tests, which flip the field.
func newExec(t *testing.T, st *store, be orderBackend, txm txSender) *executionService {
	t.Helper()
	return &executionService{
		chainID: 1, executor: common.HexToAddress("0x0000000000000000000000000000000000000010"),
		orderLimit: 20, backend: be, store: st, txm: txm, discountsEnabled: true,
		strategy: fixedFillStrategy{plan: baseFillPlan()},
		log:      logr.Discard(), now: func() time.Time { return time.Unix(0, 0) },
		inflight: make(map[string]bool),
	}
}

type fixedFillStrategy struct {
	plan *types.FillPlan
	err  error
}

func (s fixedFillStrategy) DecideQuote(
	context.Context,
	types.QuoteInput,
) (types.QuoteOutput, error) {
	return types.QuoteOutput{}, nil
}

func (s fixedFillStrategy) BuildFillPlan(
	context.Context,
	types.FillInput,
) (*types.FillPlan, error) {
	return s.plan, s.err
}

func baseFillPlan() *types.FillPlan {
	return &types.FillPlan{
		QuoteID: "q1", TokenIn: tIn, TokenOut: tOut,
		AmountIn: big.NewInt(1_000000000000000000), QuotedAmountOut: big.NewInt(900000),
		Legs: []types.FillLeg{{
			Adapter: vlt, AmountIn: big.NewInt(1_000000000000000000), AmountOut: big.NewInt(900000),
		}},
	}
}

func discountFillPlan(h common.Hash) *types.FillPlan {
	return &types.FillPlan{
		QuoteID: "q1", TokenIn: tIn, TokenOut: tOut,
		AmountIn: big.NewInt(1_000000000000000000), QuotedAmountOut: big.NewInt(900000),
		Legs: []types.FillLeg{{
			Adapter: vlt, AmountIn: big.NewInt(1_000000000000000000), AmountOut: big.NewInt(900000),
			MaxRate: big.NewInt(1), DiscountID: &h,
		}},
	}
}

// backend order whose payload matches sampleOrder() from order_test.go.
func fillFixtures(t *testing.T) (*store, *fakeBackend) {
	t.Helper()
	st := newStore(func() time.Time { return time.Unix(0, 0) })
	encoded, err := orderTupleArgs.Pack(sampleOrder())
	if err != nil {
		t.Fatalf("pack order: %v", err)
	}
	filler := "0x0000000000000000000000000000000000000010"
	executable := &backendOrder{
		OrderID: "o1", OrderStatus: "open", QuoteID: "q1",
		Outputs:      []backendOut{{Token: tOut.Hex(), Amount: "900000", Recipient: "0x0000000000000000000000000000000000000099"}},
		EncodedOrder: strPtr(hexutil.Encode(encoded)), ProtocolSignature: strPtr("0xabcd"), Deadline: i64Ptr(4_102_444_800), Filler: &filler,
	}
	be := &fakeBackend{
		open:       []backendOrder{{OrderID: "o1", OrderStatus: "open", QuoteID: "q1", Filler: &filler}},
		executable: executable,
		order:      &backendOrder{OrderID: "o1", OrderStatus: "filled", QuoteID: "q1"},
	}
	return st, be
}

func TestExecution_DirectFillHappyPath(t *testing.T) {
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: txmanager.Result{Hash: common.HexToHash("0xdead")}}
	e := newExec(t, st, be, txm)

	e.syncOnce(context.Background())

	rec := st.order("o1")
	if rec == nil || rec.Status != statusFilled {
		t.Fatalf("status = %v, want filled", rec)
	}
	if len(txm.lastData) < 4 {
		t.Fatalf("no fill calldata sent")
	}
}

func TestExecution_RevertMarksFailed(t *testing.T) {
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: txmanager.Result{Hash: common.HexToHash("0xdead"), Err: errors.New("tx reverted on-chain")}}
	e := newExec(t, st, be, txm)

	e.syncOnce(context.Background())

	if rec := st.order("o1"); rec == nil || rec.Status != statusFailed {
		t.Fatalf("status = %v, want failed", rec)
	}
}

func TestExecution_DiscountFill(t *testing.T) {
	st, be := fillFixtures(t)
	h := common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000ab")
	be.discount = &resolveDiscountResponse{
		Discount: discountTerms{
			Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Discount: "500",
			Signer:   "0x00000000000000000000000000000000000000a1",
			Protocol: "0x00000000000000000000000000000000000000a2",
			Nonce:    "0x1", Deadline: 4_102_444_800,
		},
		SignerSignature: "0xaa", ProtocolDeadline: 4_102_444_800, ProtocolSignature: "0xbb",
	}
	txm := &fakeTxm{result: txmanager.Result{Hash: common.HexToHash("0xdead")}}
	e := newExec(t, st, be, txm)
	e.strategy = fixedFillStrategy{plan: discountFillPlan(h)}

	e.syncOnce(context.Background())

	if rec := st.order("o1"); rec == nil || rec.Status != statusFilled {
		t.Fatalf("status = %v, want filled", rec)
	}
	if be.resolveCalls != 1 {
		t.Fatalf("resolveDiscount calls = %d, want 1", be.resolveCalls)
	}
	if len(txm.lastData) < 4 {
		t.Fatalf("no fill calldata sent")
	}
}

// TestExecution_DiscountOnlyRecovery_EmptyVaults proves a discount-only solver (no configured vaults)
// still rebuilds a fill plan after a restart: BuildFillPlan skips the direct (vault) read but consults
// the backend's live discounts, rebuilds a discount-leg plan, and fills. (Regression: the old
// `len(vaults)==0` guard returned before the discount path ran.)
func TestExecution_DiscountOnlyRecovery_EmptyVaults(t *testing.T) {
	_, be := fillFixtures(t)
	st := newStore(func() time.Time { return time.Unix(0, 0) })

	h := common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000ab")
	// Backend offers a live discount redeemable against tIn with collateral == tOut (the order's output).
	be.discounts = &discountsResponse{Discounts: []discountListItem{{
		DiscountID: h.Hex(), Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(),
		Collateral: tOut.Hex(), CollateralDecimals: 6,
		MaxAssets: "10000000", MaxRate: "1000000000000000000", // 1e7 liquidity, rate 1.0 → 1000000 out ≥ 900000 required
	}}}
	be.discount = &resolveDiscountResponse{
		Discount: discountTerms{
			Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Discount: "500",
			Signer:   "0x00000000000000000000000000000000000000a1",
			Protocol: "0x00000000000000000000000000000000000000a2",
			Nonce:    "0x1", Deadline: 4_102_444_800,
		},
		SignerSignature: "0xaa", ProtocolDeadline: 4_102_444_800, ProtocolSignature: "0xbb",
	}
	txm := &fakeTxm{result: txmanager.Result{Hash: common.HexToHash("0xdead")}}
	e := newExec(t, st, be, txm)
	// No vaults configured (discount-only solver); fill-plan recovery prices via the default
	// strategy's own dependency.
	e.reader = &fakeRecoveryReader{}
	e.strategy = newDefaultTestStrategy(18, map[common.Address]*big.Int{tOut: big.NewInt(500000)})

	e.syncOnce(context.Background())

	if rec := st.order("o1"); rec == nil || rec.Status != statusFilled {
		t.Fatalf("status = %v, want filled (discount-only recovery with empty vaults)", rec)
	}
	if be.resolveCalls != 1 {
		t.Fatalf("resolveDiscount calls = %d, want 1", be.resolveCalls)
	}
	if len(txm.lastData) < 4 {
		t.Fatalf("no fill calldata sent")
	}
}

func TestExecution_DiscountAdapterMismatchFails(t *testing.T) {
	st, be := fillFixtures(t)
	// Strategy quotes a discount leg through vlt, but the backend resolves the discount to a
	// different adapter — the fill must be aborted without a tx.
	h := common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000ab")
	be.discount = &resolveDiscountResponse{
		Discount: discountTerms{
			Adapter:       "0x00000000000000000000000000000000000000aa", // not the quoted leg's adapter
			TokenToRedeem: tIn.Hex(), Discount: "500",
			Signer:   "0x00000000000000000000000000000000000000a1",
			Protocol: "0x00000000000000000000000000000000000000a2",
			Nonce:    "0x1", Deadline: 4_102_444_800,
		},
		SignerSignature: "0xaa", ProtocolDeadline: 4_102_444_800, ProtocolSignature: "0xbb",
	}
	txm := &fakeTxm{result: txmanager.Result{Hash: common.HexToHash("0xdead")}}
	e := newExec(t, st, be, txm)
	e.strategy = fixedFillStrategy{plan: discountFillPlan(h)}

	e.syncOnce(context.Background())

	rec := st.order("o1")
	if rec == nil || rec.Status != statusFailed {
		t.Fatalf("status = %v, want failed", rec)
	}
	if !strings.Contains(rec.LastError, errDiscountAdapterMismatch.Error()) {
		t.Fatalf("lastError = %q, want adapter-mismatch reason", rec.LastError)
	}
	if txm.lastData != nil {
		t.Fatalf("should not have sent a fill for a mismatched discount adapter")
	}

	// The order is still open, so the next poll re-arms it and re-evaluates the discount (it could
	// resolve correctly by then — mirrors the TS filler); a persisting mismatch re-fails with no tx.
	e.syncOnce(context.Background())
	if be.resolveCalls != 2 {
		t.Fatalf("resolveDiscount calls after second cycle = %d, want 2 (re-evaluated)", be.resolveCalls)
	}
	if rec = st.order("o1"); rec == nil || rec.Status != statusFailed {
		t.Fatalf("second cycle status = %v, want failed again", rec)
	}
	if txm.lastData != nil {
		t.Fatalf("second cycle must not send a fill either")
	}
}

func TestExecution_DiscountInventoriesWhitelist(t *testing.T) {
	listedID := "0x00000000000000000000000000000000000000000000000000000000000000a1"
	rogueID := "0x00000000000000000000000000000000000000000000000000000000000000a2"
	rogue := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	be := &fakeBackend{discounts: &discountsResponse{Discounts: []discountListItem{
		{DiscountID: listedID, Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(),
			CollateralDecimals: 6, MaxRate: "1000000000000000000", MaxAssets: "10000000"},
		{DiscountID: rogueID, Adapter: rogue.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(),
			CollateralDecimals: 6, MaxRate: "2000000000000000000", MaxAssets: "10000000"},
	}}}
	st := newStore(func() time.Time { return time.Unix(0, 0) })

	e := newExec(t, st, be, &fakeTxm{})
	e.whitelist = buildAdapterWhitelist(true, []recoveryVault{{Adapter: vlt}})
	out := e.discountInventories(context.Background(), tIn, nil)
	if len(out) != 1 || out[0].Adapter != vlt {
		t.Fatalf("whitelisted discountInventories = %+v, want only the listed adapter", out)
	}

	// Disabled via config (nil whitelist): both discounts survive.
	e.whitelist = buildAdapterWhitelist(false, []recoveryVault{{Adapter: vlt}})
	out = e.discountInventories(context.Background(), tIn, nil)
	if len(out) != 2 {
		t.Fatalf("unfiltered discountInventories = %d entries, want 2", len(out))
	}
}

func TestExecution_MissingFillPlanFails(t *testing.T) {
	_, be := fillFixtures(t)
	st := newStore(func() time.Time { return time.Unix(0, 0) })
	txm := &fakeTxm{result: txmanager.Result{}}
	e := newExec(t, st, be, txm)
	e.strategy = fixedFillStrategy{}

	e.syncOnce(context.Background())

	if rec := st.order("o1"); rec == nil || rec.Status != statusFailed {
		t.Fatalf("status = %v, want failed (missing fill plan)", rec)
	}
	if txm.lastData != nil {
		t.Fatalf("should not have sent a tx without a fill plan")
	}
}

func TestExecutionRecoveryMarksPermissionedScopeAsSingleRoute(t *testing.T) {
	strategy := &inputRecordingStrategy{fillPlan: baseFillPlan()}
	e := newExec(t, newStore(func() time.Time { return time.Unix(0, 0) }), &fakeBackend{}, &fakeTxm{})
	e.discountsEnabled = false
	e.tokensToQuote = tokensToQuotePermissioned
	e.permissionedTokens = map[common.Address]bool{tIn: true}
	e.strategy = strategy

	plan, err := e.buildFillPlan(
		t.Context(), &executable{quoteID: "q1"}, sampleOrder(), tOut, big.NewInt(900000),
	)
	if err != nil {
		t.Fatalf("buildFillPlan: %v", err)
	}
	if plan == nil {
		t.Fatal("buildFillPlan returned nil")
	}
	if !strategy.fillInput.RequireSingleRoute {
		t.Fatal("permissioned fill recovery input did not require a single route")
	}
}

func TestExecutionRejectsPermissionedScopeMultiLegFillPlan(t *testing.T) {
	plan := baseFillPlan()
	plan.Legs = []types.FillLeg{
		{
			Adapter: vlt, AmountIn: big.NewInt(500000000000000000), AmountOut: big.NewInt(450000),
		},
		{
			Adapter:  common.HexToAddress("0x0000000000000000000000000000000000000004"),
			AmountIn: big.NewInt(500000000000000000), AmountOut: big.NewInt(450000),
		},
	}
	e := newExec(t, newStore(func() time.Time { return time.Unix(0, 0) }), &fakeBackend{}, &fakeTxm{})
	e.discountsEnabled = false
	e.tokensToQuote = tokensToQuotePermissioned
	e.permissionedTokens = map[common.Address]bool{tIn: true}
	e.strategy = fixedFillStrategy{plan: plan}

	got, err := e.buildFillPlan(
		t.Context(), &executable{quoteID: "q1"}, sampleOrder(), tOut, big.NewInt(900000),
	)
	if err == nil || !strings.Contains(err.Error(), "single-route input requires exactly one leg") {
		t.Fatalf("buildFillPlan error = %v, want single-route rejection", err)
	}
	if got != nil {
		t.Fatalf("buildFillPlan = %+v, want nil", got)
	}
}
