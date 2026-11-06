package rfq

import (
	"bytes"
	"context"
	"maps"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/state"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

const accountingKey = "rfq-merkle.accounting.2"

type fillSpend struct {
	CapacityID liquidlane.CapacityID `json:"capacityId"`
	IncludedAt uint64                `json:"includedAt"`
	Deadline   int64                 `json:"deadline,omitempty"` // Original signed chain time, never the local submission deadline.
}

type accountingState struct {
	Watermarks map[liquidlane.CapacityID]uint64 `json:"watermarks"`
	Completed  map[string]int64                 `json:"completed"` // Order ID -> original signed chain deadline.
}

// Quotes and execution share confirmed accounting under mu. Only canonical
// inclusion advances watermarks, persisted before acknowledgement. Completed
// orders stay protected until their signed chain deadline.
type spendLedger struct {
	mu    sync.Mutex
	store *state.Store
	state accountingState
}

func loadSpends(store *state.Store) (*spendLedger, error) {
	l := &spendLedger{store: store, state: accountingState{Watermarks: make(map[liquidlane.CapacityID]uint64), Completed: make(map[string]int64)}}
	_, err := store.Load(accountingKey, &l.state)
	if err != nil {
		return nil, err
	}
	if l.state.Watermarks == nil || l.state.Completed == nil {
		return nil, errors.New("missing RFQ accounting maps")
	}
	for capacity, block := range l.state.Watermarks {
		if capacity == "" || block == 0 {
			return nil, errors.New("invalid RFQ accounting watermark")
		}
	}
	for id, deadline := range l.state.Completed {
		if id == "" || deadline <= 0 {
			return nil, errors.New("invalid RFQ completed order")
		}
	}
	return l, nil
}

func (l *spendLedger) finalized(id string) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, found := l.state.Completed[id]
	return found
}

func (l *spendLedger) include(id string, spend fillSpend) error {
	if l == nil {
		return nil
	}
	if id == "" || spend.CapacityID == "" || spend.IncludedAt == 0 || spend.Deadline <= 0 {
		return errors.New("RFQ fill has no valid spend or inclusion block")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	next := accountingState{Watermarks: maps.Clone(l.state.Watermarks), Completed: maps.Clone(l.state.Completed)}
	next.Watermarks[spend.CapacityID] = max(next.Watermarks[spend.CapacityID], spend.IncludedAt)
	next.Completed[id] = spend.Deadline
	if err := l.store.Save(accountingKey, next); err != nil {
		return err
	}
	l.state = next
	return nil
}

// tick reconciles any SDK pending journal before calling expire. Thus only
// acknowledged work can expire; inclusion watermarks remain.
func (l *spendLedger) expire(chainTime int64) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var completed map[string]int64
	for id, deadline := range l.state.Completed {
		if deadline <= chainTime {
			if completed == nil {
				completed = maps.Clone(l.state.Completed)
			}
			delete(completed, id)
		}
	}
	if completed == nil {
		return nil
	}
	next := accountingState{Watermarks: l.state.Watermarks, Completed: completed}
	if err := l.store.Save(accountingKey, next); err != nil {
		return err
	}
	l.state = next
	return nil
}

// AcceptsSnapshots rejects inventory observed before the last confirmed fill on
// its capacity. Quotes check it right before publishing.
func (s *Solver) AcceptsSnapshots(inventory []liquidlane.Inventory) bool {
	if s.spends == nil {
		return true
	}
	s.spends.mu.Lock()
	defer s.spends.mu.Unlock()
	for _, item := range inventory {
		if item.BlockNumber < s.spends.state.Watermarks[liquidlane.RouteCapacityID(item.Route)] {
			return false
		}
	}
	return true
}

// legacySpend recovers spend metadata for intents saved before it existed, from
// the signed fill and the connector's immutable account/outputAsset at latest.
func (s *Solver) legacySpend(ctx context.Context) (fillSpend, error) {
	method := executorBinding.GetABI().Methods["fill"]
	if len(s.intent.Data) < 4 || !bytes.Equal(s.intent.Data[:4], method.ID) {
		return fillSpend{}, errors.New("invalid saved RFQ fill selector")
	}
	values, err := method.Inputs.Unpack(s.intent.Data[4:])
	if err != nil {
		return fillSpend{}, err
	}
	order := *abi.ConvertType(values[0], new(executor.IReactorOrder)).(*executor.IReactorOrder)
	funding := *abi.ConvertType(values[3], new([]executor.IReactorDiscountSwapInput)).(*[]executor.IReactorDiscountSwapInput)
	asset, _, err := orderTerms(order, s.intent.To)
	if err != nil {
		return fillSpend{}, err
	}
	if len(funding) != 1 {
		return fillSpend{}, errors.New("saved RFQ fill requires one connector")
	}
	state, err := s.workflow.state(ctx, funding[0].Adapter, nil)
	if err != nil {
		return fillSpend{}, err
	}
	if state.Asset != asset {
		return fillSpend{}, errors.New("saved RFQ payment asset mismatch")
	}
	return fillSpend{CapacityID: liquidlane.NewCapacityID(s.workflow.chainID.Int64(), state.Account, asset), Deadline: order.Request.Deadline.Int64()}, nil
}
