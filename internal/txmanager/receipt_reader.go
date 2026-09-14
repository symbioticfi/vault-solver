package txmanager

import (
	"context"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
)

type receiptRead struct {
	attempt txAttempt
	receipt *types.Receipt
	err     error
}

// receiptReader owns only RPC I/O. The lifecycle goroutine owns attempts, sweep
// progress, logs and outcomes. An unbuffered request channel and one outstanding
// request prevent overlapping reads or a backlog while confirmation is running.
type receiptReader struct {
	requests chan txAttempt
	results  chan receiptRead
	cancel   context.CancelFunc
	done     chan struct{}
}

func (m *Manager) startReceiptReader(ctx context.Context) *receiptReader {
	ctx, cancel := context.WithCancel(ctx)
	r := &receiptReader{
		requests: make(chan txAttempt),
		results:  make(chan receiptRead),
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	go func() {
		defer close(r.done)
		for {
			select {
			case <-ctx.Done():
				return
			case attempt := <-r.requests:
				readCtx, cancelRead := context.WithTimeout(ctx, m.receiptReadTimeout())
				receipt, err := m.backend.TransactionReceipt(readCtx, attempt.hash)
				cancelRead()
				select {
				case r.results <- (receiptRead{attempt: attempt, receipt: receipt, err: err}):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return r
}

func (r *receiptReader) stop() {
	r.cancel()
	<-r.done
}

// A finite snapshot prevents new replacements from extending a sweep forever.
// New attempts enter the next sweep; older attempts are never discarded or starved.
type receiptSweep struct {
	attempts   []txAttempt
	start      int
	checked    int
	firstError *receiptRead
}

func newReceiptSweep(pending *pendingTransaction) *receiptSweep {
	n := len(pending.attempts)
	if n == 0 {
		return nil
	}
	start := pending.receiptCursor % n
	attempts := make([]txAttempt, 0, n)
	attempts = append(attempts, pending.attempts[start:]...)
	attempts = append(attempts, pending.attempts[:start]...)
	return &receiptSweep{attempts: attempts, start: start}
}

// observeReceiptRead reports a validated candidate. Only the lifecycle owner may
// confirm it, change nonce ownership, or deliver a terminal result.
func (m *Manager) observeReceiptRead(pending *pendingTransaction, sweep *receiptSweep, read receiptRead) bool {
	if errors.Is(read.err, ethereum.NotFound) {
		return false
	}
	if read.err != nil {
		if sweep.firstError == nil {
			sweep.firstError = &read
		}
		return false
	}
	if err := validateReceipt(read.attempt.hash, read.receipt); err != nil {
		pending.log.Error(err, "invalid pending transaction receipt", "label", pending.req.Label, "hash", read.attempt.hash.Hex(), "nonce", pending.nonce)
		return false
	}
	m.receiptReadsRecovered(pending)
	return true
}

func (m *Manager) finishReceiptSweep(pending *pendingTransaction, sweep *receiptSweep) {
	// A nonempty sweep without RPC errors clears the transport failure streak.
	// Invalid receipts are logged separately and never complete the lifecycle.
	if sweep.firstError != nil {
		m.receiptReadFailed(pending, sweep.firstError.attempt, sweep.firstError.err)
	} else {
		m.receiptReadsRecovered(pending)
	}
}
