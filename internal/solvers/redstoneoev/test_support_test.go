package redstoneoev

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

const capturedAuction = `{"op":"auction","id":"6382e936-c915-496a-bb3e-fa3b4ccc3a8d","timestamp":1781243340988,"timeoutMs":500,"payload":{"positions":[{"market_unique_key":"0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5","borrower_address":"0x629d764ec8563afa701709b52c1a215e865632de","current_ltv":108.83,"oracle_address":"0xfED5bC312C7139743bc3ab21Ef92f5AeB353339D","lltv":"860000000000000000","collateral_decimals":18,"loan_decimals":6,"collateral_address":"0x17e892d4E802B01d7DA49Ca3542560f6851AA4D3","loan_address":"0x468BB3245BF520a0CD030BDE029c98aCEAF84C9d","collateral_assets":"1000000000000000000","borrow_assets":"1685600048","borrow_shares":"1685600000000000"}],"prices":{"0xfED5bC312C7139743bc3ab21Ef92f5AeB353339D":"1800943620100000000000000000"}}}`

func mustBig(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("bad big integer: " + s)
	}
	return n
}

func goldenMarket() morpho.MarketState {
	return morpho.MarketState{
		TotalSupplyAssets: big.NewInt(100000000068), TotalSupplyShares: mustBig("100000000000000000"),
		TotalBorrowAssets: big.NewInt(4730000068), TotalBorrowShares: mustBig("4729999932892591"),
		LastUpdate: 1780059204, Fee: new(big.Int), Lltv: mustBig("860000000000000000"),
		BorrowRatePerSec: big.NewInt(182418302),
	}
}

func goldenBorrower() morpho.PositionState {
	return morpho.PositionState{BorrowShares: mustBig("1685600000000000"), Collateral: mustBig("1000000000000000000")}
}

type testSigner struct {
	key  *ecdsa.PrivateKey
	addr common.Address
}

func (s *testSigner) Address() common.Address { return s.addr }

func (s *testSigner) SignHash(hash common.Hash) ([]byte, error) {
	sig, err := crypto.Sign(hash.Bytes(), s.key)
	if err == nil && sig[64] < 27 {
		sig[64] += 27
	}
	return sig, err
}

func (s *testSigner) SignTx(_ context.Context, tx *gethtypes.Transaction, chainID *big.Int) (*gethtypes.Transaction, error) {
	return gethtypes.SignTx(tx, gethtypes.LatestSignerForChainID(chainID), s.key)
}

func recoverSolveSigner(t *testing.T, solver *Solver, data SolveData) common.Address {
	t.Helper()
	operationData, err := hexutil.Decode(data.OperationData)
	if err != nil {
		t.Fatal(err)
	}
	bid, err := parse.EthToWei(data.Bid, "bid")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := ExecutorV6Digest(solver.chainID, common.HexToAddress(data.OperationCallback), crypto.Keccak256Hash(operationData), bid, mustBig(data.Nonce), mustBig(data.MaxTxGasPrice))
	if err != nil {
		t.Fatal(err)
	}
	signature, err := hexutil.Decode(data.LiquidationSig)
	if err != nil || len(signature) != crypto.SignatureLength {
		t.Fatalf("signature: %x, %v", signature, err)
	}
	if signature[64] >= 27 {
		signature[64] -= 27
	}
	publicKey, err := crypto.SigToPub(ethSignedMessageHash(digest).Bytes(), signature)
	if err != nil {
		t.Fatal(err)
	}
	return crypto.PubkeyToAddress(*publicKey)
}
