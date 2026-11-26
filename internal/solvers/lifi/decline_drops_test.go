package lifi

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

func TestFillDeclineNamesSnapshotDrops(t *testing.T) {
	var logs []string
	ctx := observability.WithLogger(t.Context(), funcr.NewJSON(func(line string) { logs = append(logs, line) }, funcr.Options{Verbosity: 1}))
	s := &Solver{}
	prepared := &preparedFill{drops: []liquidlane.Drop{{
		Adapter: common.HexToAddress("0xa"), Reason: `getAmountOut reverted: "stale price"`,
	}}}
	declined := errors.Errorf("%w: no allocation", types.ErrFillDeclined)
	result := s.handleFillDecline(ctx, &submittedOrder{OrderID: "order-1"}, common.HexToHash("0x1"), prepared, declined)
	if result.outcome != orderProcessingStrategyDeclined {
		t.Fatalf("outcome = %v", result.outcome)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, `no allocation (1 dropped reads: adapter 0x000000000000000000000000000000000000000A: getAmountOut reverted: \"stale price\")`) {
		t.Fatalf("decline reason lacks the snapshot drops: %s", joined)
	}
}
