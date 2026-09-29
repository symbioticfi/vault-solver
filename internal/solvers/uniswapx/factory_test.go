package uniswapx

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

// Under the horizon policy gas accounting prices quotes from the manager's reference fill gas limit (fee-gas
// strategy §2.9, §2.12 PR2), so the factory refuses it while txManager.balance.referenceGasUnits is unset.
// Under the legacy policy (the default) the limit only drives the funding gate, and a gas: lane without it
// starts as it did before the key existed.
func TestFactoryRequiresReferenceGasUnitsWithGasAccounting(t *testing.T) {
	t.Setenv("UNISWAP_API_KEY", "")
	withoutGas := strings.Replace(validUniswapXConfig, validUniswapXGasConfig, "", 1)
	for _, tc := range []struct {
		name              string
		raw               string
		policy            txmanager.FeePolicy
		referenceGasUnits uint64
		wantRefused       bool
	}{
		{name: "horizon gas accounting without reference gas", raw: validUniswapXConfig, policy: txmanager.FeePolicyHorizon, wantRefused: true},
		{name: "horizon gas accounting with reference gas", raw: validUniswapXConfig, policy: txmanager.FeePolicyHorizon, referenceGasUnits: 4_400_000},
		{name: "legacy gas accounting without reference gas", raw: validUniswapXConfig},
		{name: "legacy gas accounting with reference gas", raw: validUniswapXConfig, referenceGasUnits: 4_400_000},
		{name: "no gas accounting needs no reference gas", raw: withoutGas, policy: txmanager.FeePolicyHorizon},
	} {
		t.Run(tc.name, func(t *testing.T) {
			txm := txmanager.New(nil, nil, big.NewInt(1), txmanager.Config{
				Fees:    txmanager.FeeConfig{Policy: tc.policy},
				Balance: txmanager.BalanceConfig{ReferenceGasUnits: tc.referenceGasUnits},
			}, logr.Discard())
			_, err := factory(uniswapXConfigNode(t, tc.raw), solver.Deps{TxManager: txm, Log: logr.Discard()})
			if got := errors.Is(err, liquidlanegas.ErrReferenceGasUnitsRequired); got != tc.wantRefused {
				t.Fatalf("factory error = %v, refused for reference gas = %t, want %t", err, got, tc.wantRefused)
			}
			// Past the check, the factory stops at the deliberately missing order API key.
			if !tc.wantRefused && (err == nil || !strings.Contains(err.Error(), "API key")) {
				t.Fatalf("factory error = %v, want the missing API key error after the reference gas check", err)
			}
		})
	}
}
