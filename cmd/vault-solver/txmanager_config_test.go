package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func loadTxManagerConfig(t *testing.T, txManager string) config.TxManagerConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "chain: {rpcUrl: http://x, chainId: 1}\nsigner: {keyEnv: K}\nsolvers: [{name: x}]\n" + txManager
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg.TxManager
}

// The YAML defaults and the manager's zero-value defaults are two copies of one table: an operator
// who omits every new key must get exactly what a Config built in code gets.
func TestTxManagerConfigYAMLDefaultsMatchZeroValue(t *testing.T) {
	got := txManagerConfig(loadTxManagerConfig(t, "txManager: {maxFeeGwei: 50}\n")).WithDefaults()
	want := txmanager.Config{}.WithDefaults()
	if !reflect.DeepEqual(got.Fees, want.Fees) || !reflect.DeepEqual(got.Gas, want.Gas) ||
		!reflect.DeepEqual(got.Balance, want.Balance) || !reflect.DeepEqual(got.Shadow, want.Shadow) {
		t.Fatalf("YAML defaults diverge from txmanager zero-value defaults:\n got fees=%+v gas=%+v balance=%+v shadow=%+v\nwant fees=%+v gas=%+v balance=%+v shadow=%+v",
			got.Fees, got.Gas, got.Balance, got.Shadow, want.Fees, want.Gas, want.Balance, want.Shadow)
	}
}

func TestTxManagerConfigMapsEveryKnob(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want txmanager.Config
	}{
		{
			name: "every key set",
			yaml: `txManager:
  confirmations: 3
  maxFeeGwei: 50
  broadcastTimeoutMs: 4000
  accountPollIntervalMs: 12000
  replacementIntervalMs: 15000
  pendingTimeoutMs: 120000
  shutdownTimeoutMs: 30000
  fees:
    policy: horizon
    blockTimeMs: 6000
    minHorizonBlocks: 3
    maxHorizonBlocks: 12
    pricingHorizonBlocks: 7
    maxHeadLagBlocks: 4
    tipFloorGwei: 0.01
    singleFullBlockTipGwei: 0.05
    congestedTipFloorGwei: 0.5
    congestedTipCapGwei: 5
    congestedRewardBlocks: 4
    congestedRewardPercentile: 75
    escalateAfterFullMisses: 1
    stallAfterRoomyMisses: 5
  gas: {headroomBps: 800, nextBlockEstimate: true, fallbackHeadroomBps: 1500, estimateTimeoutMs: 2500}
  balance: {guard: true, referenceGasUnits: 4350000, fundingHysteresisBps: 1000, targetEth: 0.1}
  shadow: {enabled: true}
`,
			want: txmanager.Config{
				Confirmations: 3, MaxFeeGwei: 50,
				BroadcastTimeout: 4 * time.Second, AccountPollInterval: 12 * time.Second,
				ReplacementInterval: 15 * time.Second, PendingTimeout: 2 * time.Minute, ShutdownTimeout: 30 * time.Second,
				Fees: txmanager.FeeConfig{
					Policy: txmanager.FeePolicyHorizon, BlockTime: 6 * time.Second, MinHorizonBlocks: 3,
					MaxHorizonBlocks: 12, PricingHorizonBlocks: 7, MaxHeadLagBlocks: new(uint64(4)),
					TipFloorGwei: 0.01, SingleFullBlockTipGwei: 0.05, CongestedTipFloorGwei: 0.5,
					CongestedTipCapGwei: 5, CongestedRewardBlocks: 4, CongestedRewardPercentile: 75,
					EscalateAfterFullMisses: 1, StallAfterRoomyMisses: 5,
				},
				Gas: txmanager.GasConfig{
					HeadroomBps: new(uint64(800)), FallbackHeadroomBps: new(uint64(1500)), EstimateTimeout: 2500 * time.Millisecond,
				},
				Balance: txmanager.BalanceConfig{
					ReferenceGasUnits: 4_350_000, FundingHysteresisBps: new(uint64(1000)), TargetEth: 0.1,
				},
			},
		},
		{
			name: "explicit zero and false reach the manager",
			yaml: `txManager:
  maxFeeGwei: 50
  fees: {maxHeadLagBlocks: 0}
  gas: {headroomBps: 0, fallbackHeadroomBps: 0, nextBlockEstimate: false}
  balance: {guard: false, fundingHysteresisBps: 0}
  shadow: {enabled: false}
`,
			want: func() txmanager.Config {
				c := txmanager.Config{MaxFeeGwei: 50}.WithDefaults()
				c.PollInterval = 0 // not a YAML knob; the manager defaults it
				c.Confirmations = config.DefaultConfirmations
				c.Fees.MaxHeadLagBlocks = new(uint64(0))
				c.Gas.HeadroomBps = new(uint64(0))
				c.Gas.FallbackHeadroomBps = new(uint64(0))
				c.Gas.NextBlockEstimateDisabled = true
				c.Balance.GuardDisabled = true
				c.Balance.FundingHysteresisBps = new(uint64(0))
				c.Shadow.Disabled = true
				return c
			}(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := txManagerConfig(loadTxManagerConfig(t, tc.yaml)); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("txManagerConfig =\n%+v\nwant\n%+v", got, tc.want)
			}
		})
	}
}
