package config

import (
	"strings"
	"testing"
)

func TestLateReceiptObservationConfig(t *testing.T) {
	for _, tc := range []struct {
		name        string
		yaml        string
		wantTimeout int
		wantHashes  int
		wantErr     bool
	}{
		{name: "defaults", wantTimeout: 600_000, wantHashes: 1024},
		{name: "override", yaml: "  lateReceiptTimeoutMs: 15000\n  lateReceiptMaxHashes: 7", wantTimeout: 15_000, wantHashes: 7},
		{name: "zero selects defaults", yaml: "  lateReceiptTimeoutMs: 0\n  lateReceiptMaxHashes: 0", wantTimeout: 600_000, wantHashes: 1024},
		{name: "small bounds", yaml: "  lateReceiptTimeoutMs: 1\n  lateReceiptMaxHashes: 1", wantTimeout: 1, wantHashes: 1},
		{name: "negative timeout", yaml: "  lateReceiptTimeoutMs: -1", wantErr: true},
		{name: "negative hash limit", yaml: "  lateReceiptMaxHashes: -1", wantErr: true},
		{name: "timeout overflow", yaml: "  lateReceiptTimeoutMs: 9223372036855", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(validConfig, "  maxFeeGwei: 100", "  maxFeeGwei: 100\n"+tc.yaml, 1)
			cfg, err := Load(writeTemp(t, body))
			if tc.wantErr {
				if err == nil {
					t.Fatal("invalid receipt observation bound was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.TxManager.LateReceiptTimeoutMs != tc.wantTimeout || cfg.TxManager.LateReceiptMaxHashes != tc.wantHashes {
				t.Fatalf("observation timeout=%d hashes=%d, want %d/%d", cfg.TxManager.LateReceiptTimeoutMs, cfg.TxManager.LateReceiptMaxHashes, tc.wantTimeout, tc.wantHashes)
			}
		})
	}
}
