package rfq

import (
	"context"
	"math/big"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestConnectorQuoteUsesBackendCapacityAndCurrentPolicyBudget(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		advertised, budget, reserved, want int64
		observed                           uint64
		wantErr                            string
	}{
		{"backend recipe capacity", 600, 1000, 0, 600, 5, ""},
		{"current policy budget", 1000, 600, 0, 600, 5, ""},
		{"upstream cap", 400, 600, 0, 400, 5, ""},
		{"shared account reservation", 600, 1000, 100, 500, 5, ""},
		{"unknown backend block retains reservations", 600, 1000, 100, 500, 0, ""},
		{"policy snapshot reorganized", 600, 1000, 0, 0, 5, "reorganized"},
		{"policy budget unavailable", 600, 1000, 0, 0, 5, "remainingAssets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			connectorAddress, account := common.HexToAddress("0x42"), common.HexToAddress("0x21")
			tokenIn, tokenOut := common.HexToAddress("0x31"), common.HexToAddress("0x32")
			service := &connectorCapacityRPC{connector: connectorAddress, account: account, output: tokenOut, budget: big.NewInt(tc.budget)}
			service.fail = tc.wantErr == "remainingAssets"
			service.reorg = tc.wantErr == "reorganized"
			server := rpc.NewServer()
			if err := server.RegisterName("eth", service); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(server.Stop)
			httpServer := httptest.NewServer(server)
			t.Cleanup(httpServer.Close)
			client, err := chain.Dial(t.Context(), []string{httpServer.URL}, "", common.HexToAddress("0x66").Hex(), 0)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)
			r := newReader(client, logr.Discard(), common.Address{})
			id := common.HexToHash("0x11")
			inventory := []solverInventory{{Route: liquidlane.NewRoute(1, connectorAddress, account, tokenIn, tokenOut, 6, 6), RecipeExecutor: common.HexToAddress("0x43"), DiscountID: &id, Discount: big.NewInt(1), DiscountPrecision: connectorDiscountPrecision, MaxAssets: big.NewInt(tc.advertised), BlockNumber: tc.observed}}
			pending := func(rows []solverInventory) liquidlane.CapacityReservations {
				if len(rows) != 1 || rows[0].Vault != account || rows[0].BlockNumber != tc.observed {
					t.Fatal("capacity lost backend observation block or pinned account identity")
				}
				return liquidlane.CapacityReservations{liquidlane.NewCapacityID(1, account, tokenOut): big.NewInt(tc.reserved)}
			}
			candidates, err := r.readConnectorCandidates(t.Context(), inventory, tokenIn, tokenOut, big.NewInt(100), pending)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected capacity failure: %v", err)
				}
			} else if err != nil || len(candidates) != 1 || candidates[0].MaxAmountOut.Int64() != tc.want {
				t.Fatalf("candidates=%+v, want %d: %v", candidates, tc.want, err)
			}
			service.mu.Lock()
			defer service.mu.Unlock()
			if service.budgetReads != 1 || service.unexpectedReads != 0 {
				t.Fatalf("budget reads=%d recipe/cash reconstruction=%d", service.budgetReads, service.unexpectedReads)
			}
		})
	}
}

type connectorCapacityRPC struct {
	mu                           sync.Mutex
	connector, account, output   common.Address
	budgetReads, unexpectedReads int
	budget                       *big.Int
	fail, reorg                  bool
	headerReads                  int
}

func (*connectorCapacityRPC) ChainId(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(1)), nil
}
func (s *connectorCapacityRPC) GetBlockByNumber(context.Context, string, bool) (*types.Header, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.headerReads++
	header := &types.Header{Number: big.NewInt(7), Difficulty: big.NewInt(1)}
	if s.reorg && s.headerReads > 1 {
		header.Extra = []byte("reorganized")
	}
	return header, nil
}
func (s *connectorCapacityRPC) Call(_ context.Context, args map[string]any, block string, override *map[string]any) (hexutil.Bytes, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.call(args, block, override)
}

func (s *connectorCapacityRPC) call(args map[string]any, block string, override *map[string]any) (hexutil.Bytes, error) {
	raw, _ := args["input"].(string)
	if raw == "" {
		raw, _ = args["data"].(string)
	}
	data, err := hexutil.Decode(raw)
	if err != nil {
		return nil, err
	}
	to := common.HexToAddress(args["to"].(string))

	if override != nil || to == s.account {
		s.unexpectedReads++
		return nil, errors.New("solver must not reconstruct recipe capacity")
	}
	if len(data) >= 4 && hexutil.Encode(data[:4]) == "0x313ce567" {
		return common.LeftPadBytes([]byte{6}, 32), nil
	}
	if to != s.connector {
		b := multicall3.NewMulticall3()
		contractABI := b.GetABI()
		method, err := contractABI.MethodById(data)
		if err != nil {
			return nil, err
		}
		values, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		calls := *abi.ConvertType(values[0], new([]multicall3.Multicall3Call3)).(*[]multicall3.Multicall3Call3)
		results := make([]multicall3.Multicall3Result, len(calls))
		for i := range results {
			result, err := s.call(map[string]any{"to": calls[i].Target.Hex(), "input": hexutil.Encode(calls[i].CallData)}, block, nil)
			if err != nil {
				return nil, err
			}
			results[i] = multicall3.Multicall3Result{Success: true, ReturnData: result}
		}
		return method.Outputs.Pack(results)
	}
	if block != "0x7" {
		return nil, errors.New("connector read escaped pinned block")
	}
	contractABI := connector.NewLiquidLaneConnector().GetABI()
	method, err := contractABI.MethodById(data)
	if err != nil {
		return nil, err
	}
	switch method.RawName {
	case "eip712Domain":
		return method.Outputs.Pack([1]byte{15}, "LiquidLaneConnector", "1", big.NewInt(1), s.connector, [32]byte{}, []*big.Int{})
	case "account":
		return method.Outputs.Pack(s.account)
	case "outputAsset":
		return method.Outputs.Pack(s.output)
	case "remainingAssets":
		s.budgetReads++
		if s.fail {
			return nil, errors.New("policy budget unavailable")
		}
		return method.Outputs.Pack(s.budget)
	case "minDiscount":
		return method.Outputs.Pack(big.NewInt(0))
	case "getAmountOut":
		values, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		return method.Outputs.Pack(values[1])
	default:
		s.unexpectedReads++
		return nil, errors.New("unexpected cash or budget reconstruction")
	}
}

func (s *connectorCapacityRPC) GetLogs(context.Context, map[string]any) ([]types.Log, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unexpectedReads++
	return nil, errors.New("solver must not discover recipes for quote capacity")
}
