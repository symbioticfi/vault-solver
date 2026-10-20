// Package merklesender connects SDK journals to the host's single transaction manager.
package merklesender

import (
	"context"
	"time"

	"github.com/symbioticfi/merkle-executor/sdk/submission"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// This bridge submits through the same manager as every other solver. It owns no
// key, signing engine, nonce worker, RPC broadcaster or independent fee policy.
type Transport struct {
	Manager  *txmanager.Manager
	Solver   string
	Deadline func() time.Time
}

func (t Transport) Hold(id string) error               { return t.Manager.Hold(id) }
func (t Transport) Release(id string) error            { return t.Manager.Release(id) }
func (t Transport) RestoreConfirmedNonce(nonce uint64) { t.Manager.RestoreConfirmedNonce(nonce) }
func (t Transport) SendAsync(ctx context.Context, req submission.Request, persist submission.Persist) (<-chan submission.Outcome, bool) {
	var deadline time.Time
	if t.Deadline != nil {
		deadline = t.Deadline()
	}
	channel, accepted := t.Manager.SendAsync(ctx, txmanager.Request{Owner: req.ID, To: req.To, Data: req.Data, Deadline: deadline, Label: "execute_recipe", Solver: t.Solver, BeforeBroadcast: persist})
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
