package rfq

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

type backendClient struct {
	api       *rfqbackend.APIClient
	discounts *discounts.Client
}

func newBackendClient(url string) *backendClient {
	cfg := rfqbackend.NewConfiguration()
	cfg.Servers = rfqbackend.ServerConfigurations{{URL: strings.TrimRight(url, "/")}}
	cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: observability.TraceTransport(http.DefaultTransport, Name)}
	return &backendClient{api: rfqbackend.NewAPIClient(cfg), discounts: discounts.NewClientWithHTTPClient(url, cfg.HTTPClient)}
}

func (b *backendClient) orders(ctx context.Context, filler string) ([]rfqbackend.OrdersResponseOrdersInner, error) {
	var rows []rfqbackend.OrdersResponseOrdersInner
	cursor := ""
	seen := make(map[string]bool)
	for {
		request := b.api.RFQAPI.ApiV1OrdersGet(ctx).Filler(filler).OrderStatus("open").Limit(100)
		if cursor != "" {
			request = request.Cursor(cursor)
		}
		result, response, err := request.Execute()
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if err != nil {
			return nil, errors.Errorf("rfq-merkle: list awarded orders: %w", err)
		}
		if result == nil || result.Orders == nil {
			return nil, errors.New("rfq-merkle: missing awarded orders")
		}
		rows = append(rows, result.Orders...)
		cursor = result.GetCursor()
		if cursor == "" {
			return rows, nil
		}
		if seen[cursor] {
			return nil, errors.New("rfq-merkle: repeated awarded orders cursor")
		}
		seen[cursor] = true
	}
}

func (b *backendClient) resolveDiscount(ctx context.Context, id string) (*discounts.Resolved, error) {
	return b.discounts.Resolve(ctx, id)
}

func (b *backendClient) listDiscounts(ctx context.Context) (*discounts.List, error) {
	return b.discounts.ListDiscounts(ctx)
}
