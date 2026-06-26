package redstoneoev

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/internal/parse"
)

// testSigner is a minimal signer.Signer backed by an in-memory key, for tests. SignHash returns the
// 65-byte [R||S||V] form with V in {27,28}, matching the production signer contract.
type testSigner struct {
	key  *ecdsa.PrivateKey
	addr common.Address
}

func (s *testSigner) Address() common.Address { return s.addr }

func (s *testSigner) SignHash(hash common.Hash) ([]byte, error) {
	sig, err := crypto.Sign(hash.Bytes(), s.key)
	if err != nil {
		return nil, err
	}
	if sig[64] < 27 {
		sig[64] += 27
	}
	return sig, nil
}

func (s *testSigner) SignTx(tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	return types.SignTx(tx, types.LatestSignerForChainID(chainID), s.key)
}

// recoverSolveSigner recovers the EXECUTOR_V6 signer from a solve's signature over its operationData/bid/
// nonce — the full on-the-wire verification the Executor performs. Shared by the buildBid / lifecycle / WS
// tests, which all assert the recovered address equals the bot's signer.
func recoverSolveSigner(t *testing.T, s *Solver, d SolveData) common.Address {
	t.Helper()
	opData, err := hexutil.Decode(d.OperationData)
	if err != nil {
		t.Fatalf("decode operationData: %v", err)
	}
	bid, err := parse.EthToWei(d.Bid, "bid")
	if err != nil {
		t.Fatalf("parse bid: %v", err)
	}
	digest, err := ExecutorV6Digest(s.chainID, s.cfg.Callback, crypto.Keccak256Hash(opData), bid, mustBig(d.Nonce), mustBig(d.MaxTxGasPrice))
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	sig, err := hexutil.Decode(d.LiquidationSig)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if len(sig) != 65 {
		t.Fatalf("sig len = %d, want 65", len(sig))
	}
	if sig[64] >= 27 {
		sig[64] -= 27 // SigToPub wants V in {0,1}
	}
	pub, err := crypto.SigToPub(ethSignedMessageHash(digest).Bytes(), sig)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	return crypto.PubkeyToAddress(*pub)
}
