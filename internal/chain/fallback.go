package chain

import (
	"bytes"
	"context"
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
const rpcAttemptTimeout = 20 * time.Second

// fallbackTransport is a barebones, viem-style RPC fallback. It POSTs each JSON-RPC request to the
// configured endpoints in order, advancing to the next only on a transport failure or an unavailable
// response (HTTP 5xx / 429). A normal HTTP 200 — including a JSON-RPC error body such as a revert —
// is returned as-is and never triggers fallover, so application errors are surfaced unchanged.
//
// It plugs in below go-ethereum's read client. Signed broadcasts and startup nonce reads use an
// isolated single-endpoint write client.
type fallbackTransport struct {
	endpoints []*url.URL
	base      http.RoundTripper
	log       logr.Logger
}

// endpointPin is shared by requests derived from one read-snapshot context; mu serializes endpoint
// selection and guards transport/index when those requests are issued concurrently.
type endpointPin struct {
	mu        sync.Mutex
	transport *fallbackTransport
	index     int
}

type endpointPinKey struct{}

func withPinnedReadEndpoint(ctx context.Context) context.Context {
	return context.WithValue(ctx, endpointPinKey{}, &endpointPin{index: -1})
}

func endpointPinFromContext(ctx context.Context) *endpointPin {
	pin, _ := ctx.Value(endpointPinKey{}).(*endpointPin)
	return pin
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

	pin := endpointPinFromContext(req.Context())
	if pin != nil {
		pin.mu.Lock()
		defer pin.mu.Unlock()
		if pin.transport != nil && pin.transport != t {
			return nil, errors.New("rpc fallback: endpoint pin belongs to another transport")
		}
	}

	start, end := 0, len(t.endpoints)
	if pin != nil && pin.index >= 0 {
		if pin.index >= len(t.endpoints) {
			return nil, errors.New("rpc fallback: invalid pinned endpoint")
		}
		start, end = pin.index, pin.index+1
	}

	var lastErr error
	for i := start; i < end; i++ {
		ep := t.endpoints[i]
		attemptTimeout := endpointAttemptTimeout(req.Context(), end-i)
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
			if pin != nil && pin.index < 0 {
				pin.transport = t
				pin.index = i
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
		if i < end-1 {
			t.log.V(1).Info("rpc endpoint failed; trying fallback",
				"endpoint", ep.Redacted(), "err", lastErr.Error())
		}
	}
	if pin != nil && pin.index >= 0 {
		return nil, errors.Errorf("rpc fallback: pinned endpoint failed: %w", lastErr)
	}
	return nil, errors.Errorf("rpc fallback: all %d endpoints failed: %w", len(t.endpoints), lastErr)
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
