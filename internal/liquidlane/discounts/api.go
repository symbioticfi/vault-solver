// Package discounts owns the generated-client boundary for LiquidLane private discounts.
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
	privateAPITimeout = 10 * time.Second
	publicPrefix      = "/api/v1"
	privatePrefix     = "/api-internal/v1"
)

type Terms struct {
	Adapter       string
	TokenToRedeem string
	Discount      string
	Signer        string
	Protocol      string
	Nonce         string
	Deadline      int64
}

type Resolved struct {
	DiscountID        string
	Discount          Terms
	SignerSignature   string
	ProtocolDeadline  int64
	ProtocolSignature string
}

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

type List struct {
	Discounts []ListItem
}

type Client struct {
	generated *rfqbackend.APIClient
}

func NewClient(baseURL string) *Client {
	config := rfqbackend.NewConfiguration()
	config.Servers = rfqbackend.ServerConfigurations{{URL: strings.TrimRight(baseURL, "/")}}
	config.HTTPClient = &http.Client{
		Timeout:   privateAPITimeout,
		Transport: prefixTransport{next: http.DefaultTransport},
	}
	return &Client{generated: rfqbackend.NewAPIClient(config)}
}

func (client *Client) Resolve(ctx context.Context, discountID string) (*Resolved, error) {
	request := rfqbackend.NewApiV1DiscountsPostRequest()
	request.SetDiscountId(discountID)
	response, httpResponse, err := client.generated.RFQAPI.ApiV1DiscountsPost(ctx).
		ApiV1DiscountsPostRequest(*request).Execute()
	closeResponse(httpResponse)
	if err != nil {
		return nil, errors.Errorf("private discounts: resolve: %w", err)
	}
	if response == nil {
		return nil, errors.New("private discounts: resolve: empty response")
	}
	if single := response.ResolveDiscountResponseAnyOf; single != nil {
		return projectSingle(single), nil
	}
	if batch := response.ResolveDiscountResponseAnyOf1; batch != nil {
		items := batch.GetDiscounts()
		if len(items) != 1 {
			return nil, errors.Errorf("private discounts: resolve: expected a single discount, got %d", len(items))
		}
		return projectBatchItem(&items[0]), nil
	}
	return nil, errors.New("private discounts: resolve: response matched neither discount shape")
}

func (client *Client) ListDiscounts(ctx context.Context) (*List, error) {
	response, httpResponse, err := client.generated.RFQAPI.ApiV1DiscountsGet(ctx).Execute()
	closeResponse(httpResponse)
	if err != nil {
		return nil, errors.Errorf("private discounts: list: %w", err)
	}
	result := &List{}
	if response == nil {
		return result, nil
	}
	items := response.GetDiscounts()
	result.Discounts = make([]ListItem, len(items))
	for index := range items {
		item := &items[index]
		result.Discounts[index] = ListItem{
			DiscountID: item.GetDiscountId(), Adapter: item.GetAdapter(),
			TokenToRedeem: item.GetTokenToRedeem(), Collateral: item.GetCollateral(),
			CollateralDecimals: int(item.GetCollateralDecimals()), Discount: item.GetDiscount(),
			Signer: item.GetSigner(), Deadline: int64(item.GetDeadline()),
			MaxRate: item.GetMaxRate(), MaxAssets: item.GetMaxAssets(),
		}
	}
	return result, nil
}

type prefixTransport struct {
	next http.RoundTripper
}

func (transport prefixTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	next := transport.next
	if next == nil {
		next = http.DefaultTransport
	}
	marker := publicPrefix + "/discount"
	if index := strings.LastIndex(request.URL.Path, marker); index >= 0 {
		request = request.Clone(request.Context())
		request.URL.Path = request.URL.Path[:index] + privatePrefix +
			strings.TrimPrefix(request.URL.Path[index:], publicPrefix)
		request.URL.RawPath = ""
	}
	return next.RoundTrip(request)
}

func projectSingle(source *rfqbackend.ResolveDiscountResponseAnyOf) *Resolved {
	return &Resolved{
		DiscountID: source.GetDiscountId(), Discount: projectTerms(source.GetDiscount()),
		SignerSignature: source.GetSignerSignature(), ProtocolDeadline: int64(source.GetProtocolDeadline()),
		ProtocolSignature: source.GetProtocolSignature(),
	}
}

func projectBatchItem(source *rfqbackend.ResolveDiscountResponseAnyOf1DiscountsInner) *Resolved {
	return &Resolved{
		DiscountID: source.GetDiscountId(), Discount: projectTerms(source.GetDiscount()),
		SignerSignature: source.GetSignerSignature(), ProtocolDeadline: int64(source.GetProtocolDeadline()),
		ProtocolSignature: source.GetProtocolSignature(),
	}
}

func projectTerms(source rfqbackend.PublishDiscountRequestDiscount) Terms {
	return Terms{
		Adapter: source.GetAdapter(), TokenToRedeem: source.GetTokenToRedeem(), Discount: source.GetDiscount(),
		Signer: source.GetSigner(), Protocol: source.GetProtocol(), Nonce: source.GetNonce(),
		Deadline: int64(source.GetDeadline()),
	}
}

func closeResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}
