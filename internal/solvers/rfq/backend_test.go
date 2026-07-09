package rfq

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/httptransport"
)

// The generated rfqbackend client carries the spec's `/api/v1` prefix, so the backend client rooted at
// the httptest server URL hits `/api/v1/orders`; the discount transport rewrite (internalDiscountTransport)
// sends discount calls to `/api-internal/v1/discounts` instead (orders unchanged).

func TestBackendClient_ListOpenOrders(t *testing.T) {
	var gotPath, gotStatus, gotFiller, gotLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotStatus = r.URL.Query().Get("orderStatus")
		gotFiller = r.URL.Query().Get("filler")
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestId":"00000000-0000-0000-0000-000000000000",` +
			`"orders":[{"type":"Priority","orderId":"00000000-0000-0000-0000-0000000000a1","orderStatus":"open",` +
			`"quoteId":"00000000-0000-0000-0000-0000000000b1",` +
			`"swapper":"0x0000000000000000000000000000000000000099","txHash":null,"nonce":"0x1",` +
			`"input":{"token":"0x0000000000000000000000000000000000000001","amount":"1000"},` +
			`"outputs":[],"settledAmounts":[]}],"cursor":null}`))
	}))
	defer srv.Close()

	orders, err := newBackendClient(srv.URL).
		listOpenOrders(context.Background(), "0x0000000000000000000000000000000000000f11", 20)
	if err != nil {
		t.Fatalf("listOpenOrders: %v", err)
	}
	if gotPath != "/api/v1/orders" || gotStatus != "open" ||
		gotFiller != "0x0000000000000000000000000000000000000f11" || gotLimit != "20" {
		t.Fatalf("request = path %q status %q filler %q limit %q", gotPath, gotStatus, gotFiller, gotLimit)
	}
	if len(orders) != 1 ||
		orders[0].OrderID != "00000000-0000-0000-0000-0000000000a1" ||
		orders[0].QuoteID != "00000000-0000-0000-0000-0000000000b1" {
		t.Fatalf("orders = %+v", orders)
	}
	// txHash was an explicit null; it must round-trip as a nil pointer, not a garbage string.
	if orders[0].TxHash != nil {
		t.Fatalf("txHash = %v, want nil", *orders[0].TxHash)
	}
}

func TestBackendClient_ListOpenOrders_OversizedResponse(t *testing.T) {
	const responsePrefix = `{"requestId":"00000000-0000-0000-0000-000000000000","orders":[],"cursor":null}`
	const paddingBytes = maxGeneratedResponseBytes + 1

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(responsePrefix)+paddingBytes))
		_, _ = w.Write([]byte(responsePrefix))
		_, _ = w.Write([]byte(strings.Repeat(" ", paddingBytes)))
	}))
	defer srv.Close()

	_, err := newBackendClient(srv.URL).
		listOpenOrders(context.Background(), "0x0000000000000000000000000000000000000f11", 20)
	if !errors.Is(err, httptransport.ErrResponseTooLarge) {
		t.Fatalf("error = %v, want ErrResponseTooLarge", err)
	}
}

func TestBackendClient_NonOKStatusErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := newBackendClient(srv.URL).getOrder(context.Background(), "o1"); err == nil {
		t.Fatalf("expected an error on 500")
	}
}

func TestBackendClient_ResolveDiscount_Single(t *testing.T) {
	var gotPath, gotMethod, gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		gotID, _ = body["discountId"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestId":"00000000-0000-0000-0000-000000000000",` +
			`"discountId":"0x` + hash64 + `",` +
			`"discount":{"adapter":"0x0000000000000000000000000000000000000abc",` +
			`"tokenToRedeem":"0x0000000000000000000000000000000000000def",` +
			`"discount":"123","signer":"0x0000000000000000000000000000000000000aaa",` +
			`"protocol":"0x0000000000000000000000000000000000000bbb","nonce":"0x2","deadline":1900000000},` +
			`"signerSignature":"0xdead","protocolDeadline":1900000001,"protocolSignature":"0xbeef"}`))
	}))
	defer srv.Close()

	id := "0x" + hash64
	res, err := newBackendClient(srv.URL).resolveDiscount(context.Background(), id)
	if err != nil {
		t.Fatalf("resolveDiscount: %v", err)
	}
	if gotPath != "/api-internal/v1/discounts" || gotMethod != http.MethodPost || gotID != id {
		t.Fatalf("request = path %q method %q id %q", gotPath, gotMethod, gotID)
	}
	if res.Discount.Adapter != "0x0000000000000000000000000000000000000abc" ||
		res.Discount.Discount != "123" || res.Discount.Nonce != "0x2" ||
		res.Discount.Deadline != 1900000000 {
		t.Fatalf("discount terms = %+v", res.Discount)
	}
	if res.SignerSignature != "0xdead" || res.ProtocolSignature != "0xbeef" || res.ProtocolDeadline != 1900000001 {
		t.Fatalf("resolved = %+v", res)
	}
}

func TestBackendClient_ResolveDiscount_BatchSingleEntryAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestId":"00000000-0000-0000-0000-000000000000",` +
			`"discounts":[{"discountId":"0x` + hash64 + `",` +
			`"discount":{"adapter":"0x0000000000000000000000000000000000000abc",` +
			`"tokenToRedeem":"0x0000000000000000000000000000000000000def",` +
			`"discount":"123","signer":"0x0000000000000000000000000000000000000aaa",` +
			`"protocol":"0x0000000000000000000000000000000000000bbb","nonce":"0x2","deadline":1900000000},` +
			`"signerSignature":"0xdead","protocolDeadline":1900000001,"protocolSignature":"0xbeef"}]}`))
	}))
	defer srv.Close()

	res, err := newBackendClient(srv.URL).resolveDiscount(context.Background(), "0x"+hash64)
	if err != nil {
		t.Fatalf("resolveDiscount (batch): %v", err)
	}
	if res.Discount.Adapter != "0x0000000000000000000000000000000000000abc" || res.SignerSignature != "0xdead" {
		t.Fatalf("resolved from batch = %+v", res)
	}
}

func TestBackendClient_ResolveDiscount_BatchMultipleRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entry := `{"discountId":"0x` + hash64 + `",` +
			`"discount":{"adapter":"0x0000000000000000000000000000000000000abc",` +
			`"tokenToRedeem":"0x0000000000000000000000000000000000000def",` +
			`"discount":"1","signer":"0x0000000000000000000000000000000000000aaa",` +
			`"protocol":"0x0000000000000000000000000000000000000bbb","nonce":"0x2","deadline":1900000000},` +
			`"signerSignature":"0xdead","protocolDeadline":1900000001,"protocolSignature":"0xbeef"}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestId":"00000000-0000-0000-0000-000000000000","discounts":[` +
			entry + `,` + entry + `]}`))
	}))
	defer srv.Close()

	if _, err := newBackendClient(srv.URL).resolveDiscount(context.Background(), "0x"+hash64); err == nil {
		t.Fatalf("expected an error when the backend resolves more than one discount")
	}
}

func TestBackendClient_ListDiscounts(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestId":"00000000-0000-0000-0000-000000000000",` +
			`"protocol":"0x0000000000000000000000000000000000000bbb","discounts":[` +
			`{"discountId":"0x` + hash64 + `","adapter":"0x0000000000000000000000000000000000000abc",` +
			`"tokenToRedeem":"0x0000000000000000000000000000000000000def",` +
			`"collateral":"0x0000000000000000000000000000000000000c01","collateralDecimals":6,` +
			`"discount":"10","signer":"0x0000000000000000000000000000000000000aaa","deadline":1900000000,` +
			`"maxRate":"1000000","maxAssets":"5000"}]}`))
	}))
	defer srv.Close()

	resp, err := newBackendClient(srv.URL).listDiscounts(context.Background())
	if err != nil {
		t.Fatalf("listDiscounts: %v", err)
	}
	if gotPath != "/api-internal/v1/discounts" {
		t.Fatalf("path = %q", gotPath)
	}
	if len(resp.Discounts) != 1 || resp.Discounts[0].CollateralDecimals != 6 ||
		resp.Discounts[0].MaxAssets != "5000" || resp.Discounts[0].Deadline != 1900000000 {
		t.Fatalf("discounts = %+v", resp.Discounts)
	}
}

// hash64 is the 64-hex-char body of a 0x-prefixed discountId used across the backend client tests.
const hash64 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
