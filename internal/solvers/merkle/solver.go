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
	"github.com/symbioticfi/merkle-executor/sdk/submission"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
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
	source, err := permissions.New(cfg.curator.HTTPConfig(), &http.Client{Timeout: defaultOperationTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
	if err != nil {
		return nil, err
	}
	store, err := state.Open(cfg.stateDir)
	if err != nil {
		return nil, err
	}
	complete := false
	defer func() {
		if !complete {
			_ = store.Close()
		}
	}()
	planner := &keeper.Planner{PermissionSource: source, RecipeCache: recipes.NewRecipeCache(len(cfg.strategies), recipeCacheBytes), Chain: deps.Chain, ChainID: deps.Chain.ChainID(), Caller: deps.Signer.Address(), Confirmations: deps.TxManager.Confirmations()}
	k := &keeper.Keeper{Store: store, Planner: planner, Strategies: cfg.strategies, Mode: cfg.mode, OperationTimeout: defaultOperationTimeout, Log: deps.Log.WithName(Name)}
	sender, err := submission.New(deps.Chain, deps.Signer.Address(), deps.Chain.ChainID(), store, submission.Config{Confirmations: deps.TxManager.Confirmations()}, transport{manager: deps.TxManager}, deps.Log)
	if err != nil {
		return nil, err
	}
	if err := validateRecoveredMode(cfg.mode, sender.PendingID()); err != nil {
		return nil, err
	}
	if cfg.mode == "send" {
		k.Sender = sender
	}
	if err := k.CheckRecovery(); err != nil {
		return nil, err
	}
	complete = true
	return &Solver{keeper: k, store: store}, nil
}

// This bridge submits through the same manager as every other solver. It owns no
// key, signing engine, nonce worker, RPC broadcaster or independent fee policy.
type transport struct{ manager *txmanager.Manager }

func (t transport) Hold(id string) error               { return t.manager.Hold(id) }
func (t transport) Release(id string) error            { return t.manager.Release(id) }
func (t transport) RestoreConfirmedNonce(nonce uint64) { t.manager.RestoreConfirmedNonce(nonce) }
func (t transport) SendAsync(ctx context.Context, req submission.Request, persist submission.Persist) (<-chan submission.Outcome, bool) {
	channel, accepted := t.manager.SendAsync(ctx, txmanager.Request{Owner: req.ID, To: req.To, Data: req.Data, Label: "execute_recipe", Solver: Name, BeforeBroadcast: persist})
	if !accepted {
		return nil, false
	}
	result := make(chan submission.Outcome, 1)
	go func() {
		out := <-channel
		result <- submission.Outcome{Hash: out.Hash, Attempts: out.Attempts, Rejected: out.Outcome == txmanager.OutcomeSubmissionError, Err: out.Err}
	}()
	return result, true
}

func validateRecoveredMode(mode, pendingID string) error {
	if mode != "send" && pendingID != "" {
		return errors.New("merkle unresolved signed work requires send mode for recovery")
	}
	return nil
}
