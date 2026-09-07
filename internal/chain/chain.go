// Package chain wraps the EVM client with the generic, cross-solver primitives: dial, chain id, and
// Multicall3-batched reads. Solver-specific reads (e.g. vault/adapter liquidity) live in the owning
// solver package and use Multicall to collapse round-trips.
package chain

import (
	"context"
	"math"
	"math/big"
	"net/http"
	"time"

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
// nonce reads so startup observes one coherent nonce lane. Sender balance telemetry tries that endpoint
// first and falls back to the read client when the write endpoint does not support balance reads. All
// other reads stay on the embedded (primary) client. Broadcasts never use cross-endpoint fallback: an ambiguous first send
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
	return dial(ctx, rpcURLs, writeRPCURL, multicallAddr, nil, log)
}

// DialWithMetrics is Dial with generic HTTP JSON-RPC instrumentation on the supplied registry.
func DialWithMetrics(
	ctx context.Context,
	rpcURLs []string,
	writeRPCURL string,
	multicallAddr string,
	rpcMetrics *RPCMetrics,
	log logr.Logger,
) (*Client, error) {
	return dial(ctx, rpcURLs, writeRPCURL, multicallAddr, rpcMetrics, log)
}

func dial(ctx context.Context, rpcURLs []string, writeRPCURL, multicallAddr string, rpcMetrics *RPCMetrics, log logr.Logger) (_ *Client, err error) {
	if len(rpcURLs) == 0 {
		return nil, errors.New("chain: no rpc url configured")
	}
	if !common.IsHexAddress(multicallAddr) {
		return nil, errors.Errorf("chain: invalid multicall address %q", multicallAddr)
	}
	writeEndpoint := writeRPCURL
	if writeEndpoint == "" && len(rpcURLs) > 1 {
		writeEndpoint = rpcURLs[0]
	}
	role := rpcRoleShared
	if writeEndpoint != "" {
		role = rpcRoleRead
	}
	read, err := dialClient(ctx, rpcURLs, role, rpcMetrics, log)
	if err != nil {
		return nil, err
	}
	client := &Client{Client: read, writeClient: read, multicall: common.HexToAddress(multicallAddr)}
	defer func() {
		if err != nil {
			client.Close()
		}
	}()
	client.chainID, err = read.ChainID(ctx)
	if err != nil {
		return nil, errors.Errorf("chain: get chain id: %w", err)
	}
	if client.chainID.Sign() <= 0 {
		return nil, errors.New("chain: chain id must be positive")
	}
	if writeEndpoint != "" {
		write, err := dialClient(ctx, []string{writeEndpoint}, rpcRoleWrite, rpcMetrics, log)
		if err != nil {
			return nil, errors.Errorf("chain: dial write rpc: %w", err)
		}
		client.writeClient = write
		// An explicit relay is a separate trust boundary. The implicit primary is
		// not probed: read-only solvers must still boot through a healthy fallback.
		if writeRPCURL != "" {
			id, err := write.ChainID(ctx)
			if err != nil {
				return nil, errors.Errorf("chain: get write rpc chain id: %w", err)
			}
			if id.Cmp(client.chainID) != 0 {
				return nil, errors.Errorf("chain: write rpc chain id mismatch: read %s, write %s", client.chainID, id)
			}
		}
	}
	return client, nil
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

// TransactionSenderBalanceAt prefers the write endpoint for sender telemetry, then falls back to the
// ordinary read client when a distinct submission endpoint rejects or cannot serve eth_getBalance.
func (c *Client) TransactionSenderBalanceAt(
	ctx context.Context,
	account common.Address,
	blockNumber *big.Int,
) (*big.Int, error) {
	balance, writeErr := c.writeClient.BalanceAt(ctx, account, blockNumber)
	if writeErr == nil || c.writeClient == c.Client {
		return balance, writeErr
	}
	balance, readErr := c.BalanceAt(ctx, account, blockNumber)
	if readErr == nil {
		return balance, nil
	}
	return nil, errors.Errorf(
		"chain: transaction sender balance: %w",
		errors.Join(
			errors.Errorf("write endpoint: %w", writeErr),
			errors.Errorf("read endpoints: %w", readErr),
		),
	)
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
func dialClient(ctx context.Context, rpcURLs []string, role string, rpcMetrics *RPCMetrics, log logr.Logger) (*ethclient.Client, error) {
	operation := "chain: dial"
	var options []rpc.ClientOption
	if len(rpcURLs) != 1 || isHTTPURL(rpcURLs[0]) {
		endpoints, err := parseHTTPEndpoints(rpcURLs)
		if err != nil {
			return nil, err
		}
		rpcMetrics.bindTransport(role, len(endpoints))
		httpClient := &http.Client{
			Transport:     &fallbackTransport{endpoints: endpoints, base: http.DefaultTransport, metrics: rpcMetrics, role: role, log: log},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
		options = append(options, rpc.WithHTTPClient(httpClient))
		operation += " (fallback)"
	}
	connection, err := rpc.DialOptions(ctx, rpcURLs[0], options...)
	if err != nil {
		return nil, errors.Errorf("%s: %w", operation, err)
	}
	return ethclient.NewClient(connection), nil
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
	if len(out) != len(calls) {
		return nil, errors.Errorf("chain: multicall returned %d results for %d calls", len(out), len(calls))
	}
	res := make([]CallResult, len(out))
	for i, o := range out {
		res[i] = CallResult{Success: o.Success, ReturnData: o.ReturnData}
	}
	return res, nil
}

// BlockTime reads the latest primary-chain timestamp for deadline calculations.
func (c *Client) BlockTime(ctx context.Context) (time.Time, error) {
	header, err := c.HeaderByNumber(ctx, nil)
	if err != nil {
		return time.Time{}, errors.Errorf("latest block header: %w", err)
	}
	return HeaderTime(header)
}

// HeaderTime rejects timestamps that cannot be represented by the deadline clock.
func HeaderTime(header *types.Header) (time.Time, error) {
	if header == nil || header.Time > math.MaxInt64 {
		return time.Time{}, errors.New("block header has no valid timestamp")
	}
	return time.Unix(int64(header.Time), 0), nil
}
