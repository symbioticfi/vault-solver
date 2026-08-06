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
// When a separate write RPC is configured, writeClient carries transaction broadcasts and account
// nonce reads so startup observes one coherent nonce lane. All other reads stay on the
// embedded (primary) client. Broadcasts never use cross-endpoint fallback: an ambiguous first send
// must remain visible to txmanager instead of being masked by a later endpoint's response.
type Client struct {
	*ethclient.Client

	writeClient *ethclient.Client
	chainID     *big.Int
	multicall   common.Address
}

// Dial connects to the EVM RPC endpoint(s), records the chain id, and pins the Multicall3 address
// used for batched reads. rpcURLs[0] is the primary; any extra entries are HTTP(S) fallbacks tried in
// order when the primary is unavailable (see fallbackTransport). A single non-HTTP URL preserves
// the plain ethclient dial; HTTP(S) calls use the bounded transport even with one endpoint.
//
// writeRPCURL, when non-empty, is dialed as a SEPARATE client used to broadcast transactions and
// read account nonces (see SendTransaction, NonceAt, and PendingNonceAt). Every other read stays on
// the primary. When it is empty, broadcasts and nonce reads use rpcURLs[0] without falling over.
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

	// A distinct write endpoint (e.g. a private/MEV-protected relay) carries transaction broadcasts
	// and nonce reads; all other reads stay on the primary. Even without writeRpcUrl, isolate
	// writes from a multi-endpoint read client: replaying eth_sendRawTransaction across endpoints can
	// hide an ambiguous acceptance behind a later nonce-too-low response.
	writeClient := ec
	writeEndpoint := writeRPCURL
	if writeEndpoint == "" && len(rpcURLs) > 1 {
		writeEndpoint = rpcURLs[0]
	}
	if writeEndpoint != "" {
		wc, wcErr := dialClient(ctx, []string{writeEndpoint}, log)
		if wcErr != nil {
			ec.Close()
			return nil, errors.Errorf("chain: dial write rpc: %w", wcErr)
		}
		// An explicitly configured endpoint is an independent trust boundary and must prove it
		// belongs to the read chain. Probing the implicit primary here would break read-only solvers
		// that are running through a fallback while that primary is unavailable.
		if writeRPCURL != "" {
			writeID, writeIDErr := wc.ChainID(ctx)
			if writeIDErr != nil {
				wc.Close()
				ec.Close()
				return nil, errors.Errorf("chain: get write rpc chain id: %w", writeIDErr)
			}
			if writeID.Cmp(id) != 0 {
				wc.Close()
				ec.Close()
				return nil, errors.Errorf(
					"chain: write rpc chain id mismatch: read %s, write %s", id, writeID,
				)
			}
		}
		writeClient = wc
	}

	return &Client{Client: ec, writeClient: writeClient, chainID: id, multicall: common.HexToAddress(multicallAddr)}, nil
}

// SendTransaction broadcasts a signed transaction through the write client. It overrides the
// promoted ethclient method.
func (c *Client) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return c.writeClient.SendTransaction(ctx, tx)
}

// NonceAt reads the mined nonce through the write client so startup compares one endpoint's mined
// and pending views instead of failing on harmless head skew between independent RPC nodes.
func (c *Client) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	return c.writeClient.NonceAt(ctx, account, blockNumber)
}

// PendingNonceAt reads the pending nonce through the write client so a private write endpoint can
// report transactions that are not visible to the primary RPC. It overrides the promoted ethclient
// method. When no separate write endpoint is configured, it targets the primary endpoint.
func (c *Client) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return c.writeClient.PendingNonceAt(ctx, account)
}

// Close closes the primary client and, when a separate write client was dialed, that one too. It
// overrides the promoted ethclient method so the write client is not leaked.
func (c *Client) Close() {
	c.Client.Close()
	if c.writeClient != nil && c.writeClient != c.Client {
		c.writeClient.Close()
	}
}

// dialClient builds the ethclient. A single non-HTTP endpoint keeps a plain dial; HTTP(S) endpoints
// use fallbackTransport so each attempt remains bounded.
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
