package merklesender

import (
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/state"
	"github.com/symbioticfi/merkle-executor/sdk/submission"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

// OpenKeeper restores the shared sender fence before checking workflow recovery.
// The host must drain the sender before closing the returned store.
func OpenKeeper(k *keeper.Keeper, directory string, deps solver.Deps, name string) (*state.Store, error) {
	store, err := state.Open(directory)
	if err != nil {
		return nil, err
	}
	complete := false
	defer func() {
		if !complete {
			_ = store.Close()
		}
	}()
	k.Store = store
	sender, err := submission.New(deps.Chain, deps.Signer.Address(), deps.Chain.ChainID(), store, submission.Config{Confirmations: deps.TxManager.Confirmations()}, Transport{Manager: deps.TxManager, Solver: name}, deps.Log)
	if err != nil {
		return nil, err
	}
	if err := validateRecoveredMode(k.Mode, sender.PendingID()); err != nil {
		return nil, err
	}
	if k.Mode == "send" {
		k.Sender = sender
	}
	if err := k.CheckRecovery(); err != nil {
		return nil, err
	}
	complete = true
	return store, nil
}

func validateRecoveredMode(mode, pendingID string) error {
	if mode != "send" && pendingID != "" {
		return errors.New("merkle unresolved signed work requires send mode for recovery")
	}
	return nil
}
