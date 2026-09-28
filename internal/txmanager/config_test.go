package txmanager

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestConfigWithDefaults(t *testing.T) {
	defaults := Config{
		PollInterval:        2 * time.Second,
		BroadcastTimeout:    5 * time.Second,
		AccountPollInterval: 30 * time.Second,
		ReplacementInterval: 30 * time.Second,
		PendingTimeout:      5 * time.Minute,
		ShutdownTimeout:     time.Minute,
		Fees: FeeConfig{
			Policy:                    FeePolicyLegacy,
			BlockTime:                 12 * time.Second,
			MinHorizonBlocks:          2,
			MaxHorizonBlocks:          6,
			PricingHorizonBlocks:      5,
			MaxHeadLagBlocks:          new(uint64(2)),
			TipFloorGwei:              0.02,
			SingleFullBlockTipGwei:    0.1,
			CongestedTipFloorGwei:     0.2,
			CongestedTipCapGwei:       15,
			CongestedRewardBlocks:     3,
			CongestedRewardPercentile: 50,
			EscalateAfterFullMisses:   2,
			StallAfterRoomyMisses:     3,
		},
		Gas: GasConfig{
			HeadroomBps:         new(uint64(500)),
			FallbackHeadroomBps: new(uint64(1000)),
			EstimateTimeout:     5 * time.Second,
		},
		Balance: BalanceConfig{FundingHysteresisBps: new(uint64(2000))},
	}
	explicit := Config{
		Confirmations:       3,
		MaxFeeGwei:          50,
		TipGwei:             1,
		PollInterval:        time.Second,
		BroadcastTimeout:    time.Second,
		AccountPollInterval: 12 * time.Second,
		ReplacementInterval: 15 * time.Second,
		PendingTimeout:      time.Minute,
		ShutdownTimeout:     10 * time.Second,
		Fees: FeeConfig{
			Policy:                    FeePolicyHorizon,
			BlockTime:                 6 * time.Second,
			MinHorizonBlocks:          3,
			MaxHorizonBlocks:          12,
			PricingHorizonBlocks:      7,
			MaxHeadLagBlocks:          new(uint64(0)),
			TipFloorGwei:              0.01,
			SingleFullBlockTipGwei:    0.05,
			CongestedTipFloorGwei:     0.5,
			CongestedTipCapGwei:       5,
			CongestedRewardBlocks:     4,
			CongestedRewardPercentile: 75,
			EscalateAfterFullMisses:   1,
			StallAfterRoomyMisses:     5,
		},
		Gas: GasConfig{
			HeadroomBps:               new(uint64(0)),
			NextBlockEstimateDisabled: true,
			FallbackHeadroomBps:       new(uint64(0)),
			EstimateTimeout:           time.Second,
		},
		Balance: BalanceConfig{
			GuardDisabled:        true,
			ReferenceGasUnits:    4_350_000,
			FundingHysteresisBps: new(uint64(0)),
			TargetEth:            0.1,
		},
		Shadow: ShadowConfig{Disabled: true},
	}
	for _, tc := range []struct {
		name string
		in   Config
		want Config
	}{
		{name: "zero value is the safe default", in: Config{}, want: defaults},
		{name: "negative durations and amounts take defaults", in: Config{
			PollInterval: -1, BroadcastTimeout: -1, AccountPollInterval: -1, ReplacementInterval: -1,
			PendingTimeout: -1, ShutdownTimeout: -1,
			Fees: FeeConfig{BlockTime: -1, TipFloorGwei: -1, SingleFullBlockTipGwei: -1, CongestedTipFloorGwei: -1,
				CongestedTipCapGwei: -1, CongestedRewardPercentile: -1},
			Gas: GasConfig{EstimateTimeout: -1},
		}, want: defaults},
		{name: "explicit values, including zero basis points and lag, are kept", in: explicit, want: explicit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.WithDefaults(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("WithDefaults() =\n%+v\nwant\n%+v", got, tc.want)
			}
		})
	}
}

// The normalized pointers are fresh copies: a caller that later reuses or mutates its Config cannot
// change a running manager's gas headroom or head-lag tolerance.
func TestConfigWithDefaultsDoesNotAlias(t *testing.T) {
	headroom, lag, hysteresis := uint64(800), uint64(1), uint64(100)
	in := Config{
		Fees:    FeeConfig{MaxHeadLagBlocks: &lag},
		Gas:     GasConfig{HeadroomBps: &headroom},
		Balance: BalanceConfig{FundingHysteresisBps: &hysteresis},
	}
	got := in.WithDefaults()
	if got.Gas.HeadroomBps == &headroom || got.Fees.MaxHeadLagBlocks == &lag || got.Balance.FundingHysteresisBps == &hysteresis {
		t.Fatal("WithDefaults aliased a caller pointer")
	}
	headroom, lag, hysteresis = 0, 0, 0
	if *got.Gas.HeadroomBps != 800 || *got.Fees.MaxHeadLagBlocks != 1 || *got.Balance.FundingHysteresisBps != 100 {
		t.Fatalf("normalized values changed with the caller's: headroom=%d lag=%d hysteresis=%d",
			*got.Gas.HeadroomBps, *got.Fees.MaxHeadLagBlocks, *got.Balance.FundingHysteresisBps)
	}
}

func TestNewNormalizesConfig(t *testing.T) {
	m := New(newMockBackend(), nil, big.NewInt(1), Config{}, logr.Discard())
	if !reflect.DeepEqual(m.cfg, Config{}.WithDefaults()) {
		t.Fatalf("New stored %+v, want the normalized zero Config", m.cfg)
	}
}

func TestReferenceGasUnits(t *testing.T) {
	for _, units := range []uint64{0, 4_350_000} {
		m := New(nil, nil, big.NewInt(1), Config{Balance: BalanceConfig{ReferenceGasUnits: units}}, logr.Discard())
		if got := m.ReferenceGasUnits(); got != units {
			t.Fatalf("ReferenceGasUnits() = %d, want %d", got, units)
		}
	}
}
