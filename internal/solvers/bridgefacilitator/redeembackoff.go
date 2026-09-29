package bridgefacilitator

import (
	"context"
	"math/big"

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
// above, instead of asking the guard again every poll. The signer being funded ends the episode (strategy
// §2.10, balance up): the signer balance the manager read last rising above the one it had read when the
// last refusal returned, checked at the start of every redeem pass and on every lane-state signal, or a
// lane-state signal finding the funding gate reopened after it was closed. The balance rule is what works
// for 3F, whose balance.referenceGasUnits stays 0 until measured, which keeps the gate off and Fundable
// always true. Each successful redeem doubles the batch back toward the configured size.
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
	// refusedBalance is the signer balance the manager had read when the last refusal returned, normally
	// the balance that refusal was priced against; nil when it had read none.
	refusedBalance *big.Int
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

// refused records a batch of sent requests the balance guard refused, with the signer balance the manager
// read last, and reports whether it opened the episode. The rest of the pass and the scheduled passes
// after it send nothing.
func (b *redeemBackoff) refused(sent int, balance *big.Int) bool {
	first := !b.active()
	b.refusals++
	b.batchLimit = max(1, sent/2)
	b.skipPasses = redeemBackoffPolls[min(b.refusals, len(redeemBackoffPolls))-1] - 1
	b.holding = true
	b.refusedBalance = balance
	return first
}

// toppedUp reports a signer balance above the one the last refusal saw: the signer was funded since. A
// read from a lagging upstream can overstate it; that costs one more refused (never signed) attempt.
func (b *redeemBackoff) toppedUp(balance *big.Int) bool {
	return b.active() && b.refusedBalance != nil && balance != nil && balance.Cmp(b.refusedBalance) > 0
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

// currentSignerBalance reads the shared lane's last signer balance; an unwired reader reports none.
func (s *Solver) currentSignerBalance() *big.Int {
	if s.signerBalance == nil {
		return nil
	}
	return s.signerBalance()
}

// onLaneStateChange handles one coalesced lane-state signal: the funding gate reopening (the signer was
// funded, or the base fee fell back), or the signer balance having risen above the last refusal's, ends a
// redeem backoff so the next pass sends a full batch.
func (s *Solver) onLaneStateChange(ctx context.Context) {
	fundable := s.laneFundable()
	reopened := fundable && !s.redeem.fundable
	s.redeem.fundable = fundable
	switch {
	case !s.redeem.active():
	case reopened:
		s.endRedeemBackoff(ctx, "the funding gate reopened")
	default:
		s.endRedeemBackoffOnTopUp(ctx)
	}
}

// beginRedeemPass starts a redeem pass and reports whether it may send. A top-up the manager has read since
// the last refusal ends the backoff first, so the pass after it sends a full batch; the manager announces
// no lane-state signal for a balance rise, so this is where one is noticed with the gate off.
func (s *Solver) beginRedeemPass(ctx context.Context) bool {
	s.endRedeemBackoffOnTopUp(ctx)
	return s.redeem.beginPass()
}

// endRedeemBackoffOnTopUp ends a backoff once the signer balance rose above the last refusal's.
func (s *Solver) endRedeemBackoffOnTopUp(ctx context.Context) {
	balance := s.currentSignerBalance()
	if !s.redeem.toppedUp(balance) {
		return
	}
	s.endRedeemBackoff(ctx, "the signer balance rose", "balance", balance.String(),
		"refusedBalance", s.redeem.refusedBalance.String())
}

// endRedeemBackoff ends the episode, logged at Info once, as its start was.
func (s *Solver) endRedeemBackoff(ctx context.Context, reason string, fields ...any) {
	s.redeem.reset()
	observability.Log(ctx).Info("redeem backoff ended; the next pass sends a full batch",
		append([]any{"reason", reason}, fields...)...)
}

// subscribeLaneState subscribes to the shared lane's state changes; an unwired manager yields a nil
// channel, which never fires.
func (s *Solver) subscribeLaneState() (<-chan struct{}, func()) {
	if s.laneStates == nil {
		return nil, func() {}
	}
	return s.laneStates()
}
