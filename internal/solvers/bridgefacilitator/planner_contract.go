package bridgefacilitator

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type Planner interface {
	DecideOffers(ctx context.Context, input OfferInput) (OfferOutput, error)
}

// OfferInput is the 3F strategy decision snapshot. It is intentionally solver-local. The solver only
// supplies raw facts — adapter liquidity/caps, open auctions, and the live offers it already holds; the
// strategy owns every decision (sizing, selection, dedup) built from them.
type OfferInput struct {
	Now        time.Time
	Adapters   []AdapterSnapshot
	Auctions   []AuctionSnapshot
	LiveOffers []LiveOffer
}

type AdapterSnapshot struct {
	ID string

	Adapter    common.Address
	Vault      common.Address
	Collateral common.Address

	Fundable      *big.Int
	OpenCount     int
	MaxAssets     *big.Int
	MinAssets     *big.Int
	MinYieldPpm   *big.Int // minYieldPerRequest in ppm — the exact on-chain floor (webhook derives bps if needed)
	MaxConcurrent int
}

func (snapshot AdapterSnapshot) Matches(asset common.Address) bool {
	return snapshot.Collateral == asset
}

func (snapshot AdapterSnapshot) CanOpen(additional int) bool {
	return snapshot.MaxConcurrent <= 0 || snapshot.OpenCount+additional < snapshot.MaxConcurrent
}

type AuctionSnapshot struct {
	ID            string
	AuctionID     int64
	OriginalIndex int

	Request      common.Address
	Status       string
	DepositAsset common.Address

	AmountRequested *big.Int
	RemainingAmount *big.Int
	MaxRateBps      float64
}

// LiveOffer is one offer the solver already holds through an adapter on an auction. The strategy uses
// these to avoid re-offering through the same adapter while one is live.
type LiveOffer struct {
	AdapterID string
	AuctionID int64
}

type OfferOutput struct {
	Offers []OfferExecution
}

// OfferExecution is the trusted strategy's execution output. The solver only signs and submits it.
type OfferExecution struct {
	AuctionID      int64
	Request        common.Address
	Maker          common.Address
	Principal      *big.Int
	ExpectedReturn *big.Int
	Reason         string
}
