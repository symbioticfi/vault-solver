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

// receiptSweep belongs to the lifecycle owner. Its fixed size bounds the ordinary
// round-robin pass; priority reads of newly appended attempts do not move that cursor.
type receiptSweep struct {
	size          int
	start         int
	checked       int
	knownAttempts int
	ordinaryDue   bool
	firstError    *receiptRead
}

func newReceiptSweep(pending *pendingTransaction, knownAttempts int) *receiptSweep {
	n := len(pending.attempts)
	if n == 0 {
		return nil
	}
	return &receiptSweep{size: n, start: pending.receiptCursor % n, knownAttempts: knownAttempts}
}

// nextIndex prioritizes the newest signed variant, alternating with ordinary
// reads so repeated replacements cannot starve older hashes. Superseded new
// variants remain in pending.attempts and enter the next ordinary sweep.
func (s *receiptSweep) nextIndex(pending *pendingTransaction) int {
	if len(pending.attempts) > s.knownAttempts && !s.ordinaryDue {
		return len(pending.attempts) - 1
	}
	if s.checked < s.size {
		return (s.start + s.checked) % s.size
	}
	return -1
}

// dispatched advances only after the reader accepts the immutable attempt copy.
func (s *receiptSweep) dispatched(pending *pendingTransaction, index int) {
	if index >= s.knownAttempts {
		s.knownAttempts = index + 1
		s.ordinaryDue = true
		return
	}
	s.checked++
	s.ordinaryDue = false
	pending.receiptCursor = (s.start + s.checked) % s.size
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
