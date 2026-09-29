package txmanager

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

// horizonBackend is a mockBackend for the horizon policy: fresh latest headers with a block gas limit, a
// fee history of the requested length and percentiles around them, a pinned balance read, and next-block
// gas estimates with a capability probe.
type horizonBackend struct {
	*mockBackend

	hmu           sync.Mutex
	number        uint64
	headAges      []time.Duration // ages of successive latest headers; the last one repeats
	gasLimit      uint64
	latestBase    *big.Int
	nextBase      *big.Int
	historyOffset []int64             // fee history's newest block minus the header's, per read; the last repeats
	ratios        map[uint64]float64  // gas-used ratio by block; 0.5 by default
	baseFees      map[uint64]*big.Int // base fee by block; latestBase by default
	runRewards    map[uint64]*big.Int
	historyErr    error         // returned by every fee history read while set
	historyDelay  time.Duration // how long a fee history read takes after it read the chain
	historyHeads  []uint64      // newest block of every fee history served
	balance       *big.Int      // nil: no pinned balance capability is exercised (reads fail)
	balanceReads  []rpc.BlockNumberOrHash
	historyReads  []feeHistoryRequest

	nextBlockGas   uint64
	nextBlockErr   error
	nextBlockErrs  []error // returned by the first next-block estimates, in order, before nextBlockErr
	nextBlockCalls []nextBlockCall
	probeResult    bool
	probeErr       error
	probeCalls     atomic.Int64
	plainCalls     atomic.Int64
	blockEstimates chan struct{} // when set, estimates wait for it or their context
}

type nextBlockCall struct {
	parent    *big.Int
	overrides ethereum.BlockOverrides
}

func newHorizonBackend(balance *big.Int) *horizonBackend {
	b := &horizonBackend{
		mockBackend:  newMockBackend(),
		number:       100,
		gasLimit:     36_000_000,
		latestBase:   big.NewInt(1e9),
		nextBase:     big.NewInt(1e9),
		ratios:       map[uint64]float64{},
		baseFees:     map[uint64]*big.Int{},
		runRewards:   map[uint64]*big.Int{},
		balance:      balance,
		nextBlockGas: 100_000,
		probeResult:  true,
	}
	b.gasEstimate = 90_000
	return b
}

func (b *horizonBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if number != nil {
		return b.mockBackend.HeaderByNumber(ctx, number)
	}
	b.hmu.Lock()
	defer b.hmu.Unlock()
	var age time.Duration
	if len(b.headAges) > 0 {
		age = b.headAges[0]
		if len(b.headAges) > 1 {
			b.headAges = b.headAges[1:]
		}
	}
	stamp := time.Now().Add(time.Second)
	if age > 0 {
		stamp = time.Now().Add(-age)
	}
	return &types.Header{
		Number:   new(big.Int).SetUint64(b.number),
		Time:     uint64(stamp.Unix()),
		GasLimit: b.gasLimit,
		BaseFee:  new(big.Int).Set(b.latestBase),
	}, nil
}

func (b *horizonBackend) FeeHistory(
	ctx context.Context, blockCount uint64, newest *big.Int, percentiles []float64,
) (*ethereum.FeeHistory, error) {
	history, delay, err := b.feeHistory(blockCount, newest, percentiles)
	if err == nil && delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return history, err
}

func (b *horizonBackend) feeHistory(
	blockCount uint64, newest *big.Int, percentiles []float64,
) (*ethereum.FeeHistory, time.Duration, error) {
	b.hmu.Lock()
	defer b.hmu.Unlock()
	request := feeHistoryRequest{blocks: blockCount, percentiles: append([]float64(nil), percentiles...)}
	if newest != nil {
		request.newest = new(big.Int).Set(newest)
	}
	b.historyReads = append(b.historyReads, request)
	if b.historyErr != nil {
		return nil, 0, b.historyErr
	}
	var offset int64
	if len(b.historyOffset) > 0 {
		offset = b.historyOffset[0]
		if len(b.historyOffset) > 1 {
			b.historyOffset = b.historyOffset[1:]
		}
	}
	head := uint64(int64(b.number) + offset)
	b.historyHeads = append(b.historyHeads, head)
	history := &ethereum.FeeHistory{OldestBlock: new(big.Int).SetUint64(head - blockCount + 1)}
	for number := head - blockCount + 1; number <= head; number++ {
		ratio, ok := b.ratios[number]
		if !ok {
			ratio = 0.5
		}
		baseFee := b.latestBase
		if fee, found := b.baseFees[number]; found {
			baseFee = fee
		}
		history.BaseFee = append(history.BaseFee, new(big.Int).Set(baseFee))
		history.GasUsedRatio = append(history.GasUsedRatio, ratio)
		row := make([]*big.Int, len(percentiles))
		for i, percentile := range percentiles {
			row[i] = gwei(0.001)
			if reward, found := b.runRewards[number]; found && percentile != feeHistoryPercentile {
				row[i] = new(big.Int).Set(reward)
			}
		}
		history.Reward = append(history.Reward, row)
	}
	history.BaseFee = append(history.BaseFee, new(big.Int).Set(b.nextBase))
	return history, b.historyDelay, nil
}

func (b *horizonBackend) ReadBalanceAtBlock(
	_ context.Context, _ common.Address, block rpc.BlockNumberOrHash,
) (*big.Int, error) {
	b.hmu.Lock()
	defer b.hmu.Unlock()
	b.balanceReads = append(b.balanceReads, block)
	if b.balance == nil {
		return nil, errors.New("balance unavailable")
	}
	return new(big.Int).Set(b.balance), nil
}

func (b *horizonBackend) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	b.plainCalls.Add(1)
	if err := b.waitEstimate(ctx); err != nil {
		return 0, err
	}
	return b.mockBackend.EstimateGas(ctx, msg)
}

func (b *horizonBackend) EstimateGasWithBlockOverrides(
	ctx context.Context, _ ethereum.CallMsg, parent *big.Int, overrides ethereum.BlockOverrides,
) (uint64, error) {
	b.hmu.Lock()
	b.nextBlockCalls = append(b.nextBlockCalls, nextBlockCall{parent: new(big.Int).Set(parent), overrides: overrides})
	gas, err := b.nextBlockGas, b.nextBlockErr
	if len(b.nextBlockErrs) > 0 {
		err, b.nextBlockErrs = b.nextBlockErrs[0], b.nextBlockErrs[1:]
	}
	b.hmu.Unlock()
	if err := b.waitEstimate(ctx); err != nil {
		return 0, err
	}
	return gas, err
}

func (b *horizonBackend) waitEstimate(ctx context.Context) error {
	b.hmu.Lock()
	block := b.blockEstimates
	b.hmu.Unlock()
	if block == nil {
		return nil
	}
	select {
	case <-block:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *horizonBackend) ProbeBlockOverrides(context.Context, time.Duration) (bool, error) {
	b.probeCalls.Add(1)
	b.hmu.Lock()
	defer b.hmu.Unlock()
	return b.probeResult, b.probeErr
}

func (b *horizonBackend) set(update func(b *horizonBackend)) {
	b.hmu.Lock()
	defer b.hmu.Unlock()
	update(b)
}

func (b *horizonBackend) feeHistoryReads() []feeHistoryRequest {
	b.hmu.Lock()
	defer b.hmu.Unlock()
	return append([]feeHistoryRequest(nil), b.historyReads...)
}

func (b *horizonBackend) nextBlockEstimates() []nextBlockCall {
	b.hmu.Lock()
	defer b.hmu.Unlock()
	return append([]nextBlockCall(nil), b.nextBlockCalls...)
}

// horizonConfig is a horizon-policy config with a 50 gwei global cap and fast receipt polls.
func horizonConfig() Config {
	return Config{MaxFeeGwei: 50, PollInterval: time.Millisecond, Fees: FeeConfig{Policy: FeePolicyHorizon}}
}

// newHorizonManager builds a horizon-policy manager without starting it; broadcast then runs directly and
// next-block estimates stay unconfirmed (no probe runs) unless the test sets the verdict.
func newHorizonManager(t *testing.T, b Backend, cfg Config, metrics *Metrics) *Manager {
	t.Helper()
	return NewWithMetrics(b, mustSigner(t), big.NewInt(11155111), cfg, metrics, logr.Discard())
}

func fillRequest(gas uint64) Request {
	return Request{To: common.HexToAddress("0xabc"), Data: []byte{0x01}, GasLimit: gas, Label: "fill"}
}

func TestHorizonSendSignsTheTargetHorizon(t *testing.T) {
	const gas = 4_000_000
	for _, tc := range []struct {
		name       string
		setup      func(b *horizonBackend)
		wantTip    *big.Int
		wantMaxFee *big.Int
	}{
		{
			name: "blocks with room", wantTip: gwei(0.02),
			wantMaxFee: horizonFee(big.NewInt(1e9), 6, gwei(0.02)),
		},
		{
			name:    "an isolated full block",
			setup:   func(b *horizonBackend) { b.ratios[b.number] = 0.95 },
			wantTip: gwei(0.1), wantMaxFee: horizonFee(big.NewInt(1e9), 6, gwei(0.1)),
		},
		{
			name: "a demand run",
			setup: func(b *horizonBackend) {
				for back := range uint64(3) {
					b.ratios[b.number-back] = 0.99
					b.runRewards[b.number-back] = gwei(float64(back + 1))
				}
			},
			wantTip: gwei(3), wantMaxFee: horizonFee(big.NewInt(1e9), 6, gwei(3)),
		},
		{
			// The snapshot is one block behind the header: the next base fee is the header's own block's.
			name:    "a fee history one block behind",
			setup:   func(b *horizonBackend) { b.historyOffset = []int64{-1} },
			wantTip: gwei(0.02), wantMaxFee: horizonFee(big.NewInt(1e9), 6, gwei(0.02)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newHorizonBackend(big.NewInt(1e18))
			if tc.setup != nil {
				tc.setup(b)
			}
			metrics := newTestMetrics(t)
			m := newHorizonManager(t, b, horizonConfig(), metrics)
			pending, err := m.broadcast(managerCtx(t.Context(), m), fillRequest(gas))
			if err != nil {
				t.Fatalf("broadcast: %v", err)
			}
			tx := b.lastSent()
			if tx.GasTipCap().Cmp(tc.wantTip) != 0 || tx.GasFeeCap().Cmp(tc.wantMaxFee) != 0 {
				t.Fatalf("signed tip %s max fee %s, want tip %s max fee %s", tx.GasTipCap(), tx.GasFeeCap(), tc.wantTip, tc.wantMaxFee)
			}
			if pending.fees.baseFee.Cmp(big.NewInt(1e9)) != 0 {
				t.Fatalf("pending base fee = %s, want the next base fee", pending.fees.baseFee)
			}
			reads := b.feeHistoryReads()
			if len(reads) != 1 || reads[0].blocks != feeSnapshotBlocks || reads[0].newest != nil ||
				len(reads[0].percentiles) != 2 || reads[0].percentiles[0] != 25 || reads[0].percentiles[1] != 50 {
				t.Fatalf("fee history reads = %+v, want one FeeHistory(6, latest, [25, 50])", reads)
			}
		})
	}
}

func TestHorizonSendIsGuardedByTheBalance(t *testing.T) {
	const gas = 4_000_000
	perGas := func(fee *big.Int) *big.Int { return new(big.Int).Mul(fee, big.NewInt(gas)) }
	floor := horizonFee(big.NewInt(1e9), 2, gwei(0.02))
	t.Run("the balance clamps the fee cap", func(t *testing.T) {
		b := newHorizonBackend(perGas(gwei(1.3)))
		metrics := newTestMetrics(t)
		m := newHorizonManager(t, b, horizonConfig(), metrics)
		if _, err := m.broadcast(managerCtx(t.Context(), m), fillRequest(gas)); err != nil {
			t.Fatalf("broadcast: %v", err)
		}
		if tx := b.lastSent(); tx.GasFeeCap().Cmp(gwei(1.3)) != 0 {
			t.Fatalf("signed max fee %s, want the affordable 1.3 gwei", tx.GasFeeCap())
		}
		if pins := b.balanceReads; len(pins) != 1 {
			t.Fatalf("balance read %d times, want one pinned read", len(pins))
		} else if _, byHash := pins[0].Hash(); !byHash || !pins[0].RequireCanonical {
			t.Fatalf("balance pin = %s, want the header hash with requireCanonical", pins[0].String())
		}
	})
	t.Run("below the floor nothing is signed", func(t *testing.T) {
		rec := tracetest.Install(t)
		balance := new(big.Int).Sub(perGas(floor), big.NewInt(1))
		b := newHorizonBackend(balance)
		metrics := newTestMetrics(t)
		m := newHorizonManager(t, b, horizonConfig(), metrics)
		startManagerForTest(t, m)
		res := m.Send(t.Context(), fillRequest(gas))
		if !errors.Is(res.Err, ErrUnaffordable) || !res.NotAdmitted {
			t.Fatalf("result = %+v, want a not-admitted ErrUnaffordable", res)
		}
		if sent := b.attemptedTransactions(); len(sent) != 0 {
			t.Fatalf("refused fill sent %d transactions", len(sent))
		}
		assertMetric(t, metrics.admissionRejections.WithLabelValues("fill", string(admissionRejectionUnaffordableOneBlock)), 1)
		// The refusal is what these attributes explain: the base fee and what the balance funded.
		broadcast := tracetest.Ended(t, rec, "txmanager.broadcast")
		tracetest.RequireAttr(t, broadcast, "fee.next_base", "1000000000")
		tracetest.RequireAttr(t, broadcast, "balance.affordable", new(big.Int).Div(balance, big.NewInt(gas)).String())
		tracetest.RequireAttr(t, broadcast, "gas.estimate_mode", estimateModeSupplied)
		tracetest.RequireNoAttr(t, broadcast, "fee.horizon")
	})
	t.Run("a request cap below the floor is a submission error", func(t *testing.T) {
		b := newHorizonBackend(big.NewInt(1e18))
		m := newHorizonManager(t, b, horizonConfig(), nil)
		startManagerForTest(t, m)
		req := fillRequest(gas)
		req.MaxFeePerGas = bumpFee(new(big.Int).Sub(floor, big.NewInt(1000)))
		res := m.Send(t.Context(), req)
		if !errors.Is(res.Err, errFeeLimitReached) || res.NotAdmitted {
			t.Fatalf("result = %+v, want an admitted fee-limit submission error", res)
		}
		if sent := b.attemptedTransactions(); len(sent) != 0 {
			t.Fatalf("capped fill sent %d transactions", len(sent))
		}
	})
	t.Run("without the balance capability the target is signed", func(t *testing.T) {
		b := newHorizonBackend(big.NewInt(1))
		cfg := horizonConfig()
		cfg.Balance.GuardDisabled = true
		m := newHorizonManager(t, b, cfg, nil)
		if _, err := m.broadcast(managerCtx(t.Context(), m), fillRequest(gas)); err != nil {
			t.Fatalf("broadcast: %v", err)
		}
		if tx := b.lastSent(); tx.GasFeeCap().Cmp(horizonFee(big.NewInt(1e9), 6, gwei(0.02))) != 0 {
			t.Fatalf("signed max fee %s, want fee(6)", tx.GasFeeCap())
		}
		if len(b.balanceReads) != 0 {
			t.Fatalf("guard off read the balance %d times", len(b.balanceReads))
		}
	})
}

func TestHorizonSendWaitsOutStaleSnapshots(t *testing.T) {
	fast := horizonConfig()
	fast.Fees.BlockTime = 20 * time.Millisecond
	for _, tc := range []struct {
		name    string
		setup   func(b *horizonBackend)
		wantErr error
	}{
		{name: "a stale header", setup: func(b *horizonBackend) { b.headAges = []time.Duration{time.Hour} }, wantErr: ErrStaleHead},
		{name: "a newer header arrives", setup: func(b *horizonBackend) { b.headAges = []time.Duration{time.Hour, time.Hour, 0} }},
		{name: "fee history two blocks away", setup: func(b *horizonBackend) { b.historyOffset = []int64{-2} }, wantErr: ErrStaleHead},
		{
			// The snapshot read retries a disagreement once at once, then the send waits for agreement.
			name:  "fee history agrees after a retry",
			setup: func(b *horizonBackend) { b.historyOffset = []int64{-2, -2, -2, 0} },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newHorizonBackend(big.NewInt(1e18))
			tc.setup(b)
			m := newHorizonManager(t, b, fast, nil)
			_, err := m.broadcast(managerCtx(t.Context(), m), fillRequest(4_000_000))
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("broadcast: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("broadcast error = %v, want %v", err, tc.wantErr)
			}
			if len(b.attemptedTransactions()) != 0 || len(b.balanceReads) != 0 {
				t.Fatalf("stale snapshot sent %d transactions and read the balance %d times",
					len(b.attemptedTransactions()), len(b.balanceReads))
			}
		})
	}
}

// TestHorizonSendReadsAFreshSnapshot pins strategy §2.5 step 1: a send reads its own header rather than the
// quotes' cached snapshot, which can be a block old while still inside the poll interval, and estimates on
// top of that header.
func TestHorizonSendReadsAFreshSnapshot(t *testing.T) {
	b := newHorizonBackend(big.NewInt(1e18))
	b.headAges = []time.Duration{12 * time.Second, 0} // the cached header is one block time old
	cfg := horizonConfig()
	cfg.PollInterval = time.Hour // the cached snapshot is still inside the quotes' TTL
	m := newHorizonManager(t, b, cfg, nil)
	m.overrides.swap(blockOverridesSupported)
	if _, err := m.MaxFeePerGas(t.Context()); err != nil {
		t.Fatalf("MaxFeePerGas: %v", err)
	}
	b.set(func(b *horizonBackend) { b.number++ })
	if _, err := m.broadcast(managerCtx(t.Context(), m), fillRequest(0)); err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if reads := b.feeHistoryReads(); len(reads) != 2 {
		t.Fatalf("fee history read %d times, want the quote's and the send's own", len(reads))
	}
	if calls := b.nextBlockEstimates(); len(calls) != 1 || calls[0].parent.Uint64() != 101 {
		t.Fatalf("next-block estimates = %+v, want one on top of the fresh header 101", calls)
	}
}

// TestHorizonEstimateOverlapsTheStaleHeadWait pins the legacy path's behaviour under horizon: the estimate
// starts on the first snapshot's header, so a failing fill ends a stale-head wait with its own error instead
// of a stale_head refusal a solver would retry, and a newer header re-runs it on that header.
func TestHorizonEstimateOverlapsTheStaleHeadWait(t *testing.T) {
	cfg := horizonConfig()
	cfg.Fees.BlockTime = time.Second // a two-second stale-head wait
	t.Run("a revert ends the wait", func(t *testing.T) {
		b := newHorizonBackend(big.NewInt(1e18))
		b.headAges = []time.Duration{time.Hour}
		b.nextBlockErr = rpcCodeError{code: 3, message: "execution reverted"}
		m := newHorizonManager(t, b, cfg, nil)
		m.overrides.swap(blockOverridesSupported)
		started := time.Now()
		_, err := m.broadcast(managerCtx(t.Context(), m), fillRequest(0))
		if err == nil || errors.Is(err, ErrStaleHead) || !strings.Contains(err.Error(), "execution reverted") {
			t.Fatalf("broadcast error = %v, want the revert rather than a stale head", err)
		}
		if elapsed := time.Since(started); elapsed >= 2*cfg.Fees.BlockTime {
			t.Fatalf("revert reported after %s, want before the stale-head wait ends", elapsed)
		}
		if len(b.attemptedTransactions()) != 0 || len(b.balanceReads) != 0 {
			t.Fatalf("reverting fill sent %d transactions and read the balance %d times",
				len(b.attemptedTransactions()), len(b.balanceReads))
		}
	})
	t.Run("a newer header re-runs the estimate on it", func(t *testing.T) {
		b := newHorizonBackend(big.NewInt(1e18))
		b.headAges = []time.Duration{time.Hour, 0}
		m := newHorizonManager(t, b, cfg, nil)
		m.overrides.swap(blockOverridesSupported)
		pending, err := m.broadcast(managerCtx(t.Context(), m), fillRequest(0))
		if err != nil {
			t.Fatalf("broadcast: %v", err)
		}
		calls := b.nextBlockEstimates()
		if len(calls) != 2 {
			t.Fatalf("next-block estimates = %d, want one on the stale header and one on the fresh one", len(calls))
		}
		if fresh := uint64(time.Now().Unix()); calls[0].overrides.Time >= fresh || calls[1].overrides.Time < fresh {
			t.Fatalf("estimates at times %d and %d, want the stale header's then the fresh one's", calls[0].overrides.Time,
				calls[1].overrides.Time)
		}
		if pending.gas != 105_000 {
			t.Fatalf("gas limit = %d, want the fresh estimate with headroom", pending.gas)
		}
	})
}

func TestHorizonMaxFeePerGas(t *testing.T) {
	t.Run("priced at the pricing horizon from the cached snapshot", func(t *testing.T) {
		b := newHorizonBackend(big.NewInt(1e18))
		cfg := horizonConfig()
		cfg.PollInterval = time.Hour // the snapshot's TTL
		m := newHorizonManager(t, b, cfg, nil)
		want := bumpFee(horizonFee(big.NewInt(1e9), 5, gwei(0.02))) // about 1.80·pb
		for range 5 {
			fee, err := m.MaxFeePerGas(t.Context())
			if err != nil {
				t.Fatalf("MaxFeePerGas: %v", err)
			}
			if fee.Cmp(want) != 0 {
				t.Fatalf("MaxFeePerGas = %s, want bump(fee(5, 0.02 gwei)) = %s", fee, want)
			}
		}
		if reads := b.feeHistoryReads(); len(reads) != 1 {
			t.Fatalf("five quotes read the fee history %d times, want one cached snapshot", len(reads))
		}
	})
	t.Run("the tip is sized for the reference fill", func(t *testing.T) {
		b := newHorizonBackend(big.NewInt(1e18))
		b.ratios[b.number] = 0.9 // 3.6M gas of room: enough for a 21000-gas call, not for 4.4M
		cfg := horizonConfig()
		cfg.Balance.ReferenceGasUnits = 4_400_000
		m := newHorizonManager(t, b, cfg, nil)
		fee, err := m.MaxFeePerGas(t.Context())
		if err != nil {
			t.Fatalf("MaxFeePerGas: %v", err)
		}
		if want := bumpFee(horizonFee(big.NewInt(1e9), 5, gwei(0.1))); fee.Cmp(want) != 0 {
			t.Fatalf("MaxFeePerGas = %s, want the single-full-block tip price %s", fee, want)
		}
	})
	t.Run("capped at the normal fee limit", func(t *testing.T) {
		b := newHorizonBackend(big.NewInt(1e18))
		b.nextBase = gwei(30)
		m := newHorizonManager(t, b, horizonConfig(), nil)
		fee, err := m.MaxFeePerGas(t.Context())
		if err != nil {
			t.Fatalf("MaxFeePerGas: %v", err)
		}
		if want := m.normalFeeLimit(Request{}); fee.Cmp(want) != 0 {
			t.Fatalf("MaxFeePerGas = %s, want the normal fee limit %s", fee, want)
		}
	})
	t.Run("a base fee above the limit fails the quote", func(t *testing.T) {
		b := newHorizonBackend(big.NewInt(1e18))
		b.nextBase = gwei(40)
		m := newHorizonManager(t, b, horizonConfig(), nil)
		if _, err := m.MaxFeePerGas(t.Context()); !errors.Is(err, errFeeLimitReached) {
			t.Fatalf("MaxFeePerGas error = %v, want errFeeLimitReached", err)
		}
	})
	t.Run("a stale snapshot is read again, then fails the quote", func(t *testing.T) {
		b := newHorizonBackend(big.NewInt(1e18))
		b.headAges = []time.Duration{time.Hour}
		cfg := horizonConfig()
		cfg.PollInterval = time.Hour
		m := newHorizonManager(t, b, cfg, nil)
		if _, err := m.MaxFeePerGas(t.Context()); !errors.Is(err, errFreshFeesUnavailable) {
			t.Fatalf("MaxFeePerGas error = %v, want errFreshFeesUnavailable", err)
		}
		if reads := b.feeHistoryReads(); len(reads) != 2 {
			t.Fatalf("stale quote read the fee history %d times, want a cached read and one refresh", len(reads))
		}
	})
	t.Run("legacy keeps its own price", func(t *testing.T) {
		m := New(newMockBackend(), mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
		fee, err := m.MaxFeePerGas(t.Context())
		if err != nil {
			t.Fatalf("MaxFeePerGas: %v", err)
		}
		if fee.String() != "46125000000" {
			t.Fatalf("legacy max fee = %s, want 46125000000", fee)
		}
	})
}

// TestHorizonQuoteFillsAfterThreeBlocksOfGrowth drives the quote-to-fill invariant through the manager: the
// solver passes the quoted price back as Request.MaxFeePerGas, and the fill is signed after three blocks of
// maximum base-fee growth but not after four.
func TestHorizonQuoteFillsAfterThreeBlocksOfGrowth(t *testing.T) {
	const gas = 4_400_000
	for drift := range uint64(5) {
		b := newHorizonBackend(big.NewInt(1e18))
		cfg := horizonConfig()
		cfg.PollInterval = time.Hour // the fill reads its own snapshot, not the quote's cached one
		cfg.Balance.ReferenceGasUnits = gas
		m := newHorizonManager(t, b, cfg, nil)
		quoted, err := m.MaxFeePerGas(t.Context())
		if err != nil {
			t.Fatalf("MaxFeePerGas: %v", err)
		}
		b.set(func(b *horizonBackend) {
			b.number += drift
			b.nextBase = grow(big.NewInt(1e9), drift)
		})
		req := fillRequest(gas)
		req.MaxFeePerGas = quoted
		_, err = m.broadcast(managerCtx(t.Context(), m), req)
		switch {
		case drift <= 3 && err != nil:
			t.Fatalf("fill after %d blocks of growth: %v", drift, err)
		case drift == 4 && !errors.Is(err, errFeeLimitReached):
			t.Fatalf("fill after 4 blocks of growth = %v, want errFeeLimitReached", err)
		}
	}
}

func TestHorizonSentLogAndMetrics(t *testing.T) {
	logs, log := newLogCapture(0)
	var mu sync.Mutex
	b := newHorizonBackend(big.NewInt(1e18))
	metrics := newTestMetrics(t)
	m := NewWithMetrics(b, mustSigner(t), big.NewInt(11155111), horizonConfig(), metrics,
		logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
	if _, err := m.broadcast(managerCtx(t.Context(), m), Request{To: common.HexToAddress("0xabc"), Label: "fill"}); err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	var sent string
	for _, entry := range *logs {
		if strings.Contains(entry, `"msg":"sent"`) {
			sent = entry
		}
	}
	// No probe ran, so the estimate is unconfirmed: at latest with 1000 bps, 90000 + 9000.
	for _, field := range []string{
		`"gasLimit":99000`, `"estimateMode":"unconfirmed"`, `"tip":"20000000"`, `"maxFee":"1822032470"`,
		`"nextBaseFee":"1000000000"`, `"horizonBlocks":6`, `"balance":"1000000000000000000"`,
		`"balanceBound":false`, `"headLagBlocks":0`,
	} {
		if !strings.Contains(sent, field) {
			t.Fatalf("sent log lacks %s: %s", field, sent)
		}
	}
	assertMetric(t, metrics.gasEstimates.WithLabelValues("fill", estimateModeUnconfirmed, gasEstimateOK), 1)
	assertMetric(t, metrics.gasEstimates.WithLabelValues("fill", estimateModeFallback, gasEstimateOK), 0)
}

func TestValidateFeeHeadroomRejectsTipGweiUnderHorizon(t *testing.T) {
	m := &Manager{cfg: Config{MaxFeeGwei: 50, TipGwei: 1, Fees: FeeConfig{Policy: FeePolicyHorizon}}}
	if err := m.ValidateFeeHeadroom(); err == nil || !strings.Contains(err.Error(), "tipGwei") {
		t.Fatalf("ValidateFeeHeadroom = %v, want a tipGwei error under horizon", err)
	}
	m.cfg.Fees.Policy = FeePolicyLegacy
	if err := m.ValidateFeeHeadroom(); err != nil {
		t.Fatalf("ValidateFeeHeadroom under legacy = %v", err)
	}
}
