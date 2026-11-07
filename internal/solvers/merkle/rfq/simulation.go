package rfq

import (
	"bytes"
	"context"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

// Awarded RFQ settlement supplies the work check; the SDK authenticates its
// immutable inner call and closes the block pins around this complete simulation.
type outerFillSimulation struct {
	reader         recipes.Reader
	executor       common.Address
	inner, outer   []byte
	caller, signer common.Address
}

func (s outerFillSimulation) Simulate(ctx context.Context, request recipes.PreparedBatchSimulationRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.reader == nil || s.executor == (common.Address{}) || s.caller == (common.Address{}) || s.signer == (common.Address{}) || len(s.inner) == 0 || len(s.outer) == 0 || request.BlockNumber == nil || request.BlockNumber.Sign() < 0 || request.GasLimit == 0 {
		return errors.New("outer RFQ simulation requires identities, calldata, pinned block and gas budget")
	}
	if request.Caller != s.caller || len(request.Calls) != 1 || request.Calls[0].Executor != s.executor || !bytes.Equal(request.Calls[0].Data, s.inner) {
		return errors.New("outer RFQ simulation differs from admitted recipe execution")
	}
	_, err := s.reader.CallContract(ctx, ethereum.CallMsg{From: s.signer, To: &s.caller, Data: bytes.Clone(s.outer), Gas: request.GasLimit}, request.BlockNumber)
	return err
}
