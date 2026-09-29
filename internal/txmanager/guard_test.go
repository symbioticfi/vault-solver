package txmanager

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/codes"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

// guardBackend is a mockBackend that can read the signer balance pinned to a block, so the balance
// guard runs. Its fee history reports block numbers and a next base fee around the latest header it
// served, and its latest headers are stamped with the wall clock.
type guardBackend struct {
	*mockBackend

	guardMu       sync.Mutex
	balance       *big.Int
	balanceErrs   []error // returned, in order, by successive balance reads
	balanceErr    error   // returned by every balance read once balanceErrs is exhausted
	balanceReads  []rpc.BlockNumberOrHash
	nextBase      *big.Int        // base fee the fee history reports for the block after its newest
	historyOffset int64           // fee history's newest block minus the latest header's number
	headAges      []time.Duration // ages of successive latest headers; the last one repeats
	servedHeads   map[common.Hash]uint64
	lastHead      uint64
}

func newGuardBackend(balance *big.Int) *guardBackend {
	b := &guardBackend{
		mockBackend: newMockBackend(),
		balance:     balance,
		nextBase:    big.NewInt(20e9),
		servedHeads: map[common.Hash]uint64{},
	}
	b.lastHead = b.head
	return b
}

func (b *guardBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	header, err := b.mockBackend.HeaderByNumber(ctx, number)
	if err != nil || number != nil {
		return header, err
	}
	b.guardMu.Lock()
	defer b.guardMu.Unlock()
	var age time.Duration
	if len(b.headAges) > 0 {
		age = b.headAges[0]
		if len(b.headAges) > 1 {
			b.headAges = b.headAges[1:]
		}
	}
	// A fresh header is stamped a second ahead: header times are whole seconds, and a truncated one
	// would look a fraction of a block old under the millisecond block times these tests use.
	stamp := time.Now().Add(time.Second)
	if age > 0 {
		stamp = time.Now().Add(-age)
	}
	header.Time = uint64(stamp.Unix())
	b.lastHead = header.Number.Uint64()
	b.servedHeads[header.Hash()] = b.lastHead
	return header, nil
}

func (b *guardBackend) FeeHistory(
	ctx context.Context, blockCount uint64, newest *big.Int, percentiles []float64,
) (*ethereum.FeeHistory, error) {
	history, err := b.mockBackend.FeeHistory(ctx, blockCount, newest, percentiles)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	baseFee := new(big.Int).Set(b.baseFee)
	b.mu.Unlock()
	b.guardMu.Lock()
	defer b.guardMu.Unlock()
	newestBlock := int64(b.lastHead) + b.historyOffset
	out := &ethereum.FeeHistory{
		OldestBlock: big.NewInt(newestBlock - int64(blockCount) + 1),
		Reward:      history.Reward,
	}
	for range blockCount {
		out.BaseFee = append(out.BaseFee, new(big.Int).Set(baseFee))
	}
	out.BaseFee = append(out.BaseFee, new(big.Int).Set(b.nextBase))
	return out, nil
}

func (b *guardBackend) ReadBalanceAtBlock(
	_ context.Context, _ common.Address, block rpc.BlockNumberOrHash,
) (*big.Int, error) {
	b.guardMu.Lock()
	defer b.guardMu.Unlock()
	b.balanceReads = append(b.balanceReads, block)
	if len(b.balanceErrs) > 0 {
		err := b.balanceErrs[0]
		b.balanceErrs = b.balanceErrs[1:]
		if err != nil {
			return nil, err
		}
	} else if b.balanceErr != nil {
		return nil, b.balanceErr
	}
	return new(big.Int).Set(b.balance), nil
}

func (b *guardBackend) setBalance(balance *big.Int) {
	b.guardMu.Lock()
	defer b.guardMu.Unlock()
	b.balance = balance
}

// pins returns the balance reads so far, and the block number each one resolves to.
func (b *guardBackend) pins() ([]rpc.BlockNumberOrHash, []uint64) {
	b.guardMu.Lock()
	defer b.guardMu.Unlock()
	numbers := make([]uint64, 0, len(b.balanceReads))
	for _, pin := range b.balanceReads {
		if hash, ok := pin.Hash(); ok {
			numbers = append(numbers, b.servedHeads[hash])
			continue
		}
		number, _ := pin.Number()
		numbers = append(numbers, uint64(number.Int64()))
	}
	return append([]rpc.BlockNumberOrHash(nil), b.balanceReads...), numbers
}

func newGuardManager(t *testing.T, b Backend, cfg Config, metrics *Metrics) *Manager {
	t.Helper()
	if cfg.MaxFeeGwei == 0 {
		cfg.MaxFeeGwei = 50
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = time.Millisecond
	}
	// With fast blocks the shadow evaluator's own header and fee-history reads would consume the scripted
	// heads these tests serve to the send path; it has its own tests (shadow_test.go).
	cfg.Shadow.Disabled = true
	m := NewWithMetrics(b, mustSigner(t), big.NewInt(11155111), cfg, metrics, logr.Discard())
	startManagerForTest(t, m)
	return m
}

func eth(wei int64) *big.Int { return big.NewInt(wei) }

// TestUnaffordableFillIsNeverBroadcast replays the 2026-09-28 failures: fills whose gas limit × fee cap
// exceeded the EOA balance, which the relay accepted and never landed.
func TestUnaffordableFillIsNeverBroadcast(t *testing.T) {
	for _, tc := range []struct {
		name       string
		balance    *big.Int
		gas        uint64
		latestBase *big.Int
		nextBase   *big.Int
		wantMaxFee *big.Int // nil when refused
		wantReason admissionRejectionReason
	}{
		{
			// rfq-first nonces 19/20: 0.00649 ETH, a 3.86M gas limit and a 0.886 gwei next base fee.
			// Legacy prices 2×latest + tip = 1.782 gwei; the guard signs the 1.681 gwei the balance funds.
			name: "rfq-first 09-28 is signed at the affordable cap", balance: eth(6_490_000_000_000_000), gas: 3_860_000,
			latestBase: big.NewInt(886_000_000), nextBase: big.NewInt(886_000_000), wantMaxFee: big.NewInt(1_681_347_150),
		},
		{
			name: "funded rfq-first lane is signed at the legacy price", balance: eth(100_000_000_000_000_000), gas: 3_860_000,
			latestBase: big.NewInt(886_000_000), nextBase: big.NewInt(886_000_000), wantMaxFee: big.NewInt(1_782_000_000),
		},
		{
			// UniswapX nonces 22-27: 0.0175 ETH, a 4.24M gas limit and a 5 gwei next base fee.
			name: "uniswapx 09-28 is refused", balance: eth(17_500_000_000_000_000), gas: 4_240_000,
			latestBase: big.NewInt(4_500_000_000), nextBase: big.NewInt(5_000_000_000), wantReason: admissionRejectionUnaffordable,
		},
		{
			name: "one block feasible is refused", balance: eth(22_472_000_000_000_000), gas: 4_240_000,
			latestBase: big.NewInt(4_500_000_000), nextBase: big.NewInt(5_000_000_000), wantReason: admissionRejectionUnaffordableOneBlock,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newGuardBackend(tc.balance)
			b.baseFee = tc.latestBase
			b.nextBase = tc.nextBase
			b.history = constantFeeHistory(big.NewInt(10_000_000))
			metrics := newTestMetrics(t)
			m := newGuardManager(t, b, Config{}, metrics)
			req := Request{To: common.HexToAddress("0xabc"), Data: []byte{0x01}, GasLimit: tc.gas, Label: "fill"}

			res := m.Send(t.Context(), req)
			if tc.wantMaxFee != nil {
				if res.Err != nil {
					t.Fatalf("send: %v", res.Err)
				}
				tx := b.lastSent()
				if tx.GasFeeCap().Cmp(tc.wantMaxFee) != 0 || tx.Nonce() != 7 {
					t.Fatalf("signed max fee %s nonce %d, want %s / 7", tx.GasFeeCap(), tx.Nonce(), tc.wantMaxFee)
				}
				if need := requiredBalance(tx.Gas(), tx.GasFeeCap(), tx.Value()); need.Cmp(tc.balance) > 0 {
					t.Fatalf("signed attempt needs %s, more than the balance %s", need, tc.balance)
				}
				assertMetric(t, metrics.attemptMaxFee.WithLabelValues("fill", attemptKindFill), float64(tc.wantMaxFee.Int64()))
				assertMetric(t, metrics.nextBaseFee.WithLabelValues(), float64(tc.nextBase.Int64()))
				return
			}
			if !errors.Is(res.Err, ErrUnaffordable) || !res.NotAdmitted || res.Outcome != OutcomeSubmissionError {
				t.Fatalf("result = %+v, want a not-admitted ErrUnaffordable submission error", res)
			}
			if attempted := b.attemptedTransactions(); len(attempted) != 0 {
				t.Fatalf("unaffordable fill broadcast %d transactions", len(attempted))
			}
			assertMetric(t, metrics.admissionRejections.WithLabelValues("fill", string(tc.wantReason)), 1)
			assertMetric(t, metrics.requests.WithLabelValues("fill", string(OutcomeSubmissionError)), 1)

			// The nonce was not consumed: once funded, the same request is signed at nonce 7.
			b.setBalance(eth(1_000_000_000_000_000_000))
			if funded := m.Send(t.Context(), req); funded.Err != nil {
				t.Fatalf("send after funding: %v", funded.Err)
			}
			if tx := b.lastSent(); tx.Nonce() != 7 {
				t.Fatalf("nonce after refusal = %d, want 7", tx.Nonce())
			}
		})
	}
}

func TestUnaffordableRefusalDoesNotConsumeNonce(t *testing.T) {
	b := newGuardBackend(eth(1_000_000))
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.Discard())
	req := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "refused"}

	if pending, err := m.broadcast(t.Context(), req); !errors.Is(err, ErrUnaffordable) || pending != nil {
		t.Fatalf("unaffordable broadcast = (%+v, %v), want ErrUnaffordable without lifecycle", pending, err)
	}
	b.setBalance(eth(1_000_000_000_000_000_000))
	pending, err := m.broadcast(t.Context(), req)
	if err != nil {
		t.Fatalf("funded broadcast: %v", err)
	}
	if pending.nonce != 7 || pending.sendSeen != b.head {
		t.Fatalf("funded broadcast nonce %d send head %d, want 7 / %d", pending.nonce, pending.sendSeen, b.head)
	}
}

// TestInclusionDelayCountsFromTheNewestBlockASendKnew pins that inclusion delay and the first-attempt outcome
// count from the newest block the send's fee snapshot knew of: a fee history one block ahead of the header
// means that block was already built, so landing in the block after it is the next block (delay 1), not 2.
func TestInclusionDelayCountsFromTheNewestBlockASendKnew(t *testing.T) {
	b := newGuardBackend(eth(1_000_000_000_000_000_000))
	b.historyOffset = 1
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.Discard())
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "fill"})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if pending.sendSeen != b.head+1 {
		t.Fatalf("send seen block %d, want the fee history's %d", pending.sendSeen, b.head+1)
	}

	for _, tc := range []struct {
		name        string
		included    uint64
		wantOutcome firstAttemptOutcome
		wantDelay   float64
	}{
		{name: "the next block", included: pending.sendSeen + 1, wantOutcome: firstAttemptFirst, wantDelay: 1},
		{name: "the third block", included: pending.sendSeen + 3, wantOutcome: firstAttemptFirst, wantDelay: 3},
		{name: "the fourth block", included: pending.sendSeen + 4, wantOutcome: firstAttemptLate, wantDelay: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metrics := newTestMetrics(t)
			m.metrics = metrics
			m.recordInclusion(pending, Result{
				Hash: pending.originalHash, Receipt: &types.Receipt{BlockNumber: new(big.Int).SetUint64(tc.included)},
			})
			assertMetric(t, metrics.firstAttempts.WithLabelValues("fill", string(tc.wantOutcome), simulationUnknown), 1)
			metricstest.RequireHistogram(t, metrics.inclusionDelay.WithLabelValues("fill"), 1, tc.wantDelay)
		})
	}
}

// guardedPending broadcasts one 100k-gas fill at a 10 gwei base fee: 2×10 + 1 = 21 gwei, tip 1 gwei.
func guardedPending(t *testing.T, b *guardBackend, m *Manager) *pendingTransaction {
	t.Helper()
	b.baseFee = big.NewInt(10e9)
	b.nextBase = big.NewInt(10e9)
	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 100_000, Label: "fill",
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if pending.fees.maxFee.Cmp(big.NewInt(21e9)) != 0 {
		t.Fatalf("initial max fee = %s, want 21 gwei", pending.fees.maxFee)
	}
	return pending
}

func TestReplacementNeverExceedsBalance(t *testing.T) {
	t.Run("balance below the required bump rebroadcasts the capped attempt", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.Discard())
		pending := guardedPending(t, b, m)
		b.baseFee = big.NewInt(15e9)
		// bump(21 gwei) = 23.625 gwei; the balance funds one wei less per gas.
		b.setBalance(new(big.Int).Mul(big.NewInt(23_624_999_999), big.NewInt(100_000)))

		if _, err := m.tryReplace(managerCtx(t.Context(), m), pending, false); err != nil {
			t.Fatalf("tryReplace: %v", err)
		}
		if len(pending.attempts) != 1 {
			t.Fatalf("attempts = %d, want the original only", len(pending.attempts))
		}
		sent := b.attemptedTransactions()
		if len(sent) != 2 || sent[1].Hash() != pending.originalHash {
			t.Fatalf("sends = %v, want the original then its exact rebroadcast", transactionHashes(sent))
		}
		if !pending.balanceCapLogged {
			t.Fatal("balance-capped replacement was not logged")
		}
	})
	t.Run("balance caps the replacement fee", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.Discard())
		pending := guardedPending(t, b, m)
		// Fresh legacy fees ask 2×15 + 1 = 31 gwei; the balance funds 25 gwei per gas.
		b.baseFee = big.NewInt(15e9)
		b.setBalance(new(big.Int).Mul(big.NewInt(25e9), big.NewInt(100_000)))

		if _, err := m.tryReplace(managerCtx(t.Context(), m), pending, false); err != nil {
			t.Fatalf("tryReplace: %v", err)
		}
		if len(pending.attempts) != 2 {
			t.Fatalf("attempts = %d, want a replacement", len(pending.attempts))
		}
		replacement := pending.attempts[1].tx
		if replacement.GasFeeCap().Cmp(big.NewInt(25e9)) != 0 || replacement.GasTipCap().Cmp(big.NewInt(1_125_000_000)) != 0 {
			t.Fatalf("replacement fees = %s/%s, want 25 gwei / 1.125 gwei", replacement.GasFeeCap(), replacement.GasTipCap())
		}
		_, pinned := b.pins()
		if len(pinned) != 2 || pinned[1] != b.head {
			t.Fatalf("balance pins = %v, want the send head then the current head %d", pinned, b.head)
		}
	})
	t.Run("base fee above the balance cap rebroadcasts the capped attempt", func(t *testing.T) {
		logs, log := newLogCapture(0)
		var mu sync.Mutex
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
		pending := guardedPending(t, b, m)
		// The balance funds 25 gwei per gas, above bump(21 gwei), but the latest base fee is 30 gwei.
		b.baseFee = big.NewInt(30e9)
		b.setBalance(new(big.Int).Mul(big.NewInt(25e9), big.NewInt(100_000)))

		if _, err := m.tryReplace(managerCtx(t.Context(), m), pending, false); err != nil {
			t.Fatalf("tryReplace: %v", err)
		}
		sent := b.attemptedTransactions()
		if len(pending.attempts) != 1 || len(sent) != 2 || sent[1].Hash() != pending.originalHash {
			t.Fatalf("attempts %d sends %v, want the original then its exact rebroadcast", len(pending.attempts), transactionHashes(sent))
		}
		mu.Lock()
		defer mu.Unlock()
		if _, info := countLogs(*logs, "replacement capped"); info != 1 || !pending.balanceCapLogged {
			t.Fatalf("replacement capped logged %d times, want once: %s", info, strings.Join(*logs, "\n"))
		}
		if errorLevel, _ := countLogs(*logs, "cannot replace pending transaction"); errorLevel != 0 {
			t.Fatalf("balance-capped replacement logged %d errors", errorLevel)
		}
	})
	t.Run("unreadable balance signs nothing new", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.Discard())
		pending := guardedPending(t, b, m)
		b.balanceErr = errors.New("read endpoint unavailable")

		if _, err := m.tryReplace(managerCtx(t.Context(), m), pending, false); err != nil {
			t.Fatalf("tryReplace: %v", err)
		}
		if len(pending.attempts) != 1 || len(b.attemptedTransactions()) != 2 {
			t.Fatalf("attempts %d sends %d, want no new signature and one exact rebroadcast",
				len(pending.attempts), len(b.attemptedTransactions()))
		}
	})
}

func TestCancellationCappedByBalance(t *testing.T) {
	t.Run("capped at the balance over 21000 gas", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.Discard())
		pending := guardedPending(t, b, m)
		// Fresh fees ask 2×20 + 1 = 41 gwei under the 50 gwei global cap; the balance funds 30 gwei at
		// 21000 gas, although not even the required bump at the fill's 100k gas.
		b.baseFee = big.NewInt(20e9)
		b.setBalance(new(big.Int).Mul(big.NewInt(30e9), big.NewInt(cancellationGasLimit)))

		if _, err := m.tryReplace(managerCtx(t.Context(), m), pending, true); err != nil {
			t.Fatalf("tryReplace: %v", err)
		}
		if len(pending.attempts) != 2 || !pending.attempts[1].cancellation {
			t.Fatalf("attempts = %+v, want a cancellation", pending.attempts)
		}
		cancel := pending.attempts[1].tx
		if cancel.GasFeeCap().Cmp(big.NewInt(30e9)) != 0 || cancel.Gas() != cancellationGasLimit {
			t.Fatalf("cancellation max fee %s gas %d, want 30 gwei / 21000", cancel.GasFeeCap(), cancel.Gas())
		}
	})
	t.Run("below the required bump signs no cancellation", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.Discard())
		pending := guardedPending(t, b, m)
		b.setBalance(new(big.Int).Mul(big.NewInt(23_624_999_999), big.NewInt(cancellationGasLimit)))

		cancelling, err := m.tryReplace(managerCtx(t.Context(), m), pending, true)
		if !cancelling || !errors.Is(err, errReplacementLimitReached) {
			t.Fatalf("tryReplace = %t, %v; want cancellation mode with the replacement limit", cancelling, err)
		}
		if len(pending.attempts) != 1 || len(b.attemptedTransactions()) != 1 {
			t.Fatalf("attempts %d sends %d, want no cancellation signed", len(pending.attempts), len(b.attemptedTransactions()))
		}
	})
}

// TestCancellationWithUnreadableBalance pins that a balance read failure never stops the deadline
// cancellation: the attempts already signed were funded when they were signed and their nonce is not
// mined, so a cancellation is capped at the balance they reserved instead of waiting for the read.
func TestCancellationWithUnreadableBalance(t *testing.T) {
	for _, tc := range []struct {
		name       string
		balanceErr error
	}{
		{name: "read error", balanceErr: errors.New("read endpoint unavailable")},
		{name: "block not found", balanceErr: errors.Join(ethereum.NotFound, errors.New("header not found"))},
	} {
		t.Run(tc.name+": first cancellation is signed within the reservation", func(t *testing.T) {
			logs, log := newLogCapture(0)
			var mu sync.Mutex
			b := newGuardBackend(eth(1_000_000_000_000_000_000))
			m := New(b, mustSigner(t), big.NewInt(11155111), Config{
				MaxFeeGwei: 50, PollInterval: time.Millisecond, ReplacementInterval: 40 * time.Millisecond,
			}, logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
			pending := guardedPending(t, b, m)
			b.guardMu.Lock()
			b.balanceErr = tc.balanceErr
			b.guardMu.Unlock()

			for range 2 {
				cancelling, err := m.tryReplace(managerCtx(t.Context(), m), pending, true)
				if !cancelling || err != nil {
					t.Fatalf("tryReplace = %t, %v; want a signed cancellation", cancelling, err)
				}
			}
			if len(pending.attempts) < 2 || !pending.attempts[1].cancellation {
				t.Fatalf("attempts = %+v, want a cancellation", pending.attempts)
			}
			fill := pending.attempts[0].tx
			for _, attempt := range pending.attempts[1:] {
				if cost := attempt.tx.Cost(); cost.Cmp(fill.Cost()) > 0 {
					t.Fatalf("cancellation reserves %s wei, more than the funded fill's %s", cost, fill.Cost())
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if errorLevel, _ := countLogs(*logs, "cannot replace pending transaction"); errorLevel != 0 {
				t.Fatalf("unreadable balance logged %d errors: %s", errorLevel, strings.Join(*logs, "\n"))
			}
			if _, info := countLogs(*logs, "cancellation capped at the balance its lifecycle reserved"); info != 1 {
				t.Fatalf("reservation cap logged %d times, want once per lifecycle: %s", info, strings.Join(*logs, "\n"))
			}
		})
	}
	t.Run("the reservation caps the cancellation, then its exact rebroadcast", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 1000}, logr.Discard())
		pending := guardedPending(t, b, m)
		// The fill reserved 100k × 21 gwei = 100 gwei at 21000 gas; fresh fees ask 2×60 + 1 = 121 gwei.
		b.baseFee = big.NewInt(60e9)
		b.guardMu.Lock()
		b.balanceErr = errors.New("read endpoint unavailable")
		b.guardMu.Unlock()

		if _, err := m.tryReplace(managerCtx(t.Context(), m), pending, true); err != nil {
			t.Fatalf("tryReplace: %v", err)
		}
		if len(pending.attempts) != 2 || !pending.attempts[1].cancellation {
			t.Fatalf("attempts = %+v, want a cancellation", pending.attempts)
		}
		if cancel := pending.attempts[1].tx; cancel.GasFeeCap().Cmp(big.NewInt(100e9)) != 0 {
			t.Fatalf("cancellation max fee = %s, want the 100 gwei reservation", cancel.GasFeeCap())
		}
		// bump(100 gwei) exceeds the reservation: the cancellation is rebroadcast unchanged.
		if _, err := m.tryReplace(managerCtx(t.Context(), m), pending, true); err != nil {
			t.Fatalf("second tryReplace: %v", err)
		}
		sent := b.attemptedTransactions()
		if len(pending.attempts) != 2 || len(sent) != 3 || sent[2].Hash() != pending.attempts[1].hash {
			t.Fatalf("attempts %d sends %v, want the cancellation rebroadcast unchanged", len(pending.attempts), transactionHashes(sent))
		}
	})
	t.Run("a fill replacement still signs nothing new", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 1000}, logr.Discard())
		pending := guardedPending(t, b, m)
		b.guardMu.Lock()
		b.balanceErr = errors.New("read endpoint unavailable")
		b.guardMu.Unlock()

		if _, err := m.tryReplace(managerCtx(t.Context(), m), pending, false); err != nil {
			t.Fatalf("tryReplace: %v", err)
		}
		if len(pending.attempts) != 1 || len(b.attemptedTransactions()) != 2 {
			t.Fatalf("attempts %d sends %d, want only the exact rebroadcast", len(pending.attempts), len(b.attemptedTransactions()))
		}
	})
}

func TestGuardDisabledWithoutBalanceCapability(t *testing.T) {
	startedLogs := func(t *testing.T, b Backend, cfg Config) (*Manager, []string) {
		t.Helper()
		logs, log := newLogCapture(0)
		var mu sync.Mutex
		m := New(b, mustSigner(t), big.NewInt(11155111), cfg, logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			m.Start(ctx)
		}()
		res := m.Send(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "fill"})
		cancel()
		<-done
		if res.Err != nil {
			t.Fatalf("send: %v", res.Err)
		}
		mu.Lock()
		defer mu.Unlock()
		return m, append([]string(nil), *logs...)
	}

	t.Run("backend without a pinned balance read", func(t *testing.T) {
		m, logs := startedLogs(t, newMockBackend(), Config{PollInterval: time.Millisecond})
		if m.guardEnabled() {
			t.Fatal("guard enabled without the capability")
		}
		if _, other := countLogs(logs, "balance guard disabled: the backend cannot read a balance pinned to a block; "+
			"attempts are signed without checking that the signer can fund them"); other != 1 {
			t.Fatalf("guard warning logged %d times, want once: %s", other, strings.Join(logs, "\n"))
		}
	})
	t.Run("configured off", func(t *testing.T) {
		b := newGuardBackend(eth(1)) // could not fund anything
		m, logs := startedLogs(t, b, Config{PollInterval: time.Millisecond, Balance: BalanceConfig{GuardDisabled: true}})
		if m.guardEnabled() {
			t.Fatal("guard enabled although configured off")
		}
		if reads, _ := b.pins(); len(reads) != 0 {
			t.Fatalf("guard off read the balance %d times", len(reads))
		}
		if strings.Contains(strings.Join(logs, "\n"), "balance guard disabled") {
			t.Fatal("an explicit guard: false was warned about")
		}
	})
	t.Run("capable backend", func(t *testing.T) {
		m, logs := startedLogs(t, newGuardBackend(eth(1_000_000_000_000_000_000)), Config{PollInterval: time.Millisecond})
		if !m.guardEnabled() || !strings.Contains(strings.Join(logs, "\n"), `"balanceGuard":true`) {
			t.Fatalf("guard enabled %t, started log: %s", m.guardEnabled(), strings.Join(logs, "\n"))
		}
	})
}

func TestStaleHeadWaitsForNewerHeadThenRefuses(t *testing.T) {
	fastBlocks := FeeConfig{BlockTime: 20 * time.Millisecond}
	t.Run("stays stale", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		b.headAges = []time.Duration{time.Hour}
		metrics := newTestMetrics(t)
		m := newGuardManager(t, b, Config{Fees: fastBlocks}, metrics)

		started := time.Now()
		res := m.Send(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "fill"})
		if !errors.Is(res.Err, ErrStaleHead) || !res.NotAdmitted {
			t.Fatalf("result = %+v, want a not-admitted ErrStaleHead", res)
		}
		if waited := time.Since(started); waited < 2*fastBlocks.BlockTime {
			t.Fatalf("refused after %s, before waiting two block times", waited)
		}
		if reads, _ := b.pins(); len(reads) != 0 || len(b.attemptedTransactions()) != 0 {
			t.Fatalf("stale head read the balance %d times and sent %d transactions", len(reads), len(b.attemptedTransactions()))
		}
		assertMetric(t, metrics.admissionRejections.WithLabelValues("fill", string(admissionRejectionStaleHead)), 1)
	})
	t.Run("a newer head arrives", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		b.headAges = []time.Duration{time.Hour, time.Hour, 0}
		m := newGuardManager(t, b, Config{Fees: fastBlocks}, nil)

		if res := m.Send(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "fill"}); res.Err != nil {
			t.Fatalf("send: %v", res.Err)
		}
	})
	t.Run("the wait is bounded by CancelAt", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		b.headAges = []time.Duration{time.Hour}
		m := newGuardManager(t, b, Config{}, nil) // 12 s blocks: a 24 s stale-head budget

		started := time.Now()
		res := m.Send(t.Context(), Request{
			To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "fill", CancelAt: time.Now().Add(50 * time.Millisecond),
		})
		if !errors.Is(res.Err, ErrStaleHead) || !res.NotAdmitted {
			t.Fatalf("result = %+v, want a not-admitted ErrStaleHead", res)
		}
		if waited := time.Since(started); waited > 5*time.Second {
			t.Fatalf("stale-head wait ignored CancelAt: %s", waited)
		}
	})
}

func TestFeeHistoryBehindHeaderChargesLag(t *testing.T) {
	// 21000 gas funded at 24 gwei against a 20 gwei next base fee: fee(2) = 22.52 gwei fits, while
	// fee(3) = 25.33 gwei, the floor once a history one block behind is charged, does not.
	balance := new(big.Int).Mul(big.NewInt(24e9), big.NewInt(cancellationGasLimit))
	for _, tc := range []struct {
		name       string
		offset     int64
		wantErr    error
		wantPinned func(head uint64) uint64
	}{
		{name: "same block", offset: 0, wantPinned: func(head uint64) uint64 { return head }},
		{name: "one block behind", offset: -1, wantErr: ErrUnaffordable, wantPinned: func(head uint64) uint64 { return head - 1 }},
		{name: "one block ahead", offset: 1, wantPinned: func(head uint64) uint64 { return head + 1 }},
		{name: "two blocks behind", offset: -2, wantErr: ErrStaleHead},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newGuardBackend(balance)
			b.historyOffset = tc.offset
			m := New(b, mustSigner(t), big.NewInt(11155111), Config{
				MaxFeeGwei: 50, PollInterval: time.Millisecond, Fees: FeeConfig{BlockTime: 20 * time.Millisecond},
			}, logr.Discard())
			pending, err := m.broadcast(managerCtx(t.Context(), m), Request{
				To: common.HexToAddress("0xabc"), GasLimit: cancellationGasLimit, Label: "fill",
			})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || pending != nil {
					t.Fatalf("broadcast = (%v, %v), want %v", pending, err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("broadcast: %v", err)
			}
			reads, pinned := b.pins()
			if tc.wantPinned == nil {
				if len(reads) != 0 {
					t.Fatalf("balance read %d times from an inconsistent snapshot", len(reads))
				}
				return
			}
			if len(reads) != 1 || pinned[0] != tc.wantPinned(b.head) {
				t.Fatalf("balance pins = %v, want block %d", pinned, tc.wantPinned(b.head))
			}
			if _, byNumber := reads[0].Number(); !byNumber {
				t.Fatalf("balance pin %s, want a block number (a locally computed header hash can miss)", reads[0].String())
			}
		})
	}
}

func TestBalanceNotFoundAtPinRefusesStaleHead(t *testing.T) {
	// ReplacementInterval bounds the fee-read budget: min(1 s, 40 ms / 2).
	cfg := Config{MaxFeeGwei: 50, PollInterval: time.Millisecond, ReplacementInterval: 40 * time.Millisecond}
	req := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "fill"}
	t.Run("never found", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		b.balanceErr = errors.Join(ethereum.NotFound, errors.New("header not found"))
		m := New(b, mustSigner(t), big.NewInt(11155111), cfg, logr.Discard())
		if pending, err := m.broadcast(managerCtx(t.Context(), m), req); !errors.Is(err, ErrStaleHead) || pending != nil {
			t.Fatalf("broadcast = (%v, %v), want ErrStaleHead", pending, err)
		}
		reads, _ := b.pins()
		if len(reads) < 2 {
			t.Fatalf("balance read %d times, want retries within the fee-read budget", len(reads))
		}
		for _, pin := range reads {
			if pin.String() != reads[0].String() {
				t.Fatalf("balance pins moved from %s to %s: a retry must not fall back to another block", reads[0].String(), pin.String())
			}
		}
		if len(b.attemptedTransactions()) != 0 {
			t.Fatal("transaction sent without a balance")
		}
	})
	t.Run("found on retry", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		b.balanceErrs = []error{ethereum.NotFound}
		m := New(b, mustSigner(t), big.NewInt(11155111), cfg, logr.Discard())
		if _, err := m.broadcast(managerCtx(t.Context(), m), req); err != nil {
			t.Fatalf("broadcast: %v", err)
		}
		if reads, _ := b.pins(); len(reads) != 2 || reads[0].String() != reads[1].String() {
			t.Fatalf("balance reads = %v, want the same pin twice", reads)
		}
	})
	t.Run("read error is not retried", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		b.balanceErr = errors.New("method not allowed")
		m := New(b, mustSigner(t), big.NewInt(11155111), cfg, logr.Discard())
		if _, err := m.broadcast(managerCtx(t.Context(), m), req); !errors.Is(err, ErrStaleHead) {
			t.Fatalf("broadcast error = %v, want ErrStaleHead", err)
		}
		if reads, _ := b.pins(); len(reads) != 1 {
			t.Fatalf("balance read %d times, want 1", len(reads))
		}
	})
}

func TestLaggingUpstreamAfterInclusion(t *testing.T) {
	cfg := Config{Fees: FeeConfig{BlockTime: 20 * time.Millisecond}}
	req := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "fill"}
	includeFirst := func(t *testing.T, b *guardBackend, m *Manager) {
		t.Helper()
		b.mu.Lock()
		b.head = 105
		b.mu.Unlock()
		if res := m.Send(t.Context(), req); res.Err != nil || res.Receipt.BlockNumber.Uint64() != 105 {
			t.Fatalf("first send = %+v, want inclusion at 105", res)
		}
	}
	t.Run("waits for the upstream to reach the inclusion block", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		m := newGuardManager(t, b, cfg, nil)
		includeFirst(t, b, m)
		b.mu.Lock()
		b.latestHeads = []uint64{103, 103, 104, 106}
		b.mu.Unlock()

		if res := m.Send(t.Context(), req); res.Err != nil {
			t.Fatalf("second send: %v", res.Err)
		}
		_, pinned := b.pins()
		if len(pinned) != 2 || pinned[1] != 106 {
			t.Fatalf("balance pins = %v, want the second read at 106", pinned)
		}
	})
	t.Run("refuses while the upstream lags", func(t *testing.T) {
		b := newGuardBackend(eth(1_000_000_000_000_000_000))
		m := newGuardManager(t, b, cfg, nil)
		includeFirst(t, b, m)
		b.mu.Lock()
		b.head = 103
		b.mu.Unlock()

		if res := m.Send(t.Context(), req); !errors.Is(res.Err, ErrStaleHead) || !res.NotAdmitted {
			t.Fatalf("result = %+v, want a not-admitted ErrStaleHead", res)
		}
		if _, pinned := b.pins(); len(pinned) != 1 {
			t.Fatalf("balance pins = %v, want no read below the inclusion block", pinned)
		}
	})
}

func TestGuardedLifecycleMetrics(t *testing.T) {
	b := newGuardBackend(eth(1_000_000_000_000_000_000))
	metrics := newTestMetrics(t)
	m := newGuardManager(t, b, Config{Balance: BalanceConfig{ReferenceGasUnits: 4_000_000}}, metrics)
	if res := m.Send(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 50_000, Label: "fill"}); res.Err != nil {
		t.Fatalf("send: %v", res.Err)
	}
	assertMetric(t, metrics.firstAttempts.WithLabelValues("fill", string(firstAttemptFirst), simulationUnknown), 1)
	assertHistogramObservedOnce(t, metrics.inclusionDelay.WithLabelValues("fill"))
	assertHistogramObservedOnce(t, metrics.attemptHorizon.WithLabelValues("fill"))
	assertMetric(t, metrics.attemptGasLimit.WithLabelValues("fill", attemptKindFill), 50_000)
	// min: 4M × fee(2, 0.02 gwei) at a 20 gwei next base fee = 4M × 22.52 gwei.
	assertMetric(t, metrics.requiredBalance.WithLabelValues(requiredBalanceMin), 4_000_000*22_520_000_000)
	quote := requiredBalance(4_000_000, horizonFee(big.NewInt(20e9), 5, big.NewInt(20_000_000)), new(big.Int))
	assertMetric(t, metrics.requiredBalance.WithLabelValues(requiredBalanceQuote), weiFloat(quote))
	if m.lastInclusion.Load() != b.head {
		t.Fatalf("last inclusion = %d, want %d", m.lastInclusion.Load(), b.head)
	}
}

func TestPendingAgeMetric(t *testing.T) {
	metrics := newTestMetrics(t)
	metrics.observePendingAge("fill", false, 3*time.Second)
	assertMetric(t, metrics.pendingAge.WithLabelValues("fill", attemptKindFill), 3)
	metrics.observePendingAge("fill", true, time.Second)
	assertMetric(t, metrics.pendingAge.WithLabelValues("fill", attemptKindCancellation), 1)
	if metrics.pendingAge.DeleteLabelValues("fill", attemptKindFill) {
		t.Fatal("fill age kept after the lifecycle started cancelling")
	}
	metrics.clearPendingAge("fill")
	if metrics.pendingAge.DeleteLabelValues("fill", attemptKindCancellation) {
		t.Fatal("cancellation age kept after the lifecycle ended")
	}
}

// lockedSink serializes a log sink shared by the manager's goroutines and the test.
type lockedSink struct {
	sink logr.LogSink
	mu   *sync.Mutex
}

func (s *lockedSink) Init(info logr.RuntimeInfo) { s.sink.Init(info) }
func (s *lockedSink) Enabled(level int) bool     { return s.sink.Enabled(level) }
func (s *lockedSink) Info(level int, msg string, kv ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sink.Info(level, msg, kv...)
}
func (s *lockedSink) Error(err error, msg string, kv ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sink.Error(err, msg, kv...)
}
func (s *lockedSink) WithValues(kv ...any) logr.LogSink {
	return &lockedSink{sink: s.sink.WithValues(kv...), mu: s.mu}
}
func (s *lockedSink) WithName(name string) logr.LogSink {
	return &lockedSink{sink: s.sink.WithName(name), mu: s.mu}
}

func TestGuardRefusalDeclinesSpansWithoutErrorStatus(t *testing.T) {
	rec := tracetest.Install(t)
	b := newGuardBackend(eth(1_000_000))
	m := newGuardManager(t, b, Config{}, nil)

	res := m.Send(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "refused", Solver: "rfq"})
	if !errors.Is(res.Err, ErrUnaffordable) {
		t.Fatalf("result = %+v, want ErrUnaffordable", res)
	}
	for _, name := range []string{"txmanager.send refused", "txmanager.broadcast"} {
		span := tracetest.Ended(t, rec, name)
		if span.Status().Code != codes.Unset {
			t.Fatalf("%s status = %v, want unset", name, span.Status())
		}
		events := span.Events()
		if len(events) != 1 || events[0].Name != "declined" {
			t.Fatalf("%s events = %v, want one declined event", name, events)
		}
		for _, attr := range events[0].Attributes {
			if attr.Key == "reason" && attr.Value.AsString() != string(admissionRejectionUnaffordable) {
				t.Fatalf("%s decline reason = %s", name, attr.Value.AsString())
			}
		}
	}
}

func TestGuardRefusalLogsAtInfo(t *testing.T) {
	logs, log := newLogCapture(0)
	var mu sync.Mutex
	b := newGuardBackend(eth(1_000_000))
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
	if _, err := m.broadcast(managerCtx(t.Context(), m), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "fill",
	}); !errors.Is(err, ErrUnaffordable) {
		t.Fatalf("broadcast error = %v, want ErrUnaffordable", err)
	}
	mu.Lock()
	defer mu.Unlock()
	errorLevel, info := countLogs(*logs, "transaction refused before signing")
	if errorLevel != 0 || info != 1 {
		t.Fatalf("refusal logged %d times at error and %d at info, want 0/1: %s", errorLevel, info, strings.Join(*logs, "\n"))
	}
	if !strings.Contains(strings.Join(*logs, "\n"), `"reason":"unaffordable"`) {
		t.Fatalf("refusal log lacks its reason: %s", strings.Join(*logs, "\n"))
	}
}

func TestSentLogCarriesFeeFields(t *testing.T) {
	logs, log := newLogCapture(0)
	var mu sync.Mutex
	b := newGuardBackend(eth(1_000_000_000_000_000_000))
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
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
	for _, field := range []string{
		`"gasLimit":52500`, `"estimateMode":"latest"`, `"tip":"1000000000"`, `"maxFee":"39506172839"`,
		`"requiredBalance":"2074074074047500"`, `"nextBaseFee":"20000000000"`, `"horizonBlocks":6`,
		`"balance":"1000000000000000000"`, `"balanceBound":false`, `"headLagBlocks":0`,
	} {
		if !strings.Contains(sent, field) {
			t.Fatalf("sent log lacks %s: %s", field, sent)
		}
	}
}

// TestEstimateFailureIsNotHiddenByGuardWaits pins that a failing gas estimate ends the stale-head wait
// and the pinned-balance retries: the send fails with the estimate's error, a real submission error,
// instead of waiting out the guard's budget and returning a NotAdmitted refusal a solver would retry.
func TestEstimateFailureIsNotHiddenByGuardWaits(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(b *guardBackend)
	}{
		{name: "stale head", setup: func(b *guardBackend) { b.headAges = []time.Duration{time.Hour} }},
		{name: "balance not found at the pin", setup: func(b *guardBackend) {
			b.balanceErr = errors.Join(ethereum.NotFound, errors.New("header not found"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newGuardBackend(eth(1_000_000_000_000_000_000))
			b.gasEstimate = 0 // the estimate fails
			tc.setup(b)
			// 10 s blocks: a 20 s stale-head budget, so the send must not wait it out.
			m := newGuardManager(t, b, Config{Fees: FeeConfig{BlockTime: 10 * time.Second}}, nil)

			started := time.Now()
			res := m.Send(t.Context(), Request{To: common.HexToAddress("0xabc"), Label: "fill"})
			if errors.Is(res.Err, ErrStaleHead) || res.NotAdmitted || res.Outcome != OutcomeSubmissionError ||
				res.Err == nil || !strings.Contains(res.Err.Error(), "estimate failed") {
				t.Fatalf("result = %+v, want the estimate failure as an admitted submission error", res)
			}
			if waited := time.Since(started); waited > 5*time.Second {
				t.Fatalf("estimate failure returned after %s, having waited out the guard", waited)
			}
			if len(b.attemptedTransactions()) != 0 {
				t.Fatal("transaction sent after a failed estimate")
			}
		})
	}
}

// TestBalanceGuardWithMandatoryTip covers the legacy path with a positive tipGwei: no fee history is
// read, so the next base fee is grow(latest, 1), the balance is pinned to the header's hash, and the
// refusal floor and the clamp keep tipGwei.
func TestBalanceGuardWithMandatoryTip(t *testing.T) {
	// Latest base fee 10 gwei: pb = 11.25 gwei and the floor is grow(pb, 1) + 2 gwei = 14.65625 gwei.
	// Legacy prices 2×10 + 2 = 22 gwei: the node suggests 1 gwei and tipGwei 2 is mandatory.
	const gas = 100_000
	perGas := func(wei int64) *big.Int { return new(big.Int).Mul(big.NewInt(wei), big.NewInt(gas)) }
	for _, tc := range []struct {
		name       string
		balance    *big.Int
		wantMaxFee int64
		wantReason admissionRejectionReason // empty when signed
	}{
		{name: "funded send keeps the legacy price", balance: eth(1_000_000_000_000_000_000), wantMaxFee: 22e9},
		{name: "balance-bound send is clamped with the mandatory tip", balance: perGas(16e9), wantMaxFee: 16e9},
		{name: "clamped at the floor", balance: perGas(14_656_250_000), wantMaxFee: 14_656_250_000},
		{
			name: "one wei below the floor is refused", balance: perGas(14_656_249_999),
			wantReason: admissionRejectionUnaffordableOneBlock,
		},
		{
			name: "below the next base fee plus tipGwei is unaffordable", balance: perGas(13_249_999_999),
			wantReason: admissionRejectionUnaffordable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newGuardBackend(tc.balance)
			b.baseFee = big.NewInt(10e9) // the fee history's 20 gwei next base fee must not be read
			m := New(b, mustSigner(t), big.NewInt(11155111), Config{
				MaxFeeGwei: 50, TipGwei: 2, PollInterval: time.Millisecond,
			}, logr.Discard())
			pending, err := m.broadcast(managerCtx(t.Context(), m), Request{
				To: common.HexToAddress("0xabc"), GasLimit: gas, Label: "fill",
			})
			reads, pinned := b.pins()
			if len(reads) != 1 || pinned[0] != b.head {
				t.Fatalf("balance pins = %v, want the header %d", pinned, b.head)
			}
			if _, byNumber := reads[0].Number(); !byNumber {
				t.Fatalf("balance pin %s, want the header's block number", reads[0].String())
			}
			if tc.wantReason != "" {
				if reason, refused := guardRefusalReason(err); !refused || reason != tc.wantReason || pending != nil {
					t.Fatalf("broadcast = (%v, %v), want a %s refusal", pending, err, tc.wantReason)
				}
				return
			}
			if err != nil {
				t.Fatalf("broadcast: %v", err)
			}
			tx := pending.attempts[0].tx
			if tx.GasFeeCap().Int64() != tc.wantMaxFee || tx.GasTipCap().Cmp(big.NewInt(2e9)) != 0 {
				t.Fatalf("signed fees %s/%s, want %d / the 2 gwei tipGwei", tx.GasFeeCap(), tx.GasTipCap(), tc.wantMaxFee)
			}
		})
	}
}

// TestPinnedBalanceReadErrorsEscalate pins that a read endpoint whose pinned balance reads keep failing with an
// error other than not found (here a proxy rejecting the block parameter) is logged at error level
// once per streak, whichever block the snapshot pins, while every send stays refused as stale_head: before, each refusal was
// one Info line and the lane stopped sending without paging.
func TestPinnedBalanceReadErrorsEscalate(t *testing.T) {
	cfg := Config{MaxFeeGwei: 50, PollInterval: time.Millisecond, ReplacementInterval: 40 * time.Millisecond}
	req := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "fill"}
	const escalation = "balance reads pinned to a block keep failing"
	for _, tc := range []struct {
		name       string
		offset     int64 // fee history newest block minus the header
		err        error
		wantErrors int
	}{
		{name: "rejected pins at the header escalate once per streak", err: errors.New("invalid argument 1: hex string without 0x prefix"), wantErrors: 1},
		{name: "rejected pins a block behind escalate once per streak", offset: -1, err: errors.New("missing trie node"), wantErrors: 1},
		{name: "not found is the not-found streak, not this one", err: errors.Join(ethereum.NotFound, errors.New("header not found"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs, log := newLogCapture(0)
			var mu sync.Mutex
			b := newGuardBackend(eth(1_000_000_000_000_000_000))
			b.historyOffset = tc.offset
			b.balanceErr = tc.err
			m := New(b, mustSigner(t), big.NewInt(11155111), cfg, logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
			for range pinnedReadErrorAfter + 2 {
				if _, err := m.broadcast(managerCtx(t.Context(), m), req); !errors.Is(err, ErrStaleHead) {
					t.Fatalf("broadcast error = %v, want ErrStaleHead", err)
				}
			}
			b.guardMu.Lock()
			b.balanceErr = nil
			b.guardMu.Unlock()
			if _, err := m.broadcast(managerCtx(t.Context(), m), req); err != nil {
				t.Fatalf("broadcast after the endpoint recovered: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if errorLevel, _ := countLogs(*logs, escalation); errorLevel != tc.wantErrors {
				t.Fatalf("escalation logged %d errors, want %d: %s", errorLevel, tc.wantErrors, strings.Join(*logs, "\n"))
			}
			if _, info := countLogs(*logs, "balance reads pinned to a block recovered"); info != tc.wantErrors {
				t.Fatalf("recovery logged %d times, want %d", info, tc.wantErrors)
			}
		})
	}
}

// TestPinnedBalanceNotFoundEscalates pins that pinned balance reads that keep finding no block, an upstream
// serving heads it cannot serve state for, page once per streak instead of refusing every send at Info only.
func TestPinnedBalanceNotFoundEscalates(t *testing.T) {
	cfg := Config{MaxFeeGwei: 50, PollInterval: time.Millisecond, ReplacementInterval: 40 * time.Millisecond}
	req := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "fill"}
	const escalation = "balance reads pinned to a block keep finding no block"
	for _, tc := range []struct {
		name       string
		offset     int64 // fee history newest block minus the header
		wantErrors int
	}{
		{name: "pins at the header escalate once per streak", wantErrors: 1},
		{name: "pins a block behind escalate once per streak", offset: -1, wantErrors: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs, log := newLogCapture(0)
			var mu sync.Mutex
			b := newGuardBackend(eth(1_000_000_000_000_000_000))
			b.historyOffset = tc.offset
			b.balanceErr = errors.Join(ethereum.NotFound, errors.New("header not found"))
			m := New(b, mustSigner(t), big.NewInt(11155111), cfg, logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
			for range pinNotFoundErrorAfter + 2 {
				if _, err := m.broadcast(managerCtx(t.Context(), m), req); !errors.Is(err, ErrStaleHead) {
					t.Fatalf("broadcast error = %v, want ErrStaleHead", err)
				}
			}
			b.guardMu.Lock()
			b.balanceErr = nil
			b.guardMu.Unlock()
			if _, err := m.broadcast(managerCtx(t.Context(), m), req); err != nil {
				t.Fatalf("broadcast after the node found the block: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if errorLevel, _ := countLogs(*logs, escalation); errorLevel != tc.wantErrors {
				t.Fatalf("escalation logged %d errors, want %d: %s", errorLevel, tc.wantErrors, strings.Join(*logs, "\n"))
			}
			if _, info := countLogs(*logs, "balance reads pinned to a block find their block again"); info != tc.wantErrors {
				t.Fatalf("recovery logged %d times, want %d", info, tc.wantErrors)
			}
		})
	}
}
