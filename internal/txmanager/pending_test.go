package txmanager

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestEvaluatePending(t *testing.T) {
	const gas = 4_000_000
	// fee(6, 0.02 gwei) at a 1 gwei next base fee: what a quiet horizon send signs.
	attempt := feeQuote{baseFee: big.NewInt(1e9), tip: gwei(0.02), maxFee: big.NewInt(1_822_032_470)}
	tipped := feeQuote{baseFee: big.NewInt(1e9), tip: gwei(5), maxFee: gwei(20)}
	unaffordable := func(ratio float64) feeBlock { return feeBlock{baseFee: gwei(2), gasUsedRatio: ratio} }
	roomyRun := []feeBlock{roomy(), roomy(), roomy(), roomy(), roomy(), roomy()} // blocks 95..100
	for _, tc := range []struct {
		name        string
		in          pendingInput
		nextBase    *big.Int
		blocks      []feeBlock
		wantAction  pendingAction
		wantReason  string
		wantRoomy   uint64
		wantFullRun uint64
	}{
		{
			name: "roomy heads below the stall threshold hold", in: pendingInput{sentAt: 98, stallBase: 98},
			blocks: roomyRun, wantAction: pendingHold, wantRoomy: 2,
		},
		{
			name: "three roomy misses stall", in: pendingInput{sentAt: 97, stallBase: 97},
			blocks: roomyRun, wantAction: pendingStall, wantRoomy: 3,
		},
		{
			name: "skipped heads still count every block since the send", in: pendingInput{sentAt: 95, stallBase: 95},
			blocks: roomyRun, wantAction: pendingStall, wantRoomy: 5,
		},
		{
			name: "roomy misses count from the latest stall rebroadcast", in: pendingInput{sentAt: 95, stallBase: 98, rebroadcasts: 1},
			blocks: roomyRun, wantAction: pendingHold, wantRoomy: 2,
		},
		{
			name: "one stall rebroadcast still rebroadcasts", in: pendingInput{sentAt: 95, stallBase: 97, rebroadcasts: 1},
			blocks: roomyRun, wantAction: pendingStall, wantRoomy: 3,
		},
		{
			name: "two stall rebroadcasts turn the stall into a reprice", in: pendingInput{sentAt: 95, stallBase: 97, rebroadcasts: 2},
			blocks: roomyRun, wantAction: pendingReprice, wantReason: repriceStall, wantRoomy: 3,
		},
		{
			name: "blocks whose base fee the attempt did not cover are not misses", in: pendingInput{sentAt: 97, stallBase: 97},
			blocks:     []feeBlock{roomy(), roomy(), roomy(), unaffordable(0.5), unaffordable(0.5), roomy()},
			wantAction: pendingHold, wantRoomy: 1,
		},
		{
			name: "one full block holds", in: pendingInput{sentAt: 97, stallBase: 97},
			blocks:     []feeBlock{roomy(), roomy(), roomy(), roomy(), roomy(), full(gwei(5))},
			wantAction: pendingHold, wantRoomy: 2, wantFullRun: 1,
		},
		{
			name: "two full blocks reprice for congestion", in: pendingInput{sentAt: 97, stallBase: 97},
			blocks:     []feeBlock{roomy(), roomy(), roomy(), roomy(), full(gwei(5)), full(gwei(5))},
			wantAction: pendingReprice, wantReason: repriceCongestion, wantRoomy: 1, wantFullRun: 2,
		},
		{
			name: "full blocks before the send are not misses", in: pendingInput{sentAt: 99, stallBase: 99},
			blocks:     []feeBlock{roomy(), roomy(), roomy(), roomy(), full(gwei(5)), full(gwei(5))},
			wantAction: pendingHold, wantFullRun: 1,
		},
		{
			name: "a full block the attempt could not afford breaks the run", in: pendingInput{sentAt: 97, stallBase: 97},
			blocks:     []feeBlock{roomy(), roomy(), roomy(), roomy(), unaffordable(0.95), full(gwei(5))},
			wantAction: pendingHold, wantRoomy: 1, wantFullRun: 1,
		},
		{
			name: "no congestion reprice while the tip already follows the run", in: pendingInput{fees: tipped, sentAt: 97, stallBase: 97},
			blocks:     []feeBlock{roomy(), roomy(), roomy(), roomy(), full(gwei(5)), full(gwei(5))},
			wantAction: pendingHold, wantRoomy: 1, wantFullRun: 2,
		},
		{
			name: "a run reward above the bumped tip reprices", in: pendingInput{fees: tipped, sentAt: 97, stallBase: 97},
			blocks:     []feeBlock{roomy(), roomy(), roomy(), roomy(), full(gwei(6)), full(gwei(6))},
			wantAction: pendingReprice, wantReason: repriceCongestion, wantRoomy: 1, wantFullRun: 2,
		},
		{
			name: "a next base fee that leaves fewer than two valid blocks reprices", in: pendingInput{sentAt: 100, stallBase: 100},
			nextBase: gwei(1.7), blocks: roomyRun, wantAction: pendingReprice, wantReason: repriceValidity,
		},
		{
			name: "two valid blocks left hold", in: pendingInput{sentAt: 100, stallBase: 100},
			nextBase: gwei(1.5), blocks: roomyRun, wantAction: pendingHold,
		},
		{
			name: "the head lag counts toward validity", in: pendingInput{sentAt: 100, stallBase: 100, lag: 1},
			nextBase: gwei(1.5), blocks: roomyRun, wantAction: pendingReprice, wantReason: repriceValidity,
		},
		{
			name: "validity comes before congestion", in: pendingInput{sentAt: 97, stallBase: 97},
			nextBase: gwei(1.7), blocks: []feeBlock{roomy(), roomy(), roomy(), roomy(), full(gwei(5)), full(gwei(5))},
			wantAction: pendingReprice, wantReason: repriceValidity, wantRoomy: 1, wantFullRun: 2,
		},
		{
			name: "a cancellation has room in blocks full for the fill", in: pendingInput{gas: cancellationGasLimit, sentAt: 97, stallBase: 97},
			blocks:     []feeBlock{roomy(), roomy(), roomy(), full(gwei(5)), full(gwei(5)), full(gwei(5))},
			wantAction: pendingStall, wantRoomy: 3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			if in.fees.maxFee == nil {
				in.fees = attempt
			}
			if in.gas == 0 {
				in.gas = gas
			}
			nextBase := tc.nextBase
			if nextBase == nil {
				nextBase = big.NewInt(1e9)
			}
			got := evaluatePending(in, testSnapshot(nextBase, append([]feeBlock(nil), tc.blocks...)...), testPolicy())
			if got.action != tc.wantAction || got.reason != tc.wantReason {
				t.Fatalf("decision = %s %q, want %s %q", got.action, got.reason, tc.wantAction, tc.wantReason)
			}
			if got.roomyMisses != tc.wantRoomy || got.fullMisses != tc.wantFullRun {
				t.Fatalf("misses = %d roomy, %d full; want %d roomy, %d full",
					got.roomyMisses, got.fullMisses, tc.wantRoomy, tc.wantFullRun)
			}
		})
	}
}

func TestHorizonDerivedTimeouts(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		cfg                      Config
		tick, feeRead, receipt   time.Duration
		slackAt20s, slackAt36s   bool
		slackAt17s, slackAt17_1s bool
	}{
		{
			name: "legacy keeps the replacement interval", cfg: Config{},
			tick: 30 * time.Second, feeRead: time.Second, receipt: 2 * time.Second,
			slackAt36s: true,
		},
		{
			name: "horizon follows the block time", cfg: Config{Fees: FeeConfig{Policy: FeePolicyHorizon}},
			tick: 6 * time.Second, feeRead: time.Second, receipt: 2 * time.Second,
			slackAt20s: true, slackAt36s: true, slackAt17_1s: true,
		},
		{
			name: "fast blocks shrink the read budgets", cfg: Config{Fees: FeeConfig{Policy: FeePolicyHorizon, BlockTime: 2 * time.Second}},
			tick: time.Second, feeRead: 500 * time.Millisecond, receipt: 500 * time.Millisecond,
			slackAt20s: true, slackAt36s: true, slackAt17s: true, slackAt17_1s: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil, nil, nil, tc.cfg, logr.Discard())
			if got := m.pendingTick(); got != tc.tick {
				t.Fatalf("tick = %s, want %s", got, tc.tick)
			}
			if got := m.feeReadTimeout(); got != tc.feeRead {
				t.Fatalf("fee read timeout = %s, want %s", got, tc.feeRead)
			}
			if got := m.receiptReadTimeout(); got != tc.receipt {
				t.Fatalf("receipt read timeout = %s, want %s", got, tc.receipt)
			}
			now := time.Now()
			for _, slack := range []struct {
				until time.Duration
				want  bool
			}{{20 * time.Second, tc.slackAt20s}, {36 * time.Second, tc.slackAt36s}, {17 * time.Second, tc.slackAt17s}, {17100 * time.Millisecond, tc.slackAt17_1s}} {
				if got := m.hasExactRebroadcastSlack(&pendingTransaction{cancelDeadline: now.Add(slack.until)}, now); got != slack.want {
					t.Fatalf("slack %s before the deadline = %t, want %t", slack.until, got, slack.want)
				}
			}
		})
	}
}

// pendingChain is a horizonBackend whose sends stay pending until the test mines them: every broadcast is
// recorded, the head advances one block at a time with a chosen gas-used ratio, and a transaction is
// included only when a block is mined with it. The latest header links to the receipt headers, so receipts
// confirm against it.
type pendingChain struct {
	*horizonBackend
}

func newPendingChain(balance *big.Int) *pendingChain {
	return &pendingChain{horizonBackend: newHorizonBackend(balance)}
}

func (c *pendingChain) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	header, err := c.horizonBackend.HeaderByNumber(ctx, number)
	if err == nil && number == nil && header.Number.Uint64() > 0 {
		header.ParentHash = receiptTestHeader(header.Number.Uint64() - 1).Hash()
	}
	return header, err
}

func (c *pendingChain) SendTransaction(_ context.Context, tx *types.Transaction) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	i := c.sendCalls
	c.sendCalls++
	c.attempted = append(c.attempted, tx)
	if i < len(c.sendErrs) && c.sendErrs[i] != nil {
		return c.sendErrs[i]
	}
	c.sent = append(c.sent, tx)
	return nil
}

// mine appends a block with this gas-used ratio, including tx when it is set, and returns its number.
func (c *pendingChain) mine(ratio float64, tx *types.Transaction) uint64 {
	c.hmu.Lock()
	c.number++
	number := c.number
	c.ratios[number] = ratio
	c.hmu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.head = number
	if tx != nil {
		c.receipts[tx.Hash()] = successfulReceipt(tx, number)
		c.latestNonce = tx.Nonce() + 1
	}
	return number
}

// sends returns every transaction handed to the write endpoint, rebroadcasts included.
func (c *pendingChain) sends() []*types.Transaction {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*types.Transaction(nil), c.sent...)
}

// waitEvaluated waits until two fee histories reaching head were served: the lifecycle then read head and
// finished whatever it decided there, a re-estimate included, before its next tick read again.
func (c *pendingChain) waitEvaluated(t *testing.T, head uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.hmu.Lock()
		served := 0
		for _, newest := range c.historyHeads {
			if newest >= head {
				served++
			}
		}
		c.hmu.Unlock()
		if served >= 2 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("head %d was not evaluated", head)
}

// waitSends waits until the write endpoint received count transactions and returns them.
func (c *pendingChain) waitSends(t *testing.T, count int) []*types.Transaction {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sends := c.sends(); len(sends) >= count {
			return sends
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("write endpoint received %d transactions, want %d", len(c.sends()), count)
	return nil
}

// pendingConfig is a horizon config with 100 ms blocks (evaluation ticks every 50 ms), no fallback or
// pending timeout unless a test sets one, and a short shutdown drain for lifecycles a test leaves pending.
func pendingConfig() Config {
	cfg := horizonConfig()
	cfg.Fees.BlockTime = 100 * time.Millisecond
	cfg.ReplacementInterval = time.Hour
	cfg.PendingTimeout = time.Hour
	cfg.ShutdownTimeout = 100 * time.Millisecond
	// The shadow evaluator reads fee snapshots of its own every half block, which waitEvaluated would count as
	// the lifecycle's evaluations; it has its own tests (shadow_test.go).
	cfg.Shadow.Disabled = true
	return cfg
}

type pendingHarness struct {
	chain   *pendingChain
	m       *Manager
	metrics *Metrics
	logs    *[]string
	logMu   *sync.Mutex
}

func startPendingHarness(t *testing.T, chain *pendingChain, cfg Config) *pendingHarness {
	t.Helper()
	logs, log := newLogCapture(0)
	mu := &sync.Mutex{}
	metrics := newTestMetrics(t)
	m := NewWithMetrics(chain, mustSigner(t), big.NewInt(11155111), cfg, metrics, logr.New(&lockedSink{sink: log.GetSink(), mu: mu}))
	startManagerForTest(t, m)
	return &pendingHarness{chain: chain, m: m, metrics: metrics, logs: logs, logMu: mu}
}

// waitForNextBlockEstimates waits for the startup probe to confirm the block overrides, so estimated sends
// and re-estimates are next-block estimates.
func (h *pendingHarness) waitForNextBlockEstimates(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for h.m.overrides.load() != blockOverridesSupported {
		if time.Now().After(deadline) {
			t.Fatal("the block overrides probe did not confirm")
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *pendingHarness) send(t *testing.T, req Request) (<-chan Result, *types.Transaction) {
	t.Helper()
	result, accepted := h.m.SendAsync(t.Context(), req)
	if !accepted {
		t.Fatal("request was not accepted")
	}
	return result, h.chain.waitSends(t, 1)[0]
}

// countLog counts the captured lines with this message and, when set, this field.
func (h *pendingHarness) countLog(msg, field string) int {
	h.logMu.Lock()
	defer h.logMu.Unlock()
	count := 0
	for _, entry := range *h.logs {
		if strings.Contains(entry, `"msg":"`+msg+`"`) && strings.Contains(entry, field) {
			count++
		}
	}
	return count
}

// waitMetric waits for a metric the lifecycle goroutine records right after a broadcast the test saw.
func waitMetric(t *testing.T, collector prometheus.Collector, want float64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for testutil.ToFloat64(collector) != want {
		if time.Now().After(deadline) {
			t.Fatalf("metric = %v, want %v", testutil.ToFloat64(collector), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func waitResult(t *testing.T, result <-chan Result) Result {
	t.Helper()
	select {
	case got := <-result:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the result")
		return Result{}
	}
}

func TestHorizonPendingHoldsAcrossRoomyHeads(t *testing.T) {
	h := startPendingHarness(t, newPendingChain(big.NewInt(1e18)), pendingConfig())
	result, first := h.send(t, fillRequest(100_000))
	for range 2 {
		h.chain.waitEvaluated(t, h.chain.mine(0.5, nil))
	}
	if sends := h.chain.sends(); len(sends) != 1 {
		t.Fatalf("write endpoint received %d transactions across two roomy heads, want only the first", len(sends))
	}
	h.chain.mine(0.5, first)
	if got := waitResult(t, result); got.Outcome != OutcomeConfirmed || got.Hash != first.Hash() {
		t.Fatalf("result = %+v, want the first attempt confirmed", got)
	}
	assertMetric(t, h.metrics.firstAttempts.WithLabelValues("fill", string(firstAttemptFirst), simulationUnknown), 1)
	assertHistogramObservedOnce(t, h.metrics.inclusionDelay.WithLabelValues("fill"))
	for _, reason := range repricingReasons {
		assertMetric(t, h.metrics.repricings.WithLabelValues("fill", reason), 0)
	}
	assertMetric(t, h.metrics.rebroadcasts.WithLabelValues("fill", rebroadcastStall), 0)
}

func TestHorizonPendingRepricesBeforeItTurnsInvalid(t *testing.T) {
	h := startPendingHarness(t, newPendingChain(big.NewInt(1e18)), pendingConfig())
	_, first := h.send(t, fillRequest(100_000))
	// The next base fee rose past what fee(2, tipFloor) the signed cap still covers.
	h.chain.set(func(b *horizonBackend) { b.nextBase = gwei(1.7) })
	h.chain.waitEvaluated(t, h.chain.mine(0.5, nil))
	// A decision's send can trail the reads waitEvaluated counts; waitSends waits for it, len catches extras.
	sends := h.chain.waitSends(t, 2)
	if len(sends) != 2 {
		t.Fatalf("write endpoint received %d transactions, want the first and one validity reprice", len(sends))
	}
	tip := bumpFee(first.GasTipCap())
	want := maxBigCopy(bumpFee(first.GasFeeCap()), horizonFee(gwei(1.7), defaultMaxHorizonBlocks, tip))
	if got := sends[1]; got.GasFeeCap().Cmp(want) != 0 || got.GasTipCap().Cmp(tip) != 0 || got.Nonce() != first.Nonce() {
		t.Fatalf("reprice = max %s tip %s nonce %d, want max %s tip %s nonce %d",
			got.GasFeeCap(), got.GasTipCap(), got.Nonce(), want, tip, first.Nonce())
	}
	assertMetric(t, h.metrics.repricings.WithLabelValues("fill", repriceValidity), 1)
	if h.countLog("pending transaction replaced", `"reason":"validity"`) != 1 {
		t.Fatal("the reprice was not logged with its reason")
	}
	// The repriced cap covers the next six blocks again: the following head holds.
	h.chain.waitEvaluated(t, h.chain.mine(0.5, nil))
	if held := h.chain.sends(); len(held) != 2 {
		t.Fatalf("write endpoint received %d transactions after the reprice, want no further one", len(held))
	}
}

func TestHorizonPendingEscalatesAfterTwoFullMisses(t *testing.T) {
	const gas = 4_000_000 // a 95% full block leaves 1.8M gas: no room for it
	h := startPendingHarness(t, newPendingChain(big.NewInt(1e18)), pendingConfig())
	_, first := h.send(t, fillRequest(gas))
	h.chain.set(func(b *horizonBackend) {
		b.runRewards[101], b.runRewards[102] = gwei(3), gwei(3)
	})
	h.chain.waitEvaluated(t, h.chain.mine(0.95, nil))
	if sends := h.chain.sends(); len(sends) != 1 {
		t.Fatalf("one full block led to %d transactions, want no reprice", len(sends))
	}
	h.chain.waitEvaluated(t, h.chain.mine(0.95, nil))
	sends := h.chain.waitSends(t, 2)
	if len(sends) != 2 {
		t.Fatalf("two full blocks led to %d transactions, want one congestion reprice", len(sends))
	}
	if got, want := sends[1].GasTipCap(), gwei(3); got.Cmp(want) != 0 {
		t.Fatalf("congestion tip = %s, want the run's reward %s", got, want)
	}
	if got, want := sends[1].GasFeeCap(), horizonFee(big.NewInt(1e9), defaultMaxHorizonBlocks, gwei(3)); got.Cmp(want) != 0 {
		t.Fatalf("congestion cap = %s, want fee(6, 3 gwei) = %s", got, want)
	}
	if sends[1].GasFeeCap().Cmp(bumpFee(first.GasFeeCap())) < 0 {
		t.Fatal("the congestion reprice did not bump the fee cap")
	}
	assertMetric(t, h.metrics.repricings.WithLabelValues("fill", repriceCongestion), 1)
	// The reprice restarted the full-miss count at head 102: one more full block, whose run reward is above
	// the repriced tip's bump, is one miss of it, not the third of the first attempt.
	h.chain.set(func(b *horizonBackend) { b.runRewards[103] = gwei(4) })
	h.chain.waitEvaluated(t, h.chain.mine(0.95, nil))
	if held := h.chain.sends(); len(held) != 2 {
		t.Fatalf("one full block after the reprice led to %d transactions, want no second reprice", len(held))
	}
	h.chain.waitEvaluated(t, h.chain.mine(0.95, nil))
	if repriced := h.chain.waitSends(t, 3); repriced[2].GasTipCap().Cmp(gwei(4)) != 0 {
		t.Fatalf("second congestion tip = %s, want the run's reward 4 gwei", repriced[2].GasTipCap())
	}
	assertMetric(t, h.metrics.repricings.WithLabelValues("fill", repriceCongestion), 2)
}

func TestHorizonPendingStallRebroadcastsThenReprices(t *testing.T) {
	h := startPendingHarness(t, newPendingChain(big.NewInt(1e18)), pendingConfig())
	h.waitForNextBlockEstimates(t)
	result, first := h.send(t, fillRequest(0))
	if first.Gas() != 105_000 {
		t.Fatalf("gas limit = %d, want the next-block estimate plus 5%%", first.Gas())
	}
	mineRoomy := func(blocks int) {
		var head uint64
		for range blocks {
			head = h.chain.mine(0.5, nil)
		}
		h.chain.waitEvaluated(t, head)
	}
	for rebroadcast := 1; rebroadcast <= stallRebroadcastsBeforeReprice; rebroadcast++ {
		mineRoomy(3)
		// The stall's send follows its re-estimate, which can finish after the reads waitEvaluated counts.
		sends := h.chain.waitSends(t, 1+rebroadcast)
		if len(sends) != 1+rebroadcast || sends[rebroadcast].Hash() != first.Hash() {
			t.Fatalf("after %d stalls the write endpoint received %d transactions, want %d exact rebroadcasts",
				rebroadcast, len(sends), rebroadcast)
		}
	}
	mineRoomy(3)
	sends := h.chain.waitSends(t, 4)
	if len(sends) != 4 || sends[3].Hash() == first.Hash() {
		t.Fatalf("the third stall led to %d transactions, want a reprice after two rebroadcasts", len(sends))
	}
	if got := sends[3]; got.GasTipCap().Cmp(bumpFee(first.GasTipCap())) != 0 ||
		got.GasFeeCap().Cmp(bumpFee(first.GasFeeCap())) != 0 || got.Gas() != first.Gas() {
		t.Fatalf("stall reprice = max %s tip %s gas %d, want the minimal bump of %s / %s at gas %d",
			got.GasFeeCap(), got.GasTipCap(), got.Gas(), first.GasFeeCap(), first.GasTipCap(), first.Gas())
	}
	assertMetric(t, h.metrics.rebroadcasts.WithLabelValues("fill", rebroadcastStall), 2)
	assertMetric(t, h.metrics.repricings.WithLabelValues("fill", repriceStall), 1)
	// The send and each of the three stall responses estimated in the next block's context.
	assertMetric(t, h.metrics.gasEstimates.WithLabelValues("fill", estimateModeNextBlock, gasEstimateOK), 4)
	h.chain.mine(0.5, sends[3])
	if got := waitResult(t, result); got.Outcome != OutcomeConfirmed || got.Hash != sends[3].Hash() {
		t.Fatalf("result = %+v, want the reprice confirmed", got)
	}
	assertMetric(t, h.metrics.firstAttempts.WithLabelValues("fill", string(firstAttemptReplaced), simulationUnknown), 1)
}

func TestHorizonPendingStallReplacesAnExhaustedGasLimit(t *testing.T) {
	h := startPendingHarness(t, newPendingChain(big.NewInt(1e18)), pendingConfig())
	h.waitForNextBlockEstimates(t)
	result, first := h.send(t, fillRequest(0))
	// A fill on the same vault landed first: the call now needs more than its whole headroom.
	h.chain.set(func(b *horizonBackend) { b.nextBlockGas = 110_000 })
	for range 3 {
		h.chain.mine(0.5, nil)
	}
	sends := h.chain.waitSends(t, 2)
	if got := sends[1]; got.Gas() != 115_500 || got.Nonce() != first.Nonce() ||
		got.GasTipCap().Cmp(bumpFee(first.GasTipCap())) != 0 || got.GasFeeCap().Cmp(bumpFee(first.GasFeeCap())) != 0 {
		t.Fatalf("gas replacement = gas %d max %s tip %s, want gas 115500 at bumped fees", got.Gas(), got.GasFeeCap(), got.GasTipCap())
	}
	h.chain.mine(0.5, sends[1])
	if got := waitResult(t, result); got.Outcome != OutcomeConfirmed || got.Hash != sends[1].Hash() {
		t.Fatalf("result = %+v, want the gas replacement confirmed", got)
	}
	assertMetric(t, h.metrics.repricings.WithLabelValues("fill", repriceGas), 1)
	if h.countLog("pending transaction replaced", `"gasLimit":115500`) != 1 {
		t.Fatal("the gas replacement was not logged with its gas limit")
	}
}

func TestHorizonPendingRevertingReestimateCancels(t *testing.T) {
	reverted := rpcCodeError{code: 3, message: "execution reverted: order filled"}
	for _, tc := range []struct {
		name   string
		mine   func(c *pendingChain)
		reason string
	}{
		{
			name:   "stall",
			mine:   func(c *pendingChain) { c.mine(0.5, nil); c.mine(0.5, nil); c.mine(0.5, nil) },
			reason: "stall",
		},
		{
			name: "validity reprice",
			mine: func(c *pendingChain) {
				c.set(func(b *horizonBackend) { b.nextBase = gwei(1.7) })
				c.mine(0.5, nil)
			},
			reason: "reprice",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := startPendingHarness(t, newPendingChain(big.NewInt(1e18)), pendingConfig())
			h.waitForNextBlockEstimates(t)
			result, first := h.send(t, fillRequest(0))
			// A competitor filled the order: the call can no longer succeed.
			h.chain.set(func(b *horizonBackend) { b.nextBlockErr = reverted })
			tc.mine(h.chain)
			sends := h.chain.waitSends(t, 2)
			cancellation := sends[1]
			if cancellation.To() == nil || *cancellation.To() != h.m.signer.Address() || cancellation.Gas() != cancellationGasLimit ||
				cancellation.Nonce() != first.Nonce() || len(cancellation.Data()) != 0 {
				t.Fatalf("second transaction = to %v gas %d nonce %d, want a same-nonce self-cancellation",
					cancellation.To(), cancellation.Gas(), cancellation.Nonce())
			}
			if h.countLog("pending transaction cancellation requested", `"reason":"simulated_revert"`) != 1 ||
				h.countLog("pending transaction re-estimate reverts; cancelling", `"trigger":"`+tc.reason+`"`) != 1 {
				t.Fatal("the cancellation was not logged as a simulated revert")
			}
			h.chain.mine(0.5, cancellation)
			if got := waitResult(t, result); got.Outcome != OutcomeCancelled {
				t.Fatalf("result = %+v, want cancelled", got)
			}
			for _, reason := range repricingReasons {
				assertMetric(t, h.metrics.repricings.WithLabelValues("fill", reason), 0)
			}
		})
	}
}

func TestHorizonPendingCancellationStaysLive(t *testing.T) {
	for _, tc := range []struct {
		name string
		gas  uint64
	}{
		{name: "supplied gas limit", gas: 100_000},
		// Every production fill is estimated: its cancellation must not wait for a re-estimate either.
		{name: "estimated call"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := pendingConfig()
			cfg.ReplacementInterval = 100 * time.Millisecond // the legacy policy would bump the cancellation this often
			h := startPendingHarness(t, newPendingChain(big.NewInt(1e18)), cfg)
			if tc.gas == 0 {
				h.waitForNextBlockEstimates(t)
			}
			req := fillRequest(tc.gas)
			req.CancelAt = time.Now().Add(200 * time.Millisecond)
			result, first := h.send(t, req)
			cancellation := h.chain.waitSends(t, 2)[1]
			// Priced by cancellationFees: max(bump, fee(minHorizon + 1, tip)) with tip max(bump, tipRule(21000)).
			if cancellation.To() == nil || *cancellation.To() != h.m.signer.Address() ||
				cancellation.GasFeeCap().Cmp(bumpFee(first.GasFeeCap())) != 0 || cancellation.GasTipCap().Cmp(bumpFee(first.GasTipCap())) != 0 {
				t.Fatalf("cancellation = max %s tip %s, want the bump of %s / %s", cancellation.GasFeeCap(), cancellation.GasTipCap(),
					first.GasFeeCap(), first.GasTipCap())
			}
			time.Sleep(4 * cfg.ReplacementInterval)
			if sends := h.chain.sends(); len(sends) != 2 {
				t.Fatalf("the pending cancellation was replaced %d times without a new head", len(sends)-2)
			}
			mineRoomy := func() {
				var head uint64
				for range 3 {
					head = h.chain.mine(0.5, nil)
				}
				h.chain.waitEvaluated(t, head)
			}
			for rebroadcast := 1; rebroadcast <= stallRebroadcastsBeforeReprice; rebroadcast++ {
				mineRoomy()
				if sends := h.chain.waitSends(t, 2+rebroadcast); len(sends) != 2+rebroadcast || sends[1+rebroadcast].Hash() != cancellation.Hash() {
					t.Fatalf("stall %d: write endpoint received %d transactions, want an exact cancellation rebroadcast", rebroadcast, len(sends))
				}
			}
			mineRoomy()
			sends := h.chain.waitSends(t, 5)
			if len(sends) != 5 || sends[4].Hash() == cancellation.Hash() || sends[4].GasFeeCap().Cmp(bumpFee(cancellation.GasFeeCap())) != 0 {
				t.Fatalf("third stall: write endpoint received %d transactions, want one minimal cancellation reprice", len(sends))
			}
			assertMetric(t, h.metrics.rebroadcasts.WithLabelValues("fill", rebroadcastStall), 2)
			assertMetric(t, h.metrics.repricings.WithLabelValues("fill", repriceStall), 1)
			h.chain.mine(0.5, sends[4])
			if got := waitResult(t, result); got.Outcome != OutcomeCancelled {
				t.Fatalf("result = %+v, want cancelled", got)
			}
		})
	}
}

func TestHorizonPendingCappedRepriceHolds(t *testing.T) {
	// 1.9 gwei per gas over 100000 gas: the send's fee(6) of 1.82 gwei fits, the 12.5% bump does not.
	chain := newPendingChain(big.NewInt(190_000_000_000_000))
	// The endpoint still holds the first rebroadcast's bytes: "already known" is a rebroadcast, not a failure.
	chain.sendErrs = []error{nil, errors.New("already known")}
	h := startPendingHarness(t, chain, pendingConfig())
	_, first := h.send(t, fillRequest(100_000))
	h.chain.set(func(b *horizonBackend) { b.nextBase = gwei(1.7) })
	for held := 1; held <= 2; held++ {
		h.chain.waitEvaluated(t, h.chain.mine(0.5, nil))
		attempted := h.chain.attemptedTransactions()
		if len(attempted) != 1+held || attempted[held].Hash() != first.Hash() {
			t.Fatalf("head %d: write endpoint received %d transactions, want an exact capped rebroadcast", held, len(attempted))
		}
	}
	if h.countLog("replacement capped", `"reason":"balance"`) != 1 {
		t.Fatal(`"replacement capped" was not logged once with reason balance`)
	}
	assertMetric(t, h.metrics.rebroadcasts.WithLabelValues("fill", rebroadcastCapped), 2)
	assertMetric(t, h.metrics.repricings.WithLabelValues("fill", repriceValidity), 0)
	h.logMu.Lock()
	failedErrors, _ := countLogs(*h.logs, "capped transaction rebroadcast failed")
	_, rebroadcastInfos := countLogs(*h.logs, "capped transaction rebroadcast")
	h.logMu.Unlock()
	if failedErrors != 0 || rebroadcastInfos != 2 {
		t.Fatalf("capped rebroadcasts logged %d errors and %d info lines, want none and two", failedErrors, rebroadcastInfos)
	}
}

func TestHorizonPendingFallsBackToTimedBumps(t *testing.T) {
	cfg := pendingConfig()
	cfg.ReplacementInterval = 500 * time.Millisecond
	h := startPendingHarness(t, newPendingChain(big.NewInt(1e18)), cfg)
	_, first := h.send(t, fillRequest(100_000))
	h.chain.set(func(b *horizonBackend) { b.historyErr = errors.New("upstream unavailable") })
	// The reads fail at every 50 ms tick, but the fallback waits for them to fail for the whole interval.
	time.Sleep(cfg.ReplacementInterval / 2)
	if sends := h.chain.sends(); len(sends) != 1 {
		t.Fatalf("the fallback replaced after %s of failed reads, before the replacement interval", cfg.ReplacementInterval/2)
	}
	bumped := h.chain.waitSends(t, 2)[1]
	// Today's cached bump: fresh fees are unavailable, so both fields are bumped by 12.5%.
	if bumped.GasFeeCap().Cmp(bumpFee(first.GasFeeCap())) != 0 || bumped.GasTipCap().Cmp(bumpFee(first.GasTipCap())) != 0 {
		t.Fatalf("fallback = max %s tip %s, want the cached bump", bumped.GasFeeCap(), bumped.GasTipCap())
	}
	waitMetric(t, h.metrics.repricings.WithLabelValues("fill", repriceFallback), 1)
	h.chain.set(func(b *horizonBackend) { b.historyErr = nil })
	time.Sleep(2 * cfg.ReplacementInterval)
	if sends := h.chain.sends(); len(sends) != 2 {
		t.Fatalf("recovered reads kept timed bumps going: %d transactions", len(sends))
	}
	if h.countLog("pending transaction evaluation reads recovered", "") != 1 {
		t.Fatal("the recovery of the evaluation reads was not logged")
	}
}

// TestHorizonPendingFallbackCountsFromTheNextFreshHead: the fallback's cached bump is signed at a head no
// evaluation read showed. Full blocks mined while the reads failed, before it was signed, are not misses of
// it: its counts start at the first fresh read, so recovering reads do not follow the bump with a
// congestion reprice at once.
func TestHorizonPendingFallbackCountsFromTheNextFreshHead(t *testing.T) {
	const gas = 4_000_000 // a 95% full block leaves 1.8M gas: no room for it
	cfg := pendingConfig()
	cfg.ReplacementInterval = 500 * time.Millisecond
	h := startPendingHarness(t, newPendingChain(big.NewInt(1e18)), cfg)
	_, first := h.send(t, fillRequest(gas))
	h.chain.set(func(b *horizonBackend) { b.historyErr = errors.New("upstream unavailable") })
	h.chain.mine(0.95, nil)
	h.chain.mine(0.95, nil)
	bumped := h.chain.waitSends(t, 2)[1]
	if bumped.GasFeeCap().Cmp(bumpFee(first.GasFeeCap())) != 0 {
		t.Fatalf("fallback = max %s, want the cached bump of %s", bumped.GasFeeCap(), first.GasFeeCap())
	}
	h.chain.set(func(b *horizonBackend) { b.historyErr = nil })
	h.chain.waitEvaluated(t, 102)
	if sends := h.chain.sends(); len(sends) != 2 {
		t.Fatalf("recovered reads followed the fallback with %d more transactions, want none at head 102", len(sends)-2)
	}
	// The counts started at head 102: one full block is one miss, two are a demand run.
	h.chain.waitEvaluated(t, h.chain.mine(0.95, nil))
	if sends := h.chain.sends(); len(sends) != 2 {
		t.Fatalf("one full block after the fallback led to %d more transactions, want none", len(sends)-2)
	}
	h.chain.waitEvaluated(t, h.chain.mine(0.95, nil))
	h.chain.waitSends(t, 3)
	assertMetric(t, h.metrics.repricings.WithLabelValues("fill", repriceFallback), 1)
	assertMetric(t, h.metrics.repricings.WithLabelValues("fill", repriceCongestion), 1)
}

// TestHorizonPendingFallsBackOnAStuckHead: an upstream stuck on one head answers every read, so no read
// fails, yet no head can be judged; its stale reads count toward the fallback like failed ones.
func TestHorizonPendingFallsBackOnAStuckHead(t *testing.T) {
	cfg := pendingConfig()
	cfg.ReplacementInterval = 500 * time.Millisecond
	h := startPendingHarness(t, newPendingChain(big.NewInt(1e18)), cfg)
	_, first := h.send(t, fillRequest(100_000))
	// Every header is a minute old: 600 blocks behind the next one at 100 ms blocks.
	h.chain.set(func(b *horizonBackend) { b.headAges = []time.Duration{time.Minute} })
	time.Sleep(cfg.ReplacementInterval / 2)
	if sends := h.chain.sends(); len(sends) != 1 {
		t.Fatalf("stale reads fell back after %s, before the replacement interval", cfg.ReplacementInterval/2)
	}
	bumped := h.chain.waitSends(t, 2)[1]
	if bumped.Nonce() != first.Nonce() || bumped.GasFeeCap().Cmp(bumpFee(first.GasFeeCap())) < 0 ||
		bumped.GasTipCap().Cmp(bumpFee(first.GasTipCap())) < 0 {
		t.Fatalf("fallback = nonce %d max %s tip %s, want a same-nonce bump", bumped.Nonce(), bumped.GasFeeCap(), bumped.GasTipCap())
	}
	waitMetric(t, h.metrics.repricings.WithLabelValues("fill", repriceFallback), 1)
	if h.countLog("pending transaction evaluation reads unavailable", ErrStaleHead.Error()) == 0 {
		t.Fatal("the stale reads were not logged as unavailable")
	}
}

// TestPendingEvaluatorCountsFromTheHeadAnAttemptWasSentAt pins where the counts of an attempt signed
// outside an evaluation start: at the head of the fee snapshot a cancellation was priced from, and after a
// re-estimate, which the head may outrun, at the next fresh read.
func TestPendingEvaluatorCountsFromTheHeadAnAttemptWasSentAt(t *testing.T) {
	t.Run("a cancellation counts from the head it was priced at", func(t *testing.T) {
		chain := newPendingChain(big.NewInt(1e18))
		ctx, d := directEvaluator(t, chain, fillRequest(100_000), false)
		m, pending, evaluator := d.m, d.pending, d.evaluator
		// The deadline cancellation is priced from a fresh snapshot at head 101, which no evaluation read.
		chain.mine(0.5, nil)
		if cancelling, err := m.tryReplace(ctx, pending, true); !cancelling || err != nil {
			t.Fatalf("tryReplace = %t, %v", cancelling, err)
		}
		if got := pending.attempts[1].pricedHead; got != 101 {
			t.Fatalf("cancellation priced head = %d, want 101", got)
		}
		chain.mine(0.5, nil)
		chain.mine(0.5, nil)
		evaluator.tick(ctx, true) // head 103: two roomy misses of the cancellation, not three
		if sends := chain.sends(); len(sends) != 2 || evaluator.sentAt != 101 {
			t.Fatalf("head 103: %d sends, counts from %d; want the cancellation held, counted from 101", len(sends), evaluator.sentAt)
		}
	})
	for _, tc := range []struct {
		name         string
		nextBlockGas uint64
		wantSends    int
		wantSentAt   uint64
	}{
		// 110000 exceeds the gas limit 105000: a gas replacement, a new attempt.
		{name: "a replacement after a re-estimate counts from the next fresh read", nextBlockGas: 110_000, wantSends: 2, wantSentAt: 104},
		// An exact rebroadcast restarts only the stall count.
		{name: "a stall rebroadcast after a re-estimate counts from the next fresh read", nextBlockGas: 100_000, wantSends: 2, wantSentAt: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain := newPendingChain(big.NewInt(1e18))
			ctx, d := directEvaluator(t, chain, fillRequest(0), true)
			m, pending := d.m, d.pending
			evaluator := m.newPendingEvaluator(pending, pendingActions{
				replace: func(cancellation bool, plan *replacementPlan) { _, _ = m.replace(ctx, pending, cancellation, plan) },
				cancel:  func(string) { t.Error("the evaluator cancelled") },
			})
			release := make(chan struct{})
			chain.set(func(b *horizonBackend) { b.nextBlockGas, b.blockEstimates = tc.nextBlockGas, release })
			for range 3 {
				chain.mine(0.5, nil)
			}
			evaluator.tick(ctx, false) // head 103: a stall, re-estimated
			// Block 104 is mined while the re-estimate runs, so what it sends is sent after block 104.
			chain.mine(0.5, nil)
			close(release)
			<-evaluator.estimated()
			evaluator.finishEstimate(ctx, false)
			if sends := chain.sends(); len(sends) != tc.wantSends {
				t.Fatalf("write endpoint received %d transactions, want %d", len(sends), tc.wantSends)
			}
			evaluator.tick(ctx, false)
			if evaluator.stallBase != 104 || evaluator.sentAt != tc.wantSentAt {
				t.Fatalf("counts from %d, stall count from %d; want %d and 104", evaluator.sentAt, evaluator.stallBase, tc.wantSentAt)
			}
			if sends := chain.sends(); len(sends) != tc.wantSends {
				t.Fatalf("head 104 led to %d more transactions, want none", len(sends)-tc.wantSends)
			}
		})
	}
}

// reorgChain counts the receipts it served per hash, so a test can reorg an inclusion the lifecycle is
// confirming.
type reorgChain struct {
	*pendingChain

	served map[common.Hash]int
}

func (c *reorgChain) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	receipt, err := c.pendingChain.TransactionReceipt(ctx, hash)
	if err == nil {
		c.mu.Lock()
		c.served[hash]++
		c.mu.Unlock()
	}
	return receipt, err
}

func (c *reorgChain) servedReceipts(hash common.Hash) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.served[hash]
}

func TestHorizonPendingReorgRebroadcastsWithoutCountingMisses(t *testing.T) {
	chain := &reorgChain{pendingChain: newPendingChain(big.NewInt(1e18)), served: map[common.Hash]int{}}
	logs, log := newLogCapture(0)
	mu := &sync.Mutex{}
	metrics := newTestMetrics(t)
	m := NewWithMetrics(chain, mustSigner(t), big.NewInt(11155111), pendingConfig(), metrics, logr.New(&lockedSink{sink: log.GetSink(), mu: mu}))
	startManagerForTest(t, m)
	h := &pendingHarness{chain: chain.pendingChain, m: m, metrics: metrics, logs: logs, logMu: mu}
	req := fillRequest(100_000)
	req.Confirmations = new(uint64(2))
	result, first := h.send(t, req)

	included := chain.mine(0.5, first)
	waitServed := func(count int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for chain.servedReceipts(first.Hash()) < count {
			if time.Now().After(deadline) {
				t.Fatalf("the receipt was served %d times, want %d", chain.servedReceipts(first.Hash()), count)
			}
			time.Sleep(time.Millisecond)
		}
	}
	// The receipt sweep found it and the confirmation wait read it again: the lifecycle is confirming it.
	waitServed(2)
	// One more block while it waits for its second confirmation: the lifecycle believes it included there, and
	// the confirmation wait, not an evaluation read, sees the head move.
	chain.mine(0.5, nil)
	waitServed(chain.servedReceipts(first.Hash()) + 2)
	chain.mu.Lock()
	delete(chain.receipts, first.Hash())
	chain.latestNonce = first.Nonce()
	chain.mu.Unlock()

	sends := chain.waitSends(t, 2)
	if sends[1].Hash() != first.Hash() {
		t.Fatal("the reorged inclusion was not rebroadcast as its exact bytes")
	}
	assertMetric(t, metrics.rebroadcasts.WithLabelValues("fill", rebroadcastReorg), 1)
	// The blocks after the inclusion had room. Counted as misses from the inclusion, the one mined while it
	// was confirming and the two below would make three and stall; counted from the head the reorg was
	// found at they are two.
	chain.waitEvaluated(t, included+1)
	chain.mine(0.5, nil)
	chain.waitEvaluated(t, chain.mine(0.5, nil))
	if held := chain.sends(); len(held) != 2 {
		t.Fatalf("after the reorg the write endpoint received %d transactions, want no stall response", len(held))
	}
	chain.mine(0.5, first)
	chain.mine(0.5, nil)
	chain.mine(0.5, nil)
	if got := waitResult(t, result); got.Outcome != OutcomeConfirmed || got.Hash != first.Hash() {
		t.Fatalf("result = %+v, want the rebroadcast confirmed", got)
	}
	if h.countLog("pending transaction rebroadcast", `"reason":"reorg"`) != 1 {
		t.Fatal("the reorg rebroadcast was not logged")
	}
}

// TestHorizonRebroadcastsWithinTheLegacySlack drives one evaluation tick at production timing (12 s blocks,
// a 5 s broadcast timeout): the horizon policy keeps an exact rebroadcast, of an ambiguous broadcast or of a
// stalled call, up to 17 s before the cancellation deadline, where the legacy policy's 35 s slack (with a 30 s
// replacement interval) would already have given it up.
func TestHorizonRebroadcastsWithinTheLegacySlack(t *testing.T) {
	for _, tc := range []struct {
		name          string
		ambiguous     bool // the first broadcast's result was ambiguous
		stalledBlocks int  // roomy blocks mined before the tick
		untilDeadline time.Duration
		wantReason    string // the rebroadcast expected, or none
	}{
		{name: "ambiguous broadcast 20 s before the deadline", ambiguous: true, untilDeadline: 20 * time.Second, wantReason: rebroadcastUncertain},
		{name: "ambiguous broadcast 16 s before the deadline", ambiguous: true, untilDeadline: 16 * time.Second},
		{name: "stall 20 s before the deadline", stalledBlocks: 3, untilDeadline: 20 * time.Second, wantReason: rebroadcastStall},
		{name: "stall 16 s before the deadline", stalledBlocks: 3, untilDeadline: 16 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain := newPendingChain(big.NewInt(1e18))
			if tc.ambiguous {
				chain.sendErrs = []error{errors.New("write endpoint: connection reset by peer")}
			}
			metrics := newTestMetrics(t)
			m := NewWithMetrics(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(), metrics, logr.Discard())
			ctx := managerCtx(t.Context(), m)
			pending, err := m.broadcast(ctx, fillRequest(100_000))
			if err != nil {
				t.Fatalf("broadcast: %v", err)
			}
			pending.cancelDeadline = time.Now().Add(tc.untilDeadline)
			for range tc.stalledBlocks {
				chain.mine(0.5, nil)
			}
			unexpected := func(string) { t.Error("the tick replaced or cancelled the call") }
			evaluator := m.newPendingEvaluator(pending, pendingActions{
				replace: func(bool, *replacementPlan) { unexpected("") },
				cancel:  unexpected,
			})
			evaluator.tick(ctx, false)
			attempted := chain.attemptedTransactions()
			wantSends := 1
			if tc.wantReason != "" {
				wantSends = 2
				assertMetric(t, metrics.rebroadcasts.WithLabelValues("fill", tc.wantReason), 1)
			}
			if len(attempted) != wantSends || attempted[len(attempted)-1].Hash() != attempted[0].Hash() {
				t.Fatalf("write endpoint received %d transactions, want %d of the same bytes", len(attempted), wantSends)
			}
			if len(pending.attempts) != 1 {
				t.Fatalf("the tick signed %d attempts", len(pending.attempts)-1)
			}
		})
	}
}

// recordedActions records what an evaluator driven directly by a test asked the lifecycle loop to do.
type recordedActions struct {
	replaced []recordedReplace
	cancels  []string
}

type recordedReplace struct {
	cancellation bool
	plan         *replacementPlan
}

func (r *recordedActions) actions() pendingActions {
	return pendingActions{
		replace: func(cancellation bool, plan *replacementPlan) {
			r.replaced = append(r.replaced, recordedReplace{cancellation: cancellation, plan: plan})
		},
		cancel: func(reason string) { r.cancels = append(r.cancels, reason) },
	}
}

// direct is a horizon lifecycle an evaluator drives directly in a test, its actions recorded rather than
// performed.
type direct struct {
	m         *Manager
	pending   *pendingTransaction
	evaluator *pendingEvaluator
	recorded  *recordedActions
}

// directEvaluator broadcasts req on an unstarted horizon manager at 12 s blocks and returns an evaluator of
// its lifecycle, with the context its methods run on.
func directEvaluator(t *testing.T, chain *pendingChain, req Request, nextBlock bool) (context.Context, direct) {
	t.Helper()
	metrics := newTestMetrics(t)
	m := NewWithMetrics(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(), metrics, logr.Discard())
	if nextBlock {
		m.overrides.swap(blockOverridesSupported)
	}
	ctx := managerCtx(t.Context(), m)
	pending, err := m.broadcast(ctx, req)
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	recorded := &recordedActions{}
	return ctx, direct{m: m, pending: pending, evaluator: m.newPendingEvaluator(pending, recorded.actions()), recorded: recorded}
}

func TestPendingEvaluatorIgnoresLaggingAndStaleHeads(t *testing.T) {
	chain := newPendingChain(big.NewInt(1e18))
	ctx, d := directEvaluator(t, chain, fillRequest(100_000), false)
	m, evaluator, recorded := d.m, d.evaluator, d.recorded
	for range 3 {
		chain.mine(0.5, nil)
	}
	evaluator.tick(ctx, false) // head 103: a stall, rebroadcast
	if sends := chain.sends(); len(sends) != 2 || evaluator.stallBase != 103 {
		t.Fatalf("stall at head 103: %d sends, stall base %d", len(sends), evaluator.stallBase)
	}
	// A lagging upstream answers with head 101: ignored, the stall count keeps its base.
	chain.set(func(b *horizonBackend) { b.number = 101 })
	evaluator.tick(ctx, false)
	// Head 106 three blocks after the rebroadcast, but a minute old: nothing is decided from it.
	chain.set(func(b *horizonBackend) { b.number, b.headAges = 106, []time.Duration{time.Minute} })
	evaluator.tick(ctx, false)
	if sends := chain.sends(); len(sends) != 2 || evaluator.maxSeenHead != 106 {
		t.Fatalf("lagging and stale heads led to %d sends, max seen head %d", len(sends), evaluator.maxSeenHead)
	}
	if evaluator.failingSince.IsZero() {
		t.Fatal("the stale head did not count toward the fallback")
	}
	// A fresh head 107: four roomy misses since the rebroadcast, the second stall rebroadcast.
	chain.set(func(b *horizonBackend) { b.number, b.headAges = 107, []time.Duration{0} })
	evaluator.tick(ctx, false)
	if sends := chain.sends(); len(sends) != 3 || evaluator.rebroadcasts != 2 || !evaluator.failingSince.IsZero() {
		t.Fatalf("fresh head 107: %d sends, %d stall rebroadcasts, failing since %s; want the second rebroadcast and the reads recovered",
			len(sends), evaluator.rebroadcasts, evaluator.failingSince)
	}
	if len(recorded.replaced) != 0 || len(recorded.cancels) != 0 {
		t.Fatalf("the evaluator replaced %d and cancelled %d times", len(recorded.replaced), len(recorded.cancels))
	}
	assertMetric(t, m.metrics.rebroadcasts.WithLabelValues("fill", rebroadcastStall), 2)
}

func TestPendingEvaluatorReestimateOutcomes(t *testing.T) {
	reverted := rpcCodeError{code: 3, message: "execution reverted"}
	for _, tc := range []struct {
		name        string
		validity    bool   // raise the next base fee so the head reprices rather than stalls
		estimateErr error  // the re-estimate's error
		minedNonce  bool   // the call landed before the re-estimate: its nonce is mined, its receipt canonical
		cancelling  bool   // a cancellation began while the re-estimate ran
		estimate    uint64 // the re-estimate's gas; 100000 when zero
		wantSends   int    // write endpoint transactions, the first send included
		wantReplace *replacementPlan
		wantCancel  bool
	}{
		{name: "an unavailable estimate still rebroadcasts a stall", estimateErr: errors.New("upstream timeout"), wantSends: 2},
		{
			name: "an unavailable estimate reprices at the same gas limit", validity: true, estimateErr: errors.New("upstream timeout"),
			wantSends: 1, wantReplace: &replacementPlan{reason: repriceValidity},
		},
		{name: "a revert cancels", estimateErr: reverted, wantSends: 1, wantCancel: true},
		{name: "a revert after the call landed does not cancel", estimateErr: reverted, minedNonce: true, wantSends: 1},
		{name: "a reprice keeps the larger gas limit", validity: true, wantSends: 1, wantReplace: &replacementPlan{reason: repriceValidity, gas: 105_000}},
		{name: "a cancellation drops the estimate", estimateErr: reverted, cancelling: true, wantSends: 1},
		// 102000 is above the 100000 the gas limit was estimated at but within its 105000: noise, not a
		// landed fill, so the stall rebroadcasts rather than replacing the gas limit.
		{name: "an estimate that grew within the gas limit rebroadcasts a stall", estimate: 102_000, wantSends: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain := newPendingChain(big.NewInt(1e18))
			ctx, d := directEvaluator(t, chain, fillRequest(0), true)
			pending, evaluator, recorded := d.pending, d.evaluator, d.recorded
			if pending.gas != 105_000 {
				t.Fatalf("gas limit = %d, want the next-block estimate", pending.gas)
			}
			chain.set(func(b *horizonBackend) {
				b.nextBlockErr = tc.estimateErr
				if tc.estimate > 0 {
					b.nextBlockGas = tc.estimate
				}
				if tc.validity {
					b.nextBase = gwei(1.7)
				}
			})
			blocks := 3
			if tc.validity {
				blocks = 1
			}
			for range blocks {
				chain.mine(0.5, nil)
			}
			if tc.minedNonce {
				chain.mine(0.5, pending.attempts[0].tx)
			}
			evaluator.tick(ctx, false)
			select {
			case <-evaluator.estimated():
			case <-time.After(5 * time.Second):
				t.Fatal("no re-estimate ran")
			}
			evaluator.finishEstimate(ctx, tc.cancelling)
			if sends := chain.sends(); len(sends) != tc.wantSends {
				t.Fatalf("write endpoint received %d transactions, want %d", len(sends), tc.wantSends)
			}
			if (len(recorded.cancels) == 1) != tc.wantCancel || len(recorded.cancels) > 1 {
				t.Fatalf("cancellations = %v, want %t", recorded.cancels, tc.wantCancel)
			}
			if tc.wantCancel && recorded.cancels[0] != cancelReasonSimulatedRevert {
				t.Fatalf("cancellation reason = %q", recorded.cancels[0])
			}
			switch {
			case tc.wantReplace == nil && len(recorded.replaced) != 0:
				t.Fatalf("replaced %+v, want no replacement", recorded.replaced[0].plan)
			case tc.wantReplace != nil && (len(recorded.replaced) != 1 || recorded.replaced[0].cancellation ||
				recorded.replaced[0].plan.reason != tc.wantReplace.reason || recorded.replaced[0].plan.gas != tc.wantReplace.gas ||
				recorded.replaced[0].plan.snapshot == nil):
				t.Fatalf("replaced %+v, want one call replacement %+v priced from the snapshot", recorded.replaced, tc.wantReplace)
			}
		})
	}
}

func TestPendingEvaluatorAbandonsAnEstimateInFlight(t *testing.T) {
	chain := newPendingChain(big.NewInt(1e18))
	ctx, d := directEvaluator(t, chain, fillRequest(0), true)
	m, evaluator, recorded := d.m, d.evaluator, d.recorded
	chain.set(func(b *horizonBackend) { b.blockEstimates = make(chan struct{}) })
	for range 3 {
		chain.mine(0.5, nil)
	}
	evaluator.tick(ctx, false)
	if evaluator.estimated() == nil {
		t.Fatal("the stall did not start a re-estimate")
	}
	evaluator.tick(ctx, false) // no evaluation while it runs
	evaluator.abandonEstimate()
	if evaluator.estimated() != nil || len(recorded.replaced) != 0 || len(chain.sends()) != 1 {
		t.Fatal("the abandoned re-estimate still acted")
	}
	// Only the send's estimate is an outcome; the abandoned one is not counted.
	assertMetric(t, m.metrics.gasEstimates.WithLabelValues("fill", estimateModeNextBlock, gasEstimateOK), 1)
	assertMetric(t, m.metrics.gasEstimates.WithLabelValues("fill", estimateModeNextBlock, gasEstimateError), 0)
}

func TestPendingEvaluatorTickRetriesTheFirstCancellation(t *testing.T) {
	chain := newPendingChain(big.NewInt(1e18))
	ctx, d := directEvaluator(t, chain, fillRequest(100_000), false)
	m, pending, evaluator, recorded := d.m, d.pending, d.evaluator, d.recorded
	evaluator.tick(ctx, true)
	if len(recorded.replaced) != 1 || !recorded.replaced[0].cancellation || recorded.replaced[0].plan != nil {
		t.Fatalf("replaced %+v, want the first cancellation retried", recorded.replaced)
	}
	// It failed again: the next ticks, half a block apart, wait out the replacement interval, as the
	// legacy policy does, rather than repeating its reads and its Error log at every tick.
	evaluator.tick(ctx, true)
	evaluator.tick(ctx, true)
	if len(recorded.replaced) != 1 {
		t.Fatalf("the first cancellation was retried %d times within one replacement interval", len(recorded.replaced))
	}
	evaluator.lastCancelRetry = time.Now().Add(-m.cfg.ReplacementInterval)
	evaluator.tick(ctx, true)
	if len(recorded.replaced) != 2 || !recorded.replaced[1].cancellation {
		t.Fatalf("replaced %+v, want the first cancellation retried after the replacement interval", recorded.replaced)
	}
	m.markNonceConflict(pending.nonce, pending.originalHash)
	evaluator.lastCancelRetry = time.Now().Add(-m.cfg.ReplacementInterval)
	evaluator.tick(ctx, true)
	if len(recorded.replaced) != 2 {
		t.Fatal("a conflicted nonce was replaced")
	}
}

func TestHorizonCancellationWithoutASnapshotBumpsTheLatestFees(t *testing.T) {
	chain := newPendingChain(big.NewInt(1e18))
	ctx, d := directEvaluator(t, chain, fillRequest(100_000), false)
	m, pending := d.m, d.pending
	first := pending.attempts[0].tx
	chain.set(func(b *horizonBackend) { b.historyErr = errors.New("upstream unavailable") })
	if cancelling, err := m.tryReplace(ctx, pending, true); !cancelling || err != nil {
		t.Fatalf("tryReplace = %t, %v", cancelling, err)
	}
	cancellation := chain.sends()[1]
	if cancellation.Gas() != cancellationGasLimit || cancellation.GasFeeCap().Cmp(bumpFee(first.GasFeeCap())) != 0 ||
		cancellation.GasTipCap().Cmp(bumpFee(first.GasTipCap())) != 0 {
		t.Fatalf("cancellation = gas %d max %s tip %s, want the cached bump", cancellation.Gas(), cancellation.GasFeeCap(), cancellation.GasTipCap())
	}
}

func TestPendingStallRebroadcastFailureIsRetried(t *testing.T) {
	chain := newPendingChain(big.NewInt(1e18))
	chain.sendErrs = []error{nil, errors.New("relay unavailable")}
	ctx, d := directEvaluator(t, chain, fillRequest(100_000), false)
	m, evaluator := d.m, d.evaluator
	for range 3 {
		chain.mine(0.5, nil)
	}
	evaluator.tick(ctx, false)
	if evaluator.rebroadcasts != 0 || evaluator.stallBase != 100 {
		t.Fatalf("a failed rebroadcast counted: %d rebroadcasts, stall base %d", evaluator.rebroadcasts, evaluator.stallBase)
	}
	assertMetric(t, m.metrics.rebroadcasts.WithLabelValues("fill", rebroadcastStall), 0)
	chain.mine(0.5, nil)
	evaluator.tick(ctx, false)
	if evaluator.rebroadcasts != 1 || len(chain.sends()) != 2 {
		t.Fatalf("the next head did not retry the stall rebroadcast: %d rebroadcasts", evaluator.rebroadcasts)
	}
}
