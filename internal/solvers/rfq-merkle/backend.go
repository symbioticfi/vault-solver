package rfqmerkle

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
	result, response, err := b.api.RFQAPI.ApiV1OrdersGet(ctx).Filler(filler).OrderStatus("open").Limit(100).Execute()
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil || result == nil {
		return nil, errors.New("rfq-merkle: list awarded orders failed")
	}
	return result.Orders, nil
}

func (b *backendClient) resolveDiscount(ctx context.Context, id string) (*discounts.Resolved, error) {
	return b.discounts.Resolve(ctx, id)
}

func (b *backendClient) listDiscounts(ctx context.Context) (*discounts.List, error) {
	return b.discounts.ListDiscounts(ctx)
}
