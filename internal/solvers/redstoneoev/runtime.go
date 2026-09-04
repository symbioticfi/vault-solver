package redstoneoev

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-errors/errors"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
)

var (
	// minDeposit is the Executor's MIN_DEPOSIT (0.00001 ETH); below this, settlement reverts.
	minDeposit                   = big.NewInt(10_000_000_000_000)
	errStateRefreshBlockBoundary = errors.New("state refresh crossed block boundary")
)

// Run warms the caches, starts the strategy + ops loops, and serves the WS stream until ctx cancels.
func (s *Solver) Run(ctx context.Context) error {
	s.log.Info("starting",
		"callback", s.cfg.Callback.Hex(), "executor", s.cfg.Executor.Hex(), "adapter", s.cfg.Adapter.Hex(),
		"strategy", s.cfg.Strategy.Name,
		"dryRun", s.cfg.DryRun, "signer", s.signer.Address().Hex())
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	if err := s.refreshState(runCtx); err != nil {
		startupErr := errors.Errorf("initial state refresh: %w", err)
		s.log.Error(startupErr, "initial state refresh failed")
		return startupErr
	}
	if s.facts != nil {
		wg.Go(func() { s.facts.Run(runCtx) })
	}
	wg.Go(func() { s.opsLoop(runCtx) })

	err := s.ws.Run(runCtx)
	cancel()
	wg.Wait()
	s.auctionWG.Wait()
	return err
}

// opsLoop refreshes solver-owned state on its interval and after our liquidation results.
func (s *Solver) opsLoop(ctx context.Context) {
	t := time.NewTicker(s.cfg.OpsPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refreshStateAndLog(ctx)
		case <-s.stateRefreshCh:
			s.refreshStateAndLog(ctx)
		}
	}
}

func (s *Solver) refreshStateAndLog(ctx context.Context) {
	if err := s.refreshState(ctx); err != nil {
		if errors.Is(err, errStateRefreshBlockBoundary) {
			s.log.V(1).Info("state refresh crossed block boundary; keeping cache", "error", err)
			return
		}
		s.log.Error(err, "state refresh failed; keeping cache")
	}
}

func (s *Solver) requestStateRefresh() {
	select {
	case s.stateRefreshCh <- struct{}{}:
	default:
	}
}

// refreshState publishes one head-stable decision snapshot. A failed component read keeps the previous
// complete snapshot; it never publishes a mixture of old funding and new adapter/accounting facts.
func (s *Solver) refreshState(ctx context.Context) (err error) {
	observedAt := time.Now()
	timer := observability.StartOperation(s.metrics.operation())
	defer func() { timer.Finish(ctx, observability.OutcomeForError(err)) }()
	startHead, err := s.readHead(ctx)
	if err != nil {
		return err
	}
	st, err := s.reader.ReadExecutorState(ctx, s.cfg.Executor, s.signer.Address())
	if err != nil {
		return errors.Errorf("read executor state: %w", err)
	}
	callbackNative, err := s.reader.ReadNativeBalance(ctx, s.cfg.Callback)
	if err != nil {
		return errors.Errorf("read callback balance %s: %w", s.cfg.Callback.Hex(), err)
	}
	adapter, err := s.reader.ReadAdapterSnapshot(ctx, s.cfg.Adapter, s.cfg.Callback)
	if err != nil {
		return errors.Errorf("read adapter snapshot %s: %w", s.cfg.Adapter.Hex(), err)
	}
	gasPrices, err := s.reader.ReadGasPrices(ctx, adapter, time.Unix(int64(startHead.Time), 0))
	if err != nil {
		return errors.Errorf("read gas prices for loan %s: %w", adapter.Loan.Hex(), err)
	}
	endHead, err := s.readHead(ctx)
	if err != nil {
		return err
	}
	if endHead.Number != startHead.Number {
		return errors.Errorf(
			"%w: start %d, end %d",
			errStateRefreshBlockBoundary,
			startHead.Number,
			endHead.Number,
		)
	}
	s.state.store(cachedState{
		Exec: st, CallbackNative: callbackNative, Adapter: adapter, GasPrices: gasPrices,
		GasLimit: startHead.GasLimit, UpdatedAt: observedAt,
	})
	s.applyExecutorState(st, observedAt)
	s.metrics.stateRefreshed()
	return nil
}

type headSnapshot struct {
	Number   uint64
	GasLimit uint64
	Time     uint64
}

func (s *Solver) readHead(ctx context.Context) (headSnapshot, error) {
	header, err := s.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return headSnapshot{}, errors.Errorf("read latest header: %w", err)
	}
	if header == nil {
		return headSnapshot{}, errors.New("latest header missing")
	}
	return headSnapshot{Number: header.Number.Uint64(), GasLimit: header.GasLimit, Time: header.Time}, nil
}

// applyExecutorState reconciles bookkeeping derived from the Executor state read.
func (s *Solver) applyExecutorState(st ExecutorState, now time.Time) {
	s.exposures.reconcile(st.Nonce.Uint64(), now)
	s.updateWinMetrics()
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

// cachedState is one coherent, atomically-swapped view of every on-chain fact used by a bid decision.
type cachedState struct {
	Exec           ExecutorState
	CallbackNative *big.Int
	Adapter        decision.AdapterSnapshot
	GasPrices      *liquidlanegas.PriceSnapshot
	GasLimit       uint64
	UpdatedAt      time.Time
}

type stateCache struct {
	p atomic.Pointer[cachedState]
}

func (s *Solver) adapterSnapshot() (decision.AdapterSnapshot, bool) {
	state, ok := s.state.load()
	return state.Adapter, ok
}

func (s *stateCache) store(v cachedState) {
	v.Exec = cloneExecutorState(v.Exec)
	v.CallbackNative = cloneBig(v.CallbackNative)
	v.Adapter = cloneAdapterSnapshot(v.Adapter)
	s.p.Store(&v)
}

func (s *stateCache) load() (cachedState, bool) {
	v := s.p.Load()
	if v == nil {
		return cachedState{}, false
	}
	out := *v
	out.Exec = cloneExecutorState(out.Exec)
	out.CallbackNative = cloneBig(out.CallbackNative)
	out.Adapter = cloneAdapterSnapshot(out.Adapter)
	return out, true
}

func cloneExecutorState(source ExecutorState) ExecutorState {
	return ExecutorState{Nonce: cloneBig(source.Nonce), Deposit: cloneBig(source.Deposit), Locked: source.Locked}
}
