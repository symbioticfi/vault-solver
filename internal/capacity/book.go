// Package capacity serializes reservations of physical liquidity shared by every solver.
package capacity

import (
	"math/big"
	"strings"
	"sync"

	"github.com/go-errors/errors"
)

type ID string
type Amounts map[ID]*big.Int
type Owner string

func (a Amounts) Add(id ID, amount *big.Int) {
	if id == "" || amount == nil || amount.Sign() <= 0 {
		return
	}
	if a[id] == nil {
		a[id] = new(big.Int)
	}
	a[id].Add(a[id], amount)
}

func (a Amounts) AddAll(other Amounts) {
	for id, amount := range other {
		a.Add(id, amount)
	}
}

// Limits reconstructs the physical ceiling seen by a planner.
func Limits(observed, requested Amounts) Amounts {
	limits := make(Amounts, len(observed)+len(requested))
	limits.AddAll(observed)
	limits.AddAll(requested)
	return limits
}

func NewOwner(workflow, reference string) Owner {
	if workflow == "" || reference == "" {
		return ""
	}
	return Owner(strings.ToLower(workflow) + ":" + strings.ToLower(reference))
}

// Book is the single process-wide ledger. Its zero value is ready to use.
type Book struct {
	mu      sync.RWMutex
	pending map[Owner]Amounts
}

func (b *Book) remove(owner Owner) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.pending[owner]; !ok {
		return false
	}
	delete(b.pending, owner)
	return true
}

func (b *Book) Snapshot() Amounts {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make(Amounts)
	for _, claim := range b.pending {
		result.AddAll(claim)
	}
	return result
}

func (b *Book) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.pending)
}

// Acquire atomically admits owner under the physical limits observed by its planner.
func (b *Book) Acquire(owner Owner, amounts, limits Amounts) (*Lease, error) {
	claim, claimOK := clone(amounts)
	ceiling, limitsOK := clone(limits)
	if owner == "" || !claimOK || !limitsOK {
		return nil, errors.New("capacity: owner, claim, and limits must be positive")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.pending[owner]; exists {
		return nil, errors.Errorf("capacity: owner %q already reserved", owner)
	}
	used := make(Amounts)
	for _, pending := range b.pending {
		used.AddAll(pending)
	}
	for id, amount := range claim {
		limit := ceiling[id]
		current := used[id]
		if current == nil {
			current = new(big.Int)
		}
		if limit == nil || new(big.Int).Add(current, amount).Cmp(limit) > 0 {
			return nil, errors.Errorf("capacity: %q is unavailable", id)
		}
	}
	if b.pending == nil {
		b.pending = make(map[Owner]Amounts)
	}
	b.pending[owner] = claim
	return &Lease{book: b, owner: owner}, nil
}

type Lease struct {
	book  *Book
	owner Owner
}

func (l *Lease) Release() bool {
	return l != nil && l.book != nil && l.book.remove(l.owner)
}

func clone(source Amounts) (Amounts, bool) {
	if len(source) == 0 {
		return nil, false
	}
	result := make(Amounts, len(source))
	for id, amount := range source {
		if id == "" || amount == nil || amount.Sign() <= 0 {
			return nil, false
		}
		result[id] = new(big.Int).Set(amount)
	}
	return result, true
}
