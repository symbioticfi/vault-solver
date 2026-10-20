// Package threef submits native 3F PUSH offers and executes guarded Merkle mints.
package threef

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/state"
	"github.com/symbioticfi/vault-solver/internal/merklesender"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

const Name = "threef-merkle"

//nolint:gochecknoinits // Self-registration is the solver framework's plugin contract.
func init() { solver.Register(Name, factory) }

type Solver struct {
	keeper *keeper.Keeper
	store  *state.Store
	offers []*offerWorkflow
}

func (s *Solver) Name() string            { return Name }
func (s *Solver) RequiresTxManager() bool { return s.keeper.Mode == "send" }

// Close is called by the host after its shared sender has drained, so persistence
// callbacks cannot outlive their store. It also handles partial startup failure.
func (s *Solver) Close() error { return s.store.Close() }
func (s *Solver) Run(ctx context.Context) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.keeper.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
			observability.Log(ctx).Error(err, "3f mint iteration failed")
		}

		if s.keeper.Sender == nil || s.keeper.Sender.PendingID() == "" {
			for _, workflow := range s.offers {
				run, cancel := context.WithTimeout(ctx, operationTimeout)
				err := workflow.Tick(run)
				cancel()
				if err != nil && !errors.Is(err, context.Canceled) {
					observability.Log(ctx).Error(err, "3f offer iteration failed", "executor", workflow.strategy.Config.Executor)
				}
			}
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
		return nil, errors.New("threef-merkle requires host chain, signer and shared transaction manager")
	}
	// The factory interface has no context; bound the complete startup observation.
	startup, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	if deps.Chain.ChainID().Cmp(common.Big1) != 0 {
		return nil, errors.New("3f Merkle PUSH requires mainnet")
	}
	executors := make([]common.Address, len(cfg.Executors))
	for i, text := range cfg.Executors {
		executors[i] = common.HexToAddress(text)
	}
	identity, err := recipes.DiscoverExecutors(startup, deps.Chain, executors)
	if err != nil {
		return nil, errors.Errorf("3f executor identity discovery: %w", err)
	}
	strategies := make([]keeper.Strategy, 0, len(executors))
	var account common.Address
	for _, address := range executors {
		target := identity[address]
		if target.Caller != deps.Signer.Address() {
			return nil, errors.New("3f executor caller differs from the shared signer")
		}
		if account != (common.Address{}) && target.Account != account {
			return nil, errors.New("3f executors must share one default maker account")
		}
		account = target.Account
		strategy := keeper.Strategy{Config: keeper.Config{Name: address.Hex(), Executor: address.Hex(), Account: target.Account.Hex(), Interval: pollInterval}}
		strategy.Config.Revision, err = strategyRevision(strategy.Config, cfg, deps.Signer.Address())
		if err != nil {
			return nil, err
		}
		strategies = append(strategies, strategy)
	}
	api, err := newClient(cfg.apiConfig())
	if err != nil {
		return nil, err
	}
	runtime := &mintProvider{API: api, Chain: deps.Chain}
	for i := range strategies {
		strategies[i].Providers = []provider.Provider{runtime}
	}
	source, err := permissions.New(cfg.Curator.httpConfig(), &http.Client{Timeout: operationTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, deps.Chain, os.Getenv)
	if err != nil {
		return nil, err
	}
	planner := &keeper.Planner{PermissionSource: source, RecipeCache: recipes.NewRecipeCache(len(strategies), 16<<20), Chain: deps.Chain, ChainID: deps.Chain.ChainID(), Caller: deps.Signer.Address(), Confirmations: deps.TxManager.Confirmations()}
	k := &keeper.Keeper{Planner: &mintPlanning{Planner: planner, runtime: runtime}, Strategies: strategies, Mode: cfg.Mode, OperationTimeout: operationTimeout, Log: deps.Log.WithName(Name)}
	store, err := merklesender.OpenKeeper(k, cfg.StateDir, deps, Name)
	if err != nil {
		return nil, err
	}
	bot := &offerBot{Client: api, Store: store, Send: cfg.Mode == "send"}
	workflows := make([]*offerWorkflow, 0, len(strategies))
	for _, strategy := range strategies {
		workflows = append(workflows, &offerWorkflow{planner: planner, strategy: strategy, api: api, bot: bot})
	}
	return &Solver{keeper: k, store: store, offers: workflows}, nil
}
