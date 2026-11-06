// Package threef submits native 3F PUSH offers and executes guarded Merkle mints.
package threef

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
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
	offers *offerWorkflow
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
		logMintIteration(ctx, s.keeper.Tick(ctx))

		if s.keeper.Sender == nil || s.keeper.Sender.PendingID() == "" {
			run, cancel := context.WithTimeout(ctx, operationTimeout)
			err := s.offers.Tick(run)
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				observability.Log(ctx).Error(err, "3f offer iteration failed")
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func logMintIteration(ctx context.Context, err error) {
	if err == nil {
		return
	}
	if mintIterationWaiting(err) {
		if errors.Is(err, provider.ErrNotReady) {
			observability.Log(ctx).V(1).Info("3f mint iteration waiting", "reason", err)
		}
		return
	}
	observability.Log(ctx).Error(err, "3f mint iteration failed")
}

func mintIterationWaiting(err error) bool {
	// A keeper batch can join a pending transaction with an independent failure.
	// Every cause must be expected before lowering the whole error's severity.
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if !mintIterationWaiting(cause) {
				return false
			}
		}
		return true
	}
	if cause := errors.Unwrap(err); cause != nil {
		return mintIterationWaiting(cause)
	}
	return errors.Is(err, provider.ErrNotReady) || errors.Is(err, context.Canceled)
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
	address := common.HexToAddress(cfg.Executor)
	identity, err := recipes.DiscoverExecutors(startup, deps.Chain, []common.Address{address})
	if err != nil {
		return nil, errors.Errorf("3f executor identity discovery: %w", err)
	}
	target := identity[address]
	if target.Caller != deps.Signer.Address() {
		return nil, errors.New("3f executor caller differs from the shared signer")
	}
	strategy := keeper.Strategy{Config: keeper.Config{Name: address.Hex(), Executor: address.Hex(), Account: target.Account.Hex(), Interval: pollInterval}}
	strategy.Config.Revision, err = strategyRevision(strategy.Config, cfg, deps.Signer.Address())
	if err != nil {
		return nil, err
	}
	api, err := newClient(cfg.apiConfig())
	if err != nil {
		return nil, err
	}
	policy := common.HexToAddress(cfg.Policy)
	if err := verifyPolicyAccount(startup, deps.Chain, policy, target.Account, nil); err != nil {
		return nil, err
	}
	runtime := &mintProvider{API: api, Chain: deps.Chain, Policy: policy}
	strategy.Providers = []keeper.Provider{runtime}
	source, err := cfg.Curator.PermissionSource(deps.Chain)
	if err != nil {
		return nil, err
	}
	planner := &keeper.Planner{PermissionSource: source, RecipeCache: recipes.NewRecipeCache(1, 16<<20), Chain: deps.Chain, ChainID: deps.Chain.ChainID(), Caller: deps.Signer.Address(), Confirmations: deps.TxManager.Confirmations()}
	k := &keeper.Keeper{Planner: &mintPlanning{Planner: planner, runtime: runtime}, Strategies: []keeper.Strategy{strategy}, Mode: cfg.Mode, OperationTimeout: operationTimeout, Log: deps.Log.WithName(Name)}
	store, err := merklesender.OpenKeeper(k, cfg.StateDir, deps, Name)
	if err != nil {
		return nil, err
	}
	bot := &offerBot{Client: api, Store: store, Send: cfg.Mode == "send"}
	workflow := &offerWorkflow{planner: planner, strategy: strategy, policy: policy, api: api, bot: bot}
	return &Solver{keeper: k, store: store, offers: workflow}, nil
}
