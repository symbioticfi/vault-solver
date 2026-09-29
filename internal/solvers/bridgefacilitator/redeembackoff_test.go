package bridgefacilitator

import (
	"context"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/codes"

	"github.com/symbioticfi/vault-solver/api/bindings/3f/adapter"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestRedeemBackoffSchedule(t *testing.T) {
	var b redeemBackoff
	// Refusals of batches of 10, 5, 2, 1, 1: the batch halves down to 1, and the passes skipped after
	// each refusal follow 1, 2, 4, 12 redeem polls (5, 10, 20, 60 minutes at the default poll).
	for i, want := range []struct {
		sent, limit, skip int
		first             bool
	}{
		{sent: 10, limit: 5, skip: 0, first: true},
		{sent: 5, limit: 2, skip: 1},
		{sent: 2, limit: 1, skip: 3},
		{sent: 1, limit: 1, skip: 11},
		{sent: 1, limit: 1, skip: 11},
	} {
		if first := b.refused(want.sent); first != want.first {
			t.Fatalf("refusal %d opened the episode = %v, want %v", i+1, first, want.first)
		}
		if b.limit(10) != want.limit || b.skipPasses != want.skip || !b.holding {
			t.Fatalf("after refusal %d: limit %d, skip %d, holding %v; want %d, %d, true",
				i+1, b.limit(10), b.skipPasses, b.holding, want.limit, want.skip)
		}
	}

	// Skipped passes count down; the pass after them sends.
	for range 11 {
		if b.beginPass() {
			t.Fatal("a pass sent during the backoff")
		}
	}
	if !b.beginPass() || b.holding {
		t.Fatal("the pass after the backoff did not send")
	}

	// Successes double the batch back to the configured size and end the episode.
	for _, want := range []int{2, 4, 8, 10} {
		b.succeeded(10)
		if b.limit(10) != want || b.skipPasses != 0 || b.refusals != 0 {
			t.Fatalf("after a success: limit %d, skip %d, refusals %d; want %d, 0, 0",
				b.limit(10), b.skipPasses, b.refusals, want)
		}
	}
	if b.active() {
		t.Fatal("episode still active at the configured batch size")
	}
	if !b.refused(10) {
		t.Fatal("a refusal after recovery did not open a new episode")
	}
}

func TestRedeemBackoffResetsWhenTheLaneIsFundedAgain(t *testing.T) {
	fundable := false
	s := &Solver{cfg: &Config{RedeemBatchSize: 10}, fundable: func() bool { return fundable }}
	s.redeem.fundable = s.laneFundable()
	s.redeem.refused(10)
	s.redeem.refused(5)

	// Signals from other lane edges (every admission, the nonce lane) do not reset the episode.
	s.onLaneStateChange(t.Context())
	if !s.redeem.active() {
		t.Fatal("a signal without the gate reopening reset the backoff")
	}
	fundable = true
	s.onLaneStateChange(t.Context())
	if s.redeem.active() || s.redeem.skipPasses != 0 || s.redeem.limit(10) != 10 {
		t.Fatalf("backoff = %+v after the gate reopened, want reset", s.redeem)
	}
	s.redeem.refused(10)
	s.onLaneStateChange(t.Context())
	if !s.redeem.active() {
		t.Fatal("a signal while the gate stayed open reset the backoff")
	}
}

// TestRedeemHalvesTheBatchAndBacksOffWhenUnaffordable drives redeem passes against a balance the guard
// refuses: each refused batch halves the next, the passes in between send nothing, the refusal is an
// Info line once per episode (never an Error) and a declined span, and the gate reopening restores the
// configured batch at the next pass.
func TestRedeemHalvesTheBatchAndBacksOffWhenUnaffordable(t *testing.T) {
	rec := tracetest.Install(t)
	log, lines := tracetest.CaptureLogs(t, 0)
	ctx := observability.WithLogger(t.Context(), log)
	adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000a0")
	ready := make([]common.Address, 10)
	for i := range ready {
		ready[i] = common.BigToAddress(common.Big1)
	}
	fundable := false
	var sent []int
	s := &Solver{
		cfg: &Config{RedeemBatchSize: 10}, log: log, links: observability.NewSpanLinks(),
		fundable: func() bool { return fundable },
	}
	s.txManager = transactionSenderFunc(func(_ context.Context, req txmanager.Request) txmanager.Result {
		sent = append(sent, finalizeCount(t, req.Data))
		return txmanager.Result{
			Outcome:     txmanager.OutcomeSubmissionError,
			Err:         errors.Errorf("send %q: %w", "redeem", txmanager.ErrUnaffordable),
			NotAdmitted: true,
		}
	})
	pass := func() {
		s.redeem.beginPass()
		s.redeemReady(ctx, Target{Adapter: adapterAddr}, ready)
	}

	for range 8 {
		pass()
	}
	// Passes 1, 2, 4 and 8 send batches of 10, 5, 2 and 1; passes 3 and 5-7 wait.
	if want := []int{10, 5, 2, 1}; !equalInts(sent, want) {
		t.Fatalf("batches sent = %v, want %v", sent, want)
	}

	var info, debugOrError int
	for _, line := range lines() {
		if !strings.Contains(line, `"msg":"redeem: `) {
			continue
		}
		switch {
		case strings.Contains(line, "cannot fund the batch") && strings.Contains(line, `"level":0`):
			info++
		case !strings.Contains(line, `"level":`):
			debugOrError++
		}
	}
	if info != 1 || debugOrError != 0 {
		t.Fatalf("episode logged %d Info and %d Error lines, want 1/0: %v", info, debugOrError, lines())
	}
	for _, span := range tracetest.AllEnded(rec, "3f.redeem.submit") {
		if span.Status().Code == codes.Error {
			t.Fatalf("refused redeem submit span ended with error %q", span.Status().Description)
		}
		if reason, count := tracetest.EventAttr(span, "declined", "reason"); count != 1 || reason != "unaffordable" {
			t.Fatalf("submit declined events = %d with reason %q, want 1 with unaffordable", count, reason)
		}
	}

	fundable = true
	s.onLaneStateChange(ctx)
	pass()
	if got := sent[len(sent)-1]; len(sent) != 5 || got != 10 {
		t.Fatalf("batches sent = %v, want a full batch of 10 right after the gate reopened", sent)
	}
}

func TestRedeemTransientRefusalDoesNotBackOff(t *testing.T) {
	adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000a0")
	sends := 0
	s := &Solver{cfg: &Config{RedeemBatchSize: 10}, links: observability.NewSpanLinks()}
	s.txManager = transactionSenderFunc(func(context.Context, txmanager.Request) txmanager.Result {
		sends++
		return txmanager.Result{
			Outcome: txmanager.OutcomeSubmissionError, Err: txmanager.ErrStaleHead, NotAdmitted: true,
		}
	})
	for range 3 {
		s.redeem.beginPass()
		s.redeemReady(t.Context(), Target{Adapter: adapterAddr}, []common.Address{adapterAddr})
	}
	if sends != 3 || s.redeem.active() {
		t.Fatalf("sends = %d with backoff %+v, want a send every pass and no backoff", sends, s.redeem)
	}
}

func TestRedeemAdmittedFailureStillLogsError(t *testing.T) {
	log, lines := tracetest.CaptureLogs(t, 0)
	adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000a0")
	s := &Solver{cfg: &Config{RedeemBatchSize: 10}, links: observability.NewSpanLinks()}
	s.txManager = transactionSenderFunc(func(context.Context, txmanager.Request) txmanager.Result {
		return txmanager.Result{Outcome: txmanager.OutcomeReverted, Err: errors.New("reverted")}
	})
	s.redeemReady(observability.WithLogger(t.Context(), log), Target{Adapter: adapterAddr}, []common.Address{adapterAddr})
	errorLines := 0
	for _, line := range lines() {
		if strings.Contains(line, `"msg":"redeem: tx not included"`) && !strings.Contains(line, `"level":`) {
			errorLines++
		}
	}
	if errorLines != 1 || s.redeem.active() {
		t.Fatalf("error lines = %d with backoff %+v, want 1 and none", errorLines, s.redeem)
	}
}

// finalizeCount decodes how many finalizeRequest calls a redeem multicall batches.
func finalizeCount(t *testing.T, data []byte) int {
	t.Helper()
	parsed, err := adapter.ThreeFAdapterMetaData.ParseABI()
	if err != nil {
		t.Fatal(err)
	}
	args, err := parsed.Methods["multicall"].Inputs.Unpack(data[4:])
	if err != nil || len(args) != 1 {
		t.Fatalf("decode redeem multicall: %v", err)
	}
	calls, ok := args[0].([][]byte)
	if !ok {
		t.Fatalf("multicall argument has type %T", args[0])
	}
	return len(calls)
}

func equalInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
