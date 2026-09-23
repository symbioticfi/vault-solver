package config

import (
	"strings"
	"testing"
)

func TestLoadRPCAttemptTimeout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		field   string
		want    int
		wantErr bool
	}{
		{name: "omitted", want: 20000},
		{name: "zero", field: "0", want: 20000},
		{name: "shorter", field: "5000", want: 5000},
		{name: "longer", field: "60000", want: 60000},
		{name: "negative", field: "-1", wantErr: true},
		{name: "duration overflow", field: "9223372036855", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := validConfig
			if tc.field != "" {
				body = strings.Replace(body, "chain:\n", "chain:\n  rpcAttemptTimeoutMs: "+tc.field+"\n", 1)
			}
			cfg, err := Load(writeTemp(t, body))
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "chain.rpcAttemptTimeoutMs") {
					t.Fatalf("Load error = %v, want rpcAttemptTimeoutMs validation failure", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Chain.RPCAttemptTimeoutMs != tc.want {
				t.Fatalf("timeout = %d, want %d", cfg.Chain.RPCAttemptTimeoutMs, tc.want)
			}
		})
	}
}
