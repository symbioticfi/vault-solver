// Package uniswapx implements UniswapX RFQ and public V2 filling backed by LiquidLane.
package uniswapx

import (
	"context"
	"math/big"
	"net"
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
	reportFatal   func(error)

	// refreshMu serializes chain snapshots. quoteState is immutable after publication and is
	// published and invalidated under quotes.mu. Quote requests are stateless because Uniswap intentionally hides
	// whether each request is indicative or hard.
	refreshMu             sync.Mutex
	quotes                quotePublication
	chainTime             atomic.Int64
	blockUntil            atomic.Int64
	localBlockUntil       atomic.Int64
	exclusiveBlockUntil   atomic.Int64
	exclusiveStateUnknown atomic.Bool
	warmupUntil           atomic.Int64
	lastExclusivePoll     atomic.Int64
	refreshCh             chan struct{}
	// stateMu guards order retry/dedup and breaker history.
	stateMu      sync.Mutex
	executions   map[common.Hash]executionState
	capacity     liquidlane.CapacityLedger
	obligations  map[common.Hash]trackedExclusive
	failureTimes []time.Time
	metrics      *uniswapXMetrics
	operations   uniswapXOperationObservers
}

type chainReader interface {
	ResolveRoutes(ctx context.Context, adapters []common.Address) ([]liquidlane.Route, error)
	validateExecutorCode(ctx context.Context, executor common.Address) error
	validateExecutorCaller(ctx context.Context, executor, caller common.Address) error
	unauthorizedAdapters(
		ctx context.Context,
		executor common.Address,
		routes []liquidlane.Route,
	) ([]common.Address, error)
	ValidateGasTokens(routes []liquidlane.Route) error
	Quote(ctx context.Context, routes []liquidlane.Route, executor common.Address, now time.Time) (snapshot, error)
	Fill(
		ctx context.Context,
		routes []liquidlane.Route,
		executor common.Address,
		tokenIn common.Address,
		amountIn *big.Int,
		now time.Time,
	) (fillSnapshot, error)
	ReadFillQuotes(
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
	LaneReady() bool
	Available() bool
}

type contractCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

func New(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
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
	var discountClient liquiddiscounts.Provider
	if cfg.usesDiscounts() {
		discountClient = liquiddiscounts.NewClient(cfg.Discounts.BaseURL)
	}
	s := &Solver{
		cfg:           cfg,
		chainID:       deps.Chain.ChainID().Int64(),
		solverAddress: deps.Signer.Address(),
		chain:         deps.Chain,
		reader:        reader,
		strategy:      strategy,
		txm:           deps.TxManager,
		confirmations: deps.TxManager.Confirmations(),
		orders:        newOrderClient(cfg.OrderServer, orderKey),
		discounts:     discountClient,
		log:           log,
		reportFatal:   deps.ReportFatal,
		refreshCh:     make(chan struct{}, 1),
		executions:    make(map[common.Hash]executionState),
		obligations:   make(map[common.Hash]trackedExclusive),
	}
	if deps.Metrics != nil {
		s.metrics, err = newUniswapXMetrics(deps.Metrics.Registerer(), s, cfg.Strategy.Name)
		if err != nil {
			return nil, err
		}
		s.operations = s.metrics.operations
	}
	return s, nil
}

func (s *Solver) Name() string { return Name }

// startup validates the executor and delivery path before any quote can be advertised.
func (s *Solver) startup(ctx context.Context) ([]liquidlane.Route, error) {
	fail := func(message string, err error) ([]liquidlane.Route, error) {
		s.log.Error(err, message, "executor", s.cfg.Executor.Hex(), "caller", s.solverAddress.Hex(), "adapters", s.cfg.Adapters, "solverMode", s.cfg.SolverMode)
		return nil, err
	}
	routes, err := s.reader.ResolveRoutes(ctx, s.cfg.Adapters)
	if err != nil {
		return fail("adapter resolution failed", errors.Errorf("resolve routes: %w", err))
	}
	if len(routes) == 0 && s.cfg.restrictsToAdapters() {
		return fail("adapter resolution failed", errors.New("no LiquidLane routes resolved"))
	}
	if err := s.reader.validateExecutorCode(ctx, s.cfg.Executor); err != nil {
		return fail("executor validation failed", errors.Errorf("validate executor: %w", err))
	}
	if err := s.reader.validateExecutorCaller(ctx, s.cfg.Executor, s.solverAddress); err != nil {
		return fail("executor caller validation failed", errors.Errorf("validate executor caller: %w", err))
	}
	if s.cfg.restrictsToAdapters() {
		unauthorized, err := s.reader.unauthorizedAdapters(ctx, s.cfg.Executor, routes)
		if err != nil {
			return fail("adapter validation failed", errors.Errorf("validate adapters: %w", err))
		}
		if len(unauthorized) > 0 {
			return fail("adapter validation failed", errors.Errorf("validate adapters: executor %s is not authorized as direct filler for configured adapters: %v", s.cfg.Executor.Hex(), unauthorized))
		}
	}
	if err := s.reader.ValidateGasTokens(routes); err != nil {
		return fail("adapter validation failed", errors.Errorf("validate adapter gas tokens: %w", err))
	}
	if _, err := s.orders.openOrders(ctx, s.chainID, &s.cfg.Executor); err != nil {
		return fail("exclusive order delivery validation failed", errors.Errorf("validate exclusive order delivery: %w", err))
	}
	return routes, nil
}

func (s *Solver) Run(ctx context.Context) error {
	routes, err := s.startup(ctx)
	if err != nil {
		return err
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
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", server.Addr)
	if err != nil {
		return errors.Errorf("listen for quotes: %w", err)
	}
	g, groupCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return s.serveQuoteServer(groupCtx, server, listener) })
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

func (s *Solver) serveQuoteServer(ctx context.Context, server *http.Server, listener net.Listener) error {
	err := server.Serve(listener)
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	err = errors.Errorf("serve quote server: %w", err)
	if s.reportFatal != nil && ctx.Err() == nil {
		s.reportFatal(err)
	}
	return err
}
