package txmanager

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
)

type estimateRPCError struct {
	code    int
	message string
	data    any
}

func (e *estimateRPCError) Error() string  { return e.message }
func (e *estimateRPCError) ErrorCode() int { return e.code }
func (e *estimateRPCError) ErrorData() any { return e.data }

type gasFailureBackend struct {
	*mockBackend

	err error
}

func (b *gasFailureBackend) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	return 0, b.err
}

func (b *gasFailureBackend) EstimateGasNextBlock(
	ctx context.Context, call ethereum.CallMsg, _ *types.Header, _ time.Duration,
) (uint64, error) {
	return b.EstimateGas(ctx, call)
}

// A sibling fill can invalidate calldata before estimation. Execution reverts must carry their
// payload to the integration without paging before it reconciles business state; RPC outages page.
func TestEstimateExecutionRevertReturnsPayloadWithoutErrorAlert(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cause      error
		wantData   []byte
		wantRevert bool
	}{
		{"code three", &estimateRPCError{3, "execution reverted", "0x1f6d5aef"}, []byte{0x1f, 0x6d, 0x5a, 0xef}, true},
		{"wrapped RPC", errors.Errorf("estimate RPC: %w", &estimateRPCError{3, "execution reverted", "0x0102"}), []byte{1, 2}, true},
		{"provider code", &estimateRPCError{-32000, "execution reverted: NonceUsed", "0x0102"}, []byte{1, 2}, true},
		{"no payload", errors.New("execution reverted"), nil, true},
		{"malformed payload", &estimateRPCError{3, "execution reverted", "0xgarbage"}, nil, true},
		{"transport", io.ErrUnexpectedEOF, nil, false},
		{"unrelated RPC data", &estimateRPCError{-32000, "rate limit exceeded", "0x0102"}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &gasFailureBackend{mockBackend: newMockBackend(), err: tc.cause}
			logs, log := newLogCapture(1)
			m := newStreakManager(t, b, log)
			pending, err := m.broadcast(managerCtx(t.Context(), m), Request{To: common.Address{1}, Label: "fill"})
			if pending != nil || !errors.Is(err, tc.cause) {
				t.Fatalf("failed estimate result = %+v, %v; want wrapped cause and no signing", pending, err)
			}
			var revert interface{ RevertData() []byte }
			if errors.As(err, &revert) != tc.wantRevert {
				t.Fatalf("execution revert classification = %v; want %v", err, tc.wantRevert)
			}
			if tc.wantRevert && !bytes.Equal(revert.RevertData(), tc.wantData) {
				t.Fatalf("revert data = %x; want %x", revert.RevertData(), tc.wantData)
			}
			wantErrors := 1
			if tc.wantRevert {
				wantErrors = 0
			}
			if failures, _ := countLogs(*logs, "gas estimation failed"); failures != wantErrors {
				t.Fatalf("estimate Error alerts = %d; want %d", failures, wantErrors)
			}
			if len(b.attemptedTransactions()) != 0 {
				t.Fatal("signed a transaction after failed estimate")
			}
		})
	}
}
