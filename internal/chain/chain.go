// Package chain wraps the EVM client with the generic, cross-solver primitives: dial, chain id, and
// Multicall3-batched reads. Solver-specific reads (e.g. vault/adapter liquidity) live in the owning
// solver package and use Multicall to collapse round-trips.
package chain

import (
	"context"
	"math/big"
	"net/http"
	"slices"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// multicallB is the stateless v2 aggregate3 pack/unpack binding (no backend).
var multicallB = multicall3.NewMulticall3()

// Client is an ethclient.Client plus the chain id and the Multicall3 address, cached at dial time.
// When a separate write RPC is configured, writeClient carries normal transaction broadcasts and account
// nonce reads so startup observes one coherent nonce lane. Sender balance telemetry tries that endpoint
// first and falls back to the read client when the write endpoint does not support balance reads. All
// other reads stay on the embedded (primary) client. cancelClient routes same-nonce self-cancellations
// to an optional dedicated endpoint. Broadcasts never use cross-endpoint fallback: an ambiguous first send
// must remain visible to txmanager instead of being masked by a later endpoint's response.
type Client struct {
	*ethclient.Client

	writeClient  *ethclient.Client
	cancelClient *ethclient.Client
	chainID      *big.Int
	multicall    common.Address

	// How each endpoint's calls are traced (see calls.go). Both are zero for HTTP(S), where the
	// instrumented transport already spans every request.
	readCalls   callTracing
	writeCalls  callTracing
	cancelCalls callTracing
}

// Dial connects to the EVM RPC endpoint(s), records the chain id, and pins the Multicall3 address
// used for batched reads. rpcURLs[0] is the primary; any extra entries are HTTP(S) fallbacks tried in
// order when the primary is unavailable (see fallbackTransport). A single non-HTTP URL preserves
// the plain ethclient dial; HTTP(S) calls use the bounded transport even with one endpoint.
//
// writeRPCURL, when non-empty, is dialed as a SEPARATE client used to broadcast transactions and
// read account nonces (see SendTransaction, NonceAt, and PendingNonceAt). Every other read stays on
// the primary. When it is empty, broadcasts and nonce reads use rpcURLs[0] without falling over.
// cancelRPCURL overrides only same-nonce self-cancellation broadcasts; empty uses the write client.
// attemptTimeout bounds each HTTP(S) endpoint attempt; zero preserves the 20-second default.
func Dial(
	ctx context.Context,
	rpcURLs []string,
	writeRPCURL, cancelRPCURL, multicallAddr string,
	attemptTimeout time.Duration,
) (*Client, error) {
	return DialWithMetrics(ctx, rpcURLs, writeRPCURL, cancelRPCURL, multicallAddr, attemptTimeout, nil)
}

// DialWithMetrics is Dial with generic HTTP JSON-RPC instrumentation on the supplied registry.
func DialWithMetrics(
	ctx context.Context,
	rpcURLs []string,
	writeRPCURL, cancelRPCURL, multicallAddr string,
	attemptTimeout time.Duration,
	rpcMetrics *RPCMetrics,
) (*Client, error) {
	if attemptTimeout < 0 {
		return nil, errors.New("chain: rpc attempt timeout must not be negative")
	}
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
	readRole := rpcRoleRead
	if writeEndpoint == "" {
		readRole = rpcRoleShared
	}
	ec, readTransport, err := dialClient(ctx, rpcURLs, readRole, attemptTimeout, rpcMetrics)
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
	readCalls := callTracing{role: readRole, transport: readTransport}
	writeClient := ec
	writeCalls := readCalls
	if writeEndpoint != "" {
		wc, writeTransport, wcErr := dialClient(ctx, []string{writeEndpoint}, rpcRoleWrite, attemptTimeout, rpcMetrics)
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
		writeCalls = callTracing{role: rpcRoleWrite, transport: writeTransport}
	}

	client := &Client{
		Client:       ec,
		writeClient:  writeClient,
		cancelClient: writeClient,
		chainID:      id,
		multicall:    common.HexToAddress(multicallAddr),
		readCalls:    readCalls,
		writeCalls:   writeCalls,
		cancelCalls:  writeCalls,
	}
	if cancelRPCURL != "" {
		cc, cancelTransport, cancelErr := dialClient(ctx, []string{cancelRPCURL}, rpcRoleCancel, attemptTimeout, rpcMetrics)
		if cancelErr != nil {
			client.Close()
			return nil, errors.Errorf("chain: dial cancellation rpc: %w", cancelErr)
		}
		client.cancelClient = cc
		cancelID, cancelErr := cc.ChainID(ctx)
		if cancelErr != nil {
			client.Close()
			return nil, errors.Errorf("chain: get cancellation rpc chain id: %w", cancelErr)
		}
		if cancelID.Cmp(id) != 0 {
			client.Close()
			return nil, errors.Errorf("chain: cancellation rpc chain id mismatch: read %s, cancellation %s", id, cancelID)
		}
		client.cancelCalls = callTracing{role: rpcRoleCancel, transport: cancelTransport}
	}
	return client, nil
}

// Close closes the primary client and every separately dialed broadcast client exactly once.
func (c *Client) Close() {
	c.Client.Close()
	if c.writeClient != nil && c.writeClient != c.Client {
		c.writeClient.Close()
	}
	if c.cancelClient != nil && c.cancelClient != c.Client && c.cancelClient != c.writeClient {
		c.cancelClient.Close()
	}
}

// dialClient builds the ethclient and reports the transport label its calls are traced under: empty
// for HTTP(S), which fallbackTransport already spans per request, and ws/ipc otherwise. A single
// non-HTTP endpoint keeps a plain dial under a connect span; HTTP(S) endpoints use fallbackTransport
// so each attempt remains bounded.
func dialClient(
	ctx context.Context,
	rpcURLs []string,
	role string,
	attemptTimeout time.Duration,
	rpcMetrics *RPCMetrics,
) (*ethclient.Client, string, error) {
	if len(rpcURLs) == 1 && !isHTTPURL(rpcURLs[0]) {
		return dialNonHTTP(ctx, rpcURLs[0], role)
	}
	endpoints, err := parseHTTPEndpoints(rpcURLs)
	if err != nil {
		return nil, "", err
	}
	rpcMetrics.bindTransport(role, len(endpoints))
	httpClient := &http.Client{
		Transport: &fallbackTransport{
			endpoints:      endpoints,
			base:           http.DefaultTransport,
			metrics:        rpcMetrics,
			role:           role,
			attemptTimeout: attemptTimeout,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	rc, err := rpc.DialOptions(ctx, rpcURLs[0], rpc.WithHTTPClient(httpClient))
	if err != nil {
		return nil, "", errors.Errorf("chain: dial (fallback): %w", err)
	}
	return ethclient.NewClient(rc), "", nil
}

// dialNonHTTP dials a websocket or IPC endpoint under a chain.rpc.connect span. A websocket carries
// traceparent on its handshake, which is the only header this transport has: go-ethereum reconnects
// a dropped connection internally and replays the same headers, so the provider can tie the
// connection back to this dial but never to an individual call. IPC has no handshake at all. Either
// way the calls themselves are spanned locally, in calls.go.
func dialNonHTTP(ctx context.Context, rpcURL, role string) (_ *ethclient.Client, _ string, err error) {
	transport := rpcTransport(rpcURL)
	ctx, end := traceConnect(ctx, role, transport)
	defer func() { end(err) }()

	if transport == rpcTransportIPC {
		ec, dialErr := ethclient.DialContext(ctx, rpcURL)
		if dialErr != nil {
			return nil, "", errors.Errorf("chain: dial: %w", dialErr)
		}
		return ec, transport, nil
	}
	header := make(http.Header)
	observability.InjectTraceHeaders(ctx, header)
	rc, dialErr := rpc.DialOptions(ctx, rpcURL, rpc.WithHeaders(header))
	if dialErr != nil {
		return nil, "", errors.Errorf("chain: dial: %w", dialErr)
	}
	return ethclient.NewClient(rc), transport, nil
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

// MulticallWithTime reads calls and their block timestamp in one latest-state eth_call.
// Keep the timestamp inside the batch: a separate latest header can observe a different head.
func (c *Client) MulticallWithTime(ctx context.Context, calls []Call) ([]CallResult, time.Time, error) {
	batch := append(slices.Clone(calls), Call{Target: c.multicall, AllowFailure: true, Data: multicallB.PackGetCurrentBlockTimestamp()})
	results, err := c.Multicall(ctx, batch)
	if err != nil {
		return nil, time.Time{}, err
	}
	if len(results) != len(batch) {
		return nil, time.Time{}, errors.Errorf("chain: timed multicall got %d results, want %d", len(results), len(batch))
	}
	stamp := results[len(calls)]
	if !stamp.Success {
		return nil, time.Time{}, errors.Errorf("chain: multicall %s getCurrentBlockTimestamp call failed: check chain.multicallAddress compatibility", c.multicall.Hex())
	}
	timestamp, err := multicallB.UnpackGetCurrentBlockTimestamp(stamp.ReturnData)
	if err != nil {
		return nil, time.Time{}, errors.Errorf("chain: multicall block timestamp: %w", err)
	}
	if timestamp == nil || !timestamp.IsInt64() || timestamp.Sign() <= 0 {
		return nil, time.Time{}, errors.New("chain: multicall returned invalid block timestamp")
	}
	return results[:len(calls)], time.Unix(timestamp.Int64(), 0), nil
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
