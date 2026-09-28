package chain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/codes"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

// rpcCall is one JSON-RPC request a test server received.
type rpcCall struct {
	method string
	params []json.RawMessage
}

// rpcReply is how a test server answers: a result, a JSON-RPC error, or a raw non-2xx HTTP response.
type rpcReply struct {
	result     any
	err        *jsonRPCErrorObject
	httpStatus int
	httpBody   string
}

// paramsServer answers eth_chainId itself and every other request with handle, recording each call.
type paramsServer struct {
	*httptest.Server

	mu    sync.Mutex
	calls []rpcCall
}

func newParamsServer(t *testing.T, handle func(rpcCall) rpcReply) *paramsServer {
	t.Helper()
	s := &paramsServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		call := rpcCall{method: req.Method, params: req.Params}
		reply := rpcReply{result: "0x7a69"}
		if req.Method != rpcMethodChainID {
			s.mu.Lock()
			s.calls = append(s.calls, call)
			s.mu.Unlock()
			reply = handle(call)
		}
		if reply.httpStatus != 0 {
			w.WriteHeader(reply.httpStatus)
			_, _ = w.Write([]byte(reply.httpBody))
			return
		}
		envelope := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if reply.err != nil {
			envelope["error"] = reply.err
		} else {
			envelope["result"] = reply.result
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(envelope)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *paramsServer) recorded() []rpcCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

func (s *paramsServer) methods() []string {
	var methods []string
	for _, call := range s.recorded() {
		methods = append(methods, call.method)
	}
	return methods
}

// dialReadWrite dials read as the only read endpoint and a separate write endpoint that must never
// see the pinned reads, with RPC metrics on a private registry.
func dialReadWrite(t *testing.T, read, write *paramsServer) (*Client, *RPCMetrics) {
	t.Helper()
	metrics, err := NewRPCMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	client, err := DialWithMetrics(t.Context(), []string{read.URL}, write.URL, "", testMulticall, 0, metrics)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client, metrics
}

func noWrites(t *testing.T) *paramsServer {
	t.Helper()
	return newParamsServer(t, func(call rpcCall) rpcReply {
		t.Errorf("write endpoint served %s", call.method)
		return rpcReply{result: nil}
	})
}

// requireJSON compares a raw parameter with the expected JSON document, ignoring formatting.
func requireJSON(t *testing.T, name string, got json.RawMessage, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("%s: decode %s: %v", name, got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("%s: decode want %s: %v", name, want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("%s = %s, want %s", name, got, want)
	}
}

func TestReadBalanceAtBlock(t *testing.T) {
	account := common.HexToAddress("0x5641000000000000000000000000000000005480")
	hash := common.HexToHash("0xabc0000000000000000000000000000000000000000000000000000000000def")
	for _, tc := range []struct {
		name         string
		block        rpc.BlockNumberOrHash
		reply        rpcReply
		wantParam    string
		wantBalance  int64
		wantNotFound bool
		wantErr      bool
		wantOutcome  rpcOutcome
	}{
		{
			name: "pinned by number", block: rpc.BlockNumberOrHashWithNumber(26_042_429),
			reply: rpcReply{result: "0x2a"}, wantParam: `"0x18d603d"`, wantBalance: 42, wantOutcome: rpcOutcomeSuccess,
		},
		{
			name: "pinned by hash", block: rpc.BlockNumberOrHashWithHash(hash, false),
			reply: rpcReply{result: "0x2a"}, wantParam: `{"blockHash":"` + hash.Hex() + `"}`, wantBalance: 42,
			wantOutcome: rpcOutcomeSuccess,
		},
		{
			name: "pinned by canonical hash", block: rpc.BlockNumberOrHashWithHash(hash, true),
			reply:     rpcReply{result: "0x0"},
			wantParam: `{"blockHash":"` + hash.Hex() + `","requireCanonical":true}`, wantOutcome: rpcOutcomeSuccess,
		},
		{
			name: "number not found at the pin", block: rpc.BlockNumberOrHashWithNumber(26_042_430),
			reply:     rpcReply{err: &jsonRPCErrorObject{Code: -32000, Message: "header not found"}},
			wantParam: `"0x18d603e"`, wantNotFound: true, wantErr: true, wantOutcome: rpcOutcomeRPCError,
		},
		{
			name: "hash not found at the pin", block: rpc.BlockNumberOrHashWithHash(hash, false),
			reply:     rpcReply{err: &jsonRPCErrorObject{Code: -32000, Message: "header for hash not found"}},
			wantParam: `{"blockHash":"` + hash.Hex() + `"}`, wantNotFound: true, wantErr: true, wantOutcome: rpcOutcomeRPCError,
		},
		{
			name: "resource not found", block: rpc.BlockNumberOrHashWithNumber(7),
			reply:     rpcReply{err: &jsonRPCErrorObject{Code: -32001, Message: "resource not found"}},
			wantParam: `"0x7"`, wantNotFound: true, wantErr: true, wantOutcome: rpcOutcomeRPCError,
		},
		{
			name: "null balance", block: rpc.BlockNumberOrHashWithNumber(7),
			reply: rpcReply{result: nil}, wantParam: `"0x7"`, wantNotFound: true, wantErr: true, wantOutcome: rpcOutcomeSuccess,
		},
		{
			name: "pruned state is not a missing block", block: rpc.BlockNumberOrHashWithNumber(7),
			reply:     rpcReply{err: &jsonRPCErrorObject{Code: -32000, Message: "missing trie node"}},
			wantParam: `"0x7"`, wantErr: true, wantOutcome: rpcOutcomeRPCError,
		},
		{
			name: "unsupported method is not a missing block", block: rpc.BlockNumberOrHashWithNumber(7),
			reply:     rpcReply{err: &jsonRPCErrorObject{Code: -32601, Message: "method not found"}},
			wantParam: `"0x7"`, wantErr: true, wantOutcome: rpcOutcomeRPCError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := newParamsServer(t, func(rpcCall) rpcReply { return tc.reply })
			client, metrics := dialReadWrite(t, read, noWrites(t))

			balance, err := client.ReadBalanceAtBlock(t.Context(), account, tc.block)
			if tc.wantErr != (err != nil) {
				t.Fatalf("ReadBalanceAtBlock error = %v, wantErr %t", err, tc.wantErr)
			}
			if got := errors.Is(err, ethereum.NotFound); got != tc.wantNotFound {
				t.Fatalf("errors.Is(%v, ethereum.NotFound) = %t, want %t", err, got, tc.wantNotFound)
			}
			if !tc.wantErr && balance.Cmp(big.NewInt(tc.wantBalance)) != 0 {
				t.Fatalf("balance = %s, want %d", balance, tc.wantBalance)
			}
			calls := read.recorded()
			if len(calls) != 1 || calls[0].method != rpcMethodGetBalance || len(calls[0].params) != 2 {
				t.Fatalf("read endpoint calls = %+v, want one two-parameter eth_getBalance", calls)
			}
			requireJSON(t, "account", calls[0].params[0], `"`+account.Hex()+`"`)
			requireJSON(t, "block", calls[0].params[1], tc.wantParam)
			metricstest.RequireValue(t, metrics.requests.WithLabelValues(rpcRoleRead, rpcMethodGetBalance, string(tc.wantOutcome)), 1)
		})
	}
}

func TestReadBalanceAtBlockRefusesUnpinnedReferences(t *testing.T) {
	hash := common.HexToHash("0x01")
	number := rpc.BlockNumber(7)
	for _, tc := range []struct {
		name  string
		block rpc.BlockNumberOrHash
	}{
		{name: "latest", block: rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)},
		{name: "pending", block: rpc.BlockNumberOrHashWithNumber(rpc.PendingBlockNumber)},
		{name: "safe", block: rpc.BlockNumberOrHashWithNumber(rpc.SafeBlockNumber)},
		{name: "empty", block: rpc.BlockNumberOrHash{}},
		{name: "hash and number", block: rpc.BlockNumberOrHash{BlockHash: &hash, BlockNumber: &number}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := newParamsServer(t, func(rpcCall) rpcReply { return rpcReply{result: "0x1"} })
			client, _ := dialReadWrite(t, read, noWrites(t))
			if _, err := client.ReadBalanceAtBlock(t.Context(), common.Address{}, tc.block); err == nil {
				t.Fatal("expected an unpinned block reference to be refused")
			}
			if methods := read.methods(); len(methods) != 0 {
				t.Fatalf("refused read still reached the endpoint: %v", methods)
			}
		})
	}
}

func TestEstimateGasWithBlockOverridesSendsFourParameters(t *testing.T) {
	read := newParamsServer(t, func(rpcCall) rpcReply { return rpcReply{result: "0x37bb4a"} })
	client, metrics := dialReadWrite(t, read, noWrites(t))
	from := common.HexToAddress("0x5641000000000000000000000000000000005480")
	to := common.HexToAddress("0x1111111111111111111111111111111111111111")
	head := &types.Header{Number: big.NewInt(26_042_429), Time: 1_759_000_000}

	gas, err := client.EstimateGasWithBlockOverrides(t.Context(), ethereum.CallMsg{
		From: from, To: &to, Value: big.NewInt(5), Data: []byte{0xde, 0xad},
	}, head.Number, NextBlockOverrides(head, 12*time.Second))
	if err != nil {
		t.Fatalf("EstimateGasWithBlockOverrides: %v", err)
	}
	if gas != 3_652_426 {
		t.Fatalf("gas = %d, want 3652426", gas)
	}
	calls := read.recorded()
	if len(calls) != 1 || calls[0].method != rpcMethodEstimateGas || len(calls[0].params) != 4 {
		t.Fatalf("read endpoint calls = %+v, want one four-parameter eth_estimateGas", calls)
	}
	params := calls[0].params
	requireJSON(t, "call", params[0], `{"from":"`+from.Hex()+`","to":"`+to.Hex()+`","value":"0x5","input":"0xdead"}`)
	requireJSON(t, "parent block", params[1], `"0x18d603d"`)
	requireJSON(t, "state overrides", params[2], `null`)
	requireJSON(t, "block overrides", params[3], `{"number":"0x18d603e","time":"0x68d835cc"}`)
	metricstest.RequireValue(t, metrics.requests.WithLabelValues(rpcRoleRead, rpcMethodEstimateGas, string(rpcOutcomeSuccess)), 1)
}

func TestEstimateGasWithBlockOverridesNeedsAnExplicitParent(t *testing.T) {
	read := newParamsServer(t, func(rpcCall) rpcReply { return rpcReply{result: "0x5208"} })
	client, _ := dialReadWrite(t, read, noWrites(t))
	for _, parent := range []*big.Int{nil, big.NewInt(-2)} {
		if _, err := client.EstimateGasWithBlockOverrides(t.Context(), ethereum.CallMsg{}, parent, ethereum.BlockOverrides{}); err == nil {
			t.Fatalf("parent %v: expected an error", parent)
		}
	}
	if methods := read.methods(); len(methods) != 0 {
		t.Fatalf("refused estimates still reached the endpoint: %v", methods)
	}
}

// TestBlockOverridesErrorClassification runs real estimates against the answers upstreams give and
// checks which ones select the plain-estimate fallback and which ones are reverts.
func TestBlockOverridesErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name            string
		reply           rpcReply
		wantUnsupported bool
		wantReverted    bool
	}{
		{name: "success", reply: rpcReply{result: "0x5208"}},
		{name: "invalid params code", reply: rpcReply{err: &jsonRPCErrorObject{Code: -32602, Message: "too many arguments, want at most 3"}}, wantUnsupported: true},
		{name: "too many arguments under a generic code", reply: rpcReply{err: &jsonRPCErrorObject{Code: -32000, Message: "Too many arguments, want at most 2"}}, wantUnsupported: true},
		{name: "invalid argument", reply: rpcReply{err: &jsonRPCErrorObject{Code: -32000, Message: "invalid argument 3: json: unknown field \"time\""}}, wantUnsupported: true},
		{name: "invalid params message", reply: rpcReply{err: &jsonRPCErrorObject{Code: -32603, Message: "Invalid params"}}, wantUnsupported: true},
		{name: "http 400 with a JSON-RPC error", reply: rpcReply{httpStatus: http.StatusBadRequest, httpBody: `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"invalid params"}}`}, wantUnsupported: true},
		{name: "http 400 with a text body", reply: rpcReply{httpStatus: http.StatusBadRequest, httpBody: "too many arguments"}, wantUnsupported: true},
		{name: "revert with data", reply: rpcReply{err: &jsonRPCErrorObject{Code: 3, Message: "execution reverted: invalid argument"}}, wantReverted: true},
		{name: "revert without data", reply: rpcReply{err: &jsonRPCErrorObject{Code: -32000, Message: "execution reverted"}}, wantReverted: true},
		{name: "method not found", reply: rpcReply{err: &jsonRPCErrorObject{Code: -32601, Message: "the method eth_estimateGas does not exist/is not available: invalid params"}}},
		{name: "parent not found", reply: rpcReply{err: &jsonRPCErrorObject{Code: -32000, Message: "header not found"}}},
		{name: "upstream unavailable", reply: rpcReply{httpStatus: http.StatusServiceUnavailable}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := newParamsServer(t, func(rpcCall) rpcReply { return tc.reply })
			client, _ := dialReadWrite(t, read, noWrites(t))
			_, err := client.EstimateGasWithBlockOverrides(t.Context(), ethereum.CallMsg{}, big.NewInt(1), ethereum.BlockOverrides{Number: big.NewInt(2), Time: 12})
			if tc.reply.result != nil && err != nil {
				t.Fatalf("EstimateGasWithBlockOverrides: %v", err)
			}
			if got := IsBlockOverridesUnsupported(err); got != tc.wantUnsupported {
				t.Fatalf("IsBlockOverridesUnsupported(%v) = %t, want %t", err, got, tc.wantUnsupported)
			}
			if got := IsExecutionReverted(err); got != tc.wantReverted {
				t.Fatalf("IsExecutionReverted(%v) = %t, want %t", err, got, tc.wantReverted)
			}
		})
	}
}

// executeProbe stands in for an upstream executing the probe under the given block context: the
// call succeeds only when the installed code was assembled for exactly that number and timestamp,
// which is what the code checks (TestBlockOverridesProbeCode pins its bytes; the integration test
// runs it on a real EVM). It answers like geth: the gas used, or a code-3 revert.
func executeProbe(code []byte, number, timestamp uint64) rpcReply {
	if bytes.Equal(code, blockOverridesProbeCode(number, timestamp)) {
		return rpcReply{result: "0x5231"}
	}
	return rpcReply{err: &jsonRPCErrorObject{Code: 3, Message: "execution reverted"}}
}

type probeOverrides struct {
	Number *hexutil.Big   `json:"number"`
	Time   hexutil.Uint64 `json:"time"`
}

type probeAccount struct {
	Code hexutil.Bytes `json:"code"`
}

func TestProbeBlockOverrides(t *testing.T) {
	const headNumber, headTime = 26_078_440, 1_759_100_000
	head := &types.Header{
		Number: big.NewInt(headNumber), Time: headTime, Difficulty: new(big.Int),
		GasLimit: 45_000_000, BaseFee: big.NewInt(1),
	}
	for _, tc := range []struct {
		name          string
		headErr       bool
		upstream      func(t *testing.T, code []byte, overrides probeOverrides) rpcReply
		wantSupported bool
		wantErr       bool
	}{
		{
			name: "overrides honoured",
			upstream: func(_ *testing.T, code []byte, o probeOverrides) rpcReply {
				return executeProbe(code, o.Number.ToInt().Uint64(), uint64(o.Time))
			},
			wantSupported: true,
		},
		{
			name: "overrides silently ignored",
			upstream: func(_ *testing.T, code []byte, _ probeOverrides) rpcReply {
				return executeProbe(code, headNumber, headTime)
			},
		},
		{
			name: "only the number override honoured",
			upstream: func(_ *testing.T, code []byte, o probeOverrides) rpcReply {
				return executeProbe(code, o.Number.ToInt().Uint64(), headTime)
			},
		},
		{
			name: "state override silently ignored",
			upstream: func(*testing.T, []byte, probeOverrides) rpcReply {
				return rpcReply{result: "0x5208"} // a plain call to the empty probe account
			},
		},
		{
			name: "overrides rejected as invalid params",
			upstream: func(*testing.T, []byte, probeOverrides) rpcReply {
				return rpcReply{err: &jsonRPCErrorObject{Code: -32602, Message: "too many arguments, want at most 3"}}
			},
		},
		{
			name: "overrides rejected with http 400",
			upstream: func(*testing.T, []byte, probeOverrides) rpcReply {
				return rpcReply{httpStatus: http.StatusBadRequest, httpBody: `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"invalid params"}}`}
			},
		},
		{
			name: "upstream behind the probed head",
			upstream: func(*testing.T, []byte, probeOverrides) rpcReply {
				return rpcReply{err: &jsonRPCErrorObject{Code: -32000, Message: "header not found"}}
			},
			wantErr: true,
		},
		{
			name: "anvil behind the probed head",
			upstream: func(*testing.T, []byte, probeOverrides) rpcReply {
				return rpcReply{err: &jsonRPCErrorObject{Code: -32602, Message: "BlockOutOfRangeError: block height is 1 but requested was 6"}}
			},
			wantErr: true,
		},
		{
			name: "upstream unavailable",
			upstream: func(*testing.T, []byte, probeOverrides) rpcReply {
				return rpcReply{httpStatus: http.StatusBadGateway}
			},
			wantErr: true,
		},
		{name: "head unavailable", headErr: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := newParamsServer(t, func(call rpcCall) rpcReply {
				switch call.method {
				case "eth_getBlockByNumber":
					if tc.headErr {
						return rpcReply{err: &jsonRPCErrorObject{Code: -32000, Message: "upstream timeout"}}
					}
					return rpcReply{result: head}
				case rpcMethodEstimateGas:
					if len(call.params) != 4 {
						t.Errorf("probe sent %d parameters, want 4", len(call.params))
						return rpcReply{err: &jsonRPCErrorObject{Code: -32602, Message: "want 4 parameters"}}
					}
					requireJSON(t, "probe parent", call.params[1], `"`+hexutil.EncodeUint64(headNumber)+`"`)
					var state map[common.Address]probeAccount
					var overrides probeOverrides
					if err := json.Unmarshal(call.params[2], &state); err != nil {
						t.Fatalf("decode state overrides: %v", err)
					}
					if err := json.Unmarshal(call.params[3], &overrides); err != nil {
						t.Fatalf("decode block overrides: %v", err)
					}
					if overrides.Number.ToInt().Uint64() != headNumber+1 || uint64(overrides.Time) != headTime+12 {
						t.Fatalf("probe overrides = %d/%d, want the next block %d/%d",
							overrides.Number.ToInt(), overrides.Time, headNumber+1, headTime+12)
					}
					return tc.upstream(t, state[blockOverridesProbeTarget].Code, overrides)
				default:
					t.Errorf("unexpected probe call %s", call.method)
					return rpcReply{result: nil}
				}
			})
			client, _ := dialReadWrite(t, read, noWrites(t))

			supported, err := client.ProbeBlockOverrides(t.Context(), 12*time.Second)
			if tc.wantErr != (err != nil) {
				t.Fatalf("ProbeBlockOverrides error = %v, wantErr %t", err, tc.wantErr)
			}
			if supported != tc.wantSupported {
				t.Fatalf("ProbeBlockOverrides supported = %t, want %t", supported, tc.wantSupported)
			}
		})
	}
}

// TestBlockOverridesProbeCode pins the assembled bytes: the layout below is the one probed against
// drpc, publicnode, 1rpc and MEV Blocker (scratchpad judge_abc/probe_bo.py, widened to PUSH8 and a
// TIMESTAMP check), and the integration test executes it on anvil.
func TestBlockOverridesProbeCode(t *testing.T) {
	for _, tc := range []struct {
		number, timestamp uint64
		want              string
	}{
		{
			number: 26_078_441, timestamp: 1_759_100_012,
			want: "43" + "67" + "00000000018dece9" + "14" + "42" + "67" + "0000000068d9bc6c" + "14" + "16" +
				"60" + "1f" + "57" + "6000" + "6000" + "fd" + "5b" + "00",
		},
		{
			number: math.MaxUint64, timestamp: 0,
			want: "43" + "67" + "ffffffffffffffff" + "14" + "42" + "67" + "0000000000000000" + "14" + "16" +
				"60" + "1f" + "57" + "6000" + "6000" + "fd" + "5b" + "00",
		},
	} {
		if got := hex.EncodeToString(blockOverridesProbeCode(tc.number, tc.timestamp)); got != tc.want {
			t.Fatalf("probe code for %d/%d = %s, want %s", tc.number, tc.timestamp, got, tc.want)
		}
	}
}

func TestIsBlockNotFound(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil},
		{name: "transport", err: errors.New("dial tcp: connection refused")},
		{name: "geth number", err: &testRPCError{code: -32000, message: "header not found"}, want: true},
		{name: "geth hash", err: &testRPCError{code: -32000, message: "header for hash not found"}, want: true},
		{name: "resource not found", err: &testRPCError{code: -32001, message: "Resource not found"}, want: true},
		{name: "anvil out of range", err: &testRPCError{code: -32602, message: "BlockOutOfRangeError: block height is 1 but requested was 6"}, want: true},
		{name: "nethermind", err: &testRPCError{code: -32000, message: "Block 0x12 could not be found"}, want: true},
		{name: "wrapped", err: errors.Errorf("read: %w", &testRPCError{code: -32000, message: "unknown block"}), want: true},
		{name: "method not found", err: &testRPCError{code: -32601, message: "the method could not be found"}},
		{name: "pruned state", err: &testRPCError{code: -32000, message: "missing trie node abc (path ) state 0x is not available"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBlockNotFound(tc.err); got != tc.want {
				t.Fatalf("isBlockNotFound(%v) = %t, want %t", tc.err, got, tc.want)
			}
		})
	}
}

type testRPCError struct {
	code    int
	message string
}

func (e *testRPCError) Error() string  { return e.message }
func (e *testRPCError) ErrorCode() int { return e.code }

func TestNextBlockOverrides(t *testing.T) {
	head := &types.Header{Number: big.NewInt(100), Time: 1000}
	for _, tc := range []struct {
		blockTime time.Duration
		wantTime  uint64
	}{
		{blockTime: 12 * time.Second, wantTime: 1012},
		{blockTime: 12500 * time.Millisecond, wantTime: 1013},
		{blockTime: 500 * time.Millisecond, wantTime: 1001},
		{blockTime: time.Second, wantTime: 1001},
		{blockTime: 0, wantTime: 1001},
		{blockTime: -5 * time.Second, wantTime: 1001},
	} {
		got := NextBlockOverrides(head, tc.blockTime)
		if got.Number.Int64() != 101 || got.Time != tc.wantTime {
			t.Fatalf("NextBlockOverrides(%s) = %d/%d, want 101/%d", tc.blockTime, got.Number, got.Time, tc.wantTime)
		}
	}
	if head.Number.Int64() != 100 {
		t.Fatalf("NextBlockOverrides mutated the head number to %s", head.Number)
	}
}

// TestPinnedReadsOverWebsocket_SpanPerCall covers the transports fallbackTransport does not span:
// each new raw call must still produce one client span named for its JSON-RPC method.
func TestPinnedReadsOverWebsocket_SpanPerCall(t *testing.T) {
	rec := tracetest.Install(t)
	srv := newWSRPC(chainRPCResult)
	defer srv.close()
	c, err := Dial(t.Context(), []string{srv.url()}, "", "", testMulticall, defaultRPCAttemptTimeout)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if _, err = c.ReadBalanceAtBlock(t.Context(), common.Address{}, rpc.BlockNumberOrHashWithNumber(1)); err != nil {
		t.Fatalf("ReadBalanceAtBlock: %v", err)
	}
	if _, err = c.EstimateGasWithBlockOverrides(t.Context(), ethereum.CallMsg{}, big.NewInt(1), ethereum.BlockOverrides{Number: big.NewInt(2)}); err != nil {
		t.Fatalf("EstimateGasWithBlockOverrides: %v", err)
	}
	for _, method := range []string{rpcMethodGetBalance, rpcMethodEstimateGas} {
		spans := tracetest.AllEnded(rec, method)
		if len(spans) != 1 {
			t.Fatalf("%s spans = %d, want 1", method, len(spans))
		}
		if tracetest.Attr(spans[0], "chain.rpc.role") != rpcRoleShared || tracetest.Attr(spans[0], "chain.rpc.transport") != rpcTransportWS {
			t.Fatalf("%s attributes = %v", method, spans[0].Attributes())
		}
		if spans[0].Status().Code == codes.Error {
			t.Fatalf("%s status = %v, want unset", method, spans[0].Status())
		}
	}
}
