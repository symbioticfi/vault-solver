package filler

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// AwaitNative waits for the pending fill and reports it in the native transaction result shape,
// so a solver that keeps its adapter-era order lifecycle shares retry and outcome accounting
// between both lanes. A stopped lane ends tracking like a stopped native manager. Any other wait
// that ends without canonical evidence is nonce-uncertain: the lane may still own the
// transaction, and fresh order state decides what happens next.
func (e *Engine) AwaitNative(ctx context.Context) txmanager.Result {
	outcome, err := e.Await(ctx)
	switch {
	case errors.Is(err, ErrLaneStopped):
		return txmanager.Result{Outcome: txmanager.OutcomeTrackingStopped, Err: err}
	case err != nil:
		return txmanager.Result{Outcome: txmanager.OutcomeAbandoned, Err: err}
	case outcome == nil:
		return txmanager.Result{Outcome: txmanager.OutcomeAbandoned, Err: errors.New("fill lane lost its transaction: no fill was pending when its receipt was awaited")}
	case outcome.Reverted:
		return txmanager.Result{Hash: outcome.Hash, Attempts: []common.Hash{outcome.Hash}, Outcome: txmanager.OutcomeReverted,
			Err: errors.Errorf("fill transaction %s reverted in block %d", outcome.Hash.Hex(), outcome.Block)}
	default:
		return txmanager.Result{Hash: outcome.Hash, Attempts: []common.Hash{outcome.Hash}, Outcome: txmanager.OutcomeConfirmed}
	}
}
