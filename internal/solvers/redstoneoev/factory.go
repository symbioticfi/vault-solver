package redstoneoev

import (
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies"
)

func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	apiKey := os.Getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		return nil, errors.Errorf("%s: ws api key env %q is empty", Name, cfg.APIKeyEnv)
	}
	// Dry-run is solver-owned because it suppresses outbound solve frames for every strategy.
	dryRun, err := dryRunEnv()
	if err != nil {
		return nil, errors.Errorf("%s: %w", Name, err)
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

	s := &Solver{
		cfg:            cfg,
		deps:           deps,
		chainID:        chainID,
		dryRun:         dryRun,
		strategyName:   cfg.Strategy.Name,
		reader:         newReader(deps.Chain, log, cfg.LiquidityLens),
		nonces:         &nonceStore{},
		breaker:        newBreaker(cfg.BreakerMaxFailures, cfg.BreakerWindow),
		metrics:        mx,
		seen:           newSeenAuctions(maxSeenAuctions),
		stateRefreshCh: make(chan struct{}, 1),
		log:            log,
	}
	strategy, err := newStrategy(cfg, strategies.Deps{
		Chain:               deps.Chain,
		Signer:              deps.Signer,
		Log:                 log,
		ChainID:             chainID.Int64(),
		Adapter:             cfg.Adapter,
		Callback:            cfg.Callback,
		LoadAdapterSnapshot: s.adapterSnapshot,
	})
	if err != nil {
		return nil, errors.Errorf("%s: %w", Name, err)
	}
	s.strategy = strategy
	s.ws = newWSClient(wsConfig{URL: cfg.WSURL, APIKey: apiKey, Topics: wsTopics(cfg.Callback)}, log, s.handleMessage)
	return s, nil
}

func wsTopics(callback common.Address) []string {
	return []string{"oev/liquidations", "oev/feeds", "oev/notify/" + strings.ToLower(callback.Hex())}
}
