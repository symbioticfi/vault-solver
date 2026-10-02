package txmanager

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

const (
	horizonTestGas = 4_000_000
	// horizonTestBlockTime is the slot time of horizonConfig and of blocks mineSlots produces.
	horizonTestBlockTime = 20 * time.Millisecond
)

func block(number uint64, baseFeeGwei, ratio, rewardGwei float64) blockFee {
	return blockFee{number: number, baseFee: gweiToWei(baseFeeGwei), gasUsedRatio: ratio, reward: gweiToWei(rewardGwei)}
}

func testSnapshot(nextBaseFeeGwei float64, blocks ...blockFee) feeSnapshot {
	return feeSnapshot{
		head: blocks[len(blocks)-1].number, gasLimit: 60_000_000,
		nextBaseFee: gweiToWei(nextBaseFeeGwei), blocks: blocks,
	}
}

func TestGrowBaseFee(t *testing.T) {
	for _, tc := range []struct {
		base   int64
		blocks int
		want   int64
	}{
		{base: 100, blocks: 0, want: 100},
		{base: 100, blocks: 1, want: 112},
		{base: 8, blocks: 1, want: 9},
		{base: 7, blocks: 1, want: 8}, // the protocol raises a full block's base fee by at least 1 wei
		{base: 0, blocks: 3, want: 3},
		{base: 1_000_000_000, blocks: 5, want: 1_802_032_470},
	} {
		if got := growBaseFee(big.NewInt(tc.base), tc.blocks); got.Cmp(big.NewInt(tc.want)) != 0 {
			t.Errorf("growBaseFee(%d, %d) = %s, want %d", tc.base, tc.blocks, got, tc.want)
		}
	}
	if got := horizonMaxFee(gweiToWei(1), gweiToWei(0.02), 6); got.Cmp(big.NewInt(1_822_032_470)) != 0 {
		t.Errorf("horizonMaxFee(1 gwei, 0.02 gwei, 6) = %s, want 1822032470", got)
	}
}

func TestHorizonTip(t *testing.T) {
	policy := newHorizonPolicy(HorizonConfig{})
	for name, tc := range map[string]struct {
		blocks []blockFee
		want   float64
	}{
		"both recent blocks had room": {
			blocks: []blockFee{block(98, 1, 0.5, 9), block(99, 1, 0.5, 9), block(100, 1, 0.93, 9)}, want: 0.02,
		},
		"latest block was full": {
			blocks: []blockFee{block(98, 1, 0.5, 9), block(99, 1, 0.5, 9), block(100, 1, 0.94, 9)}, want: 0.1,
		},
		"previous block was full": {
			blocks: []blockFee{block(98, 1, 0.5, 9), block(99, 1, 0.95, 9), block(100, 1, 0.5, 9)}, want: 0.1,
		},
		"run of full blocks follows the market": {
			blocks: []blockFee{block(98, 1, 0.5, 0.001), block(99, 1, 1, 3), block(100, 1, 0.99, 2)}, want: 3,
		},
		"market tip is capped": {
			blocks: []blockFee{block(98, 1, 0.5, 0.001), block(99, 1, 1, 30), block(100, 1, 1, 12)}, want: 15,
		},
		"market tip has a floor": {
			blocks: []blockFee{block(98, 1, 0.5, 0.001), block(99, 1, 1, 0.05), block(100, 1, 1, 0.01)}, want: 0.2,
		},
		"reward window ignores older blocks": {
			blocks: []blockFee{
				block(97, 1, 0.5, 9), block(98, 1, 0.5, 0.5), block(99, 1, 1, 1), block(100, 1, 1, 2),
			},
			want: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := horizonTip(testSnapshot(1, tc.blocks...), horizonTestGas, policy); got.Cmp(gweiToWei(tc.want)) != 0 {
				t.Fatalf("horizonTip = %s, want %s", got, gweiToWei(tc.want))
			}
		})
	}
}

func TestHorizonFees(t *testing.T) {
	policy := newHorizonPolicy(HorizonConfig{})
	roomy := testSnapshot(1, block(98, 1, 0.5, 0), block(99, 1, 0.5, 0), block(100, 1, 0.5, 0))
	for name, tc := range map[string]struct {
		limit   *big.Int
		wantTip *big.Int
		wantMax *big.Int
		wantErr string
	}{
		"exact bound for the horizon":   {wantTip: gweiToWei(0.02), wantMax: big.NewInt(1_822_032_470)},
		"limit shortens the horizon":    {limit: gweiToWei(1.5), wantTip: gweiToWei(0.02), wantMax: gweiToWei(1.5)},
		"limit trims the tip":           {limit: gweiToWei(1.01), wantTip: gweiToWei(0.01), wantMax: gweiToWei(1.01)},
		"limit below the next base fee": {limit: gweiToWei(0.9), wantErr: "fee limit reached"},
	} {
		t.Run(name, func(t *testing.T) {
			fees, err := horizonFees(roomy, horizonTestGas, tc.limit, policy)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("horizonFees error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || fees.tip.Cmp(tc.wantTip) != 0 || fees.maxFee.Cmp(tc.wantMax) != 0 ||
				fees.baseFee.Cmp(gweiToWei(1)) != 0 {
				t.Fatalf("horizonFees = %+v, %v; want tip %s max %s base 1 gwei", fees, err, tc.wantTip, tc.wantMax)
			}
		})
	}
}

func TestJudgedSentHead(t *testing.T) {
	sentAt := time.Unix(1_000, 0)
	for name, tc := range map[string]struct {
		sentHead, head uint64
		elapsed        time.Duration
		want           uint64
	}{
		"an accurate send head stands":                  {sentHead: 100, head: 101, elapsed: 12400 * time.Millisecond, want: 100},
		"a stale send head is raised to the wall clock": {sentHead: 97, head: 101, elapsed: 12400 * time.Millisecond, want: 99},
		"within the first slot one block can follow":    {sentHead: 97, head: 101, elapsed: time.Second, want: 100},
		"enough elapsed slots keep the send head":       {sentHead: 97, head: 101, elapsed: time.Minute, want: 97},
		"a clock step back counts as no time":           {sentHead: 97, head: 101, elapsed: -time.Minute, want: 100},
		"a head within the bound keeps the send head":   {sentHead: 0, head: 1, elapsed: 0, want: 0},
	} {
		t.Run(name, func(t *testing.T) {
			got := judgedSentHead(tc.sentHead, tc.head, sentAt, sentAt.Add(tc.elapsed), 12*time.Second)
			if got != tc.want {
				t.Fatalf("judged send head = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDecidePending(t *testing.T) {
	policy := newHorizonPolicy(HorizonConfig{})
	attempt := feeQuote{baseFee: gweiToWei(1), tip: gweiToWei(0.02), maxFee: gweiToWei(1.8)}
	sent := block(100, 1, 0.5, 0.001)
	for name, tc := range map[string]struct {
		snapshot feeSnapshot
		attempt  feeQuote
		want     pendingDecision
	}{
		"no block since send": {
			snapshot: testSnapshot(1, block(98, 1, 0.5, 0), block(99, 1, 0.5, 0), sent),
			want:     pendingDecision{action: pendingHold},
		},
		"fee cap lapses within two blocks": {
			snapshot: testSnapshot(1.7, block(99, 1, 0.5, 0), sent, block(101, 1, 0.5, 0)),
			want:     pendingDecision{action: pendingReprice, reason: replaceReasonValidity},
		},
		"one full block is not a run": {
			snapshot: testSnapshot(1, block(99, 1, 0.5, 0), sent, block(101, 1, 1, 1)),
			want:     pendingDecision{action: pendingHold},
		},
		"run of full blocks escalates": {
			snapshot: testSnapshot(1, sent, block(101, 1, 1, 1), block(102, 1, 1, 1)),
			want:     pendingDecision{action: pendingReprice, reason: replaceReasonCongestion},
		},
		"run of full blocks holds a tip already above the market": {
			snapshot: testSnapshot(1, sent, block(101, 1, 1, 1), block(102, 1, 1, 1)),
			attempt:  feeQuote{baseFee: gweiToWei(1), tip: gweiToWei(2), maxFee: gweiToWei(3)},
			want:     pendingDecision{action: pendingHold},
		},
		"full blocks the attempt could not enter do not escalate": {
			snapshot: testSnapshot(1, sent, block(101, 1.9, 1, 1), block(102, 1.9, 1, 1)),
			want:     pendingDecision{action: pendingHold},
		},
		"only a trailing run escalates": {
			snapshot: testSnapshot(1, block(101, 1, 1, 1), block(102, 1, 0.5, 0), block(103, 1, 1, 1)),
			want:     pendingDecision{action: pendingHold},
		},
		"blocks with room lost while valid stall": {
			snapshot: testSnapshot(1, block(101, 1, 0.5, 0), block(102, 1, 0.5, 0), block(103, 1, 0.5, 0)),
			want:     pendingDecision{action: pendingStall, reason: replaceReasonStall},
		},
		"blocks up to the send head are not misses": {
			snapshot: testSnapshot(1, block(99, 1, 0.5, 0), sent, block(101, 1, 0.5, 0)),
			want:     pendingDecision{action: pendingHold},
		},
	} {
		t.Run(name, func(t *testing.T) {
			current := attempt
			if tc.attempt.maxFee != nil {
				current = tc.attempt
			}
			if got := decidePending(tc.snapshot, current, horizonTestGas, 100, policy); got != tc.want {
				t.Fatalf("decidePending = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseFeeSnapshot(t *testing.T) {
	valid := func() *ethereum.FeeHistory {
		return &ethereum.FeeHistory{
			OldestBlock:  big.NewInt(98),
			BaseFee:      []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3), big.NewInt(4)},
			GasUsedRatio: []float64{0.1, 0.2, 1.3},
			Reward:       [][]*big.Int{{big.NewInt(5)}, {big.NewInt(6)}, {big.NewInt(7)}},
		}
	}
	snapshot, err := parseFeeSnapshot(valid(), 60_000_000)
	if err != nil {
		t.Fatalf("parseFeeSnapshot: %v", err)
	}
	if snapshot.head != 100 || snapshot.nextBaseFee.Int64() != 4 || len(snapshot.blocks) != 3 ||
		snapshot.blocks[2].number != 100 || snapshot.blocks[2].gasUsedRatio != 1 || snapshot.blocks[0].reward.Int64() != 5 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	for name, mutate := range map[string]func(*ethereum.FeeHistory){
		"no oldest block":       func(h *ethereum.FeeHistory) { h.OldestBlock = nil },
		"base fee count":        func(h *ethereum.FeeHistory) { h.BaseFee = h.BaseFee[:3] },
		"reward row count":      func(h *ethereum.FeeHistory) { h.Reward = h.Reward[:2] },
		"missing reward":        func(h *ethereum.FeeHistory) { h.Reward[1] = nil },
		"negative ratio":        func(h *ethereum.FeeHistory) { h.GasUsedRatio[0] = -0.1 },
		"missing base fee":      func(h *ethereum.FeeHistory) { h.BaseFee[1] = nil },
		"missing next base fee": func(h *ethereum.FeeHistory) { h.BaseFee[3] = nil },
		"empty history":         func(h *ethereum.FeeHistory) { h.GasUsedRatio, h.BaseFee, h.Reward = nil, h.BaseFee[:1], nil },
	} {
		t.Run(name, func(t *testing.T) {
			history := valid()
			mutate(history)
			if _, err := parseFeeSnapshot(history, 60_000_000); err == nil {
				t.Fatal("invalid fee history was accepted")
			}
		})
	}
}

type testRPCError struct {
	code    int
	message string
}

func (e testRPCError) Error() string  { return e.message }
func (e testRPCError) ErrorCode() int { return e.code }

func TestIsBlockOverrideUnsupported(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{err: testRPCError{code: -32602, message: "invalid params"}, want: true},
		{err: errors.Errorf("estimate: %w", testRPCError{code: -32602, message: "bad"}), want: true},
		{err: errors.New("too many arguments, want at most 3"), want: true},
		{err: testRPCError{code: 3, message: "execution reverted: NonceUsed"}, want: false},
		{err: errors.New("execution reverted"), want: false},
		{err: errors.New("dial tcp: connection refused"), want: false},
	} {
		if got := isBlockOverrideUnsupported(tc.err); got != tc.want {
			t.Errorf("isBlockOverrideUnsupported(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestWithGasHeadroom(t *testing.T) {
	for _, tc := range []struct {
		gas  uint64
		bps  int
		want uint64
	}{
		{gas: 100_000, bps: 500, want: 105_000},
		{gas: 3_674_547, bps: 800, want: 3_968_510},
		{gas: 0, bps: 500, want: 0},
	} {
		if got := withGasHeadroom(tc.gas, tc.bps); got != tc.want {
			t.Errorf("withGasHeadroom(%d, %d) = %d, want %d", tc.gas, tc.bps, got, tc.want)
		}
	}
}

// horizonChain scripts mined blocks for horizon tests. Nothing it receives is included unless a test
// includes it unless a test explicitly publishes a receipt.
type horizonChain struct {
	*mockBackend

	blocks           []blockFee
	nextBaseFee      *big.Int
	feeErr           error
	nextBlockGas     uint64
	nextBlockErr     error
	nextBlockParents []uint64
	// hangEstimates leaves every estimate that would succeed unanswered until its context ends, the way
	// a read endpoint that keeps the connection open but never replies does.
	hangEstimates bool
}

func newHorizonChain(t *testing.T) *horizonChain {
	t.Helper()
	chain := &horizonChain{mockBackend: newMockBackend(), nextBaseFee: gweiToWei(1), nextBlockGas: 100_000}
	for number := uint64(98); number <= 100; number++ {
		chain.blocks = append(chain.blocks, block(number, 1, 0.5, 0.001))
	}
	chain.head = 100
	return chain
}

// mineSlots mines one block per slot, the way a chain produces them, so evidence the manager bounds
// by elapsed slots accrues as it would on chain.
func (c *horizonChain) mineSlots(rewardGwei float64, ratios ...float64) {
	for _, ratio := range ratios {
		time.Sleep(horizonTestBlockTime)
		c.mine(rewardGwei, ratio)
	}
}

// mine appends blocks at the current next base fee, one per gas used ratio, all at once.
func (c *horizonChain) mine(rewardGwei float64, ratios ...float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ratio := range ratios {
		c.head++
		c.blocks = append(c.blocks, blockFee{
			number: c.head, baseFee: new(big.Int).Set(c.nextBaseFee), gasUsedRatio: ratio, reward: gweiToWei(rewardGwei),
		})
	}
}

func (c *horizonChain) set(mutate func(*horizonChain)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	mutate(c)
}

func (c *horizonChain) FeeHistory(
	_ context.Context, blockCount uint64, _ *big.Int, _ []float64,
) (*ethereum.FeeHistory, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.feeErr != nil {
		return nil, c.feeErr
	}
	window := c.blocks[max(0, len(c.blocks)-int(blockCount)):]
	history := &ethereum.FeeHistory{OldestBlock: new(big.Int).SetUint64(window[0].number)}
	for _, mined := range window {
		history.BaseFee = append(history.BaseFee, new(big.Int).Set(mined.baseFee))
		history.GasUsedRatio = append(history.GasUsedRatio, mined.gasUsedRatio)
		history.Reward = append(history.Reward, []*big.Int{new(big.Int).Set(mined.reward)})
	}
	history.BaseFee = append(history.BaseFee, new(big.Int).Set(c.nextBaseFee))
	return history, nil
}

func (c *horizonChain) EstimateGasNextBlock(
	ctx context.Context, _ ethereum.CallMsg, parent *types.Header, _ time.Duration,
) (uint64, error) {
	c.mu.Lock()
	c.nextBlockParents = append(c.nextBlockParents, parent.Number.Uint64())
	gas, err, hang := c.nextBlockGas, c.nextBlockErr, c.hangEstimates
	c.mu.Unlock()
	if hang && err == nil {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	return gas, err
}

// EstimateGas is the latest-state fallback. It hangs like the next-block estimate and counts every
// call, answered or not.
func (c *horizonChain) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	c.mu.Lock()
	hang := c.hangEstimates
	c.mu.Unlock()
	if !hang {
		return c.mockBackend.EstimateGas(ctx, call)
	}
	c.estimateCalls.Add(1)
	<-ctx.Done()
	return 0, ctx.Err()
}

// hungEstimate is where a stalled call's re-estimate hangs: in the next-block estimate, or in the
// latest-state fallback of a read RPC that rejects block overrides.
type hungEstimate struct {
	nextBlockErr    error
	latestEstimates int64 // latest-state estimates the lifecycle makes, the initial one included
}

var hungEstimates = map[string]hungEstimate{
	"next-block estimate": {},
	"latest-state fallback": {
		nextBlockErr: testRPCError{code: -32602, message: "too many arguments, want at most 3"}, latestEstimates: 2,
	},
}

// waitForEstimates waits until count next-block estimates have been requested, answered or not.
func (c *horizonChain) waitForEstimates(t *testing.T, count int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		requested := len(c.nextBlockParents)
		c.mu.Unlock()
		if requested >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d next-block estimates", count)
}

func (c *horizonChain) SendTransaction(_ context.Context, tx *types.Transaction) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sendCalls++
	c.attempted = append(c.attempted, tx)
	c.sent = append(c.sent, tx)
	return nil
}

func (c *horizonChain) include(tx *types.Transaction) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.receipts[tx.Hash()] = successfulReceipt(tx, c.head)
}

func (c *horizonChain) sentTransactions() []*types.Transaction {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*types.Transaction(nil), c.sent...)
}

func (c *horizonChain) waitForSends(t *testing.T, count int) []*types.Transaction {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sent := c.sentTransactions(); len(sent) >= count {
			return sent
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d broadcasts; have %d", count, len(c.sentTransactions()))
	return nil
}

func horizonConfig(overrides func(*Config)) Config {
	cfg := Config{
		MaxFeeGwei: 50, PollInterval: time.Millisecond, ReplacementInterval: time.Hour, PendingTimeout: time.Hour, ShutdownTimeout: 20 * time.Millisecond,
		Horizon: HorizonConfig{BlockTime: horizonTestBlockTime},
	}
	if overrides != nil {
		overrides(&cfg)
	}
	return cfg
}

func TestHorizonBroadcastEstimatesForNextBlockAndCapsAtExactBound(t *testing.T) {
	chain := newHorizonChain(t)
	m := New(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(nil), logr.Discard())

	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{
		To: common.HexToAddress("0xabc"), Data: []byte{0x01}, Label: "fill",
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	tx := chain.sentTransactions()[0]
	if tx.Gas() != 105_000 || pending.gas != 105_000 {
		t.Fatalf("gas limit = %d, want the next-block estimate plus 5%%", tx.Gas())
	}
	if len(chain.nextBlockParents) != 1 || chain.nextBlockParents[0] != 100 || chain.estimateCalls.Load() != 0 {
		t.Fatalf("next-block estimates at %v, latest-state estimates %d; want one at head 100",
			chain.nextBlockParents, chain.estimateCalls.Load())
	}
	if tx.GasTipCap().Cmp(gweiToWei(0.02)) != 0 || tx.GasFeeCap().Cmp(big.NewInt(1_822_032_470)) != 0 {
		t.Fatalf("fees = tip %s cap %s, want the tip floor and the six-block bound", tx.GasTipCap(), tx.GasFeeCap())
	}
	if pending.horizon.sentHead != 100 || m.lastGas.Load() != 105_000 {
		t.Fatalf("send head = %d, last gas = %d", pending.horizon.sentHead, m.lastGas.Load())
	}
}

func TestBroadcastSendHeadIsTheFresherOfFeeWindowAndHeader(t *testing.T) {
	chain := newHorizonChain(t)
	chain.head = 103 // the header is ahead of a fee window that still ends at block 100
	m := New(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(nil), logr.Discard())

	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{
		To: common.HexToAddress("0xabc"), Data: []byte{0x01}, Label: "fill",
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if pending.horizon.sentHead != 103 || pending.horizon.lastHead != 103 || pending.horizon.sentAt.IsZero() {
		t.Fatalf("send head = %d, last head = %d, sent at %s; want 103, 103 and a send time",
			pending.horizon.sentHead, pending.horizon.lastHead, pending.horizon.sentAt)
	}
}

// A fee window that lagged when the call went out must not turn blocks mined before the send into
// blocks the call missed: two processes on a stale read endpoint reported a stall 12 seconds after
// sending, with one block mined in between.
func TestStaleSendHeadDoesNotReportAStallBeforeSlotsElapse(t *testing.T) {
	chain := newHorizonChain(t)
	chain.set(func(c *horizonChain) {
		c.head = 97
		c.blocks = []blockFee{block(95, 1, 0.5, 0.001), block(96, 1, 0.5, 0.001), block(97, 1, 0.5, 0.001)}
	})
	m := New(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(func(cfg *Config) {
		cfg.Horizon.BlockTime = 12 * time.Second
	}), logr.Discard())
	ctx := managerCtx(t.Context(), m)
	pending, err := m.broadcast(ctx, Request{To: common.HexToAddress("0xabc"), Data: []byte{0x01}, Label: "fill"})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	var intents []replaceIntent
	replace := func(intent replaceIntent) bool {
		intents = append(intents, intent)
		return false
	}

	// The endpoint catches up: four blocks with room, all mined before the send in fact.
	chain.mine(0.001, 0.5, 0.5, 0.5, 0.5)
	m.evaluateHorizon(ctx, pending, replace)
	if sent := chain.sentTransactions(); len(sent) != 1 || len(intents) != 0 {
		t.Fatalf("stale send head acted within the first slot: %d sends, intents %+v", len(sent), intents)
	}

	// Once enough slots have passed, the same evidence is a stall and the call is rebroadcast.
	pending.horizon.sentAt = time.Now().Add(-time.Minute)
	chain.mine(0.001, 0.5)
	m.evaluateHorizon(ctx, pending, replace)
	if sent := chain.sentTransactions(); len(sent) != 2 || sent[1].Hash() != sent[0].Hash() || len(intents) != 0 {
		t.Fatalf("stall after elapsed slots: %d sends, intents %+v; want one exact rebroadcast", len(sent), intents)
	}
}

func TestHorizonBroadcastGasFallbacks(t *testing.T) {
	for name, tc := range map[string]struct {
		nextBlockErr error
		request      Request
		wantGas      uint64
		wantErr      bool
		wantLatest   int64
	}{
		"override unsupported uses a latest-state estimate with wider headroom": {
			nextBlockErr: testRPCError{code: -32602, message: "too many arguments, want at most 3"},
			wantGas:      55_000, wantLatest: 1,
		},
		"a reverting next-block estimate fails the send": {
			nextBlockErr: testRPCError{code: 3, message: "execution reverted"}, wantErr: true,
		},
		"a fixed gas limit is kept": {
			request: Request{GasLimit: 77_000}, wantGas: 77_000,
		},
	} {
		t.Run(name, func(t *testing.T) {
			chain := newHorizonChain(t)
			chain.nextBlockErr = tc.nextBlockErr
			m := New(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(nil), logr.Discard())
			req := tc.request
			req.To, req.Label = common.HexToAddress("0xabc"), "fill"

			pending, err := m.broadcast(managerCtx(t.Context(), m), req)
			if tc.wantErr {
				if err == nil || len(chain.sentTransactions()) != 0 {
					t.Fatalf("broadcast = %v with %d sends, want an error before signing", err, len(chain.sentTransactions()))
				}
				return
			}
			if err != nil || pending.gas != tc.wantGas || chain.estimateCalls.Load() != tc.wantLatest {
				t.Fatalf("broadcast err %v, gas %d, latest estimates %d; want gas %d and %d latest estimates",
					err, pending.gas, chain.estimateCalls.Load(), tc.wantGas, tc.wantLatest)
			}
		})
	}
}

func TestCallGasWithoutNextBlockEstimatorUsesLatestState(t *testing.T) {
	b := newMockBackend()
	// Embedding only the Backend interface hides the mock's next-block estimator.
	m := New(struct{ Backend }{b}, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	gas, err := m.callGas(managerCtx(t.Context(), m), Request{
		To: common.HexToAddress("0xabc"), Label: "fill",
	}, receiptTestHeader(b.head))
	if err != nil || gas != 55_000 || b.estimateCalls.Load() != 1 {
		t.Fatalf("gas = %d, %v after %d estimates; want 50000 plus the 10%% fallback headroom from one estimate",
			gas, err, b.estimateCalls.Load())
	}
}

func TestHorizonMaxFeePerGasAndHeadroom(t *testing.T) {
	chain := newHorizonChain(t)
	m := New(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(nil), logr.Discard())
	got, err := m.MaxFeePerGas(t.Context())
	if want := bumpFee(big.NewInt(1_822_032_470)); err != nil || got.Cmp(want) != 0 {
		t.Fatalf("MaxFeePerGas = %s, %v; want one bump over the six-block bound %s", got, err, want)
	}
	if err := m.ValidateFeeHeadroom(); err != nil {
		t.Fatalf("default horizon headroom: %v", err)
	}
	tooHigh := New(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(func(cfg *Config) {
		cfg.Horizon.CongestedTipCapGwei = 45
	}), logr.Discard())
	if err := tooHigh.ValidateFeeHeadroom(); err == nil {
		t.Fatal("a congested tip cap above the initial fee limit was accepted")
	}
}

func TestHorizonStallRebroadcastsThenBumps(t *testing.T) {
	chain := newHorizonChain(t)
	metrics := newTestMetrics(t)
	m := NewWithMetrics(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(nil), metrics, logr.Discard())
	startManagerForTest(t, m)
	result, accepted := m.SendAsync(t.Context(), Request{To: common.HexToAddress("0xabc"), Data: []byte{1}, Label: "fill"})
	if !accepted {
		t.Fatal("request was not accepted")
	}
	original := chain.waitForSends(t, 1)[0]

	for rebroadcast := 1; rebroadcast <= 2; rebroadcast++ {
		chain.mineSlots(0.001, 0.5, 0.5, 0.5)
		sent := chain.waitForSends(t, 1+rebroadcast)
		if sent[rebroadcast].Hash() != original.Hash() {
			t.Fatalf("stall %d sent %s, want an exact rebroadcast of %s", rebroadcast, sent[rebroadcast].Hash(), original.Hash())
		}
	}
	chain.mineSlots(0.001, 0.5, 0.5, 0.5)
	replacement := chain.waitForSends(t, 4)[3]
	if replacement.Hash() == original.Hash() || replacement.GasTipCap().Cmp(bumpFee(original.GasTipCap())) < 0 {
		t.Fatalf("third stall sent tip %s, want a bumped replacement", replacement.GasTipCap())
	}
	assertMetric(t, metrics.replacements.WithLabelValues("fill", replacementKindRebroadcast, replaceReasonStall), 2)
	assertMetric(t, metrics.replacements.WithLabelValues("fill", replacementKindReplacement, replaceReasonStall), 1)

	chain.include(replacement)
	if got := <-result; got.Outcome != OutcomeConfirmed || got.Hash != replacement.Hash() {
		t.Fatalf("result = %+v, want the replacement confirmed", got)
	}
}

// A read endpoint that never answers a stalled call's re-estimate must not hold the lifecycle, which
// also owns receipts, deadlines and shutdown: once the estimate's budget runs out, the call is
// rebroadcast and its receipt still resolves it. The latest-state fallback gets the same budget.
func TestHungStallEstimateDoesNotHoldTheLifecycle(t *testing.T) {
	for name, hung := range hungEstimates {
		t.Run(name, func(t *testing.T) {
			chain := newHorizonChain(t)
			chain.nextBlockErr = hung.nextBlockErr
			m := New(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(func(cfg *Config) {
				cfg.ReplacementInterval = 100 * time.Millisecond // a 50ms estimate budget
			}), logr.Discard())
			startManagerForTest(t, m)
			result, accepted := m.SendAsync(t.Context(), Request{To: common.HexToAddress("0xabc"), Data: []byte{1}, Label: "fill"})
			if !accepted {
				t.Fatal("request was not accepted")
			}
			original := chain.waitForSends(t, 1)[0]

			chain.set(func(c *horizonChain) { c.hangEstimates = true })
			chain.mineSlots(0.001, 0.5, 0.5, 0.5)
			chain.waitForEstimates(t, 2) // the stall re-estimate is in flight
			chain.include(original)
			select {
			case got := <-result:
				if got.Hash != original.Hash() || (got.Outcome != OutcomeConfirmed && got.Outcome != OutcomeNonceConsumed) {
					t.Fatalf("result = %+v, want the original receipt or confirmed nonce consumption", got)
				}
				if got.Outcome == OutcomeNonceConsumed && (got.Receipt != nil || got.Outcome.Included() || !errors.Is(got.Err, ErrNonceConsumed)) {
					t.Fatalf("proof-time inclusion fabricated execution: %+v", got)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("a hung stall re-estimate kept the lifecycle from reading a mined receipt")
			}
			if sent := chain.sentTransactions(); len(sent) > 2 || (len(sent) == 2 && sent[1].Hash() != original.Hash()) {
				t.Fatalf("sent %d transactions, want at most one exact rebroadcast before inclusion", len(sent))
			}
			if got := chain.estimateCalls.Load(); got != hung.latestEstimates {
				t.Fatalf("latest-state estimates = %d, want %d", got, hung.latestEstimates)
			}
		})
	}
}

// A stalled call whose re-estimate hangs yields to its absolute deadline. No stale calldata is
// rebroadcast after expiry, and the owned nonce is retained for fresh business work.
func TestHungStallEstimateYieldsToTheDeadline(t *testing.T) {
	for name, hung := range hungEstimates {
		t.Run(name, func(t *testing.T) {
			chain := newHorizonChain(t)
			chain.nextBlockErr = hung.nextBlockErr
			m := New(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(nil), logr.Discard())
			startManagerForTest(t, m)
			submissionDeadline := time.Now().Add(500 * time.Millisecond)
			result, accepted := m.SendAsync(t.Context(), Request{
				To: common.HexToAddress("0xabc"), Data: []byte{1}, Label: "fill", Deadline: submissionDeadline,
			})
			if !accepted {
				t.Fatal("request was not accepted")
			}
			chain.waitForSends(t, 1)

			chain.set(func(c *horizonChain) { c.hangEstimates = true })
			chain.mineSlots(0.001, 0.5, 0.5, 0.5)
			chain.waitForEstimates(t, 2) // the stall re-estimate is in flight
			select {
			case got := <-result:
				if got.Outcome != OutcomeAbandoned {
					t.Fatalf("outcome = %s, want abandoned", got.Outcome)
				}
			case <-time.After(time.Until(submissionDeadline) + time.Second):
				t.Fatal("a hung stall re-estimate delayed abandonment past the deadline")
			}
			sent := chain.sentTransactions()
			if len(sent) != 1 {
				t.Fatalf("sent %d transactions, want only the original call", len(sent))
			}
			if got := chain.estimateCalls.Load(); got != hung.latestEstimates {
				t.Fatalf("latest-state estimates = %d, want %d", got, hung.latestEstimates)
			}
		})
	}
}

func TestHorizonRepricesOnBlockEvidence(t *testing.T) {
	for name, tc := range map[string]struct {
		configure func(*Config)
		evidence  func(*horizonChain)
		reason    string
		check     func(t *testing.T, original, replacement *types.Transaction)
	}{
		"a run of full blocks raises the tip to the market": {
			evidence: func(c *horizonChain) { c.mineSlots(1, 1, 1) },
			reason:   replaceReasonCongestion,
			check: func(t *testing.T, _, replacement *types.Transaction) {
				t.Helper()
				if replacement.GasTipCap().Cmp(gweiToWei(1)) != 0 {
					t.Fatalf("congestion tip = %s, want the 1 gwei market reward", replacement.GasTipCap())
				}
			},
		},
		"a fee cap about to lapse is raised to the bound": {
			evidence: func(c *horizonChain) {
				c.set(func(c *horizonChain) { c.nextBaseFee = gweiToWei(1.7) })
				c.mine(0.001, 0.5)
			},
			reason: replaceReasonValidity,
			check: func(t *testing.T, _, replacement *types.Transaction) {
				t.Helper()
				want := horizonMaxFee(gweiToWei(1.7), gweiToWei(0.02), defaultHorizonMaxBlocks)
				if replacement.GasFeeCap().Cmp(want) != 0 {
					t.Fatalf("validity fee cap = %s, want %s", replacement.GasFeeCap(), want)
				}
			},
		},
		"a call that outgrew its limit is re-sent with a larger one": {
			evidence: func(c *horizonChain) {
				c.set(func(c *horizonChain) { c.nextBlockGas = 110_000 })
				c.mineSlots(0.001, 0.5, 0.5, 0.5)
			},
			reason: replaceReasonGas,
			check: func(t *testing.T, original, replacement *types.Transaction) {
				t.Helper()
				if original.Gas() != 105_000 || replacement.Gas() != 115_500 {
					t.Fatalf("gas limits = %d then %d, want 105000 then 115500", original.Gas(), replacement.Gas())
				}
			},
		},
		"unreadable fee windows fall back to the replacement interval": {
			configure: func(cfg *Config) { cfg.ReplacementInterval = 50 * time.Millisecond },
			evidence: func(c *horizonChain) {
				c.set(func(c *horizonChain) { c.feeErr = errors.New("fee history unavailable") })
			},
			reason: replaceReasonFallback,
			check: func(t *testing.T, original, replacement *types.Transaction) {
				t.Helper()
				if replacement.GasTipCap().Cmp(bumpFee(original.GasTipCap())) != 0 {
					t.Fatalf("fallback tip = %s, want a cached bump of %s", replacement.GasTipCap(), original.GasTipCap())
				}
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			chain := newHorizonChain(t)
			metrics := newTestMetrics(t)
			m := NewWithMetrics(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(tc.configure), metrics, logr.Discard())
			startManagerForTest(t, m)
			if _, accepted := m.SendAsync(t.Context(), Request{
				To: common.HexToAddress("0xabc"), Data: []byte{1}, Label: "fill",
			}); !accepted {
				t.Fatal("request was not accepted")
			}
			original := chain.waitForSends(t, 1)[0]
			time.Sleep(30 * time.Millisecond) // ticks without a new block must not replace
			if sent := chain.sentTransactions(); len(sent) != 1 {
				t.Fatalf("sent %d transactions before any block evidence", len(sent))
			}

			tc.evidence(chain)
			replacement := chain.waitForSends(t, 2)[1]
			if replacement.Nonce() != original.Nonce() || replacement.Hash() == original.Hash() {
				t.Fatal("repricing did not replace the pending nonce")
			}
			tc.check(t, original, replacement)
			assertMetric(t, metrics.replacements.WithLabelValues("fill", replacementKindReplacement, tc.reason), 1)
		})
	}
}

func TestHorizonAbandonsAtDeadlineWithoutTimerBumps(t *testing.T) {
	chain := newHorizonChain(t)
	metrics := newTestMetrics(t)
	m := NewWithMetrics(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(nil), metrics, logr.Discard())
	startManagerForTest(t, m)

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), Data: []byte{1}, Label: "fill", Deadline: time.Now().Add(60 * time.Millisecond),
	})
	if !accepted {
		t.Fatal("request was not accepted")
	}
	got := <-result
	if got.Outcome != OutcomeAbandoned {
		t.Fatalf("outcome = %s, want abandoned", got.Outcome)
	}
	sent := chain.sentTransactions()
	if len(sent) != 1 {
		t.Fatalf("sent %d transactions, want only the original fill", len(sent))
	}
	assertMetric(t, metrics.replacements.WithLabelValues("fill", replacementKindReplacement, "request_deadline"), 0)
}
