package filler

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

const (
	pollInterval = 3 * time.Second
	// OperationTimeout bounds each phase of planning and submitting a fill.
	OperationTimeout = 30 * time.Second
	// ConfirmationPatience is how long a consumed nonce without a confirmed receipt is treated as
	// an included fill awaiting its confirmations rather than as a failure worth an error log.
	ConfirmationPatience = 2 * time.Minute
)

var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler", "")

var (
	// ErrBusy reports that the signer lane already owns a fill.
	ErrBusy = errors.New("merkle fill lane is busy")
	// ErrLaneStopped reports that the transaction manager stopped before the fill settled.
	ErrLaneStopped = errors.New("merkle fill lane stopped before the fill settled")
	// ErrLaneStalled reports that the sender dropped the fill without canonical evidence. The
	// lane keeps the intent and its reservation: only a restart, after an operator has checked
	// the transaction, may plan against that capacity again.
	ErrLaneStalled = errors.New("merkle fill lane lost its transaction without canonical evidence")
)

// Intent is one validated settlement transaction and the capacity it spends.
type Intent struct {
	OrderID string
	// Data is the complete call to the settlement contract.
	Data []byte
	// Deadline is the wall-clock instant after which the lane stops submitting.
	Deadline time.Time
	Spend    connector.Spend
}

// Outcome is the canonical result of a finished fill.
type Outcome struct {
	OrderID  string
	Hash     common.Hash
	Block    uint64
	Reverted bool
}

// Engine owns one settlement contract's Merkle fill lane. At most one fill is in flight:
// an admitted intent owns the signer lane until its canonical receipt is returned.
type Engine struct {
	// Name labels the lane, its spans and its logs with the owning solver.
	Name     string
	Workflow *connector.Workflow
	Routes   *connector.Routes
	Spends   *connector.SpendLedger
	// Sender and LaneReady are nil for quote-only deployments.
	Sender keeper.Sender
	// LaneReady reports whether the transaction manager behind Sender accepts work.
	LaneReady func() bool
	// MinValidity is the lifetime a callback-funded fill must still have, before the order
	// expiry and the signed discount deadline, when it is handed to the lane.
	MinValidity time.Duration
	// Decimals reads token decimals to price connector inventory; hosts with a token metadata
	// cache point it there.
	Decimals Decimals

	intent atomic.Pointer[Intent]
	// stalled is set when the driver finds the sender dropped the intent's transaction.
	stalled atomic.Bool
}

// New builds the engine from host dependencies. Quote-only engines discover routes and
// price reservations but never sign.
func New(name string, cfg Config, deps solver.Deps) (*Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.Chain == nil || (!cfg.QuoteOnly && (deps.Signer == nil || deps.MerkleTxManager == nil)) {
		return nil, errors.Errorf("%s requires the host chain, signer and transaction manager", name)
	}
	workflow := &connector.Workflow{Chain: deps.Chain, ChainID: deps.Chain.ChainID(), Filler: cfg.Executor, MaxGas: deps.MaxGas}
	source, err := cfg.Curator.PermissionSource(deps.Chain)
	if err != nil {
		return nil, err
	}
	workflow.Source = source
	e := &Engine{
		Name: name, Workflow: workflow, Routes: connector.NewRoutes(workflow, cfg.RecipeExecutors), Spends: connector.NewSpendLedger(),
		Decimals: chain.NewDecimals(deps.Chain).Get,
	}
	if cfg.QuoteOnly {
		return e, nil
	}
	workflow.Signer = deps.Signer.Address()
	manager, err := deps.MerkleTxManager()
	if err != nil {
		return nil, err
	}
	sender := manager.Sender(name)
	sender.Deadline = e.deadline
	e.Sender, e.LaneReady = sender, manager.Ready
	return e, nil
}

// Sends reports whether the engine submits transactions.
func (e *Engine) Sends() bool { return e.Sender != nil }

// Ready reports whether the signer lane can accept work; quote-only engines are never ready.
func (e *Engine) Ready() bool { return e.LaneReady != nil && e.LaneReady() }

// Initialize publishes the verified payment routes of the configured recipe executors.
func (e *Engine) Initialize(ctx context.Context) error {
	if err := e.Routes.Initialize(ctx); err != nil {
		return errors.Errorf("initialize recipe executors: %w", err)
	}
	return nil
}

// ResolveRoute selects a unique compatible recipe using only the startup cache.
func (e *Engine) ResolveRoute(venue, rwa, asset common.Address) (executor, account common.Address, ok bool) {
	return e.Routes.Resolve(venue, rwa, asset)
}

// Reservations subtracts queued, pending and not-yet-reflected canonical spends from quote inventory.
func (e *Engine) Reservations(inventory []liquidlane.Inventory) liquidlane.CapacityReservations {
	return e.Spends.Reservations(inventory)
}

// Stalled reports a lane that still owns a fill its sender has dropped: no receipt can settle
// it any more, so the lane accepts no work until restart. Only the driver decides it: a lane between
// publishing an intent and handing it to the sender, or between a receipt and settling it,
// briefly looks the same.
func (e *Engine) Stalled() bool { return e.stalled.Load() }

// Pending returns the fill that owns the lane, or nil.
func (e *Engine) Pending() *Intent { return e.intent.Load() }

// The sender reads the deadline at admission, after Submit published the intent.
func (e *Engine) deadline() time.Time {
	if intent := e.intent.Load(); intent != nil {
		return intent.Deadline
	}
	return time.Time{}
}

// Submit reserves the intent's spend and hands it to the signer lane. The reservation is
// captured before submission because a timeout may follow a send; a submission error keeps
// ownership whenever the sender does.
func (e *Engine) Submit(ctx context.Context, intent *Intent) error {
	if err := e.reserve(intent); err != nil {
		return err
	}
	return e.send(ctx, intent)
}

func (e *Engine) reserve(intent *Intent) error {
	if !e.intent.CompareAndSwap(nil, intent) {
		return e.errBusy()
	}
	if err := e.Spends.Reserve(intent.OrderID, intent.Spend); err != nil {
		e.intent.CompareAndSwap(intent, nil)
		return err
	}
	return nil
}

// errBusy names the fill that owns the signer lane.
func (e *Engine) errBusy() error {
	if pending := e.Pending(); pending != nil {
		return errors.Errorf("%w: order %s owns the signer lane", ErrBusy, pending.OrderID)
	}
	return errors.Errorf("%w: another fill took the signer lane", ErrBusy)
}

func (e *Engine) send(ctx context.Context, intent *Intent) error {
	err := e.Sender.Submit(ctx, transaction.ExecutionRequest{To: e.Workflow.Filler, Data: intent.Data})
	if err != nil && !e.Sender.Busy() && e.intent.CompareAndSwap(intent, nil) {
		e.Spends.Release(intent.OrderID)
	}
	return err
}

// Poll returns the pending fill's canonical outcome, or nil while it is still in flight.
// Uncertain results keep the intent: only canonical evidence settles its spend.
func (e *Engine) Poll(ctx context.Context) (outcome *Outcome, err error) {
	intent := e.intent.Load()
	if intent == nil {
		return nil, nil
	}
	ctx, end := tracer.Start(ctx, e.Name+".fill", observability.AttrSolver.String(e.Name), observability.AttrOrderID.String(intent.OrderID))
	defer func() { end(err) }()
	result, err := e.Sender.Poll(ctx)
	if err != nil || result == nil {
		return nil, err
	}
	if result.Hash == (common.Hash{}) || result.Execution == nil || result.Execution.TxHash != result.Hash {
		return nil, errors.New("fill sender returned invalid execution identity")
	}
	if err := result.Execution.Validate(); err != nil {
		return nil, err
	}
	block := result.Execution.BlockNumber.Uint64()
	if result.Reverted {
		e.Spends.Release(intent.OrderID)
	} else {
		spend := intent.Spend
		spend.IncludedAt = block
		if err := e.Spends.Include(intent.OrderID, spend); err != nil {
			return nil, err
		}
	}
	observability.Log(ctx).Info("fill transaction finalized", "orderId", intent.OrderID, "tx", result.Hash.Hex(), "reverted", result.Reverted)
	e.intent.CompareAndSwap(intent, nil)
	return &Outcome{OrderID: intent.OrderID, Hash: result.Hash, Block: block, Reverted: result.Reverted}, nil
}

// Await polls until the pending fill settles. It returns nil when no fill is pending. It gives
// up, leaving the intent with the lane, when the transaction manager has stopped or the sender
// has dropped the transaction: neither can produce a receipt any more.
func (e *Engine) Await(ctx context.Context) (*Outcome, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var included time.Time
	for {
		intent := e.Pending()
		if intent == nil {
			return nil, nil
		}
		log := observability.Log(ctx).WithValues("orderId", intent.OrderID)
		outcome, err := e.Poll(ctx)
		if outcome != nil {
			return outcome, nil
		}
		if !e.Sender.Busy() {
			e.stalled.Store(true)
			if err == nil {
				err = ErrLaneStalled
			} else {
				err = errors.Errorf("%w: %w", ErrLaneStalled, err)
			}
			log.Error(err, "fill lane stalled; restart required")
			return nil, err
		}
		if err != nil && e.LaneReady != nil && !e.LaneReady() {
			return nil, errors.Errorf("%w: %w", ErrLaneStopped, err)
		}
		// While the sender still owns the transaction, a consumed nonce means it was included and
		// its receipt is not confirmed yet: the manager reports that on every observation.
		awaitingConfirmations := errors.Is(err, transaction.ErrNonceConsumed)
		if awaitingConfirmations && included.IsZero() {
			included = time.Now()
		}
		switch {
		case err == nil || ctx.Err() != nil:
		case awaitingConfirmations && time.Since(included) < ConfirmationPatience:
			log.V(1).Info("fill included; awaiting confirmations")
		default:
			log.Error(err, "fill poll failed")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Run polls awards while one worker owns submission and receipt tracking. An
// executable intent can accompany an error from the independent award refresh.
func (e *Engine) Run(ctx context.Context, next func(context.Context, bool) (*Intent, error)) error {
	if err := e.Initialize(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var fill <-chan bool
	defer func() {
		if fill != nil {
			<-fill
		}
	}()
	for {
		idle := fill == nil && e.Pending() == nil
		intent, err := func() (_ *Intent, err error) {
			run, end := tracer.Start(ctx, e.Name+".poll", observability.AttrSolver.String(e.Name))
			defer func() { end(err) }()
			return next(run, idle)
		}()
		if idle && intent != nil && ctx.Err() == nil {
			var startErr error
			fill, startErr = e.startFill(ctx, intent)
			err = errors.Join(err, startErr)
		}
		if err != nil && ctx.Err() == nil {
			observability.Log(ctx).Error(err, "Merkle award iteration failed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		case settled := <-fill:
			fill = nil
			if !settled {
				// Retry unsigned errors on the normal cadence.
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
				}
			}
		}
	}
}

func (e *Engine) startFill(ctx context.Context, intent *Intent) (<-chan bool, error) {
	if e.Sender.Busy() {
		return nil, errors.Errorf("%w: the fill sender still tracks a transaction", ErrBusy)
	}
	if err := e.reserve(intent); err != nil {
		return nil, err
	}
	done := make(chan bool, 1)
	go func() {
		log := observability.Log(ctx).WithValues("orderId", intent.OrderID)
		send, cancel := context.WithTimeout(ctx, OperationTimeout)
		err := e.send(send, intent)
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Error(err, "Merkle fill submission failed")
		}
		outcome, err := e.Await(ctx)
		if errors.Is(err, ErrLaneStopped) && ctx.Err() == nil {
			log.Error(err, "Merkle fill tracking stopped")
		}
		done <- outcome != nil
	}()
	return done, nil
}
