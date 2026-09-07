package signer

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/symbioticfi/vault-solver/internal/config"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

func TestSignerRejectsAmbiguousSourceAndInvalidTransaction(t *testing.T) {
	if _, err := FromConfig(config.SignerConfig{KeyEnv: "KEY", KeystorePath: "key.json"}); err == nil {
		t.Fatal("ambiguous source accepted")
	}
	s, err := NewFromHexKey("0000000000000000000000000000000000000000000000000000000000000001")
	testcheck.NoError(t, err)
	for _, test := range []struct {
		name string
		tx   *types.Transaction
		id   *big.Int
	}{
		{"nil transaction", nil, big.NewInt(1)},
		{"nil chain", types.NewTx(&types.LegacyTx{}), nil},
		{"zero chain", types.NewTx(&types.LegacyTx{}), new(big.Int)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := s.SignTx(t.Context(), test.tx, test.id); err == nil {
				t.Fatal("invalid signing request accepted")
			}
		})
	}
}
