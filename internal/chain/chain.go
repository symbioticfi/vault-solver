// Package chain wraps the EVM client with the generic, cross-solver primitives: dial, chain id, and
// Multicall3-batched reads. Solver-specific reads (e.g. vault/adapter liquidity) live in the owning
// solver package and use Multicall to collapse round-trips.
package chain

import (
	"context"
	"math/big"
	"net/http"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
)

// multicallB is the stateless v2 aggregate3 pack/unpack binding (no backend).
var multicallB = multicall3.NewMulticall3()

// Client is an ethclient.Client plus the chain id and the Multicall3 address, cached at dial time.
type Client struct {
	*ethclient.Client

	chainID   *big.Int
	multicall common.Address
}

// Dial connects to the EVM RPC endpoint(s), records the chain id, and pins the Multicall3 address
// used for batched reads. rpcURLs[0] is the primary; any extra entries are HTTP(S) fallbacks tried in
// order when the primary is unavailable (see fallbackTransport). A single URL preserves the plain
// ethclient dial (any scheme), so non-HTTP transports keep working when no fallback is configured.
func Dial(ctx context.Context, rpcURLs []string, multicallAddr string, log logr.Logger) (*Client, error) {
	if len(rpcURLs) == 0 {
		return nil, errors.New("chain: no rpc url configured")
	}
	if !common.IsHexAddress(multicallAddr) {
		return nil, errors.Errorf("chain: invalid multicall address %q", multicallAddr)
	}

	ec, err := dialClient(ctx, rpcURLs, log)
	if err != nil {
		return nil, err
	}
	id, err := ec.ChainID(ctx)
	if err != nil {
		ec.Close()
		return nil, errors.Errorf("chain: get chain id: %w", err)
	}
	return &Client{Client: ec, chainID: id, multicall: common.HexToAddress(multicallAddr)}, nil
}

// dialClient builds the ethclient. A single non-HTTP endpoint (ws/ipc) keeps a plain dial, since the
// fallback transport — which carries the per-call rpcAttemptTimeout — only supports http(s). Every
// http(s) endpoint, even a single one, goes through that transport so a hung node call times out
// instead of blocking the caller (e.g. the txmanager worker) forever.
func dialClient(ctx context.Context, rpcURLs []string, log logr.Logger) (*ethclient.Client, error) {
	if len(rpcURLs) == 1 && !isHTTPURL(rpcURLs[0]) {
		ec, err := ethclient.DialContext(ctx, rpcURLs[0])
		if err != nil {
			return nil, errors.Errorf("chain: dial: %w", err)
		}
		return ec, nil
	}
	endpoints, err := parseHTTPEndpoints(rpcURLs)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Transport: &fallbackTransport{endpoints: endpoints, base: http.DefaultTransport, log: log}}
	rc, err := rpc.DialOptions(ctx, rpcURLs[0], rpc.WithHTTPClient(httpClient))
	if err != nil {
		return nil, errors.Errorf("chain: dial (fallback): %w", err)
	}
	return ethclient.NewClient(rc), nil
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

// Multicall batches reads through Multicall3.aggregate3 at the latest block.
func (c *Client) Multicall(ctx context.Context, calls []Call) ([]CallResult, error) {
	in := make([]multicall3.Multicall3Call3, len(calls))
	for i, call := range calls {
		in[i] = multicall3.Multicall3Call3{Target: call.Target, AllowFailure: call.AllowFailure, CallData: call.Data}
	}
	data := multicallB.PackAggregate3(in)
	ret, err := c.CallContract(ctx, ethereum.CallMsg{To: &c.multicall, Data: data}, nil)
	if err != nil {
		return nil, errors.Errorf("chain: multicall aggregate3: %w", err)
	}
	out, err := multicallB.UnpackAggregate3(ret)
	if err != nil {
		return nil, errors.Errorf("chain: multicall unpack aggregate3: %w", err)
	}
	res := make([]CallResult, len(out))
	for i, o := range out {
		res[i] = CallResult{Success: o.Success, ReturnData: o.ReturnData}
	}
	return res, nil
}
