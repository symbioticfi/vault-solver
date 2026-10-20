// Package merkle hosts the Merkle SDK using vault-solver's existing shared sender.
package merkle

import (
	"context"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/state"
	"github.com/symbioticfi/vault-solver/internal/merklesender"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

const Name = "merkle"

//nolint:gochecknoinits // Self-registration is the solver framework's plugin contract.
func init() { solver.Register(Name, factory) }

type Solver struct {
	keeper *keeper.Keeper
	store  *state.Store
}

func (s *Solver) Name() string            { return Name }
func (s *Solver) RequiresTxManager() bool { return s.keeper.Mode == "send" }

// Close is called by the host after its shared sender has drained, so persistence
// callbacks cannot outlive their store. It also handles partial startup failure.
func (s *Solver) Close() error { return s.store.Close() }
func (s *Solver) Run(ctx context.Context) error {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.keeper.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
			observability.Log(ctx).Error(err, "merkle workflow iteration failed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	if deps.Chain == nil || deps.Signer == nil || deps.TxManager == nil {
		return nil, errors.New("merkle requires host chain, signer and shared transaction manager")
	}
	// The factory interface has no context; bound the complete startup observation.
	startup, cancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer cancel()
	targets := make([]common.Address, len(cfg.strategies))
	for i, strategy := range cfg.strategies {
		targets[i] = common.HexToAddress(strategy.Config.Executor)
	}
	identities, err := recipes.DiscoverExecutors(startup, deps.Chain, targets)
	if err != nil {
		return nil, errors.Errorf("merkle executor identity discovery: %w", err)
	}
	for i := range cfg.strategies {
		strategy := &cfg.strategies[i]
		identity := identities[common.HexToAddress(strategy.Config.Executor)]
		if identity.Caller != deps.Signer.Address() {
			return nil, errors.New("merkle executor caller differs from the shared signer")
		}
		strategy.Config.Account = identity.Account.Hex()
		strategy.Config.Revision, err = strategyRevision(strategy.Config, cfg.curator, deps.Signer.Address())
		if err != nil {
			return nil, err
		}
	}
	source, err := permissions.New(cfg.curator.HTTPConfig(), &http.Client{Timeout: defaultOperationTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, deps.Chain)
	if err != nil {
		return nil, err
	}
	planner := &keeper.Planner{PermissionSource: source, RecipeCache: recipes.NewRecipeCache(len(cfg.strategies), recipeCacheBytes), Chain: deps.Chain, ChainID: deps.Chain.ChainID(), Caller: deps.Signer.Address(), Confirmations: deps.TxManager.Confirmations()}
	k := &keeper.Keeper{Planner: planner, Strategies: cfg.strategies, Mode: cfg.mode, OperationTimeout: defaultOperationTimeout, Log: deps.Log.WithName(Name)}
	store, err := merklesender.OpenKeeper(k, cfg.stateDir, deps, Name)
	if err != nil {
		return nil, err
	}
	return &Solver{keeper: k, store: store}, nil
}
