package chain

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

// rpcAttemptTimeout bounds a single endpoint attempt so a hung endpoint fails over instead of
// blocking. Short caller deadlines are divided across the remaining endpoints.
const (
	rpcAttemptTimeout              = 20 * time.Second
	jsonRPCVersion                 = "2.0"
	rpcMethodChainID               = "eth_chainId"
	rpcMethodGetBalance            = "eth_getBalance"
	rpcMethodGetTransactionCount   = "eth_getTransactionCount"
	rpcMethodGetTransactionReceipt = "eth_getTransactionReceipt"
	rpcMethodSendRawTransaction    = "eth_sendRawTransaction"
)

// fallbackTransport is a barebones, viem-style RPC fallback. It POSTs each JSON-RPC request to the
// configured endpoints in order, advancing to the next only on a transport failure or an unavailable
// response (HTTP 3xx / 5xx / 429). Receipt and header reads also fall over when a non-final endpoint returns
// a successful JSON-RPC null result, because that can mean the endpoint has not observed the object
// yet. Other HTTP 2xx responses — including JSON-RPC errors such as reverts — are returned as-is, so
// application errors are surfaced unchanged.
//
// It plugs in below go-ethereum's read client. Signed broadcasts and startup nonce reads use an
// isolated single-endpoint write client.
type fallbackTransport struct {
	endpoints []*url.URL
	base      http.RoundTripper
	metrics   *RPCMetrics
	role      string
	log       logr.Logger
}

func (t *fallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, errors.Errorf("rpc fallback: read body: %w", err)
		}
	}
	info := inspectRPCRequest(body)
	observation := t.metrics.beginRequest(t.role, info.boundedMethod)
	outcome := rpcOutcomeTransportError
	var lastErr error
	for index := range t.endpoints {
		response, attemptOutcome, err := t.attempt(req, body, info, index, observation)
		if err == nil {
			return response, nil
		}
		lastErr, outcome = err, attemptOutcome
		t.metrics.observeAttempt(t.role, endpointLabel(index), info.boundedMethod, outcome)
		if req.Context().Err() != nil {
			break
		}
		if index+1 < len(t.endpoints) {
			t.log.V(1).Info("rpc endpoint unavailable; trying fallback", "endpoint", t.endpoints[index].Redacted(), "method", info.rawMethod, "err", err.Error())
		}
	}
	observation(outcome)
	return nil, errors.Errorf("rpc fallback: all %d endpoints failed: %w", len(t.endpoints), lastErr)
}

func (t *fallbackTransport) attempt(req *http.Request, body []byte, info rpcRequestInfo, index int, observation func(rpcOutcome)) (*http.Response, rpcOutcome, error) {
	timeout := endpointAttemptTimeout(req.Context(), len(t.endpoints)-index)
	if timeout <= 0 {
		err := req.Context().Err()
		if err == nil {
			err = context.DeadlineExceeded
		}
		return nil, classifyRPCFailure(err, req.Context().Err()), err
	}
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	request := req.Clone(ctx)
	request.URL, request.Host = t.endpoints[index], t.endpoints[index].Host
	request.Body, request.ContentLength = io.NopCloser(bytes.NewReader(body)), int64(len(body))
	if isPendingLookupMethod(info.rawMethod) {
		request.Header.Set(erpcRetryEmptyHeader, "false")
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		cancel()
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, classifyRPCFailure(err, req.Context().Err()), err
	}
	if unavailableHTTPStatus(response.StatusCode) {
		_ = response.Body.Close()
		cancel()
		return nil, classifyHTTPStatus(response.StatusCode), errors.Errorf("status %d", response.StatusCode)
	}
	reader := io.Reader(response.Body)
	if info.nullFallback && index+1 < len(t.endpoints) && response.StatusCode >= 200 && response.StatusCode < 300 {
		// A null JSON-RPC envelope is small. Peek only a bounded prefix; large valid
		// block/receipt payloads remain streaming and are never buffered in full.
		prefix, err := io.ReadAll(io.LimitReader(response.Body, rpcResponseObservationLimit+1))
		if err != nil {
			_ = response.Body.Close()
			cancel()
			return nil, classifyRPCFailure(err, req.Context().Err()), errors.Errorf("read rpc response body: %w", err)
		}
		if len(prefix) <= rpcResponseObservationLimit && matchingNullResult(prefix, info.id) {
			_ = response.Body.Close()
			cancel()
			return nil, rpcOutcomeNullResult, errors.Errorf("%s returned a null result", info.rawMethod)
		}
		reader = io.MultiReader(bytes.NewReader(prefix), response.Body)
	}
	// The attempt deadline and logical request span end together when the RPC
	// decoder closes the body, including errors while streaming that body.
	response.Body = &rpcBody{
		reader: reader, closer: response.Body, cancel: cancel,
		observe: t.metrics != nil, method: info.boundedMethod, status: response.StatusCode,
		finish: func(outcome rpcOutcome) {
			t.metrics.observeAttempt(t.role, endpointLabel(index), info.boundedMethod, outcome)
			observation(outcome)
		},
	}
	return response, rpcOutcomeSuccess, nil
}

type rpcRequestInfo struct {
	boundedMethod string
	rawMethod     string
	id            json.RawMessage
	nullFallback  bool
}

func inspectRPCRequest(body []byte) rpcRequestInfo {
	info := rpcRequestInfo{boundedMethod: "unknown"}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return info
	}
	if trimmed[0] == '[' {
		info.boundedMethod = "batch"
		return info
	}
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
	}
	if json.Unmarshal(trimmed, &request) != nil || request.Method == "" {
		return info
	}
	info.rawMethod, info.boundedMethod = request.Method, boundedRPCMethodName(request.Method)
	lookup := request.Method == rpcMethodGetTransactionReceipt || request.Method == "eth_getBlockByHash" || request.Method == "eth_getBlockByNumber"
	if lookup && request.JSONRPC == jsonRPCVersion && hasJSONValue(request.ID) {
		info.id, info.nullFallback = request.ID, true
	}
	return info
}

func hasJSONValue(value json.RawMessage) bool {
	value = bytes.TrimSpace(value)
	return len(value) != 0 && !bytes.Equal(value, []byte("null"))
}

// erpcRetryEmptyHeader opts a request out of eRPC's empty-result retry. Transactions are sent
// through a private relay, so until one is mined every public upstream legitimately returns null
// for its receipt; eRPC treats that null as a lagging upstream and retries across all of them,
// which turns "still pending" into a timeout at our 2s receipt read budget. With the header, the
// null comes straight back and the tx manager keeps polling normally. Other proxies ignore it.
const erpcRetryEmptyHeader = "X-ERPC-Retry-Empty"

// isPendingLookupMethod reports the methods whose null result means "not mined yet", not "missing".
func isPendingLookupMethod(method string) bool {
	return method == rpcMethodGetTransactionReceipt || method == "eth_getTransactionByHash"
}

func boundedRPCMethodName(method string) string {
	// The client is internal, but keep the label bounded if a future raw-RPC call is added.
	switch method {
	case "eth_blockNumber", "eth_call", rpcMethodChainID, "eth_estimateGas", "eth_feeHistory",
		"eth_gasPrice", rpcMethodGetBalance, "eth_getBlockByHash", "eth_getBlockByNumber",
		"eth_getBlockReceipts", "eth_getCode", "eth_getLogs", "eth_getStorageAt",
		"eth_getTransactionByHash", rpcMethodGetTransactionCount, rpcMethodGetTransactionReceipt,
		"eth_maxPriorityFeePerGas", rpcMethodSendRawTransaction, "net_version", "web3_clientVersion":
		return method
	default:
		return "other"
	}
}

func classifyRPCFailure(err, parentErr error) rpcOutcome {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(parentErr, context.Canceled):
		return rpcOutcomeContextCanceled
	case errors.Is(err, context.DeadlineExceeded), errors.Is(parentErr, context.DeadlineExceeded):
		return rpcOutcomeDeadlineExceeded
	default:
		return rpcOutcomeTransportError
	}
}

func unavailableHTTPStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode >= 500 ||
		(statusCode >= 300 && statusCode < 400)
}

func classifyHTTPStatus(statusCode int) rpcOutcome {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return rpcOutcomeRateLimited
	case statusCode >= 500:
		return rpcOutcomeHTTP5xx
	case statusCode >= 300 && statusCode < 400:
		return rpcOutcomeHTTP3xx
	case statusCode < 200 || statusCode >= 400:
		return rpcOutcomeHTTP4xx
	default:
		return rpcOutcomeSuccess
	}
}

func classifyRPCResponse(method string, statusCode int, body []byte, truncated bool, readErr error) rpcOutcome {
	if readErr != nil {
		return classifyRPCFailure(readErr, nil)
	}
	status := classifyHTTPStatus(statusCode)
	if status != rpcOutcomeSuccess || truncated {
		// Responses larger than the observation cap are ordinary large results.
		return status
	}
	var responses []rpcResponseEnvelope
	if method == "batch" {
		if json.Unmarshal(body, &responses) != nil || len(responses) == 0 {
			return rpcOutcomeDecodeError
		}
	} else {
		response, valid := decodeRPCResponse(body)
		if !valid || response.JSONRPC != jsonRPCVersion {
			return rpcOutcomeDecodeError
		}
		responses = []rpcResponseEnvelope{response}
	}
	for _, response := range responses {
		if hasJSONValue(response.Error) {
			return rpcOutcomeRPCError
		}
	}
	return rpcOutcomeSuccess
}

const rpcResponseObservationLimit = 64 << 10

type rpcBody struct {
	reader    io.Reader
	closer    io.Closer
	cancel    context.CancelFunc
	observe   bool
	method    string
	status    int
	finish    func(rpcOutcome)
	prefix    []byte
	truncated bool
	readErr   error
	once      sync.Once
}

func (b *rpcBody) Read(data []byte) (int, error) {
	n, err := b.reader.Read(data)
	if b.observe && n > 0 {
		take := min(n, rpcResponseObservationLimit-len(b.prefix))
		b.prefix = append(b.prefix, data[:take]...)
		b.truncated = b.truncated || take < n
	}
	if err != nil && !errors.Is(err, io.EOF) {
		b.readErr = err
	}
	return n, err
}

func (b *rpcBody) Close() error {
	err := b.closer.Close()
	b.once.Do(func() {
		defer b.cancel()
		if b.readErr == nil {
			b.readErr = err
		}
		if b.observe {
			b.finish(classifyRPCResponse(b.method, b.status, b.prefix, b.truncated, b.readErr))
		}
	})
	return err
}

type rpcResponseEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

func decodeRPCResponse(body []byte) (rpcResponseEnvelope, bool) {
	var response rpcResponseEnvelope
	if err := json.Unmarshal(body, &response); err != nil {
		return rpcResponseEnvelope{}, false
	}
	return response, true
}

func matchingNullResult(body []byte, requestID json.RawMessage) bool {
	response, ok := decodeRPCResponse(body)
	return ok && response.JSONRPC == jsonRPCVersion &&
		bytes.Equal(bytes.TrimSpace(response.ID), bytes.TrimSpace(requestID)) &&
		(len(response.Error) == 0 || bytes.Equal(bytes.TrimSpace(response.Error), []byte("null"))) &&
		bytes.Equal(bytes.TrimSpace(response.Result), []byte("null"))
}

func endpointAttemptTimeout(ctx context.Context, endpointsLeft int) time.Duration {
	if endpointsLeft <= 0 {
		return 0
	}
	deadline, bounded := ctx.Deadline()
	if !bounded {
		return rpcAttemptTimeout
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0
	}
	return min(rpcAttemptTimeout, remaining/time.Duration(endpointsLeft))
}

// isHTTPURL reports whether raw is an http(s) URL — the schemes the fallback transport (and thus the
// per-call rpcAttemptTimeout) supports.
func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

// parseHTTPEndpoints validates that every URL is HTTP(S) (the only scheme the fallback transport
// supports) and returns the parsed endpoints in order, dropping duplicates so the same endpoint is
// never tried twice in a fallover sweep.
func parseHTTPEndpoints(urls []string) ([]*url.URL, error) {
	out := make([]*url.URL, 0, len(urls))
	seen := make(map[string]bool, len(urls))
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, errors.Errorf("chain: invalid rpc url %q: %w", raw, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, errors.Errorf("chain: rpc fallback supports http(s) only, got %q", raw)
		}
		if key := u.String(); !seen[key] {
			seen[key] = true
			out = append(out, u)
		}
	}
	return out, nil
}
