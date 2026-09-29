package txmanager

import (
	"context"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"go.opentelemetry.io/otel/attribute"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// The shadow evaluator (strategy §2.13; docs/TXMANAGER-PLAN.md §4.5) scores both fee policies on every head
// without sending anything: real lifecycles, about eight a week across all lanes, are far too few to tell a
// policy that lands 99% of first attempts from one that lands 95%.
//
// At each new head s it starts one virtual fill of G_ref gas (balance.referenceGasUnits, or while that is 0
// the gas limit of the latest fill this process signed) with the signer balance the manager read last. Once
// block s+3 is known it prices that fill as each policy would have at s (a refusal scores refused), applies
// the policy's own replacement rule between blocks (legacy: a timed bump every replacementIntervalMs from
// the send; horizon: evaluatePending at every head), and scores blocks s+1..s+3. A block includes the attempt
// in force when its base fee is at most the fee cap and either it left room for G_ref or the attempt's
// effective tip is at least the block's run-percentile reward (p50 at the default
// fees.congestedRewardPercentile), a conservative stand-in for displacement by the transactions a builder
// preferred. The outcome is counted in shadow_lifecycles_total{policy, outcome}.
//
// It is fee-side only: blocks our relay cannot reach, reverts and competitor fills are not modelled, and
// consecutive heads are correlated, so the effective sample is smaller than one virtual fill per head.
//
// Input: the fee snapshot the rest of the manager shares (feeSnapshotCache), one per tick, every half block. A
// tick takes a snapshot a send, quote or pending evaluation read since the evaluator's previous one and less
// than a quarter block ago; otherwise it starts the shared read itself (the latest header and one
// eth_feeHistory, on the read endpoints), which those readers then share in turn. So it adds at most one read
// per tick, one per half block when nothing else reads (the legacy policy's normal case), and consecutive
// snapshots it takes are at most three quarters of a block apart plus a read's latency: it sees every head
// that stays the newest for longer. Each snapshot's fee history is merged into a short store of blocks, from
// which the evaluator rebuilds the snapshot each policy would have seen at any head it needs.
//
// Goroutine model: Start runs one goroutine (runShadow) that owns its shadowEvaluator; nothing else reads or
// writes it. It reads the manager's shared state only through the snapshot cache and atomics.

// Values of shadow_lifecycles_total{outcome}.
type shadowOutcome string

const (
	shadowFirst1   shadowOutcome = "first1"   // the first attempt would be included in block s+1
	shadowFirst2   shadowOutcome = "first2"   // in block s+2, no replacement signed before it
	shadowFirst3   shadowOutcome = "first3"   // in block s+3, no replacement signed before it
	shadowReplaced shadowOutcome = "replaced" // included within 3 blocks, after a replacement was signed
	shadowMissed3  shadowOutcome = "missed3"  // not included within 3 blocks
	shadowRefused  shadowOutcome = "refused"  // the policy would not have signed it (balance or fee limit)
)

var (
	shadowOutcomes      = [...]shadowOutcome{shadowFirst1, shadowFirst2, shadowFirst3, shadowReplaced, shadowMissed3, shadowRefused}
	shadowPolicies      = [...]FeePolicy{FeePolicyLegacy, FeePolicyHorizon}
	shadowFirstOutcomes = [firstAttemptBlocks]shadowOutcome{shadowFirst1, shadowFirst2, shadowFirst3}
)

// shadowStart is one virtual fill.
type shadowStart struct {
	head    uint64   // the head it was sent at: blocks head+1..head+3 are scored
	lag     uint64   // blocks the snapshot trailed the real next block when it was sent
	gas     uint64   // G_ref
	balance *big.Int // signer balance then; nil with the balance guard off
}

// shadowLane is the lane a virtual fill starts on at one snapshot.
type shadowLane struct {
	lag     uint64
	gas     uint64
	balance *big.Int
}

// shadowScore is one scored virtual fill.
type shadowScore struct {
	policy  FeePolicy
	outcome shadowOutcome
}

// shadowRules is what the evaluator needs of the manager's configuration, resolved once when it starts.
type shadowRules struct {
	horizon horizonPolicy
	// legacyQuote prices a legacy attempt from one fee reading under a limit: Manager.legacyQuote itself, so
	// the shadow cannot drift from the rule a legacy send signs.
	legacyQuote         func(feeReading, *big.Int) (feeQuote, error)
	mandatoryTip        *big.Int // tipGwei in wei; zero selects the fee-history tip
	floorTip            *big.Int // the balance guard's floor tip (Manager.floorTip)
	initialCap          *big.Int // reserveFeeBump(normalFeeLimit(Request{})), a new attempt's cap; nil is unbounded
	replacementCap      *big.Int // normalFeeLimit(Request{}), a replacement's cap; nil is unbounded
	blockTime           time.Duration
	replacementInterval time.Duration
	maxHeadLag          uint64
}

// shadowEvaluator is the state of the shadow evaluator: the recent blocks of the fee histories it was fed,
// and the virtual fills waiting for their third block.
type shadowEvaluator struct {
	rules    shadowRules
	blocks   map[uint64]feeBlock
	gasLimit uint64 // gas limit of the newest header, which room is measured against (as tipRule does)
	head     uint64 // newest fee-history block merged
	pending  []shadowStart
	seen     time.Time // read time of the latest snapshot a tick took, which the next one never takes again
}

func newShadowEvaluator(rules shadowRules) *shadowEvaluator {
	return &shadowEvaluator{rules: rules, blocks: make(map[uint64]feeBlock)}
}

// observe merges one fee snapshot and, when it advances the head, starts a virtual fill at the new head
// (lane nil starts none) and scores every fill whose third block it completes. A fill starts only at a
// consistent snapshot (its fee history ends at the header) whose lag a send would accept; a send at a
// staler one waits for a newer head. A fill whose blocks the store lost (reads failed for longer than the
// snapshot's history covers) is dropped unscored. advanced reports whether the snapshot was newer.
func (e *shadowEvaluator) observe(snapshot *feeSnapshot, lane *shadowLane) (scores []shadowScore, advanced bool) {
	head := snapshot.historyHead
	if e.head != 0 && head <= e.head {
		return nil, false
	}
	for _, block := range snapshot.blocks {
		e.blocks[block.number] = block
	}
	e.gasLimit, e.head = snapshot.header.GasLimit, head
	if lane != nil && lane.gas > 0 && headerNumber(snapshot.header) == head && lane.lag <= e.rules.maxHeadLag {
		e.pending = append(e.pending, shadowStart{head: head, lag: lane.lag, gas: lane.gas, balance: lane.balance})
	}
	waiting := e.pending[:0]
	for _, start := range e.pending {
		if start.head+firstAttemptBlocks > e.head {
			waiting = append(waiting, start)
			continue
		}
		for _, policy := range shadowPolicies {
			if outcome, ok := e.score(policy, start); ok {
				scores = append(scores, shadowScore{policy: policy, outcome: outcome})
			}
		}
	}
	e.pending = waiting
	e.prune()
	return scores, true
}

// viewBlocks is how many blocks up to a head the evaluator needs to rebuild the snapshot there: the legacy
// tip's five, the tip rule's reward window, and the tip rule's two latest blocks.
func (e *shadowEvaluator) viewBlocks() uint64 {
	return max(feeHistoryBlocks, e.rules.horizon.rewardBlocks, 2)
}

// prune drops the blocks no waiting or future fill needs.
func (e *shadowEvaluator) prune() {
	oldest := e.head
	for _, start := range e.pending {
		oldest = min(oldest, start.head)
	}
	depth := e.viewBlocks()
	if oldest < depth {
		return
	}
	for number := range e.blocks {
		if number+depth <= oldest {
			delete(e.blocks, number)
		}
	}
}

// view rebuilds the fee snapshot a policy would have read at head: the viewBlocks blocks up to it, the
// header's base fee and the newest gas limit, and the next block's base fee, which the store holds once
// that block is known. It reports false when a block is missing.
func (e *shadowEvaluator) view(head uint64) (*feeSnapshot, bool) {
	depth := e.viewBlocks()
	if head+1 < depth {
		return nil, false
	}
	blocks := make([]feeBlock, 0, depth)
	for number := head + 1 - depth; number <= head; number++ {
		block, ok := e.blocks[number]
		if !ok {
			return nil, false
		}
		blocks = append(blocks, block)
	}
	next, ok := e.blocks[head+1]
	if !ok {
		return nil, false
	}
	return &feeSnapshot{
		header: &types.Header{
			Number:   new(big.Int).SetUint64(head),
			GasLimit: e.gasLimit,
			BaseFee:  new(big.Int).Set(blocks[len(blocks)-1].baseFee),
		},
		historyHead: head,
		nextBase:    new(big.Int).Set(next.baseFee),
		blocks:      blocks,
	}, true
}

// score runs one virtual fill under policy. It reports false when a block it needs is missing or, under the
// legacy policy, the fee history carries no rewards (a legacy send could not price then).
func (e *shadowEvaluator) score(policy FeePolicy, start shadowStart) (shadowOutcome, bool) {
	if policy == FeePolicyHorizon {
		return e.scoreHorizon(start)
	}
	return e.scoreLegacy(start)
}

// scoreLegacy prices the fill with the legacy rule (2×base + tip, capped, then the balance guard) and bumps
// it on the legacy timer: the bumps due before block s+k are signed at head s+k−1 from the fees read there.
func (e *shadowEvaluator) scoreLegacy(start shadowStart) (shadowOutcome, bool) {
	view, ok := e.view(start.head)
	if !ok {
		return "", false
	}
	fees, priced, err := e.legacyInitial(view, start)
	switch {
	case !priced:
		return "", false
	case err != nil:
		return shadowRefused, true
	}
	bumps, replaced := 0, false
	for k := uint64(1); k <= firstAttemptBlocks; k++ {
		if due := legacyBumpsBefore(k, e.rules.blockTime, e.rules.replacementInterval); bumps < due {
			at, found := e.view(start.head + k - 1)
			if !found {
				return "", false
			}
			for ; bumps < due; bumps++ {
				if next, bumped := e.legacyBump(fees, at, start); bumped {
					fees, replaced = next, true
				}
			}
		}
		block, found := e.blocks[start.head+k]
		if !found {
			return "", false
		}
		if shadowIncludes(fees, block, e.gasLimit, start.gas) {
			return shadowLanded(k, replaced), true
		}
	}
	return shadowMissed3, true
}

// legacyInitial prices a new legacy attempt at view as priceLegacyAttempt does. priced is false when the
// legacy rule cannot price there; err is a refusal: the fee limit or the balance guard.
func (e *shadowEvaluator) legacyInitial(view *feeSnapshot, start shadowStart) (fees feeQuote, priced bool, err error) {
	reading, ok := e.legacyReading(view)
	if !ok {
		return feeQuote{}, false, nil
	}
	if fees, err = e.rules.legacyQuote(reading, e.rules.initialCap); err != nil {
		return feeQuote{}, true, err
	}
	if start.balance == nil {
		return fees, true, nil
	}
	guard, err := applyBalanceGuard(guardInput{
		fees:       fees,
		nextBase:   view.nextBase,
		lag:        start.lag,
		minHorizon: e.rules.horizon.minHorizon,
		floorTip:   e.rules.floorTip,
		balance:    start.balance,
		value:      new(big.Int),
		gas:        start.gas,
	})
	if err != nil {
		return feeQuote{}, true, err
	}
	return guard.fees, true, nil
}

// legacyBump is the legacy policy's timed replacement of previous at view, as replacementFees signs it:
// under the request cap and, with the balance guard, the fee cap the balance funds. It reports false when
// no replacement would be signed (the cap cannot fund the bump), which leaves the attempt in force.
func (e *shadowEvaluator) legacyBump(previous feeQuote, view *feeSnapshot, start shadowStart) (feeQuote, bool) {
	var current *feeQuote
	if reading, ok := e.legacyReading(view); ok {
		if quote, err := e.rules.legacyQuote(reading, nil); err == nil {
			current = &quote
		}
	}
	limit := e.rules.replacementCap
	if start.balance != nil {
		affordable := affordableMaxFee(start.balance, new(big.Int), start.gas)
		if affordable.Cmp(bumpFee(previous.maxFee)) < 0 {
			return feeQuote{}, false
		}
		if limit == nil || affordable.Cmp(limit) < 0 {
			limit = affordable
		}
	}
	next, err := legacyReplacementFees(previous, current, limit)
	return next, err == nil
}

// legacyReading is the legacy rule's fee reading at view: the header and the median p25 reward of the latest
// five blocks (feeHistoryTip), or with a positive tipGwei that floor alone, since the node's tip suggestion
// that a legacy send floors at tipGwei is an RPC the shadow does not make. It reports false when the fee
// history carries no rewards, where a legacy send fails its fee read.
func (e *shadowEvaluator) legacyReading(view *feeSnapshot) (feeReading, bool) {
	if e.rules.mandatoryTip.Sign() > 0 {
		return feeReading{head: view.header, tip: new(big.Int).Set(e.rules.mandatoryTip)}, true
	}
	history := &ethereum.FeeHistory{Reward: make([][]*big.Int, 0, feeHistoryBlocks)}
	for _, block := range view.blocks[len(view.blocks)-feeHistoryBlocks:] {
		history.Reward = append(history.Reward, []*big.Int{block.legacyReward})
	}
	tip, ok := feeHistoryTip(history)
	if !ok {
		return feeReading{}, false
	}
	return feeReading{head: view.header, tip: tip}, true
}

// scoreHorizon prices the fill with initialFees and lets evaluatePending decide at each head before the
// next scored block: a reprice signs repriceFees (at the same gas: the shadow has no call to re-estimate),
// a stall's exact rebroadcast changes nothing on the fee side.
func (e *shadowEvaluator) scoreHorizon(start shadowStart) (shadowOutcome, bool) {
	view, ok := e.view(start.head)
	if !ok {
		return "", false
	}
	quote, err := initialFees(view, start.gas, start.balance, new(big.Int), e.rules.initialCap, start.lag, e.rules.horizon)
	if err != nil {
		return shadowRefused, true
	}
	var affordable *big.Int
	if start.balance != nil {
		affordable = affordableMaxFee(start.balance, new(big.Int), start.gas)
	}
	fees, replaced := quote.fees, false
	attempt := pendingInput{fees: fees, gas: start.gas, sentAt: start.head, stallBase: start.head}
	for k := uint64(1); k <= firstAttemptBlocks; k++ {
		if k > 1 {
			head := start.head + k - 1
			at, found := e.view(head)
			if !found {
				return "", false
			}
			attempt.fees = fees
			switch decision := evaluatePending(attempt, at, e.rules.horizon); decision.action {
			case pendingReprice:
				if next, _, err := repriceFees(fees, at, start.gas, affordable, e.rules.replacementCap, e.rules.horizon); err == nil {
					fees, replaced = next, true
					attempt.sentAt, attempt.stallBase, attempt.rebroadcasts = head, head, 0
				}
			case pendingStall:
				attempt.stallBase, attempt.rebroadcasts = head, attempt.rebroadcasts+1
			case pendingHold:
			}
		}
		block, found := e.blocks[start.head+k]
		if !found {
			return "", false
		}
		if shadowIncludes(fees, block, e.gasLimit, start.gas) {
			return shadowLanded(k, replaced), true
		}
	}
	return shadowMissed3, true
}

// shadowIncludes reports whether block would include an attempt of gas gas at fees: its base fee is at most
// the fee cap, and either it left room for the gas (against the newest header's gas limit) or the effective
// tip, min(tip, maxFee − baseFee), is at least the block's run-percentile reward. A block without room and
// without a reward does not include it.
func shadowIncludes(fees feeQuote, block feeBlock, blockGasLimit, gas uint64) bool {
	if fees.maxFee.Cmp(block.baseFee) < 0 {
		return false
	}
	if room(blockGasLimit, block.gasUsedRatio, gas) {
		return true
	}
	if block.runReward == nil {
		return false
	}
	tip := new(big.Int).Sub(fees.maxFee, block.baseFee)
	if fees.tip.Cmp(tip) < 0 {
		tip.Set(fees.tip)
	}
	return tip.Cmp(block.runReward) >= 0
}

// shadowLanded is the outcome of a virtual fill included k blocks after its head.
func shadowLanded(k uint64, replaced bool) shadowOutcome {
	if replaced {
		return shadowReplaced
	}
	return shadowFirstOutcomes[k-1]
}

// legacyBumpsBefore is how many timed bumps a legacy lifecycle has signed before block head+k: it is sent at
// head's time, block head+k follows k block times later, and bump n is signed n replacement intervals after
// the send, so the count is the n ≥ 1 with n·interval < k·blockTime.
func legacyBumpsBefore(k uint64, blockTime, interval time.Duration) int {
	until := time.Duration(k) * blockTime
	if interval <= 0 || until <= 0 {
		return 0
	}
	return int((until - 1) / interval)
}

// shadowEnabled reports whether the shadow evaluator runs: shadow.enabled, and metrics to report to.
func (m *Manager) shadowEnabled() bool {
	return !m.cfg.Shadow.Disabled && m.metrics != nil
}

// shadowInterval is how often the shadow evaluator takes a fee snapshot: every half block, so it sees each
// head within half a block of its arrival when it reads its own.
func (m *Manager) shadowInterval() time.Duration {
	return max(m.cfg.Fees.BlockTime/2, time.Millisecond)
}

// shadowShareAge is the age below which a tick takes another reader's newer snapshot instead of reading: half
// the interval. A shared snapshot then trails the tick by at most a quarter block, and the evaluator's own
// read is never reused by its next tick however long the read took (it takes only newer snapshots), so
// without other readers it reads at every tick rather than every other one at a fixed phase in the slot.
func (m *Manager) shadowShareAge() time.Duration {
	return m.shadowInterval() / 2
}

// shadowGas is the virtual fill's gas limit: balance.referenceGasUnits, or the latest signed fill's gas
// limit while that is 0; 0 until one is known.
func (m *Manager) shadowGas() uint64 {
	if reference := m.cfg.Balance.ReferenceGasUnits; reference > 0 {
		return reference
	}
	return m.lastFillGas.Load()
}

func (m *Manager) newShadowEvaluator() *shadowEvaluator {
	return newShadowEvaluator(shadowRules{
		horizon:             m.horizon,
		legacyQuote:         m.legacyQuote,
		mandatoryTip:        gweiToWei(m.cfg.TipGwei),
		floorTip:            m.floorTip(),
		initialCap:          reserveFeeBump(m.normalFeeLimit(Request{})),
		replacementCap:      m.normalFeeLimit(Request{}),
		blockTime:           m.cfg.Fees.BlockTime,
		replacementInterval: m.cfg.ReplacementInterval,
		maxHeadLag:          m.maxHeadLagBlocks(),
	})
}

// runShadow runs the shadow evaluator until ctx ends. It is metrics-only: its failures are logged at Info
// when a run of failed snapshot reads starts and ends and at V(1) in between, never at Error, since nothing
// but its own series depends on it.
func (m *Manager) runShadow(ctx context.Context) {
	if !m.shadowEnabled() {
		return
	}
	m.metrics.startShadow()
	evaluator := m.newShadowEvaluator()
	ticker := time.NewTicker(m.shadowInterval())
	defer ticker.Stop()
	failing := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		err := m.shadowTick(ctx, evaluator)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil && !failing:
			failing = true
			observability.Log(ctx).Info("shadow fee evaluation paused: fee snapshot unavailable", "error", err.Error())
		case err != nil:
			observability.Log(ctx).V(1).Info("shadow fee snapshot unavailable", "error", err.Error())
		case failing:
			failing = false
			observability.Log(ctx).Info("shadow fee evaluation resumed")
		}
	}
}

// shadowTick is one evaluation, the root span txmanager.shadow: the shared fee snapshot (another reader's
// newer one younger than shadowShareAge, or else a read), a virtual fill at a new head, and the scores it
// completes. A new head also refreshes the next-base-fee and required-balance gauges for the reference fill,
// so they follow every head even while the funding gate is off.
func (m *Manager) shadowTick(ctx context.Context, evaluator *shadowEvaluator) (err error) {
	ctx, end := tracer.Start(ctx, "txmanager.shadow")
	defer func() { end(err) }()
	snapshot, err := m.snapshots.getNewer(ctx, m.shadowShareAge(), evaluator.seen)
	if err != nil {
		return err
	}
	evaluator.seen = snapshot.readAt
	gas := m.shadowGas()
	var lane *shadowLane
	if gas > 0 {
		lane = &shadowLane{lag: snapshot.lag(time.Now(), m.cfg.Fees.BlockTime), gas: gas}
		if m.guardEnabled() {
			// Without a balance read yet the guard's verdict is unknown, so no fill starts.
			if lane.balance = m.SignerBalance(); lane.balance == nil {
				lane = nil
			}
		}
	}
	scores, advanced := evaluator.observe(snapshot, lane)
	observability.SetAttributes(ctx,
		attribute.Int64("fee.head", int64(snapshot.historyHead)),
		attribute.Bool("shadow.new_head", advanced),
		attribute.Int("shadow.scored", len(scores)),
	)
	if !advanced {
		return nil
	}
	if gas > 0 {
		m.observeFeeSnapshot(snapshot.nextBase, gas)
	}
	for _, score := range scores {
		m.metrics.shadowLifecycle(score.policy, score.outcome)
	}
	if len(scores) > 0 {
		outcomes := make([]string, len(scores))
		for i, score := range scores {
			outcomes[i] = string(score.policy) + "=" + string(score.outcome)
		}
		observability.Log(ctx).V(1).Info("shadow fills scored", "head", snapshot.historyHead, "gas", gas, "outcomes", outcomes)
	}
	return nil
}
