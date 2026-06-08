// Package chain wraps the EVM client with the generic, cross-solver primitives: dial, chain id, and
// Multicall3-batched reads. Solver-specific reads (e.g. vault/adapter liquidity) live in the owning
// solver package and use Multicall to collapse round-trips.
package chain

import (
	"context"
	"math/big"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
)

// Client is an ethclient.Client plus the chain id and the Multicall3 address, cached at dial time.
type Client struct {
	*ethclient.Client

	chainID   *big.Int
	multicall common.Address
}

// Dial connects to an EVM RPC endpoint, records its chain id, and pins the Multicall3 address used
// for batched reads.
func Dial(ctx context.Context, rpcURL, multicallAddr string) (*Client, error) {
	if !common.IsHexAddress(multicallAddr) {
		return nil, errors.Errorf("chain: invalid multicall address %q", multicallAddr)
	}
	ec, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, errors.Errorf("chain: dial: %w", err)
	}
	id, err := ec.ChainID(ctx)
	if err != nil {
		ec.Close()
		return nil, errors.Errorf("chain: get chain id: %w", err)
	}
	return &Client{Client: ec, chainID: id, multicall: common.HexToAddress(multicallAddr)}, nil
}

// ChainID returns a copy of the cached chain id.
func (c *Client) ChainID() *big.Int { return new(big.Int).Set(c.chainID) }

// Call is one batched read. AllowFailure=false makes the whole batch revert if this call reverts;
// true lets it fail independently (its CallResult.Success is then false).
type Call struct {
	Target       common.Address
	AllowFailure bool
	Data         []byte
}

// CallResult is the per-call outcome from Multicall.
type CallResult struct {
	Success    bool
	ReturnData []byte
}

// Multicall batches reads through Multicall3.aggregate3, collapsing N eth_calls into one round-trip.
func (c *Client) Multicall(ctx context.Context, calls []Call) ([]CallResult, error) {
	caller, err := multicall3.NewMulticall3Caller(c.multicall, c.Client)
	if err != nil {
		return nil, errors.Errorf("chain: bind multicall3 %s: %w", c.multicall, err)
	}
	in := make([]multicall3.Multicall3Call3, len(calls))
	for i, call := range calls {
		in[i] = multicall3.Multicall3Call3{Target: call.Target, AllowFailure: call.AllowFailure, CallData: call.Data}
	}
	out, err := caller.Aggregate3(&bind.CallOpts{Context: ctx}, in)
	if err != nil {
		return nil, errors.Errorf("chain: multicall aggregate3: %w", err)
	}
	res := make([]CallResult, len(out))
	for i, o := range out {
		res[i] = CallResult{Success: o.Success, ReturnData: o.ReturnData}
	}
	return res, nil
}
