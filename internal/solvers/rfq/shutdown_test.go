package rfq

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
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

	testcheck.ReceiveWithin(t, txm.started, time.Second, "RFQ execution did not reach the accepted transaction")

	cancel()
	select {
	case err := <-done:
		t.Fatalf("Run returned before the accepted transaction completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	wantHash := common.HexToHash("0xdead")
	txm.result <- txmanager.Result{Hash: wantHash, Outcome: txmanager.OutcomeConfirmed}
	if err := testcheck.ReceiveWithin(t, done, time.Second, "Run did not return after the accepted transaction completed"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context cancellation", err)
	}

	rec := orderFixture(st)
	if rec == nil || rec.Status != statusSubmitted || rec.TxHash != wantHash {
		t.Fatalf("order after shutdown drain = %+v, want submitted with tx %s", rec, wantHash.Hex())
	}
}

// Listener binding is a startup precondition: an unavailable quote endpoint
// must not race an order fill into the transaction manager.
func TestRunListenerFailureDoesNotStartExecution(t *testing.T) {
	st, backend := fillFixtures(t)
	txm := &blockingAcceptedTxSender{started: make(chan struct{}), result: make(chan txmanager.Result, 1)}
	exec := newExec(t, st, backend, txm)
	s := &Solver{
		cfg:    &Config{ListenAddr: "[::1", Executor: exec.executor, PollInterval: time.Hour},
		server: &server{sharedSecret: "test-secret", quotes: &quoteService{}, log: logr.Discard()},
		exec:   exec, log: logr.Discard(),
	}
	if err := s.Run(t.Context()); err == nil {
		t.Fatal("expected listener failure")
	}
	select {
	case <-txm.started:
		t.Fatal("execution started without a listener")
	default:
	}
	if rec := orderFixture(st); rec != nil {
		t.Fatalf("listener failure changed order: %+v", rec)
	}
}
