package txmanager

import (
	"errors"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

func gwei(value float64) *big.Int { return gweiToWei(value) }

func TestGrow(t *testing.T) {
	saturated := new(big.Int).Set(maxGrownFeeWei)
	for _, tc := range []struct {
		name   string
		x      *big.Int
		blocks uint64
		want   *big.Int
	}{
		{name: "no blocks", x: big.NewInt(1e9), want: big.NewInt(1e9)},
		{name: "one block of 12.5%", x: big.NewInt(1e9), blocks: 1, want: big.NewInt(1_125_000_000)},
		{name: "H=3 floor", x: big.NewInt(1e9), blocks: 2, want: big.NewInt(1_265_625_000)},
		{name: "H=5 pricing horizon", x: big.NewInt(1e9), blocks: 4, want: big.NewInt(1_601_806_640)},
		{name: "H=6 max horizon", x: big.NewInt(1e9), blocks: 5, want: big.NewInt(1_802_032_470)},
		// A testnet base fee of a few wei still grows by the protocol's 1-wei minimum.
		{name: "one-wei testnet growth", x: big.NewInt(1), blocks: 3, want: big.NewInt(4)},
		{name: "seven wei grows by one", x: big.NewInt(7), blocks: 1, want: big.NewInt(8)},
		{name: "zero grows by one", x: new(big.Int), blocks: 1, want: big.NewInt(1)},
		{name: "saturates", x: saturated, blocks: 1_000_000, want: saturated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := new(big.Int).Set(tc.x)
			if got := grow(tc.x, tc.blocks); got.Cmp(tc.want) != 0 {
				t.Fatalf("grow(%s, %d) = %s, want %s", tc.x, tc.blocks, got, tc.want)
			}
			if tc.x.Cmp(before) != 0 {
				t.Fatalf("grow mutated its input to %s", tc.x)
			}
		})
	}
	// However stale the head, growth ends once no balance could fund the result.
	started := time.Now()
	if got := grow(big.NewInt(1), 1<<62); got.Cmp(maxGrownFeeWei) < 0 {
		t.Fatalf("grow of a huge horizon = %s, want saturation", got)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("saturating grow took %s", elapsed)
	}
}

func TestHorizonFee(t *testing.T) {
	tip := big.NewInt(20_000_000)
	for _, tc := range []struct {
		blocks uint64
		want   *big.Int
	}{
		{blocks: 0, want: big.NewInt(1_020_000_000)},
		{blocks: 1, want: big.NewInt(1_020_000_000)},
		{blocks: 2, want: big.NewInt(1_145_000_000)},
		{blocks: 6, want: big.NewInt(1_822_032_470)},
	} {
		if got := horizonFee(big.NewInt(1e9), tc.blocks, tip); got.Cmp(tc.want) != 0 {
			t.Fatalf("horizonFee(1 gwei, %d, 0.02 gwei) = %s, want %s", tc.blocks, got, tc.want)
		}
	}
}

func TestValidHorizon(t *testing.T) {
	tip := big.NewInt(20_000_000)
	nextBase := big.NewInt(1e9)
	for _, tc := range []struct {
		name   string
		maxFee *big.Int
		want   uint64
	}{
		{name: "below the next block", maxFee: big.NewInt(1_019_999_999), want: 0},
		{name: "exactly one block", maxFee: big.NewInt(1_020_000_000), want: 1},
		{name: "just short of two", maxFee: big.NewInt(1_144_999_999), want: 1},
		{name: "exactly two", maxFee: big.NewInt(1_145_000_000), want: 2},
		{name: "legacy 2x", maxFee: big.NewInt(2_020_000_000), want: 6},
		{name: "bounded", maxFee: gwei(1e9), want: maxReportedHorizonBlocks},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validHorizon(nextBase, tip, tc.maxFee, maxReportedHorizonBlocks); got != tc.want {
				t.Fatalf("validHorizon = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAffordableMaxFee(t *testing.T) {
	for _, tc := range []struct {
		name    string
		balance *big.Int
		value   *big.Int
		gas     uint64
		want    *big.Int
	}{
		// 2026-09-28 rfq-first nonces 19/20: 0.00649 ETH over a 3.86M gas limit.
		{name: "rfq-first 09-28", balance: big.NewInt(6_490_000_000_000_000), value: new(big.Int), gas: 3_860_000, want: big.NewInt(1_681_347_150)},
		// 2026-09-28 UniswapX nonces 22-27: 0.0175 ETH over a 4.24M gas limit.
		{name: "uniswapx 09-28", balance: big.NewInt(17_500_000_000_000_000), value: new(big.Int), gas: 4_240_000, want: big.NewInt(4_127_358_490)},
		{name: "value is paid first", balance: big.NewInt(1_000_000), value: big.NewInt(790_000), gas: 21_000, want: big.NewInt(10)},
		{name: "value above balance", balance: big.NewInt(1), value: big.NewInt(2), gas: 21_000, want: big.NewInt(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := affordableMaxFee(tc.balance, tc.value, tc.gas); got.Cmp(tc.want) != 0 {
				t.Fatalf("affordableMaxFee = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestApplyBalanceGuard(t *testing.T) {
	floorTip := big.NewInt(20_000_000) // fees.tipFloorGwei 0.02
	quote := func(baseFee, tip, maxFee *big.Int) feeQuote {
		return feeQuote{baseFee: baseFee, tip: tip, maxFee: maxFee}
	}
	for _, tc := range []struct {
		name        string
		fees        feeQuote
		nextBase    *big.Int
		lag         uint64
		balance     *big.Int
		value       *big.Int
		gas         uint64
		wantErr     error
		wantOneBlk  bool
		wantMaxFee  *big.Int
		wantTip     *big.Int
		wantClamped bool
		wantHorizon uint64
	}{
		{
			// rfq-first nonce 19 on 09-28: the legacy 2×latest cap needed more than the balance held.
			name:     "09-28 rfq-first clamps to the affordable cap",
			fees:     quote(big.NewInt(886_000_000), big.NewInt(10_100_000), big.NewInt(1_782_100_000)),
			nextBase: big.NewInt(886_000_000), balance: big.NewInt(6_490_000_000_000_000), value: new(big.Int), gas: 3_860_000,
			wantMaxFee: big.NewInt(1_681_347_150), wantTip: big.NewInt(10_100_000), wantClamped: true, wantHorizon: 6,
		},
		{
			name:     "funded lane is signed unchanged",
			fees:     quote(big.NewInt(886_000_000), big.NewInt(10_100_000), big.NewInt(1_782_100_000)),
			nextBase: big.NewInt(886_000_000), balance: big.NewInt(100_000_000_000_000_000), value: new(big.Int), gas: 3_860_000,
			wantMaxFee: big.NewInt(1_782_100_000), wantTip: big.NewInt(10_100_000), wantHorizon: 6,
		},
		{
			// UniswapX nonces 22-27 on 09-28: 4.13 gwei affordable against a 5 gwei next base fee.
			name:     "09-28 uniswapx is refused",
			fees:     quote(big.NewInt(4_500_000_000), big.NewInt(10_000_000), big.NewInt(9_010_000_000)),
			nextBase: big.NewInt(5_000_000_000), balance: big.NewInt(17_500_000_000_000_000), value: new(big.Int), gas: 4_240_000,
			wantErr: ErrUnaffordable,
		},
		{
			name:     "one block feasible is refused apart",
			fees:     quote(big.NewInt(4_500_000_000), big.NewInt(10_000_000), big.NewInt(9_010_000_000)),
			nextBase: big.NewInt(5_000_000_000), balance: big.NewInt(22_472_000_000_000_000), value: new(big.Int), gas: 4_240_000,
			wantErr: ErrUnaffordable, wantOneBlk: true,
		},
		{
			// Exactly fee(2) = 5.625 + 0.02 gwei is still two guaranteed blocks.
			name:     "affordable exactly at the floor is signed",
			fees:     quote(big.NewInt(4_500_000_000), big.NewInt(10_000_000), big.NewInt(9_010_000_000)),
			nextBase: big.NewInt(5_000_000_000), balance: new(big.Int).Mul(big.NewInt(5_645_000_000), big.NewInt(4_240_000)), value: new(big.Int), gas: 4_240_000,
			wantMaxFee: big.NewInt(5_645_000_000), wantTip: big.NewInt(10_000_000), wantClamped: true, wantHorizon: 2,
		},
		{
			// A stale head charges its lag: the same balance no longer guarantees two blocks.
			name:     "head lag raises the floor",
			fees:     quote(big.NewInt(4_500_000_000), big.NewInt(10_000_000), big.NewInt(9_010_000_000)),
			nextBase: big.NewInt(5_000_000_000), lag: 1, balance: new(big.Int).Mul(big.NewInt(5_645_000_000), big.NewInt(4_240_000)), value: new(big.Int), gas: 4_240_000,
			wantErr: ErrUnaffordable, wantOneBlk: true,
		},
		{
			name:     "tip is clamped under the affordable cap",
			fees:     quote(big.NewInt(1e9), big.NewInt(3e9), big.NewInt(5e9)),
			nextBase: big.NewInt(1e9), balance: new(big.Int).Mul(big.NewInt(2e9), big.NewInt(21_000)), value: new(big.Int), gas: 21_000,
			wantMaxFee: big.NewInt(2e9), wantTip: big.NewInt(1e9), wantClamped: true, wantHorizon: 6,
		},
		{
			name:     "value comes out of the balance first",
			fees:     quote(big.NewInt(1e9), big.NewInt(1e8), big.NewInt(2_100_000_000)),
			nextBase: big.NewInt(1e9), balance: big.NewInt(1e18), value: big.NewInt(1e18), gas: 21_000,
			wantErr: ErrUnaffordable,
		},
		{
			// The request cap, not the balance, is below the floor: that stays the policy's decision.
			name:     "request cap below the floor is left to the policy",
			fees:     quote(big.NewInt(1e9), big.NewInt(1e7), big.NewInt(1_050_000_000)),
			nextBase: big.NewInt(1e9), balance: big.NewInt(1e18), value: new(big.Int), gas: 21_000,
			wantMaxFee: big.NewInt(1_050_000_000), wantTip: big.NewInt(1e7), wantHorizon: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := guardInput{
				fees: tc.fees, nextBase: tc.nextBase, lag: tc.lag, minHorizon: 2, floorTip: floorTip,
				balance: tc.balance, value: tc.value, gas: tc.gas,
			}
			original := cloneFeeQuote(tc.fees)
			got, err := applyBalanceGuard(input)
			if tc.fees.maxFee.Cmp(original.maxFee) != 0 || tc.fees.tip.Cmp(original.tip) != 0 {
				t.Fatal("applyBalanceGuard mutated the policy quote")
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				if oneBlock := errors.Is(err, errUnaffordableOneBlock); oneBlock != tc.wantOneBlk {
					t.Fatalf("one-block refusal = %t, want %t (%v)", oneBlock, tc.wantOneBlk, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("applyBalanceGuard: %v", err)
			}
			if got.fees.maxFee.Cmp(tc.wantMaxFee) != 0 || got.fees.tip.Cmp(tc.wantTip) != 0 {
				t.Fatalf("fees = maxFee %s tip %s, want %s / %s", got.fees.maxFee, got.fees.tip, tc.wantMaxFee, tc.wantTip)
			}
			if got.clamped != tc.wantClamped || got.horizon != tc.wantHorizon {
				t.Fatalf("clamped %t horizon %d, want %t / %d", got.clamped, got.horizon, tc.wantClamped, tc.wantHorizon)
			}
			if need := requiredBalance(tc.gas, got.fees.maxFee, tc.value); need.Cmp(tc.balance) > 0 {
				t.Fatalf("signed attempt needs %s, more than the balance %s", need, tc.balance)
			}
		})
	}
	if _, err := applyBalanceGuard(guardInput{
		fees: quote(big.NewInt(1), big.NewInt(1), big.NewInt(3)), nextBase: big.NewInt(1), floorTip: floorTip,
		balance: big.NewInt(1e18), value: new(big.Int),
	}); err == nil {
		t.Fatal("zero gas limit was guarded")
	}
}

func TestFeeHistoryNext(t *testing.T) {
	history := func(oldest int64, baseFees ...int64) *ethereum.FeeHistory {
		h := &ethereum.FeeHistory{OldestBlock: big.NewInt(oldest)}
		for _, fee := range baseFees {
			h.BaseFee = append(h.BaseFee, big.NewInt(fee))
		}
		return h
	}
	for _, tc := range []struct {
		name     string
		history  *ethereum.FeeHistory
		wantHead uint64
		wantNext int64
		wantOK   bool
	}{
		{name: "five blocks", history: history(96, 1, 2, 3, 4, 5, 6), wantHead: 100, wantNext: 6, wantOK: true},
		{name: "one block", history: history(100, 5, 6), wantHead: 100, wantNext: 6, wantOK: true},
		{name: "nil history"},
		{name: "no oldest block", history: &ethereum.FeeHistory{BaseFee: []*big.Int{big.NewInt(1), big.NewInt(2)}}},
		{name: "rewards only", history: history(96)},
		{name: "negative next", history: history(96, 1, -1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			head, next, ok := feeHistoryNext(tc.history)
			if ok != tc.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tc.wantOK)
			}
			if ok && (head != tc.wantHead || next.Int64() != tc.wantNext) {
				t.Fatalf("head %d next %s, want %d / %d", head, next, tc.wantHead, tc.wantNext)
			}
		})
	}
}

func TestHeadTimeLag(t *testing.T) {
	now := time.Unix(1_000_000, 500_000_000)
	for _, tc := range []struct {
		name     string
		headTime uint64
		want     uint64
	}{
		{name: "same second", headTime: 1_000_000, want: 0},
		{name: "one slot missed", headTime: 1_000_000 - 12, want: 1},
		{name: "just under two slots", headTime: 1_000_000 - 23, want: 1},
		{name: "two slots", headTime: 1_000_000 - 24, want: 2},
		{name: "future header", headTime: 1_000_100, want: 0},
		{name: "unrepresentable", headTime: 1 << 63, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := headTimeLag(tc.headTime, now, 12*time.Second); got != tc.want {
				t.Fatalf("headTimeLag = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestClassifyFirstAttempt(t *testing.T) {
	first, replacement, cancel := common.Hash{1}, common.Hash{2}, common.Hash{3}
	for _, tc := range []struct {
		name     string
		attempts []txAttempt
		landed   common.Hash
		delay    uint64
		want     firstAttemptOutcome
	}{
		{name: "next block", attempts: []txAttempt{{hash: first}}, landed: first, delay: 1, want: firstAttemptFirst},
		{name: "third block", attempts: []txAttempt{{hash: first}}, landed: first, delay: 3, want: firstAttemptFirst},
		{name: "fourth block", attempts: []txAttempt{{hash: first}}, landed: first, delay: 4, want: firstAttemptLate},
		{name: "replacement landed", attempts: []txAttempt{{hash: first}, {hash: replacement}}, landed: replacement, delay: 2, want: firstAttemptReplaced},
		// A replacement was signed, so the first attempt did not land unassisted even though it won.
		{name: "original won after a replacement", attempts: []txAttempt{{hash: first}, {hash: replacement}}, landed: first, delay: 2, want: firstAttemptReplaced},
		{name: "cancellation landed", attempts: []txAttempt{{hash: first}, {hash: cancel, cancellation: true}}, landed: cancel, delay: 30, want: firstAttemptCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, delay := classifyFirstAttempt(tc.attempts, tc.landed, tc.delay)
			if got != tc.want || delay != tc.delay {
				t.Fatalf("classifyFirstAttempt = %s/%d, want %s/%d", got, delay, tc.want, tc.delay)
			}
		})
	}
}
