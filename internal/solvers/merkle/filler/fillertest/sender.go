// Package fillertest provides the scripted signer lane shared by Merkle filler tests.
package fillertest

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
)

// Sender is a scripted keeper.Sender. Unsigned models a submission refused before signing.
type Sender struct {
	IsBusy     bool
	Request    transaction.ExecutionRequest
	Result     *transaction.ExecutionResult
	SubmitErr  error
	PollErr    error
	Before     func(transaction.ExecutionRequest)
	DuringPoll func()
	Submits    int
	Polls      int
	Unsigned   bool
}

func (s *Sender) Busy() bool { return s.IsBusy }

func (s *Sender) Submit(_ context.Context, req transaction.ExecutionRequest) error {
	if s.Before != nil {
		s.Before(req)
	}
	s.Submits++
	s.Request = req
	s.IsBusy = !s.Unsigned
	return s.SubmitErr
}

func (s *Sender) Poll(context.Context) (*transaction.ExecutionResult, error) {
	s.Polls++
	if s.Result != nil && s.PollErr == nil {
		s.IsBusy = false
		if s.DuringPoll != nil {
			s.DuringPoll()
		}
	}
	return s.Result, s.PollErr
}

// Confirmed is a canonical successful receipt in block 8.
func Confirmed() *transaction.ExecutionResult {
	hash := common.HexToHash("0x77")
	return &transaction.ExecutionResult{Hash: hash, Execution: &provider.Execution{TxHash: hash, BlockNumber: big.NewInt(8), BlockHash: common.HexToHash("0x78")}}
}
