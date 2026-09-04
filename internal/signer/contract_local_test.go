package signer

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/config"
)

const testPrivateKey = "0000000000000000000000000000000000000000000000000000000000000001"

func TestLocalSignerContracts(t *testing.T) {
	signer, err := NewFromHexKey("  0x" + testPrivateKey + "  ")
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	wantAddress := common.HexToAddress("0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf")
	if signer.Address() != wantAddress {
		t.Fatalf("address = %s, want %s", signer.Address(), wantAddress)
	}

	hash := crypto.Keccak256Hash([]byte("vault-solver"))
	signature, err := signer.SignHash(hash)
	if err != nil {
		t.Fatalf("sign hash: %v", err)
	}
	if len(signature) != crypto.SignatureLength || signature[64] < 27 {
		t.Fatalf("signature = %x", signature)
	}
	recoverySignature := append([]byte(nil), signature...)
	recoverySignature[64] -= 27
	publicKey, err := crypto.SigToPub(hash.Bytes(), recoverySignature)
	if err != nil || crypto.PubkeyToAddress(*publicKey) != wantAddress {
		t.Fatalf("recover signer = %v, err %v", publicKey, err)
	}

	chainID := big.NewInt(1)
	transaction := types.NewTx(&types.DynamicFeeTx{ChainID: chainID, Nonce: 7, Gas: 21_000, GasFeeCap: big.NewInt(2), GasTipCap: big.NewInt(1), To: new(common.Address), Value: big.NewInt(3)})
	signed, err := signer.SignTx(t.Context(), transaction, chainID)
	if err != nil {
		t.Fatalf("sign transaction: %v", err)
	}
	sender, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
	if err != nil || sender != wantAddress {
		t.Fatalf("transaction sender = %s, err %v", sender, err)
	}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := signer.SignTx(canceled, transaction, chainID); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled sign error = %v", err)
	}
}

func TestSignerLoadingFailsClosed(t *testing.T) {
	invalid := "definitely-invalid"
	if _, err := NewFromHexKey(invalid); err == nil || strings.Contains(err.Error(), invalid) {
		t.Fatalf("unsafe invalid-key error = %v", err)
	}
	if _, err := FromConfig(config.SignerConfig{}); err == nil {
		t.Fatal("missing key source accepted")
	}
	t.Setenv("EMPTY_SIGNER_KEY", "")
	if _, err := FromConfig(config.SignerConfig{KeyEnv: "EMPTY_SIGNER_KEY"}); err == nil {
		t.Fatal("empty key environment accepted")
	}
	t.Setenv("TEST_SIGNER_KEY", testPrivateKey)
	loaded, err := FromConfig(config.SignerConfig{KeyEnv: "TEST_SIGNER_KEY"})
	if err != nil || loaded.Address() == (common.Address{}) {
		t.Fatalf("load from environment = %v, %v", loaded, err)
	}
}
