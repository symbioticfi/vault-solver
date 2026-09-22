package config

import (
	"strings"
	"testing"
)

func TestLoadDelegation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block string
		valid bool
	}{
		{"env keys", "delegateAddress: 0x1111111111111111111111111111111111111111\n    auxiliarySigners: [{keyEnv: AUX_KEY}]", true},
		{"keystore", "delegateAddress: 0x1111111111111111111111111111111111111111\n    auxiliarySigners: [{keystorePath: aux.json, passphraseEnv: AUX_PASSWORD}]", true},
		{"missing address", "auxiliarySigners: [{keyEnv: AUX_KEY}]", false},
		{"invalid address", "delegateAddress: invalid\n    auxiliarySigners: [{keyEnv: AUX_KEY}]", false},
		{"zero address", "delegateAddress: 0x0000000000000000000000000000000000000000\n    auxiliarySigners: [{keyEnv: AUX_KEY}]", false},
		{"no auxiliaries", "delegateAddress: 0x1111111111111111111111111111111111111111\n    auxiliarySigners: []", false},
		{"too many auxiliaries", "delegateAddress: 0x1111111111111111111111111111111111111111\n    auxiliarySigners: [{keyEnv: A}, {keyEnv: B}, {keyEnv: C}, {keyEnv: D}, {keyEnv: E}, {keyEnv: F}]", false},
		{"missing key", "delegateAddress: 0x1111111111111111111111111111111111111111\n    auxiliarySigners: [{}]", false},
		{"ambiguous key", "delegateAddress: 0x1111111111111111111111111111111111111111\n    auxiliarySigners: [{keyEnv: AUX_KEY, keystorePath: aux.json}]", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(validConfig, "  maxFeeGwei: 100", "  maxFeeGwei: 100\n  delegation:\n    "+tc.block, 1)
			_, err := Load(writeTemp(t, body))
			if (err == nil) != tc.valid {
				t.Fatalf("Load error = %v, valid = %v", err, tc.valid)
			}
		})
	}
}
