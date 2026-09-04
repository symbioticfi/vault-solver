package redstoneoev

import (
	"math/big"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
)

const reservationTTL = 5 * time.Minute

// exposureBook is the sole owner of sent-bid resources. Snapshot returns pending auction and
// economic exposure facts under the same lock, so a planner never combines different result epochs.
type exposureBook struct {
	mu   sync.Mutex
	bids map[string]bidLease
}

type bidLease struct {
	nonce    uint64
	sentAt   time.Time
	won      bool
	resolved bool
	exposure decision.Exposure
}

func (b *exposureBook) acquire(id string, nonce uint64, at time.Time, exposure decision.Exposure) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.bids == nil {
		b.bids = make(map[string]bidLease)
	}
	b.bids[id] = bidLease{nonce: nonce, sentAt: at, exposure: cloneExposure(exposure)}
}

func (b *exposureBook) snapshot(now time.Time) ([]decision.PendingAuction, decision.Exposure) {
	b.mu.Lock()
	defer b.mu.Unlock()
	pending := make([]decision.PendingAuction, 0, len(b.bids))
	exposure := decision.Exposure{BidNative: new(big.Int), GasNative: new(big.Int)}
	for id, lease := range b.bids {
		expiresAt := lease.sentAt.Add(reservationTTL)
		if id == "" || !expiresAt.After(now) {
			continue
		}
		pending = append(pending, decision.PendingAuction{
			ID: id, SentAt: lease.sentAt, Won: lease.won, ExpiresAt: expiresAt,
		})
		exposure.BidNative.Add(exposure.BidNative, orZero(lease.exposure.BidNative))
		exposure.GasNative.Add(exposure.GasNative, orZero(lease.exposure.GasNative))
		exposure.Positions = append(exposure.Positions, lease.exposure.Positions...)
	}
	slices.SortFunc(pending, func(a, b decision.PendingAuction) int { return strings.Compare(a.ID, b.ID) })
	return pending, cloneExposure(exposure)
}

func (b *exposureBook) remove(id string) {
	b.mu.Lock()
	delete(b.bids, id)
	b.mu.Unlock()
}

func (b *exposureBook) markWon(id string) *big.Int {
	if id == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	lease, ok := b.bids[id]
	if ok {
		lease.won = true
		b.bids[id] = lease
	}
	return cloneBig(lease.exposure.BidNative)
}

func (b *exposureBook) markResolved(id string) *big.Int {
	if id == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	lease, ok := b.bids[id]
	if ok {
		lease.resolved = true
		b.bids[id] = lease
	}
	return cloneBig(lease.exposure.BidNative)
}

// reconcile releases only bids proven settled by nonce, acknowledged by a liquidation result, or
// expired after the missed-result safety TTL. Won bids otherwise stay pinned across state refreshes.
func (b *exposureBook) reconcile(onChainNonce uint64, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, lease := range b.bids {
		if lease.resolved || lease.nonce <= onChainNonce || now.Sub(lease.sentAt) > reservationTTL {
			delete(b.bids, id)
		}
	}
}

func (b *exposureBook) wonMetrics(now time.Time) (int, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	count := 0
	var oldest time.Time
	for _, lease := range b.bids {
		if !lease.won || lease.resolved {
			continue
		}
		count++
		if oldest.IsZero() || lease.sentAt.Before(oldest) {
			oldest = lease.sentAt
		}
	}
	if oldest.IsZero() {
		return count, 0
	}
	return count, max(now.Sub(oldest), 0)
}

func cloneExposure(source decision.Exposure) decision.Exposure {
	return decision.Exposure{
		BidNative: cloneBig(source.BidNative),
		GasNative: cloneBig(source.GasNative),
		Positions: slices.Clone(source.Positions),
	}
}

// maxSeenAuctions bounds replay de-duplication across websocket reconnects.
const maxSeenAuctions = 1024

type seenAuctions struct {
	set   map[string]struct{}
	order []string
	cap   int
}

func newSeenAuctions(capacity int) *seenAuctions {
	if capacity <= 0 {
		panic("redstoneoev: seen auction capacity must be positive")
	}
	return &seenAuctions{set: make(map[string]struct{}, capacity), cap: capacity}
}

func (s *seenAuctions) seen(id string) bool {
	if _, ok := s.set[id]; ok {
		return true
	}
	if len(s.order) == s.cap {
		delete(s.set, s.order[0])
		s.order = s.order[1:]
	}
	s.set[id] = struct{}{}
	s.order = append(s.order, id)
	return false
}
