// Package lifi implements the LI.FI same-chain intent coordinator.
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
	"github.com/symbioticfi/vault-solver/internal/app"
	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const Name = "lifi-samechain"

const (
	lifiOrderStatusNone uint8 = iota
	lifiOrderStatusDeposited
	lifiOrderStatusClaimed
	lifiOrderStatusRefunded
)

type chainReader interface {
	ResolveRoutes(ctx context.Context, adapters []common.Address) ([]route, error)
	ValidateGasTokens(routes []route) error
	Quote(ctx context.Context, routes []route, executor common.Address, chainTime time.Time) (quoteSnapshotSet, error)
	Fill(
		ctx context.Context, routes []route, executor, tokenIn common.Address, amountIn *big.Int, chainTime time.Time,
	) (fillSnapshotSet, error)
	validateExecutor(ctx context.Context, executor, inputSettler, outputSettler, caller common.Address) error
	validateZeroGovernanceFee(ctx context.Context, inputSettler common.Address) error
	validateDirectAuthorization(ctx context.Context, executor common.Address, routes []route) error
	orderIdentifier(
		ctx context.Context, inputSettler common.Address, order inputsettler.StandardOrder,
	) (common.Hash, error)
	orderStatus(ctx context.Context, inputSettler common.Address, orderID common.Hash) (uint8, error)
	latestBlockNumber(ctx context.Context) (uint64, error)
}

type txSender interface {
	Send(ctx context.Context, request txmanager.Request) txmanager.Result
}

type transactionLaneState interface {
	LaneReady() bool
	SubscribeLaneState() (<-chan struct{}, func())
}

// Solver coordinates quote publication, matched-order intake, serial tx admission, and capacity ownership.
type Solver struct {
	cfg          *Config
	chainID      int64
	reader       chainReader
	planner      Planner
	caller       common.Address
	orders       *orderClient
	feed         *orderFeed
	txm          txSender
	log          logr.Logger
	now          func(context.Context) (time.Time, error)
	maxFeePerGas func(context.Context) (*big.Int, error)
	wallNow      func() time.Time
	txLaneState  transactionLaneState
	capacity     *capacity.Book
	quoteRefresh chan struct{}
	discounts    discounts.Provider
	metrics      *lifiMetrics
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
		return errors.Errorf("unknown LI.FI strategy %q", spec.Name)
	}
}

func newPlanner(spec StrategyConfig) (Planner, error) {
	switch spec.Name {
	case defaultPlannerName:
		return newDefaultPlannerFromConfig(spec.Config)
	case webhookPlannerName:
		return newWebhookPlannerFromConfig(spec.Config)
	default:
		return nil, errors.Errorf("unknown LI.FI strategy %q", spec.Name)
	}
}

func Factory(raw yaml.Node, services app.Services) (app.Integration, error) {
	config, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	apiKey := os.Getenv(config.OrderServer.APIKeyEnv)
	if apiKey == "" {
		return nil, errors.Errorf("%s: order server api key env %q is empty", Name, config.OrderServer.APIKeyEnv)
	}
	log := services.Log.WithName(Name)
	planner, err := newPlanner(config.Strategy)
	if err != nil {
		return nil, err
	}
	reader, err := newReader(services.Chain, log, config.Gas, config.LiquidityLens)
	if err != nil {
		return nil, err
	}
	chainID := services.Chain.ChainID().Int64()
	coordinator := &Solver{
		cfg: config, chainID: chainID, reader: reader, planner: planner, caller: services.Signer.Address(),
		orders: newOrderClient(config.OrderServer.BaseURL, apiKey, config.OrderServer.HTTPTimeout, chainID),
		feed:   newOrderFeed(config.OrderServer.WSURL, apiKey, log), txm: services.TxManager, log: log,
		now: reader.latestBlockTime, maxFeePerGas: services.TxManager.MaxFeePerGas,
		wallNow: time.Now, txLaneState: services.TxManager, capacity: services.Capacity,
	}
	if config.usesDiscounts() {
		coordinator.discounts = discounts.NewClient(config.DiscountsURL)
	}
	if services.Metrics != nil {
		coordinator.metrics, err = newLIFIMetrics(services.Metrics.Registerer(), config.Strategy.Name)
		if err != nil {
			return nil, err
		}
	}
	return coordinator, nil
}

func (solver *Solver) Name() string { return Name }
func (solver *Solver) ShutdownPreparationTimeout() time.Duration {
	return 2 * solver.cfg.OrderServer.HTTPTimeout
}
