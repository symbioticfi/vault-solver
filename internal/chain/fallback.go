package chain

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

// rpcAttemptTimeout bounds a single endpoint attempt so a hung endpoint fails over instead of
// blocking. Short caller deadlines are divided across the remaining endpoints.
const rpcAttemptTimeout = 20 * time.Second

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
	nullFallbackID, nullFallbackMethod, nullFallback := nullResultFallbackRequest(body)

	var lastErr error
	for i, ep := range t.endpoints {
		attemptTimeout := endpointAttemptTimeout(req.Context(), len(t.endpoints)-i)
		if attemptTimeout <= 0 {
			lastErr = req.Context().Err()
			if lastErr == nil {
				lastErr = context.DeadlineExceeded
			}
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
					cancel()
					_ = resp.Body.Close()
					if inspectErr != nil {
						lastErr = inspectErr
					} else {
						lastErr = errors.Errorf("%s returned a null result", nullFallbackMethod)
					}
					t.log.V(1).Info("rpc result unavailable; trying fallback",
						"endpoint", ep.Redacted(), "method", nullFallbackMethod, "err", lastErr.Error())
					continue
				}
			}
			// Success: keep the attempt context alive until the rpc layer finishes reading the body.
			resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
			return resp, nil
		}
		cancel()
		if err != nil {
			lastErr = err
		} else {
			lastErr = errors.Errorf("status %d", resp.StatusCode)
			_ = resp.Body.Close()
		}
		if i < len(t.endpoints)-1 {
			t.log.V(1).Info("rpc endpoint failed; trying fallback",
				"endpoint", ep.Redacted(), "err", lastErr.Error())
		}
	}
	return nil, errors.Errorf("rpc fallback: all %d endpoints failed: %w", len(t.endpoints), lastErr)
}

// nullResultFallbackRequest identifies the narrow read methods for which a JSON-RPC null result can
// mean endpoint lag. Batch and malformed requests are deliberately left to the RPC client unchanged.
func nullResultFallbackRequest(body []byte) (json.RawMessage, string, bool) {
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
	}
	if err := json.Unmarshal(body, &request); err != nil || request.JSONRPC != "2.0" ||
		len(request.ID) == 0 || bytes.Equal(bytes.TrimSpace(request.ID), []byte("null")) {
		return nil, "", false
	}
	switch request.Method {
	case "eth_getTransactionReceipt", "eth_getBlockByHash", "eth_getBlockByNumber":
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
	if !valid || response.JSONRPC != "2.0" {
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
