package redstoneoev

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/oev/callback"
)

type revertError struct{ data []byte }

func (revertError) Error() string        { return "execution reverted" }
func (e revertError) RevertData() []byte { return e.data }

type fakeReplayer struct {
	tx      *ethtypes.Transaction
	receipt *ethtypes.Receipt
	rcptErr error
	callErr error
	fetches int
	calls   int
	msg     ethereum.CallMsg
	block   *big.Int
}

func (f *fakeReplayer) TransactionByHash(context.Context, common.Hash) (*ethtypes.Transaction, bool, error) {
	f.fetches++
	return f.tx, false, nil
}

func (f *fakeReplayer) TransactionReceipt(context.Context, common.Hash) (*ethtypes.Receipt, error) {
	return f.receipt, f.rcptErr
}

func (f *fakeReplayer) ReplayMinedCall(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	f.calls++
	f.msg, f.block = msg, block
	return nil, f.callErr
}

func TestSettlementRevertNamesRevert(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	chainID := big.NewInt(1)
	signer := ethtypes.LatestSignerForChainID(chainID)
	to := common.Address{9}
	tx, err := ethtypes.SignNewTx(key, signer, &ethtypes.DynamicFeeTx{
		ChainID: chainID, Nonce: 1, Gas: 500_000, To: &to, Data: []byte{1, 2, 3, 4},
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	failed := &ethtypes.Receipt{Status: ethtypes.ReceiptStatusFailed, BlockNumber: big.NewInt(100)}
	notExecutor := callback.SymbioticOevSolverNotExecutorErrorID().Bytes()[:4]

	tests := []struct {
		name    string
		receipt *ethtypes.Receipt
		callErr error
		want    string
		wantErr string
	}{
		{"custom error", failed, revertError{notExecutor}, "NotExecutor()", ""},
		{"error string", failed, revertError{errorStringRevert(t, "Executor: bad nonce")}, `"Executor: bad nonce"`, ""},
		{"revert without data", failed, revertError{}, "reverted: execution reverted", ""},
		{"replay does not revert", failed, nil, "did not revert", ""},
		{"tx succeeded", &ethtypes.Receipt{Status: ethtypes.ReceiptStatusSuccessful, BlockNumber: big.NewInt(100)}, nil, "succeeded on-chain", ""},
		{"transport error", failed, errors.New("dial tcp: refused"), "revert reason unavailable: replay of block 100 failed: dial tcp: refused", ""},
		{"no block number", &ethtypes.Receipt{Status: ethtypes.ReceiptStatusFailed}, nil, "unknown inclusion block", ""},
		{"receipt unavailable", nil, nil, "", "fetch settlement receipt: not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeReplayer{tx: tx, receipt: tc.receipt, callErr: tc.callErr}
			if tc.receipt == nil {
				f.rcptErr = errors.New("not found")
			}
			got, err := settlementRevert(t.Context(), f, signer, tx.Hash())
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || !strings.Contains(got, tc.want) {
				t.Fatalf("reason = %q, err = %v; want %q", got, err, tc.want)
			}
			if f.calls == 1 && (f.block.Int64() != 100 || f.msg.From != crypto.PubkeyToAddress(key.PublicKey) ||
				*f.msg.To != to || f.msg.Gas != tx.Gas() || !bytes.Equal(f.msg.Data, tx.Data())) {
				t.Fatalf("replayed %+v at block %v, want the tx at mined block 100", f.msg, f.block)
			}
		})
	}
}

func TestFailedLiquidationOfOursIsReplayed(t *testing.T) {
	s, _ := seededSolver(t)
	f := &fakeReplayer{receipt: &ethtypes.Receipt{Status: ethtypes.ReceiptStatusSuccessful, BlockNumber: big.NewInt(1)}}
	s.replayer = f
	hash := common.Hash{7}.Hex()
	frame := func(id string, success bool, txHash string) []byte {
		return marshal(LiquidationResult{Op: "liquidation-result", ID: id, Data: LiquidationResultData{
			Success: success, Liquidator: seedCallback.Hex(), TxHash: txHash,
		}})
	}
	s.handleMessage(t.Context(), frame("ok", true, hash))
	s.handleMessage(t.Context(), frame("bad-hash", false, "0xnope"))
	s.handleMessage(t.Context(), frame("failed", false, hash))
	s.handleMessage(t.Context(), frame("failed", false, hash)) // duplicate frame is not replayed again
	s.replays.Wait()
	if f.fetches != 1 || f.calls != 0 {
		t.Fatalf("fetches = %d, calls = %d; want one replay that a successful receipt short-circuits", f.fetches, f.calls)
	}
	if !s.replayBusy.CompareAndSwap(false, false) {
		t.Fatal("replay slot not released")
	}
}
