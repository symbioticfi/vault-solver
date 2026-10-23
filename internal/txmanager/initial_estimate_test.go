package txmanager

import (
	"context"
	"math/big"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/signer"
)

// estimateCountingSigner proves a failed unsigned estimate never reaches signing.
type estimateCountingSigner struct {
	signer.Signer

	calls atomic.Int64
}

func (s *estimateCountingSigner) SignTx(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	s.calls.Add(1)
	return s.Signer.SignTx(ctx, tx, chainID)
}

func TestInitialGasEstimateTimeout(t *testing.T) {
	for name, hung := range hungEstimates {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				chain := newHorizonChain(t)
				chain.nextBlockErr, chain.hangEstimates = hung.nextBlockErr, true
				s := &estimateCountingSigner{Signer: mustSigner(t)}
				m := New(chain, s, big.NewInt(11155111), horizonConfig(func(cfg *Config) {
					cfg.ReplacementInterval = 100 * time.Millisecond
				}), logr.Discard())
				startManagerForTest(t, m)
				result, accepted := m.SendAsync(t.Context(), Request{To: common.Address{1}, Label: "initial_estimate"})
				if !accepted {
					t.Fatal("request was not admitted")
				}
				select {
				case got := <-result:
					if got.Outcome != OutcomeSubmissionError || !errors.Is(got.Err, context.DeadlineExceeded) || got.Hash != (common.Hash{}) {
						t.Fatalf("unsigned timeout result = %+v", got)
					}
				case <-time.After(time.Second):
					t.Fatal("initial estimate exceeded its production budget")
				}
				if s.calls.Load() != 0 || len(chain.sentTransactions()) != 0 {
					t.Fatalf("timed-out estimate signed %d and broadcast %d transactions", s.calls.Load(), len(chain.sentTransactions()))
				}
				wantLatest := int64(0)
				if hung.nextBlockErr != nil {
					wantLatest = 1
				}
				if chain.estimateCalls.Load() != wantLatest {
					t.Fatalf("latest-state estimates = %d, want %d", chain.estimateCalls.Load(), wantLatest)
				}
			})
		})
	}
}

// budgetEstimateChain records both RPC deadlines while the override consumes part
// of the budget. Its fallback waits for the remaining budget rather than replying.
type budgetEstimateChain struct {
	*horizonChain

	overrideDelay    time.Duration
	overrideDeadline time.Time
	fallbackDeadline time.Time
}

func (c *budgetEstimateChain) EstimateGasNextBlock(ctx context.Context, _ ethereum.CallMsg, _ *types.Header, _ time.Duration) (uint64, error) {
	c.overrideDeadline, _ = ctx.Deadline()
	time.Sleep(c.overrideDelay)
	return 0, testRPCError{code: -32602, message: "too many arguments"}
}

func (c *budgetEstimateChain) EstimateGas(ctx context.Context, _ ethereum.CallMsg) (uint64, error) {
	c.fallbackDeadline, _ = ctx.Deadline()
	<-ctx.Done()
	return 0, ctx.Err()
}

func TestInitialGasEstimateSharedBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		chain := &budgetEstimateChain{horizonChain: newHorizonChain(t), overrideDelay: 40 * time.Millisecond}
		m := New(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(func(cfg *Config) {
			cfg.ReplacementInterval = 200 * time.Millisecond
		}), logr.Discard())
		// This safety timeout is deliberately longer than the manager's 100ms budget.
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		started := time.Now()
		_, err := m.callGas(managerCtx(ctx, m), Request{To: common.Address{1}, Label: "shared_budget"}, receiptTestHeader(100))
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 100*time.Millisecond {
			t.Fatalf("initial estimate = %v after %s, want one 100ms budget", err, time.Since(started))
		}
		if chain.overrideDeadline.IsZero() || !chain.overrideDeadline.Equal(chain.fallbackDeadline) {
			t.Fatalf("RPC deadlines differ: override %s, fallback %s", chain.overrideDeadline, chain.fallbackDeadline)
		}
	})
}

func TestInitialGasEstimateRequestDeadline(t *testing.T) {
	for _, tc := range []struct {
		name           string
		requestTimeout time.Duration
		parentTimeout  time.Duration
	}{
		{name: "request", requestTimeout: 20 * time.Millisecond, parentTimeout: time.Second},
		{name: "inherited context", requestTimeout: time.Second, parentTimeout: 20 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				chain := &budgetEstimateChain{horizonChain: newHorizonChain(t)}
				m := New(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(func(cfg *Config) {
					cfg.ReplacementInterval = 200 * time.Millisecond
				}), logr.Discard())
				ctx, cancel := context.WithTimeout(t.Context(), tc.parentTimeout)
				defer cancel()
				started := time.Now()
				req := Request{To: common.Address{1}, Label: "earlier_deadline", Deadline: started.Add(tc.requestTimeout)}
				_, err := m.broadcast(managerCtx(ctx, m), req)
				want := started.Add(min(tc.requestTimeout, tc.parentTimeout))
				if !errors.Is(err, context.DeadlineExceeded) || !chain.overrideDeadline.Equal(want) || !chain.fallbackDeadline.Equal(want) {
					t.Fatalf("estimate err %v, deadlines %s / %s, want %s", err, chain.overrideDeadline, chain.fallbackDeadline, want)
				}
			})
		})
	}
}

func TestInitialGasEstimateSuppliedLimit(t *testing.T) {
	chain := newHorizonChain(t)
	chain.hangEstimates = true
	m := New(chain, mustSigner(t), big.NewInt(11155111), horizonConfig(nil), logr.Discard())
	gas, err := m.callGas(managerCtx(t.Context(), m), Request{GasLimit: 77_000}, receiptTestHeader(100))
	if err != nil || gas != 77_000 || len(chain.nextBlockParents) != 0 || chain.estimateCalls.Load() != 0 {
		t.Fatalf("supplied gas limit = %d, %v, next-block calls %d, fallback calls %d", gas, err, len(chain.nextBlockParents), chain.estimateCalls.Load())
	}
}

func TestInitialGasEstimateRetainsAdmissionUntilUnsignedOutcome(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		chain := newHorizonChain(t)
		chain.hangEstimates = true
		s := &estimateCountingSigner{Signer: mustSigner(t)}
		m := New(chain, s, big.NewInt(11155111), horizonConfig(func(cfg *Config) {
			cfg.ReplacementInterval = 200 * time.Millisecond
		}), logr.Discard())
		startManagerForTest(t, m)
		caller, cancelCaller := context.WithCancel(t.Context())
		result, accepted := m.SendAsync(caller, Request{To: common.Address{1}, Label: "unsigned"})
		if !accepted {
			t.Fatal("request not admitted")
		}
		synctest.Wait()
		cancelCaller()
		synctest.Wait()
		if m.LaneReady() || m.Idle() {
			t.Fatal("caller cancellation released admitted estimate")
		}
		if competing, admitted := m.TrySend(t.Context(), Request{To: common.Address{2}, GasLimit: 21_000}); admitted {
			t.Fatalf("competing work admitted before unsigned outcome: %+v", competing)
		}
		select {
		case got := <-result:
			t.Fatalf("caller cancellation ended admitted work: %+v", got)
		default:
		}
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		got := <-result
		if got.Outcome != OutcomeSubmissionError || !errors.Is(got.Err, context.DeadlineExceeded) || s.calls.Load() != 0 || len(chain.sentTransactions()) != 0 {
			t.Fatalf("unsigned timeout result %+v, signed %d", got, s.calls.Load())
		}
		if !m.LaneReady() {
			t.Fatal("unsigned timeout did not release the lane after its outcome")
		}
		chain.set(func(c *horizonChain) { c.hangEstimates = false })
		next, accepted := m.SendAsync(t.Context(), Request{To: common.Address{2}, Label: "healthy"})
		if !accepted {
			t.Fatal("healthy request not admitted after unsigned outcome")
		}
		sent := chain.waitForSends(t, 1)[0]
		synctest.Wait()
		chain.include(sent)
		if healthy := <-next; healthy.Outcome != OutcomeConfirmed || healthy.Hash != sent.Hash() {
			t.Fatalf("healthy request outcome: %+v", healthy)
		}
	})
}
