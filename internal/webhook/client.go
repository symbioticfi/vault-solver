package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/parse"
)

const (
	defaultWebhookTimeout      = 5 * time.Second
	defaultWebhookMaxBodyBytes = 1 << 20
)

// HeaderValue is one configured HTTP header. Value is for non-secret literals; Env names an env var
// whose value is resolved when the webhook client is built.
type HeaderValue struct {
	Value string `yaml:"value"`
	Env   string `yaml:"env"`
}

// Config is the shared HTTP transport config for webhook-style strategies.
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

// ParseConfig decodes a strict webhook config.
func ParseConfig(node yaml.Node) (Config, error) {
	var raw rawConfig
	if err := parse.DecodeStrict(node, &raw); err != nil {
		return Config{}, err
	}
	if raw.URL == "" {
		return Config{}, errors.New("url is required")
	}
	if err := validateURL(raw.URL); err != nil {
		return Config{}, err
	}
	timeout, err := parse.Duration(raw.Timeout, defaultWebhookTimeout, "timeout")
	if err != nil {
		return Config{}, err
	}
	for name, h := range raw.Headers {
		switch {
		case name == "":
			return Config{}, errors.New("headers: empty header name")
		case h.Value != "" && h.Env != "":
			return Config{}, errors.Errorf("headers.%s: set value or env, not both", name)
		case h.Value == "" && h.Env == "":
			return Config{}, errors.Errorf("headers.%s: value or env is required", name)
		}
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
	if n < 0 {
		return 0, errors.Errorf("%s: invalid byte size %d", field, n)
	}
	return n, nil
}

// Client posts JSON strategy requests to an external decider.
type Client struct {
	url              string
	client           *http.Client
	maxRequestBytes  int64
	maxResponseBytes int64
	headers          map[string]string
}

func normalizeConfig(cfg Config) (Config, error) {
	if err := validateURL(cfg.URL); err != nil {
		return Config{}, err
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

// NewClient resolves env-backed headers and builds a client.
func NewClient(cfg Config) (*Client, error) {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	headers := make(map[string]string, len(cfg.Headers))
	for name, h := range cfg.Headers {
		v := h.Value
		if h.Env != "" {
			v = os.Getenv(h.Env)
			if v == "" {
				return nil, errors.Errorf("headers.%s: env %q is empty", name, h.Env)
			}
		}
		headers[name] = v
	}
	return &Client{
		url: cfg.URL,
		client: &http.Client{
			Timeout: cfg.Timeout,
			// Do not follow redirects: the https/loopback guard in validateURL only vets the configured
			// URL, and Go forwards custom (secret-bearing) headers on same-host redirects. A redirect to
			// an internal address would defeat both. Surface the 3xx to the caller instead (SSRF guard).
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		maxRequestBytes:  cfg.MaxRequestBytes,
		maxResponseBytes: cfg.MaxResponseBytes,
		headers:          headers,
	}, nil
}

// PostJSON sends req as JSON and decodes a strict JSON response into resp.
func (c *Client) PostJSON(ctx context.Context, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return errors.Errorf("webhook: encode request: %w", err)
	}
	if int64(len(body)) > c.maxRequestBytes {
		return errors.Errorf("webhook: request body exceeds %d bytes", c.maxRequestBytes)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return errors.Errorf("webhook: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return errors.Errorf("webhook: post: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(httpResp.Body, 1024))
		return errors.Errorf("webhook: status %d: %s", httpResp.StatusCode, string(b))
	}
	b, err := readLimited(httpResp.Body, c.maxResponseBytes, "response body")
	if err != nil {
		return errors.Errorf("webhook: read response: %w", err)
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return errors.New("webhook: empty response")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(resp); err != nil {
		return errors.Errorf("webhook: decode response: %w", err)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return errors.Errorf("webhook: decode response: %w", err)
	}
	return nil
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
