package rfq

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// blockingAcceptedTxSender models txmanager.Send after admission: caller cancellation no longer
// abandons the signed lifecycle, and Send returns only once its terminal result is known.
type blockingAcceptedTxSender struct {
	started chan struct{}
	result  chan txmanager.Result
}

func (s *blockingAcceptedTxSender) Send(context.Context, txmanager.Request) txmanager.Result {
	close(s.started)
	return <-s.result
}

func TestShutdownPreparationTimeoutCoversQuoteServerDrain(t *testing.T) {
	s := &Solver{}
	if got, want := s.ShutdownPreparationTimeout(), 5*time.Second; got != want {
		t.Fatalf("ShutdownPreparationTimeout() = %v, want %v", got, want)
	}
}

func TestRunDrainsAcceptedExecutionBeforeReturning(t *testing.T) {
	st, backend := fillFixtures(t)
	// Reconciliation is a later, context-bound backend read. Keep it unavailable here so the
	// assertion pins the bookkeeping performed directly from the accepted txmanager result.
	backend.order = nil
	txm := &blockingAcceptedTxSender{
		started: make(chan struct{}),
		result:  make(chan txmanager.Result, 1),
	}
	exec := newExec(t, st, backend, txm)
	s := &Solver{
		cfg: &Config{
			ListenAddr:   "127.0.0.1:0",
			Executor:     exec.executor,
			PollInterval: time.Hour,
		},
		server: &server{
			sharedSecret: "test-secret",
			quotes:       &quoteService{},
			log:          logr.Discard(),
		},
		exec: exec,
		log:  logr.Discard(),
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	select {
	case <-txm.started:
	case <-time.After(time.Second):
		t.Fatal("RFQ execution did not reach the accepted transaction")
	}

	cancel()
	select {
	case err := <-done:
		t.Fatalf("Run returned before the accepted transaction completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	wantHash := common.HexToHash("0xdead")
	txm.result <- txmanager.Result{Hash: wantHash}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after the accepted transaction completed")
	}

	rec := testOrder(st)
	if rec == nil || rec.Status != statusSubmitted || rec.TxHash != wantHash {
		t.Fatalf("order after shutdown drain = %+v, want submitted with tx %s", rec, wantHash.Hex())
	}
}

func TestRunReportsListenerFailureBeforeDrainingAcceptedExecution(t *testing.T) {
	st, backend := fillFixtures(t)
	backend.order = nil
	txm := &blockingAcceptedTxSender{
		started: make(chan struct{}),
		result:  make(chan txmanager.Result, 1),
	}
	exec := newExec(t, st, backend, txm)

	ctx, reportFatal := context.WithCancelCause(t.Context())
	fatalReported := make(chan error, 1)
	s := &Solver{
		cfg: &Config{
			ListenAddr:   "[::1", // malformed address: ListenAndServe fails before opening a socket
			Executor:     exec.executor,
			PollInterval: time.Hour,
		},
		server: &server{
			sharedSecret: "test-secret",
			quotes:       &quoteService{},
			log:          logr.Discard(),
		},
		exec: exec,
		log:  logr.Discard(),
		reportFatal: func(err error) {
			// A listener can fail before the execution goroutine is scheduled. Wait until Send has
			// definitely reached its accepted, context-independent phase before simulating the
			// process-wide fatal cancellation.
			<-txm.started
			reportFatal(err)
			fatalReported <- err
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	var listenerErr error
	select {
	case listenerErr = <-fatalReported:
	case <-time.After(time.Second):
		t.Fatal("listener failure was not reported after execution reached Send")
	}
	if !errors.Is(context.Cause(ctx), listenerErr) {
		t.Fatalf("fatal cancellation cause = %v, want reported listener error %v", context.Cause(ctx), listenerErr)
	}
	select {
	case err := <-done:
		t.Fatalf("Run returned before draining accepted execution after listener failure: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	wantHash := common.HexToHash("0xbeef")
	txm.result <- txmanager.Result{Hash: wantHash}
	select {
	case err := <-done:
		if !errors.Is(err, listenerErr) {
			t.Fatalf("Run() error = %v, want original reported listener error %v", err, listenerErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after accepted execution completed")
	}

	rec := testOrder(st)
	if rec == nil || rec.Status != statusSubmitted || rec.TxHash != wantHash {
		t.Fatalf("order after listener-failure drain = %+v, want submitted with tx %s", rec, wantHash.Hex())
	}
}
