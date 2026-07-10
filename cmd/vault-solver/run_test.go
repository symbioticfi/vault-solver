package main

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"golang.org/x/sync/errgroup"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// Anvil account #0 — a public throwaway key used only to exercise the production signer boundary.
const runtimeTestKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

type blockingRuntimeBackend struct {
	receiptStarted  chan struct{}
	managerCanceled chan struct{}
	releaseReceipt  chan struct{}
	startOnce       sync.Once
	cancelOnce      sync.Once
}

func (*blockingRuntimeBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return 7, nil
}

func (*blockingRuntimeBackend) SuggestGasTipCap(context.Context) (*big.Int, error) {
	return big.NewInt(1_000_000_000), nil
}

func (*blockingRuntimeBackend) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(100), BaseFee: big.NewInt(20_000_000_000)}, nil
}

func (*blockingRuntimeBackend) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	return 21_000, nil
}

func (*blockingRuntimeBackend) SendTransaction(context.Context, *types.Transaction) error {
	return nil
}

func (b *blockingRuntimeBackend) TransactionReceipt(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
	b.startOnce.Do(func() { close(b.receiptStarted) })
	<-ctx.Done()
	b.cancelOnce.Do(func() { close(b.managerCanceled) })
	<-b.releaseReceipt
	return nil, ctx.Err()
}

func (*blockingRuntimeBackend) BlockNumber(context.Context) (uint64, error) { return 100, nil }

func TestSuperviseRuntime_FatalCancelsManagerAndJoinsEnqueuedSend(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	backend := &blockingRuntimeBackend{
		receiptStarted:  make(chan struct{}),
		managerCanceled: make(chan struct{}),
		releaseReceipt:  make(chan struct{}),
	}
	released := false
	defer func() {
		if !released {
			close(backend.releaseReceipt)
		}
	}()
	testSigner, err := signer.NewFromHexKey(runtimeTestKey)
	if err != nil {
		t.Fatalf("new test signer: %v", err)
	}
	txm := txmanager.New(backend, testSigner, big.NewInt(1), txmanager.Config{
		PollInterval:    time.Millisecond,
		PendingInterval: time.Hour,
		MaxReplacements: 1,
	}, logr.Discard())
	health := &observability.Health{}
	probe := observability.NewHTTPServer("127.0.0.1:0", observability.NewMetrics(), health)
	fatal := solver.NewFatalSignal()
	listenerErr := errors.New("rfq-like listener failed")
	triggerFatal := make(chan struct{})
	listenerReady := make(chan struct{})
	sendResult := make(chan txmanager.Result, 1)
	managerDone := make(chan struct{})
	nestedDone := make(chan struct{})

	managerWorker := func(ctx context.Context) error {
		defer close(managerDone)
		return txm.Start(ctx)
	}
	nestedWorker := func(ctx context.Context) error {
		defer close(nestedDone)
		g, gctx := errgroup.WithContext(ctx)
		g.Go(func() error {
			sendResult <- txm.Send(gctx, txmanager.Request{
				To: common.HexToAddress("0x1234"), GasLimit: 21_000, Label: "rfq-like-fill",
			})
			return nil
		})
		g.Go(func() error {
			select {
			case <-backend.receiptStarted:
				close(listenerReady)
			case <-gctx.Done():
				return errors.Errorf("wait for enqueued fill: %w", gctx.Err())
			}
			select {
			case <-triggerFatal:
				fatalErr := errors.Errorf("rfq-like quote server: %w", listenerErr)
				fatal.Report(fatalErr)
				return fatalErr
			case <-gctx.Done():
				return errors.Errorf("wait to fail listener: %w", gctx.Err())
			}
		})
		return g.Wait()
	}

	runtimeDone := make(chan error, 1)
	go func() {
		runtimeDone <- superviseRuntime(ctx, health, fatal, managerWorker, nestedWorker)
	}()

	select {
	case <-listenerReady:
	case <-time.After(time.Second):
		t.Fatal("fill did not reach manager-owned receipt tracking")
	}
	assertReadinessStatus(t, probe, http.StatusOK)
	close(triggerFatal)
	select {
	case <-backend.managerCanceled:
	case <-time.After(time.Second):
		t.Fatal("fatal listener error did not cancel the root transaction manager")
	}
	assertReadinessStatus(t, probe, http.StatusServiceUnavailable)
	select {
	case err := <-runtimeDone:
		t.Fatalf("runtime returned before the blocked receipt worker joined: %v", err)
	default:
	}
	select {
	case result := <-sendResult:
		t.Fatalf("Send returned before its manager-owned receipt attempt joined: %+v", result)
	default:
	}

	close(backend.releaseReceipt)
	released = true
	var result txmanager.Result
	select {
	case result = <-sendResult:
	case <-time.After(time.Second):
		t.Fatal("enqueued Send did not return after receipt tracking joined")
	}
	if result.State != txmanager.StateUnresolved || result.Hash == (common.Hash{}) || result.SafeToRetry() ||
		!errors.Is(result.Err, txmanager.ErrUnresolved) || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("Send result = %+v, want hashed non-retryable unresolved outcome", result)
	}
	select {
	case err := <-runtimeDone:
		if !errors.Is(err, listenerErr) || !strings.Contains(err.Error(), "runtime component failed") {
			t.Fatalf("runtime error = %v, want wrapped listener fatal", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not join all workers")
	}
	select {
	case <-managerDone:
	default:
		t.Fatal("transaction manager worker was not joined")
	}
	select {
	case <-nestedDone:
	default:
		t.Fatal("nested RFQ-like workers were not joined")
	}
}

func assertReadinessStatus(t *testing.T, srv *http.Server, want int) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
	resp := httptest.NewRecorder()
	srv.Handler.ServeHTTP(resp, req)
	if resp.Code != want {
		t.Fatalf("GET /readyz status = %d, want %d", resp.Code, want)
	}
}
