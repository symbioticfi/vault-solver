package liquidlane

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// Plan is the canonical executable allocation returned by LiquidLane planning.
// Protocol workflows attach their own metadata without copying these legs.
type Plan struct {
	Routes []PlanLeg `json:"routes"`
}

// PlanLeg is one adapter contribution to a Plan.
type PlanLeg struct {
	CandidateID       CandidateID    `json:"-"`
	RouteID           RouteID        `json:"routeId"`
	CapacityID        CapacityID     `json:"capacityId"`
	Adapter           common.Address `json:"adapter"`
	AmountIn          *big.Int       `json:"amountIn"`
	ExpectedAmountOut *big.Int       `json:"expectedAmountOut"`
	MinAmountOut      *big.Int       `json:"minAmountOut"`
	ReservedAmountOut *big.Int       `json:"reservedAmountOut"`
	DiscountID        *common.Hash   `json:"discountId"`
}
