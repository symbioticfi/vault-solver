package uniswapx

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

func TestRequireConfirmationDepth(t *testing.T) {
	if err := requireConfirmationDepth(big.NewInt(100), big.NewInt(101), 2); err == nil {
		t.Fatal("unconfirmed receipt was accepted")
	}
	if err := requireConfirmationDepth(big.NewInt(100), big.NewInt(102), 2); err != nil {
		t.Fatalf("confirmed receipt rejected: %v", err)
	}
}

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

func TestPermit2NonceBit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		nonce    *big.Int
		wantWord *big.Int
		wantBit  int
	}{
		{name: "zero", nonce: big.NewInt(0), wantWord: big.NewInt(0), wantBit: 0},
		{name: "last bit of first word", nonce: big.NewInt(255), wantWord: big.NewInt(0), wantBit: 255},
		{name: "first bit of second word", nonce: big.NewInt(256), wantWord: big.NewInt(1), wantBit: 0},
		{name: "mid word", nonce: big.NewInt(0x1_2345), wantWord: big.NewInt(0x123), wantBit: 0x45},
		{
			name:     "uint256 max",
			nonce:    new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)),
			wantWord: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 248), big.NewInt(1)),
			wantBit:  255,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			word, bit := permit2NonceBit(tc.nonce)
			if word.Cmp(tc.wantWord) != 0 || bit != tc.wantBit {
				t.Fatalf("permit2NonceBit(%s) = %s/%d, want %s/%d", tc.nonce, word, bit, tc.wantWord, tc.wantBit)
			}
		})
	}
}
