package chain

import (
	"context"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
)

var multicallB = multicall3.NewMulticall3()

type Call struct {
	Target       common.Address
	AllowFailure bool
	Data         []byte
}

type CallResult struct {
	Success    bool
	ReturnData []byte
}

// Multicall executes aggregate3 against latest state. It never introduces a historical block tag.
func (c *Client) Multicall(ctx context.Context, calls []Call) ([]CallResult, error) {
	requests := make([]multicall3.Multicall3Call3, len(calls))
	for index, call := range calls {
		requests[index] = multicall3.Multicall3Call3{
			Target:       call.Target,
			AllowFailure: call.AllowFailure,
			CallData:     append([]byte(nil), call.Data...),
		}
	}
	payload := multicallB.PackAggregate3(requests)
	response, err := c.CallContract(ctx, ethereum.CallMsg{To: &c.multicall, Data: payload}, nil)
	if err != nil {
		return nil, errors.Errorf("chain: multicall aggregate3: %w", err)
	}
	decoded, err := multicallB.UnpackAggregate3(response)
	if err != nil {
		return nil, errors.Errorf("chain: multicall unpack aggregate3: %w", err)
	}
	results := make([]CallResult, len(decoded))
	for index, result := range decoded {
		results[index] = CallResult{
			Success:    result.Success,
			ReturnData: append([]byte(nil), result.ReturnData...),
		}
	}
	return results, nil
}
