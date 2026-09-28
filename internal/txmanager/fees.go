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
