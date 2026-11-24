package iris

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

var (
	errLaneBusy    = errors.New("iris signing lane is busy")
	errLaneStalled = errors.New("iris signing lane lost its transaction without canonical evidence; restart required")
)

// settled is the canonical outcome of one execution, or why the lane stopped waiting.
type settled struct {
	result *transaction.ExecutionResult
	err    error
}

// lane owns the SDK sender for one execution at a time. Handlers that stop waiting leave
// the execution with the lane: a background goroutine polls until the sender settles it,
// and the lane admits nothing new until then. A stalled lane admits nothing until restart.
type lane struct {
	sender keeper.Sender
	ready  func() bool
	poll   time.Duration

	busy    atomic.Bool
	stalled atomic.Bool
	// until is the wall clock, in unix nanoseconds, after which the sender stops submitting.
	until atomic.Int64
}

func newLane(sender *transaction.Sender, ready func() bool, poll time.Duration) *lane {
	l := &lane{sender: sender, ready: ready, poll: poll}
	sender.Deadline = func() time.Time { return time.Unix(0, l.until.Load()) }
	return l
}

// execute submits the call and returns a channel that receives its outcome. background is
// the solver's run context: polling outlives the request that started it.
func (l *lane) execute(ctx, background context.Context, call recipes.ExecutorCall, until time.Time) (<-chan settled, error) {
	if l.stalled.Load() {
		return nil, errLaneStalled
	}
	if !l.busy.CompareAndSwap(false, true) {
		return nil, errLaneBusy
	}
	l.until.Store(until.UnixNano())
	if err := l.sender.Submit(ctx, transaction.ExecutionRequest{To: call.Executor, Data: call.Data}); err != nil && !l.sender.Busy() {
		l.busy.Store(false)
		return nil, err
	}
	out := make(chan settled, 1)
	go func() { out <- l.await(background) }()
	return out, nil
}

func (l *lane) await(ctx context.Context) settled {
	ticker := time.NewTicker(l.poll)
	defer ticker.Stop()
	for {
		result, err := l.sender.Poll(ctx)
		if result != nil {
			l.busy.Store(false)
			return settled{result: result}
		}
		if !l.sender.Busy() {
			l.stalled.Store(true)
			observability.Log(ctx).Error(err, "iris signing lane stalled; restart required")
			return settled{err: errLaneStalled}
		}
		if err != nil && l.ready != nil && !l.ready() {
			return settled{err: errors.Errorf("transaction manager stopped: %w", err)}
		}
		if err != nil && !errors.Is(err, transaction.ErrNonceConsumed) && ctx.Err() == nil {
			observability.Log(ctx).Error(err, "iris signing poll failed")
		}
		select {
		case <-ctx.Done():
			return settled{err: ctx.Err()}
		case <-ticker.C:
		}
	}
}
