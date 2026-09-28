package lifi

import (
	"math/big"
	"strings"
	"testing"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const factoryTestConfig = `
orderServer:
  baseUrl: https://order.example
  wsUrl: wss://order.example
  apiKeyEnv: LIFI_SOLVER_API_KEY
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters:
  - "0x5555555555555555555555555555555555555555"
`

const factoryTestGasConfig = `gas:
  nativeUsdFeed: "0x7777777777777777777777777777777777777777"
  nativeMaxAge: 30m
  tokenUsdFeeds:
    - token: "0x6666666666666666666666666666666666666666"
      feed: "0x8888888888888888888888888888888888888888"
      maxAge: 1h
`

// Gas accounting prices quotes from the manager's reference fill gas limit (fee-gas strategy §2.9,
// §2.12 PR2), so the factory refuses it while txManager.balance.referenceGasUnits is unset.
func TestFactoryRequiresReferenceGasUnitsWithGasAccounting(t *testing.T) {
	t.Setenv("LIFI_SOLVER_API_KEY", "")
	for _, tc := range []struct {
		name              string
		raw               string
		referenceGasUnits uint64
		wantRefused       bool
	}{
		{name: "gas accounting without reference gas", raw: factoryTestConfig + factoryTestGasConfig, wantRefused: true},
		{name: "gas accounting with reference gas", raw: factoryTestConfig + factoryTestGasConfig, referenceGasUnits: 4_400_000},
		{name: "no gas accounting needs no reference gas", raw: factoryTestConfig},
	} {
		t.Run(tc.name, func(t *testing.T) {
			txm := txmanager.New(nil, nil, big.NewInt(1), txmanager.Config{
				Balance: txmanager.BalanceConfig{ReferenceGasUnits: tc.referenceGasUnits},
			}, logr.Discard())
			_, err := factory(parseYAMLNode(t, tc.raw), solver.Deps{TxManager: txm, Log: logr.Discard()})
			if got := errors.Is(err, liquidlanegas.ErrReferenceGasUnitsRequired); got != tc.wantRefused {
				t.Fatalf("factory error = %v, refused for reference gas = %t, want %t", err, got, tc.wantRefused)
			}
			// Past the check, the factory stops at the deliberately missing order API key.
			if !tc.wantRefused && (err == nil || !strings.Contains(err.Error(), "api key")) {
				t.Fatalf("factory error = %v, want the missing API key error after the reference gas check", err)
			}
		})
	}
}
