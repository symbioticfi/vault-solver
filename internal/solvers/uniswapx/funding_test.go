package uniswapx

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestQuoteDeclinesWhileLaneUnfundable(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	strategy := &quoteTestStrategy{quote: &strategytypes.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}}
	solver := newQuoteTestSolver(t, tokenIn, strategy)
	txm := &executionTestTxManager{unfundable: true}
	solver.txm = txm

	response, err := solver.quote(t.Context(), validQuoteRequest(tokenIn, tokenOut))
	if err != nil || response.AmountOut != "0" || response.declineReason != quoteDeclineLaneUnfundable ||
		len(strategy.inputs) != 0 {
		t.Fatalf("unfundable-lane quote = %+v, inputs = %d, err %v", response, len(strategy.inputs), err)
	}
	if !solver.quoteBlocked(time.Now().Unix()) {
		t.Fatal("quoteBlocked = false while the lane is unfundable")
	}

	txm.unfundable = false
	response, err = solver.quote(t.Context(), validQuoteRequest(tokenIn, tokenOut))
	if err != nil || response.AmountOut != "90" || len(strategy.inputs) != 1 {
		t.Fatalf("funded-lane quote = %+v, inputs = %d, err %v", response, len(strategy.inputs), err)
	}
}

// TestFillLoopAttemptsQueuedOrderWhileLaneUnfundable pins that the funding gate, which closes at the pricing
// horizon, does not stop a won order from being attempted: the balance guard still sends it down to its own
// floor (fee-gas strategy §2.10 keeps that margin for won quotes), and a refusal backs off exponentially
// (TestUnaffordableFillIsNotRetriedHot) instead of re-polling hot. Deferring it would fade the order.
func TestFillLoopAttemptsQueuedOrderWhileLaneUnfundable(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	fixture.txm.unfundable = true
	if !fixture.solver.claim(fixture.order.Hash, fixture.now) {
		t.Fatal("order was not claimed")
	}
	orders := make(chan *resolvedOrder, 1)
	orders <- fixture.order
	close(orders)
	// The guard refuses it: the fill loop must then back the order off rather than retry it next poll.
	fixture.txm.result <- txmanager.Result{
		Outcome:     txmanager.OutcomeSubmissionError,
		Err:         errors.Errorf("send %q: %w", "uniswapx-fill", txmanager.ErrUnaffordable),
		NotAdmitted: true,
	}
	before := time.Now()

	if err := fixture.solver.fillLoop(t.Context(), []liquidlane.Route{fixture.route}, orders); err != nil {
		t.Fatalf("fill loop: %v", err)
	}
	reader := fixture.solver.reader.(*executionTestReader)
	if reader.latestBlockReads == 0 || len(fixture.txm.reqs) != 1 {
		t.Fatalf("unfundable lane did not attempt the won order: chainReads=%d requests=%d",
			reader.latestBlockReads, len(fixture.txm.reqs))
	}
	retryAt, scheduled := fixture.solver.retryAt[fixture.order.Hash]
	if poll := fixture.solver.cfg.OrderServer.PollInterval; !scheduled || retryAt.Sub(before) < poll {
		t.Fatalf("refused order retry at %v (scheduled %t), want a backoff of at least %s", retryAt.Sub(before), scheduled, poll)
	}
	if len(fixture.solver.failureTimes) != 0 {
		t.Fatal("an unaffordable refusal counted toward the fade breaker")
	}
}

// TestUnaffordableFillIsNotRetriedHot completes fills the balance guard refused. Each refusal backs the
// order off exponentially instead of re-planning it every poll, and none counts toward the fade breaker
// or logs an error; a transient stale-head refusal keeps the plain poll-interval retry.
func TestUnaffordableFillIsNotRetriedHot(t *testing.T) {
	log, lines := tracetest.CaptureLogs(t, 1)
	fixture := newDirectExecutionFixture(t)
	fixture.order.Source = orderSourcePublicV2
	fixture.solver.cfg.Breaker = BreakerConfig{MaxFailures: 1, Window: time.Minute}
	fixture.solver.log = log
	poll := fixture.solver.cfg.OrderServer.PollInterval
	refused := txmanager.Result{
		Outcome:     txmanager.OutcomeSubmissionError,
		Err:         errors.Errorf("send %q: %w", "uniswapx-fill", txmanager.ErrUnaffordable),
		NotAdmitted: true,
	}

	var backoffs []time.Duration
	for range 3 {
		fixture.solver.inFlight[fixture.order.Hash] = true
		before := time.Now()
		fixture.solver.completePendingFill(t.Context(), testPendingFill(t, fixture.order), refused)
		backoffs = append(backoffs, fixture.solver.retryAt[fixture.order.Hash].Sub(before))
	}
	for i, want := range []time.Duration{poll, 2 * poll, 4 * poll} {
		if backoffs[i] < want || backoffs[i] > want+time.Second {
			t.Fatalf("backoff after refusal %d = %s, want about %s (all %v)", i+1, backoffs[i], want, backoffs)
		}
	}
	if len(fixture.solver.failureTimes) != 0 || fixture.solver.localBlockUntil.Load() != 0 {
		t.Fatal("unaffordable refusals counted toward the fade breaker")
	}
	for _, line := range lines() {
		if strings.Contains(line, `"msg":"order fill`) && !strings.Contains(line, `"level":`) {
			t.Fatalf("refusal logged at error: %s", line)
		}
	}

	staleOrder := *fixture.order
	staleOrder.Hash = common.HexToHash("0x5")
	stale := &staleOrder
	fixture.solver.inFlight[stale.Hash] = true
	before := time.Now()
	fixture.solver.completePendingFill(t.Context(), testPendingFill(t, stale), txmanager.Result{
		Outcome: txmanager.OutcomeSubmissionError, Err: txmanager.ErrStaleHead, NotAdmitted: true,
	})
	if got := fixture.solver.retryAt[stale.Hash].Sub(before); got < poll || got > poll+time.Second {
		t.Fatalf("stale-head retry = %s, want the %s poll interval", got, poll)
	}
	if fixture.solver.attempts[stale.Hash] != 0 {
		t.Fatal("a stale-head refusal counted as a failed attempt")
	}
}

// TestFillRequestObsoleteFollowsOrderStatus drives the Obsolete check the fill request carries: the order
// API reporting the order filled (by a competitor), cancelled or expired makes the manager cancel the
// pending fill before its CancelAt, while any other status or a failed read keeps it alive.
func TestFillRequestObsoleteFollowsOrderStatus(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	poller := &stateTestOrderPoller{}
	fixture.solver.orders = poller
	if _, err := fixture.solver.startFill(
		t.Context(), []liquidlane.Route{fixture.route}, fixture.order, fixture.now, fixture.now,
	); err != nil {
		t.Fatalf("startFill: %v", err)
	}
	if len(fixture.txm.reqs) != 1 || fixture.txm.reqs[0].Obsolete == nil {
		t.Fatal("fill request carries no Obsolete check")
	}
	check := fixture.txm.reqs[0].Obsolete
	hash := fixture.order.Hash

	for _, tc := range []struct {
		name         string
		terminals    map[common.Hash]orderTerminal
		err          error
		wantObsolete bool
		wantErr      bool
	}{
		{name: "open", terminals: map[common.Hash]orderTerminal{hash: {Status: orderStatusOpen}}},
		{
			name:         "filled by a competitor",
			terminals:    map[common.Hash]orderTerminal{hash: {Status: orderStatusFilled, TxHash: common.HexToHash("0xc0")}},
			wantObsolete: true,
		},
		{name: "cancelled", terminals: map[common.Hash]orderTerminal{hash: {Status: orderStatusCancelled}}, wantObsolete: true},
		{name: "expired", terminals: map[common.Hash]orderTerminal{hash: {Status: orderStatusExpired}}, wantObsolete: true},
		{name: "error keeps the fill", terminals: map[common.Hash]orderTerminal{hash: {Status: orderStatusError}}},
		{
			name:      "insufficient funds keeps the fill",
			terminals: map[common.Hash]orderTerminal{hash: {Status: orderStatusInsufficientFunds}},
		},
		{name: "unknown status", terminals: map[common.Hash]orderTerminal{hash: {Status: "renamed"}}, wantErr: true},
		{name: "order missing", terminals: map[common.Hash]orderTerminal{}, wantErr: true},
		{name: "order API unavailable", err: errors.New("rate limited"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			poller.terminals, poller.err = tc.terminals, tc.err
			obsolete, err := check(t.Context())
			if obsolete != tc.wantObsolete || (err != nil) != tc.wantErr {
				t.Fatalf("Obsolete = (%v, %v), want (%v, error %v)", obsolete, err, tc.wantObsolete, tc.wantErr)
			}
		})
	}
}
