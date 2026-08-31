package uniswapx

import (
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

type confirmationRPCScenario struct {
	receiptResult json.RawMessage
	receiptError  string
	blockResult   json.RawMessage
	blockError    string
	headResult    json.RawMessage
	headError     string
	record        *confirmationRPCRecord
}

type confirmationRPCRecord struct {
	mu      sync.Mutex
	methods []string
}

func (s *confirmationRPCScenario) handler(w http.ResponseWriter, request *http.Request) {
	var rpcRequest struct {
		ID     json.RawMessage   `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &rpcRequest); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.record.mu.Lock()
	s.record.methods = append(s.record.methods, rpcRequest.Method)
	s.record.mu.Unlock()

	var result json.RawMessage
	var rpcError string
	switch rpcRequest.Method {
	case "eth_chainId":
		result = json.RawMessage(`"0x1"`)
	case "eth_getTransactionReceipt":
		result, rpcError = s.receiptResult, s.receiptError
	case "eth_getBlockByNumber":
		if len(rpcRequest.Params) > 0 && string(rpcRequest.Params[0]) == `"latest"` {
			result, rpcError = s.headResult, s.headError
		} else {
			result, rpcError = s.blockResult, s.blockError
		}
	default:
		rpcError = "unexpected method " + rpcRequest.Method
	}
	w.Header().Set("Content-Type", "application/json")
	if rpcError != "" {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(rpcRequest.ID) +
			`,"error":{"code":-32000,"message":` + mustJSONQuote(rpcError) + `}}`))
		return
	}
	if len(result) == 0 {
		result = json.RawMessage("null")
	}
	_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(rpcRequest.ID) +
		`,"result":` + string(result) + `}`))
}

func (s *confirmationRPCScenario) recordedMethods() []string {
	s.record.mu.Lock()
	defer s.record.mu.Unlock()
	return append([]string(nil), s.record.methods...)
}

func mustJSONQuote(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func TestTransactionBlockTimeConfirmedViaChainClient(t *testing.T) {
	txHash := common.HexToHash("0x1234")
	blockNumber := big.NewInt(42)
	blockTime := uint64(1_700_000_000)
	blockHeader := confirmationHeader(t, blockNumber, blockTime)
	headHeader := confirmationHeader(t, big.NewInt(45), blockTime+10)
	canonicalReceipt := confirmationReceipt(t, txHash, blockNumber, blockHeader.Hash(), types.ReceiptStatusSuccessful)
	zeroHashReceipt := confirmationReceipt(t, txHash, blockNumber, common.Hash{}, types.ReceiptStatusSuccessful)
	failedReceipt := confirmationReceipt(t, txHash, blockNumber, blockHeader.Hash(), types.ReceiptStatusFailed)
	missingBlockReceipt := replaceJSONField(t, canonicalReceipt, "blockNumber", nil)
	missingNumberHead := replaceJSONField(t, mustMarshalJSON(t, headHeader), "number", nil)

	tests := []struct {
		name          string
		scenario      confirmationRPCScenario
		confirmations uint64
		wantTime      time.Time
		wantError     string
		wantMethods   []string
	}{
		{
			name:        "receipt RPC error",
			scenario:    confirmationRPCScenario{receiptError: "receipt unavailable"},
			wantError:   "read transaction receipt " + txHash.Hex() + ": receipt unavailable",
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt"},
		},
		{
			name:        "receipt not found",
			scenario:    confirmationRPCScenario{receiptResult: json.RawMessage("null")},
			wantError:   "read transaction receipt " + txHash.Hex() + ": not found",
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt"},
		},
		{
			name:        "failed receipt",
			scenario:    confirmationRPCScenario{receiptResult: failedReceipt},
			wantError:   "transaction " + txHash.Hex() + " has no successful canonical receipt",
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt"},
		},
		{
			name:        "receipt missing block number",
			scenario:    confirmationRPCScenario{receiptResult: missingBlockReceipt},
			wantError:   "transaction " + txHash.Hex() + " has no successful canonical receipt",
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt"},
		},
		{
			name:        "transaction block header RPC error",
			scenario:    confirmationRPCScenario{receiptResult: canonicalReceipt, blockError: "block unavailable"},
			wantError:   "read transaction block " + txHash.Hex() + ": block unavailable",
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt", "eth_getBlockByNumber"},
		},
		{
			name:        "transaction block header not found",
			scenario:    confirmationRPCScenario{receiptResult: canonicalReceipt, blockResult: json.RawMessage("null")},
			wantError:   "read transaction block " + txHash.Hex() + ": not found",
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt", "eth_getBlockByNumber"},
		},
		{
			name: "nonzero receipt hash mismatch is a reorg",
			scenario: confirmationRPCScenario{
				receiptResult: confirmationReceipt(t, txHash, blockNumber, common.HexToHash("0xbeef"), types.ReceiptStatusSuccessful),
				blockResult:   mustMarshalJSON(t, blockHeader),
			},
			wantError:   "transaction " + txHash.Hex() + " receipt is not canonical",
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt", "eth_getBlockByNumber"},
		},
		{
			name: "zero receipt block hash skips comparison",
			scenario: confirmationRPCScenario{
				receiptResult: zeroHashReceipt, blockResult: mustMarshalJSON(t, blockHeader),
				headResult: mustMarshalJSON(t, headHeader),
			},
			confirmations: 2, wantTime: time.Unix(int64(blockTime), 0),
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt", "eth_getBlockByNumber", "eth_getBlockByNumber"},
		},
		{
			name: "latest head RPC error",
			scenario: confirmationRPCScenario{
				receiptResult: canonicalReceipt, blockResult: mustMarshalJSON(t, blockHeader), headError: "head unavailable",
			},
			wantError:   "read latest block for transaction " + txHash.Hex() + ": head unavailable",
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt", "eth_getBlockByNumber", "eth_getBlockByNumber"},
		},
		{
			name: "latest head not found",
			scenario: confirmationRPCScenario{
				receiptResult: canonicalReceipt, blockResult: mustMarshalJSON(t, blockHeader), headResult: json.RawMessage("null"),
			},
			wantError:   "read latest block for transaction " + txHash.Hex() + ": not found",
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt", "eth_getBlockByNumber", "eth_getBlockByNumber"},
		},
		{
			name: "latest head missing number",
			scenario: confirmationRPCScenario{
				receiptResult: canonicalReceipt, blockResult: mustMarshalJSON(t, blockHeader), headResult: missingNumberHead,
			},
			wantError:   "read latest block for transaction " + txHash.Hex() + ": missing required field 'number' for Header",
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt", "eth_getBlockByNumber", "eth_getBlockByNumber"},
		},
		{
			name: "insufficient confirmation depth",
			scenario: confirmationRPCScenario{
				receiptResult: canonicalReceipt, blockResult: mustMarshalJSON(t, blockHeader),
				headResult: mustMarshalJSON(t, confirmationHeader(t, big.NewInt(43), blockTime+10)),
			},
			confirmations: 2,
			wantError:     "transaction " + txHash.Hex() + ": 1 confirmations pending at head 43",
			wantMethods:   []string{"eth_chainId", "eth_getTransactionReceipt", "eth_getBlockByNumber", "eth_getBlockByNumber"},
		},
		{
			name: "canonical confirmed success",
			scenario: confirmationRPCScenario{
				receiptResult: canonicalReceipt, blockResult: mustMarshalJSON(t, blockHeader),
				headResult: mustMarshalJSON(t, headHeader),
			},
			confirmations: 3, wantTime: time.Unix(int64(blockTime), 0),
			wantMethods: []string{"eth_chainId", "eth_getTransactionReceipt", "eth_getBlockByNumber", "eth_getBlockByNumber"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.scenario.record = new(confirmationRPCRecord)
			server := httptest.NewServer(http.HandlerFunc(tc.scenario.handler))
			t.Cleanup(server.Close)
			client, err := chain.Dial(
				t.Context(), []string{server.URL}, "", common.Address{}.Hex(), logr.Discard(),
			)
			if err != nil {
				t.Fatalf("chain.Dial() error = %v", err)
			}
			t.Cleanup(client.Close)

			got, err := (&reader{chain: client}).transactionBlockTimeConfirmed(
				t.Context(), txHash, tc.confirmations,
			)
			if tc.wantError == "" {
				if err != nil || !got.Equal(tc.wantTime) {
					t.Fatalf("transactionBlockTimeConfirmed() = (%v, %v), want (%v, nil)", got, err, tc.wantTime)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("transactionBlockTimeConfirmed() error = %v, want containing %q", err, tc.wantError)
			}
			if methods := tc.scenario.recordedMethods(); !reflect.DeepEqual(methods, tc.wantMethods) {
				t.Fatalf("RPC methods = %v, want %v", methods, tc.wantMethods)
			}
		})
	}
}

func confirmationHeader(t *testing.T, number *big.Int, timestamp uint64) *types.Header {
	t.Helper()
	return &types.Header{
		ParentHash: common.HexToHash("0x01"), UncleHash: types.EmptyUncleHash,
		Root: common.HexToHash("0x02"), TxHash: types.EmptyTxsHash,
		ReceiptHash: types.EmptyReceiptsHash, Difficulty: new(big.Int), Number: number,
		GasLimit: 30_000_000, Time: timestamp, Extra: []byte{},
	}
}

func confirmationReceipt(
	t *testing.T,
	txHash common.Hash,
	blockNumber *big.Int,
	blockHash common.Hash,
	status uint64,
) json.RawMessage {
	t.Helper()
	return mustMarshalJSON(t, &types.Receipt{
		Type: types.DynamicFeeTxType, Status: status, CumulativeGasUsed: 21_000,
		Logs: []*types.Log{}, TxHash: txHash, GasUsed: 21_000, EffectiveGasPrice: big.NewInt(1),
		BlockHash: blockHash, BlockNumber: blockNumber,
	})
}

func mustMarshalJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return encoded
}

func replaceJSONField(t *testing.T, encoded json.RawMessage, field string, value any) json.RawMessage {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	object[field] = value
	return mustMarshalJSON(t, object)
}
