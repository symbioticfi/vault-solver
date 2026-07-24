package uniswapx

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

func TestRequireExecutorCode(t *testing.T) {
	executor := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tests := []struct {
		name      string
		code      []byte
		wantError string
	}{
		{name: "contract", code: []byte{0x60}},
		{name: "empty account", wantError: "has no bytecode"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := requireExecutorCode(executor, tc.code)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("requireExecutorCode() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("requireExecutorCode() error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

func TestRequireExecutorCaller(t *testing.T) {
	caller := common.HexToAddress("0x1111111111111111111111111111111111111111")
	other := common.HexToAddress("0x2222222222222222222222222222222222222222")
	encoded := func(address common.Address) []byte {
		return common.LeftPadBytes(address.Bytes(), 32)
	}

	tests := []struct {
		name      string
		results   []chain.CallResult
		wantError string
	}{
		{
			name: "authorized",
			results: []chain.CallResult{
				{Success: true, ReturnData: encoded(other)},
				{Success: true, ReturnData: encoded(caller)},
				{Success: false},
			},
		},
		{
			name: "not authorized",
			results: []chain.CallResult{
				{Success: true, ReturnData: encoded(other)},
				{Success: false},
			},
			wantError: "is not authorized",
		},
		{
			name:      "malformed",
			results:   []chain.CallResult{{Success: true, ReturnData: []byte{1}}},
			wantError: "decode executor caller 0",
		},
		{
			name:      "safety limit",
			results:   []chain.CallResult{{Success: true, ReturnData: encoded(other)}},
			wantError: "safety limit",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := requireExecutorCaller(caller, tc.results)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("requireExecutorCaller() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("requireExecutorCaller() error = %v, want %q", err, tc.wantError)
			}
		})
	}
}
