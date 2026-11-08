package txmanager

import (
	"bytes"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
)

// ExecutionRevertError is an execution failure returned by gas estimation. The integration owns
// protocol decoding, reconciliation and log severity. Data is nil when the RPC omitted or malformed
// the revert payload; Cause retains the original RPC error for errors.Is and errors.As.
type ExecutionRevertError struct {
	Data  []byte
	Cause error
}

func (e *ExecutionRevertError) Error() string {
	if e.Cause == nil {
		return "execution reverted"
	}
	return e.Cause.Error()
}

func (e *ExecutionRevertError) Unwrap() error { return e.Cause }

// RevertData returns an independent copy for integrations that consume revert-bearing errors
// through an interface instead of depending on this concrete error type.
func (e *ExecutionRevertError) RevertData() []byte { return bytes.Clone(e.Data) }

func executionRevert(err error) *ExecutionRevertError {
	var revert *ExecutionRevertError
	if errors.As(err, &revert) {
		return revert
	}
	var rpcErr rpc.Error
	if (!errors.As(err, &rpcErr) || rpcErr.ErrorCode() != 3) &&
		!strings.Contains(strings.ToLower(err.Error()), "execution reverted") {
		return nil
	}
	var data []byte
	var dataErr rpc.DataError
	if errors.As(err, &dataErr) {
		if encoded, ok := dataErr.ErrorData().(string); ok {
			// A node may omit or malformedly encode its payload. It still reported a revert;
			// an undecodable payload is not evidence of a transport outage.
			if decoded, decodeErr := hexutil.Decode(encoded); decodeErr == nil {
				data = decoded
			}
		}
	}
	return &ExecutionRevertError{Data: data, Cause: err}
}
