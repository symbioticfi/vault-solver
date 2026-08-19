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
	rpcMethodGetTransactionCount   = "eth_getTransactionCount"
	rpcMethodGetTransactionReceipt = "eth_getTransactionReceipt"
	rpcMethodSendRawTransaction    = "eth_sendRawTransaction"
)

// fallbackTransport is a barebones, viem-style RPC fallback. It POSTs each JSON-RPC request to the
// configured endpoints in order, advancing to the next only on a transport failure or an unavailable
// response (HTTP 5xx / 429). Receipt and header reads also fall over when a non-final endpoint returns
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
	// Buffer the body once so it can be replayed against each endpoint.
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, errors.Errorf("rpc fallback: read body: %w", err)
		}
		body = b
	}
	method := boundedRPCMethod(body)
	requestObservation := t.metrics.beginRequest(t.role, method)
	nullFallbackID, nullFallbackMethod, nullFallback := nullResultFallbackRequest(body)

	var (
		lastErr     error
		lastOutcome = rpcOutcomeTransportError
	)
	for i, ep := range t.endpoints {
		endpoint := endpointLabel(i)
		attemptTimeout := endpointAttemptTimeout(req.Context(), len(t.endpoints)-i)
		if attemptTimeout <= 0 {
			lastErr = req.Context().Err()
			if lastErr == nil {
				lastErr = context.DeadlineExceeded
			}
			lastOutcome = classifyRPCFailure(lastErr, req.Context().Err())
			break
		}
		ctx, cancel := context.WithTimeout(req.Context(), attemptTimeout)
		attempt := req.Clone(ctx)
		attempt.URL = ep
		attempt.Host = ep.Host
		if body != nil {
			attempt.Body = io.NopCloser(bytes.NewReader(body))
			attempt.ContentLength = int64(len(body))
		}

		resp, err := t.base.RoundTrip(attempt)
		if err == nil && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			if nullFallback && i < len(t.endpoints)-1 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				unavailable, inspectErr := hasNullRPCResult(resp, nullFallbackID)
				if inspectErr != nil || unavailable {
					_ = resp.Body.Close()
					cancel()
					if inspectErr != nil {
						lastErr = inspectErr
						lastOutcome = classifyRPCFailure(inspectErr, req.Context().Err())
					} else {
						lastErr = errors.Errorf("%s returned a null result", nullFallbackMethod)
						lastOutcome = rpcOutcomeNullResult
					}
					t.metrics.observeAttempt(t.role, endpoint, method, lastOutcome)
					t.log.V(1).Info("rpc result unavailable; trying fallback",
						"endpoint", ep.Redacted(), "method", nullFallbackMethod, "err", lastErr.Error())
					continue
				}
			}
			// Keep the attempt context alive until the rpc layer finishes reading the body. Metrics
			// classify JSON-RPC error envelopes only after that body has been consumed.
			if t.metrics == nil {
				resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
			} else {
				statusCode := resp.StatusCode
				resp.Body = newObservedRPCBody(resp.Body, cancel, func(responseBody []byte, truncated bool, readErr error) {
					outcome := classifyRPCResponse(method, statusCode, responseBody, truncated, readErr)
					t.metrics.observeAttempt(t.role, endpoint, method, outcome)
					requestObservation.finish(outcome)
				})
			}
			return resp, nil
		}

		if err != nil {
			lastErr = err
			lastOutcome = classifyRPCFailure(err, req.Context().Err())
		} else {
			lastErr = errors.Errorf("status %d", resp.StatusCode)
			lastOutcome = classifyHTTPStatus(resp.StatusCode)
			_ = resp.Body.Close()
		}
		cancel()
		t.metrics.observeAttempt(t.role, endpoint, method, lastOutcome)
		if i < len(t.endpoints)-1 {
			t.log.V(1).Info("rpc endpoint failed; trying fallback",
				"endpoint", ep.Redacted(), "err", lastErr.Error())
		}
	}
	requestObservation.finish(lastOutcome)
	return nil, errors.Errorf("rpc fallback: all %d endpoints failed: %w", len(t.endpoints), lastErr)
}

func boundedRPCMethod(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "unknown"
	}
	if trimmed[0] == '[' {
		return "batch"
	}
	var request struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(trimmed, &request); err != nil || request.Method == "" {
		return "unknown"
	}
	// The client is internal, but keep the label bounded if a future raw-RPC call is added.
	switch request.Method {
	case "eth_blockNumber", "eth_call", rpcMethodChainID, "eth_estimateGas", "eth_feeHistory",
		"eth_gasPrice", "eth_getBalance", "eth_getBlockByHash", "eth_getBlockByNumber",
		"eth_getBlockReceipts", "eth_getCode", "eth_getLogs", "eth_getStorageAt",
		"eth_getTransactionByHash", rpcMethodGetTransactionCount, rpcMethodGetTransactionReceipt,
		"eth_maxPriorityFeePerGas", rpcMethodSendRawTransaction, "net_version", "web3_clientVersion":
		return request.Method
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

func classifyHTTPStatus(statusCode int) rpcOutcome {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return rpcOutcomeRateLimited
	case statusCode >= 500:
		return rpcOutcomeHTTP5xx
	case statusCode < 200 || statusCode >= 300:
		return rpcOutcomeHTTP4xx
	default:
		return rpcOutcomeSuccess
	}
}

func classifyRPCResponse(method string, statusCode int, body []byte, truncated bool, readErr error) rpcOutcome {
	if readErr != nil {
		return classifyRPCFailure(readErr, nil)
	}
	if outcome := classifyHTTPStatus(statusCode); outcome != rpcOutcomeSuccess {
		return outcome
	}
	if truncated {
		// JSON-RPC errors are small; a response exceeding the observation cap is an ordinary large result.
		return rpcOutcomeSuccess
	}
	if method == "batch" {
		var responses []rpcResponseEnvelope
		if err := json.Unmarshal(body, &responses); err != nil || len(responses) == 0 {
			return rpcOutcomeDecodeError
		}
		for _, response := range responses {
			if len(response.Error) > 0 && !bytes.Equal(bytes.TrimSpace(response.Error), []byte("null")) {
				return rpcOutcomeRPCError
			}
		}
		return rpcOutcomeSuccess
	}
	response, valid := decodeRPCResponse(body)
	if !valid || response.JSONRPC != jsonRPCVersion {
		return rpcOutcomeDecodeError
	}
	if len(response.Error) > 0 && !bytes.Equal(bytes.TrimSpace(response.Error), []byte("null")) {
		return rpcOutcomeRPCError
	}
	return rpcOutcomeSuccess
}

const rpcResponseObservationLimit = 64 << 10

type observedRPCBody struct {
	io.ReadCloser

	cancel    context.CancelFunc
	observe   func([]byte, bool, error)
	body      []byte
	truncated bool
	readErr   error
	once      sync.Once
}

func newObservedRPCBody(
	body io.ReadCloser,
	cancel context.CancelFunc,
	observe func([]byte, bool, error),
) *observedRPCBody {
	return &observedRPCBody{ReadCloser: body, cancel: cancel, observe: observe}
}

func (b *observedRPCBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		remaining := rpcResponseObservationLimit - len(b.body)
		if remaining > 0 {
			copied := min(n, remaining)
			b.body = append(b.body, p[:copied]...)
			b.truncated = copied < n
		} else {
			b.truncated = true
		}
	}
	if err != nil && !errors.Is(err, io.EOF) {
		b.readErr = err
	}
	return n, err
}

func (b *observedRPCBody) Close() error {
	closeErr := b.ReadCloser.Close()
	b.once.Do(func() {
		readErr := b.readErr
		if readErr == nil {
			readErr = closeErr
		}
		b.observe(b.body, b.truncated, readErr)
		b.cancel()
	})
	return closeErr
}

// nullResultFallbackRequest identifies the narrow read methods for which a JSON-RPC null result can
// mean endpoint lag. Batch and malformed requests are deliberately left to the RPC client unchanged.
func nullResultFallbackRequest(body []byte) (json.RawMessage, string, bool) {
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
	}
	if err := json.Unmarshal(body, &request); err != nil || request.JSONRPC != jsonRPCVersion ||
		len(request.ID) == 0 || bytes.Equal(bytes.TrimSpace(request.ID), []byte("null")) {
		return nil, "", false
	}
	switch request.Method {
	case rpcMethodGetTransactionReceipt, "eth_getBlockByHash", "eth_getBlockByNumber":
		return request.ID, request.Method, true
	default:
		return nil, "", false
	}
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

// hasNullRPCResult buffers and restores resp.Body, then reports whether it is a matching successful
// JSON-RPC response whose result is null. Error, malformed, and mismatched-id responses are preserved
// for the RPC client to interpret instead of being hidden by a fallback endpoint.
func hasNullRPCResult(resp *http.Response, requestID json.RawMessage) (bool, error) {
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	if err != nil {
		return false, errors.Errorf("read rpc response body: %w", err)
	}

	response, valid := decodeRPCResponse(body)
	if !valid || response.JSONRPC != jsonRPCVersion {
		return false, nil
	}
	if !bytes.Equal(bytes.TrimSpace(response.ID), bytes.TrimSpace(requestID)) {
		return false, nil
	}
	if len(response.Error) > 0 && !bytes.Equal(bytes.TrimSpace(response.Error), []byte("null")) {
		return false, nil
	}
	return bytes.Equal(bytes.TrimSpace(response.Result), []byte("null")), nil
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

// cancelOnClose cancels the per-attempt context when the response body is closed, so the timeout
// covers the full request+body-read without aborting an in-flight read.
type cancelOnClose struct {
	io.ReadCloser

	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
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
