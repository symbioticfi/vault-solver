package defaultstrategy

import (
	"fmt"
	"math/big"
	"slices"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

// bidSkip is a declined bid: code is the bounded metric label, detail names the failed check and the
// values it compared. The zero value means no skip.
type bidSkip struct {
	code   string
	detail fmt.Stringer
}

func skipf(code, format string, args ...any) bidSkip {
	return bidSkip{code: code, detail: types.Text(fmt.Sprintf(format, args...))}
}

func (k bidSkip) bid() types.BidOutput {
	return types.BidOutput{Decision: types.DecisionSkip, Reason: k.code, Detail: k.detail}
}

func (k bidSkip) String() string {
	if k.detail == nil {
		return k.code
	}
	return k.code + ": " + k.detail.String()
}

// noLegsDetail formats the candidate tally only if the skip is logged or traced.
type noLegsDetail struct{ tally *rejectTally }

func (d noLegsDetail) String() string { return "no liquidatable leg: " + d.tally.String() }

// rejectCheck names one condition that drops a candidate leg; checkNone means the leg passed.
type rejectCheck uint8

const (
	checkNone rejectCheck = iota
	checkPriceNotPositive
	checkMaxRateNotPositive
	checkHealthy
	checkNoCollateral
	checkSeizeZero
	checkZeroLoanOut
	checkProceedsBelowRepaid
	checkNoAuctionPrice
	checkNoQuote
	checkGasLimit
	checkBudget
	checkNoMarketID
	checkSeizeReplay
	numRejectChecks
)

var rejectCheckNames = [numRejectChecks]string{
	checkPriceNotPositive:    "price not positive",
	checkMaxRateNotPositive:  "adapter max rate not positive",
	checkHealthy:             "position healthy at auction price",
	checkNoCollateral:        "no collateral to seize",
	checkSeizeZero:           "seize clamped to zero",
	checkZeroLoanOut:         "seize quotes zero loan out",
	checkProceedsBelowRepaid: "proceeds do not cover repayment",
	checkNoAuctionPrice:      "no auction price for market oracle",
	checkNoQuote:             "adapter has no usable quote for collateral",
	checkGasLimit:            "bundle exceeds settlement gas limit",
	checkBudget:              "adapter collateral budget exceeded",
	checkNoMarketID:          "replay leg has no market id",
	checkSeizeReplay:         "seize replay rejected by Morpho state",
}

func (c rejectCheck) String() string { return rejectCheckNames[c] }

// legRejection is the check that dropped one candidate plus the values it compared. It only points at
// existing amounts, so the hot path (sizing every tracked position, beam-search replay) never allocates
// for it; values formats only when a tally records the first example of a check.
type legRejection struct {
	check rejectCheck
	v     [6]*big.Int // compared amounts, in the order values prints them
	addr  common.Address
	n     int64
	flag  bool
	note  string
}

// healthyValues fills a checkHealthy rejection's amounts from the sizing inputs (sizeLeg leaves them out).
func (r *legRejection) healthyValues(c Candidate, price, accrued *big.Int) {
	m := c.Market.State
	r.v = [6]*big.Int{price, c.Position.Collateral, c.Position.BorrowShares, m.Lltv, accrued, m.TotalBorrowShares}
}

func (r legRejection) values() string {
	v := r.v
	switch r.check {
	case checkPriceNotPositive:
		return fmt.Sprintf("price %v", v[0])
	case checkMaxRateNotPositive:
		return fmt.Sprintf("maxRate %v", v[0])
	case checkHealthy:
		return fmt.Sprintf("price %v, collateral %v, borrowShares %v, lltv %v, totalBorrowAssets %v, totalBorrowShares %v",
			v[0], v[1], v[2], v[3], v[4], v[5])
	case checkNoCollateral:
		return fmt.Sprintf("collateral %v, allowFull %t", v[0], r.flag)
	case checkSeizeZero:
		return fmt.Sprintf("collateral %v, full-debt max seize %v, adapter maxAssets %v", v[0], v[1], v[2])
	case checkZeroLoanOut:
		return fmt.Sprintf("seize %v, maxRate %v, haircutBps %d", v[0], v[1], r.n)
	case checkProceedsBelowRepaid:
		return fmt.Sprintf("expected loan out %v <= repaid %v for seize %v", v[0], v[1], v[2])
	case checkNoAuctionPrice:
		return fmt.Sprintf("oracle %s, positions %d", r.addr.Hex(), r.n)
	case checkNoQuote:
		return fmt.Sprintf("collateral %s, positions %d: %s", r.addr.Hex(), r.n, r.note)
	case checkGasLimit:
		return fmt.Sprintf("usable gas limit %d", r.n)
	case checkBudget:
		return fmt.Sprintf("consumed %v + leg loan out %v > maxAssets %v", orZero(v[0]), v[1], v[2])
	case checkNoMarketID:
		return "borrower " + r.addr.Hex()
	case checkSeizeReplay:
		return fmt.Sprintf("seize %v at price %v", v[0], v[1])
	case checkNone, numRejectChecks:
	}
	return ""
}

func (r legRejection) String() string {
	return r.check.String() + ": " + r.values()
}

// rejectTally counts candidate rejections per check and keeps the first example of each, so a no_legs
// skip names every check that dropped a position in one bounded line.
type rejectTally struct {
	counts  [numRejectChecks]int
	first   [numRejectChecks]firstRejection
	adapter types.AdapterSnapshot // explains checkNoQuote when printed
}

// firstRejection is kept unformatted; its amounts are immutable snapshot or freshly computed values.
type firstRejection struct {
	market   common.Hash
	borrower common.Address
	rej      legRejection
}

// needsExample reports whether the next rejection of c will be printed, so callers can skip building
// costly detail (like quoteGap) for the rest.
func (t *rejectTally) needsExample(c rejectCheck) bool {
	return t.counts[c] == 0
}

// add records r against market (and borrower, unless zero). It stays small enough to inline: it runs for
// every rejected position.
func (t *rejectTally) add(market common.Hash, borrower common.Address, r *legRejection) {
	t.counts[r.check]++
	if t.counts[r.check] == 1 {
		t.recordFirst(market, borrower, r)
	}
}

func (t *rejectTally) recordFirst(market common.Hash, borrower common.Address, r *legRejection) {
	t.first[r.check] = firstRejection{market: market, borrower: borrower, rej: *r}
}

func (t *rejectTally) example(c rejectCheck) string {
	f := t.first[c]
	if c == checkNoQuote {
		f.rej.note = quoteGap(t.adapter, f.rej.addr)
	}
	subject := "market " + f.market.Hex()
	if f.borrower != (common.Address{}) {
		subject += " borrower " + f.borrower.Hex()
	}
	return subject + ": " + f.rej.values()
}

func (t *rejectTally) String() string {
	var (
		checks []rejectCheck
		total  int
	)
	for c := range numRejectChecks {
		if t.counts[c] > 0 {
			checks = append(checks, c)
			total += t.counts[c]
		}
	}
	if total == 0 {
		return "no tracked positions"
	}
	slices.SortFunc(checks, func(a, b rejectCheck) int { return strings.Compare(a.String(), b.String()) })
	parts := make([]string, 0, len(checks))
	for _, c := range checks {
		parts = append(parts, fmt.Sprintf("%dx %s (first %s)", t.counts[c], c, t.example(c)))
	}
	return fmt.Sprintf("%d rejections: %s", total, strings.Join(parts, "; "))
}
