package chain

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestReadNonceAtHashUsesCanonicalReadState(t *testing.T) {
	type request struct {
		ID     json.RawMessage   `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	queries := make(chan request, 1)
	read := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		result := json.RawMessage(cancellationRPCResult(req.Method))
		if req.Method == rpcMethodGetTransactionCount {
			queries <- req
			result = json.RawMessage(`"0x7"`)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}); err != nil {
			t.Error(err)
		}
	}))
	defer read.Close()
	var writeMethods []string
	write := rpcRecorder(&writeMethods, cancellationRPCResult)
	defer write.Close()
	client, err := Dial(t.Context(), []string{read.URL}, write.URL, "", testMulticall, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	account := common.HexToAddress("0x123")
	hash := common.HexToHash("0xbeef")
	nonce, err := client.ReadNonceAtHash(t.Context(), account, hash)
	if err != nil || nonce != 7 {
		t.Fatalf("nonce = %d, error = %v", nonce, err)
	}
	if slices.Contains(writeMethods, rpcMethodGetTransactionCount) {
		t.Fatal("canonical account state read through submission endpoint")
	}
	query := <-queries
	if len(query.Params) != 2 {
		t.Fatalf("unexpected nonce query params: %s", query.Params)
	}
	var gotAccount common.Address
	var gotBlock struct {
		Hash             common.Hash `json:"blockHash"`
		RequireCanonical bool        `json:"requireCanonical"`
	}
	if err := json.Unmarshal(query.Params[0], &gotAccount); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(query.Params[1], &gotBlock); err != nil {
		t.Fatal(err)
	}
	if gotAccount != account || gotBlock.Hash != hash || !gotBlock.RequireCanonical {
		t.Fatalf("account/block proof changed: %s %+v", gotAccount, gotBlock)
	}
}
