package bridgefacilitator

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

func newMulticallFakeClient(t *testing.T, replies ...[]byte) (*chain.Client, func()) {
	t.Helper()
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		id, _ := json.Marshal(request.ID)
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "eth_chainId":
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x1"}`, id)
		case "eth_call":
			i := min(int(calls.Add(1))-1, len(replies)-1)
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x%x"}`, id, replies[i])
		default:
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"method not found"}}`, id)
		}
	}))
	client, err := chain.Dial(t.Context(), []string{server.URL}, "", common.HexToAddress("0x1").Hex(), logr.Discard())
	if err != nil {
		server.Close()
		t.Fatalf("dial fake chain: %v", err)
	}
	return client, server.Close
}

func abiEncodeAggregate3Results(t *testing.T, payloads ...[]byte) []byte {
	t.Helper()
	tuple, err := abi.NewType("tuple[]", "", []abi.ArgumentMarshaling{{Name: "success", Type: "bool"}, {Name: "returnData", Type: "bytes"}})
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		Success    bool
		ReturnData []byte
	}
	values := make([]result, len(payloads))
	for i, payload := range payloads {
		values[i] = result{Success: true, ReturnData: payload}
	}
	encoded, err := abi.Arguments{{Type: tuple}}.Pack(values)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func abiEncodeUint256(t *testing.T, value int64) []byte {
	t.Helper()
	typeUint, err := abi.NewType("uint256", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := abi.Arguments{{Type: typeUint}}.Pack(big.NewInt(value))
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func contractAuctionDTO(id float32, request, collateral string) threef.AuctionDto {
	amount, maxRate := "1000", float32(250.5)
	name, version, chainID := "characterization-domain", "9.8.7", float32(11155111)
	return threef.AuctionDto{Id: id, RequestId: request, AmountRequested: *threef.NewNullableString(&amount), MaxRate: *threef.NewNullableFloat32(&maxRate), Status: "open", DepositAsset: *threef.NewNullableAuctionDepositAssetDto(threef.NewAuctionDepositAssetDto(collateral, "USDC", 6)), Eip712Domain: *threef.NewNullableAuctionEip712DomainDto(threef.NewAuctionEip712DomainDto(*threef.NewNullableString(&name), *threef.NewNullableString(&version), *threef.NewNullableFloat32(&chainID)))}
}

func testAuctionDto(id int64, depositAsset common.Address) threef.AuctionDto {
	amount, rate := "1000", float32(250)
	return threef.AuctionDto{Id: float32(id), RequestId: fmt.Sprintf("req-%d", id), AmountRequested: *threef.NewNullableString(&amount), MaxRate: *threef.NewNullableFloat32(&rate), Status: "OPEN", DepositAsset: *threef.NewNullableAuctionDepositAssetDto(threef.NewAuctionDepositAssetDto(depositAsset.Hex(), "TKN", 18))}
}
