// Package rfq implements the Symbiotic RFQ quote-and-fill coordinator.
package rfq

import (
	"os"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/app"
	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const (
	Name                       = "rfq-filler"
	quoteServerShutdownTimeout = 5 * time.Second
)

// Solver owns two concurrent lanes: a quote server and one serial execution coordinator.
type Solver struct {
	cfg    *Config
	server *server
	exec   *executionService
	log    logr.Logger
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
		return errors.Errorf("unknown RFQ strategy %q", spec.Name)
	}
}

func newPlanner(spec StrategyConfig) (Planner, error) {
	switch spec.Name {
	case defaultPlannerName:
		return newDefaultPlannerFromConfig(spec.Config)
	case webhookPlannerName:
		return newWebhookPlannerFromConfig(spec.Config)
	default:
		return nil, errors.Errorf("unknown RFQ strategy %q", spec.Name)
	}
}

func Factory(raw yaml.Node, services app.Services) (app.Integration, error) {
	config, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	secret := os.Getenv(config.BackendSharedSecretEnv)
	if secret == "" {
		return nil, errors.Errorf("%s: backend shared secret env %q is empty", Name, config.BackendSharedSecretEnv)
	}
	planner, err := newPlanner(config.Strategy)
	if err != nil {
		return nil, err
	}
	log := services.Log.WithName(Name)
	store := newStore(time.Now)
	reader, err := newReader(services.Chain, log, config.LiquidityLens)
	if err != nil {
		return nil, errors.Errorf("create LiquidLane reader: %w", err)
	}
	var metrics *rfqMetrics
	if services.Metrics != nil {
		metrics, err = newRFQMetrics(services.Metrics.Registerer(), config.Strategy.Name)
		if err != nil {
			return nil, err
		}
	}
	quotes, execution := buildServices(
		config, services.Chain.ChainID().Int64(), store, reader,
		services.TxManager, services.TxManager.LaneReady, planner, services.Capacity, metrics, log,
	)
	return &Solver{
		cfg:    config,
		server: &server{sharedSecret: secret, quotes: quotes, metrics: metrics, log: log},
		exec:   execution,
		log:    log,
	}, nil
}

func buildServices(
	config *Config,
	chainID int64,
	store *store,
	reader *reader,
	txManager txSender,
	laneReady func() bool,
	planner Planner,
	capacityLedger *capacity.Book,
	metrics *rfqMetrics,
	log logr.Logger,
) (*quoteService, *executionService) {
	quoteWhitelist := buildAdapterWhitelist(config.quoteScopesToAdapters(), config.Adapters)
	executionWhitelist := buildAdapterWhitelist(config.restrictsToAdapters(), config.Adapters)
	quotes := &quoteService{
		chainID: chainID, executor: config.Executor, laneReady: laneReady,
		whitelist: quoteWhitelist, tokenPolicy: config.TokenPolicy, minAmountsIn: config.MinAmountsIn,
		reader: reader, planner: planner, capacity: capacityLedger, metrics: metrics, log: log, now: time.Now,
	}
	execution := &executionService{
		chainID: chainID, executor: config.Executor,
		orderLimit: config.OrderLimit, vaults: config.Adapters,
		whitelist: executionWhitelist, tokenPolicy: config.TokenPolicy, discountsEnabled: config.usesDiscounts(),
		backend: newBackendClient(config.BackendURL), store: store, reader: reader, planner: planner,
		txm: txManager, capacity: capacityLedger, metrics: metrics, log: log, now: time.Now,
	}
	return quotes, execution
}

func (solver *Solver) Name() string                              { return Name }
func (solver *Solver) ShutdownPreparationTimeout() time.Duration { return quoteServerShutdownTimeout }
