package uniswapx

import (
	"context"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// revertRPCError is how geth's client surfaces an eth_call or eth_estimateGas revert.
type revertRPCError struct{ data []byte }

func (e revertRPCError) Error() string  { return "execution reverted" }
func (e revertRPCError) ErrorCode() int { return 3 }
func (e revertRPCError) ErrorData() any { return hexutil.Encode(e.data) }

func revertPayload(signature string, args ...byte) []byte {
	return append(crypto.Keccak256([]byte(signature))[:4], args...)
}

func TestFillPreflightRevertNamesContractError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"custom error", revertRPCError{revertPayload("NotCaller()")}, "execution reverted (revert NotCaller())"},
		{"no revert data", errors.New("execution reverted"), "execution reverted (node returned no revert data)"},
		{"transport", errors.New("RPC unavailable"), "fill preflight failed: RPC unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newDirectExecutionFixture(t)
			fixture.solver.chain = contractCallerFunc(func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
				return nil, tc.err
			})
			_, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route}, fixture.order, fixture.now, fixture.now)
			if !errors.Is(err, errFillPreflight) || !strings.HasSuffix(err.Error(), tc.want) {
				t.Fatalf("preflight error = %v, want suffix %q", err, tc.want)
			}
		})
	}
}

func TestExplainFillFailure(t *testing.T) {
	executor := common.HexToAddress("0xe")
	call := ethereum.CallMsg{From: common.HexToAddress("0xa"), To: &executor, Data: []byte{1}}
	mined := errors.New("tx 0x1 reverted on-chain")
	reverted := txmanager.Result{
		Outcome: txmanager.OutcomeReverted, Err: mined, Receipt: &types.Receipt{BlockNumber: big.NewInt(7)},
	}
	balance := revertPayload("InsufficientBalance(uint256,uint256)",
		append(common.LeftPadBytes([]byte{5}, 32), common.LeftPadBytes([]byte{9}, 32)...)...)
	for _, tc := range []struct {
		name      string
		call      ethereum.CallMsg
		result    txmanager.Result
		replayErr error
		want      string
	}{
		{
			name: "mined revert replayed", call: call, result: reverted, replayErr: revertRPCError{balance},
			want: "tx 0x1 reverted on-chain (replay of block 7 reverted with InsufficientBalance(balance=5, needed=9))",
		},
		{
			name: "replay succeeds", call: call, result: reverted,
			want: "tx 0x1 reverted on-chain (replay of block 7 did not revert (an earlier transaction in the block changed the state))",
		},
		{
			name: "replay unavailable", call: call, result: reverted, replayErr: errors.New("RPC unavailable"),
			want: "tx 0x1 reverted on-chain (revert reason unavailable: replay of block 7 failed: RPC unavailable)",
		},
		{
			name: "sender not ours", result: reverted,
			want: "tx 0x1 reverted on-chain",
		},
		{
			name: "gas estimate revert", call: call,
			result: txmanager.Result{Outcome: txmanager.OutcomeSubmissionError, Err: errors.Errorf(
				"estimate gas: %w", &txmanager.ExecutionRevertError{Data: revertPayload("NotReactor()"), Cause: errors.New("execution reverted")})},
			want: "estimate gas: execution reverted (revert NotReactor())",
		},
		{
			name: "no error", call: call, result: txmanager.Result{Outcome: txmanager.OutcomeAbandoned},
			want: `unknown transaction outcome "abandoned"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var replays []*big.Int
			s := &Solver{chain: contractCallerFunc(func(_ context.Context, got ethereum.CallMsg, block *big.Int) ([]byte, error) {
				if got.To == nil || *got.To != executor {
					t.Fatalf("replay call = %+v, want the fill call", got)
				}
				replays = append(replays, block)
				return nil, tc.replayErr
			})}
			err := s.explainFillFailure(t.Context(), &pendingUniswapFill{call: tc.call}, tc.result)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("explainFillFailure() = %v, want %q", err, tc.want)
			}
			if tc.result.Err != nil && !errors.Is(err, tc.result.Err) {
				t.Fatalf("explainFillFailure() = %v does not wrap the result error", err)
			}
			wantReplay := tc.call.To != nil && tc.result.Outcome == txmanager.OutcomeReverted
			if wantReplay != (len(replays) == 1) || wantReplay && replays[0].Int64() != 7 {
				t.Fatalf("replays = %v, want replay %v", replays, wantReplay)
			}
		})
	}
}
