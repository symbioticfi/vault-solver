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
// When a separate write RPC is configured, writeClient carries transaction broadcasts only; every
// read stays on the embedded (primary) client. writeClient equals the embedded client when no
// separate write endpoint is configured.
type Client struct {
	*ethclient.Client

	writeClient *ethclient.Client
	chainID     *big.Int
	multicall   common.Address
}

// Dial connects to the EVM RPC endpoint(s), records the chain id, and pins the Multicall3 address
// used for batched reads. rpcURLs[0] is the primary; any extra entries are HTTP(S) fallbacks tried in
// order when the primary is unavailable (see fallbackTransport). A single URL preserves the plain
// ethclient dial (any scheme), so non-HTTP transports keep working when no fallback is configured.
//
// writeRPCURL, when non-empty, is dialed as a SEPARATE client used only to broadcast transactions
// (see SendTransaction); every read stays on the primary. Empty reuses the primary for broadcasts,
// so behaviour is unchanged.
func Dial(ctx context.Context, rpcURLs []string, writeRPCURL, multicallAddr string, log logr.Logger) (*Client, error) {
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

	// A distinct write endpoint (e.g. a private/MEV-protected relay) carries only transaction
	// broadcasts; reads stay on the primary. Empty reuses the primary so behaviour is unchanged.
	writeClient := ec
	if writeRPCURL != "" {
		wc, wcErr := dialClient(ctx, []string{writeRPCURL}, log)
		if wcErr != nil {
			ec.Close()
			return nil, errors.Errorf("chain: dial write rpc: %w", wcErr)
		}
		writeClient = wc
	}

	return &Client{Client: ec, writeClient: writeClient, chainID: id, multicall: common.HexToAddress(multicallAddr)}, nil
}

// SendTransaction broadcasts a signed transaction through the write client. When a separate
// writeRpcUrl is configured this is the ONLY call routed there — nonce, gas, fee, receipt and
// block-number reads all stay on the primary client — so fills can be submitted through a private
// endpoint while state is read from a normal RPC. It overrides the promoted ethclient method.
func (c *Client) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return c.writeClient.SendTransaction(ctx, tx)
}

// Close closes the primary client and, when a separate write client was dialed, that one too. It
// overrides the promoted ethclient method so the write client is not leaked.
func (c *Client) Close() {
	c.Client.Close()
	if c.writeClient != nil && c.writeClient != c.Client {
		c.writeClient.Close()
	}
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
