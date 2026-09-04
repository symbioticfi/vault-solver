// Package webhook owns the protocol-neutral strict JSON strategy transport.
package webhook

import (
	"net"
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

type Config struct {
	URL              string
	Timeout          time.Duration
	MaxRequestBytes  int64
	MaxResponseBytes int64
	Headers          map[string]HeaderValue
}

type HeaderValue struct {
	Value string `yaml:"value"`
	Env   string `yaml:"env"`
}

type rawConfig struct {
	URL              string                 `yaml:"url"`
	Timeout          string                 `yaml:"timeout"`
	MaxRequestBytes  int64                  `yaml:"maxRequestBytes"`
	MaxResponseBytes int64                  `yaml:"maxResponseBytes"`
	Headers          map[string]HeaderValue `yaml:"headers"`
}

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

func ValidateConfig(node yaml.Node) error {
	_, err := ParseConfig(node)
	return err
}

func normalizeConfig(config Config) (Config, error) {
	if err := validateURL(config.URL); err != nil {
		return Config{}, err
	}
	if err := validateHeaders(config.Headers); err != nil {
		return Config{}, err
	}
	if config.Timeout == 0 {
		config.Timeout = defaultWebhookTimeout
	}
	if config.Timeout < 0 {
		return Config{}, errors.New("timeout must be > 0")
	}
	var err error
	config.MaxRequestBytes, err = bodyLimit(config.MaxRequestBytes, "maxRequestBytes")
	if err != nil {
		return Config{}, err
	}
	config.MaxResponseBytes, err = bodyLimit(config.MaxResponseBytes, "maxResponseBytes")
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

func validateURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.Errorf("url: %w", err)
	}
	if parsed.Host == "" {
		return errors.New("url: host is required")
	}
	if parsed.Scheme == "https" || parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()) {
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

func bodyLimit(value int64, field string) (int64, error) {
	if value == 0 {
		return defaultWebhookMaxBodyBytes, nil
	}
	if value < 0 {
		return 0, errors.Errorf("%s: invalid byte size %d", field, value)
	}
	return value, nil
}

func validateHeaders(headers map[string]HeaderValue) error {
	for name, header := range headers {
		switch {
		case name == "":
			return errors.New("headers: empty header name")
		case header.Value != "" && header.Env != "":
			return errors.Errorf("headers.%s: set value or env, not both", name)
		case header.Value == "" && header.Env == "":
			return errors.Errorf("headers.%s: value or env is required", name)
		}
	}
	return nil
}

func resolveHeaders(headers map[string]HeaderValue) (map[string]string, error) {
	resolved := make(map[string]string, len(headers))
	for name, header := range headers {
		value := header.Value
		if header.Env != "" {
			value = os.Getenv(header.Env)
			if value == "" {
				return nil, errors.Errorf("headers.%s: env %q is empty", name, header.Env)
			}
		}
		resolved[name] = value
	}
	return resolved, nil
}
