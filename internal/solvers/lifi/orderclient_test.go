package lifi

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/lifiorder"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

func TestOrderClientSubmitQuotes(t *testing.T) {
	var gotHeader string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quotes/submit" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotHeader = r.Header.Get("x-api-key")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","quotesAdded":2}`))
	}))
	defer srv.Close()

	client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
	err := client.submitQuotes(context.Background(), []types.Quote{submitQuotesTestQuote()})
	if err != nil {
		t.Fatalf("submitQuotes: %v", err)
	}
	if gotHeader != "test-key" {
		t.Fatalf("x-api-key = %q", gotHeader)
	}

	quotes := gotBody["quotes"].([]any)
	q := quotes[0].(map[string]any)
	if q["fromChain"] != "11155111" || q["toChain"] != "11155111" {
		t.Fatalf("chains = %v/%v", q["fromChain"], q["toChain"])
	}
	if q["fromAsset"] != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("fromAsset = %v", q["fromAsset"])
	}
	if q["fromDecimals"] != float64(6) || q["toDecimals"] != float64(18) {
		t.Fatalf("decimals = %v/%v", q["fromDecimals"], q["toDecimals"])
	}
	if q["exclusiveFor"] != "0x3333333333333333333333333333333333333333" {
		t.Fatalf("exclusiveFor = %v", q["exclusiveFor"])
	}
	ranges := q["ranges"].([]any)
	if len(ranges) != 2 {
		t.Fatalf("ranges = %d, want 2", len(ranges))
	}
	rng := ranges[0].(map[string]any)
	if rng["minAmount"] != "1" || rng["maxAmount"] != "1000000" || rng["quote"] != "0.99" {
		t.Fatalf("range = %#v", rng)
	}
}

func TestOrderClientSubmitQuotesValidatesAcknowledgedRanges(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response string
		wantErr  string
	}{
		{
			name:     "partial acknowledgement",
			response: `{"status":"success","quotesAdded":1}`,
			wantErr:  "quotesAdded 1, want 2",
		},
		{
			name:     "empty response",
			response: `null`,
			wantErr:  "empty response",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer srv.Close()

			client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
			err := client.submitQuotes(context.Background(), []types.Quote{submitQuotesTestQuote()})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("submitQuotes() error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func submitQuotesTestQuote() types.Quote {
	return types.Quote{
		FromAsset:    common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ToAsset:      common.HexToAddress("0x2222222222222222222222222222222222222222"),
		FromDecimals: 6,
		ToDecimals:   18,
		Expiry:       1_800_000_000,
		ExclusiveFor: common.HexToAddress("0x3333333333333333333333333333333333333333"),
		Ranges: []types.QuoteRange{
			{
				MinAmount: big.NewInt(1),
				MaxAmount: big.NewInt(1_000_000),
				Quote:     "0.99",
			},
			{
				MinAmount: big.NewInt(1_000_001),
				MaxAmount: big.NewInt(2_000_000),
				Quote:     "0.98",
			},
		},
	}
}

func TestOrderClientValidateExecutorRegistration(t *testing.T) {
	executor := common.HexToAddress("0x4444444444444444444444444444444444444444")
	for _, tc := range []struct {
		name    string
		address string
		wantErr bool
	}{
		{name: "registered", address: executor.Hex()},
		{name: "missing", address: "0x5555555555555555555555555555555555555555", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/solver-api/solver/identities" {
					t.Fatalf("%s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("x-api-key"); got != "test-key" {
					t.Fatalf("x-api-key = %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[{"id":1,"createdAt":"now","updatedAt":"now","address":"` +
					tc.address + `","solverId":1}]}`))
			}))
			defer srv.Close()

			client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
			err := client.validateExecutorRegistration(context.Background(), executor)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateExecutorRegistration() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestOrderClientReplaceSupportedContracts(t *testing.T) {
	var gotBody lifiorder.PutSupportedContractsDto
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/solver/supported-contracts" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"oracle":[],"inputSettler":[],"outputSettler":[]}}`))
	}))
	defer srv.Close()

	inputSettler := common.HexToAddress("0x1111111111111111111111111111111111111111")
	outputSettler := common.HexToAddress("0x2222222222222222222222222222222222222222")
	client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
	err := client.replaceSupportedContracts(
		context.Background(),
		supportedContractsDTO(
			lifiorder.ContractsByKindDto{},
			chainRef(11155111),
			inputSettler,
			outputSettler,
		),
	)
	if err != nil {
		t.Fatalf("replaceSupportedContracts: %v", err)
	}
	if len(gotBody.InputSettler) != 1 ||
		gotBody.InputSettler[0].Chain != "eip155:11155111" ||
		gotBody.InputSettler[0].Address != inputSettler.Hex() {
		t.Fatalf("input settlers = %+v", gotBody.InputSettler)
	}
	if len(gotBody.OutputSettler) != 1 ||
		gotBody.OutputSettler[0].Chain != "eip155:11155111" ||
		gotBody.OutputSettler[0].Address != outputSettler.Hex() {
		t.Fatalf("output settlers = %+v", gotBody.OutputSettler)
	}
	if gotBody.Oracle != nil {
		t.Fatalf("oracles = %+v, want omitted", gotBody.Oracle)
	}
}

func TestOrderClientEnsureSupportedContractsSkipsPutWhenPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/solver/supported-contracts" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"oracle":[],"inputSettler":[{"chain":"eip155:11155111","address":"0x1111111111111111111111111111111111111111"}],"outputSettler":[{"chain":"eip155:11155111","address":"0x2222222222222222222222222222222222222222"}]}}`))
	}))
	defer srv.Close()

	client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
	err := client.ensureSupportedContracts(
		context.Background(),
		11155111,
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
	)
	if err != nil {
		t.Fatalf("ensureSupportedContracts: %v", err)
	}
}

func TestOrderClientEnsureSupportedContractsPutsWhenMissing(t *testing.T) {
	var methods []string
	var gotBody lifiorder.PutSupportedContractsDto
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/solver/supported-contracts" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		methods = append(methods, r.Method)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"oracle":[{"chain":"eip155:1","address":"0x3333333333333333333333333333333333333333"}],"inputSettler":[{"chain":"eip155:1","address":"0x4444444444444444444444444444444444444444"}],"outputSettler":[{"chain":"eip155:1","address":"0x5555555555555555555555555555555555555555"}]}}`))
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"data":{"oracle":[],"inputSettler":[],"outputSettler":[]}}`))
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
	defer srv.Close()

	client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
	err := client.ensureSupportedContracts(
		context.Background(),
		11155111,
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
	)
	if err != nil {
		t.Fatalf("ensureSupportedContracts: %v", err)
	}
	if len(methods) != 2 || methods[0] != http.MethodGet || methods[1] != http.MethodPut {
		t.Fatalf("methods = %v", methods)
	}
	inputSettlers := gotBody.InputSettler
	if got := len(inputSettlers); got != 2 {
		t.Fatalf("inputSettler count = %d", got)
	}
	if got := inputSettlers[0].Address; got != "0x4444444444444444444444444444444444444444" {
		t.Fatalf("preserved inputSettler address = %v", got)
	}
	if got := inputSettlers[1].Address; got != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("configured inputSettler address = %v", got)
	}
	outputSettlers := gotBody.OutputSettler
	if got := len(outputSettlers); got != 2 {
		t.Fatalf("outputSettler count = %d", got)
	}
	if got := outputSettlers[0].Address; got != "0x5555555555555555555555555555555555555555" {
		t.Fatalf("preserved outputSettler address = %v", got)
	}
	if got := outputSettlers[1].Address; got != "0x2222222222222222222222222222222222222222" {
		t.Fatalf("configured outputSettler address = %v", got)
	}
	oracles := gotBody.Oracle
	if got := len(oracles); got != 1 {
		t.Fatalf("oracle count = %d", got)
	}
	if got := oracles[0].Address; got != "0x3333333333333333333333333333333333333333" {
		t.Fatalf("preserved oracle address = %v", got)
	}
}

func TestOrderClientListRecoverableOrdersPaginatesAndFilters(t *testing.T) {
	cfg := testLifiConfig()
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	type request struct {
		status string
		offset string
	}
	var requests []request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/orders" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("limit"); got != "50" {
			t.Fatalf("limit = %q", got)
		}
		if got := query.Get("exclusiveFor"); got != cfg.Executor.Hex() {
			t.Fatalf("exclusiveFor = %q", got)
		}
		if got := query.Get("originChainId"); got != "11155111" {
			t.Fatalf("originChainId = %q", got)
		}
		if got := query.Get("destinationChainId"); got != "11155111" {
			t.Fatalf("destinationChainId = %q", got)
		}
		status := query.Get("status")
		offset := query.Get("offset")
		requests = append(requests, request{status: status, offset: offset})

		row := testListedOrderJSON(t, cfg, tokenIn, tokenOut, status)
		count := 50
		pageOffset := 0
		if offset == "50" {
			count = 1
			pageOffset = 50
		}
		rows := make([]json.RawMessage, count)
		for i := range rows {
			rows[i] = row
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(testListedOrdersPageJSON(t, rows, 51, pageOffset))
	}))
	defer srv.Close()

	client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
	orders, err := client.listRecoverableOrders(t.Context(), cfg.Executor)
	if err != nil {
		t.Fatalf("listRecoverableOrders: %v", err)
	}
	if len(orders) != 102 {
		t.Fatalf("orders = %d, want 102", len(orders))
	}
	wantRequests := []request{
		{status: orderStatusSigned, offset: "0"},
		{status: orderStatusSigned, offset: "50"},
		{status: orderStatusDelivered, offset: "0"},
		{status: orderStatusDelivered, offset: "50"},
	}
	if !slices.Equal(requests, wantRequests) {
		t.Fatalf("requests = %+v, want %+v", requests, wantRequests)
	}

	solver := &Solver{cfg: cfg, chainID: 11155111, log: logr.Discard()}
	order := solver.parseOrderMessage(orderMessage{Event: orderSubmitEvent, Data: orders[0]})
	if order == nil {
		t.Fatal("listed order did not pass the WebSocket admission parser")
	}
	if len(order.Output.Context) != 0 {
		t.Fatalf("listed limit order context = %x, want non-exclusive context", order.Output.Context)
	}
}

func TestOrderClientListRecoverableOrdersPaginationLimit(t *testing.T) {
	cfg := testLifiConfig()
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	row := testListedOrderJSON(t, cfg, tokenIn, tokenOut, orderStatusSigned)

	for _, tc := range []struct {
		name       string
		total      int
		wantErr    string
		wantOrders int
	}{
		{name: "maximum reachable total", total: 1_050, wantOrders: 1_050},
		{name: "first unreachable row", total: 1_051, wantErr: "pagination requires offset 1050"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
				if err != nil {
					t.Errorf("offset: %v", err)
					http.Error(w, "invalid offset", http.StatusBadRequest)
					return
				}
				count := min(int(orderRecoveryPageLimit), tc.total-offset)
				rows := make([]json.RawMessage, count)
				for index := range rows {
					rows[index] = row
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]any{
					"data": rows,
					"meta": map[string]any{
						"total": tc.total, "limit": orderRecoveryPageLimit, "offset": offset,
					},
				}); err != nil {
					t.Errorf("encode orders page: %v", err)
				}
			}))
			defer srv.Close()

			client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
			orders, err := client.listRecoverableOrdersByStatus(t.Context(), cfg.Executor, orderStatusSigned)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) ||
					!strings.Contains(err.Error(), "reported total 1051") {
					t.Fatalf("listRecoverableOrdersByStatus() error = %v", err)
				}
				if len(orders) != 0 {
					t.Fatalf("orders = %d after pagination failure, want 0", len(orders))
				}
				return
			}
			if err != nil {
				t.Fatalf("listRecoverableOrdersByStatus: %v", err)
			}
			if len(orders) != tc.wantOrders {
				t.Fatalf("orders = %d, want %d", len(orders), tc.wantOrders)
			}
		})
	}
}
