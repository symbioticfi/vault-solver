package rfq

import (
	"testing"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func testSpend() fillSpend {
	return fillSpend{CapacityID: liquidlane.NewCapacityID(1, vlt, tOut), IncludedAt: 8, Deadline: 100}
}

func TestCanonicalFillRecordsSpendBeforeQuotesResume(t *testing.T) {
	for _, outcome := range []string{"pending", "confirmed", "reverted", "unknown", "invalid receipt"} {
		t.Run(outcome, func(t *testing.T) {
			ledger := newSpendLedger()
			ledger.setActive(true)
			sender := &fakeSender{busy: true}
			switch outcome {
			case "confirmed":
				sender.result = fillResult()
			case "reverted":
				sender.result = fillResult()
				sender.result.Reverted = true
			case "unknown":
				sender.pollErr = errors.New("receipt RPC unavailable")
			case "invalid receipt":
				sender.result = fillResult()
				sender.result.Execution.BlockNumber = nil
			}
			s := &Solver{sender: sender, spends: ledger, intent: &fillIntent{OrderID: "award", Spend: testSpend()}}
			err := s.poll(t.Context())
			if (err != nil) != (outcome == "unknown" || outcome == "invalid receipt") {
				t.Fatal(err)
			}
			pending := outcome == "pending" || outcome == "unknown" || outcome == "invalid receipt"
			if (s.intent != nil) != pending || ledger.finalized("award") != (outcome == "confirmed") {
				t.Fatal("fill accounting or ownership changed incorrectly")
			}
			for _, block := range []uint64{0, 7, 8, 999, 7} {
				inventory := []liquidlane.Inventory{{Route: liquidlane.Route{CapacityID: testSpend().CapacityID}, BlockNumber: block}}
				want := !pending && (outcome != "confirmed" || block >= 8)
				if s.AcceptsSnapshots(inventory) != want {
					t.Fatalf("block %d: snapshot acceptance = %v, want %v", block, s.AcceptsSnapshots(inventory), want)
				}
			}
		})
	}
}

func TestSpendExpiryRetainsInclusionWatermark(t *testing.T) {
	ledger := newSpendLedger()
	if err := ledger.include("award", testSpend()); err != nil {
		t.Fatal(err)
	}
	ledger.expire(99)
	if !ledger.finalized("award") {
		t.Fatal("completed award expired before signed deadline")
	}
	ledger.expire(100)
	if ledger.finalized("award") || ledger.watermarks[testSpend().CapacityID] != 8 {
		t.Fatal("signed deadline cleanup lost inclusion watermark")
	}
	older := testSpend()
	older.IncludedAt = 7
	if err := ledger.include("other", older); err != nil || ledger.watermarks[older.CapacityID] != 8 {
		t.Fatal("older fill moved watermark backwards", err)
	}
}
