package uniswapx

import (
	"context"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
	// txmanager already appends the replayed reason of a mined revert; it passes through unchanged.
	mined := errors.New("tx 0x1 reverted on-chain (replay of block 7 reverted with NotReactor())")
	for _, tc := range []struct {
		name   string
		result txmanager.Result
		want   string
	}{
		{
			name:   "mined revert keeps the txmanager reason",
			result: txmanager.Result{Outcome: txmanager.OutcomeReverted, Err: mined},
			want:   mined.Error(),
		},
		{
			name: "gas estimate revert",
			result: txmanager.Result{Outcome: txmanager.OutcomeSubmissionError, Err: errors.Errorf(
				"estimate gas: %w", &txmanager.ExecutionRevertError{Data: revertPayload("NotReactor()"), Cause: errors.New("execution reverted")})},
			want: "estimate gas: execution reverted (revert NotReactor())",
		},
		{
			name:   "no error",
			result: txmanager.Result{Outcome: txmanager.OutcomeAbandoned},
			want:   `unknown transaction outcome "abandoned"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := explainFillFailure(tc.result)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("explainFillFailure() = %v, want %q", err, tc.want)
			}
			if tc.result.Err != nil && !errors.Is(err, tc.result.Err) {
				t.Fatalf("explainFillFailure() = %v does not wrap the result error", err)
			}
		})
	}
}
