package rfq

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// External-solver path (discountsEnabled false, the default): the solver never touches the discounts API.
// The internal path is covered by the TestExecution_Discount* tests via newExec.

// External recovery never calls GET /discounts; with no vaults + discounts off there's no inventory to
// rebuild from, so the order fails.
func TestExecution_DiscountsDisabled_RecoverySkipsListDiscounts(t *testing.T) {
	_, be := fillFixtures(t)
	st := newStore(func() time.Time { return time.Unix(0, 0) }) // empty store → forces recovery
	be.discounts = &discountsResponse{Discounts: []discountListItem{{
		DiscountID: "0x00000000000000000000000000000000000000000000000000000000000000ab",
		Adapter:    vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), CollateralDecimals: 6,
		MaxAssets: "10000000", MaxRate: "1000000000000000000",
	}}}
	txm := &fakeTxm{result: txmanager.Result{Hash: common.HexToHash("0xdead")}}
	e := newExec(t, st, be, txm)
	e.discountsEnabled = false // external solver; no vaults configured

	e.syncOnce(context.Background())

	if rec := st.order("o1"); rec == nil || rec.Status != statusFailed {
		t.Fatalf("status = %v, want failed (no inventory: no vaults, discounts disabled)", rec)
	}
	if be.listCalls != 0 {
		t.Fatalf("listDiscounts calls = %d, want 0 (external solver must not call the discounts API)", be.listCalls)
	}
	if be.resolveCalls != 0 {
		t.Fatalf("resolveDiscount calls = %d, want 0", be.resolveCalls)
	}
	if txm.lastData != nil {
		t.Fatal("should not have sent a fill")
	}
}

// A cached discount leg with discounts off fails closed (terminal, no tx) and never calls POST /discounts.
func TestExecution_DiscountsDisabled_FillFailsClosed(t *testing.T) {
	st, be := fillFixtures(t)
	h := common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000ab")
	st.putStrategy(&strategyRecord{
		QuoteID: "q1", TokenIn: tIn, TokenOut: tOut, Asset: tOut, AssetDecimals: 6,
		AmountIn: big.NewInt(1_000000000000000000), QuotedAmountOut: big.NewInt(900000), AssetAmountOut: big.NewInt(900000),
		Legs:      []strategyLeg{{Adapter: vlt, AmountIn: big.NewInt(1_000000000000000000), AmountOut: big.NewInt(900000), MaxRate: big.NewInt(1), DiscountID: &h}},
		CreatedAt: time.Unix(0, 0),
	})
	txm := &fakeTxm{result: txmanager.Result{Hash: common.HexToHash("0xdead")}}
	e := newExec(t, st, be, txm)
	e.discountsEnabled = false

	e.syncOnce(context.Background())

	rec := st.order("o1")
	if rec == nil || rec.Status != statusFailed {
		t.Fatalf("status = %v, want failed", rec)
	}
	if be.resolveCalls != 0 {
		t.Fatalf("resolveDiscount calls = %d, want 0 (must not call the discounts API)", be.resolveCalls)
	}
	if txm.lastData != nil {
		t.Fatal("should not have sent a fill for a disabled discount leg")
	}
}
