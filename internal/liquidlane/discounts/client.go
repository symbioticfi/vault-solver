// Package discounts wraps the LiquidLane signed-discounts API shared by solvers.
package discounts

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/rfqbackend"
)

const (
	defaultTimeout    = 10 * time.Second
	publicAPIPrefix   = "/api/v1"
	internalAPIPrefix = "/api-internal/v1"
)

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

// Resolved is the fresh signed discount returned at fill time.
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
}

// List is the GET /discounts response projected into solver-owned types.
type List struct {
	RequestID string
	Protocol  string
	Discounts []ListItem
}

// Client is a small adapter over the generated rfqbackend client for the shared signed-discount
// endpoints. The generated client emits /api/v1/discount(s); rewriteTransport routes only those calls
// to the backend's /api-internal/v1 path.
type Client struct {
	api *rfqbackend.APIClient
}

func NewClient(baseURL string) *Client {
	cfg := rfqbackend.NewConfiguration()
	cfg.Servers = rfqbackend.ServerConfigurations{{URL: strings.TrimRight(baseURL, "/")}}
	cfg.HTTPClient = &http.Client{
		Timeout:   defaultTimeout,
		Transport: rewriteTransport{base: http.DefaultTransport},
	}
	return &Client{api: rfqbackend.NewAPIClient(cfg)}
}

// rewriteTransport routes private-discount requests to the backend's internal API prefix. Other
// generated-client requests pass through unchanged.
type rewriteTransport struct {
	base http.RoundTripper
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if index := strings.LastIndex(req.URL.Path, publicAPIPrefix+"/discount"); index >= 0 {
		req = req.Clone(req.Context())
		req.URL.Path = req.URL.Path[:index] +
			internalAPIPrefix +
			strings.TrimPrefix(req.URL.Path[index:], publicAPIPrefix)
		req.URL.RawPath = ""
	}
	return base.RoundTrip(req)
}

// Resolve fetches a fresh signed discount for discountID.
//
// The backend response is an anyOf union of a single resolved discount and a batch. Solvers resolve one
// discountId at a time, so a batch is accepted only when it has exactly one entry.
func (c *Client) Resolve(ctx context.Context, discountID string) (*Resolved, error) {
	body := rfqbackend.NewApiV1DiscountsPostRequest()
	body.SetDiscountId(discountID)
	resp, httpResp, err := c.api.RFQAPI.ApiV1DiscountsPost(ctx).ApiV1DiscountsPostRequest(*body).Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("private discounts: resolve: %w", err)
	}
	if resp == nil {
		return nil, errors.New("private discounts: resolve: empty response")
	}
	if single := resp.ResolveDiscountResponseAnyOf; single != nil {
		return resolvedFromSingle(single), nil
	}
	if batch := resp.ResolveDiscountResponseAnyOf1; batch != nil {
		items := batch.GetDiscounts()
		if len(items) != 1 {
			return nil, errors.Errorf("private discounts: resolve: expected a single discount, got %d", len(items))
		}
		return resolvedFromBatchItem(batch.GetRequestId(), &items[0]), nil
	}
	return nil, errors.New("private discounts: resolve: response matched neither discount shape")
}

func resolvedFromSingle(s *rfqbackend.ResolveDiscountResponseAnyOf) *Resolved {
	return &Resolved{
		RequestID:         s.GetRequestId(),
		DiscountID:        s.GetDiscountId(),
		Discount:          termsFromModel(s.GetDiscount()),
		SignerSignature:   s.GetSignerSignature(),
		ProtocolDeadline:  int64(s.GetProtocolDeadline()),
		ProtocolSignature: s.GetProtocolSignature(),
	}
}

func resolvedFromBatchItem(requestID string, it *rfqbackend.ResolveDiscountResponseAnyOf1DiscountsInner) *Resolved {
	return &Resolved{
		RequestID:         requestID,
		DiscountID:        it.GetDiscountId(),
		Discount:          termsFromModel(it.GetDiscount()),
		SignerSignature:   it.GetSignerSignature(),
		ProtocolDeadline:  int64(it.GetProtocolDeadline()),
		ProtocolSignature: it.GetProtocolSignature(),
	}
}

func termsFromModel(d rfqbackend.PublishDiscountRequestDiscount) Terms {
	return Terms{
		Adapter:       d.GetAdapter(),
		TokenToRedeem: d.GetTokenToRedeem(),
		Discount:      d.GetDiscount(),
		Signer:        d.GetSigner(),
		Protocol:      d.GetProtocol(),
		Nonce:         d.GetNonce(),
		Deadline:      int64(d.GetDeadline()),
	}
}

// ListDiscounts lists currently advertised private discounts.
func (c *Client) ListDiscounts(ctx context.Context) (*List, error) {
	resp, httpResp, err := c.api.RFQAPI.ApiV1DiscountsGet(ctx).Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("private discounts: list: %w", err)
	}
	out := &List{}
	if resp == nil {
		return out, nil
	}
	out.RequestID = resp.GetRequestId()
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
			Deadline:           int64(d.GetDeadline()),
			MaxRate:            d.GetMaxRate(),
			MaxAssets:          d.GetMaxAssets(),
		})
	}
	return out, nil
}

func closeResp(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}
