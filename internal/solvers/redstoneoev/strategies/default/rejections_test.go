package defaultstrategy

import (
	"fmt"
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
			_, rej := sizeLegForTest(cand, tc.price, tc.quote, accrued, full)
			if rej.check.String() != tc.want {
				t.Fatalf("rejection = %v, want check %q", rej, tc.want)
			}
		})
	}
}

func TestRejectTallyCountsByCheckWithFirstExample(t *testing.T) {
	if got := new(rejectTally).String(); got != "no tracked positions" {
		t.Fatalf("empty tally = %q", got)
	}

	var tally rejectTally
	b1, b2 := common.Address{1}, common.Address{2}
	tally.add(common.Hash{1}, b1, &legRejection{check: checkPriceNotPositive, v: [6]*big.Int{big.NewInt(-7)}})
	tally.add(common.Hash{1}, b2, &legRejection{check: checkPriceNotPositive, v: [6]*big.Int{big.NewInt(-8)}})
	if !tally.needsExample(checkNoQuote) || tally.needsExample(checkPriceNotPositive) {
		t.Fatal("needsExample must be true only before a check's first rejection")
	}
	tally.adapter = types.AdapterSnapshot{Address: common.Address{0xa}, Paused: true}
	tally.add(common.Hash{2}, common.Address{}, &legRejection{check: checkNoQuote, addr: common.Address{3}, n: 3})
	got := tally.String()
	want := "3 rejections: 1x adapter has no usable quote for collateral (first market " + common.Hash{2}.Hex() +
		": collateral " + common.Address{3}.Hex() + ", positions 3: adapter is paused); 2x price not positive (first market " +
		common.Hash{1}.Hex() + " borrower " + b1.Hex() + ": price -7)"
	if got != want {
		t.Fatalf("tally = %q\nwant    %q", got, want)
	}
}

// Rejection text is what operators grep for; pin every check's wording.
func TestLegRejectionText(t *testing.T) {
	one, two, three := big.NewInt(1), big.NewInt(2), big.NewInt(3)
	tests := []struct {
		rej  legRejection
		want string
	}{
		{legRejection{check: checkPriceNotPositive}, "price not positive: price <nil>"},
		{legRejection{check: checkMaxRateNotPositive, v: [6]*big.Int{one}}, "adapter max rate not positive: maxRate 1"},
		{legRejection{check: checkHealthy, v: [6]*big.Int{one, two, three, one, two, three}},
			"position healthy at auction price: price 1, collateral 2, borrowShares 3, lltv 1, totalBorrowAssets 2, totalBorrowShares 3"},
		{legRejection{check: checkNoCollateral, v: [6]*big.Int{one}, flag: true}, "no collateral to seize: collateral 1, allowFull true"},
		{legRejection{check: checkSeizeZero, v: [6]*big.Int{one, two}},
			"seize clamped to zero: collateral 1, full-debt max seize 2, adapter maxAssets <nil>"},
		{legRejection{check: checkZeroLoanOut, v: [6]*big.Int{one, two}, n: 50}, "seize quotes zero loan out: seize 1, maxRate 2, haircutBps 50"},
		{legRejection{check: checkProceedsBelowRepaid, v: [6]*big.Int{one, two, three}},
			"proceeds do not cover repayment: expected loan out 1 <= repaid 2 for seize 3"},
		{legRejection{check: checkNoAuctionPrice, addr: common.Address{1}, n: 4},
			"no auction price for market oracle: oracle " + common.Address{1}.Hex() + ", positions 4"},
		{legRejection{check: checkGasLimit, n: 9}, "bundle exceeds settlement gas limit: usable gas limit 9"},
		{legRejection{check: checkBudget, v: [6]*big.Int{nil, two, three}},
			"adapter collateral budget exceeded: consumed 0 + leg loan out 2 > maxAssets 3"},
		{legRejection{check: checkNoMarketID, addr: common.Address{1}}, "replay leg has no market id: borrower " + common.Address{1}.Hex()},
		{legRejection{check: checkSeizeReplay, v: [6]*big.Int{one, two}}, "seize replay rejected by Morpho state: seize 1 at price 2"},
	}
	for _, tc := range tests {
		if got := tc.rej.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

// Rejecting a healthy position, the common case, must not allocate.
func TestSizeLegHealthyRejectionDoesNotAllocate(t *testing.T) {
	m := goldenMarket()
	cand := Candidate{Market: MarketInfo{State: m}, Position: goldenBorrower()}
	accrued := morpho.AccruedTotalBorrowAssets(m, m.LastUpdate)
	price, q := mustBig("2000000000000000000000000000"), newQuote("1534500000000000000000", nil)
	base := testing.AllocsPerRun(100, func() { morpho.IsLiquidatableAt(cand.Position, price, m.Lltv, accrued, m.TotalBorrowShares) })
	got := testing.AllocsPerRun(100, func() {
		if _, rej := sizeLegForTest(cand, price, q, accrued, SizingParams{}); rej.check != checkHealthy {
			t.Fatalf("rejection = %v", rej)
		}
	})
	if got > base {
		t.Fatalf("sizeLeg healthy rejection allocates %.0f, liquidatability check alone %.0f", got, base)
	}
}

func TestSelectBundleNamesGasLimitShortfall(t *testing.T) {
	_, skip := testBundleEngine(Config{}).selectBundleWithGas(
		[]scoredLeg{scoredFor(1, big.NewInt(1))}, nil, 1, defaultPriceUpdateFeeds)
	if skip.code != skipNoLegs || !strings.Contains(detailOf(skip.detail), "usable gas limit 0 is below") {
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
			if skip.code != tc.code || !strings.Contains(detailOf(skip.detail), tc.detail) {
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

func sizeLegForTest(c Candidate, price *big.Int, q AdapterQuote, accrued *big.Int, sp SizingParams) (sizedLeg, legRejection) {
	var why legRejection
	sized, _ := sizeLeg(c, price, q, accrued, sp, &why)
	return sized, why
}

func detailOf(d fmt.Stringer) string {
	if d == nil {
		return ""
	}
	return d.String()
}
