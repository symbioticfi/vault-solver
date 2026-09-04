package rfq

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

// backendOrder is one order row from the RFQ backend (GET /orders), projected from the generated
// rfqbackend.OrdersResponseOrdersInner. The optional fields (encodedOrder/protocolSignature/deadline/
// filler) are populated only for executable orders; the generated model exposes them as pointers, so
// they are copied here only when present (nil ⇒ absent), preserving the executable-payload nil checks
// in execution.go.
type backendOrder struct {
	OrderID           string
	OrderStatus       string
	QuoteID           string
	TxHash            *string
	Outputs           []backendOut
	EncodedOrder      *string
	ProtocolSignature *string
	Deadline          *int64
	Filler            *string
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

	orders *rfqbackend.APIClient
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
	return &backendClient{orders: rfqbackend.NewAPIClient(cfg), Client: discounts.NewClient(baseURL)}
}

// closeResp drains and closes the HTTP response body. The generated client already reads the body
// fully into memory and closes it before returning, so this is belt-and-suspenders: it satisfies the
// "response body must be closed" contract and is a harmless no-op on the already-closed body.
func closeResp(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

// listOpenOrders lists open orders assigned to filler.
func (c *backendClient) listOpenOrders(ctx context.Context, filler string, limit int) ([]backendOrder, error) {
	// limit is the operator-bounded poll size (orderLimit); the spec caps it at 100, so the int→int32
	// narrowing is safe.
	req := c.orders.RFQAPI.ApiV1OrdersGet(ctx).
		Filler(filler).
		OrderStatus("open").
		Limit(int32(limit))
	resp, httpResp, err := req.Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("backend: list open orders: %w", err)
	}
	return ordersFromResponse(resp), nil
}

// getExecutableOrder reads the canonical open executable view for one order, or nil if absent.
func (c *backendClient) getExecutableOrder(ctx context.Context, orderID, filler string) (*backendOrder, error) {
	req := c.orders.RFQAPI.ApiV1OrdersGet(ctx).
		OrderId(orderID).
		Filler(filler).
		OrderStatus("open")
	resp, httpResp, err := req.Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("backend: get executable order: %w", err)
	}
	return first(ordersFromResponse(resp)), nil
}

// getOrder reads the backend view of one order regardless of status, or nil if absent.
func (c *backendClient) getOrder(ctx context.Context, orderID string) (*backendOrder, error) {
	resp, httpResp, err := c.orders.RFQAPI.ApiV1OrdersGet(ctx).OrderId(orderID).Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("backend: get order: %w", err)
	}
	return first(ordersFromResponse(resp)), nil
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

func orderFromModel(o *rfqbackend.OrdersResponseOrdersInner) backendOrder {
	bo := backendOrder{
		OrderID:     o.GetOrderId(),
		OrderStatus: o.GetOrderStatus(),
		QuoteID:     o.GetQuoteId(),
	}
	// txHash is a nullable string in the schema; copy through whatever the backend reported (including
	// an explicit null) so reconcileTerminalStatus can validate it.
	if v, ok := o.GetTxHashOk(); ok {
		bo.TxHash = v
	}
	bo.Outputs = projectBackendOutputs(o.GetOutputs())
	// Executable-only optional fields: copy only when present so a non-executable row keeps them nil
	// and executableFromBackend rejects it as incomplete.
	if v, ok := o.GetEncodedOrderOk(); ok {
		bo.EncodedOrder = v
	}
	if v, ok := o.GetProtocolSignatureOk(); ok {
		bo.ProtocolSignature = v
	}
	if v, ok := o.GetDeadlineOk(); ok {
		d := int64(*v)
		bo.Deadline = &d
	}
	if v, ok := o.GetFillerOk(); ok {
		bo.Filler = v
	}
	return bo
}

func projectBackendOutputs(outputs []rfqbackend.PublicQuoteResponseQuoteOrderInfoOutputsInner) []backendOut {
	projected := make([]backendOut, len(outputs))
	for index := range outputs {
		projected[index] = backendOut{
			Token: outputs[index].GetToken(), Amount: outputs[index].GetAmount(),
			Recipient: outputs[index].GetRecipient(),
		}
	}
	return projected
}

func first(orders []backendOrder) *backendOrder {
	if len(orders) == 0 {
		return nil
	}
	return &orders[0]
}
