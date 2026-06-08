package rfq

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type fakeBackend struct {
	open         []backendOrder
	executable   *backendOrder
	order        *backendOrder
	discount     *resolveDiscountResponse
	discounts    *discountsResponse
	resolveCalls int
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
	if f.discounts == nil {
		return &discountsResponse{}, nil
	}
	return f.discounts, nil
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

func newExec(t *testing.T, st *store, be orderBackend, txm txSender) *executionService {
	t.Helper()
	return &executionService{
		chainID: 1, executor: common.HexToAddress("0x0000000000000000000000000000000000000010"),
		orderLimit: 20, backend: be, store: st, txm: txm,
		log: logr.Discard(), now: func() time.Time { return time.Unix(0, 0) },
		inflight: make(map[string]bool),
	}
}

// seededStrategy + a backend order whose payload matches sampleOrder() from order_test.go.
func fillFixtures(t *testing.T) (*store, *fakeBackend) {
	t.Helper()
	st := newStore(func() time.Time { return time.Unix(0, 0) })
	st.putStrategy(&strategyRecord{
		QuoteID: "q1", TokenIn: tIn, TokenOut: tOut, Asset: tOut, AssetDecimals: 6,
		AmountIn: big.NewInt(1_000000000000000000), QuotedAmountOut: big.NewInt(900000), AssetAmountOut: big.NewInt(900000),
		Legs: []strategyLeg{{Adapter: vlt, AmountIn: big.NewInt(1_000000000000000000), AmountOut: big.NewInt(900000)}},
	})
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
	// Replace the cached strategy with a discount leg, and have the backend resolve the discount.
	h := common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000ab")
	st.putStrategy(&strategyRecord{
		QuoteID: "q1", TokenIn: tIn, TokenOut: tOut, Asset: tOut, AssetDecimals: 6,
		AmountIn: big.NewInt(1_000000000000000000), QuotedAmountOut: big.NewInt(900000), AssetAmountOut: big.NewInt(900000),
		Legs: []strategyLeg{{Adapter: vlt, AmountIn: big.NewInt(1_000000000000000000), AmountOut: big.NewInt(900000), MaxRate: big.NewInt(1), DiscountID: &h}},
	})
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

func TestExecution_MissingStrategyFails(t *testing.T) {
	_, be := fillFixtures(t)
	st := newStore(func() time.Time { return time.Unix(0, 0) }) // empty store: no cached strategy, no vaults
	txm := &fakeTxm{result: txmanager.Result{}}
	e := newExec(t, st, be, txm)

	e.syncOnce(context.Background())

	if rec := st.order("o1"); rec == nil || rec.Status != statusFailed {
		t.Fatalf("status = %v, want failed (missing strategy)", rec)
	}
	if txm.lastData != nil {
		t.Fatalf("should not have sent a tx without a strategy")
	}
}
