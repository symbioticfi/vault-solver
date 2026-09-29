package txmanager

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

// testPolicy is the horizon policy at its defaults: horizons 2 / 5 / 6, tips 0.02 / 0.1 / 0.2–15 gwei and
// a three-block reward window.
func testPolicy() horizonPolicy {
	return newHorizonPolicy(Config{}.WithDefaults().Fees)
}

// testSnapshot is a fee snapshot at next base fee nextBase whose blocks, oldest first, ended at block 100
// under a 36M block gas limit.
func testSnapshot(nextBase *big.Int, blocks ...feeBlock) *feeSnapshot {
	for i := range blocks {
		blocks[i].number = 100 - uint64(len(blocks)-1-i)
	}
	return &feeSnapshot{
		header:      &types.Header{Number: big.NewInt(100), GasLimit: 36_000_000, BaseFee: new(big.Int).Set(nextBase)},
		historyHead: 100,
		nextBase:    new(big.Int).Set(nextBase),
		blocks:      blocks,
	}
}

// block is a snapshot block with this gas-used ratio and run-percentile reward (nil: none reported).
func block(ratio float64, reward *big.Int) feeBlock {
	return feeBlock{baseFee: big.NewInt(1e9), gasUsedRatio: ratio, runReward: reward, legacyReward: reward}
}

// roomy is a snapshot block with room for any fill.
func roomy() feeBlock { return block(0.5, gwei(0.001)) }

// full is a snapshot block without room for a 4M-gas fill, with this reward.
func full(reward *big.Int) feeBlock { return block(0.95, reward) }

func TestRoom(t *testing.T) {
	for _, tc := range []struct {
		name     string
		gasLimit uint64
		ratio    float64
		gas      uint64
		want     bool
	}{
		{name: "half full block", gasLimit: 36_000_000, ratio: 0.5, gas: 4_000_000, want: true},
		{name: "remaining equals the gas limit", gasLimit: 16_000_000, ratio: 0.75, gas: 4_000_000, want: true},
		{name: "one gas short", gasLimit: 16_000_000, ratio: 0.75, gas: 4_000_001},
		{name: "full block", gasLimit: 36_000_000, ratio: 1, gas: 21_000},
		{name: "measured against our own gas limit", gasLimit: 36_000_000, ratio: 0.95, gas: 1_000_000, want: true},
		{name: "a large fill needs an emptier block", gasLimit: 36_000_000, ratio: 0.95, gas: 4_000_000},
		{name: "no gas always fits", gasLimit: 36_000_000, ratio: 1, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := room(tc.gasLimit, tc.ratio, tc.gas); got != tc.want {
				t.Fatalf("room(%d, %v, %d) = %t, want %t", tc.gasLimit, tc.ratio, tc.gas, got, tc.want)
			}
		})
	}
}

func TestTipRule(t *testing.T) {
	const fill = 4_000_000
	for _, tc := range []struct {
		name   string
		blocks []feeBlock
		gas    uint64
		want   *big.Int
	}{
		{name: "blocks with room pay the floor", blocks: []feeBlock{roomy(), roomy(), roomy()}, gas: fill, want: gwei(0.02)},
		{
			name:   "a zero-reward snapshot with room gives the floor",
			blocks: []feeBlock{block(0, new(big.Int)), block(0, new(big.Int))}, gas: fill, want: gwei(0.02),
		},
		{name: "latest block full", blocks: []feeBlock{roomy(), roomy(), full(gwei(9))}, gas: fill, want: gwei(0.1)},
		{name: "previous block full", blocks: []feeBlock{roomy(), full(gwei(9)), roomy()}, gas: fill, want: gwei(0.1)},
		{
			name:   "a demand run follows the largest reward of the latest three blocks",
			blocks: []feeBlock{full(gwei(9)), full(gwei(1)), full(gwei(3)), full(gwei(2))}, gas: fill, want: gwei(3),
		},
		{
			name:   "a zero-reward demand run gives the congested floor",
			blocks: []feeBlock{full(new(big.Int)), full(new(big.Int)), full(new(big.Int))}, gas: fill, want: gwei(0.2),
		},
		{name: "a demand run without rewards gives the congested floor", blocks: []feeBlock{full(nil), full(nil)}, gas: fill, want: gwei(0.2)},
		{name: "a demand run is capped", blocks: []feeBlock{full(gwei(40)), full(gwei(1))}, gas: fill, want: gwei(15)},
		{name: "full blocks have room for a cancellation", blocks: []feeBlock{full(gwei(9)), full(gwei(9))}, gas: cancellationGasLimit, want: gwei(0.02)},
		{name: "one known block that is full", blocks: []feeBlock{full(gwei(9))}, gas: fill, want: gwei(0.1)},
		{name: "no gas limit has room everywhere", blocks: []feeBlock{full(gwei(9)), full(gwei(9))}, want: gwei(0.02)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tipRule(testSnapshot(big.NewInt(1e9), tc.blocks...), tc.gas, testPolicy()); got.Cmp(tc.want) != 0 {
				t.Fatalf("tipRule = %s, want %s", got, tc.want)
			}
		})
	}
	// The window is fees.congestedRewardBlocks long.
	policy := testPolicy()
	policy.rewardBlocks = 4
	snapshot := testSnapshot(big.NewInt(1e9), full(gwei(9)), full(gwei(1)), full(gwei(3)), full(gwei(2)))
	if got := tipRule(snapshot, fill, policy); got.Cmp(gwei(9)) != 0 {
		t.Fatalf("tipRule over four blocks = %s, want 9 gwei", got)
	}
}

func TestInitialFees(t *testing.T) {
	const gas = 4_000_000
	nextBase := big.NewInt(1e9)
	perGas := func(wei *big.Int) *big.Int { return new(big.Int).Mul(wei, big.NewInt(gas)) }
	oneEth := big.NewInt(1e18)
	floor := func(lag uint64) *big.Int { return horizonFee(nextBase, 2+lag, gwei(0.02)) }
	for _, tc := range []struct {
		name        string
		blocks      []feeBlock
		balance     *big.Int
		value       *big.Int
		capLimit    *big.Int
		lag         uint64
		wantMaxFee  *big.Int
		wantTip     *big.Int
		wantHorizon uint64
		wantBinding feeBinding
		wantErr     error
	}{
		{
			// fee(6, 0.02 gwei) = 1.8020·pb + tip, about today's 2×latest.
			name: "a funded lane signs the six-block target", balance: oneEth, capLimit: gwei(39.5),
			wantMaxFee: big.NewInt(1_822_032_470), wantTip: gwei(0.02), wantHorizon: 6, wantBinding: feeBindingTarget,
		},
		{
			name: "without the balance guard", capLimit: gwei(39.5),
			wantMaxFee: big.NewInt(1_822_032_470), wantTip: gwei(0.02), wantHorizon: 6, wantBinding: feeBindingTarget,
		},
		{
			name: "no fee limit", balance: oneEth,
			wantMaxFee: big.NewInt(1_822_032_470), wantTip: gwei(0.02), wantHorizon: 6, wantBinding: feeBindingTarget,
		},
		{
			name: "the request cap binds above the floor", balance: oneEth, capLimit: gwei(1.5),
			wantMaxFee: gwei(1.5), wantTip: gwei(0.02), wantHorizon: 4, wantBinding: feeBindingCap,
		},
		{
			name: "a request cap below the floor is a fee limit, not a refusal", balance: oneEth,
			capLimit: new(big.Int).Sub(floor(0), big.NewInt(1)), wantBinding: feeBindingCap, wantErr: errFeeLimitReached,
		},
		{
			name: "the balance clamps the fee cap", balance: perGas(gwei(1.3)), capLimit: gwei(39.5),
			wantMaxFee: gwei(1.3), wantTip: gwei(0.02), wantHorizon: 3, wantBinding: feeBindingBalance,
		},
		{
			name: "the balance exactly at the floor", balance: perGas(floor(0)), capLimit: gwei(39.5),
			wantMaxFee: floor(0), wantTip: gwei(0.02), wantHorizon: 2, wantBinding: feeBindingBalance,
		},
		{
			name: "one wei below the floor funds one block", balance: new(big.Int).Sub(perGas(floor(0)), big.NewInt(gas)),
			capLimit: gwei(39.5), wantBinding: feeBindingBalance, wantErr: errUnaffordableOneBlock,
		},
		{
			name: "below the next block is unaffordable", balance: perGas(gwei(1)), capLimit: gwei(39.5),
			wantBinding: feeBindingBalance, wantErr: ErrUnaffordable,
		},
		{
			name: "head lag raises the floor", balance: perGas(gwei(1.3)), capLimit: gwei(39.5), lag: 2,
			wantBinding: feeBindingBalance, wantErr: errUnaffordableOneBlock,
		},
		{
			name: "the value is paid first", balance: new(big.Int).Add(perGas(gwei(1.3)), oneEth), value: oneEth,
			capLimit: gwei(39.5), wantMaxFee: gwei(1.3), wantTip: gwei(0.02), wantHorizon: 3, wantBinding: feeBindingBalance,
		},
		{
			name: "a value above the balance is unaffordable", balance: oneEth, value: new(big.Int).Add(oneEth, big.NewInt(1)),
			capLimit: gwei(39.5), wantBinding: feeBindingBalance, wantErr: ErrUnaffordable,
		},
		{
			// A 15 gwei demand-run tip does not fit under a 5 gwei cap over a 1 gwei base fee.
			name: "the tip is clamped under the cap", blocks: []feeBlock{full(gwei(30)), full(gwei(30))}, balance: oneEth,
			capLimit: gwei(5), wantMaxFee: gwei(5), wantTip: gwei(4), wantHorizon: 14, wantBinding: feeBindingCap,
		},
		{
			// A lag beyond maxHorizon − minHorizon puts the floor above fee(6): the target rises to it.
			name: "the target never sits below the floor", balance: oneEth, capLimit: gwei(39.5), lag: 5,
			wantMaxFee: floor(5), wantTip: gwei(0.02), wantHorizon: 7, wantBinding: feeBindingTarget,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocks := tc.blocks
			if blocks == nil {
				blocks = []feeBlock{roomy(), roomy(), roomy()}
			}
			value := tc.value
			if value == nil {
				value = new(big.Int)
			}
			quote, err := initialFees(testSnapshot(nextBase, blocks...), gas, tc.balance, value, tc.capLimit, tc.lag, testPolicy())
			fees := quote.fees
			if quote.binding != tc.wantBinding {
				t.Fatalf("binding = %q, want %q", quote.binding, tc.wantBinding)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				if errors.Is(tc.wantErr, ErrUnaffordable) != errors.Is(err, ErrUnaffordable) ||
					errors.Is(tc.wantErr, errUnaffordableOneBlock) != errors.Is(err, errUnaffordableOneBlock) {
					t.Fatalf("error = %v, want exactly %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("initialFees: %v", err)
			}
			if fees.maxFee.Cmp(tc.wantMaxFee) != 0 || fees.tip.Cmp(tc.wantTip) != 0 || fees.baseFee.Cmp(nextBase) != 0 {
				t.Fatalf("fees = max %s tip %s base %s, want max %s tip %s base %s",
					fees.maxFee, fees.tip, fees.baseFee, tc.wantMaxFee, tc.wantTip, nextBase)
			}
			if quote.horizon != tc.wantHorizon {
				t.Fatalf("horizon = %d, want %d", quote.horizon, tc.wantHorizon)
			}
		})
	}
	if _, err := initialFees(testSnapshot(nextBase, roomy()), 0, oneEth, new(big.Int), nil, 0, testPolicy()); err == nil {
		t.Fatal("initialFees accepted a zero gas limit")
	}
}

func TestInitialFeesDoNotModifyTheSnapshot(t *testing.T) {
	snapshot := testSnapshot(big.NewInt(1e9), full(gwei(30)), full(gwei(30)))
	quote, err := initialFees(snapshot, 4_000_000, nil, new(big.Int), gwei(5), 0, testPolicy())
	if err != nil {
		t.Fatalf("initialFees: %v", err)
	}
	fees := quote.fees
	fees.baseFee.SetInt64(7)
	fees.tip.SetInt64(7)
	if snapshot.nextBase.Cmp(big.NewInt(1e9)) != 0 || snapshot.blocks[1].runReward.Cmp(gwei(30)) != 0 {
		t.Fatalf("initialFees shares its result with the cached snapshot: next base %s, reward %s",
			snapshot.nextBase, snapshot.blocks[1].runReward)
	}
}

func TestPricingFee(t *testing.T) {
	const referenceGas = 4_400_000
	policy := testPolicy()
	t.Run("formula", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			blocks []feeBlock
			lag    uint64
			limit  *big.Int
			want   *big.Int
		}{
			// bump(fee(5, 0.02 gwei)) = 1.125 × (1.6018·pb + tip), about 1.80·pb.
			{name: "blocks with room", want: big.NewInt(1_824_532_470)},
			{name: "a lagging snapshot prices from the real next block", lag: 1, want: bumpFee(horizonFee(big.NewInt(1e9), 6, gwei(0.02)))},
			{
				name: "a demand run for the reference fill", blocks: []feeBlock{full(gwei(3)), full(gwei(3))},
				want: bumpFee(horizonFee(big.NewInt(1e9), 5, gwei(3))),
			},
			{name: "the fee limit caps the price", limit: gwei(1.7), want: gwei(1.7)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				blocks := tc.blocks
				if blocks == nil {
					blocks = []feeBlock{roomy(), roomy()}
				}
				got, err := pricingFee(testSnapshot(big.NewInt(1e9), blocks...), tc.lag, referenceGas, tc.limit, policy)
				if err != nil {
					t.Fatalf("pricingFee: %v", err)
				}
				if got.Cmp(tc.want) != 0 {
					t.Fatalf("pricingFee = %s, want %s", got, tc.want)
				}
			})
		}
	})
	t.Run("a limit the fill cannot clear its floor under fails", func(t *testing.T) {
		// reserveFeeBump(1.2 gwei) = 1.0667 gwei, below fee(2, 0.02 gwei) = 1.145 gwei.
		if _, err := pricingFee(testSnapshot(big.NewInt(1e9), roomy()), 0, referenceGas, gwei(1.2), policy); !errors.Is(err, errFeeLimitReached) {
			t.Fatalf("error = %v, want errFeeLimitReached", err)
		}
	})
	// The quote-to-fill invariant (strategy §2.9): a fill whose Request.MaxFeePerGas is the quoted price
	// is still signed after pricingHorizon − minHorizon = 3 blocks of maximum base-fee growth, and not after
	// 4, on mainnet and on a testnet whose base fee is a few wei.
	for _, quoted := range []*big.Int{big.NewInt(1e9), big.NewInt(1)} {
		t.Run("three-block drift from a "+quoted.String()+" wei base fee", func(t *testing.T) {
			priced, err := pricingFee(testSnapshot(quoted, roomy(), roomy()), 0, referenceGas, gwei(44.44), policy)
			if err != nil {
				t.Fatalf("pricingFee: %v", err)
			}
			for drift := range uint64(5) {
				fill := testSnapshot(grow(quoted, drift), roomy(), roomy())
				_, err := initialFees(fill, referenceGas, nil, new(big.Int), reserveFeeBump(priced), 0, policy)
				if drift <= 3 && err != nil {
					t.Fatalf("fill after %d blocks of growth refused: %v", drift, err)
				}
				if drift == 4 && !errors.Is(err, errFeeLimitReached) {
					t.Fatalf("fill after 4 blocks of growth = %v, want errFeeLimitReached", err)
				}
			}
		})
	}
}

func TestRepriceFees(t *testing.T) {
	const gas = 4_000_000
	previous := feeQuote{baseFee: big.NewInt(1e9), tip: gwei(0.02), maxFee: big.NewInt(1_822_032_470)}
	bumpedMax := bumpFee(previous.maxFee) // 2_049_786_529
	oneEth := big.NewInt(1e18)
	for _, tc := range []struct {
		name        string
		nextBase    *big.Int
		blocks      []feeBlock
		balance     *big.Int
		limit       *big.Int
		wantMaxFee  *big.Int
		wantTip     *big.Int
		wantBinding feeBinding
		wantErr     error
	}{
		{
			name: "the bump dominates a quiet target", nextBase: big.NewInt(1e9), balance: oneEth, limit: gwei(44.44),
			wantMaxFee: bumpedMax, wantTip: gwei(0.0225), wantBinding: feeBindingTarget,
		},
		{
			name: "a demand run escalates the tip and the target", nextBase: big.NewInt(1e9),
			blocks: []feeBlock{full(gwei(5)), full(gwei(5))}, balance: oneEth, limit: gwei(44.44),
			wantMaxFee: horizonFee(big.NewInt(1e9), 6, gwei(5)), wantTip: gwei(5), wantBinding: feeBindingTarget,
		},
		{
			name: "a risen base fee reprices to the six-block target", nextBase: gwei(3), balance: oneEth, limit: gwei(44.44),
			wantMaxFee: horizonFee(gwei(3), 6, gwei(0.0225)), wantTip: gwei(0.0225), wantBinding: feeBindingTarget,
		},
		{
			name: "the request limit caps the target", nextBase: gwei(3), balance: oneEth, limit: gwei(4),
			wantMaxFee: gwei(4), wantTip: gwei(0.0225), wantBinding: feeBindingCap,
		},
		{
			name: "a limit below the bump is not signed", nextBase: big.NewInt(1e9), balance: oneEth,
			limit: new(big.Int).Sub(bumpedMax, big.NewInt(1)), wantBinding: feeBindingCap, wantErr: errReplacementLimitReached,
		},
		{
			name: "a balance below the bump is not signed", nextBase: big.NewInt(1e9), limit: gwei(44.44),
			balance: new(big.Int).Mul(gwei(2), big.NewInt(gas)), wantBinding: feeBindingBalance, wantErr: errReplacementLimitReached,
		},
		{
			name: "a cap without room for the tip is not signed", nextBase: gwei(3), balance: oneEth, limit: gwei(3.01),
			wantBinding: feeBindingCap, wantErr: errReplacementLimitReached,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocks := tc.blocks
			if blocks == nil {
				blocks = []feeBlock{roomy(), roomy()}
			}
			fees, binding, err := repriceFees(previous, testSnapshot(tc.nextBase, blocks...), gas, tc.balance, new(big.Int), tc.limit, testPolicy())
			if binding != tc.wantBinding {
				t.Fatalf("binding = %q, want %q", binding, tc.wantBinding)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("repriceFees: %v", err)
			}
			if fees.maxFee.Cmp(tc.wantMaxFee) != 0 || fees.tip.Cmp(tc.wantTip) != 0 || fees.baseFee.Cmp(tc.nextBase) != 0 {
				t.Fatalf("fees = max %s tip %s base %s, want max %s tip %s", fees.maxFee, fees.tip, fees.baseFee, tc.wantMaxFee, tc.wantTip)
			}
		})
	}
}

func TestCancellationFees(t *testing.T) {
	previous := feeQuote{baseFee: big.NewInt(1e9), tip: gwei(0.02), maxFee: big.NewInt(1_822_032_470)}
	bumpedMax := bumpFee(previous.maxFee)
	for _, tc := range []struct {
		name        string
		nextBase    *big.Int
		blocks      []feeBlock
		balance     *big.Int
		limit       *big.Int
		wantMaxFee  *big.Int
		wantTip     *big.Int
		wantBinding feeBinding
		wantErr     error
	}{
		{
			name: "the bump dominates fee(3)", nextBase: big.NewInt(1e9), limit: gwei(50),
			wantMaxFee: bumpedMax, wantTip: gwei(0.0225), wantBinding: feeBindingTarget,
		},
		{
			// Blocks full for the fill still have room for 21000 gas: the cancellation keeps the floor tip rule.
			name: "blocks full for the fill have room for the cancellation", nextBase: big.NewInt(1e9),
			blocks: []feeBlock{full(gwei(9)), full(gwei(9))}, limit: gwei(50),
			wantMaxFee: bumpedMax, wantTip: gwei(0.0225), wantBinding: feeBindingTarget,
		},
		{
			name: "a risen base fee targets three blocks", nextBase: gwei(5), limit: gwei(50),
			wantMaxFee: big.NewInt(6_350_625_000), wantTip: gwei(0.0225), wantBinding: feeBindingTarget,
		},
		{
			name: "the global limit caps it", nextBase: gwei(5), limit: gwei(6),
			wantMaxFee: gwei(6), wantTip: gwei(0.0225), wantBinding: feeBindingCap,
		},
		{
			name: "the balance over 21000 gas caps it", nextBase: gwei(5), limit: gwei(50),
			balance:    new(big.Int).Mul(gwei(5.5), big.NewInt(cancellationGasLimit)),
			wantMaxFee: gwei(5.5), wantTip: gwei(0.0225), wantBinding: feeBindingBalance,
		},
		{
			name: "a balance below the bump is not signed", nextBase: big.NewInt(1e9), limit: gwei(50),
			balance: new(big.Int).Mul(gwei(2), big.NewInt(cancellationGasLimit)), wantBinding: feeBindingBalance,
			wantErr: errReplacementLimitReached,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocks := tc.blocks
			if blocks == nil {
				blocks = []feeBlock{roomy(), roomy()}
			}
			fees, binding, err := cancellationFees(previous, testSnapshot(tc.nextBase, blocks...), tc.balance, tc.limit, testPolicy())
			if binding != tc.wantBinding {
				t.Fatalf("binding = %q, want %q", binding, tc.wantBinding)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("cancellationFees: %v", err)
			}
			if fees.maxFee.Cmp(tc.wantMaxFee) != 0 || fees.tip.Cmp(tc.wantTip) != 0 {
				t.Fatalf("fees = max %s tip %s, want max %s tip %s", fees.maxFee, fees.tip, tc.wantMaxFee, tc.wantTip)
			}
		})
	}
}
