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
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
)

// multicallB is the stateless v2 aggregate3 pack/unpack binding (no backend).
var multicallB = multicall3.NewMulticall3()

// Client is an ethclient.Client plus the chain id and the Multicall3 address, cached at dial time.
// writeClient carries transaction broadcasts only and is always dialed against exactly one endpoint;
// every read stays on the embedded client and may use its configured fallbacks.
type Client struct {
	*ethclient.Client

	writeClient *ethclient.Client
	chainID     *big.Int
	multicall   common.Address
}

// Dial connects to the EVM RPC endpoint(s), validates every distinct endpoint against the expected
// chain id, and pins the Multicall3 address used for batched reads. rpcURLs[0] is the primary; any
// extra entries are HTTP(S) read fallbacks tried in order when the primary is unavailable (see
// fallbackTransport). HTTP(S), even alone, uses the bounded fallback transport; only one supported
// non-HTTP endpoint preserves plain ethclient dialing.
//
// writeRPCURL, when non-empty, selects the single endpoint used only to broadcast transactions (see
// SendTransaction). Empty selects rpcURLs[0]. Broadcasts never traverse read fallbacks.
func Dial(
	ctx context.Context,
	rpcURLs []string,
	writeRPCURL,
	multicallAddr string,
	expectedChainID uint64,
	log logr.Logger,
) (*Client, error) {
	if len(rpcURLs) == 0 {
		return nil, errors.New("chain: no rpc url configured")
	}
	if !common.IsHexAddress(multicallAddr) {
		return nil, errors.Errorf("chain: invalid multicall address %q", multicallAddr)
	}

	expected := new(big.Int).SetUint64(expectedChainID)
	seen := make(map[string]bool, len(rpcURLs)+1)
	for i, raw := range rpcURLs {
		if seen[raw] {
			continue
		}
		if err := validateEndpointChainID(ctx, raw, expected, log); err != nil {
			return nil, endpointFailure("rpc", i+1, endpointURL(raw), err)
		}
		seen[raw] = true
	}
	if writeRPCURL != "" && !seen[writeRPCURL] {
		if err := validateEndpointChainID(ctx, writeRPCURL, expected, log); err != nil {
			return nil, endpointFailure("write rpc", 1, endpointURL(writeRPCURL), err)
		}
	}

	ec, err := dialClient(ctx, rpcURLs, log)
	if err != nil {
		return nil, err
	}

	// The write client always has exactly one endpoint. This prevents an ambiguous primary broadcast
	// failure from falling through to a read fallback whose JSON-RPC response could misclassify the
	// original attempt as a definitive rejection.
	writeEndpoint := writeRPCURL
	if writeEndpoint == "" {
		writeEndpoint = rpcURLs[0]
	}
	writeClient, writeErr := dialClient(ctx, []string{writeEndpoint}, log)
	if writeErr != nil {
		ec.Close()
		return nil, endpointFailure("write rpc", 1, endpointURL(writeEndpoint), endpointClass(writeErr))
	}

	return &Client{
		Client:      ec,
		writeClient: writeClient,
		chainID:     expected,
		multicall:   common.HexToAddress(multicallAddr),
	}, nil
}

// validateEndpointChainID checks one endpoint in isolation so a healthy primary cannot hide a bad
// fallback. Endpoint/transport causes are classified before crossing this security boundary because
// their concrete error strings can contain credentials or private routing tokens from the raw URL.
func validateEndpointChainID(ctx context.Context, raw string, expected *big.Int, log logr.Logger) error {
	ec, err := dialClient(ctx, []string{raw}, log)
	if err != nil {
		return endpointClass(err)
	}
	defer ec.Close()

	got, err := ec.ChainID(ctx)
	if err != nil {
		return errChainIDRequest
	}
	if got.Cmp(expected) != 0 {
		return errors.Errorf("chain id mismatch: got %s, want %s", got, expected)
	}
	return nil
}

// SendTransaction broadcasts a signed transaction through the single-endpoint write client.
// writeRpcUrl selects that endpoint when configured; otherwise rpcUrls[0] does. Nonce, gas, fee,
// receipt and block-number reads stay on the fallback-capable read client. It overrides the promoted
// ethclient method.
func (c *Client) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return c.writeClient.SendTransaction(ctx, tx)
}

// Close closes the read and write clients. It overrides the promoted ethclient method so the
// independently dialed write client is not leaked.
func (c *Client) Close() {
	c.Client.Close()
	if c.writeClient != nil {
		c.writeClient.Close()
	}
}

// dialClient builds the ethclient. A single non-HTTP endpoint (ws/ipc) keeps a plain dial, since the
// fallback transport — which carries the per-call rpcAttemptTimeout — only supports http(s). Every
// http(s) endpoint, even a single one, goes through that transport so a hung node call times out
// instead of blocking the caller (e.g. the txmanager worker) forever.
func dialClient(ctx context.Context, rpcURLs []string, log logr.Logger) (*ethclient.Client, error) {
	if len(rpcURLs) == 0 {
		return nil, errors.New("chain: no rpc endpoint configured")
	}
	if len(rpcURLs) == 1 && !isHTTPURL(rpcURLs[0]) {
		u, validateErr := validateNonHTTPEndpoint(rpcURLs[0])
		if validateErr != nil {
			return nil, endpointFailure("rpc", 1, u, validateErr)
		}
		ec, err := ethclient.DialContext(ctx, rpcURLs[0])
		if err != nil {
			return nil, endpointFailure("rpc", 1, u, errDialTransport)
		}
		return ec, nil
	}
	endpoints, err := parseHTTPEndpoints(rpcURLs)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Transport: &fallbackTransport{endpoints: endpoints, base: http.DefaultTransport, log: log}}
	// The RPC layer only needs a base URL; fallbackTransport replaces it with the complete configured
	// endpoint for each attempt. Supplying the origin here prevents net/http's outer url.Error from
	// reattaching the primary path/query/userinfo when all runtime attempts fail.
	rc, err := rpc.DialOptions(ctx, endpointLabel(endpoints[0]), rpc.WithHTTPClient(httpClient))
	if err != nil {
		return nil, endpointFailure("rpc", 1, endpoints[0], errDialTransport)
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
	return c.MulticallAt(ctx, calls, nil)
}

// MulticallAt batches reads through Multicall3.aggregate3 at blockNumber.
func (c *Client) MulticallAt(ctx context.Context, calls []Call, blockNumber *big.Int) ([]CallResult, error) {
	in := make([]multicall3.Multicall3Call3, len(calls))
	for i, call := range calls {
		in[i] = multicall3.Multicall3Call3{Target: call.Target, AllowFailure: call.AllowFailure, CallData: call.Data}
	}
	data := multicallB.PackAggregate3(in)
	ret, err := c.CallContract(ctx, ethereum.CallMsg{To: &c.multicall, Data: data}, blockNumber)
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
