package signer

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"os"
	"strings"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/internal/config"
)

// local is an in-process Signer backed by an ECDSA private key held in memory. It is immutable
// after construction and therefore safe for concurrent use.
type local struct {
	key  *ecdsa.PrivateKey
	addr common.Address
}

// FromConfig builds a Signer from the configured key source, reading secrets from the environment.
func FromConfig(cfg config.SignerConfig) (Signer, error) {
	if (cfg.KeyEnv == "") == (cfg.KeystorePath == "") {
		return nil, errors.New("signer: exactly one key source is required")
	}
	env := cfg.KeyEnv
	if cfg.KeystorePath != "" {
		env = cfg.PassphraseEnv
	}
	secret := os.Getenv(env)
	if secret == "" {
		return nil, errors.Errorf("signer: env %q is empty", env)
	}
	if cfg.KeystorePath != "" {
		return NewFromKeystore(cfg.KeystorePath, secret)
	}
	return NewFromHexKey(secret)
}

// NewFromHexKey builds a Signer from a hex-encoded private key (with or without the 0x prefix).
func NewFromHexKey(hexKey string) (Signer, error) {
	key, err := crypto.HexToECDSA(strings.TrimPrefix(strings.TrimSpace(hexKey), "0x"))
	if err != nil {
		// Deliberately do not wrap the underlying error — it can echo key material.
		return nil, errors.New("signer: invalid hex private key")
	}
	return newLocal(key), nil
}

// NewFromKeystore builds a Signer by decrypting a go-ethereum keystore file with passphrase.
func NewFromKeystore(path, passphrase string) (Signer, error) {
	jsonBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Errorf("signer: read keystore: %w", err)
	}
	key, err := keystore.DecryptKey(jsonBytes, passphrase)
	if err != nil {
		return nil, errors.New("signer: failed to decrypt keystore (wrong passphrase or corrupt file)")
	}
	return newLocal(key.PrivateKey), nil
}

func newLocal(key *ecdsa.PrivateKey) *local {
	return &local{key: key, addr: crypto.PubkeyToAddress(key.PublicKey)}
}

func (l *local) Address() common.Address { return l.addr }

func (l *local) SignHash(hash common.Hash) ([]byte, error) {
	sig, err := crypto.Sign(hash.Bytes(), l.key)
	if err != nil {
		return nil, errors.Errorf("signer: sign hash: %w", err)
	}
	// crypto.Sign returns V in {0,1}; normalize to {27,28} for ecrecover / EIP-1271 verifiers.
	sig[64] += 27
	return sig, nil
}

func (l *local) SignTx(
	ctx context.Context,
	tx *types.Transaction,
	chainID *big.Int,
) (*types.Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if tx == nil || chainID == nil || chainID.Sign() <= 0 {
		return nil, errors.New("signer: transaction and positive chain id are required")
	}
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), l.key)
	if err != nil {
		return nil, errors.Errorf("signer: sign tx: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return signed, nil
}
