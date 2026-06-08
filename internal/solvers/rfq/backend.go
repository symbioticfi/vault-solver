package rfq

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-errors/errors"
)

// backendOrder is one order row from the RFQ backend (GET /orders). The optional fields
// (encodedOrder/protocolSignature/deadline/filler) are populated only for executable orders.
type backendOrder struct {
	Type              string       `json:"type"`
	OrderID           string       `json:"orderId"`
	OrderStatus       string       `json:"orderStatus"`
	QuoteID           string       `json:"quoteId"`
	Swapper           string       `json:"swapper"`
	TxHash            *string      `json:"txHash"`
	Nonce             string       `json:"nonce"`
	Input             backendToken `json:"input"`
	Outputs           []backendOut `json:"outputs"`
	EncodedOrder      *string      `json:"encodedOrder,omitempty"`
	ProtocolSignature *string      `json:"protocolSignature,omitempty"`
	Deadline          *int64       `json:"deadline,omitempty"`
	Filler            *string      `json:"filler,omitempty"`
}

type backendToken struct {
	Token  string `json:"token"`
	Amount string `json:"amount"`
}

type backendOut struct {
	Token     string `json:"token"`
	Amount    string `json:"amount"`
	Recipient string `json:"recipient"`
}

type ordersResponse struct {
	RequestID string         `json:"requestId"`
	Orders    []backendOrder `json:"orders"`
	Cursor    *string        `json:"cursor"`
}

// backendClient is a small HTTP client for the backend's filler-facing order endpoints.
type backendClient struct {
	baseURL string
	http    *http.Client
}

// maxBackendResponseBytes caps a backend JSON response (order/discount lists are small; this is a
// safety bound against an unbounded body, not a tuning knob).
const maxBackendResponseBytes = 8 << 20 // 8 MiB

func newBackendClient(baseURL string) *backendClient {
	// Trim any trailing slash so path joins (baseURL + "/orders") never double up.
	return &backendClient{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 10 * time.Second}}
}

// listOpenOrders lists open orders assigned to filler.
func (c *backendClient) listOpenOrders(ctx context.Context, filler string, limit int) ([]backendOrder, error) {
	resp, err := c.getOrders(ctx, url.Values{
		"filler":      {filler},
		"orderStatus": {"open"},
		"limit":       {strconv.Itoa(limit)},
	})
	if err != nil {
		return nil, err
	}
	return resp.Orders, nil
}

// getExecutableOrder reads the canonical open executable view for one order, or nil if absent.
func (c *backendClient) getExecutableOrder(ctx context.Context, orderID, filler string) (*backendOrder, error) {
	resp, err := c.getOrders(ctx, url.Values{
		"orderId":     {orderID},
		"filler":      {filler},
		"orderStatus": {"open"},
	})
	if err != nil {
		return nil, err
	}
	return first(resp.Orders), nil
}

// getOrder reads the backend view of one order regardless of status, or nil if absent.
func (c *backendClient) getOrder(ctx context.Context, orderID string) (*backendOrder, error) {
	resp, err := c.getOrders(ctx, url.Values{"orderId": {orderID}})
	if err != nil {
		return nil, err
	}
	return first(resp.Orders), nil
}

func (c *backendClient) getOrders(ctx context.Context, query url.Values) (*ordersResponse, error) {
	var out ordersResponse
	if err := c.doJSON(ctx, http.MethodGet, "/orders", query, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func first(orders []backendOrder) *backendOrder {
	if len(orders) == 0 {
		return nil
	}
	return &orders[0]
}

/* ───────── discounts (P3) ───────── */

// discountTerms is the signed discount the adapter's discount-swap verifies. Amounts/nonce are
// numeric/hex strings on the wire.
type discountTerms struct {
	Adapter       string `json:"adapter"`
	TokenToRedeem string `json:"tokenToRedeem"`
	Discount      string `json:"discount"`
	Signer        string `json:"signer"`
	Protocol      string `json:"protocol"`
	Nonce         string `json:"nonce"`
	Deadline      int64  `json:"deadline"`
}

// resolveDiscountResponse is the fresh, signed discount the backend issues at fill time.
type resolveDiscountResponse struct {
	RequestID         string        `json:"requestId"`
	DiscountID        string        `json:"discountId"`
	Discount          discountTerms `json:"discount"`
	SignerSignature   string        `json:"signerSignature"`
	ProtocolDeadline  int64         `json:"protocolDeadline"`
	ProtocolSignature string        `json:"protocolSignature"`
}

// discountListItem is one offered discount (GET /discounts), used during strategy recovery.
type discountListItem struct {
	DiscountID         string `json:"discountId"`
	Adapter            string `json:"adapter"`
	TokenToRedeem      string `json:"tokenToRedeem"`
	Collateral         string `json:"collateral"`
	CollateralDecimals int    `json:"collateralDecimals"`
	Discount           string `json:"discount"`
	Signer             string `json:"signer"`
	Deadline           int64  `json:"deadline"`
	MaxRate            string `json:"maxRate"`
	MaxAssets          string `json:"maxAssets"`
}

type discountsResponse struct {
	RequestID string             `json:"requestId"`
	Protocol  string             `json:"protocol"`
	Discounts []discountListItem `json:"discounts"`
}

// resolveDiscount fetches the fresh signed discount for a discountId (POST /discounts).
func (c *backendClient) resolveDiscount(ctx context.Context, discountID string) (*resolveDiscountResponse, error) {
	var out resolveDiscountResponse
	if err := c.doJSON(ctx, http.MethodPost, "/discounts", nil, map[string]string{"discountId": discountID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// listDiscounts lists currently-offered discounts (GET /discounts).
func (c *backendClient) listDiscounts(ctx context.Context) (*discountsResponse, error) {
	var out discountsResponse
	if err := c.doJSON(ctx, http.MethodGet, "/discounts", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// doJSON performs a JSON request to baseURL+path (with optional query), decoding the 2xx body into
// dst. A nil body sends no request body (GET).
func (c *backendClient) doJSON(ctx context.Context, method, path string, query url.Values, body, dst any) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return errors.Errorf("backend: bad base url: %w", err)
	}
	u.Path += path
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	var reqBody io.Reader // stays an untyped-nil interface when there's no body
	if body != nil {
		b, mErr := json.Marshal(body)
		if mErr != nil {
			return errors.Errorf("backend: marshal %s body: %w", path, mErr)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return errors.Errorf("backend: build %s request: %w", path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.Errorf("backend: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return errors.Errorf("backend: %s %s: status %d", method, path, resp.StatusCode)
	}
	// Cap the response so a hostile/buggy backend can't stream an unbounded body into the decoder.
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBackendResponseBytes)).Decode(dst); err != nil {
		return errors.Errorf("backend: decode %s: %w", path, err)
	}
	return nil
}
