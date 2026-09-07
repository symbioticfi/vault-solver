package redstoneoev

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/observability"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
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
		"strategy", s.strategyName,
		"dryRun", s.dryRun, "signer", s.deps.Signer.Address().Hex())
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	if err := s.refreshStateWithBoundaryRetry(runCtx); err != nil {
		startupErr := errors.Errorf("initial state refresh: %w", err)
		s.log.Error(startupErr, "initial state refresh failed")
		return startupErr
	}
	wg.Go(func() { s.strategy.Run(runCtx) })
	wg.Go(func() { s.opsLoop(runCtx) })

	err := s.ws.Run(runCtx)
	cancel()
	// The WebSocket has stopped dispatching; join the single outstanding decision.
	s.bidWG.Wait()
	wg.Wait()
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
	if err := s.refreshStateWithBoundaryRetry(ctx); err != nil && ctx.Err() == nil {
		s.log.Error(err, "state refresh failed; keeping cache")
	}
}

// refreshStateWithBoundaryRetry gives a latest-state snapshot one immediate second chance when its
// bracketing head changes. A second crossing returns to the caller: startup fails visibly, while the
// runtime loop retains its last-known-good cache and tries again on the next poll or refresh signal.
func (s *Solver) refreshStateWithBoundaryRetry(ctx context.Context) error {
	for attempt := range 2 {
		err := s.refreshState(ctx)
		if !errors.Is(err, errStateRefreshBlockBoundary) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt != 0 {
			return errors.Errorf("state refresh head remained unstable after immediate retry: %w", err)
		}
		s.log.V(1).Info("state refresh crossed block boundary; retrying latest snapshot", "error", err)
	}
	return nil
}

func (s *Solver) requestStateRefresh() {
	select {
	case s.stateRefreshCh <- struct{}{}:
	default:
	}
}

// refreshState reads solver-owned Executor and adapter state into the cache. Strategy-owned
// callback/funding state is read inside strategies.
func (s *Solver) refreshState(ctx context.Context) error {
	observation := observability.StartOperation(s.stateRefreshObserver)
	outcome := observability.ExternalOperationSuccess
	defer func() { observation.Finish(ctx, outcome) }()
	next, err := s.stateSource.Snapshot(ctx)
	if err == nil {
		s.state.store(next)
		s.applyExecutorState(next.Exec, next.UpdatedAt)
		s.metrics.stateRefreshed()
		return nil
	}
	outcome = observability.ExternalOperationError
	if _, retained := s.state.load(); retained && errors.Is(err, errStateRefreshBlockBoundary) {
		outcome = observability.ExternalOperationDegraded
	}
	return err
}

// stateSnapshotSource owns the cross-read consistency boundary. A successful result is complete and
// safe to install atomically; an error means the solver must retain its previous snapshot and metrics.
type stateSnapshotSource interface {
	Snapshot(context.Context) (cachedState, error)
}

type stateChainReader interface {
	ReadExecutorState(ctx context.Context, executor, signer common.Address) (ExecutorState, error)
	ReadAdapterSnapshot(ctx context.Context, adapter, callback common.Address) (strategytypes.AdapterSnapshot, error)
	ReadGasPrices(
		ctx context.Context,
		adapter strategytypes.AdapterSnapshot,
		observedAt time.Time,
	) (*liquidlanegas.PriceSnapshot, error)
}

type headReader interface {
	HeaderByNumber(ctx context.Context, number *big.Int) (*ethtypes.Header, error)
}

type coherentStateSource struct {
	heads             headReader
	reader            stateChainReader
	executor, adapter common.Address
	callback, signer  common.Address
}

func (r *coherentStateSource) Snapshot(ctx context.Context) (cachedState, error) {
	next := cachedState{UpdatedAt: time.Now()}
	before, err := readHead(ctx, r.heads)
	if err != nil {
		return cachedState{}, err
	}
	next.GasLimit = before.GasLimit
	next.Exec, err = r.reader.ReadExecutorState(ctx, r.executor, r.signer)
	if err != nil {
		return cachedState{}, errors.Errorf("read executor state: %w", err)
	}
	next.Adapter, err = r.reader.ReadAdapterSnapshot(ctx, r.adapter, r.callback)
	if err != nil {
		return cachedState{}, errors.Errorf("read adapter snapshot %s: %w", r.adapter.Hex(), err)
	}
	next.GasPrices, err = r.reader.ReadGasPrices(ctx, next.Adapter, time.Unix(int64(before.Time), 0))
	if err != nil {
		return cachedState{}, errors.Errorf("read gas prices for loan %s: %w", next.Adapter.Loan.Hex(), err)
	}
	after, err := readHead(ctx, r.heads)
	if err != nil {
		return cachedState{}, err
	}
	if !before.sameBlock(after) {
		return cachedState{}, errors.Errorf("%w: start %d/%s, end %d/%s", errStateRefreshBlockBoundary, before.Number, before.Hash.Hex(), after.Number, after.Hash.Hex())
	}
	return next, nil
}

type headSnapshot struct {
	Hash     common.Hash
	Number   uint64
	GasLimit uint64
	Time     uint64
}

func (h headSnapshot) sameBlock(other headSnapshot) bool {
	return h.Number == other.Number && h.Hash == other.Hash
}

func readHead(ctx context.Context, source headReader) (headSnapshot, error) {
	header, err := source.HeaderByNumber(ctx, nil)
	if err != nil {
		return headSnapshot{}, errors.Errorf("read latest header: %w", err)
	}
	if header == nil {
		return headSnapshot{}, errors.New("latest header missing")
	}
	if header.Number == nil {
		return headSnapshot{}, errors.New("latest header number missing")
	}
	return headSnapshot{
		Hash:     header.Hash(),
		Number:   header.Number.Uint64(),
		GasLimit: header.GasLimit,
		Time:     header.Time,
	}, nil
}

// applyExecutorState reconciles bookkeeping derived from the Executor state read.
func (s *Solver) applyExecutorState(st ExecutorState, now time.Time) {
	s.pruneReservations(st.Nonce.Uint64(), now)
	s.nonces.reconcile(st.Nonce.Uint64())
	s.metrics.depositWei(weiFloat(st.Deposit))

	belowFloor := st.Deposit.Cmp(minDeposit) < 0
	s.metrics.depositFloorState(belowFloor)
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
	Adapter   strategytypes.AdapterSnapshot
	GasPrices *liquidlanegas.PriceSnapshot
	GasLimit  uint64
	UpdatedAt time.Time
}

type stateCache struct {
	p atomic.Pointer[cachedState]
}

func (s *Solver) adapterSnapshot() (strategytypes.AdapterSnapshot, bool) {
	state, ok := s.state.load()
	return state.Adapter, ok
}

func (s *stateCache) store(snapshot cachedState) {
	owned := snapshot.clone()
	s.p.Store(&owned)
}

func (s *stateCache) load() (cachedState, bool) {
	if snapshot := s.p.Load(); snapshot != nil {
		return snapshot.clone(), true
	}
	return cachedState{}, false
}

func (snapshot cachedState) clone() cachedState {
	snapshot.Adapter = snapshot.Adapter.Clone()
	snapshot.Exec.Nonce, snapshot.Exec.Deposit = bigmath.Clone(snapshot.Exec.Nonce), bigmath.Clone(snapshot.Exec.Deposit)
	// Gas prices expose only copying accessors and are immutable after construction.
	return snapshot
}
