package txmanager

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

// Measurements belong to the lifecycle owner, independently of the scheduling cursor.
// A priority read can revisit a hash in the ordinary sweep, so count unique hashes separately.
type receiptSweepDiagnostics struct {
	started time.Time
	budget  time.Duration
	hashes  map[common.Hash]struct{}
	reads   int
	lastRPC time.Duration
}

func (s *receiptSweepDiagnostics) observe(read receiptRead) {
	s.hashes[read.attempt.hash] = struct{}{}
	s.reads++
	s.budget += read.budget
	s.lastRPC = read.duration
}

func (r receiptRead) reason() string {
	switch {
	case r.parentCanceled:
		return "receipt_parent_canceled"
	case errors.Is(r.err, context.DeadlineExceeded):
		return "receipt_rpc_deadline"
	case errors.Is(r.err, context.Canceled):
		return "receipt_rpc_canceled"
	default:
		return "receipt_rpc_error"
	}
}
