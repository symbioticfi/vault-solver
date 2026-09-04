package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"
)

type Client struct {
	baseURL          *url.URL
	client           *http.Client
	maxRequestBytes  int64
	maxResponseBytes int64
	headers          map[string]string
}

func NewClient(config Config) (*Client, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	baseURL, err := url.Parse(normalized.URL)
	if err != nil {
		return nil, errors.Errorf("url: %w", err)
	}
	headers, err := resolveHeaders(normalized.Headers)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: normalized.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		maxRequestBytes:  normalized.MaxRequestBytes,
		maxResponseBytes: normalized.MaxResponseBytes,
		headers:          headers,
	}, nil
}

func NewClientFromConfig(node yaml.Node) (*Client, error) {
	config, err := ParseConfig(node)
	if err != nil {
		return nil, err
	}
	return NewClient(config)
}

func (c *Client) DoJSON(ctx context.Context, route string, request, response any) error {
	if response == nil {
		return errors.New("webhook: response target is nil")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return errors.Errorf("webhook: encode request: %w", err)
	}
	if int64(len(payload)) > c.maxRequestBytes {
		return errors.Errorf("webhook: request body exceeds %d bytes", c.maxRequestBytes)
	}
	endpoint, err := c.endpoint(route)
	if err != nil {
		return err
	}
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		return errors.Errorf("webhook: build request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	for name, value := range c.headers {
		httpRequest.Header.Set(name, value)
	}
	httpResponse, err := c.client.Do(httpRequest)
	if err != nil {
		return errors.Errorf("webhook: POST: %w", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(httpResponse.Body, 1024))
		return &HTTPStatusError{statusCode: httpResponse.StatusCode, responseBody: string(body)}
	}
	body, err := readLimited(httpResponse.Body, c.maxResponseBytes, "response body")
	if err != nil {
		return errors.Errorf("webhook: read response: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return errors.New("webhook: empty response")
	}
	return decodeResponse(body, response)
}

func decodeResponse(body []byte, response any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(response); err != nil {
		return errors.Errorf("webhook: decode response: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return errors.Errorf("webhook: decode response: %w", err)
	}
	return nil
}

func (c *Client) PostJSON(ctx context.Context, request, response any) error {
	return c.DoJSON(ctx, "", request, response)
}

func (c *Client) endpoint(route string) (string, error) {
	if route == "" {
		return c.baseURL.String(), nil
	}
	reference, err := url.Parse(route)
	if err != nil {
		return "", errors.Errorf("webhook: route: %w", err)
	}
	if reference.IsAbs() || reference.Host != "" {
		return "", errors.New("webhook: route must be relative")
	}
	endpoint := *c.baseURL
	endpoint.Path = path.Join(endpoint.Path, reference.Path)
	if !path.IsAbs(endpoint.Path) {
		endpoint.Path = "/" + endpoint.Path
	}
	endpoint.RawQuery = reference.RawQuery
	endpoint.Fragment = ""
	return endpoint.String(), nil
}

func readLimited(reader io.Reader, limit int64, label string) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.Errorf("%s exceeds %d bytes", label, limit)
	}
	return body, nil
}
