package rfq

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/httptransport"
)

const maxGeneratedResponseBytes = 8 << 20

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

// backendClient is a thin adapter over the generated rfqbackend client for the filler-facing order
// and discount endpoints. It owns no transport state of its own beyond the generated APIClient, whose
// HTTPClient carries the request timeout. Used from the single execution goroutine.
type backendClient struct {
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
		Transport: httptransport.LimitResponses(
			internalDiscountTransport{base: http.DefaultTransport}, maxGeneratedResponseBytes),
	}
	return &backendClient{api: rfqbackend.NewAPIClient(cfg)}
}

const (
	publicAPIPrefix   = "/api/v1"          // the spec's prefix, baked into the generated client
	internalAPIPrefix = "/api-internal/v1" // where the backend serves the internal-only discounts API
)

// internalDiscountTransport routes discount requests to the backend's internal API prefix. The discounts
// API is internal-only, but the generated client emits the public /api/v1/discount(s) paths from the
// spec; rather than regenerate the client for a deployment routing detail, we rewrite just those paths to
// /api-internal/v1/... at the transport layer. Orders and everything else pass through unchanged.
type internalDiscountTransport struct{ base http.RoundTripper }

func (t internalDiscountTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.Path, publicAPIPrefix+"/discount") {
		req = req.Clone(req.Context()) // RoundTrippers must not mutate the caller's request
		req.URL.Path = internalAPIPrefix + strings.TrimPrefix(req.URL.Path, publicAPIPrefix)
		req.URL.RawPath = "" // drop any cached encoding so the URL re-encodes from Path
	}
	return t.base.RoundTrip(req)
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
	req := c.api.RFQAPI.ApiV1OrdersGet(ctx).
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
	req := c.api.RFQAPI.ApiV1OrdersGet(ctx).
		OrderId(orderID).
		Filler(filler).
		OrderStatus("open")
	resp, httpResp, err := req.Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("backend: get executable order: %w", err)
	}
	order, err := selectOrder(ordersFromResponse(resp), orderID)
	if err != nil {
		return nil, errors.Errorf("backend: get executable order: %w", err)
	}
	return order, nil
}

// getOrder reads the backend view of one order regardless of status, or nil if absent.
func (c *backendClient) getOrder(ctx context.Context, orderID string) (*backendOrder, error) {
	resp, httpResp, err := c.api.RFQAPI.ApiV1OrdersGet(ctx).OrderId(orderID).Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("backend: get order: %w", err)
	}
	order, err := selectOrder(ordersFromResponse(resp), orderID)
	if err != nil {
		return nil, errors.Errorf("backend: get order: %w", err)
	}
	return order, nil
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
		Type:        o.GetType(),
		OrderID:     o.GetOrderId(),
		OrderStatus: o.GetOrderStatus(),
		QuoteID:     o.GetQuoteId(),
		Swapper:     o.GetSwapper(),
		Nonce:       o.GetNonce(),
		Input: backendToken{
			Token:  o.Input.GetToken(),
			Amount: o.Input.GetAmount(),
		},
	}
	// txHash is a nullable string in the schema; copy through whatever the backend reported (including
	// an explicit null) so reconcileTerminalStatus can validate it.
	if v, ok := o.GetTxHashOk(); ok {
		bo.TxHash = v
	}
	outs := o.GetOutputs()
	bo.Outputs = make([]backendOut, 0, len(outs))
	for i := range outs {
		bo.Outputs = append(bo.Outputs, backendOut{
			Token:     outs[i].GetToken(),
			Amount:    outs[i].GetAmount(),
			Recipient: outs[i].GetRecipient(),
		})
	}
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

func selectOrder(orders []backendOrder, orderID string) (*backendOrder, error) {
	var match *backendOrder
	for i := range orders {
		if orders[i].OrderID != orderID {
			continue
		}
		if match != nil {
			return nil, errors.Errorf("response contained duplicate order %q", orderID)
		}
		match = &orders[i]
	}
	if match != nil || len(orders) == 0 {
		return match, nil
	}
	return nil, errors.Errorf(
		"response for order %q contained %d non-matching row(s)", orderID, len(orders))
}

/* ───────── discounts (P3) ───────── */

// discountTerms is the signed discount the adapter's discount-swap verifies. Amounts/nonce are
// numeric/hex strings on the wire.
type discountTerms struct {
	Adapter       string
	TokenToRedeem string
	Discount      string
	Signer        string
	Protocol      string
	Nonce         string
	Deadline      int64
}

// resolveDiscountResponse is the fresh, signed discount the backend issues at fill time (the single
// shape of the backend's ResolveDiscountResponse anyOf union; see resolveDiscount).
type resolveDiscountResponse struct {
	RequestID         string
	DiscountID        string
	Discount          discountTerms
	SignerSignature   string
	ProtocolDeadline  int64
	ProtocolSignature string
}

// discountListItem is one offered discount (GET /discounts), used during strategy recovery.
type discountListItem struct {
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

type discountsResponse struct {
	RequestID string
	Protocol  string
	Discounts []discountListItem
}

// resolveDiscount fetches the fresh signed discount for a discountId (POST /discounts).
//
// The backend's ResolveDiscountResponse is an anyOf union of a single resolved discount (anyOf[0]) and
// a batch (anyOf[1]). The filler resolves one discountId at a time, so it expects — and requires — the
// single shape. If the backend returns the batch shape with exactly one entry, that lone entry is
// accepted (it carries the same signed fields); anything else (neither shape, or a batch with ≠1
// entries) is rejected so we never fill on an ambiguous resolution.
func (c *backendClient) resolveDiscount(ctx context.Context, discountID string) (*resolveDiscountResponse, error) {
	body := rfqbackend.NewApiV1DiscountsPostRequest()
	body.SetDiscountId(discountID)
	resp, httpResp, err := c.api.RFQAPI.ApiV1DiscountsPost(ctx).ApiV1DiscountsPostRequest(*body).Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("backend: resolve discount: %w", err)
	}
	if resp == nil {
		return nil, errors.New("backend: resolve discount: empty response")
	}
	if single := resp.ResolveDiscountResponseAnyOf; single != nil {
		return resolvedFromSingle(single), nil
	}
	if batch := resp.ResolveDiscountResponseAnyOf1; batch != nil {
		items := batch.GetDiscounts()
		if len(items) != 1 {
			return nil, errors.Errorf("backend: resolve discount: expected a single discount, got %d", len(items))
		}
		return resolvedFromBatchItem(batch.GetRequestId(), &items[0]), nil
	}
	return nil, errors.New("backend: resolve discount: response matched neither discount shape")
}

func resolvedFromSingle(s *rfqbackend.ResolveDiscountResponseAnyOf) *resolveDiscountResponse {
	return &resolveDiscountResponse{
		RequestID:         s.GetRequestId(),
		DiscountID:        s.GetDiscountId(),
		Discount:          termsFromModel(s.GetDiscount()),
		SignerSignature:   s.GetSignerSignature(),
		ProtocolDeadline:  int64(s.GetProtocolDeadline()),
		ProtocolSignature: s.GetProtocolSignature(),
	}
}

func resolvedFromBatchItem(requestID string, it *rfqbackend.ResolveDiscountResponseAnyOf1DiscountsInner) *resolveDiscountResponse {
	return &resolveDiscountResponse{
		RequestID:         requestID,
		DiscountID:        it.GetDiscountId(),
		Discount:          termsFromModel(it.GetDiscount()),
		SignerSignature:   it.GetSignerSignature(),
		ProtocolDeadline:  int64(it.GetProtocolDeadline()),
		ProtocolSignature: it.GetProtocolSignature(),
	}
}

func termsFromModel(d rfqbackend.PublishDiscountRequestDiscount) discountTerms {
	return discountTerms{
		Adapter:       d.GetAdapter(),
		TokenToRedeem: d.GetTokenToRedeem(),
		Discount:      d.GetDiscount(),
		Signer:        d.GetSigner(),
		Protocol:      d.GetProtocol(),
		Nonce:         d.GetNonce(),
		Deadline:      int64(d.GetDeadline()),
	}
}

// listDiscounts lists currently-offered discounts (GET /discounts).
func (c *backendClient) listDiscounts(ctx context.Context) (*discountsResponse, error) {
	resp, httpResp, err := c.api.RFQAPI.ApiV1DiscountsGet(ctx).Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("backend: list discounts: %w", err)
	}
	out := &discountsResponse{}
	if resp == nil {
		return out, nil
	}
	out.RequestID = resp.GetRequestId()
	out.Protocol = resp.GetProtocol()
	gen := resp.GetDiscounts()
	out.Discounts = make([]discountListItem, 0, len(gen))
	for i := range gen {
		d := &gen[i]
		out.Discounts = append(out.Discounts, discountListItem{
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
