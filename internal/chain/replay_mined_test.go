package chain

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

// replayRPC serves the header of block 0x64 and records eth_call params; callError, when set, is
// returned as the eth_call error object for calls that carry blockOverrides.
type replayRPC struct {
	server    *httptest.Server
	callError string

	mu    sync.Mutex
	calls [][]json.RawMessage
}

func newReplayRPC(t *testing.T, callError string) *replayRPC {
	t.Helper()
	s := &replayRPC{callError: callError}
	header := `{"number":"0x64","timestamp":"0x3e8","parentHash":"0x` + strings.Repeat("0", 64) +
		`","sha3Uncles":"0x` + strings.Repeat("0", 64) + `","miner":"0x` + strings.Repeat("0", 40) +
		`","stateRoot":"0x` + strings.Repeat("0", 64) + `","transactionsRoot":"0x` + strings.Repeat("0", 64) +
		`","receiptsRoot":"0x` + strings.Repeat("0", 64) + `","logsBloom":"0x` + strings.Repeat("0", 512) +
		`","difficulty":"0x0","gasLimit":"0x1","gasUsed":"0x0","extraData":"0x","mixHash":"0x` + strings.Repeat("0", 64) +
		`","nonce":"0x0000000000000000"}`
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
		result, failure := `"0x1"`, ""
		switch req.Method {
		case "eth_getBlockByNumber":
			result = header
		case "eth_call":
			s.mu.Lock()
			s.calls = append(s.calls, req.Params)
			s.mu.Unlock()
			result = `"0x"`
			if len(req.Params) == 4 {
				failure = s.callError
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if failure != "" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"error":` + failure + `}`))
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":` + result + `}`))
	}))
	t.Cleanup(s.server.Close)
	return s
}

func TestReplayMinedCallRunsOnParentStateWithMinedBlockTime(t *testing.T) {
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	msg := ethereum.CallMsg{From: common.HexToAddress("0x1111111111111111111111111111111111111111"), To: &to, Data: []byte{0xab}}
	for name, tc := range map[string]struct {
		callError string
		wantErr   string
		wantCalls int
	}{
		"revert keeps its payload": {
			callError: `{"code":3,"message":"execution reverted","data":"0x6c3d3f32"}`,
			wantErr:   "execution reverted", wantCalls: 1,
		},
		"unsupported overrides fall back to a plain call": {
			callError: `{"code":-32602,"message":"too many arguments"}`, wantCalls: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newReplayRPC(t, tc.callError)
			c, err := Dial(t.Context(), []string{srv.server.URL}, "", testMulticall, defaultRPCAttemptTimeout)
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			defer c.Close()
			_, err = c.ReplayMinedCall(t.Context(), msg, big.NewInt(100))
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("ReplayMinedCall err = %v, want %q", err, tc.wantErr)
			}
			if tc.wantErr != "" {
				if data, reverted := RevertData(err); !reverted || len(data) != 4 {
					t.Fatalf("revert data = %x, %t", data, reverted)
				}
			}
			srv.mu.Lock()
			defer srv.mu.Unlock()
			if len(srv.calls) != tc.wantCalls {
				t.Fatalf("eth_call count = %d, want %d", len(srv.calls), tc.wantCalls)
			}
			first := srv.calls[0]
			if len(first) != 4 || string(first[1]) != `"0x63"` || !strings.Contains(string(first[3]), `"number":"0x64"`) ||
				!strings.Contains(string(first[3]), `"time":"0x3e8"`) {
				t.Fatalf("replay params = %s", first)
			}
			if tc.wantCalls == 2 && (len(srv.calls[1]) != 2 || string(srv.calls[1][1]) != `"0x63"`) {
				t.Fatalf("fallback params = %s", srv.calls[1])
			}
		})
	}
}
