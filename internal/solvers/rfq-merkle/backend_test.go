package rfqmerkle

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
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/v1/orders" || r.Method != http.MethodGet || r.URL.Query().Get("filler") != strings.ToLower(order.Filler.Hex()) || r.URL.Query().Get("orderStatus") != "open" || r.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected awarded-order request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"orders": []rfqbackend.OrdersResponseOrdersInner{row}})
	}))
	defer server.Close()
	rows, err := newBackendClient(server.URL).orders(t.Context(), strings.ToLower(order.Filler.Hex()))
	if err != nil || len(rows) != 1 || requests != 1 {
		t.Fatalf("requests=%d rows=%d error=%v", requests, len(rows), err)
	}
	decoded, _, err := decodeOrder(rows[0], order.Filler)
	if err != nil || decoded.Request.AmountIn.Cmp(order.Request.AmountIn) != 0 {
		t.Fatalf("awarded amount changed: %v", err)
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
