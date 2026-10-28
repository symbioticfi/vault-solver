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

func TestConnectorQuoteUsesRecipeCapacityWithoutCashReconstruction(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		advertised, reserved, want int64
		fail                       bool
	}{
		{"funded by recipe", 1000, 0, 600, false},
		{"upstream cap", 400, 0, 400, false},
		{"shared account reservation", 1000, 100, 500, false},
		{"unavailable does not fall back", 1000, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			connectorAddress, account := common.HexToAddress("0x42"), common.HexToAddress("0x21")
			tokenIn, tokenOut := common.HexToAddress("0x31"), common.HexToAddress("0x32")
			service := &connectorCapacityRPC{connector: connectorAddress, account: account, output: tokenOut, fail: tc.fail}
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
			inventory := []solverInventory{{Route: liquidlane.NewRoute(1, connectorAddress, account, tokenIn, tokenOut, 6, 6), Kind: "connector", DiscountID: &id, Discount: big.NewInt(1), DiscountPrecision: connectorDiscountPrecision, MaxAssets: big.NewInt(tc.advertised)}}
			pending := func(rows []solverInventory) liquidlane.CapacityReservations {
				if len(rows) != 1 || rows[0].Vault != account || rows[0].BlockNumber != 7 {
					t.Fatal("capacity lost pinned account identity")
				}
				return liquidlane.CapacityReservations{liquidlane.NewCapacityID(1, account, tokenOut): big.NewInt(tc.reserved)}
			}
			candidates, err := r.readConnectorCandidates(t.Context(), inventory, tokenIn, tokenOut, big.NewInt(100), pending)
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), "getMaxAssets") {
					t.Fatalf("expected capacity failure: %v", err)
				}
			} else if err != nil || len(candidates) != 1 || candidates[0].MaxAmountOut.Int64() != tc.want {
				t.Fatalf("candidates=%+v, want %d: %v", candidates, tc.want, err)
			}
			service.mu.Lock()
			defer service.mu.Unlock()
			if service.capacityReads != 1 || service.balanceReads != 0 {
				t.Fatalf("capacity reads=%d cash reconstruction=%d", service.capacityReads, service.balanceReads)
			}
		})
	}
}

type connectorCapacityRPC struct {
	mu                          sync.Mutex
	connector, account, output  common.Address
	capacityReads, balanceReads int
	fail                        bool
}

func (*connectorCapacityRPC) ChainId(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(1)), nil
}
func (*connectorCapacityRPC) GetBlockByNumber(context.Context, string, bool) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(7), Difficulty: big.NewInt(1)}, nil
}
func (s *connectorCapacityRPC) Call(_ context.Context, args map[string]any, block string) (hexutil.Bytes, error) {
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
			results[i] = multicall3.Multicall3Result{Success: true, ReturnData: common.LeftPadBytes([]byte{6}, 32)}
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
	case "getMaxAssets":
		s.capacityReads++
		if s.fail {
			return nil, errors.New("recipe unavailable")
		}
		return method.Outputs.Pack(big.NewInt(600))
	case "minDiscount":
		return method.Outputs.Pack(big.NewInt(0))
	case "getAmountOut":
		values, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		return method.Outputs.Pack(values[1])
	default:
		s.balanceReads++
		return nil, errors.New("unexpected cash or budget reconstruction")
	}
}
