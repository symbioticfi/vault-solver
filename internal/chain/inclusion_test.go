package chain

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-errors/errors"
)

type inclusionHeaders struct {
	header *types.Header
	err    error
	block  *big.Int
}

type inclusionReceipts struct {
	headers map[uint64]*types.Header
	receipt *types.Receipt
	err     error
	queried common.Hash
}

func (r *inclusionReceipts) HeaderByNumber(_ context.Context, block *big.Int) (*types.Header, error) {
	header := r.headers[block.Uint64()]
	if header == nil {
		return nil, ethereum.NotFound
	}
	return header, nil
}

func (r *inclusionReceipts) TransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	r.queried = hash
	return r.receipt, r.err
}

func TestReconcileInclusionTracksCanonicalReinclusion(t *testing.T) {
	old := &types.Header{Number: big.NewInt(10), Extra: []byte("old")}
	replacement := &types.Header{Number: big.NewInt(10), Extra: []byte("replacement")}
	newBlock := &types.Header{Number: big.NewInt(12), Extra: []byte("reincluded")}
	txHash := common.HexToHash("0x1234")
	original := &types.Receipt{
		TxHash: txHash, BlockNumber: big.NewInt(10), BlockHash: old.Hash(), Status: types.ReceiptStatusSuccessful,
	}
	reincluded := &types.Receipt{
		TxHash: txHash, BlockNumber: big.NewInt(12), BlockHash: newBlock.Hash(), Status: types.ReceiptStatusSuccessful,
	}
	reverted := *reincluded
	reverted.Status = types.ReceiptStatusFailed
	wrongHash := *reincluded
	wrongHash.TxHash = common.HexToHash("0x9999")
	untrusted := *reincluded
	untrusted.BlockHash = common.HexToHash("0xdead")
	for _, tc := range []struct {
		name      string
		header    *types.Header
		refreshed *types.Receipt
		err       error
		wantReorg bool
		wantBlock uint64
		wantError bool
		wantRead  bool
	}{
		{name: "canonical original requires no receipt query", header: old, wantBlock: 10},
		{name: "same tx in new block remains completed", header: replacement, refreshed: reincluded, wantBlock: 12, wantRead: true},
		{name: "orphaned and missing", header: replacement, err: ethereum.NotFound, wantReorg: true, wantRead: true},
		{name: "new canonical execution reverted", header: replacement, refreshed: &reverted, wantReorg: true, wantRead: true},
		{name: "receipt RPC error defers", header: replacement, err: errors.New("unavailable"), wantError: true, wantRead: true},
		{name: "wrong receipt hash defers", header: replacement, refreshed: &wrongHash, wantError: true, wantRead: true},
		{name: "noncanonical refreshed receipt defers", header: replacement, refreshed: &untrusted, wantError: true, wantRead: true},
		{name: "nil refreshed receipt defers", header: replacement, wantError: true, wantRead: true},
		{name: "missing old block is inconclusive", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &inclusionReceipts{
				headers: map[uint64]*types.Header{10: tc.header, 12: newBlock}, receipt: tc.refreshed, err: tc.err,
			}
			reorged, canonical, err := ReconcileInclusion(t.Context(), backend, original)
			if reorged != tc.wantReorg || (err != nil) != tc.wantError {
				t.Fatalf("ReconcileInclusion = (%v, %v, %v), want reorg=%v, error=%v", reorged, canonical, err, tc.wantReorg, tc.wantError)
			}
			if tc.wantBlock == 0 {
				if canonical != nil {
					t.Fatalf("unproven completion retained: %+v", canonical)
				}
			} else if canonical == nil || canonical.BlockNumber.Uint64() != tc.wantBlock {
				t.Fatalf("canonical receipt = %+v, want block %d", canonical, tc.wantBlock)
			}
			if (backend.queried != (common.Hash{})) != tc.wantRead || tc.wantRead && backend.queried != txHash {
				t.Fatalf("queried hash = %s, want read=%v of %s", backend.queried, tc.wantRead, txHash)
			}
		})
	}
}

func TestInclusionReorgedOverRPC(t *testing.T) {
	original := &types.Header{Number: big.NewInt(10), Difficulty: big.NewInt(1), Extra: []byte("original")}
	replacement := &types.Header{Number: big.NewInt(10), Difficulty: big.NewInt(1), Extra: []byte("replacement")}
	for _, tc := range []struct {
		name      string
		header    *types.Header
		rpcError  bool
		wantReorg bool
		wantError bool
	}{
		{name: "canonical fill", header: original},
		{name: "replaced block", header: replacement, wantReorg: true},
		{name: "lagged provider missing block", wantError: true},
		{name: "provider failure", rpcError: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantFullTransactions := false
				var request struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
					Params []any           `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if request.Method != "eth_getBlockByNumber" || len(request.Params) != 2 ||
					request.Params[0] != "0xa" || request.Params[1] != wantFullTransactions {
					t.Errorf("unexpected canonical block query: %+v", request)
				}
				w.Header().Set("Content-Type", "application/json")
				response := map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": tc.header}
				if tc.rpcError {
					delete(response, "result")
					response["error"] = map[string]any{"code": -32000, "message": "canonical provider unavailable"}
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			client, err := ethclient.DialContext(t.Context(), server.URL)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)
			reorged, err := InclusionReorged(t.Context(), client, 10, original.Hash())
			if reorged != tc.wantReorg || (err != nil) != tc.wantError {
				t.Fatalf("InclusionReorged = (%v, %v), want (%v, error=%v)", reorged, err, tc.wantReorg, tc.wantError)
			}
		})
	}
}

func (r *inclusionHeaders) HeaderByNumber(_ context.Context, block *big.Int) (*types.Header, error) {
	r.block = new(big.Int).Set(block)
	return r.header, r.err
}

func TestInclusionReorgedRequiresCanonicalBlockEvidence(t *testing.T) {
	original := &types.Header{Number: big.NewInt(10), Extra: []byte("original")}
	replacement := &types.Header{Number: big.NewInt(10), Extra: []byte("replacement")}
	rpcErr := errors.New("RPC unavailable")
	for _, tc := range []struct {
		name       string
		header     *types.Header
		err        error
		hash       common.Hash
		wantReorg  bool
		wantErr    bool
		wantNoRead bool
	}{
		{name: "canonical completion", header: original, hash: original.Hash()},
		{name: "replaced inclusion block", header: replacement, hash: original.Hash(), wantReorg: true},
		{name: "missing block is inconclusive", err: ethereum.NotFound, hash: original.Hash(), wantErr: true},
		{name: "RPC failure is inconclusive", err: rpcErr, hash: original.Hash(), wantErr: true},
		{name: "nil header is inconclusive", hash: original.Hash(), wantErr: true},
		{name: "missing header number", header: &types.Header{}, hash: original.Hash(), wantErr: true},
		{name: "wrong header height", header: &types.Header{Number: big.NewInt(9)}, hash: original.Hash(), wantErr: true},
		{name: "overflowing header height", header: &types.Header{Number: new(big.Int).Lsh(big.NewInt(1), 65)}, hash: original.Hash(), wantErr: true},
		{name: "unknown inclusion hash", header: original, wantErr: true, wantNoRead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &inclusionHeaders{header: tc.header, err: tc.err}
			reorged, err := InclusionReorged(t.Context(), backend, 10, tc.hash)
			if reorged != tc.wantReorg || (err != nil) != tc.wantErr {
				t.Fatalf("InclusionReorged = (%v, %v), want (%v, error=%v)", reorged, err, tc.wantReorg, tc.wantErr)
			}
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("lost RPC cause: %v", err)
			}
			if tc.wantNoRead {
				if backend.block != nil {
					t.Fatal("queried chain without a known inclusion hash")
				}
			} else if backend.block == nil || backend.block.Uint64() != 10 {
				t.Fatalf("read block %v, want retained inclusion height 10", backend.block)
			}
		})
	}
}

func TestReconcileInclusionFindsReincludedTransactionOverRPC(t *testing.T) {
	originalBlock := &types.Header{Number: big.NewInt(10), Difficulty: big.NewInt(1), Extra: []byte("original")}
	replacedBlock := &types.Header{Number: big.NewInt(10), Difficulty: big.NewInt(1), Extra: []byte("replaced")}
	newBlock := &types.Header{Number: big.NewInt(12), Difficulty: big.NewInt(1), Extra: []byte("reincluded")}
	txHash := common.HexToHash("0x1234")
	original := &types.Receipt{
		TxHash: txHash, BlockNumber: big.NewInt(10), BlockHash: originalBlock.Hash(), Status: types.ReceiptStatusSuccessful,
	}
	canonical := &types.Receipt{
		TxHash: txHash, BlockNumber: big.NewInt(12), BlockHash: newBlock.Hash(), Status: types.ReceiptStatusSuccessful,
		Logs: []*types.Log{}, GasUsed: 21_000, CumulativeGasUsed: 21_000,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantFullTransactions := false
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params []any           `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var result any
		switch request.Method {
		case "eth_getBlockByNumber":
			if len(request.Params) != 2 || request.Params[1] != wantFullTransactions {
				t.Errorf("unexpected block request: %+v", request)
				break
			}
			switch request.Params[0] {
			case "0xa":
				result = replacedBlock
			case "0xc":
				result = newBlock
			default:
				t.Errorf("unexpected inclusion height: %+v", request)
			}
		case "eth_getTransactionReceipt":
			if len(request.Params) != 1 || request.Params[0] != txHash.Hex() {
				t.Errorf("unexpected exact-hash receipt request: %+v", request)
			}
			result = canonical
		default:
			t.Errorf("unexpected RPC: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	client, err := ethclient.DialContext(t.Context(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	reorged, refreshed, err := ReconcileInclusion(t.Context(), client, original)
	if err != nil || reorged || refreshed == nil || refreshed.BlockNumber.Uint64() != 12 || refreshed.BlockHash != newBlock.Hash() {
		t.Fatalf("ReconcileInclusion = (%v, %v, %v), want successful canonical re-inclusion at block 12", reorged, refreshed, err)
	}
}
