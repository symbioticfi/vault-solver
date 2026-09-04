package signer

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
)

type local struct {
	key     *ecdsa.PrivateKey
	address common.Address
}

func NewFromHexKey(encoded string) (Signer, error) {
	key, err := crypto.HexToECDSA(strings.TrimPrefix(strings.TrimSpace(encoded), "0x"))
	if err != nil {
		// Never wrap an implementation error that may contain fragments of the supplied secret.
		return nil, errors.New("signer: invalid hex private key")
	}
	return newLocal(key), nil
}

func NewFromKeystore(path, passphrase string) (Signer, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Errorf("signer: read keystore: %w", err)
	}
	key, err := keystore.DecryptKey(contents, passphrase)
	if err != nil {
		return nil, errors.New("signer: failed to decrypt keystore (wrong passphrase or corrupt file)")
	}
	return newLocal(key.PrivateKey), nil
}

func newLocal(key *ecdsa.PrivateKey) *local {
	return &local{key: key, address: crypto.PubkeyToAddress(key.PublicKey)}
}

func (s *local) Address() common.Address {
	return s.address
}

func (s *local) SignHash(hash common.Hash) ([]byte, error) {
	signature, err := crypto.Sign(hash[:], s.key)
	if err != nil {
		return nil, errors.Errorf("signer: sign hash: %w", err)
	}
	signature[64] += 27
	return signature, nil
}

func (s *local) SignTx(
	ctx context.Context,
	transaction *types.Transaction,
	chainID *big.Int,
) (*types.Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	signed, err := types.SignTx(transaction, types.LatestSignerForChainID(chainID), s.key)
	if err != nil {
		return nil, errors.Errorf("signer: sign tx: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return signed, nil
}
