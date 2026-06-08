package rfq

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBackendClient_ListOpenOrders(t *testing.T) {
	var gotPath, gotStatus, gotFiller string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotStatus = r.URL.Query().Get("orderStatus")
		gotFiller = r.URL.Query().Get("filler")
		_, _ = w.Write([]byte(`{"requestId":"00000000-0000-0000-0000-000000000000",` +
			`"orders":[{"type":"Priority","orderId":"o1","orderStatus":"open","quoteId":"q1",` +
			`"swapper":"0x0000000000000000000000000000000000000099","txHash":null,"nonce":"0x1",` +
			`"input":{"token":"0x0000000000000000000000000000000000000001","amount":"1000"},` +
			`"outputs":[],"settledAmounts":[]}],"cursor":null}`))
	}))
	defer srv.Close()

	orders, err := newBackendClient(srv.URL).listOpenOrders(context.Background(), "0xfiller", 20)
	if err != nil {
		t.Fatalf("listOpenOrders: %v", err)
	}
	if gotPath != "/orders" || gotStatus != "open" || gotFiller != "0xfiller" {
		t.Fatalf("request = path %q status %q filler %q", gotPath, gotStatus, gotFiller)
	}
	if len(orders) != 1 || orders[0].OrderID != "o1" || orders[0].QuoteID != "q1" {
		t.Fatalf("orders = %+v", orders)
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
