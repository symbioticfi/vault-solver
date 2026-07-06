package redstoneoev

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

// minDeposit is the Executor's MIN_DEPOSIT (0.00001 ETH); below this, settlement reverts.
var minDeposit = big.NewInt(10_000_000_000_000)

// Run warms the caches, starts the strategy + ops loops, and serves the WS stream until ctx cancels.
func (s *Solver) Run(ctx context.Context) error {
	s.log.Info("starting",
		"callback", s.cfg.Callback.Hex(), "executor", s.cfg.Executor.Hex(), "adapter", s.cfg.Adapter.Hex(),
		"strategy", s.strategyName,
		"dryRun", s.dryRun, "signer", s.deps.Signer.Address().Hex())
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Go(func() { s.strategy.Run(runCtx) })
	s.refreshState(runCtx) // seed nonce + deposit before any bid
	wg.Go(func() { s.opsLoop(runCtx) })

	err := s.ws.Run(runCtx)
	cancel()
	wg.Wait()
	return err
}

// opsLoop periodically refreshes the Executor accounting used for pre-bid checks and nonce reconciliation.
func (s *Solver) opsLoop(ctx context.Context) {
	t := time.NewTicker(s.cfg.OpsPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refreshState(ctx)
		}
	}
}

// refreshState reads solver-owned Executor and adapter state into the cache. Strategy-owned
// callback/funding state is read inside strategies.
func (s *Solver) refreshState(ctx context.Context) {
	now := time.Now()
	gasLimit := s.readGasLimit(ctx)
	st, err := s.reader.ReadExecutorState(ctx, s.cfg.Executor, s.deps.Signer.Address())
	if err != nil {
		s.log.Error(err, "read executor state failed; keeping cache")
		return
	}
	adapter, err := s.reader.ReadAdapterSnapshot(ctx, s.cfg.Adapter, s.cfg.Callback)
	if err != nil {
		s.log.Error(err, "read adapter snapshot failed; keeping cache", "adapter", s.cfg.Adapter.Hex())
		return
	}
	s.state.store(cachedState{Exec: st, Adapter: adapter, GasLimit: gasLimit, UpdatedAt: now})
	s.applyExecutorState(st, now)
}

func (s *Solver) readGasLimit(ctx context.Context) uint64 {
	header, err := s.deps.Chain.HeaderByNumber(ctx, nil)
	if err != nil {
		s.log.Error(err, "read latest header failed; using zero gas limit")
		return 0
	}
	if header == nil {
		s.log.Error(errors.New("latest header missing"), "using zero gas limit")
		return 0
	}
	return header.GasLimit
}

// applyExecutorState reconciles bookkeeping derived from the Executor state read.
func (s *Solver) applyExecutorState(st ExecutorState, now time.Time) {
	s.pruneReservations(st.Nonce.Uint64(), now)
	s.nonces.reconcile(st.Nonce.Uint64())
	s.metrics.depositWei(weiFloat(st.Deposit))

	belowFloor := st.Deposit.Cmp(minDeposit) < 0
	s.metrics.depositBelowFloor(belowFloor)
	if belowFloor {
		s.log.Error(errors.New("executor deposit below MIN_DEPOSIT"),
			"bidding will skip until the operator refuels the Executor deposit",
			"depositWei", st.Deposit, "minDepositWei", minDeposit)
	}
	s.log.V(1).Info("state", "nonce", st.Nonce, "depositWei", st.Deposit, "locked", st.Locked)
}

// weiFloat converts wei to a float64 for gauge reporting only.
func weiFloat(n *big.Int) float64 {
	f, _ := new(big.Float).SetInt(n).Float64()
	return f
}

// cachedState is the atomically-swapped snapshot of solver-owned on-chain state needed for pre-bid checks.
type cachedState struct {
	Exec      ExecutorState
	Adapter   types.AdapterSnapshot
	GasLimit  uint64
	UpdatedAt time.Time
}

type stateCache struct {
	p atomic.Pointer[cachedState]
}

func (s *stateCache) store(v cachedState) {
	v.Adapter = cloneAdapterSnapshot(v.Adapter)
	s.p.Store(&v)
}

func (s *stateCache) load() (cachedState, bool) {
	v := s.p.Load()
	if v == nil {
		return cachedState{}, false
	}
	out := *v
	out.Adapter = cloneAdapterSnapshot(out.Adapter)
	return out, true
}
