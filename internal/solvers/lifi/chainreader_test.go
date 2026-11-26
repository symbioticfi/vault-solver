package lifi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/executor"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

func TestValidateZeroGovernanceFee(t *testing.T) {
	tests := []struct {
		name    string
		fee     uint64
		wantErr string
	}{
		{name: "zero", fee: 0},
		{name: "non-zero", fee: 1, wantErr: "governance fee is 1, expected zero"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := governanceFeeRPC(t, tt.fee)
			defer server.Close()

			client, err := chain.Dial(t.Context(), []string{server.URL}, "", common.Address{}.Hex(), 20*time.Second)
			if err != nil {
				t.Fatalf("chain.Dial: %v", err)
			}
			defer client.Close()

			err = (&reader{chain: client}).validateZeroGovernanceFee(
				t.Context(), common.HexToAddress("0x1111111111111111111111111111111111111111"),
			)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("validateZeroGovernanceFee: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("validateZeroGovernanceFee error = %v", err)
			}
		})
	}
}

func governanceFeeRPC(t *testing.T, fee uint64) *httptest.Server {
	t.Helper()
	return callRPC(t, func() string { return fmt.Sprintf(`"result":"0x%064x"`, fee) })
}

// callRPC answers eth_call with the JSON member respond returns (a "result" or an "error") and
// every other method with a chain id.
func callRPC(t *testing.T, respond func() string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var rpcRequest struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read RPC request: %v", err)
		}
		if err := json.Unmarshal(body, &rpcRequest); err != nil {
			t.Errorf("decode RPC request: %v", err)
		}
		member := `"result":"0xaa36a7"`
		if rpcRequest.Method == "eth_call" {
			member = respond()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,%s}`, rpcRequest.ID, member)
	}))
}

func dialTestReader(t *testing.T, respond func() string) *reader {
	t.Helper()
	server := callRPC(t, respond)
	t.Cleanup(server.Close)
	client, err := chain.Dial(t.Context(), []string{server.URL}, "", common.Address{}.Hex(), 20*time.Second)
	if err != nil {
		t.Fatalf("chain.Dial: %v", err)
	}
	t.Cleanup(client.Close)
	return &reader{chain: client}
}

func notCallerRevert(t *testing.T) []byte {
	t.Helper()
	parsed, err := executor.LiquidLaneLifiExecutorMetaData.ParseABI()
	if err != nil {
		t.Fatal(err)
	}
	id := parsed.Errors["NotCaller"].ID
	return id[:4]
}

func revertMember(data []byte) string {
	if data == nil {
		return `"error":{"code":3,"message":"execution reverted"}`
	}
	return fmt.Sprintf(`"error":{"code":3,"message":"execution reverted","data":%q}`, hexutil.Encode(data))
}

// Mined-revert replay is the shared chain.RevertDecoder.MinedRevert and is tested there.
func TestOrderStatusDecodesRevert(t *testing.T) {
	tests := []struct {
		name    string
		respond string
		want    string
	}{
		{name: "custom error", respond: revertMember(notCallerRevert(t)), want: "call orderStatus: execution reverted (revert NotCaller())"},
		{name: "no revert data", respond: revertMember(nil), want: "call orderStatus: execution reverted (node returned no revert data)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := dialTestReader(t, func() string { return tt.respond })
			_, err := r.orderStatus(t.Context(), common.Address{2}, common.Hash{3})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("orderStatus error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestValidateExecutorNamesRevertedRead(t *testing.T) {
	parsed, err := multicall3.Multicall3MetaData.ParseABI()
	if err != nil {
		t.Fatal(err)
	}
	address := func(a common.Address) []byte { return common.LeftPadBytes(a.Bytes(), 32) }
	inputSettler, outputSettler := common.Address{4}, common.Address{5}
	ret, err := parsed.Methods["aggregate3"].Outputs.Pack([]multicall3.Multicall3Result{
		{Success: true, ReturnData: address(inputSettler)},
		{Success: true, ReturnData: address(outputSettler)},
		{Success: false, ReturnData: notCallerRevert(t)},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := dialTestReader(t, func() string { return fmt.Sprintf(`"result":%q`, hexutil.Encode(ret)) })

	err = r.validateExecutor(t.Context(), common.Address{1}, inputSettler, outputSettler, common.Address{6})
	if want := "isCaller() reverted (revert NotCaller())"; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("validateExecutor error = %v, want it to contain %q", err, want)
	}
}
