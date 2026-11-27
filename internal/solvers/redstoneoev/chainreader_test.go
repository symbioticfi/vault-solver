package redstoneoev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/chain"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type unexpectedGasMulticaller struct {
	t *testing.T
}

func (m unexpectedGasMulticaller) MulticallWithTime(context.Context, []chain.Call) ([]chain.CallResult, time.Time, error) {
	m.t.Fatal("gas multicall reached before token coverage validation")
	return nil, time.Time{}, nil
}

func TestReadGasPricesValidatesConfiguredMode(t *testing.T) {
	loan := common.HexToAddress("0x1111111111111111111111111111111111111111")
	adapter := types.AdapterSnapshot{Loan: loan, LoanDecimals: 6}

	t.Run("gas omitted", func(t *testing.T) {
		prices, err := (&reader{}).ReadGasPrices(t.Context(), adapter)
		if err != nil || prices != nil {
			t.Fatalf("prices = %v, error = %v; want nil, nil", prices, err)
		}
	})

	t.Run("configured feed does not cover adapter loan", func(t *testing.T) {
		feed := common.HexToAddress("0x2222222222222222222222222222222222222222")
		otherToken := common.HexToAddress("0x3333333333333333333333333333333333333333")
		oracle, err := liquidlanegas.NewOracleReader(unexpectedGasMulticaller{t}, liquidlanegas.OracleConfig{
			NativeUSDFeed: liquidlanegas.USDFeed{Address: feed, MaxAge: time.Hour},
			TokenUSDFeeds: map[common.Address]liquidlanegas.USDFeed{
				otherToken: {Address: feed, MaxAge: time.Hour},
			},
		})
		if err != nil {
			t.Fatalf("new gas reader: %v", err)
		}
		_, err = (&reader{gas: oracle}).ReadGasPrices(t.Context(), adapter)
		if err == nil || !strings.Contains(err.Error(), "missing USD feed for token "+loan.Hex()) {
			t.Fatalf("error = %v, want missing adapter-loan feed", err)
		}
	})
}

// multicallClient serves every eth_call with one aggregate3 reply carrying results.
func multicallClient(t *testing.T, results ...chain.CallResult) *chain.Client {
	t.Helper()
	parsed, err := multicall3.Multicall3MetaData.ParseABI()
	if err != nil {
		t.Fatal(err)
	}
	reply := make([]multicall3.Multicall3Result, len(results))
	for i, r := range results {
		reply[i] = multicall3.Multicall3Result{Success: r.Success, ReturnData: r.ReturnData}
	}
	encoded, err := parsed.Methods["aggregate3"].Outputs.Pack(reply)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		result := `"0x1"`
		if req.Method == "eth_call" {
			result = `"` + hexutil.Encode(encoded) + `"`
		}
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":%s}`, req.ID, result)
	}))
	t.Cleanup(srv.Close)
	c, err := chain.Dial(t.Context(), []string{srv.URL}, "", "0x0000000000000000000000000000000000000001", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func errorStringRevert(t *testing.T, reason string) []byte {
	t.Helper()
	str, _ := abi.NewType("string", "", nil)
	packed, err := abi.Arguments{{Type: str}}.Pack(reason)
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte{0x08, 0xc3, 0x79, 0xa0}, packed...)
}

func TestReadExecutorStateNamesRevertedSubCall(t *testing.T) {
	ok := chain.CallResult{Success: true, ReturnData: common.LeftPadBytes([]byte{1}, 32)}
	c := multicallClient(t, ok, chain.CallResult{ReturnData: errorStringRevert(t, "paused")}, ok)
	_, err := (&reader{chain: c}).ReadExecutorState(t.Context(), common.Address{1}, common.Address{2})
	if err == nil || !strings.Contains(err.Error(), `.deposits(`) || !strings.Contains(err.Error(), `reverted: "paused"`) {
		t.Fatalf("error = %v, want the deposits sub-call named with its decoded revert", err)
	}
}
