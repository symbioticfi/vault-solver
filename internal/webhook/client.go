package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"time"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/parse"
)

const (
	defaultWebhookTimeout      = 5 * time.Second
	defaultWebhookMaxBodyBytes = 1 << 20
)

// Config describes HTTP transport for JSON webhook clients without resolving env-backed secrets.
type Config struct {
	URL              string
	Timeout          time.Duration
	MaxRequestBytes  int64
	MaxResponseBytes int64
	Headers          map[string]HeaderValue
}

type rawConfig struct {
	URL              string                 `yaml:"url"`
	Timeout          string                 `yaml:"timeout"`
	MaxRequestBytes  int64                  `yaml:"maxRequestBytes"`
	MaxResponseBytes int64                  `yaml:"maxResponseBytes"`
	Headers          map[string]HeaderValue `yaml:"headers"`
}

// HeaderValue holds a literal or an environment variable name, resolved for each request.
type HeaderValue struct {
	Value string `yaml:"value"`
	Env   string `yaml:"env"`
}

// ParseConfig decodes common webhook YAML config into runtime HTTP client config.
func ParseConfig(node yaml.Node) (Config, error) {
	var raw rawConfig
	if err := parse.DecodeStrict(node, &raw); err != nil {
		return Config{}, err
	}
	timeout, err := parse.Duration(raw.Timeout, defaultWebhookTimeout, "timeout")
	if err != nil {
		return Config{}, err
	}
	return normalizeConfig(Config{
		URL:              raw.URL,
		Timeout:          timeout,
		MaxRequestBytes:  raw.MaxRequestBytes,
		MaxResponseBytes: raw.MaxResponseBytes,
		Headers:          raw.Headers,
	})
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.Errorf("url: %w", err)
	}
	if u.Host == "" {
		return errors.Errorf("url: host is required")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return errors.New("url must use https, except loopback http for local development")
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func parseSize(n int64, field string) (int64, error) {
	if n == 0 {
		return defaultWebhookMaxBodyBytes, nil
	}
	if n < 0 || n == math.MaxInt64 {
		return 0, errors.Errorf("%s: invalid byte size %d", field, n)
	}
	return n, nil
}

func validateHeaders(raw map[string]HeaderValue) error {
	for name, h := range raw {
		switch {
		case name == "":
			return errors.New("headers: empty header name")
		case h.Value != "" && h.Env != "":
			return errors.Errorf("headers.%s: set value or env, not both", name)
		case h.Value == "" && h.Env == "":
			return errors.Errorf("headers.%s: value or env is required", name)
		}
	}
	return nil
}

func applyHeaders(destination http.Header, raw map[string]HeaderValue) error {
	for name, h := range raw {
		value := h.Value
		if h.Env != "" {
			value = os.Getenv(h.Env)
			if value == "" {
				return errors.Errorf("headers.%s: env %q is empty", name, h.Env)
			}
		}
		destination.Set(name, value)
	}
	return nil
}

// Client sends strict JSON requests to an external HTTP endpoint.
type Client struct {
	baseURL          *url.URL
	client           *http.Client
	maxRequestBytes  int64
	maxResponseBytes int64
	headers          map[string]HeaderValue
}

// HTTPStatusError reports a non-successful response from a webhook endpoint.
type HTTPStatusError struct {
	statusCode   int
	responseBody string
}

func (e *HTTPStatusError) Error() string {
	return "webhook: status " + strconv.Itoa(e.statusCode) + ": " + e.responseBody
}

// StatusCode returns the response's HTTP status code.
func (e *HTTPStatusError) StatusCode() int {
	return e.statusCode
}

// IsHTTPStatus reports whether err contains a webhook response with one of the supplied status codes.
func IsHTTPStatus(err error, statusCodes ...int) bool {
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return slices.Contains(statusCodes, statusErr.statusCode)
}

func normalizeConfig(cfg Config) (Config, error) {
	if err := validateURL(cfg.URL); err != nil {
		return Config{}, err
	}
	if err := validateHeaders(cfg.Headers); err != nil {
		return Config{}, err
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultWebhookTimeout
	}
	if cfg.Timeout < 0 {
		return Config{}, errors.New("timeout must be > 0")
	}
	var err error
	cfg.MaxRequestBytes, err = parseSize(cfg.MaxRequestBytes, "maxRequestBytes")
	if err != nil {
		return Config{}, err
	}
	cfg.MaxResponseBytes, err = parseSize(cfg.MaxResponseBytes, "maxResponseBytes")
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// NewClient validates config, resolves env-backed headers, and builds a client.
// NewFromConfig applies the same endpoint and timeout validation for each
// integration's webhook adapter; integration-specific route names stay with it.
func NewFromConfig(raw yaml.Node) (*Client, error) {
	cfg, err := ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	return NewClient(cfg)
}

func NewClient(cfg Config) (*Client, error) {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	baseURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, errors.Errorf("url: %w", err)
	}
	if err := applyHeaders(make(http.Header), cfg.Headers); err != nil {
		return nil, err
	}
	return &Client{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: cfg.Timeout,
			// Do not follow redirects: the https/loopback guard in validateURL only vets the configured
			// URL, and Go forwards custom (secret-bearing) headers on same-host redirects. A redirect to
			// an internal address would defeat both. Surface the 3xx to the caller instead (SSRF guard).
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		maxRequestBytes:  cfg.MaxRequestBytes,
		maxResponseBytes: cfg.MaxResponseBytes,
		headers:          maps.Clone(cfg.Headers),
	}, nil
}

// DoJSON performs one bounded exchange. Redirects remain disabled so secret headers
// cannot leave the configured endpoint's authority through a redirect response.
func (c *Client) DoJSON(ctx context.Context, method, route string, req, resp any) error {
	if resp == nil {
		return errors.New("webhook: response target is nil")
	}
	request, err := c.request(ctx, method, route, req)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return errors.Errorf("webhook: %s: %w", method, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1024))
		statusErr := &HTTPStatusError{statusCode: response.StatusCode, responseBody: string(body)}
		if readErr != nil {
			return errors.Join(statusErr, errors.Errorf("webhook: read error response: %w", readErr))
		}
		return statusErr
	}
	body, err := readLimited(response.Body, c.maxResponseBytes, "response body")
	if err != nil {
		return errors.Errorf("webhook: read response: %w", err)
	}
	return decodeResponse(body, resp)
}

func (c *Client) request(ctx context.Context, method, route string, payload any) (*http.Request, error) {
	endpoint, err := c.endpoint(route)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, errors.Errorf("webhook: encode request: %w", err)
		}
		if int64(len(encoded)) > c.maxRequestBytes {
			return nil, errors.Errorf("webhook: request body exceeds %d bytes", c.maxRequestBytes)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, errors.Errorf("webhook: build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if err := applyHeaders(request.Header, c.headers); err != nil {
		return nil, err
	}
	return request, nil
}

func decodeResponse(body []byte, target any) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return errors.New("webhook: empty response")
	}
	if err := parse.JSON(bytes.NewReader(body), target); err != nil {
		return errors.Errorf("webhook: decode response: %w", err)
	}
	return nil
}

// PostJSON sends req as JSON to the configured base URL and decodes a strict JSON response into resp.
func (c *Client) PostJSON(ctx context.Context, req, resp any) error {
	return c.DoJSON(ctx, http.MethodPost, "", req, resp)
}

// GetJSON sends a GET request and decodes a strict JSON response into resp.
func (c *Client) GetJSON(ctx context.Context, route string, resp any) error {
	return c.DoJSON(ctx, http.MethodGet, route, nil, resp)
}

func (c *Client) endpoint(route string) (string, error) {
	if route == "" {
		return c.baseURL.String(), nil
	}
	ref, err := url.Parse(route)
	if err != nil {
		return "", errors.Errorf("webhook: route: %w", err)
	}
	if ref.IsAbs() || ref.Host != "" {
		return "", errors.New("webhook: route must be relative")
	}
	base := *c.baseURL
	base.Path = path.Join(base.Path, ref.Path)
	if !path.IsAbs(base.Path) {
		base.Path = "/" + base.Path
	}
	base.RawQuery = ref.RawQuery
	base.Fragment = ""
	return base.String(), nil
}

func readLimited(r io.Reader, limit int64, label string) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.Errorf("%s exceeds %d bytes", label, limit)
	}
	return b, nil
}

// Post preserves typed null responses and returns no partially decoded value on
// failure. Callers validate the returned protocol decision before acting on it.
func Post[T any](ctx context.Context, client *Client, path string, input any) (T, error) {
	var output T
	if err := client.DoJSON(ctx, http.MethodPost, path, input, &output); err != nil {
		var zero T
		return zero, err
	}
	return output, nil
}
