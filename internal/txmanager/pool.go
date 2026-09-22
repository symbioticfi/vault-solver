package txmanager

import (
	"context"
	"math/big"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

// Sender is the shared transaction service. Each sending account still owns at
// most one unresolved signed lifecycle; Capacity reports the number of accounts.
// Integrations accept smaller interfaces for the operations they actually use.
type Sender interface {
	Send(ctx context.Context, req Request) Result
	TrySend(ctx context.Context, req Request) (Result, bool)
	SendAsync(ctx context.Context, req Request) (<-chan Result, bool)
	Available() bool
	Idle() bool
	LaneReady() bool
	Capacity() int
	SubscribeLaneState() (<-chan struct{}, func())
	MaxFeePerGas(context.Context) (*big.Int, error)
	Confirmations() uint64
	ValidateFeeHeadroom() error
	Initialize(context.Context) error
	Start(context.Context)
}

// Pool chooses an idle account in priority order (primary first). Managers own
// their nonce locks, admission slots, and signed lifecycles independently. A call
// is assigned once: errors or ambiguous broadcasts never move it to another
// account. The mutex protects subscriptions only; admission is atomic in Manager.
type Pool struct {
	lanes       []*Manager
	waiters     atomic.Int64
	stopping    chan struct{}
	mu          sync.Mutex
	subscribers map[uint64]chan struct{}
	nextID      uint64
}

// NewPool takes exclusive ownership of the managers before any worker starts.
func NewPool(lanes ...*Manager) (*Pool, error) {
	if len(lanes) == 0 {
		return nil, errors.New("txmanager: pool requires at least one sender")
	}
	seen := make(map[common.Address]bool, len(lanes))
	for _, lane := range lanes {
		if lane == nil || lane.signer == nil {
			return nil, errors.New("txmanager: missing pool sender")
		}
		address := lane.signer.Address()
		if seen[address] {
			return nil, errors.Errorf("txmanager: duplicate pool sender %s", address)
		}
		seen[address] = true
	}
	p := &Pool{lanes: slices.Clone(lanes), stopping: make(chan struct{}), subscribers: make(map[uint64]chan struct{})}
	for _, lane := range p.lanes {
		lane.onLaneStateChange = p.notify
	}
	return p, nil
}

func (p *Pool) Initialize(ctx context.Context) error {
	for _, lane := range p.lanes {
		if err := lane.Initialize(ctx); err != nil {
			return errors.Errorf("initialize sender %s: %w", lane.signer.Address(), err)
		}
	}
	return nil
}

func (p *Pool) ValidateFeeHeadroom() error {
	for _, lane := range p.lanes {
		if err := lane.ValidateFeeHeadroom(); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pool) Start(ctx context.Context) {
	var workers sync.WaitGroup
	for _, lane := range p.lanes {
		workers.Go(func() { lane.Start(ctx) })
	}
	<-ctx.Done()
	close(p.stopping)
	p.notify()
	// Each account drains concurrently under its own bounded shutdown policy.
	workers.Wait()
}

func (p *Pool) Capacity() int         { return len(p.lanes) }
func (p *Pool) Confirmations() uint64 { return p.lanes[0].Confirmations() }
func (p *Pool) MaxFeePerGas(ctx context.Context) (*big.Int, error) {
	return p.lanes[0].MaxFeePerGas(ctx)
}

func (p *Pool) Available() bool {
	select {
	case <-p.stopping:
		return false
	default:
	}
	return slices.ContainsFunc(p.lanes, func(m *Manager) bool { return m.Available() })
}

func (p *Pool) Idle() bool {
	return p.waiters.Load() == 0 && !slices.ContainsFunc(p.lanes, func(m *Manager) bool { return !m.Idle() })
}

func (p *Pool) LaneReady() bool {
	select {
	case <-p.stopping:
		return false
	default:
	}
	return p.waiters.Load() == 0 && slices.ContainsFunc(p.lanes, func(m *Manager) bool { return m.LaneReady() })
}

func (p *Pool) Send(ctx context.Context, req Request) Result {
	result, accepted := p.sendAsync(ctx, req, false)
	if !accepted {
		return notAdmittedResult(ctx.Err())
	}
	return <-result
}

func (p *Pool) TrySend(ctx context.Context, req Request) (Result, bool) {
	result, accepted := p.sendAsync(ctx, req, true)
	if !accepted {
		return Result{}, false
	}
	return <-result, true
}

func (p *Pool) SendAsync(ctx context.Context, req Request) (<-chan Result, bool) {
	return p.sendAsync(ctx, req, false)
}

func (p *Pool) sendAsync(ctx context.Context, req Request, try bool) (<-chan Result, bool) {
	started := time.Now()
	admissionCtx := ctx
	cancel := func() {}
	if !req.CancelAt.IsZero() {
		admissionCtx, cancel = context.WithDeadline(ctx, req.CancelAt)
	}
	defer cancel()
	changes, unsubscribe := p.SubscribeLaneState()
	defer unsubscribe()
	p.waiters.Add(1)
	p.notify()
	defer func() { p.waiters.Add(-1); p.notify() }()
	fail := func(err error) (<-chan Result, bool) { return p.lanes[0].admissionFailure(ctx, req, started, err) }
	for {
		if err := admissionCtx.Err(); err != nil {
			return fail(err)
		}
		select {
		case <-p.stopping:
			return fail(errManagerStopped)
		default:
		}
		for _, lane := range p.lanes {
			if !lane.LaneReady() {
				continue
			}
			result, accepted := lane.sendAsyncAt(ctx, req, true, started)
			if accepted || ctx.Err() != nil {
				return result, accepted
			}
		}
		if try {
			return nil, false
		}
		select {
		case <-changes:
		case <-admissionCtx.Done():
			return fail(admissionCtx.Err())
		case <-p.stopping:
			return fail(errManagerStopped)
		}
	}
}

func (p *Pool) SubscribeLaneState() (<-chan struct{}, func()) {
	p.mu.Lock()
	id := p.nextID
	p.nextID++
	changes := make(chan struct{}, 1)
	p.subscribers[id] = changes
	p.mu.Unlock()
	var once sync.Once
	return changes, func() { once.Do(func() { p.mu.Lock(); delete(p.subscribers, id); p.mu.Unlock() }) }
}

func (p *Pool) notify() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, changes := range p.subscribers {
		select {
		case changes <- struct{}{}:
		default:
		}
	}
}
