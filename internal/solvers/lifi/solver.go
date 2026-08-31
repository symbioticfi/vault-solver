// Package lifi implements the LI.FI same-chain intent solver. It publishes LiquidLane-backed
// standing quotes to the LI.FI order server and listens for matched escrow orders.
package lifi

import (
	"context"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/solver"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/webhook"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const Name = "lifi-samechain"

const (
	lifiOrderStatusNone uint8 = iota
	lifiOrderStatusDeposited
	lifiOrderStatusClaimed
	lifiOrderStatusRefunded
)

type Solver struct {
	cfg          *Config
	chainID      int64
	reader       chainReader
	strategy     types.Strategy
	caller       common.Address
	orders       *orderClient
	feed         *orderFeed
	txm          txSender
	log          logr.Logger
	now          func(context.Context) (time.Time, error)
	maxFeePerGas func(context.Context) (*big.Int, error)
	wallNow      func() time.Time
	txLaneState  transactionLaneState
	capacity     liquidlane.CapacityLedger
	quoteRefresh chan struct{}
	discounts    discounts.Provider
}

type chainReader interface {
	routeReader
	settlementReader
	chainHeadReader
}

type routeReader interface {
	ResolveRoutes(ctx context.Context, adapters []common.Address) ([]route, error)
	ValidateGasTokens(routes []route) error
	Quote(ctx context.Context, routes []route, executor common.Address, chainTime time.Time) (quoteSnapshotSet, error)
	Fill(
		ctx context.Context, routes []route, executor, tokenIn common.Address, amountIn *big.Int, chainTime time.Time,
	) (fillSnapshotSet, error)
}

type settlementReader interface {
	validateExecutor(
		ctx context.Context,
		executor, inputSettler, outputSettler, caller common.Address,
	) error
	validateZeroGovernanceFee(ctx context.Context, inputSettler common.Address) error
	validateDirectAuthorization(ctx context.Context, executor common.Address, routes []route) error
	orderIdentifier(ctx context.Context, inputSettler common.Address, order inputsettler.StandardOrder) (common.Hash, error)
	orderStatus(ctx context.Context, inputSettler common.Address, orderID common.Hash) (uint8, error)
}

type chainHeadReader interface {
	latestBlockNumber(ctx context.Context) (uint64, error)
}

type txSender interface {
	SendAsync(ctx context.Context, req txmanager.Request) (<-chan txmanager.Result, bool)
}

type transactionLaneState interface {
	LaneReady() bool
	SubscribeLaneState() (<-chan struct{}, func())
}

func ValidateConfig(raw yaml.Node) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	if err := validateStrategyConfig(cfg.Strategy); err != nil {
		return errors.Errorf("strategy: %w", err)
	}
	return nil
}

func validateStrategyConfig(spec StrategyConfig) error {
	switch spec.Name {
	case defaultstrategy.Name:
		return defaultstrategy.ValidateConfig(spec.Config)
	case webhookstrategy.Name:
		return webhookstrategy.ValidateConfig(spec.Config)
	default:
		return unknownStrategyError(spec.Name)
	}
}

func newStrategy(spec StrategyConfig) (types.Strategy, error) {
	switch spec.Name {
	case defaultstrategy.Name:
		return defaultstrategy.NewFromConfig(spec.Config)
	case webhookstrategy.Name:
		return webhookstrategy.NewFromConfig(spec.Config)
	default:
		return nil, unknownStrategyError(spec.Name)
	}
}

func unknownStrategyError(name string) error {
	return errors.Errorf("unknown LI.FI strategy %q (registered: %v)", name, strategyNames())
}

func strategyNames() []string {
	return []string{defaultstrategy.Name, webhookstrategy.Name}
}

func Factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	apiKey := os.Getenv(cfg.OrderServer.APIKeyEnv)
	if apiKey == "" {
		return nil, errors.Errorf("%s: order server api key env %q is empty", Name, cfg.OrderServer.APIKeyEnv)
	}

	log := deps.Log.WithName(Name)
	chainID := deps.Chain.ChainID().Int64()
	strategy, err := newStrategy(cfg.Strategy)
	if err != nil {
		return nil, err
	}
	reader, err := newReader(deps.Chain, log, cfg.Gas, cfg.LiquidityLens)
	if err != nil {
		return nil, err
	}
	result := &Solver{
		cfg:          cfg,
		chainID:      chainID,
		reader:       reader,
		strategy:     strategy,
		caller:       deps.Signer.Address(),
		orders:       newOrderClient(cfg.OrderServer.BaseURL, apiKey, cfg.OrderServer.HTTPTimeout, chainID),
		feed:         newOrderFeed(cfg.OrderServer.WSURL, apiKey, log),
		txm:          deps.TxManager,
		log:          log,
		now:          reader.latestBlockTime,
		maxFeePerGas: deps.TxManager.MaxFeePerGas,
		wallNow:      time.Now,
		txLaneState:  deps.TxManager,
	}
	if cfg.usesDiscounts() {
		result.discounts = discounts.NewClient(cfg.DiscountsURL)
	}
	return result, nil
}

func (s *Solver) Name() string { return Name }

func (s *Solver) ShutdownPreparationTimeout() time.Duration {
	return 2 * s.cfg.OrderServer.HTTPTimeout
}

func (s *Solver) startupFailure(err error, message string, keysAndValues ...any) error {
	s.log.Error(err, message, keysAndValues...)
	return err
}

func (s *Solver) Run(ctx context.Context) error {
	routes, err := s.reader.ResolveRoutes(ctx, s.cfg.Adapters)
	if err != nil {
		return s.startupFailure(errors.Errorf("lifi: resolve routes: %w", err), "adapter resolution failed",
			"solverMode", s.cfg.SolverMode, "executor", s.cfg.Executor.Hex(), "adapters", s.cfg.Adapters)
	}
	if len(routes) == 0 {
		return s.startupFailure(errors.New("lifi: no quoteable routes resolved from configured adapters"),
			"adapter resolution failed", "solverMode", s.cfg.SolverMode, "executor", s.cfg.Executor.Hex(),
			"adapters", s.cfg.Adapters)
	}
	if err := s.reader.ValidateGasTokens(routes); err != nil {
		return s.startupFailure(errors.Errorf("lifi: validate gas oracles: %w", err),
			"gas oracle validation failed", "routes", len(routes), "gasAccounting", s.cfg.Gas != nil)
	}
	if err := s.reader.validateExecutor(
		ctx, s.cfg.Executor, s.cfg.InputSettler, s.cfg.OutputSettler, s.caller,
	); err != nil {
		return s.startupFailure(errors.Errorf("lifi: validate executor: %w", err), "executor validation failed",
			"executor", s.cfg.Executor.Hex(), "caller", s.caller.Hex(),
			"inputSettler", s.cfg.InputSettler.Hex(), "outputSettler", s.cfg.OutputSettler.Hex())
	}
	if err := s.reader.validateZeroGovernanceFee(ctx, s.cfg.InputSettler); err != nil {
		return s.startupFailure(errors.Errorf("lifi: validate governance fee: %w", err),
			"governance fee validation failed", "inputSettler", s.cfg.InputSettler.Hex())
	}
	if !s.cfg.usesDiscounts() {
		if err := s.reader.validateDirectAuthorization(ctx, s.cfg.Executor, routes); err != nil {
			return s.startupFailure(errors.Errorf("lifi: validate direct authorization: %w", err),
				"external adapter authorization failed", "solverMode", s.cfg.SolverMode,
				"executor", s.cfg.Executor.Hex(), "adapters", s.cfg.Adapters)
		}
	}
	if err := s.orders.validateExecutorRegistration(ctx, s.cfg.Executor); err != nil {
		return s.startupFailure(err, "executor registration validation failed",
			"executor", s.cfg.Executor.Hex(), "baseUrl", s.cfg.OrderServer.BaseURL)
	}
	if err := s.orders.ensureSupportedContracts(ctx, s.chainID, s.cfg.InputSettler, s.cfg.OutputSettler); err != nil {
		return s.startupFailure(err, "supported contract reconciliation failed", "chainId", s.chainID,
			"inputSettler", s.cfg.InputSettler.Hex(), "outputSettler", s.cfg.OutputSettler.Hex())
	}

	s.log.Info("starting",
		"chainId", s.chainID,
		"strategy", s.cfg.Strategy.Name,
		"adapters", len(s.cfg.Adapters),
		"routes", len(routes),
		"baseUrl", s.cfg.OrderServer.BaseURL,
		"wsUrl", s.cfg.OrderServer.WSURL,
		"quoteRefreshMode", s.cfg.QuoteRefreshMode,
		"quoteInterval", s.cfg.QuoteInterval.String(),
		"quoteTTL", s.cfg.QuoteTTL.String(),
		"solverMode", s.cfg.SolverMode,
		"privateDiscounts", s.cfg.usesDiscounts(),
		"gasAccounting", s.cfg.Gas != nil,
		"tokensToQuote", s.cfg.TokenPolicy.Scope(),
		"executor", s.cfg.Executor.Hex(),
		"caller", s.caller.Hex(),
		"inputSettler", s.cfg.InputSettler.Hex(),
		"outputSettler", s.cfg.OutputSettler.Hex(),
	)

	s.quoteRefresh = make(chan struct{}, 1)
	return s.runLoops(ctx, routes)
}

func (s *Solver) runLoops(ctx context.Context, routes []route) error {
	feedConnections := make(chan context.Context)
	feedCtx, stopFeed := context.WithCancel(context.WithoutCancel(ctx))
	defer stopFeed()
	quoteCtx, stopQuotes := context.WithCancel(ctx)
	defer stopQuotes()

	feedDone := make(chan error, 1)
	quoteDone := make(chan error, 1)
	go func() { feedDone <- s.runOrderFeed(feedCtx, routes, feedConnections) }()
	go func() { quoteDone <- s.quoteLoop(quoteCtx, routes, s.quoteRefresh, feedConnections) }()

	select {
	case quoteErr := <-quoteDone:
		// Keep consuming matched orders until active quotes are expired or the bounded
		// shutdown attempt finishes, then stop intake and await accepted fills until the
		// shared tx manager completes or reaches its finite hard stop.
		stopFeed()
		return preferLifecycleError(quoteErr, <-feedDone)
	case feedErr := <-feedDone:
		// A failed feed cannot consume matches, so stop quote renewal and expire known curves.
		stopQuotes()
		return preferLifecycleError(feedErr, <-quoteDone)
	}
}

func preferLifecycleError(primary, secondary error) error {
	if primary != nil && !errors.Is(primary, context.Canceled) {
		return primary
	}
	if secondary != nil && !errors.Is(secondary, context.Canceled) {
		return secondary
	}
	return primary
}
