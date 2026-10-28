package rfq

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

var observationBad = common.HexToAddress("0xbad")
var observationGood = common.HexToAddress("0x42")
var observationNative = common.HexToAddress("0x43")
var observationAccount = common.HexToAddress("0x21")

func TestConnectorFailurePreservesHealthyCandidates(t *testing.T) {
	for _, failure := range []string{"eip712Domain", "outputAsset", "account", "getMaxAssets", "minDiscount", "getAmountOut", "decode domain", "wrong domain", "wrong output", "zero account", "malformed capacity", "wrong quote"} {
		t.Run(failure, func(t *testing.T) {
			service := &connectorObservationRPC{failure: failure}
			r := connectorObservationReader(t, service)
			native := testInventory(observationNative, tIn, tOut, big.NewInt(600), big.NewInt(1e18))
			native.TokenInDecimals = 6
			r.setQuoteAdapters([]recoveryVault{{Adapter: observationNative, Vault: observationNative, TokenOut: tOut, TokenOutDecimals: 6}})
			inventory := []solverInventory{observationInventory(observationBad, 1), observationInventory(observationGood, 2), native}
			candidates, err := r.readQuoteCandidates(t.Context(), inventory, tIn, tOut, big.NewInt(100), nil)
			if err != nil || len(candidates) != 2 {
				t.Fatalf("bad connector suppressed healthy native/connector routes: candidates=%+v err=%v", candidates, err)
			}
			for _, candidate := range candidates {
				if candidate.Route.Adapter == observationBad {
					t.Fatal("failed connector remained eligible")
				}
			}
		})
	}
}

func TestConnectorObservationSharesReadsAcrossDiscountAlternatives(t *testing.T) {
	service := &connectorObservationRPC{}
	r := connectorObservationReader(t, service)
	inventory := make([]solverInventory, 256)
	for i := range inventory {
		inventory[i] = observationInventory(observationGood, int64(i+1))
	}
	candidates, err := r.readConnectorCandidates(t.Context(), inventory, tIn, tOut, big.NewInt(100), nil)
	if err != nil || len(candidates) == 0 {
		t.Fatalf("healthy catalog rejected: %v", err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.connectorCalls != 7 || service.batches != 1 || service.largestBatch != 7 {
		t.Fatalf("duplicate/serial connector observations: calls=%d batches=%d largest=%d", service.connectorCalls, service.batches, service.largestBatch)
	}
}

func TestConnectorObservationSharedFailuresRemainErrors(t *testing.T) {
	for _, failure := range []string{"header", "nil header", "decimals", "batch"} {
		t.Run(failure, func(t *testing.T) {
			service := &connectorObservationRPC{failure: failure}
			r := connectorObservationReader(t, service)
			_, err := r.readConnectorCandidates(t.Context(), []solverInventory{observationInventory(observationGood, 1)}, tIn, tOut, big.NewInt(100), nil)
			if err == nil {
				t.Fatalf("shared %s failure was swallowed", failure)
			}
		})
	}
}

func TestConnectorObservationChunksMaximumIndependentCatalog(t *testing.T) {
	service := &connectorObservationRPC{connectors: make(map[common.Address]bool)}
	inventory := make([]solverInventory, 256)
	for i := range inventory {
		address := common.BigToAddress(big.NewInt(int64(1000 + i)))
		service.connectors[address] = true
		inventory[i] = observationInventory(address, int64(i+1))
	}
	r := connectorObservationReader(t, service)
	if candidates, err := r.readConnectorCandidates(t.Context(), inventory, tIn, tOut, big.NewInt(100), nil); err != nil || len(candidates) == 0 {
		t.Fatalf("maximum catalog rejected: candidates=%+v err=%v", candidates, err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.connectorCalls != 1792 || service.batches != 7 || service.largestBatch != 256 {
		t.Fatalf("independent observations escaped bounded batches: calls=%d batches=%d largest=%d", service.connectorCalls, service.batches, service.largestBatch)
	}
}

func observationInventory(address common.Address, discount int64) solverInventory {
	id := common.BigToHash(big.NewInt(discount))
	return solverInventory{Route: liquidlane.NewRoute(1, address, common.Address{}, tIn, tOut, 6, 6), Kind: "connector", DiscountID: &id, Discount: big.NewInt(discount), DiscountPrecision: connectorDiscountPrecision, MaxAssets: big.NewInt(600)}
}

func connectorObservationReader(t *testing.T, service *connectorObservationRPC) *reader {
	t.Helper()
	server := rpc.NewServer()
	if err := server.RegisterName("eth", service); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Stop)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if len(body) > 0 && body[0] == '[' {
			var batch []json.RawMessage
			if err := json.Unmarshal(body, &batch); err != nil {
				t.Error(err)
				return
			}
			service.mu.Lock()
			service.batches++
			service.largestBatch = max(service.largestBatch, len(batch))
			service.mu.Unlock()
			if service.failure == "batch" {
				_, _ = w.Write([]byte("[]"))
				return
			}
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		server.ServeHTTP(w, req)
	}))
	t.Cleanup(httpServer.Close)
	client, err := chain.Dial(t.Context(), []string{httpServer.URL}, "", common.HexToAddress("0x66").Hex(), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return newReader(client, logr.Discard(), common.Address{})
}

type connectorObservationRPC struct {
	mu                                    sync.Mutex
	failure                               string
	connectorCalls, batches, largestBatch int
	connectors                            map[common.Address]bool
}

func (*connectorObservationRPC) ChainId(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(1)), nil
}

func (s *connectorObservationRPC) GetBlockByNumber(context.Context, string, bool) (*types.Header, error) {
	if s.failure == "header" {
		return nil, errors.New("header unavailable")
	}
	if s.failure == "nil header" {
		return &types.Header{Difficulty: big.NewInt(1)}, nil
	}
	return &types.Header{Number: big.NewInt(7), Difficulty: big.NewInt(1)}, nil
}

func (s *connectorObservationRPC) Call(_ context.Context, args map[string]any, block string) (hexutil.Bytes, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _ := args["input"].(string)
	if raw == "" {
		raw, _ = args["data"].(string)
	}
	data, err := hexutil.Decode(raw)
	if err != nil {
		return nil, err
	}
	to := common.HexToAddress(args["to"].(string))
	if to != observationBad && to != observationGood && !s.connectors[to] {
		method := multicall3.NewMulticall3().GetABI().Methods["aggregate3"]
		values, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		calls := *abi.ConvertType(values[0], new([]multicall3.Multicall3Call3)).(*[]multicall3.Multicall3Call3)
		results := make([]multicall3.Multicall3Result, len(calls))
		for i, call := range calls {
			result := common.LeftPadBytes([]byte{6}, 32)
			if call.Target == observationNative {
				contractABI := adapter.NewLiquidLaneAdapter().GetABI()
				m, err := contractABI.MethodById(call.CallData)
				if err != nil {
					return nil, err
				}
				switch m.RawName {
				case "paused":
					result, err = m.Outputs.Pack(false)
				case "getMaxAssets":
					result, err = m.Outputs.Pack(big.NewInt(600))
				case "getAmountOut":
					result, err = m.Outputs.Pack(big.NewInt(100))
				case "minDiscount":
					result, err = m.Outputs.Pack(new(big.Int))
				}
				if err != nil {
					return nil, err
				}
			}
			results[i] = multicall3.Multicall3Result{Success: s.failure != "decimals", ReturnData: result}
		}
		return method.Outputs.Pack(results)
	}
	if block != "0x7" || common.HexToAddress(args["from"].(string)) != (common.Address{}) {
		return nil, errors.New("connector read changed block or caller")
	}
	s.connectorCalls++
	contractABI := connector.NewLiquidLaneConnector().GetABI()
	method, err := contractABI.MethodById(data)
	if err != nil {
		return nil, err
	}
	if to == observationBad && s.failure == method.RawName {
		return nil, errors.New("unavailable connector source")
	}
	switch method.RawName {
	case "eip712Domain":
		if to == observationBad && s.failure == "decode domain" {
			return []byte{1}, nil
		}
		name := "LiquidLaneConnector"
		if to == observationBad && s.failure == "wrong domain" {
			name = "wrong"
		}
		return method.Outputs.Pack([1]byte{15}, name, "1", big.NewInt(1), to, [32]byte{}, []*big.Int{})
	case "account":
		if to == observationBad && s.failure == "zero account" {
			return method.Outputs.Pack(common.Address{})
		}
		return method.Outputs.Pack(observationAccount)
	case "outputAsset":
		if to == observationBad && s.failure == "wrong output" {
			return method.Outputs.Pack(tIn)
		}
		return method.Outputs.Pack(tOut)
	case "getMaxAssets":
		if to == observationBad && s.failure == "malformed capacity" {
			return []byte{1}, nil
		}
		return method.Outputs.Pack(big.NewInt(600))
	case "minDiscount":
		return method.Outputs.Pack(new(big.Int))
	case "getAmountOut":
		values, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		if to == observationBad && s.failure == "wrong quote" && values[1].(*big.Int).Cmp(big.NewInt(100)) == 0 {
			return method.Outputs.Pack(big.NewInt(99))
		}
		return method.Outputs.Pack(values[1])
	}
	return nil, errors.New("unexpected connector read")
}
