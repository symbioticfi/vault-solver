package redstoneoev

import (
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/app"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/policy"
)

func ValidateConfig(raw yaml.Node) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	if err := validatePlannerConfig(cfg.Strategy, cfg.Gas != nil); err != nil {
		return errors.Errorf("strategy: %w", err)
	}
	return nil
}

func Factory(raw yaml.Node, deps app.Services) (app.Integration, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	apiKey := os.Getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		return nil, errors.Errorf("%s: ws api key env %q is empty", Name, cfg.APIKeyEnv)
	}
	chainID := deps.Chain.ChainID()
	if !chainID.IsInt64() || chainID.Sign() <= 0 {
		return nil, errors.Errorf("%s: chain id %s out of supported range", Name, chainID)
	}

	log := deps.Log.WithName(Name)
	var mx *metrics
	if deps.Metrics != nil {
		if mx, err = newMetrics(deps.Metrics.Registerer(), cfg.Strategy.Name); err != nil {
			return nil, err
		}
	}
	reader, err := newReader(deps.Chain, log, cfg.Gas, cfg.LiquidityLens)
	if err != nil {
		return nil, errors.Errorf("%s: gas reader: %w", Name, err)
	}

	s := &Solver{
		cfg:            cfg,
		chainID:        chainID,
		chain:          deps.Chain,
		signer:         deps.Signer,
		reader:         reader,
		nonces:         &nonceStore{},
		breaker:        newBreaker(cfg.BreakerMaxFailures, cfg.BreakerWindow),
		metrics:        mx,
		seen:           newSeenAuctions(maxSeenAuctions),
		stateRefreshCh: make(chan struct{}, 1),
		log:            log,
	}
	planner, facts, err := newPlanner(cfg, policy.FactoryDeps{
		Chain:               deps.Chain,
		Signer:              deps.Signer,
		Log:                 log,
		ChainID:             chainID.Int64(),
		Adapter:             cfg.Adapter,
		Callback:            cfg.Callback,
		LoadAdapterSnapshot: s.adapterSnapshot,
		GasAccounting:       cfg.Gas != nil,
	})
	if err != nil {
		return nil, errors.Errorf("%s: %w", Name, err)
	}
	s.planner = planner
	s.facts = facts
	s.ws = newWSClient(wsConfig{URL: cfg.WSURL, APIKey: apiKey, Topics: wsTopics(cfg.Callback)}, log, s.handleMessage)
	s.ws.onConnected = s.metrics.setFeedConnected
	return s, nil
}

func wsTopics(callback common.Address) []string {
	return []string{"oev/liquidations", "oev/feeds", "oev/notify/" + strings.ToLower(callback.Hex())}
}
