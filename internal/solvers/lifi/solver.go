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
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const Name = "lifi-samechain"

const lifiOrderStatusDeposited uint8 = 1

//nolint:gochecknoinits // self-registration with the solver framework is the intended plugin pattern.
func init() {
	solver.Register(Name, factory)
}

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
	capacity     liquidlane.CapacityLedger
	quoteRefresh chan struct{}
	discounts    discounts.Provider
}

type chainReader interface {
	resolveRoutes(ctx context.Context, adapters []common.Address) ([]route, error)
	validateExecutor(
		ctx context.Context,
		executor, inputSettler, outputSettler, caller common.Address,
	) error
	validateZeroGovernanceFee(ctx context.Context, inputSettler common.Address) error
	validateDirectAuthorization(ctx context.Context, executor common.Address, routes []route) error
	validateGasTokens(routes []route) error
	quoteSnapshots(ctx context.Context, routes []route, executor common.Address, chainTime time.Time) (quoteSnapshotSet, error)
	fillSnapshots(
		ctx context.Context, routes []route, executor, tokenIn common.Address, amountIn *big.Int, chainTime time.Time,
	) (fillSnapshotSet, error)
	orderIdentifier(ctx context.Context, inputSettler common.Address, order inputsettler.StandardOrder) (common.Hash, error)
	orderStatus(ctx context.Context, inputSettler common.Address, orderID common.Hash) (uint8, error)
	latestBlockNumber(ctx context.Context) (uint64, error)
	latestBlockTime(ctx context.Context) (time.Time, error)
}

type txSender interface {
	SendAsync(ctx context.Context, req txmanager.Request) (<-chan txmanager.Result, bool)
}

func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
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
	}
	if cfg.usesDiscounts() {
		result.discounts = discounts.NewClient(cfg.DiscountsURL)
	}
	return result, nil
}

func (s *Solver) Name() string { return Name }

func (s *Solver) Run(ctx context.Context) error {
	routes, err := s.reader.resolveRoutes(ctx, s.cfg.Adapters)
	if err != nil {
		startupErr := errors.Errorf("lifi: resolve routes: %w", err)
		s.log.Error(startupErr, "adapter resolution failed",
			"solverMode", s.cfg.SolverMode, "executor", s.cfg.Executor.Hex(), "adapters", s.cfg.Adapters)
		return startupErr
	}
	if len(routes) == 0 {
		startupErr := errors.New("lifi: no quoteable routes resolved from configured adapters")
		s.log.Error(startupErr, "adapter resolution failed",
			"solverMode", s.cfg.SolverMode, "executor", s.cfg.Executor.Hex(), "adapters", s.cfg.Adapters)
		return startupErr
	}
	if err := s.reader.validateGasTokens(routes); err != nil {
		return errors.Errorf("lifi: validate gas oracles: %w", err)
	}
	if err := s.reader.validateExecutor(
		ctx, s.cfg.Executor, s.cfg.InputSettler, s.cfg.OutputSettler, s.caller,
	); err != nil {
		startupErr := errors.Errorf("lifi: validate executor: %w", err)
		s.log.Error(startupErr, "executor validation failed",
			"executor", s.cfg.Executor.Hex(), "caller", s.caller.Hex(),
			"inputSettler", s.cfg.InputSettler.Hex(), "outputSettler", s.cfg.OutputSettler.Hex())
		return startupErr
	}
	if err := s.reader.validateZeroGovernanceFee(ctx, s.cfg.InputSettler); err != nil {
		return errors.Errorf("lifi: validate governance fee: %w", err)
	}
	if !s.cfg.usesDiscounts() {
		if err := s.reader.validateDirectAuthorization(ctx, s.cfg.Executor, routes); err != nil {
			startupErr := errors.Errorf("lifi: validate direct authorization: %w", err)
			s.log.Error(startupErr, "external adapter authorization failed",
				"solverMode", s.cfg.SolverMode,
				"executor", s.cfg.Executor.Hex(),
				"adapters", s.cfg.Adapters,
			)
			return startupErr
		}
	}
	if err := s.orders.validateExecutorRegistration(ctx, s.cfg.Executor); err != nil {
		return err
	}
	if err := s.orders.ensureSupportedContracts(ctx, s.chainID, s.cfg.InputSettler, s.cfg.OutputSettler); err != nil {
		return err
	}

	s.log.Info("starting",
		"routes", len(routes),
		"baseUrl", s.cfg.OrderServer.BaseURL,
		"wsUrl", s.cfg.OrderServer.WSURL,
		"quoteRefreshMode", s.cfg.QuoteRefreshMode,
		"quoteInterval", s.cfg.QuoteInterval.String(),
		"quoteTTL", s.cfg.QuoteTTL.String(),
		"solverMode", s.cfg.SolverMode,
		"tokensToQuote", s.cfg.TokenPolicy.Scope(),
		"executor", s.cfg.Executor.Hex(),
		"caller", s.caller.Hex(),
	)

	s.quoteRefresh = make(chan struct{}, 1)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return s.quoteLoop(gctx, routes, s.quoteRefresh) })
	g.Go(func() error { return s.runOrderFeed(gctx, routes) })
	return g.Wait()
}
