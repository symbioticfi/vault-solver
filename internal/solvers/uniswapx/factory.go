// Package uniswapx implements UniswapX RFQ and public-order filling through LiquidLane.
package uniswapx

import (
	"context"
	"math/big"
	"os"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/app"
	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const (
	Name               = "uniswapx-filler"
	orderQueueCapacity = 256
)

type chainReader interface {
	ResolveRoutes(ctx context.Context, adapters []common.Address) ([]liquidlane.Route, error)
	validateExecutorCode(ctx context.Context, executor common.Address) error
	validateExecutorCaller(ctx context.Context, executor, caller common.Address) error
	unauthorizedAdapters(
		ctx context.Context, executor common.Address, routes []liquidlane.Route,
	) ([]common.Address, error)
	ValidateGasTokens(routes []liquidlane.Route) error
	Quote(ctx context.Context, routes []liquidlane.Route, executor common.Address, now time.Time) (snapshot, error)
	Fill(
		ctx context.Context, routes []liquidlane.Route, executor, tokenIn common.Address,
		amountIn *big.Int, now time.Time,
	) (fillSnapshot, error)
	ReadFillQuotes(
		ctx context.Context, routes []liquidlane.Route, tokenIn common.Address, amountIn *big.Int,
	) ([]liquidlane.FillQuote, error)
	latestBlockTime(ctx context.Context) (time.Time, error)
	transactionBlockTimeConfirmed(
		ctx context.Context, txHash common.Hash, confirmations uint64,
	) (time.Time, error)
}

type orderPoller interface {
	openOrders(ctx context.Context, chainID int64, filler *common.Address) ([]orderEntry, error)
	recentOrders(
		ctx context.Context, chainID int64, filler common.Address, createdAfter time.Time,
	) ([]orderEntry, error)
	ordersByHash(
		ctx context.Context, chainID int64, hashes []common.Hash,
	) (map[common.Hash]orderTerminal, error)
}

type transactionManager interface {
	MaxFeePerGas(ctx context.Context) (*big.Int, error)
	Send(ctx context.Context, request txmanager.Request) txmanager.Result
	LaneReady() bool
	Available() bool
}

type contractCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// Solver owns solver-wide quote state and one unified execution/exclusivity lifecycle per order hash.
type Solver struct {
	quoteRuntime

	ledger  orderLedger
	breaker fillBreaker

	cfg           *Config
	chainID       int64
	solverAddress common.Address
	chain         contractCaller
	reader        chainReader
	planner       Planner
	txm           transactionManager
	confirmations uint64
	orders        orderPoller
	discounts     liquiddiscounts.Provider
	log           logr.Logger
	capacity      *capacity.Book
	metrics       *uniswapXMetrics
}

func ValidateConfig(raw yaml.Node) error {
	config, err := parseConfig(raw)
	if err != nil {
		return err
	}
	if err := validatePlannerConfig(config.Strategy); err != nil {
		return errors.Errorf("strategy: %w", err)
	}
	return nil
}

func validatePlannerConfig(spec StrategyConfig) error {
	switch spec.Name {
	case defaultPlannerName:
		return validateDefaultPlannerConfig(spec.Config)
	case webhookPlannerName:
		return webhook.ValidateConfig(spec.Config)
	default:
		return errors.Errorf("unknown UniswapX strategy %q", spec.Name)
	}
}

func newPlanner(spec StrategyConfig) (Planner, error) {
	switch spec.Name {
	case defaultPlannerName:
		return newDefaultPlannerFromConfig(spec.Config)
	case webhookPlannerName:
		return newWebhookPlannerFromConfig(spec.Config)
	default:
		return nil, errors.Errorf("unknown UniswapX strategy %q", spec.Name)
	}
}

func Factory(raw yaml.Node, services app.Services) (app.Integration, error) {
	config, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	orderKey := os.Getenv(config.OrderServer.APIKeyEnv)
	if orderKey == "" {
		return nil, errors.New("UniswapX order API key env must be non-empty")
	}
	log := services.Log.WithName(Name)
	reader, err := newReader(services.Chain, log, config.Gas, config.LiquidityLens)
	if err != nil {
		return nil, err
	}
	planner, err := newPlanner(config.Strategy)
	if err != nil {
		return nil, err
	}
	var discountClient liquiddiscounts.Provider
	if config.usesDiscounts() {
		discountClient = liquiddiscounts.NewClient(config.Discounts.BaseURL)
	}
	coordinator := &Solver{
		cfg: config, chainID: services.Chain.ChainID().Int64(), solverAddress: services.Signer.Address(),
		chain: services.Chain, reader: reader, planner: planner, txm: services.TxManager,
		confirmations: services.TxManager.Confirmations(), orders: newOrderClient(config.OrderServer, orderKey),
		discounts: discountClient, log: log,
		quoteRuntime: quoteRuntime{refreshCh: make(chan struct{}, 1)},
		ledger:       orderLedger{records: make(map[common.Hash]orderLifecycle)},
		capacity:     services.Capacity,
	}
	if services.Metrics != nil {
		coordinator.metrics, err = newUniswapXMetrics(
			services.Metrics.Registerer(), coordinator.ready, config.Strategy.Name,
		)
		if err != nil {
			return nil, err
		}
	}
	return coordinator, nil
}

func (solver *Solver) Name() string { return Name }

func (solver *Solver) startupFailure(err error, message string, keysAndValues ...any) error {
	solver.log.Error(err, message, keysAndValues...)
	return err
}
