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

var (
	errInvalidEndpoint   = errors.New("invalid endpoint")
	errUnsupportedScheme = errors.New("unsupported scheme")
	errDialTransport     = errors.New("dial/transport failure")
	errChainIDRequest    = errors.New("chain-id request failed")
)

// fallbackTransport is a barebones, viem-style RPC fallback. It POSTs each JSON-RPC request to the
// configured endpoints in order, advancing to the next only on a transport failure or an unavailable
// response (HTTP 5xx / 429). Any other non-2xx response is closed and reduced to a safe status error
// at this boundary. A normal HTTP 200 — including a JSON-RPC error body such as a revert — is returned
// as-is and never triggers fallover, so application errors are surfaced unchanged.
//
// It plugs in below go-ethereum's rpc/ethclient as the HTTP RoundTripper. The read client receives all
// configured endpoints; the separate transaction-broadcast client receives exactly one endpoint.
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

	lastFailure := "transport failure"
	lastIndex := 0
	for i, ep := range t.endpoints {
		ctx, cancel := context.WithTimeout(req.Context(), rpcAttemptTimeout)
		attempt := req.Clone(ctx)
		attempt.URL = ep
		attempt.Host = ep.Host
		if ep.User != nil {
			password, _ := ep.User.Password()
			attempt.SetBasicAuth(ep.User.Username(), password)
		}
		if body != nil {
			attempt.Body = io.NopCloser(bytes.NewReader(body))
			attempt.ContentLength = int64(len(body))
		}

		resp, err := t.base.RoundTrip(attempt)
		if err == nil {
			if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
				// Success: keep the attempt context alive until the rpc layer finishes reading the body.
				resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
				return resp, nil
			}
			retryableStatus := resp.StatusCode == http.StatusTooManyRequests ||
				(resp.StatusCode >= http.StatusInternalServerError && resp.StatusCode < 600)
			if !retryableStatus {
				cancel()
				statusCode := resp.StatusCode
				if resp.Body != nil {
					_ = resp.Body.Close()
				}
				return nil, errors.Errorf(
					"rpc fallback: endpoint %d (%s): HTTP %d",
					i+1,
					endpointLabel(ep),
					statusCode,
				)
			}
		}
		cancel()
		lastIndex = i
		if err != nil {
			lastFailure = "transport failure"
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
		} else {
			lastFailure = errors.Errorf("HTTP %d", resp.StatusCode).Error()
			_ = resp.Body.Close()
		}
		if i < len(t.endpoints)-1 {
			t.log.V(1).Info("rpc endpoint failed; trying fallback",
				"endpointOrdinal", i+1,
				"endpointOrigin", endpointLabel(ep),
				"failure", lastFailure,
			)
		}
	}
	if len(t.endpoints) == 0 {
		return nil, errors.New("rpc fallback: no endpoints configured")
	}
	return nil, errors.Errorf(
		"rpc fallback: all %d endpoints failed; endpoint %d (%s): %s",
		len(t.endpoints),
		lastIndex+1,
		endpointLabel(t.endpoints[lastIndex]),
		lastFailure,
	)
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

// endpointLabel deliberately retains only the parsed origin. url.URL.Redacted still preserves the
// path, query, and fragment and is therefore not safe for RPC diagnostics.
func endpointLabel(u *url.URL) string {
	if u == nil || u.Scheme == "" || u.Host == "" {
		return "invalid endpoint"
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}

func endpointURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	return u
}

func endpointFailure(role string, ordinal int, u *url.URL, cause error) error {
	label := endpointLabel(u)
	if label == "invalid endpoint" {
		return errors.Errorf("chain: %s endpoint %d: %w", role, ordinal, cause)
	}
	return errors.Errorf("chain: %s endpoint %d (%s): %w", role, ordinal, label, cause)
}

// endpointClass returns only a fixed, non-sensitive class. The original cause is intentionally not
// wrapped: URL and transport errors frequently render the complete credential-bearing endpoint.
func endpointClass(err error) error {
	for _, class := range []error{errInvalidEndpoint, errUnsupportedScheme, errDialTransport} {
		if errors.Is(err, class) {
			return class
		}
	}
	return errDialTransport
}

func validateNonHTTPEndpoint(raw string) (*url.URL, error) {
	u := endpointURL(raw)
	if u == nil {
		return nil, errInvalidEndpoint
	}
	switch u.Scheme {
	case "ws", "wss":
		if u.Host == "" {
			return u, errInvalidEndpoint
		}
	case "stdio":
	case "":
		if raw == "" {
			return u, errInvalidEndpoint
		}
	default:
		return u, errUnsupportedScheme
	}
	return u, nil
}

// parseHTTPEndpoints validates that every URL is HTTP(S) (the only scheme the fallback transport
// supports) and returns the parsed endpoints in order, dropping duplicates so the same endpoint is
// never tried twice in a fallover sweep.
func parseHTTPEndpoints(urls []string) ([]*url.URL, error) {
	out := make([]*url.URL, 0, len(urls))
	seen := make(map[string]bool, len(urls))
	for i, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, endpointFailure("rpc", i+1, nil, errInvalidEndpoint)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, endpointFailure("rpc", i+1, u, errUnsupportedScheme)
		}
		if u.Host == "" {
			return nil, endpointFailure("rpc", i+1, u, errInvalidEndpoint)
		}
		if !seen[raw] {
			seen[raw] = true
			out = append(out, u)
		}
	}
	return out, nil
}
