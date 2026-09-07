package rfq

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/httpclient"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

const backendOrderStatusOpen = "open"

// backendOrder is one order row from the RFQ backend (GET /orders), projected from the generated
// rfqbackend.OrdersResponseOrdersInner. The optional fields (encodedOrder/protocolSignature/deadline/
// filler) are populated only for executable orders; the generated model exposes them as pointers, so
// they are copied here only when present (nil ⇒ absent), preserving the executable-payload nil checks
// in execution.go.
type backendOrder struct {
	Type              string
	OrderID           string
	OrderStatus       string
	QuoteID           string
	Swapper           string
	TxHash            *string
	Nonce             string
	Input             backendToken
	Outputs           []backendOut
	EncodedOrder      *string
	ProtocolSignature *string
	Deadline          *int64
	Filler            *string
}

type backendToken struct {
	Token  string
	Amount string
}

type backendOut struct {
	Token     string
	Amount    string
	Recipient string
}

// backendClient is a thin adapter over the generated rfqbackend client for filler-facing orders plus
// the shared private-discounts client. Used from the single execution goroutine.
type backendClient struct {
	*discounts.Client

	api *rfqbackend.APIClient
}

// newBackendClient builds a backend client rooted at baseURL. The generated client carries the
// `/api/v1` path prefix from the spec, so baseURL is the backend host root. A trailing slash is
// trimmed so the spec paths join cleanly. The 10s per-request timeout matches the prior hand-rolled
// client.
func newBackendClient(baseURL string) *backendClient {
	cfg := rfqbackend.NewConfiguration()
	cfg.Servers = rfqbackend.ServerConfigurations{{URL: strings.TrimRight(baseURL, "/")}}
	cfg.HTTPClient = &http.Client{
		Timeout: 10 * time.Second,
	}
	return &backendClient{api: rfqbackend.NewAPIClient(cfg), Client: discounts.NewClient(baseURL)}
}

// listOpenOrders retrieves the bounded working set assigned to this filler.
func (c *backendClient) listOpenOrders(ctx context.Context, filler string, limit int) ([]backendOrder, error) {
	request := c.api.RFQAPI.ApiV1OrdersGet(ctx).Filler(filler).OrderStatus(backendOrderStatusOpen).Limit(int64(limit))
	orders, err := c.fetch(request, "list open orders")
	if err != nil {
		return nil, err
	}
	if len(orders) > limit {
		return nil, errors.Errorf("backend: list open orders: got %d orders, limit %d", len(orders), limit)
	}
	for index, order := range orders {
		if strings.TrimSpace(order.OrderID) == "" || order.OrderStatus != backendOrderStatusOpen {
			return nil, errors.Errorf("backend: list open orders: row %d has invalid identity or status %q", index, order.OrderStatus)
		}
	}
	return orders, nil
}

func (c *backendClient) getExecutableOrder(ctx context.Context, orderID, filler string) (*backendOrder, error) {
	request := c.api.RFQAPI.ApiV1OrdersGet(ctx).OrderId(orderID).Filler(filler).OrderStatus(backendOrderStatusOpen)
	return c.lookup(request, orderID, "get executable order")
}

func (c *backendClient) getOrder(ctx context.Context, orderID string) (*backendOrder, error) {
	return c.lookup(c.api.RFQAPI.ApiV1OrdersGet(ctx).OrderId(orderID), orderID, "get order")
}

// A canonical lookup must be unambiguous and bound to the requested identity.
// An unrelated first row cannot resolve an outstanding order's lifecycle.
func (c *backendClient) lookup(request rfqbackend.ApiApiV1OrdersGetRequest, id, operation string) (*backendOrder, error) {
	orders, err := c.fetch(request, operation)
	if err != nil || len(orders) == 0 {
		return nil, err
	}
	if len(orders) != 1 || orders[0].OrderID != id {
		return nil, errors.Errorf("backend: %s: response does not identify exactly order %q", operation, id)
	}
	return &orders[0], nil
}

func (c *backendClient) fetch(request rfqbackend.ApiApiV1OrdersGetRequest, operation string) ([]backendOrder, error) {
	response, err := httpclient.Execute("backend: "+operation, request.Execute)

	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.Errorf("backend: %s: empty response", operation)
	}
	return ordersFromResponse(response), nil
}

// ordersFromResponse projects the generated orders response into the internal order rows. A nil
// response (no body) yields no orders. Optional fields are copied only when the generated model
// reports them set, preserving execution.go's incomplete-payload detection.
func ordersFromResponse(resp *rfqbackend.OrdersResponse) []backendOrder {
	if resp == nil {
		return nil
	}
	gen := resp.GetOrders()
	out := make([]backendOrder, 0, len(gen))
	for i := range gen {
		out = append(out, orderFromModel(&gen[i]))
	}
	return out
}

func orderFromModel(model *rfqbackend.OrdersResponseOrdersInner) backendOrder {
	order := backendOrder{Type: model.GetType(), OrderID: model.GetOrderId(), OrderStatus: model.GetOrderStatus(),
		QuoteID: model.GetQuoteId(), Swapper: model.GetSwapper(), Nonce: model.GetNonce(),
		Input: backendToken{Token: model.Input.GetToken(), Amount: model.Input.GetAmount()}, Outputs: make([]backendOut, len(model.Outputs))}
	for index := range model.Outputs {
		output := &model.Outputs[index]
		order.Outputs[index] = backendOut{Token: output.GetToken(), Amount: output.GetAmount(), Recipient: output.GetRecipient()}
	}
	// Optional pointers retain the generated model's presence information. Clone
	// their values so a later response mutation cannot change an executable record.
	for _, field := range []struct {
		get func() (*string, bool)
		to  **string
	}{
		{model.GetTxHashOk, &order.TxHash}, {model.GetEncodedOrderOk, &order.EncodedOrder},
		{model.GetProtocolSignatureOk, &order.ProtocolSignature}, {model.GetFillerOk, &order.Filler},
	} {
		if value, present := field.get(); present && value != nil {
			copied := *value
			*field.to = &copied
		}
	}
	if deadline, present := model.GetDeadlineOk(); present && deadline != nil {
		copied := *deadline
		order.Deadline = &copied
	}
	return order
}

// RFQ uses the common discount surface and preserves these aliases for its
// existing order fixtures and internal transport vocabulary.
type discountTerms = discounts.Terms
type resolveDiscountResponse = discounts.Resolved
type discountListItem = discounts.ListItem
type discountsResponse = discounts.List
