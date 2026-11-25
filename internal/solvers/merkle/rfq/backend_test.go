package rfq

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
)

func TestBackendReadsAwardedExecutableOrders(t *testing.T) {
	order, _, _ := connectorExecutionAward(common.HexToAddress("0x10"))
	row := awardRow(t, order)
	filler := strings.ToLower(order.Filler.Hex())
	for _, repeat := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			query := r.URL.Query()
			if r.URL.Path != "/api/v1/orders" || r.Method != http.MethodGet || query.Get("filler") != filler ||
				query.Get("orderStatus") != "open" || query.Get("limit") != "100" || query.Get("sortKey") != "createdAt" || query.Get("sort") != "asc" {
				t.Errorf("unexpected award request: %s %s", r.Method, r.URL)
			}
			w.Header().Set("Content-Type", "application/json")
			cursor := ""
			if query.Get("cursor") == "" || repeat {
				cursor = "page-two"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"orders": []rfqbackend.OrdersResponseOrdersInner{row}, "cursor": cursor})
		}))
		rows, err := newBackendClient(server.URL).orders(t.Context(), filler)
		server.Close()
		if repeat {
			if err == nil || len(rows) != 0 {
				t.Fatalf("repeated cursor returned partial results: %v", err)
			}
			continue
		}
		if err != nil || len(rows) != 2 {
			t.Fatalf("expected both award pages, got %d: %v", len(rows), err)
		}
		decoded, _, err := decodeOrder(rows[0], order.Filler)
		if err != nil || decoded.Request.AmountIn.Cmp(order.Request.AmountIn) != 0 {
			t.Fatalf("awarded amount changed: %v", err)
		}
	}
}

func TestBackendRefusesRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/unexpected" {
			t.Error("followed backend redirect")
		}
		http.Redirect(w, r, "/unexpected", http.StatusFound)
	}))
	defer server.Close()
	if _, err := newBackendClient(server.URL).orders(t.Context(), vlt.Hex()); err == nil {
		t.Fatal("redirect accepted")
	}
}
