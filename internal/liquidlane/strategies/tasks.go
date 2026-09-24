package strategies

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// UncoveredInputPolicy describes whether a LiquidLane quote or fill must source output
// for every input unit or may absorb excess input as price impact.
type UncoveredInputPolicy uint8

const (
	RejectUncoveredInput UncoveredInputPolicy = iota
	AbsorbUncoveredInput
)

// QuoteTask is a protocol-neutral LiquidLane pricing problem. Exactly one of
// ExactInput and ExactOutput must be set.
type QuoteTask struct {
	ExactInput  *big.Int
	ExactOutput *big.Int

	Candidates []liquidlane.QuoteCandidate
	MaxRoutes  int
	MinInput   *big.Int

	OutputBufferBps int
	InputPolicy     UncoveredInputPolicy
	GasPricing      *GasPricing
	Trace           DecisionTrace
}

// QuoteSolution is the priced amount pair and the LiquidLane allocation that
// produced it. Protocol adapters may omit the allocation from their wire reply.
type QuoteSolution struct {
	AmountIn       *big.Int
	GrossAmountOut *big.Int
	GasCost        *big.Int
	AmountOut      *big.Int
	Allocations    []QuoteAllocation
}

// SingleCandidateID identifies a quote covered by one source, or returns empty for an aggregate.
func (s *QuoteSolution) SingleCandidateID() liquidlane.CandidateID {
	if s == nil || len(s.Allocations) != 1 {
		return ""
	}
	return s.Allocations[0].Candidate.ID
}

// QuoteAllocation is the selected amount for one candidate.
type QuoteAllocation struct {
	Candidate liquidlane.QuoteCandidate
	AmountIn  *big.Int
	AmountOut *big.Int
}

// FillTask contains the protocol-neutral facts needed to route an exact-input fill.
type FillTask struct {
	TokenIn  common.Address
	TokenOut common.Address
	AmountIn *big.Int

	Quotes       []liquidlane.FillQuote
	Reservations liquidlane.CapacityReservations
	ValidAfter   time.Time

	// PrivateCapacityBufferBps reserves headroom for uncapped signed-discount payouts.
	// It never reduces the output available to honor an order.
	PrivateCapacityBufferBps int
	MaxRoutes                int
	InventoryReserveBps      int
	InputPolicy              UncoveredInputPolicy
	GasPricing               *GasPricing
	Trace                    DecisionTrace
}
