package main

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// Construction never performs RPC; startup performs the on-chain validation.
type constructionBackend struct{ txmanager.Backend }

func (constructionBackend) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	panic("unexpected RPC during construction")
}

func TestTransactionSenderConfiguration(t *testing.T) {
	primary, err := signer.NewFromHexKey("ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, key string
		delegated bool
		capacity  int
		errorText string
	}{
		{name: "legacy", capacity: 1},
		{name: "pool", delegated: true, key: "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d", capacity: 2},
		{name: "duplicate primary", delegated: true, key: "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", errorText: "unique"},
		{name: "missing secret", delegated: true, errorText: "auxiliary signer 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_AUXILIARY_KEY", tc.key)
			cfg := config.TxManagerConfig{MaxFeeGwei: 50}
			if tc.delegated {
				cfg.Delegation = &config.DelegationConfig{
					DelegateAddress:  "0x0000000000000000000000000000000000000123",
					AuxiliarySigners: []config.SignerConfig{{KeyEnv: "TEST_AUXILIARY_KEY"}},
				}
			}
			registry := prometheus.NewRegistry()
			sender, err := newTransactionSender(constructionBackend{}, primary, big.NewInt(1), cfg, registry, logr.Discard())
			if tc.errorText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errorText) {
					t.Fatalf("error = %v, want %q", err, tc.errorText)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if sender.Capacity() != tc.capacity {
				t.Fatalf("capacity = %d", sender.Capacity())
			}
			// Emit a bounded admission metric without starting any RPC worker.
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			sender.Send(ctx, txmanager.Request{Label: "test"})
			families, err := registry.Gather()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, family := range families {
				if family.GetName() != "solver_bot_txmanager_admission_rejections_total" {
					continue
				}
				found = true
				for _, metric := range family.GetMetric() {
					address := ""
					for _, label := range metric.GetLabel() {
						if label.GetName() == "sender" {
							address = label.GetValue()
						}
					}
					if tc.delegated && address != strings.ToLower(primary.Address().Hex()) {
						t.Fatalf("sender label = %q", address)
					}
					if !tc.delegated && address != "" {
						t.Fatal("legacy metric acquired a new label")
					}
				}
			}
			if !found {
				t.Fatal("missing admission metric")
			}
		})
	}
}
