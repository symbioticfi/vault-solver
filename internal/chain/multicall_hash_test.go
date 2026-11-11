package chain

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
)

func TestMulticallAtHashPinsEveryRead(t *testing.T) {
	hash := common.HexToHash("0x1234")
	call := Call{Target: common.HexToAddress("0x5678"), AllowFailure: true, Data: []byte{1, 2, 3}}
	wantData := multicallB.PackAggregate3([]multicall3.Multicall3Call3{{Target: call.Target, AllowFailure: true, CallData: call.Data}})
	parsed, err := multicall3.Multicall3MetaData.ParseABI()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := parsed.Methods["aggregate3"].Outputs.Pack([]multicall3.Multicall3Result{{Success: true, ReturnData: []byte{0xab}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&request); decodeErr != nil {
			t.Error(decodeErr)
			return
		}
		var block struct {
			Hash common.Hash `json:"blockHash"`
		}
		if len(request.Params) != 2 || json.Unmarshal(request.Params[1], &block) != nil || block.Hash != hash {
			t.Errorf("eth_call must use the requested block hash: %s", request.Params)
		}
		var input struct {
			Input hexutil.Bytes `json:"input"`
		}
		if json.Unmarshal(request.Params[0], &input) != nil || !bytes.Equal(input.Input, wantData) {
			t.Error("aggregate3 input changed")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": hexutil.Encode(encoded)})
	}))
	defer server.Close()
	ec, err := dialClient(t.Context(), []string{server.URL}, rpcRoleRead, defaultRPCAttemptTimeout, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ec.client.Close()
	c := &Client{Client: ec.client, chainID: big.NewInt(1)}
	for range 2 {
		results, callErr := c.MulticallAtHash(t.Context(), []Call{call}, hash)
		if callErr != nil || len(results) != 1 || !results[0].Success || !bytes.Equal(results[0].ReturnData, []byte{0xab}) {
			t.Fatalf("pinned results = %+v, error = %v", results, callErr)
		}
	}
}
