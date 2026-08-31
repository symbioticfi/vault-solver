package rfq

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// External-solver path (discountsEnabled false, the default): the solver never touches the discounts API.
// The internal path is covered by the TestExecution_Discount* tests via newExec.

// External fill planning never calls GET /discounts; with no vaults + discounts off there's no inventory,
// so the order fails.
func TestExecution_DiscountsDisabled_RecoverySkipsListDiscounts(t *testing.T) {
	_, be := fillFixtures(t)
	st := newStore(func() time.Time { return time.Unix(0, 0) }) // empty store → forces recovery
	be.discounts = &discounts.List{Discounts: []discounts.ListItem{{
		DiscountID: "0x00000000000000000000000000000000000000000000000000000000000000ab",
		Adapter:    vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), CollateralDecimals: 6,
		Discount: "500", MaxAssets: "10000000", MaxRate: "1000000000000000000",
	}}}
	txm := &fakeTxm{result: txmanager.Result{Hash: common.HexToHash("0xdead")}}
	e := newExec(t, st, be, txm)
	e.discountsEnabled = false // external solver; no vaults configured
	e.strategy = fixedFillStrategy{}

	e.syncOnce(context.Background())

	if rec := testOrder(st); rec == nil || rec.Status != statusFailed {
		t.Fatalf("status = %v, want failed (no inventory: no vaults, discounts disabled)", rec)
	}
	if be.listCalls != 0 {
		t.Fatalf("listDiscounts calls = %d, want 0 (external solver must not call the discounts API)", be.listCalls)
	}
	if be.resolveCalls != 0 {
		t.Fatalf("resolveDiscount calls = %d, want 0", be.resolveCalls)
	}
	if txm.lastReq.Data != nil {
		t.Fatal("should not have sent a fill")
	}
}

// A discount leg with discounts off fails closed (terminal, no tx) and never calls POST /discounts.
func TestExecution_DiscountsDisabled_FillFailsClosed(t *testing.T) {
	st, be := fillFixtures(t)
	h := common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txm := &fakeTxm{result: txmanager.Result{Hash: common.HexToHash("0xdead")}}
	e := newExec(t, st, be, txm)
	e.discountsEnabled = false
	e.strategy = fixedFillStrategy{plan: discountFillPlan(h)}

	e.syncOnce(context.Background())

	rec := testOrder(st)
	if rec == nil || rec.Status != statusFailed {
		t.Fatalf("status = %v, want failed", rec)
	}
	if be.resolveCalls != 0 {
		t.Fatalf("resolveDiscount calls = %d, want 0 (must not call the discounts API)", be.resolveCalls)
	}
	if txm.lastReq.Data != nil {
		t.Fatal("should not have sent a fill for a disabled discount leg")
	}
}
