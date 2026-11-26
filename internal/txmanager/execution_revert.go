package txmanager

import (
	"bytes"
	"strings"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

// ExecutionRevertError is an execution failure returned by gas estimation or by a write relay's
// pre-admission simulation. The integration owns
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

// relaySimulationRevert recognizes MEV Blocker's "Failed in pending block with: Reverted": its
// pre-admission simulation reverted and the bytes were not admitted. It carries no revert data.
func relaySimulationRevert(err error) *ExecutionRevertError {
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "failed in pending block with: reverted") {
		return nil
	}
	return &ExecutionRevertError{Cause: err}
}

func executionRevert(err error) *ExecutionRevertError {
	var revert *ExecutionRevertError
	if errors.As(err, &revert) {
		return revert
	}
	// A node may omit or malformedly encode its payload. It still reported a revert;
	// an undecodable payload is not evidence of a transport outage.
	data, reverted := chain.RevertData(err)
	if !reverted {
		return nil
	}
	return &ExecutionRevertError{Data: data, Cause: err}
}
