package rfq

import (
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/google/uuid"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

const maxSwapRecords = 10_000

var (
	errSwapRecordNotFound = errors.New("swap record not found")
	errSwapRecordExpired  = errors.New("swap record expired")
	errSwapStoreFull      = errors.New("swap record store full")
	errSwapBuildConflict  = errors.New("swap confirmation already bound to a different build")
)

type discoveryPointRecord struct {
	AmountIn  *big.Int
	AmountOut *big.Int
	Domains   []liquidlane.CapacityID
}

type discoveryRecord struct {
	RequestID uuid.UUID
	QuoteID   uuid.UUID
	ChainID   int64
	Swapper   common.Address
	TokenIn   common.Address
	TokenOut  common.Address
	Points    map[string]discoveryPointRecord
	ExpiresAt time.Time
}

type confirmationRecord struct {
	SolverQuoteID      uuid.UUID
	DiscoveryRequestID uuid.UUID
	QuoteID            uuid.UUID
	ChainID            int64
	Swapper            common.Address
	TokenIn            common.Address
	TokenOut           common.Address
	AmountIn           *big.Int
	AmountOut          *big.Int
	PublicDeadline     time.Time
	ValidUntil         time.Time
	Domains            []liquidlane.CapacityID
	Plan               *fillPlan
}

type confirmationEntry struct {
	record confirmationRecord

	buildMu          sync.Mutex
	buildID          uuid.UUID
	buildFingerprint common.Hash
	built            *swapResponse
}

type swapStore struct {
	mu            sync.Mutex
	discoveries   map[uuid.UUID]discoveryRecord
	confirmations map[uuid.UUID]*confirmationEntry
	now           func() time.Time
}

func newSwapStore(now func() time.Time) *swapStore {
	return &swapStore{
		discoveries: make(map[uuid.UUID]discoveryRecord), confirmations: make(map[uuid.UUID]*confirmationEntry),
		now: now,
	}
}

func (s *swapStore) putDiscovery(record discoveryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	if _, exists := s.discoveries[record.RequestID]; exists {
		return errSwapBuildConflict
	}
	if len(s.discoveries)+len(s.confirmations) >= maxSwapRecords {
		return errSwapStoreFull
	}
	s.discoveries[record.RequestID] = cloneDiscoveryRecord(record)
	return nil
}

func (s *swapStore) discovery(id uuid.UUID) (*discoveryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.discoveries[id]
	if !exists {
		return nil, errSwapRecordNotFound
	}
	if !s.now().Before(record.ExpiresAt) {
		delete(s.discoveries, id)
		return nil, errSwapRecordExpired
	}
	cloned := cloneDiscoveryRecord(record)
	return &cloned, nil
}

func (s *swapStore) putConfirmation(record confirmationRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	if _, exists := s.confirmations[record.SolverQuoteID]; exists {
		return errSwapBuildConflict
	}
	if len(s.discoveries)+len(s.confirmations) >= maxSwapRecords {
		return errSwapStoreFull
	}
	s.confirmations[record.SolverQuoteID] = &confirmationEntry{record: cloneConfirmationRecord(record)}
	return nil
}

func (s *swapStore) confirmation(id uuid.UUID) (*confirmationRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.confirmations[id]
	if !exists {
		return nil, errSwapRecordNotFound
	}
	if !s.now().Before(entry.record.ValidUntil) {
		delete(s.confirmations, id)
		return nil, errSwapRecordExpired
	}
	cloned := cloneConfirmationRecord(entry.record)
	return &cloned, nil
}

func (s *swapStore) acquireBuild(id, buildID uuid.UUID, fingerprint common.Hash) (*buildLease, error) {
	s.mu.Lock()
	entry, exists := s.confirmations[id]
	s.mu.Unlock()
	if !exists {
		return nil, errSwapRecordNotFound
	}

	entry.buildMu.Lock()
	if !s.now().Before(entry.record.ValidUntil) {
		entry.buildMu.Unlock()
		return nil, errSwapRecordExpired
	}
	if entry.buildID == uuid.Nil {
		entry.buildID = buildID
		entry.buildFingerprint = fingerprint
	} else if entry.buildID != buildID || entry.buildFingerprint != fingerprint {
		entry.buildMu.Unlock()
		return nil, errSwapBuildConflict
	}
	return &buildLease{
		entry: entry, record: cloneConfirmationRecord(entry.record), cached: cloneSwapResponse(entry.built),
	}, nil
}

func (s *swapStore) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
}

func (s *swapStore) sweepLocked() {
	now := s.now()
	for id, record := range s.discoveries {
		if !now.Before(record.ExpiresAt) {
			delete(s.discoveries, id)
		}
	}
	for id, entry := range s.confirmations {
		if !now.Before(entry.record.ValidUntil) {
			delete(s.confirmations, id)
		}
	}
}

type buildLease struct {
	entry   *confirmationEntry
	record  confirmationRecord
	cached  *swapResponse
	release sync.Once
}

func (l *buildLease) Record() *confirmationRecord {
	cloned := cloneConfirmationRecord(l.record)
	return &cloned
}

func (l *buildLease) Cached() *swapResponse { return cloneSwapResponse(l.cached) }

func (l *buildLease) Complete(response *swapResponse) {
	l.entry.built = cloneSwapResponse(response)
	l.cached = cloneSwapResponse(response)
}

func (l *buildLease) Release() {
	l.release.Do(func() { l.entry.buildMu.Unlock() })
}

func cloneDiscoveryRecord(record discoveryRecord) discoveryRecord {
	out := record
	out.Points = make(map[string]discoveryPointRecord, len(record.Points))
	for key, point := range record.Points {
		out.Points[key] = discoveryPointRecord{
			AmountIn:  liquidlane.CloneBig(point.AmountIn),
			AmountOut: liquidlane.CloneBig(point.AmountOut),
			Domains:   append([]liquidlane.CapacityID(nil), point.Domains...),
		}
	}
	return out
}

func cloneConfirmationRecord(record confirmationRecord) confirmationRecord {
	out := record
	out.AmountIn = liquidlane.CloneBig(record.AmountIn)
	out.AmountOut = liquidlane.CloneBig(record.AmountOut)
	out.Domains = append([]liquidlane.CapacityID(nil), record.Domains...)
	out.Plan = cloneFillPlan(record.Plan)
	return out
}

func cloneFillPlan(plan *fillPlan) *fillPlan {
	if plan == nil {
		return nil
	}
	out := *plan
	out.AmountIn = liquidlane.CloneBig(plan.AmountIn)
	out.QuotedAmountOut = liquidlane.CloneBig(plan.QuotedAmountOut)
	out.Legs = make([]fillLeg, len(plan.Legs))
	for i, leg := range plan.Legs {
		out.Legs[i] = leg
		out.Legs[i].AmountIn = liquidlane.CloneBig(leg.AmountIn)
		out.Legs[i].AmountOut = liquidlane.CloneBig(leg.AmountOut)
		out.Legs[i].MaxRate = liquidlane.CloneBig(leg.MaxRate)
		out.Legs[i].DiscountID = liquidlane.CloneHash(leg.DiscountID)
	}
	return &out
}

func cloneSwapResponse(response *swapResponse) *swapResponse {
	if response == nil {
		return nil
	}
	out := *response
	out.LiquidityDomains = append([]string(nil), response.LiquidityDomains...)
	if response.Points != nil {
		points := make([]swapPointResponse, len(*response.Points))
		for i, point := range *response.Points {
			points[i] = point
			points[i].LiquidityDomains = append([]string(nil), point.LiquidityDomains...)
		}
		out.Points = &points
	}
	if response.Calls != nil {
		calls := append([]swapCallResponse(nil), (*response.Calls)...)
		out.Calls = &calls
	}
	return &out
}
