package config

import (
	"strings"
	"testing"
)

func TestLoadReconcileNonces(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  bool
		bad   bool
	}{
		{name: "default"},
		{name: "enabled", value: "true", want: true},
		{name: "disabled", value: "false"},
		{name: "invalid value", value: "invalid", bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := validConfig
			if tc.value != "" {
				body = strings.Replace(body, "txManager:\n", "txManager:\n  reconcileNonces: "+tc.value+"\n", 1)
			}
			cfg, err := Load(writeTemp(t, body))
			if (err != nil) != tc.bad {
				t.Fatalf("Load error = %v, want error %t", err, tc.bad)
			}
			if err == nil && cfg.TxManager.ReconcileNonces != tc.want {
				t.Fatalf("reconcileNonces = %t, want %t", cfg.TxManager.ReconcileNonces, tc.want)
			}
		})
	}
}
