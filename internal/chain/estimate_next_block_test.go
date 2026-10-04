package chain

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
)

// estimateRPC answers eth_chainId for Dial and eth_estimateGas with either a gas result or an error
// object, recording the estimate's params.
type estimateRPC struct {
	server *httptest.Server
	reply  string

	mu     sync.Mutex
	params []json.RawMessage
}

func newEstimateRPC(t *testing.T, reply string) *estimateRPC {
	t.Helper()
	s := &estimateRPC{reply: reply}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body := `"0x1"`
		if req.Method == "eth_estimateGas" {
			s.mu.Lock()
			s.params = req.Params
			s.mu.Unlock()
			body = s.reply
		}
		w.Header().Set("Content-Type", "application/json")
		if len(body) > 0 && body[0] == '{' {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"error":` + body + `}`))
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":` + body + `}`))
	}))
	t.Cleanup(s.server.Close)
	return s
}

func TestEstimateGasNextBlockSendsBlockOverrides(t *testing.T) {
	srv := newEstimateRPC(t, `"0x5208"`)
	c, err := Dial(t.Context(), []string{srv.server.URL}, "", testMulticall, defaultRPCAttemptTimeout)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	from := common.HexToAddress("0x1111111111111111111111111111111111111111")
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	gas, err := c.EstimateGasNextBlock(t.Context(), ethereum.CallMsg{
		From: from, To: &to, Data: []byte{0xab, 0xcd}, Value: big.NewInt(5),
	}, &types.Header{Number: big.NewInt(100), Time: 1_000}, 12*time.Second)
	if err != nil || gas != 21_000 {
		t.Fatalf("EstimateGasNextBlock = %d, %v; want 21000", gas, err)
	}

	srv.mu.Lock()
	params := srv.params
	srv.mu.Unlock()
	if len(params) != 4 {
		t.Fatalf("eth_estimateGas params = %d, want call, block, state override, block overrides", len(params))
	}
	var call map[string]string
	if err := json.Unmarshal(params[0], &call); err != nil {
		t.Fatalf("decode call: %v", err)
	}
	if call["from"] != from.Hex() || call["to"] != to.Hex() || call["input"] != "0xabcd" || call["value"] != "0x5" {
		t.Fatalf("call = %v", call)
	}
	if string(params[1]) != `"latest"` || string(params[2]) != "null" {
		t.Fatalf("block = %s, state override = %s; want latest and null", params[1], params[2])
	}
	var overrides map[string]string
	if err := json.Unmarshal(params[3], &overrides); err != nil {
		t.Fatalf("decode block overrides: %v", err)
	}
	if overrides["number"] != "0x65" || overrides["time"] != "0x3f4" {
		t.Fatalf("block overrides = %v, want number 101 (0x65) and time 1012 (0x3f4)", overrides)
	}
}

func TestEstimateGasNextBlockSurfacesInvalidParams(t *testing.T) {
	srv := newEstimateRPC(t, `{"code":-32602,"message":"too many arguments, want at most 3"}`)
	c, err := Dial(t.Context(), []string{srv.server.URL}, "", testMulticall, defaultRPCAttemptTimeout)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	_, err = c.EstimateGasNextBlock(t.Context(), ethereum.CallMsg{To: &to},
		&types.Header{Number: big.NewInt(1)}, 12*time.Second)
	var rpcErr rpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.ErrorCode() != -32602 {
		t.Fatalf("error = %v, want the endpoint's -32602 rpc error", err)
	}
	if _, err := c.EstimateGasNextBlock(t.Context(), ethereum.CallMsg{To: &to}, nil, 12*time.Second); err == nil {
		t.Fatal("a nil parent header must be rejected")
	}
}
