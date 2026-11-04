package merklesender

import (
	"bytes"
	"context"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/merkle-executor/sdk/submission"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// Sender adds exact-byte rebroadcast to the SDK's receipt-only recovery.
// Callers serialize Send and Acknowledge, as with the SDK manager.
type Sender struct {
	*submission.Manager

	backend     submission.Backend
	transport   Transport
	recoveredID string
	attempts    []*types.Transaction
}

func New(backend submission.Backend, address common.Address, chainID *big.Int, store submission.Store, cfg submission.Config, transport Transport, log logr.Logger) (*Sender, error) {
	manager, err := submission.New(backend, address, chainID, store, cfg, transport, log)
	if err != nil {
		return nil, err
	}
	s := &Sender{Manager: manager, backend: backend, transport: transport}
	// The SDK owns and has validated this journal. Read only the signed attempts;
	// TestSDKJournalSharedSenderReplacementRestartAndAcknowledge pins the layout.
	var saved struct {
		ID       string          `json:"id"`
		Raw      hexutil.Bytes   `json:"raw"`
		Attempts []hexutil.Bytes `json:"attempts"`
		Terminal string          `json:"terminal"`
	}
	found, err := store.Load("txmanager.active.v1", &saved)
	if err != nil {
		return nil, err
	}
	if saved.ID != manager.PendingID() || (found && saved.ID == "") {
		return nil, errors.New("SDK journal recovery identity mismatch")
	}
	if found && saved.Terminal == "" {
		s.recoveredID = saved.ID
		for _, raw := range append([]hexutil.Bytes{saved.Raw}, saved.Attempts...) {
			var tx types.Transaction
			if err := tx.UnmarshalBinary(raw); err != nil {
				return nil, err
			}
			s.attempts = append(s.attempts, &tx)
		}
	}
	return s, nil
}

func (s *Sender) Send(ctx context.Context, req submission.Request) (submission.Result, error) {
	if s.recoveredID != "" && s.PendingID() == s.recoveredID && req.ID == s.recoveredID {
		last := s.attempts[len(s.attempts)-1]
		if last.To() == nil || *last.To() != req.To || !bytes.Equal(last.Data(), req.Data) {
			return submission.Result{ID: s.PendingID(), Pending: true}, submission.ErrLaneBusy
		}
		// Rebroadcast only if no attempt has a receipt. An unreadable receipt
		// forbids it, but the SDK still sweeps the others for a canonical one.
		missing := true
		for _, tx := range s.attempts {
			receipt, err := s.backend.TransactionReceipt(ctx, tx.Hash())
			if receipt != nil || (err != nil && !errors.Is(err, ethereum.NotFound)) {
				missing = false
			}
		}
		if missing {
			if err := s.transport.Manager.Rebroadcast(ctx, req.ID, last); err != nil {
				// Not proof of execution: keep the journal and retry on a later tick.
				observability.Log(ctx).V(1).Info("journal rebroadcast unavailable", "id", req.ID, "error", err)
			}
		}
	}
	return s.Manager.Send(ctx, req)
}

func (s *Sender) Acknowledge(ctx context.Context, id string) error {
	if err := s.Manager.Acknowledge(ctx, id); err != nil {
		return err
	}
	if id == s.recoveredID {
		s.recoveredID = ""
		s.attempts = nil
	}
	return nil
}
