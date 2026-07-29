package uniswapx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestOrderClientPagesOpenOrders(t *testing.T) {
	filler := common.HexToAddress("0x1111111111111111111111111111111111111111")
	firstHash := "0x" + strings.Repeat("1", 64)
	secondHash := "0x" + strings.Repeat("2", 64)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("x-api-key") != "secret" || r.Header.Get("x-beta-rfq") != "true" {
			t.Error("missing Uniswap API headers")
		}
		query := r.URL.Query()
		if query.Get("chainId") != "1" || query.Get("filler") != filler.Hex() ||
			query.Get("orderType") != orderTypeDutchV2 ||
			query.Get("orderStatus") != "open" || query.Get("sortKey") != "createdAt" ||
			query.Get("desc") != "true" || query.Get("sort") != "gt(0)" ||
			query.Get("limit") != "1000" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		if query.Get("cursor") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"orders": []any{testAPIOrder(firstHash)}, "cursor": "next"})
			return
		}
		if query.Get("cursor") != "next" {
			t.Errorf("unexpected cursor %q", query.Get("cursor"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"orders": []any{testAPIOrder(secondHash)}})
	}))
	defer server.Close()

	client := newOrderClient(OrderServerConfig{
		BaseURL: server.URL, PollInterval: time.Second, HTTPTimeout: time.Second, Beta: true,
	}, "secret")
	client.requestGap = 0
	orders, err := client.openOrders(context.Background(), 1, &filler)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(orders) != 2 || orders[0].OrderHash != firstHash || orders[1].OrderHash != secondHash {
		t.Fatalf("requests/orders = %d/%+v", requests, orders)
	}
}

func TestOrderClientDecodesCurrentDutchV2Response(t *testing.T) {
	fixture, err := os.ReadFile("testdata/orders-v2-current.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	client := newOrderClient(OrderServerConfig{BaseURL: server.URL, HTTPTimeout: time.Second}, "secret")
	client.requestGap = 0
	orders, err := client.openOrders(t.Context(), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 {
		t.Fatalf("orders = %d, want 1", len(orders))
	}
	order := orders[0]
	if order.Type != orderTypeDutchV2 || order.QuoteID != "quote-1" ||
		order.Input.StartAmount != "100" || len(order.Outputs) != 1 || order.Outputs[0].EndAmount != "200" {
		t.Fatalf("unexpected order: %+v", order)
	}
}

func TestOrderClientRejectsWrongVariant(t *testing.T) {
	order := map[string]any{
		"type": "Dutch_V3", "encodedOrder": "0x01", "signature": "0x" + strings.Repeat("1", 130),
		"orderHash": "0x" + strings.Repeat("3", 64), "orderStatus": "open", "chainId": 1,
		"swapper": "0x1111111111111111111111111111111111111111",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"orders": []any{order}})
	}))
	defer server.Close()

	client := newOrderClient(OrderServerConfig{BaseURL: server.URL, HTTPTimeout: time.Second}, "secret")
	client.requestGap = 0
	_, err := client.openOrders(t.Context(), 1, nil)
	if err == nil || !strings.Contains(err.Error(), "order 0 is not Dutch_V2") {
		t.Fatalf("err = %v", err)
	}
}

func TestOrderClientOrdersByHashBatches(t *testing.T) {
	hashes := make([]common.Hash, maxOrderHashBatch+1)
	txHashes := make(map[common.Hash]common.Hash, len(hashes))
	for i := range hashes {
		hashes[i] = common.BytesToHash([]byte{byte(i + 1)})
		txHashes[hashes[i]] = common.BytesToHash([]byte{byte(i + 101)})
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		query := r.URL.Query()
		requested := strings.Split(query.Get("orderHashes"), ",")
		if len(requested) > maxOrderHashBatch || query.Get("chainId") != "1" ||
			query.Get("orderType") != orderTypeDutchV2 ||
			query.Get("limit") != strconv.Itoa(len(requested)) ||
			query.Has("orderStatus") || query.Has("filler") || query.Has("sortKey") ||
			query.Has("sort") || query.Has("desc") || query.Has("cursor") {
			t.Errorf("unexpected hash query: %s", r.URL.RawQuery)
		}
		orders := make([]any, len(requested))
		for i, rawHash := range requested {
			hash := common.HexToHash(rawHash)
			orders[i] = testTerminalAPIOrder(hash, orderStatusFilled, txHashes[hash])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"orders": orders})
	}))
	defer server.Close()

	client := newOrderClient(OrderServerConfig{BaseURL: server.URL, HTTPTimeout: time.Second}, "secret")
	client.requestGap = 0
	terminals, err := client.ordersByHash(t.Context(), 1, hashes)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(terminals) != len(hashes) {
		t.Fatalf("requests/terminals = %d/%d, want 2/%d", requests, len(terminals), len(hashes))
	}
	for _, hash := range hashes {
		if terminal := terminals[hash]; terminal.Status != orderStatusFilled || terminal.TxHash != txHashes[hash] {
			t.Fatalf("terminal for %s = %+v", hash.Hex(), terminal)
		}
	}
}

func TestOrderClientOrdersByHashRejectsInvalidResponses(t *testing.T) {
	orderHash := common.HexToHash("0x1")
	txHash := common.HexToHash("0x2")
	tests := []struct {
		name   string
		orders func() []any
	}{
		{name: "missing", orders: func() []any { return nil }},
		{name: "duplicate", orders: func() []any {
			order := testTerminalAPIOrder(orderHash, orderStatusFilled, txHash)
			return []any{order, order}
		}},
		{name: "wrong variant", orders: func() []any {
			return []any{map[string]any{
				"type": "Dutch_V3", "encodedOrder": "0x01", "signature": "0x" + strings.Repeat("1", 130),
				"orderHash": orderHash.Hex(), "orderStatus": "open", "chainId": 1,
				"swapper": "0x1111111111111111111111111111111111111111",
			}}
		}},
		{name: "invalid order hash", orders: func() []any {
			order := testTerminalAPIOrder(orderHash, orderStatusFilled, txHash)
			order["orderHash"] = "0x1234"
			return []any{order}
		}},
		{name: "invalid status", orders: func() []any {
			order := testTerminalAPIOrder(orderHash, orderStatusFilled, txHash)
			order["orderStatus"] = "unknown"
			return []any{order}
		}},
		{name: "filled without transaction", orders: func() []any {
			return []any{testTerminalAPIOrder(orderHash, orderStatusFilled, common.Hash{})}
		}},
		{name: "open with transaction", orders: func() []any {
			return []any{testTerminalAPIOrder(orderHash, orderStatusOpen, txHash)}
		}},
		{name: "invalid transaction hash", orders: func() []any {
			order := testTerminalAPIOrder(orderHash, orderStatusFilled, txHash)
			order["txHash"] = "0x1234"
			return []any{order}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"orders": tc.orders()})
			}))
			defer server.Close()

			client := newOrderClient(OrderServerConfig{BaseURL: server.URL, HTTPTimeout: time.Second}, "secret")
			client.requestGap = 0
			if _, err := client.ordersByHash(t.Context(), 1, []common.Hash{orderHash}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestOrderClientLeavesFillerUnsetForPublicSources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orders" || r.URL.Query().Has("filler") {
			t.Errorf("unexpected public query: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"orders": []any{}})
	}))
	defer server.Close()
	client := newOrderClient(OrderServerConfig{BaseURL: server.URL, HTTPTimeout: time.Second}, "secret")
	client.requestGap = 0
	if _, err := client.openOrders(context.Background(), 1, nil); err != nil {
		t.Fatal(err)
	}
}

func TestOrderClientReadsRecentFillerHistoryAcrossStatuses(t *testing.T) {
	filler := common.HexToAddress("0x1111111111111111111111111111111111111111")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("chainId") != "1" || query.Get("filler") != filler.Hex() ||
			query.Get("orderType") != orderTypeDutchV2 ||
			query.Get("sortKey") != "createdAt" || query.Get("sort") != "gt(900)" ||
			query.Get("desc") != "true" || query.Has("orderStatus") {
			t.Errorf("unexpected history query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"orders": []any{}})
	}))
	defer server.Close()

	client := newOrderClient(OrderServerConfig{BaseURL: server.URL, HTTPTimeout: time.Second}, "secret")
	client.requestGap = 0
	orders, err := client.recentOrders(t.Context(), 1, filler, time.Unix(900, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 0 {
		t.Fatalf("history = %+v, want empty response", orders)
	}
}

func TestOrderClientRateLimitsEveryPage(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"orders": []any{}, "cursor": "next"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"orders": []any{}})
	}))
	defer server.Close()

	client := newOrderClient(OrderServerConfig{BaseURL: server.URL, HTTPTimeout: time.Second}, "secret")
	client.requestGap = 20 * time.Millisecond
	started := time.Now()
	client.lastRequest = started
	if _, err := client.openOrders(context.Background(), 1, nil); err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}
	if elapsed := client.lastRequest.Sub(started); elapsed < 2*client.requestGap {
		t.Fatalf("request slots elapsed = %s, want at least %s", elapsed, 2*client.requestGap)
	}
}

func TestOrderResponseLimitReturnsError(t *testing.T) {
	reader := &errorLimitReader{reader: strings.NewReader("abc"), remaining: 2}
	if _, err := io.ReadAll(reader); err == nil || !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf("err = %v", err)
	}
}

func testAPIOrder(hash string) map[string]any {
	return map[string]any{
		"type": "Dutch_V2", "encodedOrder": "0x01", "signature": "0x" + strings.Repeat("1", 130), "nonce": "1",
		"orderHash": hash, "orderStatus": "open", "chainId": 1,
		"swapper": "0x1111111111111111111111111111111111111111",
		"input": map[string]any{
			"token":       "0x3333333333333333333333333333333333333333",
			"startAmount": "1", "endAmount": "1",
		},
		"outputs": []any{map[string]any{
			"token":       "0x4444444444444444444444444444444444444444",
			"startAmount": "1", "endAmount": "1",
			"recipient": "0x5555555555555555555555555555555555555555",
		}},
		"cosignerData": map[string]any{
			"decayStartTime": 1, "decayEndTime": 2,
			"exclusiveFiller": "0x6666666666666666666666666666666666666666",
			"inputOverride":   "1", "outputOverrides": []string{"1"},
		},
		"cosignature": "0x" + strings.Repeat("2", 130),
		"createdAt":   1,
		"quoteId":     "quote-1", "requestId": "request-1",
	}
}

func testTerminalAPIOrder(hash common.Hash, status string, txHash common.Hash) map[string]any {
	order := map[string]any{
		"type": "Dutch_V2", "encodedOrder": "0x01", "signature": "0x" + strings.Repeat("1", 130),
		"orderHash": hash.Hex(), "orderStatus": status, "chainId": 1,
		"swapper": "0x1111111111111111111111111111111111111111",
	}
	if txHash != (common.Hash{}) {
		order["txHash"] = txHash.Hex()
	}
	return order
}
