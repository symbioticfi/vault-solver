package redstoneoev

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// TestExecutorV6DigestGoldenVector pins the digest computation to the verified live vector
// (docs/OEV-PLAN.md §6.7): the same inputs that produced a winning, signature-valid bid on Sepolia.
func TestExecutorV6DigestGoldenVector(t *testing.T) {
	chainID := big.NewInt(11155111)
	callback := common.HexToAddress("0x812492C36b003837C30cB0B63960b86eC9B27309")
	opDataHash := common.HexToHash("0x0a85a1be3cf06539edd05476a60cca5482e8ef0c4fa0bb6c1cf3f79fd0945509")
	bid := big.NewInt(100000000000000) // 0.0001 ETH
	nonce := big.NewInt(1)
	maxGas := big.NewInt(50000000000) // 50 gwei

	got, err := ExecutorV6Digest(chainID, callback, opDataHash, bid, nonce, maxGas)
	if err != nil {
		t.Fatal(err)
	}
	want := common.HexToHash("0x78f6eb68948cfeb1e16a81b050c111bf099628ff9dc51debb55f0b4fff2c7e5a")
	if got != want {
		t.Fatalf("digest = %s, want %s", got.Hex(), want.Hex())
	}
}

// TestSignBidRecoversToSigner round-trips signing + recovery with a throwaway key: the EIP-191
// wrapping + signature must recover to the signer, exactly as the Executor's
// ECDSA.recover(toEthSignedMessageHash(digest)) does on-chain.
func TestSignBidRecoversToSigner(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s := &testSigner{key: key, addr: crypto.PubkeyToAddress(key.PublicKey)}

	chainID := big.NewInt(11155111)
	callback := common.HexToAddress("0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1")
	opData := []byte{0x12, 0x34}
	bid := big.NewInt(300000000000000)
	nonce := big.NewInt(3)
	maxGas := big.NewInt(60000000000)

	sig, err := SignBid(s, chainID, callback, opData, bid, nonce, maxGas)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != 65 {
		t.Fatalf("sig length = %d, want 65", len(sig))
	}

	digest, _ := ExecutorV6Digest(chainID, callback, crypto.Keccak256Hash(opData), bid, nonce, maxGas)
	ethHash := ethSignedMessageHash(digest)

	// Recover: normalize v from {27,28} back to {0,1} for crypto.SigToPub.
	rs := make([]byte, 65)
	copy(rs, sig)
	if rs[64] >= 27 {
		rs[64] -= 27
	}
	pub, err := crypto.SigToPub(ethHash.Bytes(), rs)
	if err != nil {
		t.Fatal(err)
	}
	if got := crypto.PubkeyToAddress(*pub); got != s.addr {
		t.Fatalf("recovered %s, want %s", got.Hex(), s.addr.Hex())
	}
}
