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
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		query := r.URL.Query()
		if r.URL.Path != "/api/v1/orders" || r.Method != http.MethodGet || query.Get("filler") != strings.ToLower(order.Filler.Hex()) ||
			query.Get("orderStatus") != "open" || query.Get("limit") != "100" || query.Get("sortKey") != "createdAt" || query.Get("sort") != "asc" {
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

func TestBackendReadsAllAwardPages(t *testing.T) {
	for _, tc := range []struct {
		name   string
		repeat bool
	}{
		{name: "complete snapshot"},
		{name: "repeated cursor", repeat: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Query().Get("cursor") {
				case "":
					_, _ = w.Write([]byte(`{"orders":[{"orderId":"first"}],"cursor":"page-two"}`))
				case "page-two":
					if tc.repeat {
						_, _ = w.Write([]byte(`{"orders":[],"cursor":"page-two"}`))
						return
					}
					_, _ = w.Write([]byte(`{"orders":[{"orderId":"second"}],"cursor":null}`))
				default:
					t.Error("unexpected award cursor")
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			rows, err := newBackendClient(server.URL).orders(t.Context(), vlt.Hex())
			if tc.repeat {
				if err == nil || len(rows) != 0 {
					t.Fatalf("partial result escaped repeated cursor: %v %v", rows, err)
				}
				return
			}
			if err != nil || len(rows) != 2 || rows[0].OrderId != "first" || rows[1].OrderId != "second" {
				t.Fatalf("incomplete award snapshot: %v %v", rows, err)
			}
		})
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
