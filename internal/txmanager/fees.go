package txmanager

import (
	"math/big"

	"github.com/ethereum/go-ethereum/params"
	"github.com/go-errors/errors"
)

// Fee arithmetic of the fee and gas strategy (docs/TXMANAGER-PLAN.md §4). The helpers here are pure:
// they read no manager state and do no I/O, so the balance guard and the fee policies share one
// table-tested definition of base-fee growth, validity horizons and affordability.

// maxGrownFeeWei saturates grow. No balance funds 2^256 wei per unit of gas, so growth past it can only
// cost time; the bound caps grow at about 1,500 iterations whatever horizon a stale head asks for.
var maxGrownFeeWei = new(big.Int).Lsh(big.NewInt(1), 256)

// maxReportedHorizonBlocks bounds the validity horizon the manager reports for an attempt. A legacy
// 2×base fee is about 6 blocks, so a report at the bound only says "comfortably more than the policy".
const maxReportedHorizonBlocks = 32

// firstAttemptBlocks is how many blocks after its send head the first signed attempt may take to land
// and still count as a first attempt (first@3 in the strategy).
const firstAttemptBlocks = 3

// grow applies k blocks of the largest EIP-1559 base-fee increase to x: x += max(floor(x/8), 1) per
// block, with go-ethereum's denominator. It is the exact protocol bound, deliberately not a knob: a
// smaller growth rate would silently void every validity guarantee built on it.
func grow(x *big.Int, k uint64) *big.Int {
	out := new(big.Int).Set(x)
	denominator := big.NewInt(int64(params.DefaultBaseFeeChangeDenominator))
	step := new(big.Int)
	for range k {
		if out.Cmp(maxGrownFeeWei) >= 0 {
			break
		}
		step.Quo(out, denominator)
		if step.Sign() <= 0 {
			step.SetInt64(1)
		}
		out.Add(out, step)
	}
	return out
}

// horizonFee is fee(H, tip) = grow(nextBase, H−1) + tip: a fee cap that pays the full tip in every one
// of the next H blocks, nextBase being the base fee of the first of them. H below 1 is treated as 1.
func horizonFee(nextBase *big.Int, blocks uint64, tip *big.Int) *big.Int {
	fee := grow(nextBase, max(blocks, 1)-1)
	return fee.Add(fee, tip)
}

// validHorizon is the largest H, up to limit, with fee(H, tip) ≤ maxFee: how many blocks from the next
// one an attempt stays valid at that tip. Zero means it may be invalid in the very next block.
func validHorizon(nextBase, tip, maxFee *big.Int, limit uint64) uint64 {
	base := new(big.Int).Set(nextBase)
	fee := new(big.Int)
	var blocks uint64
	for blocks < limit {
		if fee.Add(base, tip).Cmp(maxFee) > 0 {
			break
		}
		blocks++
		base = grow(base, 1)
	}
	return blocks
}

// affordableMaxFee is floor((balance − value) / gas): the largest fee cap the balance funds in full at
// that gas limit. It is negative when the balance cannot even cover the value.
func affordableMaxFee(balance, value *big.Int, gas uint64) *big.Int {
	spendable := new(big.Int).Sub(balance, value)
	return spendable.Div(spendable, new(big.Int).SetUint64(gas))
}

// requiredBalance is gas·maxFee + value, what a node checks the sender can pay before it admits the
// attempt, whatever the attempt ends up paying.
func requiredBalance(gas uint64, maxFee, value *big.Int) *big.Int {
	required := new(big.Int).Mul(new(big.Int).SetUint64(gas), maxFee)
	return required.Add(required, value)
}

// guardInput is one attempt priced by the fee policy, with the signer balance pinned at its head.
type guardInput struct {
	fees       feeQuote // priced by the policy, already under the request and global caps
	nextBase   *big.Int // base fee of the next block after the snapshot head (pb)
	lag        uint64   // blocks the snapshot trails the real next block
	minHorizon uint64   // fees.minHorizonBlocks
	floorTip   *big.Int // tip the refusal floor assumes: fees.tipFloorGwei, or a larger mandatory tipGwei
	balance    *big.Int // signer balance at the pinned head
	value      *big.Int
	gas        uint64
}

// guardResult is the attempt the balance can fund, or why none.
type guardResult struct {
	fees       feeQuote
	affordable *big.Int // floor((balance − value) / gas)
	floor      *big.Int // fee(minHorizon + lag, floorTip), the lowest cap the guard signs
	horizon    uint64   // blocks the signed cap stays valid at floorTip, bounded for reporting
	clamped    bool     // the balance, not the policy or a cap, set the fee cap
}

// applyBalanceGuard caps the fee cap at what the balance funds and refuses an attempt the balance
// cannot keep valid for minHorizon blocks from the real next block (strategy §2.4). It only acts when
// the balance binds: a policy fee or request cap already below the floor is left to the policy, so a
// funded lane signs exactly what it signed without the guard. A refusal wraps ErrUnaffordable, or
// errUnaffordableOneBlock when the balance would still fund one block at the floor tip.
func applyBalanceGuard(in guardInput) (guardResult, error) {
	if in.gas == 0 {
		return guardResult{}, errors.New("balance guard needs a positive gas limit")
	}
	result := guardResult{
		fees:       cloneFeeQuote(in.fees),
		affordable: affordableMaxFee(in.balance, in.value, in.gas),
		floor:      horizonFee(in.nextBase, in.minHorizon+in.lag, in.floorTip),
	}
	if result.affordable.Cmp(result.fees.maxFee) < 0 {
		if result.affordable.Cmp(result.floor) < 0 {
			refusal := ErrUnaffordable
			if oneBlock := new(big.Int).Add(in.nextBase, in.floorTip); result.affordable.Cmp(oneBlock) >= 0 {
				refusal = errUnaffordableOneBlock
			}
			return result, errors.Errorf(
				"%w: balance %s funds %s wei per gas over gas limit %d and value %s, below the %d-block floor %s "+
					"(next base fee %s, head lag %d blocks)",
				refusal, in.balance, affordableString(result.affordable), in.gas, in.value,
				in.minHorizon, result.floor, in.nextBase, in.lag,
			)
		}
		result.fees.maxFee.Set(result.affordable)
		if tipRoom := new(big.Int).Sub(result.affordable, in.nextBase); result.fees.tip.Cmp(tipRoom) > 0 {
			result.fees.tip.Set(tipRoom)
		}
		result.clamped = true
	}
	result.horizon = validHorizon(in.nextBase, in.floorTip, result.fees.maxFee, maxReportedHorizonBlocks)
	return result, nil
}

func affordableString(affordable *big.Int) string {
	if affordable.Sign() < 0 {
		return "0"
	}
	return affordable.String()
}

// errFeeLimitReached is a horizon-policy price the request or global fee limit cannot fund: the limit
// sits below the lowest fee cap the policy signs, so the request fails as a submission error, as a legacy
// quote does when the base fee is above the limit.
var errFeeLimitReached = errors.New("fee limit reached")

// feeBinding names what set a horizon-policy fee cap: the policy's target, a request or global fee limit,
// or the signer balance. It is the reason of a "replacement capped" log and of balanceBound.
type feeBinding string

const (
	feeBindingTarget  feeBinding = "target"
	feeBindingCap     feeBinding = "cap"
	feeBindingBalance feeBinding = "balance"
)

// horizonPolicy is the horizon fee policy's configuration resolved to wei (strategy §2.3, §2.4). New
// builds it once; the pure helpers below take it by value.
type horizonPolicy struct {
	minHorizon         uint64 // fees.minHorizonBlocks: the refusal floor's horizon
	maxHorizon         uint64 // fees.maxHorizonBlocks: the horizon a send targets
	pricingHorizon     uint64 // fees.pricingHorizonBlocks: the horizon quotes are priced at
	rewardBlocks       uint64 // fees.congestedRewardBlocks: latest blocks a demand-run tip follows
	tipFloor           *big.Int
	singleFullBlockTip *big.Int
	congestedTipFloor  *big.Int
	congestedTipCap    *big.Int
}

func newHorizonPolicy(fees FeeConfig) horizonPolicy {
	return horizonPolicy{
		minHorizon:         fees.MinHorizonBlocks,
		maxHorizon:         fees.MaxHorizonBlocks,
		pricingHorizon:     fees.PricingHorizonBlocks,
		rewardBlocks:       fees.CongestedRewardBlocks,
		tipFloor:           gweiToWei(fees.TipFloorGwei),
		singleFullBlockTip: gweiToWei(fees.SingleFullBlockTipGwei),
		congestedTipFloor:  gweiToWei(fees.CongestedTipFloorGwei),
		congestedTipCap:    gweiToWei(fees.CongestedTipCapGwei),
	}
}

// room reports whether a block with this gas-used ratio left room for gas, measured against the block
// gas limit of the latest header: gasLimit × (1 − gasUsedRatio) ≥ gas. Room is measured against the
// attempt's own gas limit rather than a fixed size, so a larger fill needs emptier blocks.
func room(blockGasLimit uint64, gasUsedRatio float64, gas uint64) bool {
	return float64(blockGasLimit)*(1-gasUsedRatio) >= float64(gas)
}

// tipRule is the three-level priority fee of strategy §2.3 for an attempt of this gas limit. With N the
// snapshot's newest block: when neither N nor N−1 had room it is a demand run, and the tip follows the
// largest run-percentile reward of the latest rewardBlocks blocks, clamped to [congestedTipFloor,
// congestedTipCap]; when exactly one of them had no room it is singleFullBlockTip; otherwise tipFloor. A
// block the snapshot does not cover counts as having room, and a block without rewards as a zero reward,
// so the rule never falls below the floors.
func tipRule(snapshot *feeSnapshot, gas uint64, policy horizonPolicy) *big.Int {
	blocks := snapshot.blocks
	full := func(back int) bool {
		i := len(blocks) - 1 - back
		return i >= 0 && !room(snapshot.header.GasLimit, blocks[i].gasUsedRatio, gas)
	}
	latestFull, previousFull := full(0), full(1)
	switch {
	case latestFull && previousFull:
		reward := new(big.Int)
		for i, counted := len(blocks)-1, uint64(0); i >= 0 && counted < policy.rewardBlocks; i, counted = i-1, counted+1 {
			if runReward := blocks[i].runReward; runReward != nil && runReward.Cmp(reward) > 0 {
				reward.Set(runReward)
			}
		}
		return clampBig(reward, policy.congestedTipFloor, policy.congestedTipCap)
	case latestFull || previousFull:
		return new(big.Int).Set(policy.singleFullBlockTip)
	default:
		return new(big.Int).Set(policy.tipFloor)
	}
}

// initialFees prices a new horizon-policy attempt of gas gas and value value from a fee snapshot whose
// next base fee trails the real next block by lag blocks (strategy §2.4):
//
//	target = fee(maxHorizon, tip), at least the floor
//	maxFee = min(target, capLimit, floor((balance − value) / gas))
//	floor  = fee(minHorizon + lag, tipFloor)
//
// with tip = tipRule(gas). Below the floor it refuses: ErrUnaffordable (errUnaffordableOneBlock when the
// balance still funds pb + tipFloor) when the balance set the cap, errFeeLimitReached when the fee limit
// did. The tip is then clamped to maxFee − pb, which the floor keeps at or above tipFloor. capLimit is the
// request's initial cap, reserveFeeBump(normalFeeLimit); nil is unbounded. A nil balance skips the
// balance guard. The binding is reported with a refusal too.
func initialFees(
	snapshot *feeSnapshot, gas uint64, balance, value, capLimit *big.Int, lag uint64, policy horizonPolicy,
) (initialQuote, error) {
	if gas == 0 {
		return initialQuote{}, errors.New("horizon fees need a positive gas limit")
	}
	nextBase := snapshot.nextBase
	tip := tipRule(snapshot, gas, policy)
	floor := horizonFee(nextBase, policy.minHorizon+lag, policy.tipFloor)
	// The target never sits below the floor, which a head lag beyond maxHorizon − minHorizon could cause.
	maxFee := maxBigCopy(horizonFee(nextBase, policy.maxHorizon, tip), floor)
	binding := feeBindingTarget
	if capLimit != nil && capLimit.Cmp(maxFee) < 0 {
		maxFee.Set(capLimit)
		binding = feeBindingCap
	}
	var affordable *big.Int
	if balance != nil {
		affordable = affordableMaxFee(balance, value, gas)
		if affordable.Cmp(maxFee) < 0 {
			maxFee.Set(affordable)
			binding = feeBindingBalance
		}
	}
	if maxFee.Cmp(floor) < 0 {
		if binding == feeBindingBalance {
			refusal := ErrUnaffordable
			if oneBlock := new(big.Int).Add(nextBase, policy.tipFloor); affordable.Cmp(oneBlock) >= 0 {
				refusal = errUnaffordableOneBlock
			}
			return initialQuote{binding: binding}, errors.Errorf(
				"%w: balance %s funds %s wei per gas over gas limit %d and value %s, below the %d-block floor %s "+
					"(next base fee %s, head lag %d blocks)",
				refusal, balance, affordableString(affordable), gas, value,
				policy.minHorizon, floor, nextBase, lag,
			)
		}
		return initialQuote{binding: binding}, errors.Errorf(
			"%w: fee limit %s is below the %d-block floor %s (next base fee %s, head lag %d blocks)",
			errFeeLimitReached, maxFee, policy.minHorizon, floor, nextBase, lag,
		)
	}
	if tipRoom := new(big.Int).Sub(maxFee, nextBase); tip.Cmp(tipRoom) > 0 {
		tip.Set(tipRoom)
	}
	return initialQuote{
		fees:    feeQuote{baseFee: new(big.Int).Set(nextBase), tip: tip, maxFee: maxFee},
		horizon: validHorizon(nextBase, policy.tipFloor, maxFee, maxReportedHorizonBlocks),
		binding: binding,
	}, nil
}

// initialQuote is a new horizon-policy attempt's fees (their baseFee is the snapshot's next base fee), the
// blocks its fee cap stays valid at the floor tip from that next block, and what bound the cap.
type initialQuote struct {
	fees    feeQuote
	horizon uint64
	binding feeBinding
}

// pricingFee is the fee per gas a quote is priced at (strategy §2.9): bump(fee(pricingHorizon + lag,
// tipRule(referenceGas))) capped at limit, normalFeeLimit(Request{}). A solver passes it back as the fill's
// Request.MaxFeePerGas, whose initial cap reserveFeeBump(P) is then at least fee(pricingHorizon + lag), so
// the fill clears its fee(minHorizon + lag') floor after up to pricingHorizon − minHorizon blocks of
// maximum base-fee growth between quote and fill. The lag term keeps that margin from the real next block
// when the snapshot trails it. A limit too low for the fill to clear its floor even without growth fails
// with errFeeLimitReached, as a legacy quote fails when the base fee is above the limit.
func pricingFee(snapshot *feeSnapshot, lag, referenceGas uint64, limit *big.Int, policy horizonPolicy) (*big.Int, error) {
	tip := tipRule(snapshot, referenceGas, policy)
	priced := bumpFee(horizonFee(snapshot.nextBase, policy.pricingHorizon+lag, tip))
	if limit != nil && priced.Cmp(limit) > 0 {
		priced.Set(limit)
	}
	floor := horizonFee(snapshot.nextBase, policy.minHorizon+lag, policy.tipFloor)
	if reserveFeeBump(priced).Cmp(floor) < 0 {
		return nil, errors.Errorf(
			"%w: fee limit %s leaves an initial cap %s below the %d-block floor %s (next base fee %s, head lag %d blocks)",
			errFeeLimitReached, feeLimitString(limit), reserveFeeBump(priced), policy.minHorizon, floor,
			snapshot.nextBase, lag,
		)
	}
	return priced, nil
}

// repriceFees prices a fee-changing same-nonce replacement of a pending call under the horizon policy
// (strategy §2.7): tip' = max(bump(tip), tipRule(gas)) and maxFee' = max(bump(maxFee), fee(maxHorizon,
// tip')), capped at limit (normalFeeLimit of the request; nil is unbounded) and, with a balance, at
// floor((balance − value) / gas). When the cap cannot fund the required 12.5% bump of both fields, or
// the full tip over the snapshot's next base fee, nothing is signed: the error wraps
// errReplacementLimitReached, which leads to the capped exact rebroadcast, and the binding says whether
// the balance or the limit stopped it.
func repriceFees(
	previous feeQuote, snapshot *feeSnapshot, gas uint64, balance, value, limit *big.Int, policy horizonPolicy,
) (feeQuote, feeBinding, error) {
	tip := maxBigCopy(bumpFee(previous.tip), tipRule(snapshot, gas, policy))
	target := horizonFee(snapshot.nextBase, policy.maxHorizon, tip)
	return cappedReplacement(previous, snapshot.nextBase, tip, target, gas, balance, value, limit)
}

// cancellationFees prices a same-nonce cancellation, a 21000-gas zero-value self-transfer, under the
// horizon policy (strategy §2.8): tipC = max(bump(tip), tipRule(21000)) and maxFeeC = max(bump(maxFee),
// fee(minHorizon + 1, tipC)), one block beyond the refusal floor, capped at limit (the global fee limit;
// nil is unbounded) and, with a balance, at floor(balance / 21000). It refuses as repriceFees does.
func cancellationFees(
	previous feeQuote, snapshot *feeSnapshot, balance, limit *big.Int, policy horizonPolicy,
) (feeQuote, feeBinding, error) {
	tip := maxBigCopy(bumpFee(previous.tip), tipRule(snapshot, cancellationGasLimit, policy))
	target := horizonFee(snapshot.nextBase, policy.minHorizon+1, tip)
	return cappedReplacement(previous, snapshot.nextBase, tip, target, cancellationGasLimit, balance, new(big.Int), limit)
}

// cappedReplacement completes repriceFees and cancellationFees: maxFee = max(bump(previous maxFee),
// target) under limit and the balance, refused when the cap is below the bump or leaves less than tip over
// nextBase.
func cappedReplacement(
	previous feeQuote, nextBase, tip, target *big.Int, gas uint64, balance, value, limit *big.Int,
) (feeQuote, feeBinding, error) {
	required := bumpFee(previous.maxFee)
	maxFee := maxBigCopy(required, target)
	binding := feeBindingTarget
	if limit != nil && limit.Cmp(maxFee) < 0 {
		maxFee.Set(limit)
		binding = feeBindingCap
	}
	if balance != nil {
		if affordable := affordableMaxFee(balance, value, gas); affordable.Cmp(maxFee) < 0 {
			maxFee.Set(affordable)
			binding = feeBindingBalance
		}
	}
	if maxFee.Cmp(required) < 0 {
		return feeQuote{}, binding, errors.Errorf(
			"%w: %s caps the fee at %s wei per gas, below the required bump %s of %s",
			errReplacementLimitReached, binding, affordableString(maxFee), required, previous.maxFee,
		)
	}
	if tipRoom := new(big.Int).Sub(maxFee, nextBase); tip.Cmp(tipRoom) > 0 {
		return feeQuote{}, binding, errors.Errorf(
			"%w: %s caps the fee at %s wei per gas, which leaves less than the tip %s over the next base fee %s",
			errReplacementLimitReached, binding, maxFee, tip, nextBase,
		)
	}
	return feeQuote{baseFee: new(big.Int).Set(nextBase), tip: new(big.Int).Set(tip), maxFee: maxFee}, binding, nil
}

// clampBig returns value limited to [low, high], as a fresh copy.
func clampBig(value, low, high *big.Int) *big.Int {
	switch {
	case value.Cmp(low) < 0:
		return new(big.Int).Set(low)
	case value.Cmp(high) > 0:
		return new(big.Int).Set(high)
	default:
		return new(big.Int).Set(value)
	}
}
