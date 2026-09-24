// Package discounts wraps the LiquidLane signed-discounts API shared by solvers.
package discounts

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/rfqbackendinternal"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

const defaultTimeout = 10 * time.Second

// Terms is the signed discount the LiquidLane adapter's discountSwap verifies.
// Amounts/nonce stay as wire strings until a solver maps them into its executor-specific calldata.
type Terms struct {
	Adapter       string
	TokenToRedeem string
	Discount      string
	Signer        string
	Protocol      string
	Nonce         string
	Deadline      int64
}

// Resolved is the fresh signed discount returned at fill time. RequestID comes from X-Request-Id.
type Resolved struct {
	RequestID         string
	DiscountID        string
	Discount          Terms
	SignerSignature   string
	ProtocolDeadline  int64
	ProtocolSignature string
}

// ListItem is one currently advertised private discount.
type ListItem struct {
	DiscountID         string
	Adapter            string
	TokenToRedeem      string
	Collateral         string
	CollateralDecimals int
	Discount           string
	Signer             string
	Deadline           int64
	MaxRate            string
	MaxAssets          string
	// BlockNumber is the block maxAssets was read at; empty when the backend does not report it.
	BlockNumber string
}

// List projects discount data and the X-Request-Id header into solver-owned types.
type List struct {
	RequestID string
	Protocol  string
	Discounts []ListItem
}

// Client is a small adapter over the generated rfqbackendinternal client for the shared
// signed-discount endpoints, which live on the backend's /api-internal/v1 surface. The client is
// generated from that surface's own spec, so it addresses those paths directly.
type Client struct {
	api *rfqbackendinternal.APIClient
}

func NewClient(baseURL string) *Client {
	return NewClientWithHTTPClient(baseURL, &http.Client{
		Timeout:   defaultTimeout,
		Transport: observability.TraceTransport(nil, "rfq-discounts"),
	})
}

// NewClientWithHTTPClient permits callers to attach their existing request correlation transport.
func NewClientWithHTTPClient(baseURL string, client *http.Client) *Client {
	cfg := rfqbackendinternal.NewConfiguration()
	cfg.Servers = rfqbackendinternal.ServerConfigurations{{URL: strings.TrimRight(baseURL, "/")}}
	cfg.HTTPClient = client
	return &Client{api: rfqbackendinternal.NewAPIClient(cfg)}
}

// Resolve fetches a fresh signed discount for discountID.
//
// The backend response is an anyOf union of a single resolved discount and a batch. Solvers resolve one
// discountId at a time, so a batch is accepted only when it has exactly one entry.
func (c *Client) Resolve(ctx context.Context, discountID string) (*Resolved, error) {
	body := rfqbackendinternal.ResolveDiscountRequest{
		ResolveDiscountRequestOneOf: rfqbackendinternal.NewResolveDiscountRequestOneOf(discountID),
	}
	resp, httpResp, err := c.api.RFQAPI.ApiInternalV1DiscountsPost(ctx).ResolveDiscountRequest(body).Execute()
	closeResp(httpResp)
	if err != nil {
		if httpResp != nil && (httpResp.StatusCode == http.StatusNotFound || httpResp.StatusCode == http.StatusGone) {
			return nil, errors.Errorf("%w: %w", ErrUnavailable, err)
		}
		return nil, errors.Errorf("private discounts: resolve: %w", err)
	}
	if resp == nil {
		return nil, errors.New("private discounts: resolve: empty response")
	}
	if single := resp.ResolveDiscountResponseOneOf; single != nil {
		return resolvedFromSingle(httpResp.Header.Get("X-Request-Id"), single), nil
	}
	if batch := resp.ResolveDiscountResponseOneOf1; batch != nil {
		items := batch.GetDiscounts()
		if len(items) != 1 {
			return nil, errors.Errorf("private discounts: resolve: expected a single discount, got %d", len(items))
		}
		return resolvedFromSingle(httpResp.Header.Get("X-Request-Id"), &items[0]), nil
	}
	return nil, errors.New("private discounts: resolve: response matched neither discount shape")
}

func resolvedFromSingle(requestID string, s *rfqbackendinternal.ResolveDiscountResponseOneOf) *Resolved {
	return &Resolved{
		RequestID:         requestID,
		DiscountID:        s.GetDiscountId(),
		Discount:          termsFromModel(s.GetDiscount()),
		SignerSignature:   s.GetSignerSignature(),
		ProtocolDeadline:  s.GetProtocolDeadline(),
		ProtocolSignature: s.GetProtocolSignature(),
	}
}

func termsFromModel(d rfqbackendinternal.ResolveDiscountResponseOneOfDiscount) Terms {
	return Terms{
		Adapter:       d.GetAdapter(),
		TokenToRedeem: d.GetTokenToRedeem(),
		Discount:      d.GetDiscount(),
		Signer:        d.GetSigner(),
		Protocol:      d.GetProtocol(),
		Nonce:         d.GetNonce(),
		Deadline:      d.GetDeadline(),
	}
}

// ListDiscounts lists currently advertised private discounts.
func (c *Client) ListDiscounts(ctx context.Context) (*List, error) {
	resp, httpResp, err := c.api.RFQAPI.ApiInternalV1DiscountsGet(ctx).Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("private discounts: list: %w", err)
	}
	out := &List{}
	if resp == nil {
		return out, nil
	}
	out.RequestID = httpResp.Header.Get("X-Request-Id")
	out.Protocol = resp.GetProtocol()
	gen := resp.GetDiscounts()
	out.Discounts = make([]ListItem, 0, len(gen))
	for i := range gen {
		d := &gen[i]
		out.Discounts = append(out.Discounts, ListItem{
			DiscountID:         d.GetDiscountId(),
			Adapter:            d.GetAdapter(),
			TokenToRedeem:      d.GetTokenToRedeem(),
			Collateral:         d.GetCollateral(),
			CollateralDecimals: int(d.GetCollateralDecimals()),
			Discount:           d.GetDiscount(),
			Signer:             d.GetSigner(),
			Deadline:           d.GetDeadline(),
			MaxRate:            d.GetMaxRate(),
			MaxAssets:          d.GetMaxAssets(),
			BlockNumber:        d.GetBlockNumber(),
		})
	}
	return out, nil
}

func closeResp(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}
