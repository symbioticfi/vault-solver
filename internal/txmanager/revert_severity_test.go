package txmanager

import (
	"context"
	"io"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

// The production backend must take the replay path.
var _ chain.MinedCallReplayer = (*chain.Client)(nil)

func TestMinedRevertLeavesAlertSeverityToIntegration(t *testing.T) {
	logs, logger := newLogCapture(0)
	b := &metricsReceiptBackend{mockBackend: newMockBackend(), reverted: true}
	m := newStreakManager(t, b, logger)
	startManagerForTest(t, m)
	result := m.Send(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000, Label: "fill"})
	if result.Outcome != OutcomeReverted || result.Err == nil {
		t.Fatalf("revert outcome or cause lost: %+v", result)
	}
	if failures, info := countLogs(*logs, "transaction reverted"); failures != 0 || info != 1 {
		t.Fatalf("manager paged before integration reconciliation: errors=%d info=%d", failures, info)
	}
}

type replayBackend struct {
	*metricsReceiptBackend

	err   error
	call  ethereum.CallMsg
	block *big.Int
}

func (b *replayBackend) ReplayMinedCall(_ context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	b.call, b.block = call, block
	return nil, b.err
}

// "boom" as Error(string).
const boomRevert = "0x08c379a0" +
	"0000000000000000000000000000000000000000000000000000000000000020" +
	"0000000000000000000000000000000000000000000000000000000000000004" +
	"626f6f6d00000000000000000000000000000000000000000000000000000000"

func TestMinedRevertReplaysCallForReason(t *testing.T) {
	parsed, err := abi.JSON(strings.NewReader(`[{"type":"error","name":"Nope","inputs":[]}]`))
	if err != nil {
		t.Fatal(err)
	}
	nope := hexutil.Encode(crypto.Keccak256([]byte("Nope()"))[:4])
	for _, tc := range []struct {
		name    string
		err     error
		decoder *chain.RevertDecoder
		want    string
	}{
		{"error string without decoder", &estimateRPCError{3, "execution reverted", boomRevert}, nil, `reverted with "boom")`},
		{"custom error with decoder", &estimateRPCError{3, "execution reverted", nope}, chain.NewRevertDecoder(&parsed), "reverted with Nope())"},
		{"custom error without decoder", &estimateRPCError{3, "execution reverted", nope}, nil, "reverted with unknown error " + nope + " (4 bytes))"},
		{"no data", &estimateRPCError{3, "execution reverted", nil}, nil, "reverted: execution reverted)"},
		{"not reproduced", nil, nil, "did not revert (state changed earlier in the block, or the tx ran out of gas))"},
		{"replay failed", io.ErrUnexpectedEOF, nil, "failed: unexpected EOF)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, logger := newLogCapture(0)
			b := &replayBackend{metricsReceiptBackend: &metricsReceiptBackend{mockBackend: newMockBackend(), reverted: true}, err: tc.err}
			m := newStreakManager(t, b, logger)
			startManagerForTest(t, m)
			to := common.Address{1}
			result := m.Send(t.Context(), Request{To: to, Data: []byte{0xab}, GasLimit: 21_000, Label: "fill", RevertDecoder: tc.decoder})
			if result.Outcome != OutcomeReverted || result.Err == nil ||
				!strings.Contains(result.Err.Error(), "replay of block ") || !strings.HasSuffix(result.Err.Error(), tc.want) {
				t.Fatalf("mined revert reason = %v, want suffix %q", result.Err, tc.want)
			}
			if b.call.From != m.signer.Address() || *b.call.To != to || b.call.Gas != 21_000 ||
				hexutil.Encode(b.call.Data) != "0xab" || b.block.Cmp(result.Receipt.BlockNumber) != 0 {
				t.Fatalf("replay did not repeat the mined call at its block: %+v at %v", b.call, b.block)
			}
		})
	}
}

func TestMinedRevertWithoutReplayerSaysNoReplay(t *testing.T) {
	_, logger := newLogCapture(0)
	m := newStreakManager(t, &metricsReceiptBackend{mockBackend: newMockBackend(), reverted: true}, logger)
	startManagerForTest(t, m)
	result := m.Send(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000, Label: "fill"})
	if result.Err == nil || !strings.Contains(result.Err.Error(), "(revert reason unavailable: no replay)") {
		t.Fatalf("missing replay not reported: %v", result.Err)
	}
}
