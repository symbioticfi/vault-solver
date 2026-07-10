package signer

import (
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/internal/config"
)

const localTestKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

var localTestAddress = common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")

func TestFromConfig_HexEnvironmentAndHashRecovery(t *testing.T) {
	t.Setenv("TEST_SIGNER_KEY", "  0x"+localTestKey+"  ")
	s, err := FromConfig(config.SignerConfig{KeyEnv: "TEST_SIGNER_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Address() != localTestAddress {
		t.Fatalf("address = %s, want %s", s.Address(), localTestAddress)
	}
	assertHashSigner(t, s)
}

func assertHashSigner(t *testing.T, s Signer) {
	t.Helper()
	digest := crypto.Keccak256Hash([]byte("vault-solver signer characterization"))
	sig, err := s.SignHash(digest)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != 65 || (sig[64] != 27 && sig[64] != 28) {
		t.Fatalf("signature shape = len %d V %d", len(sig), sig[64])
	}

	recovery := append([]byte(nil), sig...)
	recovery[64] -= 27
	pub, err := crypto.SigToPub(digest.Bytes(), recovery)
	if err != nil {
		t.Fatal(err)
	}
	if got := crypto.PubkeyToAddress(*pub); got != localTestAddress {
		t.Fatalf("recovered = %s, want %s", got, localTestAddress)
	}
}

func TestLocalSignTx_BindsEIP155SenderAndChain(t *testing.T) {
	s, err := NewFromHexKey(localTestKey)
	if err != nil {
		t.Fatal(err)
	}
	assertTransactionSigner(t, s)
}

func assertTransactionSigner(t *testing.T, s Signer) {
	t.Helper()
	chainID := big.NewInt(11_155_111)
	tx := types.NewTransaction(
		7,
		common.HexToAddress("0x1234"),
		big.NewInt(5),
		21_000,
		big.NewInt(1e9),
		[]byte{1, 2},
	)
	signed, err := s.SignTx(tx, chainID)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
	if err != nil {
		t.Fatal(err)
	}
	if sender != localTestAddress || signed.ChainId().Cmp(chainID) != 0 {
		t.Fatalf(
			"sender/chain = %s/%s, want %s/%s",
			sender,
			signed.ChainId(),
			localTestAddress,
			chainID,
		)
	}
}

func TestFromConfig_EncryptedKeystore(t *testing.T) {
	key, err := crypto.HexToECDSA(localTestKey)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := keystore.EncryptKey(
		&keystore.Key{Address: localTestAddress, PrivateKey: key},
		"correct horse battery staple",
		keystore.LightScryptN,
		keystore.LightScryptP,
	)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "key.json")
	if err := os.WriteFile(path, encrypted, 0o600); err != nil {
		t.Fatal(err)
	}

	passphraseEnv := t.Name()
	t.Setenv(passphraseEnv, "correct horse battery staple")
	s, err := FromConfig(config.SignerConfig{
		KeystorePath:  path,
		PassphraseEnv: passphraseEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Address() != localTestAddress {
		t.Fatalf("address = %s, want %s", s.Address(), localTestAddress)
	}
	assertHashSigner(t, s)
	assertTransactionSigner(t, s)

	t.Setenv(passphraseEnv, "SENSITIVE-WRONG-PASSPHRASE")
	_, err = FromConfig(config.SignerConfig{
		KeystorePath:  path,
		PassphraseEnv: passphraseEnv,
	})
	if err == nil || strings.Contains(err.Error(), "SENSITIVE-WRONG-PASSPHRASE") {
		t.Fatalf("wrong-passphrase error leaked secret: %v", err)
	}
}

func TestNewFromHexKey_DoesNotEchoMalformedSecret(t *testing.T) {
	const secret = "SENSITIVE-not-a-private-key"
	_, err := NewFromHexKey(secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("malformed-key error leaked secret: %v", err)
	}
}

func TestLocalSigner_ConcurrentUse(t *testing.T) {
	s, err := NewFromHexKey(localTestKey)
	if err != nil {
		t.Fatal(err)
	}

	const workers, iterations = 32, 100
	errs := make(chan error, workers*iterations*2)
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range iterations {
				digest := crypto.Keccak256Hash(
					[]byte(strconv.Itoa(worker)),
					[]byte(strconv.Itoa(i)),
				)
				if _, err := s.SignHash(digest); err != nil {
					errs <- err
				}
				tx := types.NewTransaction(
					uint64(worker*iterations+i),
					localTestAddress,
					big.NewInt(0),
					21_000,
					big.NewInt(1),
					nil,
				)
				if _, err := s.SignTx(tx, big.NewInt(1)); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
