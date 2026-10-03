package txmanager

import (
	"context"
	"math"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/go-errors/errors"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// HorizonConfig tunes how the manager prices a call and when it replaces a pending one. Every call is
// priced for inclusion in the next block at the lowest spend: fees are capped at the exact EIP-1559
// base-fee bound for MaxBlocks blocks, the tip rises only during runs of full blocks, gas is
// estimated against the next block, and a pending call is repriced only on block evidence. Zero
// values select the defaults below.
type HorizonConfig struct {
	MaxBlocks                 int           // blocks the initial fee cap keeps the full tip valid; 0 => 6
	BlockTime                 time.Duration // chain slot time; 0 => 12s
	TipFloorGwei              float64       // tip while the last two blocks had room for the call; 0 => 0.02
	FullBlockTipGwei          float64       // tip when one of the last two blocks had no room; 0 => 0.1
	CongestedTipFloorGwei     float64       // lower clamp when both had no room; 0 => 0.2
	CongestedTipCapGwei       float64       // upper clamp when both had no room; 0 => 15
	CongestedRewardBlocks     int           // blocks whose reward percentile sets the congested tip; 0 => 3
	CongestedRewardPercentile float64       // fee-history reward percentile for that tip; 0 => 50
	EscalateAfterFullBlocks   int           // consecutive full blocks lost while valid before repricing; 0 => 2
	StallAfterBlocks          int           // blocks with room lost while valid before a stall response; 0 => 3
	GasHeadroomBps            int           // headroom over the next-block gas estimate; 0 => 500
	FallbackGasHeadroomBps    int           // headroom over a latest-state estimate; 0 => 1000
}

const (
	defaultHorizonMaxBlocks                 = 6
	defaultHorizonBlockTime                 = 12 * time.Second
	defaultHorizonTipFloorGwei              = 0.02
	defaultHorizonFullBlockTipGwei          = 0.1
	defaultHorizonCongestedTipFloorGwei     = 0.2
	defaultHorizonCongestedTipCapGwei       = 15
	defaultHorizonCongestedRewardBlocks     = 3
	defaultHorizonCongestedRewardPercentile = 50.0
	defaultHorizonEscalateAfterFullBlocks   = 2
	defaultHorizonStallAfterBlocks          = 3
	defaultHorizonGasHeadroomBps            = 500
	defaultHorizonFallbackGasHeadroomBps    = 1000
	// validityHorizonBlocks is how far ahead a pending attempt must stay valid at the tip floor
	// before it is repriced: it survives the next block even at the maximum base-fee increase.
	validityHorizonBlocks = 2
	// stallRebroadcastsBeforeReprice is how many exact rebroadcasts a stalled attempt gets before a
	// minimal fee bump replaces it, in case the relay dropped the hash while still deduplicating it.
	stallRebroadcastsBeforeReprice = 2
	basisPoints                    = 10_000
)

// Replacement and rebroadcast reasons exported on replacements_total{reason}.
const (
	replaceReasonValidity      = "validity"
	replaceReasonCongestion    = "congestion"
	replaceReasonStall         = "stall"
	replaceReasonGas           = "gas"
	replaceReasonFallback      = "fallback"
	rebroadcastReasonCapped    = "capped"
	rebroadcastReasonUncertain = "uncertain"
)

func (c HorizonConfig) withDefaults() HorizonConfig {
	if c.MaxBlocks <= 0 {
		c.MaxBlocks = defaultHorizonMaxBlocks
	}
	if c.BlockTime <= 0 {
		c.BlockTime = defaultHorizonBlockTime
	}
	if c.TipFloorGwei <= 0 {
		c.TipFloorGwei = defaultHorizonTipFloorGwei
	}
	if c.FullBlockTipGwei <= 0 {
		c.FullBlockTipGwei = defaultHorizonFullBlockTipGwei
	}
	if c.CongestedTipFloorGwei <= 0 {
		c.CongestedTipFloorGwei = defaultHorizonCongestedTipFloorGwei
	}
	if c.CongestedTipCapGwei <= 0 {
		c.CongestedTipCapGwei = defaultHorizonCongestedTipCapGwei
	}
	if c.CongestedRewardBlocks <= 0 {
		c.CongestedRewardBlocks = defaultHorizonCongestedRewardBlocks
	}
	if c.CongestedRewardPercentile <= 0 {
		c.CongestedRewardPercentile = defaultHorizonCongestedRewardPercentile
	}
	if c.EscalateAfterFullBlocks <= 0 {
		c.EscalateAfterFullBlocks = defaultHorizonEscalateAfterFullBlocks
	}
	if c.StallAfterBlocks <= 0 {
		c.StallAfterBlocks = defaultHorizonStallAfterBlocks
	}
	if c.GasHeadroomBps <= 0 {
		c.GasHeadroomBps = defaultHorizonGasHeadroomBps
	}
	if c.FallbackGasHeadroomBps <= 0 {
		c.FallbackGasHeadroomBps = defaultHorizonFallbackGasHeadroomBps
	}
	return c
}

// horizonPolicy is HorizonConfig resolved to wei, built once by New and read-only afterwards.
type horizonPolicy struct {
	maxBlocks               int
	blockTime               time.Duration
	tipFloor                *big.Int
	fullBlockTip            *big.Int
	congestedTipFloor       *big.Int
	congestedTipCap         *big.Int
	rewardBlocks            int
	rewardPercentile        float64
	escalateAfterFullBlocks int
	stallAfterBlocks        int
	gasHeadroomBps          int
	fallbackGasHeadroomBps  int
}

func newHorizonPolicy(c HorizonConfig) horizonPolicy {
	c = c.withDefaults()
	return horizonPolicy{
		maxBlocks:               c.MaxBlocks,
		blockTime:               c.BlockTime,
		tipFloor:                gweiToWei(c.TipFloorGwei),
		fullBlockTip:            gweiToWei(c.FullBlockTipGwei),
		congestedTipFloor:       gweiToWei(c.CongestedTipFloorGwei),
		congestedTipCap:         gweiToWei(c.CongestedTipCapGwei),
		rewardBlocks:            c.CongestedRewardBlocks,
		rewardPercentile:        c.CongestedRewardPercentile,
		escalateAfterFullBlocks: c.EscalateAfterFullBlocks,
		stallAfterBlocks:        c.StallAfterBlocks,
		gasHeadroomBps:          c.GasHeadroomBps,
		fallbackGasHeadroomBps:  c.FallbackGasHeadroomBps,
	}
}

// window is how many recent blocks one fee read covers: enough for the reward window, the run of
// full blocks that escalates a tip, and the blocks with room that make a stall.
func (p horizonPolicy) window() int {
	return max(p.rewardBlocks, p.escalateAfterFullBlocks, p.stallAfterBlocks, 2)
}

// blockFee is one mined block as eth_feeHistory reports it.
type blockFee struct {
	number       uint64
	baseFee      *big.Int
	gasUsedRatio float64
	reward       *big.Int // the congestion reward percentile of the block's priority fees
}

// feeSnapshot is one fee-history window ending at the latest block.
type feeSnapshot struct {
	head        uint64
	gasLimit    uint64   // latest header gas limit; block gas limits move slowly
	nextBaseFee *big.Int // base fee of block head+1, as the node computes it
	blocks      []blockFee
}

// roomFor reports whether the block left at least gas units unused, so a transaction of that size
// could have been included without displacing another.
func (b blockFee) roomFor(gasLimit, gas uint64) bool {
	return float64(gasLimit)*(1-b.gasUsedRatio) >= float64(gas)
}

func parseFeeSnapshot(history *ethereum.FeeHistory, gasLimit uint64) (feeSnapshot, error) {
	if history == nil || history.OldestBlock == nil || !history.OldestBlock.IsUint64() {
		return feeSnapshot{}, errors.New("fee history has no oldest block")
	}
	count := len(history.GasUsedRatio)
	if count == 0 || len(history.BaseFee) != count+1 || len(history.Reward) != count {
		return feeSnapshot{}, errors.Errorf(
			"fee history has %d blocks, %d base fees and %d reward rows",
			count, len(history.BaseFee), len(history.Reward),
		)
	}
	oldest := history.OldestBlock.Uint64()
	snapshot := feeSnapshot{
		head:     oldest + uint64(count) - 1,
		gasLimit: gasLimit,
		blocks:   make([]blockFee, count),
	}
	for i := range count {
		baseFee, ratio := history.BaseFee[i], history.GasUsedRatio[i]
		if baseFee == nil || baseFee.Sign() < 0 || math.IsNaN(ratio) || ratio < 0 {
			return feeSnapshot{}, errors.Errorf("fee history block %d has invalid base fee or gas used ratio", oldest+uint64(i))
		}
		if len(history.Reward[i]) != 1 || history.Reward[i][0] == nil || history.Reward[i][0].Sign() < 0 {
			return feeSnapshot{}, errors.Errorf("fee history block %d has invalid rewards", oldest+uint64(i))
		}
		snapshot.blocks[i] = blockFee{
			number:       oldest + uint64(i),
			baseFee:      new(big.Int).Set(baseFee),
			gasUsedRatio: min(ratio, 1),
			reward:       new(big.Int).Set(history.Reward[i][0]),
		}
	}
	next := history.BaseFee[count]
	if next == nil || next.Sign() < 0 {
		return feeSnapshot{}, errors.New("fee history has an invalid next base fee")
	}
	snapshot.nextBaseFee = new(big.Int).Set(next)
	return snapshot, nil
}

// readFeeSnapshot reads the latest header (for the block gas limit and the next-block gas estimate)
// and the fee-history window ending at the latest block.
func (m *Manager) readFeeSnapshot(ctx context.Context) (feeSnapshot, *types.Header, error) {
	feeCtx, cancel := context.WithTimeout(ctx, m.feeReadTimeout())
	defer cancel()
	header, err := m.backend.HeaderByNumber(feeCtx, nil)
	if err != nil {
		return feeSnapshot{}, nil, errors.Errorf("%w: header by number: %w", errFreshFeesUnavailable, err)
	}
	if header == nil || header.Number == nil || header.GasLimit == 0 {
		return feeSnapshot{}, nil, errors.Errorf("%w: latest header has no number or gas limit", errFreshFeesUnavailable)
	}
	history, err := m.backend.FeeHistory(
		feeCtx, uint64(m.horizon.window()), nil, []float64{m.horizon.rewardPercentile},
	)
	if err != nil {
		return feeSnapshot{}, nil, errors.Errorf("%w: fee history: %w", errFreshFeesUnavailable, err)
	}
	snapshot, err := parseFeeSnapshot(history, header.GasLimit)
	if err != nil {
		return feeSnapshot{}, nil, errors.Errorf("%w: %w", errFreshFeesUnavailable, err)
	}
	return snapshot, header, nil
}

// growBaseFee is the largest base fee the protocol allows after the given number of full blocks:
// EIP-1559 raises it by at most max(baseFee/8, 1) per block.
func growBaseFee(baseFee *big.Int, blocks int) *big.Int {
	grown := new(big.Int).Set(baseFee)
	for range blocks {
		step := new(big.Int).Div(grown, big.NewInt(params.DefaultBaseFeeChangeDenominator))
		if step.Sign() == 0 {
			step.SetInt64(1)
		}
		grown.Add(grown, step)
	}
	return grown
}

// horizonMaxFee is the fee cap that keeps a transaction valid, at its full tip, in each of the next
// `blocks` blocks even if every one of them is full.
func horizonMaxFee(nextBaseFee, tip *big.Int, blocks int) *big.Int {
	return new(big.Int).Add(growBaseFee(nextBaseFee, blocks-1), tip)
}

// horizonTip prices the priority fee from whether the last two blocks had room for gas units. Room
// in both means any positive tip is included; a single full block needs a modest premium; only a
// run of full blocks, where the call must outbid the marginal transaction, follows the market
// reward percentile, clamped to the configured range.
func horizonTip(snapshot feeSnapshot, gas uint64, policy horizonPolicy) *big.Int {
	blocks := snapshot.blocks
	lastFull := len(blocks) >= 1 && !blocks[len(blocks)-1].roomFor(snapshot.gasLimit, gas)
	previousFull := len(blocks) >= 2 && !blocks[len(blocks)-2].roomFor(snapshot.gasLimit, gas)
	switch {
	case lastFull && previousFull:
		tip := new(big.Int)
		for _, block := range blocks[max(0, len(blocks)-policy.rewardBlocks):] {
			if block.reward.Cmp(tip) > 0 {
				tip.Set(block.reward)
			}
		}
		if tip.Cmp(policy.congestedTipFloor) < 0 {
			tip.Set(policy.congestedTipFloor)
		}
		if tip.Cmp(policy.congestedTipCap) > 0 {
			tip.Set(policy.congestedTipCap)
		}
		return tip
	case lastFull || previousFull:
		return new(big.Int).Set(policy.fullBlockTip)
	default:
		return new(big.Int).Set(policy.tipFloor)
	}
}

// horizonFees prices a new call of gas units under limit: the tip rule plus the exact base-fee bound
// for the policy horizon. A limit below that bound shortens the horizon; one below the next base
// fee fails, because the call could not be included in the next block.
func horizonFees(snapshot feeSnapshot, gas uint64, limit *big.Int, policy horizonPolicy) (feeQuote, error) {
	tip := horizonTip(snapshot, gas, policy)
	maxFee := horizonMaxFee(snapshot.nextBaseFee, tip, policy.maxBlocks)
	if limit != nil && maxFee.Cmp(limit) > 0 {
		maxFee.Set(limit)
	}
	if maxFee.Cmp(snapshot.nextBaseFee) < 0 {
		return feeQuote{}, errors.Errorf(
			"%w: next base fee %s exceeds tx manager max fee %s", ErrFeeLimitReached, snapshot.nextBaseFee, maxFee,
		)
	}
	if room := new(big.Int).Sub(maxFee, snapshot.nextBaseFee); tip.Cmp(room) > 0 {
		tip.Set(room)
	}
	return feeQuote{baseFee: new(big.Int).Set(snapshot.nextBaseFee), tip: tip, maxFee: maxFee}, nil
}

type pendingAction uint8

const (
	pendingHold pendingAction = iota
	pendingReprice
	pendingStall
)

type pendingDecision struct {
	action pendingAction
	reason string
}

// decidePending judges the current attempt of gas units, sent when sentHead was the latest block,
// against the blocks mined since. It reprices when the fee cap would lapse within
// validityHorizonBlocks at the tip floor, or when the attempt lost the last escalateAfterFullBlocks
// blocks to full blocks while valid and the tip rule now asks for at least a replacement bump. Blocks
// it lost while valid and with room point to a dropped or unreachable transaction, which a higher
// fee does not fix, so enough of them report a stall instead. Anything else holds: waiting is free.
func decidePending(snapshot feeSnapshot, attempt feeQuote, gas, sentHead uint64, policy horizonPolicy) pendingDecision {
	if attempt.maxFee.Cmp(horizonMaxFee(snapshot.nextBaseFee, policy.tipFloor, validityHorizonBlocks)) < 0 {
		return pendingDecision{action: pendingReprice, reason: replaceReasonValidity}
	}
	var fullRun, roomy int
	running := true
	for _, block := range slices.Backward(snapshot.blocks) {
		if block.number <= sentHead {
			break
		}
		valid := attempt.maxFee.Cmp(block.baseFee) >= 0
		hasRoom := block.roomFor(snapshot.gasLimit, gas)
		if running && valid && !hasRoom {
			fullRun++
		} else {
			running = false
		}
		if valid && hasRoom {
			roomy++
		}
	}
	if fullRun >= policy.escalateAfterFullBlocks &&
		horizonTip(snapshot, gas, policy).Cmp(bumpFee(attempt.tip)) >= 0 {
		return pendingDecision{action: pendingReprice, reason: replaceReasonCongestion}
	}
	if roomy >= policy.stallAfterBlocks {
		return pendingDecision{action: pendingStall, reason: replaceReasonStall}
	}
	return pendingDecision{action: pendingHold}
}

// horizonProgress is the per-lifecycle state of horizon repricing. Only the lifecycle goroutine
// reads or writes it.
type horizonProgress struct {
	sentHead          uint64    // latest block when the current attempt was last sent or rebroadcast
	sentAt            time.Time // when that was
	lastHead          uint64    // latest block the lifecycle has evaluated
	stallRebroadcasts int       // exact rebroadcasts since the last fee change
	lastEvaluation    time.Time
	feeReads          readStreak
}

// sent records that the current attempt went out, or out again, after head.
func (p *horizonProgress) sent(head uint64) {
	p.sentHead, p.sentAt = head, time.Now()
}

// judgedSentHead is the block the current attempt counts evidence from. A read endpoint that lagged
// when the attempt went out reports an older head than the one it actually followed, which would
// count blocks mined before the send as blocks it lost. At most one block per elapsed slot, plus one
// already in flight, can follow a send, so the wall clock bounds how far back the evidence reaches.
func judgedSentHead(sentHead, head uint64, sentAt, now time.Time, blockTime time.Duration) uint64 {
	followed := uint64(max(now.Sub(sentAt), 0)/blockTime) + 1
	if head <= followed {
		return sentHead
	}
	return max(sentHead, head-followed)
}

// replacementTick is the cadence of the pending-transaction replacement loop: twice per block, where
// most ticks find no new block and do nothing.
func (m *Manager) replacementTick() time.Duration {
	return max(m.horizon.blockTime/2, time.Millisecond)
}

// replacementCadence is the expected time between replacement decisions, which bounds how close to
// its deadline an ambiguous broadcast may still be rebroadcast.
func (m *Manager) replacementCadence() time.Duration {
	return m.horizon.blockTime
}

// callQuote is a new call's pricing: its fees, its gas limit, and the latest block known when it was
// priced.
type callQuote struct {
	fees feeQuote
	gas  uint64
	head uint64
}

// quoteCall prices a new call under limit. One fee read supplies both the parent header for the
// next-block gas estimate and the window the tip rule reads.
func (m *Manager) quoteCall(ctx context.Context, req Request, limit *big.Int) (callQuote, error) {
	snapshot, header, err := m.readFeeSnapshot(ctx)
	if err != nil {
		return callQuote{}, errors.Errorf("send %q: %w", req.Label, err)
	}
	gas, err := m.callGas(ctx, req, header)
	if err != nil {
		return callQuote{}, err
	}
	fees, err := horizonFees(snapshot, gas, limit, m.horizon)
	if err != nil {
		return callQuote{}, errors.Errorf("send %q: %w", req.Label, err)
	}
	// The fee window can trail the latest header when the read endpoint serves them from different
	// nodes or caches fee history; the call goes out after the fresher of the two.
	return callQuote{fees: fees, gas: gas, head: max(snapshot.head, header.Number.Uint64())}, nil
}

// NextBlockGasEstimator estimates a call as if it were included in the block after parent. The
// chain client implements it with eth_estimateGas block overrides; a backend without it gets
// latest-state estimates.
type NextBlockGasEstimator interface {
	EstimateGasNextBlock(
		ctx context.Context, call ethereum.CallMsg, parent *types.Header, blockTime time.Duration,
	) (uint64, error)
}

// callGas sizes a new call for the next block. A request with a gas limit keeps it.
func (m *Manager) callGas(ctx context.Context, req Request, parent *types.Header) (uint64, error) {
	if req.GasLimit != 0 {
		return req.GasLimit, nil
	}
	gas, headroomBps, err := m.estimateHorizonGas(ctx, req, parent)
	if err != nil {
		if revert := executionRevert(err); revert != nil {
			return 0, errors.Errorf("estimate gas %q: %w", req.Label, revert)
		}
		// Calldata can contain unpublished authorizations. Keep it out of error logs and Sentry.
		observability.Log(ctx).Error(err, "gas estimation failed", "label", req.Label)
		return 0, errors.Errorf("estimate gas %q: %w", req.Label, err)
	}
	return withGasHeadroom(gas, headroomBps), nil
}

// estimateHorizonGas estimates req against parent's state with the next block's number and time, so
// time-dependent work the latest block already did (interest accrual, for one) is priced in, and
// returns the raw estimate with the headroom it calls for. An endpoint that rejects block overrides
// falls back to a latest-state estimate with wider headroom.
func (m *Manager) estimateHorizonGas(
	ctx context.Context, req Request, parent *types.Header,
) (gas uint64, headroomBps int, err error) {
	call := ethereum.CallMsg{From: m.signer.Address(), To: &req.To, Value: req.Value, Data: req.Data}
	if estimator, ok := m.backend.(NextBlockGasEstimator); ok {
		gas, err = estimator.EstimateGasNextBlock(ctx, call, parent, m.horizon.blockTime)
		if err == nil || !isBlockOverrideUnsupported(err) {
			return gas, m.horizon.gasHeadroomBps, err
		}
		if m.overrideFallbackLogged.CompareAndSwap(false, true) {
			observability.Log(ctx).Info("next-block gas estimate unsupported by the read RPC; using latest-state estimates",
				"label", req.Label, "error", err.Error())
		}
	}
	gas, err = m.backend.EstimateGas(ctx, call)
	return gas, m.horizon.fallbackGasHeadroomBps, err
}

func withGasHeadroom(gas uint64, bps int) uint64 {
	return gas + gas*uint64(bps)/basisPoints
}

// isBlockOverrideUnsupported recognizes an endpoint that rejects eth_estimateGas's fourth
// (blockOverrides) parameter, as opposed to a call that reverts or a transport failure.
func isBlockOverrideUnsupported(err error) bool {
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) && (rpcErr.ErrorCode() == -32602 || rpcErr.ErrorCode() == -32601) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"too many arguments", "invalid params", "invalid argument", "unknown field"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// horizonIntent is the replacement a horizon decision asks for: a fee change of the normal call, or a
// evaluateHorizon applies the horizon repricing rules to the pending lifecycle once per new block.
// It reports whether the deadline passed during replacement preparation.
func (m *Manager) evaluateHorizon(
	ctx context.Context, pending *pendingTransaction, replace func(replaceIntent) bool,
) bool {
	log := observability.Log(ctx)
	snapshot, parent, err := m.readFeeSnapshot(ctx)
	if err != nil {
		pending.horizon.feeReads.failed(log, err, "pending transaction fee window unavailable; holding attempt",
			"label", pending.req.Label, "nonce", pending.nonce)
		if time.Since(pending.horizon.lastEvaluation) < m.cfg.ReplacementInterval {
			return false
		}
		// Without block evidence for a whole replacement interval, bump once per interval so the
		// attempt cannot freeze.
		pending.horizon.lastEvaluation = time.Now()
		return replace(replaceIntent{reason: replaceReasonFallback})
	}
	pending.horizon.feeReads.recovered(log, "pending transaction fee window recovered",
		"label", pending.req.Label, "nonce", pending.nonce)
	pending.horizon.lastEvaluation = time.Now()
	if snapshot.head <= pending.horizon.lastHead {
		return false // no new block, or an endpoint behind one we already saw
	}
	pending.horizon.lastHead = snapshot.head
	if m.rebroadcastUncertainAttempt(ctx, pending) {
		pending.horizon.sent(snapshot.head)
		return false
	}
	gas := pending.gas
	sentHead := judgedSentHead(
		pending.horizon.sentHead, snapshot.head, pending.horizon.sentAt, time.Now(), m.horizon.blockTime,
	)
	decision := decidePending(snapshot, pending.fees, gas, sentHead, m.horizon)
	switch decision.action {
	case pendingReprice:
		return replace(replaceIntent{reason: decision.reason})
	case pendingStall:
		return m.handleStall(ctx, pending, parent, replace)
	case pendingHold:
		// Waiting is free: the attempt is valid and nothing shows it being outbid or dropped.
	}
	return false
}

// handleStall answers blocks the current attempt lost while valid and with room: the relay dropped
// it, a builder it cannot reach built them, or the call outgrew its gas limit. A normal call is
// re-estimated for the next block first and replaced with a larger limit once it no longer fits;
// a deadline reached during that estimate is left to the lifecycle, which abandons tracking.
// Otherwise the exact bytes are rebroadcast, and after stallRebroadcastsBeforeReprice rebroadcasts a
// minimal fee bump replaces them.
func (m *Manager) handleStall(
	ctx context.Context,
	pending *pendingTransaction,
	parent *types.Header,
	replace func(replaceIntent) bool,
) bool {
	if pending.req.GasLimit == 0 {
		gas, headroomBps, err := m.reestimateStalledCall(ctx, pending, parent)
		switch {
		case pending.abandonmentDue(time.Now()):
			return false // no normal rebroadcast past the deadline or a shutdown request
		case err != nil:
			observability.Log(ctx).V(1).Info("stalled transaction re-estimate failed; rebroadcasting",
				"label", pending.req.Label, "nonce", pending.nonce, "error", err.Error())
		case gas > pending.gas:
			return replace(replaceIntent{reason: replaceReasonGas, gas: withGasHeadroom(gas, headroomBps)})
		}
	}
	if pending.horizon.stallRebroadcasts >= stallRebroadcastsBeforeReprice {
		return replace(replaceIntent{reason: replaceReasonStall})
	}
	if m.rebroadcastStalledAttempt(ctx, pending) {
		pending.horizon.stallRebroadcasts++
		pending.horizon.sent(pending.horizon.lastHead)
	}
	return false
}

// reestimateStalledCall sizes a stalled normal call for the next block. It runs on the lifecycle
// goroutine, which also services receipts, the request deadline and shutdown, so a read endpoint
// that never answers must not hold it: the estimate, fallback included, gets its own budget and, like
// a normal replacement broadcast, ends at the request deadline.
func (m *Manager) reestimateStalledCall(
	ctx context.Context, pending *pendingTransaction, parent *types.Header,
) (gas uint64, headroomBps int, err error) {
	deadline := time.Now().Add(m.gasEstimateTimeout())
	if !pending.deadline.IsZero() && pending.deadline.Before(deadline) {
		deadline = pending.deadline
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	return m.estimateHorizonGas(ctx, pending.req, parent)
}

// rebroadcastStalledAttempt resends the latest attempt's exact bytes for a relay that dropped it,
// after the same nonce checks a replacement makes. It reports whether it sent.
func (m *Manager) rebroadcastStalledAttempt(ctx context.Context, pending *pendingTransaction) bool {
	if available, err := m.replacementNonceAvailable(ctx, pending); err != nil || !available {
		return false
	}
	attempt := pending.latestAttempt()
	if attempt.tx == nil {
		return false
	}
	sendCtx, cancelSend := replacementBroadcastContext(ctx, pending)
	err := m.sendSigned(sendCtx, attempt.tx)
	cancelSend()
	if m.replacementContention(ctx, pending, err) {
		return true
	}
	fields := []any{
		"label", pending.req.Label, "hash", attempt.hash.Hex(), "nonce", pending.nonce,
		"reason", replaceReasonStall,
	}
	if err != nil && !isKnownTransactionError(err) {
		observability.Log(ctx).Error(err, "stalled transaction rebroadcast failed", fields...)
		return true
	}
	m.metrics.replacement(pending.req.Label, replacementKindRebroadcast, replaceReasonStall)
	observability.Log(ctx).Info("stalled transaction rebroadcast", fields...)
	return true
}
