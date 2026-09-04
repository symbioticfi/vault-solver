package signer

import (
	"os"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/config"
)

// FromConfig resolves the configured secret reference at the signing boundary.
func FromConfig(cfg config.SignerConfig) (Signer, error) {
	if cfg.KeyEnv != "" {
		secret := os.Getenv(cfg.KeyEnv)
		if secret == "" {
			return nil, errors.Errorf("signer: env %q is empty", cfg.KeyEnv)
		}
		return NewFromHexKey(secret)
	}
	if cfg.KeystorePath != "" {
		passphrase := os.Getenv(cfg.PassphraseEnv)
		if passphrase == "" {
			return nil, errors.Errorf("signer: passphrase env %q is empty", cfg.PassphraseEnv)
		}
		return NewFromKeystore(cfg.KeystorePath, passphrase)
	}
	return nil, errors.New("signer: no key source configured")
}
