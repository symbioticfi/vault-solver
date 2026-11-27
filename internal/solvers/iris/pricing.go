package iris

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// RateRule prices the markets it matches; nil fields match anything.
type RateRule struct {
	CollateralToken, DebtToken *common.Address
	VenueID                    *uint64
	FixedRate                  *big.Int
}

// Pricing is the v1 rate table: the most specific matching rule wins, earlier rules break
// ties, and Default prices the rest. A nil result means the market is not quoted.
type Pricing struct {
	Default *big.Int
	Rules   []RateRule
	// MaxBond caps the bond of one quote in debt-token units; nil means no extra cap.
	MaxBond *big.Int
}

func (p Pricing) rate(m Market) *big.Int {
	best, bestScore := p.Default, -1
	for _, rule := range p.Rules {
		score := 0
		if rule.CollateralToken != nil {
			if *rule.CollateralToken != m.CollateralToken {
				continue
			}
			score++
		}
		if rule.DebtToken != nil {
			if *rule.DebtToken != m.DebtToken {
				continue
			}
			score++
		}
		if rule.VenueID != nil {
			if *rule.VenueID != m.VenueID {
				continue
			}
			score++
		}
		if score > bestScore {
			best, bestScore = rule.FixedRate, score
		}
	}
	return best
}

// schedule places the signature in the first slot the transaction can make and dates the
// deadline from that slot, so the quote reaches the coordinator with about quoteTtl left.
func (t Timing) schedule(now time.Time, headTimestamp uint64) (inclusion, deadline uint64) {
	step := uint64(t.BlockTime / time.Second)
	// Round up: truncating to whole seconds would shorten the lead by up to a second.
	earliest := uint64(now.Add(t.SubmitLead).Add(time.Second - 1).Unix())
	inclusion = headTimestamp + step
	if earliest > inclusion {
		inclusion += (earliest - inclusion + step - 1) / step * step
	}
	return inclusion, inclusion + uint64((t.QuoteTTL+t.ResponseSlack)/time.Second)
}

// respondAt is the first moment the coordinator, receiving the response responseSlack
// later, accepts the deadline. A signature mined before its target slot waits until then.
func (t Timing) respondAt(deadline uint64) time.Time {
	return time.Unix(int64(deadline), 0).Add(-t.QuoteTTL - t.ResponseSlack)
}

// fresh reports whether the coordinator accepts the deadline of a response sent now: it must
// lie within the final deadlineWindow of the quote TTL measured from arrival, responseSlack later.
func (t Timing) fresh(now time.Time, deadline uint64) bool {
	latest := now.Add(t.ResponseSlack + t.QuoteTTL).Unix()
	return int64(deadline) <= latest && int64(deadline) >= latest-int64(t.DeadlineWindow/time.Second)
}
