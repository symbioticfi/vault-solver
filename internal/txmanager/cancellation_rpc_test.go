package txmanager

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

type cancellationRoutingBackend struct {
	*mockBackend

	cancellations []*types.Transaction
	cancelErr     error
}

func (b *cancellationRoutingBackend) SendCancellationTransaction(_ context.Context, tx *types.Transaction) error {
	b.cancellations = append(b.cancellations, tx)
	return b.cancelErr
}

func TestCancellationUsesDedicatedSenderForReplacementAndRebroadcast(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "accepted"},
		{name: "endpoint unavailable", err: errors.New("cancellation endpoint unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &cancellationRoutingBackend{mockBackend: newMockBackend(), cancelErr: tc.err}
			s := mustSigner(t)
			m := New(backend, s, big.NewInt(11155111), Config{MaxFeeGwei: 100}, logr.Discard())
			pending, err := m.broadcast(t.Context(), Request{
				To: common.HexToAddress("0xabc"), Data: []byte{0x01}, GasLimit: 21_000, Label: "fill",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.tryReplace(t.Context(), pending, false); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				cancelling, err := m.tryReplace(t.Context(), pending, true)
				if !cancelling || !errors.Is(err, tc.err) {
					t.Fatalf("cancellation = %v, error = %v; want cancellation and %v", cancelling, err, tc.err)
				}
			}
			if !m.rebroadcastLatestAttempt(t.Context(), pending, true) {
				t.Fatal("cancellation was not rebroadcast")
			}
			if len(backend.cancellations) != 3 {
				t.Fatalf("dedicated cancellation sends = %d, want initial cancellation, replacement, and rebroadcast", len(backend.cancellations))
			}
			for _, tx := range backend.cancellations {
				if tx.Nonce() != pending.nonce || tx.To() == nil || *tx.To() != s.Address() ||
					len(tx.Data()) != 0 || tx.Value().Sign() != 0 || tx.Gas() != cancellationGasLimit {
					t.Fatalf("cancellation endpoint received a non-cancellation transaction: %v", tx)
				}
			}
			if backend.cancellations[0].Hash() == backend.cancellations[1].Hash() ||
				backend.cancellations[1].Hash() != backend.cancellations[2].Hash() {
				t.Fatal("expected fee-bumped cancellation followed by its exact rebroadcast")
			}
			if got := backend.attemptedTransactions(); len(got) != 2 {
				t.Fatalf("ordinary write sends = %d, want original fill and fee replacement only", len(got))
			}
		})
	}
}
