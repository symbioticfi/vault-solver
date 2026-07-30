// Package uniswapx implements UniswapX RFQ and public V2 filling backed by LiquidLane.
package uniswapx

import (
	"context"
	"math/big"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/solver"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const Name = "uniswapx-filler"

const orderQueueCapacity = 256

//nolint:gochecknoinits // solver registration follows the framework plugin convention.
func init() { solver.Register(Name, factory) }

type Solver struct {
	cfg           *Config
	chainID       int64
	solverAddress common.Address
	chain         contractCaller
	reader        chainReader
	strategy      strategytypes.Strategy
	txm           transactionManager
	confirmations uint64
	orders        orderPoller
	discounts     liquiddiscounts.Provider
	log           logr.Logger

	// refreshMu serializes chain snapshots. quoteState is immutable after publication and is
	// replaced atomically. Quote requests are stateless because Uniswap intentionally hides
	// whether each request is indicative or hard.
	refreshMu             sync.Mutex
	quoteState            atomic.Pointer[quoteState]
	quoteEpoch            atomic.Uint64
	planningFills         atomic.Int64
	chainTime             atomic.Int64
	blockUntil            atomic.Int64
	localBlockUntil       atomic.Int64
	exclusiveBlockUntil   atomic.Int64
	exclusiveStateUnknown atomic.Bool
	warmupUntil           atomic.Int64
	lastExclusivePoll     atomic.Int64
	refreshCh             chan struct{}
	// stateMu guards order retry/dedup and breaker history.
	stateMu           sync.Mutex
	filled            map[common.Hash]time.Time
	retryAt           map[common.Hash]time.Time
	inFlight          map[common.Hash]bool
	attempts          map[common.Hash]int
	capacity          liquidlane.CapacityLedger
	exclusiveUntil    map[common.Hash]time.Time
	exclusiveTerminal map[common.Hash]time.Time
	failureTimes      []time.Time
	metrics           *uniswapXMetrics
}

type chainReader interface {
	resolveRoutes(ctx context.Context, adapters []common.Address) ([]liquidlane.Route, error)
	validateExecutorCode(ctx context.Context, executor common.Address) error
	validateExecutorCaller(ctx context.Context, executor, caller common.Address) error
	unauthorizedAdapters(
		ctx context.Context,
		executor common.Address,
		routes []liquidlane.Route,
	) ([]common.Address, error)
	validateGasTokens(routes []liquidlane.Route) error
	quoteSnapshot(ctx context.Context, routes []liquidlane.Route, executor common.Address, now time.Time) (snapshot, error)
	fillSnapshot(
		ctx context.Context,
		routes []liquidlane.Route,
		executor common.Address,
		tokenIn common.Address,
		amountIn *big.Int,
		now time.Time,
	) (fillSnapshot, error)
	physicalFillQuotes(
		ctx context.Context,
		routes []liquidlane.Route,
		tokenIn common.Address,
		amountIn *big.Int,
	) ([]liquidlane.FillQuote, error)
	latestBlockTime(ctx context.Context) (time.Time, error)
	transactionBlockTimeConfirmed(
		ctx context.Context,
		txHash common.Hash,
		confirmations uint64,
	) (time.Time, error)
}

type orderPoller interface {
	openOrders(ctx context.Context, chainID int64, filler *common.Address) ([]orderEntry, error)
	recentOrders(
		ctx context.Context,
		chainID int64,
		filler common.Address,
		createdAfter time.Time,
	) ([]orderEntry, error)
	ordersByHash(
		ctx context.Context,
		chainID int64,
		hashes []common.Hash,
	) (map[common.Hash]orderTerminal, error)
}

type transactionManager interface {
	MaxFeePerGas(ctx context.Context) (*big.Int, error)
	SendAsync(ctx context.Context, request txmanager.Request) (<-chan txmanager.Result, bool)
}

type contractCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	orderKey := os.Getenv(cfg.OrderServer.APIKeyEnv)
	if orderKey == "" {
		return nil, errors.New("UniswapX order API key env must be non-empty")
	}
	log := deps.Log.WithName(Name)
	reader, err := newReader(deps.Chain, log, cfg.Gas, cfg.LiquidityLens)
	if err != nil {
		return nil, err
	}
	strategy, err := newStrategy(cfg.Strategy)
	if err != nil {
		return nil, err
	}
	var metrics *uniswapXMetrics
	if deps.Metrics != nil {
		metrics, err = newUniswapXMetrics(deps.Metrics.Registerer())
		if err != nil {
			return nil, err
		}
	}
	var discountClient liquiddiscounts.Provider
	if cfg.usesDiscounts() {
		discountClient = liquiddiscounts.NewClient(cfg.Discounts.BaseURL)
	}
	return &Solver{
		cfg:               cfg,
		chainID:           deps.Chain.ChainID().Int64(),
		solverAddress:     deps.Signer.Address(),
		chain:             deps.Chain,
		reader:            reader,
		strategy:          strategy,
		txm:               deps.TxManager,
		confirmations:     deps.TxManager.Confirmations(),
		orders:            newOrderClient(cfg.OrderServer, orderKey),
		discounts:         discountClient,
		log:               log,
		refreshCh:         make(chan struct{}, 1),
		filled:            make(map[common.Hash]time.Time),
		retryAt:           make(map[common.Hash]time.Time),
		inFlight:          make(map[common.Hash]bool),
		attempts:          make(map[common.Hash]int),
		metrics:           metrics,
		exclusiveUntil:    make(map[common.Hash]time.Time),
		exclusiveTerminal: make(map[common.Hash]time.Time),
	}, nil
}

func (s *Solver) Name() string { return Name }

func (s *Solver) Run(ctx context.Context) error {
	routes, err := s.reader.resolveRoutes(ctx, s.cfg.Adapters)
	if err != nil {
		startupErr := errors.Errorf("resolve routes: %w", err)
		s.log.Error(startupErr, "adapter resolution failed",
			"solverMode", s.cfg.SolverMode, "executor", s.cfg.Executor.Hex(), "adapters", s.cfg.Adapters)
		return startupErr
	}
	if len(routes) == 0 && s.cfg.restrictsToAdapters() {
		startupErr := errors.New("no LiquidLane routes resolved")
		s.log.Error(startupErr, "adapter resolution failed",
			"solverMode", s.cfg.SolverMode, "executor", s.cfg.Executor.Hex(), "adapters", s.cfg.Adapters)
		return startupErr
	}
	if err := s.reader.validateExecutorCode(ctx, s.cfg.Executor); err != nil {
		startupErr := errors.Errorf("validate executor: %w", err)
		s.log.Error(startupErr, "executor validation failed", "executor", s.cfg.Executor.Hex())
		return startupErr
	}
	if err := s.reader.validateExecutorCaller(ctx, s.cfg.Executor, s.solverAddress); err != nil {
		startupErr := errors.Errorf("validate executor caller: %w", err)
		s.log.Error(
			startupErr,
			"executor caller validation failed",
			"executor", s.cfg.Executor.Hex(),
			"caller", s.solverAddress.Hex(),
		)
		return startupErr
	}
	if s.cfg.restrictsToAdapters() {
		unauthorized, err := s.reader.unauthorizedAdapters(ctx, s.cfg.Executor, routes)
		if err != nil {
			startupErr := errors.Errorf("validate adapters: %w", err)
			s.log.Error(startupErr, "adapter validation failed",
				"solverMode", s.cfg.SolverMode, "executor", s.cfg.Executor.Hex(), "adapters", s.cfg.Adapters)
			return startupErr
		}
		if len(unauthorized) > 0 {
			startupErr := errors.Errorf(
				"validate adapters: executor %s is not authorized as direct filler for configured adapters: %v",
				s.cfg.Executor.Hex(), unauthorized,
			)
			s.log.Error(startupErr, "adapter validation failed",
				"solverMode", s.cfg.SolverMode, "executor", s.cfg.Executor.Hex(), "adapters", s.cfg.Adapters)
			return startupErr
		}
	}
	if err := s.reader.validateGasTokens(routes); err != nil {
		startupErr := errors.Errorf("validate adapter gas tokens: %w", err)
		s.log.Error(startupErr, "adapter validation failed", "executor", s.cfg.Executor.Hex(), "adapters", s.cfg.Adapters)
		return startupErr
	}
	if _, err := s.orders.openOrders(ctx, s.chainID, &s.cfg.Executor); err != nil {
		startupErr := errors.Errorf("validate exclusive order delivery: %w", err)
		s.log.Error(startupErr, "exclusive order delivery validation failed",
			"executor", s.cfg.Executor.Hex(), "orderApi", s.cfg.OrderServer.BaseURL)
		return startupErr
	}
	// Reconcile recent terminal history before serving quotes after every process start.
	s.exclusiveStateUnknown.Store(true)
	s.warmupUntil.Store(time.Now().Add(s.cfg.QuoteServer.QuoteTTL).Unix())
	if err := s.refreshQuoteState(ctx, routes); err != nil {
		startupErr := errors.Errorf("initial quote refresh: %w", err)
		s.log.Error(startupErr, "initial quote refresh failed", "routes", len(routes))
		return startupErr
	}
	s.log.Info("starting", "chainId", s.chainID, "solverMode", s.cfg.SolverMode,
		"reactor", s.cfg.Reactor.Hex(), "executor", s.cfg.Executor.Hex(),
		"routes", len(routes), "gasAccounting", s.cfg.Gas != nil,
		"listen", s.cfg.QuoteServer.ListenAddress, "orderApi", s.cfg.OrderServer.BaseURL)

	server := s.newQuoteHTTPServer()
	g, groupCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-groupCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(groupCtx), 2*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	})
	g.Go(func() error { return s.refreshLoop(groupCtx, routes) })
	orders := make(chan *resolvedOrder, orderQueueCapacity)
	g.Go(func() error { return s.orderLoop(groupCtx, orders) })
	g.Go(func() error { return s.fillLoop(groupCtx, routes, orders) })
	return g.Wait()
}
