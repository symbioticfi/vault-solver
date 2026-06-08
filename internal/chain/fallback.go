package chain

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

// rpcAttemptTimeout bounds a single endpoint attempt so a hung endpoint fails over instead of
// blocking. JSON-RPC reads (incl. a batched Multicall) comfortably fit; a slower response is treated
// as the endpoint being unhealthy.
const rpcAttemptTimeout = 20 * time.Second

// fallbackTransport is a barebones, viem-style RPC fallback. It POSTs each JSON-RPC request to the
// configured endpoints in order, advancing to the next only on a transport failure or an unavailable
// response (HTTP 5xx / 429). A normal HTTP 200 — including a JSON-RPC error body such as a revert —
// is returned as-is and never triggers fallover, so application errors are surfaced unchanged.
//
// It plugs in below go-ethereum's rpc/ethclient as the HTTP RoundTripper, so every existing read/send
// path gains fallback without any other change.
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

	var lastErr error
	for i, ep := range t.endpoints {
		ctx, cancel := context.WithTimeout(req.Context(), rpcAttemptTimeout)
		attempt := req.Clone(ctx)
		attempt.URL = ep
		attempt.Host = ep.Host
		if body != nil {
			attempt.Body = io.NopCloser(bytes.NewReader(body))
			attempt.ContentLength = int64(len(body))
		}

		resp, err := t.base.RoundTrip(attempt)
		if err == nil && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
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
