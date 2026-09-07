package txmanager

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/signer"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

type heldSigner struct {
	signer.Signer

	entered chan struct{}
	release chan struct{}
}

func (s *heldSigner) SignTx(ctx context.Context, tx *types.Transaction, id *big.Int) (*types.Transaction, error) {
	close(s.entered)
	<-s.release
	// Deliberately violate the cancellation contract to exercise the worker's final pre-send check.
	return s.Signer.SignTx(context.WithoutCancel(ctx), tx, id)
}

func TestShutdownBoundsInitialSignerAndNeverBroadcastsLate(t *testing.T) {
	backend := newMockBackend()
	key := &heldSigner{Signer: mustSigner(t), entered: make(chan struct{}), release: make(chan struct{})}
	m := New(backend, key, big.NewInt(11155111), Config{ShutdownTimeout: 10 * time.Millisecond}, logr.Discard())
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { m.Start(ctx); close(stopped) }()
	result, accepted := m.SendAsync(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21000})
	if !accepted {
		t.Fatal("request not accepted")
	}
	<-key.entered
	cancel()
	if got := testcheck.ReceiveWithin(t, result, time.Second, "shutdown waited for the blocked initial signer"); !errors.Is(got.Err, errShutdownTimeout) || got.Hash != (common.Hash{}) {
		t.Fatalf("result = %+v", got)
	}
	<-stopped
	close(key.release)
	m.lifecycleWG.Wait()
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.sent) != 0 {
		t.Fatal("broadcast after shutdown")
	}
	select {
	case extra := <-result:
		t.Fatalf("second result: %+v", extra)
	default:
	}
}

func TestEstimateGasRejectsOverflow(t *testing.T) {
	backend := newMockBackend()
	backend.gasEstimate = ^uint64(0)
	manager := newTestManager(t, backend)
	if _, err := manager.estimateGas(context.Background(), Request{}); err == nil {
		t.Fatal("overflowing gas estimate was accepted")
	}
}
