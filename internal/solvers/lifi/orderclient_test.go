package lifi

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

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
		_, _ = w.Write([]byte(`{"status":"success","quotesAdded":1}`))
	}))
	defer srv.Close()

	client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
	err := client.submitQuotes(context.Background(), []types.Quote{{
		FromAsset:    common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ToAsset:      common.HexToAddress("0x2222222222222222222222222222222222222222"),
		FromDecimals: 6,
		ToDecimals:   18,
		Expiry:       1_800_000_000,
		ExclusiveFor: common.HexToAddress("0x3333333333333333333333333333333333333333"),
		Ranges: []types.QuoteRange{{
			MinAmount: big.NewInt(1),
			MaxAmount: big.NewInt(1_000_000),
			Quote:     "0.99",
		}},
	}})
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
	rng := ranges[0].(map[string]any)
	if rng["minAmount"] != "1" || rng["maxAmount"] != "1000000" || rng["quote"] != "0.99" {
		t.Fatalf("range = %#v", rng)
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
	var gotBody map[string]any
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

	client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
	err := client.replaceSupportedContracts(
		context.Background(),
		supportedContractsDTO(
			lifiorder.ContractsByKindDto{},
			chainRef(11155111),
			common.HexToAddress("0x1111111111111111111111111111111111111111"),
			common.HexToAddress("0x2222222222222222222222222222222222222222"),
		),
	)
	if err != nil {
		t.Fatalf("replaceSupportedContracts: %v", err)
	}
	if got := gotBody["inputSettler"].([]any)[0].(map[string]any)["chain"]; got != "eip155:11155111" {
		t.Fatalf("chain = %v", got)
	}
	if got := gotBody["oracle"].([]any)[0].(map[string]any)["address"]; got != "0x2222222222222222222222222222222222222222" {
		t.Fatalf("oracle address = %v", got)
	}
}

func TestOrderClientEnsureSupportedContractsSkipsPutWhenPresent(t *testing.T) {
	var putCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/solver/supported-contracts" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Method == http.MethodPut {
			putCalled = true
			t.Fatal("unexpected PUT")
		}
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"oracle":[{"chain":"eip155:11155111","address":"0x2222222222222222222222222222222222222222"}],"inputSettler":[{"chain":"eip155:11155111","address":"0x1111111111111111111111111111111111111111"}],"outputSettler":[{"chain":"eip155:11155111","address":"0x2222222222222222222222222222222222222222"}]}}`))
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
	if putCalled {
		t.Fatal("PUT was called")
	}
}

func TestOrderClientEnsureSupportedContractsPutsWhenMissing(t *testing.T) {
	var methods []string
	var gotBody map[string]any
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
	inputSettlers := gotBody["inputSettler"].([]any)
	if got := len(inputSettlers); got != 2 {
		t.Fatalf("inputSettler count = %d", got)
	}
	if got := inputSettlers[0].(map[string]any)["address"]; got != "0x4444444444444444444444444444444444444444" {
		t.Fatalf("preserved inputSettler address = %v", got)
	}
	if got := inputSettlers[1].(map[string]any)["address"]; got != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("configured inputSettler address = %v", got)
	}
	outputSettlers := gotBody["outputSettler"].([]any)
	if got := len(outputSettlers); got != 2 {
		t.Fatalf("outputSettler count = %d", got)
	}
	if got := outputSettlers[0].(map[string]any)["address"]; got != "0x5555555555555555555555555555555555555555" {
		t.Fatalf("preserved outputSettler address = %v", got)
	}
	if got := outputSettlers[1].(map[string]any)["address"]; got != "0x2222222222222222222222222222222222222222" {
		t.Fatalf("configured outputSettler address = %v", got)
	}
	oracles := gotBody["oracle"].([]any)
	if got := len(oracles); got != 2 {
		t.Fatalf("oracle count = %d", got)
	}
	if got := oracles[0].(map[string]any)["address"]; got != "0x3333333333333333333333333333333333333333" {
		t.Fatalf("preserved oracle address = %v", got)
	}
	if got := oracles[1].(map[string]any)["address"]; got != "0x2222222222222222222222222222222222222222" {
		t.Fatalf("configured oracle address = %v", got)
	}
}
