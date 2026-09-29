package bridgefacilitator

import (
	"context"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// redeemBackoffPolls is how many redeem polls (intervals.redeemPoll) a redeem waits after each refusal of
// an episode for an unaffordable batch, the last repeating: 5, 10, 20 and then 60 minutes at the default
// 5-minute poll (strategy §2.10). It is a shape over the configured cadence rather than a duration of its
// own, so it scales with the poll and counts whole passes instead of racing their ticks.
var redeemBackoffPolls = [...]int{1, 2, 4, 12}

// redeemBackoff throttles redeem while the signer balance cannot fund a finalize batch. Redeem has no
// deadline and recovers fronted liquidity, so it runs regardless of the funding gate and lets the balance
// guard decide per attempt; on a refusal it halves the batch (down to 1) and waits out the schedule
// above, instead of asking the guard again every poll. A lane-state signal that finds the lane fundable
// again after it was not ends the episode, and each successful redeem doubles the batch back toward the
// configured size.
//
// Owned by the Run goroutine: redeemAll, redeemReady and the lane-state handler all run on it.
type redeemBackoff struct {
	refusals   int  // unaffordable refusals since the episode's last successful redeem
	skipPasses int  // redeem passes still to skip before the next send
	holding    bool // the current pass sends nothing
	batchLimit int  // halved batch size; 0 while the configured size applies
	// fundable is the funding gate as the last lane-state signal saw it, so only its reopening (not the
	// other lane edges every send produces) resets the episode.
	fundable bool
}

// active reports an episode in progress: a refusal not yet followed by a successful redeem, or a batch
// size not yet doubled back to the configured one.
func (b *redeemBackoff) active() bool {
	return b.refusals > 0 || b.batchLimit > 0
}

// beginPass starts a redeem pass and reports whether it may send.
func (b *redeemBackoff) beginPass() bool {
	b.holding = b.skipPasses > 0
	if b.holding {
		b.skipPasses--
	}
	return !b.holding
}

// limit is the batch size a send may use.
func (b *redeemBackoff) limit(configured int) int {
	if b.batchLimit > 0 && b.batchLimit < configured {
		return b.batchLimit
	}
	return configured
}

// refused records a batch of sent requests the balance guard refused and reports whether it opened the
// episode. The rest of the pass and the scheduled passes after it send nothing.
func (b *redeemBackoff) refused(sent int) bool {
	first := !b.active()
	b.refusals++
	b.batchLimit = max(1, sent/2)
	b.skipPasses = redeemBackoffPolls[min(b.refusals, len(redeemBackoffPolls))-1] - 1
	b.holding = true
	return first
}

// succeeded records an included redeem: the wait ends and the batch doubles back toward configured.
func (b *redeemBackoff) succeeded(configured int) {
	b.refusals, b.skipPasses = 0, 0
	if b.batchLimit > 0 {
		b.batchLimit *= 2
		if b.batchLimit >= configured {
			b.batchLimit = 0
		}
	}
}

// reset ends the episode, keeping the last seen funding state.
func (b *redeemBackoff) reset() {
	*b = redeemBackoff{fundable: b.fundable}
}

// laneFundable reads the shared lane's funding gate; an unwired gate reads as funded, which leaves the
// backoff to its schedule.
func (s *Solver) laneFundable() bool {
	return s.fundable == nil || s.fundable()
}

// onLaneStateChange handles one coalesced lane-state signal: the funding gate reopening (the signer was
// funded, or the base fee fell back) ends a redeem backoff so the next pass sends a full batch.
func (s *Solver) onLaneStateChange(ctx context.Context) {
	fundable := s.laneFundable()
	reopened := fundable && !s.redeem.fundable
	s.redeem.fundable = fundable
	if !reopened || !s.redeem.active() {
		return
	}
	s.redeem.reset()
	observability.Log(ctx).V(1).Info("redeem backoff reset: the lane is fundable again")
}

// subscribeLaneState subscribes to the shared lane's state changes; an unwired manager yields a nil
// channel, which never fires.
func (s *Solver) subscribeLaneState() (<-chan struct{}, func()) {
	if s.laneStates == nil {
		return nil, func() {}
	}
	return s.laneStates()
}
