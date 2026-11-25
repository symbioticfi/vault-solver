package defaultstrategy

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func TestSizeLegNamesTheFailedCheck(t *testing.T) {
	m := goldenMarket()
	cand := Candidate{Market: MarketInfo{State: m}, Position: goldenBorrower()}
	accrued := morpho.AccruedTotalBorrowAssets(m, m.LastUpdate)
	full := SizingParams{AllowFullLiquidation: true}
	tests := []struct {
		name  string
		price *big.Int
		quote AdapterQuote
		want  string
	}{
		{"zero price", big.NewInt(0), newQuote("1534500000000000000000", nil), "price not positive"},
		{"zero adapter rate", mustBig("1550000000000000000000000000"), newQuote("0", nil), "adapter max rate not positive"},
		{"healthy position", mustBig("2000000000000000000000000000"), newQuote("1534500000000000000000", nil), "position healthy at auction price"},
		{"exit below repayment", mustBig("1550000000000000000000000000"), newQuote("1400000000000000000000", nil), "proceeds do not cover repayment"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, rej := sizeLeg(cand, tc.price, tc.quote, accrued, full)
			if rej == nil || rej.check != tc.want {
				t.Fatalf("rejection = %v, want check %q", rej, tc.want)
			}
		})
	}
}

func TestRejectTallyCountsByCheckWithFirstExample(t *testing.T) {
	var nilTally *rejectTally
	nilTally.add(common.Hash{1}, nil, reject("ignored", "")) // nil-safe
	if got := nilTally.String(); got != "no tracked positions" {
		t.Fatalf("nil tally = %q", got)
	}

	var tally rejectTally
	b1, b2 := common.Address{1}, common.Address{2}
	tally.add(common.Hash{1}, &b1, reject("position healthy at auction price", "price %d", 7))
	tally.add(common.Hash{1}, &b2, reject("position healthy at auction price", "price %d", 8))
	tally.add(common.Hash{2}, nil, reject("adapter has no usable quote for collateral", "positions %d", 3))
	got := tally.String()
	want := "3 rejections: 1x adapter has no usable quote for collateral (first market " + common.Hash{2}.Hex() +
		": positions 3); 2x position healthy at auction price (first market " + common.Hash{1}.Hex() +
		" borrower " + b1.Hex() + ": price 7)"
	if got != want {
		t.Fatalf("tally = %q\nwant    %q", got, want)
	}
}

func TestSelectBundleNamesGasLimitShortfall(t *testing.T) {
	_, skip := testBundleEngine(Config{}).selectBundleWithGas(
		[]scoredLeg{scoredFor(1, big.NewInt(1))}, nil, 1, defaultPriceUpdateFeeds)
	if skip.code != skipNoLegs || !strings.Contains(skip.detail, "usable gas limit 0 is below") {
		t.Fatalf("skip = %v, want no_legs naming the gas limit", skip)
	}
}

type fixedSnapshotMonitor struct {
	monitorSource

	snap *snapshot
}

func (m fixedSnapshotMonitor) snapshot() *snapshot { return m.snap }

func TestPreconditionSkipNamesTheFailedCheck(t *testing.T) {
	now := time.Unix(1_000, 0)
	adapter, callback, other := common.Address{0xa}, common.Address{0xc}, common.Address{0xe}
	fresh := &snapshot{updatedAt: now}
	ok := types.BidInput{
		Now:     now,
		Adapter: types.AdapterSnapshot{Address: adapter, Filler: true},
		Context: types.BidContext{Callback: callback},
	}
	tests := []struct {
		name       string
		edit       func(*types.BidInput)
		snap       *snapshot
		callbackAt time.Time
		code       string
		detail     string
	}{
		{"adapter mismatch", func(in *types.BidInput) { in.Adapter.Address = other }, fresh, now, skipNoLegs, "is not the configured adapter"},
		{"callback mismatch", func(in *types.BidInput) { in.Context.Callback = other }, fresh, now, skipNoLegs, "is not the configured callback"},
		{"not a filler", func(in *types.BidInput) { in.Adapter.Filler = false }, fresh, now, skipNoLegs, "not an authorized filler"},
		{"paused", func(in *types.BidInput) { in.Adapter.Paused = true }, fresh, now, skipNoLegs, "is paused"},
		{"snapshot missing", nil, nil, now, skipStaleState, "position snapshot not loaded yet"},
		{"snapshot old", nil, &snapshot{updatedAt: now.Add(-time.Minute)}, now, skipStaleState, "position snapshot age 1m0s exceeds max age 10s"},
		{"callback missing", nil, fresh, time.Time{}, skipStaleState, "callback balance not loaded yet"},
		{"callback old", nil, fresh, now.Add(-time.Minute), skipStaleState, "callback balance age 1m0s exceeds max age 10s"},
		{"fresh", nil, fresh, now, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &Strategy{adapter: adapter, callback: callback, maxAge: 10 * time.Second, mon: fixedSnapshotMonitor{snap: tc.snap}}
			s.state.store(decisionState{CallbackUpdatedAt: tc.callbackAt})
			in := ok
			if tc.edit != nil {
				tc.edit(&in)
			}
			_, skip := s.preconditionSkip(in)
			if skip.code != tc.code || !strings.Contains(skip.detail, tc.detail) {
				t.Fatalf("skip = %v, want code %q with detail containing %q", skip, tc.code, tc.detail)
			}
		})
	}
}

func TestQuoteGapNamesWhyNoQuote(t *testing.T) {
	coll := common.Address{0xc}
	adapter := types.AdapterSnapshot{Address: common.Address{0xa}, LoanDecimals: 6}
	tests := []struct {
		name    string
		adapter types.AdapterSnapshot
		want    string
	}{
		{"no adapter", types.AdapterSnapshot{}, "no cached quote for the market"},
		{"paused", types.AdapterSnapshot{Address: adapter.Address, Paused: true}, "adapter is paused"},
		{"bad loan decimals", types.AdapterSnapshot{Address: adapter.Address, LoanDecimals: -1}, "adapter loan decimals -1 are invalid"},
		{"not redeemable", adapter, "collateral is not redeemable by the adapter"},
		{"zero max assets", types.AdapterSnapshot{Address: adapter.Address, Redeemable: []types.RedeemableSnapshot{
			{Asset: coll, Decimals: 18, MaxRate: big.NewInt(1), MaxAssets: big.NewInt(0)},
		}}, "got decimals 18, maxRate 1, maxAssets 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := quoteGap(tc.adapter, coll); !strings.Contains(got, tc.want) {
				t.Fatalf("quoteGap = %q, want %q", got, tc.want)
			}
		})
	}
}
