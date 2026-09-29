package txmanager

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// shadowTestGas is the virtual fill's gas in these tests: a block has room for it at a gas-used ratio of at most
// 8/9 of the 36M block gas limit.
const shadowTestGas = 4_000_000

// shadowChain describes a chain block by block for the shadow evaluator: every block has a 1 gwei base fee,
// is half full, and pays 0.001 gwei at both reward percentiles, unless the maps say otherwise.
type shadowChain struct {
	base   map[uint64]*big.Int
	ratio  map[uint64]float64
	run    map[uint64]*big.Int // run-percentile (p50) reward
	legacy map[uint64]*big.Int // p25 reward
	// noRewards drops every reward, as a node that omits them does.
	noRewards bool
}

func newShadowChain() *shadowChain {
	return &shadowChain{
		base: map[uint64]*big.Int{}, ratio: map[uint64]float64{}, run: map[uint64]*big.Int{}, legacy: map[uint64]*big.Int{},
	}
}

func (c *shadowChain) baseFee(number uint64) *big.Int {
	if fee, ok := c.base[number]; ok {
		return new(big.Int).Set(fee)
	}
	return big.NewInt(1e9)
}

// full marks blocks as having no room for the fill, paying reward at the run percentile.
func (c *shadowChain) full(reward *big.Int, numbers ...uint64) *shadowChain {
	for _, number := range numbers {
		c.ratio[number] = 0.95
		c.run[number] = reward
	}
	return c
}

// snapshot is the fee snapshot a read at head returns: six blocks up to it, and the header at head (or at
// headerOffset blocks from it).
func (c *shadowChain) snapshot(head uint64, headerOffset int64) *feeSnapshot {
	snapshot := &feeSnapshot{
		header: &types.Header{
			Number:   new(big.Int).SetUint64(uint64(int64(head) + headerOffset)),
			GasLimit: 36_000_000,
			BaseFee:  c.baseFee(head),
		},
		historyHead: head,
		nextBase:    c.baseFee(head + 1),
	}
	for number := head - feeSnapshotBlocks + 1; number <= head; number++ {
		block := feeBlock{number: number, baseFee: c.baseFee(number), gasUsedRatio: 0.5}
		if ratio, ok := c.ratio[number]; ok {
			block.gasUsedRatio = ratio
		}
		if !c.noRewards {
			block.runReward, block.legacyReward = gwei(0.001), gwei(0.001)
			if reward, ok := c.run[number]; ok {
				block.runReward = reward
			}
			if reward, ok := c.legacy[number]; ok {
				block.legacyReward = reward
			}
		}
		snapshot.blocks = append(snapshot.blocks, block)
	}
	return snapshot
}

// shadowTestEvaluator is the evaluator of a manager with a 50 gwei global cap and the given config.
func shadowTestEvaluator(t *testing.T, cfg Config) *shadowEvaluator {
	t.Helper()
	cfg.MaxFeeGwei = 50
	return New(newMockBackend(), mustSigner(t), big.NewInt(1), cfg, logr.Discard()).newShadowEvaluator()
}

// feed observes the chain's heads from..to, each with lane, and returns every score.
func (e *shadowEvaluator) feed(chain *shadowChain, from, to uint64, lane *shadowLane) []shadowScore {
	var scores []shadowScore
	for head := from; head <= to; head++ {
		observed, _ := e.observe(chain.snapshot(head, 0), lane)
		scores = append(scores, observed...)
	}
	return scores
}

func TestShadowEvaluatorScoresBothPolicies(t *testing.T) {
	const start = 100
	oneEth := big.NewInt(1e18)
	funded := &shadowLane{gas: shadowTestGas, balance: oneEth}
	for _, tc := range []struct {
		name       string
		chain      func() *shadowChain
		cfg        Config
		lane       *shadowLane
		wantLegacy shadowOutcome
		wantHoriz  shadowOutcome
	}{
		{
			name: "blocks with room include both first attempts at once", chain: newShadowChain, lane: funded,
			wantLegacy: shadowFirst1, wantHoriz: shadowFirst1,
		},
		{
			// The horizon floor tip 0.02 gwei beats a full block's 0.01 gwei; the legacy p25 tip 0.001 gwei
			// does not, and waits for the next block with room.
			name:  "a full block with a small reward",
			chain: func() *shadowChain { return newShadowChain().full(gwei(0.01), start+1) }, lane: funded,
			wantLegacy: shadowFirst2, wantHoriz: shadowFirst1,
		},
		{
			name:  "a full block neither tip wins",
			chain: func() *shadowChain { return newShadowChain().full(gwei(0.05), start+1) }, lane: funded,
			wantLegacy: shadowFirst2, wantHoriz: shadowFirst2,
		},
		{
			// Already in a demand run at the send, the horizon tip follows the run's 5 gwei reward; the legacy
			// p25 tip (1 gwei) and its 30 s bump never reach it.
			name: "a demand run at the send",
			chain: func() *shadowChain {
				c := newShadowChain().full(gwei(5), start-1, start, start+1, start+2, start+3)
				for number := start - 5; number <= start+3; number++ {
					c.legacy[uint64(number)] = gwei(1)
				}
				return c
			},
			lane:       funded,
			wantLegacy: shadowMissed3, wantHoriz: shadowFirst1,
		},
		{
			// A run that starts after the send: the horizon evaluation holds after one full block and reprices
			// for congestion after the second, which the third block includes.
			name:  "a demand run after the send escalates the horizon tip",
			chain: func() *shadowChain { return newShadowChain().full(gwei(5), start+1, start+2, start+3) }, lane: funded,
			wantLegacy: shadowMissed3, wantHoriz: shadowReplaced,
		},
		{
			// A 15 s replacement interval bumps before the second block, which then includes the replacement.
			name:  "a legacy bump before the block that includes it",
			chain: func() *shadowChain { return newShadowChain().full(gwei(0.05), start+1) },
			cfg:   Config{ReplacementInterval: 15 * time.Second}, lane: funded,
			wantLegacy: shadowReplaced, wantHoriz: shadowFirst2,
		},
		{
			// The balance funds 1 gwei per gas, below the 2-block floor fee(2, 0.02 gwei) = 1.145 gwei.
			name: "an unfundable lane refuses under both policies", chain: newShadowChain,
			lane:       &shadowLane{gas: shadowTestGas, balance: new(big.Int).Mul(big.NewInt(shadowTestGas), gwei(1))},
			wantLegacy: shadowRefused, wantHoriz: shadowRefused,
		},
		{
			// The guard clamps both fee caps to 1.5 gwei, which still includes them at a flat base fee.
			name: "a clamped lane still lands", chain: newShadowChain,
			lane:       &shadowLane{gas: shadowTestGas, balance: new(big.Int).Mul(big.NewInt(shadowTestGas), gwei(1.5))},
			wantLegacy: shadowFirst1, wantHoriz: shadowFirst1,
		},
		{
			// Without the balance guard nothing limits the fee cap.
			name: "without the balance guard", chain: newShadowChain, lane: &shadowLane{gas: shadowTestGas},
			wantLegacy: shadowFirst1, wantHoriz: shadowFirst1,
		},
		{
			// The node reports a next base fee (3 gwei) above the legacy cap priced from the latest block's
			// (2×1 gwei + tip): the legacy attempt is invalid until its bump, priced at head 102, lands. The
			// horizon price starts from the next base fee itself.
			name: "a next base fee above the legacy cap",
			chain: func() *shadowChain {
				c := newShadowChain()
				for number := uint64(start + 1); number <= start+3; number++ {
					c.base[number] = gwei(3)
				}
				return c
			},
			lane: funded, wantLegacy: shadowReplaced, wantHoriz: shadowFirst1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evaluator := shadowTestEvaluator(t, tc.cfg)
			chain := tc.chain()
			if scores := evaluator.feed(chain, start, start+2, tc.lane); len(scores) != 0 {
				t.Fatalf("scored before the third block: %v", scores)
			}
			scores, _ := evaluator.observe(chain.snapshot(start+3, 0), tc.lane)
			want := []shadowScore{{FeePolicyLegacy, tc.wantLegacy}, {FeePolicyHorizon, tc.wantHoriz}}
			if len(scores) != len(want) || scores[0] != want[0] || scores[1] != want[1] {
				t.Fatalf("scores = %v, want %v", scores, want)
			}
		})
	}
}

func TestShadowEvaluatorStartsOneFillPerFreshHead(t *testing.T) {
	const start = 100
	lane := &shadowLane{gas: shadowTestGas, balance: big.NewInt(1e18)}
	count := func(scores []shadowScore) int { return len(scores) / len(shadowPolicies) }

	t.Run("one fill per head", func(t *testing.T) {
		evaluator := shadowTestEvaluator(t, Config{})
		if got := count(evaluator.feed(newShadowChain(), start, start+9, lane)); got != 7 {
			t.Fatalf("fills scored over ten heads = %d, want 7 (heads 100..106)", got)
		}
	})
	t.Run("an older or repeated head is ignored", func(t *testing.T) {
		evaluator := shadowTestEvaluator(t, Config{})
		chain := newShadowChain()
		evaluator.feed(chain, start, start+1, lane)
		for _, head := range []uint64{start + 1, start} {
			if scores, advanced := evaluator.observe(chain.snapshot(head, 0), lane); advanced || len(scores) != 0 {
				t.Fatalf("head %d: advanced %t with scores %v", head, advanced, scores)
			}
		}
		if got := len(evaluator.pending); got != 2 {
			t.Fatalf("waiting fills = %d, want 2", got)
		}
	})
	for _, tc := range []struct {
		name         string
		lane         *shadowLane
		headerOffset int64
	}{
		{name: "no lane (no reference gas or no balance yet)"},
		{name: "no reference gas", lane: &shadowLane{balance: big.NewInt(1e18)}},
		{name: "a head a send would wait out", lane: &shadowLane{gas: shadowTestGas, lag: defaultMaxHeadLagBlocks + 1}},
		{name: "a fee history a block behind its header", lane: lane, headerOffset: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evaluator := shadowTestEvaluator(t, Config{})
			if _, advanced := evaluator.observe(newShadowChain().snapshot(start, tc.headerOffset), tc.lane); !advanced {
				t.Fatal("a newer snapshot did not advance the evaluator")
			}
			if len(evaluator.pending) != 0 {
				t.Fatalf("started %v", evaluator.pending)
			}
		})
	}
}

func TestShadowEvaluatorDropsFillsItCannotScore(t *testing.T) {
	const start = 100
	lane := &shadowLane{gas: shadowTestGas, balance: big.NewInt(1e18)}

	t.Run("blocks lost to a gap in the reads", func(t *testing.T) {
		evaluator := shadowTestEvaluator(t, Config{})
		chain := newShadowChain()
		evaluator.feed(chain, start, start, lane)
		// Heads 101..109 were never read: the snapshot at 110 covers 105..110 only.
		if scores, _ := evaluator.observe(chain.snapshot(start+10, 0), lane); len(scores) != 0 {
			t.Fatalf("scored %v across the gap", scores)
		}
		if len(evaluator.pending) != 1 || evaluator.pending[0].head != start+10 {
			t.Fatalf("waiting fills = %v, want only the one at 110", evaluator.pending)
		}
	})
	t.Run("a fee history without rewards cannot price a legacy fill", func(t *testing.T) {
		evaluator := shadowTestEvaluator(t, Config{})
		chain := newShadowChain()
		chain.noRewards = true
		scores := evaluator.feed(chain, start, start+3, lane)
		if len(scores) != 1 || scores[0] != (shadowScore{FeePolicyHorizon, shadowFirst1}) {
			t.Fatalf("scores = %v, want only horizon first1", scores)
		}
	})
	t.Run("the store keeps only the blocks it needs", func(t *testing.T) {
		evaluator := shadowTestEvaluator(t, Config{})
		evaluator.feed(newShadowChain(), 1000, 1200, lane)
		// A waiting fill needs its view (5 blocks) and three blocks after it; three fills wait.
		if got, limit := len(evaluator.blocks), int(evaluator.viewBlocks())+feeSnapshotBlocks; got > limit {
			t.Fatalf("store holds %d blocks, want at most %d", got, limit)
		}
	})
}

func TestShadowIncludes(t *testing.T) {
	const gas = shadowTestGas
	fees := feeQuote{baseFee: gwei(1), tip: gwei(0.1), maxFee: gwei(2)}
	for _, tc := range []struct {
		name  string
		fees  feeQuote
		block feeBlock
		want  bool
	}{
		{name: "room", fees: fees, block: feeBlock{baseFee: gwei(1), gasUsedRatio: 0.5}, want: true},
		{name: "base fee above the fee cap", fees: fees, block: feeBlock{baseFee: gwei(2.1), gasUsedRatio: 0.5}},
		{name: "base fee at the fee cap", fees: fees, block: feeBlock{baseFee: gwei(2), gasUsedRatio: 0.5}, want: true},
		{name: "no room, tip at the reward", fees: fees, block: feeBlock{baseFee: gwei(1), gasUsedRatio: 0.95, runReward: gwei(0.1)}, want: true},
		{name: "no room, tip below the reward", fees: fees, block: feeBlock{baseFee: gwei(1), gasUsedRatio: 0.95, runReward: gwei(0.11)}},
		{
			// Only maxFee − baseFee = 0.05 gwei of the 0.1 gwei tip is paid at a 1.95 gwei base fee.
			name: "the effective tip is capped by the fee cap", fees: fees,
			block: feeBlock{baseFee: gwei(1.95), gasUsedRatio: 0.95, runReward: gwei(0.06)},
		},
		{name: "no room and no reward", fees: fees, block: feeBlock{baseFee: gwei(1), gasUsedRatio: 0.95}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shadowIncludes(tc.fees, tc.block, 36_000_000, gas); got != tc.want {
				t.Fatalf("shadowIncludes = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestLegacyBumpsBefore(t *testing.T) {
	const block = 12 * time.Second
	for _, tc := range []struct {
		name     string
		interval time.Duration
		want     [3]int // before blocks 1, 2 and 3
	}{
		{name: "30 s (RFQ, LI.FI)", interval: 30 * time.Second, want: [3]int{0, 0, 1}},
		{name: "15 s (UniswapX)", interval: 15 * time.Second, want: [3]int{0, 1, 2}},
		{name: "exactly one block", interval: block, want: [3]int{0, 1, 2}},
		{name: "faster than blocks", interval: 5 * time.Second, want: [3]int{2, 4, 7}},
		{name: "no interval", want: [3]int{0, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k := range tc.want {
				if got := legacyBumpsBefore(uint64(k+1), block, tc.interval); got != tc.want[k] {
					t.Fatalf("bumps before block %d = %d, want %d", k+1, got, tc.want[k])
				}
			}
		})
	}
}

func TestLegacyReplacementFees(t *testing.T) {
	wei := big.NewInt
	previous := feeQuote{baseFee: wei(1e9), tip: wei(100_000_000), maxFee: wei(2_100_000_000)}
	for _, tc := range []struct {
		name    string
		current *feeQuote
		limit   *big.Int
		want    feeQuote
		wantErr error
	}{
		{
			name: "cached bump without fresh fees",
			want: feeQuote{baseFee: wei(1e9), tip: wei(112_500_000), maxFee: wei(2_362_500_000)},
		},
		{
			name:    "fresh fees raise the bump",
			current: &feeQuote{baseFee: wei(2e9), tip: wei(5e8), maxFee: wei(4.5e9)},
			want:    feeQuote{baseFee: wei(2e9), tip: wei(5e8), maxFee: wei(4.5e9)},
		},
		{
			name:    "the fresh tip is clamped under the limit, never below the bump",
			current: &feeQuote{baseFee: wei(2e9), tip: wei(5e8), maxFee: wei(4.5e9)}, limit: wei(2.4e9),
			want: feeQuote{baseFee: wei(2e9), tip: wei(4e8), maxFee: wei(2.4e9)},
		},
		{name: "a limit below the bump", limit: wei(2.2e9), wantErr: errReplacementLimitReached},
		{
			name:    "a limit below the base fee",
			current: &feeQuote{baseFee: wei(3e9), tip: wei(1e8), maxFee: wei(6.1e9)}, limit: wei(2.5e9),
			wantErr: errReplacementBaseAboveLimit,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := legacyReplacementFees(previous, tc.current, tc.limit)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.baseFee.Cmp(tc.want.baseFee) != 0 || got.tip.Cmp(tc.want.tip) != 0 || got.maxFee.Cmp(tc.want.maxFee) != 0 {
				t.Fatalf("fees = %s/%s/%s, want %s/%s/%s",
					got.baseFee, got.tip, got.maxFee, tc.want.baseFee, tc.want.tip, tc.want.maxFee)
			}
		})
	}
}

func TestPaidTip(t *testing.T) {
	tx := types.NewTx(&types.DynamicFeeTx{GasTipCap: gwei(0.1), GasFeeCap: gwei(3)})
	for _, tc := range []struct {
		name   string
		tx     *types.Transaction
		price  *big.Int
		want   *big.Int
		wantOK bool
	}{
		{name: "below the fee cap the tip cap is paid", tx: tx, price: gwei(1.1), want: gwei(0.1), wantOK: true},
		{name: "at the fee cap the tip cap is an upper bound", tx: tx, price: gwei(3), want: gwei(0.1), wantOK: true},
		{name: "never more than the price", tx: tx, price: gwei(0.05), want: gwei(0.05), wantOK: true},
		{name: "no transaction", price: gwei(1)},
		{name: "no effective gas price", tx: tx},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := paidTip(tc.tx, &types.Receipt{EffectiveGasPrice: tc.price})
			if ok != tc.wantOK || (ok && got.Cmp(tc.want) != 0) {
				t.Fatalf("paidTip = %v, %t; want %v, %t", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// expireSnapshot drops the cached fee snapshot, so the next shadow tick reads a new one.
func expireSnapshot(m *Manager) {
	m.snapshots.store(nil)
}

func TestShadowTickEmitsOutcomesAndFeeGauges(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := NewMetrics(reg)
	if err != nil {
		t.Fatal(err)
	}
	backend := newHorizonBackend(big.NewInt(1e18))
	m := NewWithMetrics(backend, mustSigner(t), big.NewInt(1), Config{
		MaxFeeGwei: 50,
		Balance:    BalanceConfig{ReferenceGasUnits: shadowTestGas},
	}, metrics, logr.Discard())
	m.metrics.startShadow()
	evaluator := m.newShadowEvaluator()
	tick := func() {
		t.Helper()
		expireSnapshot(m)
		if err := m.shadowTick(t.Context(), evaluator); err != nil {
			t.Fatal(err)
		}
	}
	outcome := func(policy FeePolicy, outcome shadowOutcome) float64 {
		return testutil.ToFloat64(metrics.shadowLifecycles.WithLabelValues(string(policy), string(outcome)))
	}

	// The balance guard is on and no balance was read yet: nothing starts.
	tick()
	if len(evaluator.pending) != 0 {
		t.Fatalf("started %v before any balance read", evaluator.pending)
	}
	m.observeSignerBalance(big.NewInt(1e18))
	for range firstAttemptBlocks + 1 {
		backend.hmu.Lock()
		backend.number++
		backend.hmu.Unlock()
		tick()
	}
	for _, policy := range shadowPolicies {
		for _, want := range shadowOutcomes {
			wantCount := 0.0
			if want == shadowFirst1 {
				wantCount = 1
			}
			if got := outcome(policy, want); got != wantCount {
				t.Fatalf("shadow_lifecycles_total{%s,%s} = %v, want %v", policy, want, got, wantCount)
			}
		}
	}
	// A new head refreshes the fee gauges for the reference fill, gate or no gate.
	wantMin := weiFloat(requiredBalance(shadowTestGas, horizonFee(big.NewInt(1e9), 2, gwei(0.02)), new(big.Int)))
	if got := testutil.ToFloat64(metrics.requiredBalance.WithLabelValues(requiredBalanceMin)); got != wantMin {
		t.Fatalf("account_required_balance_wei{min} = %v, want %v", got, wantMin)
	}
	if got := testutil.ToFloat64(metrics.nextBaseFee); got != 1e9 {
		t.Fatalf("fee_next_base_fee_wei = %v, want 1e9", got)
	}

	// A tick within the shadow interval shares the cached snapshot: no fee-history read of its own.
	historyReads := func() int {
		backend.hmu.Lock()
		defer backend.hmu.Unlock()
		return len(backend.historyReads)
	}
	reads := historyReads()
	if err := m.shadowTick(t.Context(), evaluator); err != nil {
		t.Fatal(err)
	}
	if got := historyReads(); got != reads {
		t.Fatalf("fee history reads = %d, want %d (the cached snapshot)", got, reads)
	}
}

func TestShadowGasFallsBackToTheLatestFill(t *testing.T) {
	m := New(newMockBackend(), mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
	if got := m.shadowGas(); got != 0 {
		t.Fatalf("shadowGas before any fill = %d, want 0", got)
	}
	m.lastFillGas.Store(3_500_000)
	if got := m.shadowGas(); got != 3_500_000 {
		t.Fatalf("shadowGas = %d, want the latest fill's 3500000", got)
	}
	m = New(newMockBackend(), mustSigner(t), big.NewInt(1), Config{Balance: BalanceConfig{ReferenceGasUnits: 4_350_000}}, logr.Discard())
	m.lastFillGas.Store(3_500_000)
	if got := m.shadowGas(); got != 4_350_000 {
		t.Fatalf("shadowGas = %d, want balance.referenceGasUnits 4350000", got)
	}
}

func TestShadowRunsFromStartOnlyWhenEnabledWithMetrics(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metrics  bool
		disabled bool
		wantRuns bool
	}{
		{name: "enabled with metrics", metrics: true, wantRuns: true},
		{name: "shadow.enabled false", metrics: true, disabled: true},
		{name: "no metrics to report to"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := newHorizonBackend(big.NewInt(1e18))
			var metrics *Metrics
			if tc.metrics {
				metrics = newTestMetrics(t)
			}
			cfg := Config{
				MaxFeeGwei: 50,
				Fees:       FeeConfig{BlockTime: 2 * time.Millisecond},
				Balance:    BalanceConfig{ReferenceGasUnits: shadowTestGas, GuardDisabled: true},
				Shadow:     ShadowConfig{Disabled: tc.disabled},
			}
			m := NewWithMetrics(backend, mustSigner(t), big.NewInt(1), cfg, metrics, logr.Discard())
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() {
				defer close(done)
				m.Start(ctx)
			}()
			deadline := time.Now().Add(300 * time.Millisecond)
			// Only the shadow reads fee snapshots here; the funding gate's account poll reads a one-block history.
			reads := func() int {
				backend.hmu.Lock()
				defer backend.hmu.Unlock()
				count := 0
				for _, read := range backend.historyReads {
					if read.blocks == feeSnapshotBlocks {
						count++
					}
				}
				return count
			}
			for reads() == 0 && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			cancel()
			<-done
			if runs := reads() > 0; runs != tc.wantRuns {
				t.Fatalf("shadow fee-history reads: %d, want runs %t", reads(), tc.wantRuns)
			}
			// Every policy and outcome is exported from the start, so a first outcome is an increase.
			if tc.wantRuns {
				if got, want := testutil.CollectAndCount(metrics.shadowLifecycles), len(shadowPolicies)*len(shadowOutcomes); got != want {
					t.Fatalf("shadow_lifecycles_total series = %d, want %d", got, want)
				}
			}
		})
	}
}

// TestShadowLogsReadOutagesAtInfo: the shadow evaluator is metrics-only, so a run of failed snapshot reads is
// one Info line when it starts and one when reads recover, never an Error that would page.
func TestShadowLogsReadOutagesAtInfo(t *testing.T) {
	logs, log := newLogCapture(0)
	var mu sync.Mutex
	backend := newHorizonBackend(big.NewInt(1e18))
	backend.historyErr = errors.New("upstream down")
	m := NewWithMetrics(backend, mustSigner(t), big.NewInt(1), Config{
		MaxFeeGwei: 50,
		Fees:       FeeConfig{BlockTime: 2 * time.Millisecond},
		Balance:    BalanceConfig{ReferenceGasUnits: shadowTestGas, GuardDisabled: true},
	}, newTestMetrics(t), logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
	// Start stores the manager logger on the context the evaluator logs through.
	ctx, cancel := context.WithCancel(observability.WithLogger(t.Context(), m.log))
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.runShadow(ctx)
	}()
	count := func(msg string) (errorLevel, info int) {
		mu.Lock()
		defer mu.Unlock()
		return countLogs(*logs, msg)
	}
	waitLog := func(msg string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, info := count(msg); info > 0 {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("no %q log", msg)
			}
			time.Sleep(time.Millisecond)
		}
	}
	waitLog("shadow fee evaluation paused: fee snapshot unavailable")
	// Let a few more reads fail: they stay at V(1).
	for failed := 0; failed < 5; {
		backend.hmu.Lock()
		failed = len(backend.historyReads)
		backend.hmu.Unlock()
		time.Sleep(time.Millisecond)
	}
	backend.hmu.Lock()
	backend.historyErr = nil
	backend.hmu.Unlock()
	waitLog("shadow fee evaluation resumed")
	cancel()
	<-done
	for _, msg := range []string{"shadow fee evaluation paused: fee snapshot unavailable", "shadow fee evaluation resumed"} {
		if errorLevel, info := count(msg); errorLevel != 0 || info != 1 {
			t.Fatalf("%q logged %d times at Error and %d at Info, want once at Info", msg, errorLevel, info)
		}
	}
}
