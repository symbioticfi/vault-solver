package defaultstrategy

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

// bidSkip is a declined bid: code is the bounded metric label, detail names the failed check and the
// values it compared. The zero value means no skip.
type bidSkip struct {
	code, detail string
}

func skipf(code, format string, args ...any) bidSkip {
	return bidSkip{code: code, detail: fmt.Sprintf(format, args...)}
}

func (k bidSkip) bid() types.BidOutput {
	return types.BidOutput{Decision: types.DecisionSkip, Reason: k.code, Detail: k.detail}
}

func (k bidSkip) String() string {
	return k.code + ": " + k.detail
}

// legRejection is the check that dropped one candidate. Formatting is deferred to String because bundle
// replay re-sizes legs on the hot path and only the first rejection per check is ever printed.
type legRejection struct {
	check  string // fixed phrase, the tally key
	format string
	args   []any
}

func reject(check, format string, args ...any) *legRejection {
	return &legRejection{check: check, format: format, args: args}
}

func (r *legRejection) values() string {
	return fmt.Sprintf(r.format, r.args...)
}

func (r *legRejection) String() string {
	return r.check + ": " + r.values()
}

// rejectTally counts candidate rejections per check and keeps the first example of each, so a no_legs
// skip names every check that dropped a position in one bounded line.
type rejectTally struct {
	total  int
	counts map[string]int
	first  map[string]string
}

func (t *rejectTally) add(market common.Hash, borrower *common.Address, r *legRejection) {
	if t == nil {
		return
	}
	if t.counts == nil {
		t.counts = make(map[string]int)
		t.first = make(map[string]string)
	}
	t.total++
	t.counts[r.check]++
	if _, ok := t.first[r.check]; ok {
		return
	}
	subject := "market " + market.Hex()
	if borrower != nil {
		subject += " borrower " + borrower.Hex()
	}
	t.first[r.check] = subject + ": " + r.values()
}

func (t *rejectTally) String() string {
	if t == nil || t.total == 0 {
		return "no tracked positions"
	}
	checks := slices.Sorted(maps.Keys(t.counts))
	parts := make([]string, 0, len(checks))
	for _, c := range checks {
		parts = append(parts, fmt.Sprintf("%dx %s (first %s)", t.counts[c], c, t.first[c]))
	}
	return fmt.Sprintf("%d rejections: %s", t.total, strings.Join(parts, "; "))
}
