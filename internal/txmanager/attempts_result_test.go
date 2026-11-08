package txmanager

import (
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

func TestNonceConflictResultRetainsSignedAttempt(t *testing.T) {
	b := newMockBackend()
	b.sendErrs = []error{errors.New("replacement transaction underpriced")}
	m := newTestManager(t, b)
	result := m.Send(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	if result.Outcome != OutcomeNonceConflict || !slices.Equal(result.Attempts, []common.Hash{result.Hash}) {
		t.Fatalf("rejected signed attempt lost its identity: %+v", result)
	}
}

func TestLifecycleResultRetainsAllReplacementAttempts(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "shutdown"}[shutdown], func(t *testing.T) {
			b := newMockBackend()
			m := New(b, mustSigner(t), big.NewInt(1), Config{PollInterval: time.Millisecond}, logr.Discard())
			pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
			if err != nil {
				t.Fatal(err)
			}
			first := pending.originalHash
			delete(b.receipts, first)
			if _, err := m.tryReplace(t.Context(), pending, replaceIntent{reason: replaceReasonValidity}); err != nil {
				t.Fatal(err)
			}
			second := pending.latestAttempt().hash
			result := make(chan Result, 1)
			pending.result = result
			m.trackUnminedTransaction(pending)
			if shutdown {
				m.deliverActiveShutdownTimeout()
			} else {
				pending.lifecycle = m.metrics.beginLifecycle("test")
				m.complete(t.Context(), pending)
			}
			completed := <-result
			if !slices.Equal(completed.Attempts, []common.Hash{first, second}) {
				t.Fatalf("owned replacement identities lost: %+v", completed)
			}
		})
	}
}
