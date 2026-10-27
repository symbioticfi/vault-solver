// Package types defines the 3F-local strategy contract.
package types

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type Strategy interface {
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

type AuctionSnapshot struct {
	ID            string
	AuctionID     int64
	OriginalIndex int

	Request      common.Address
	Status       string
	DepositAsset common.Address

	AmountRequested *big.Int
	// RemainingAmount is AmountRequested minus this solver's live offers on the auction. It informs
	// remote strategies; the default strategy sizes every adapter against AmountRequested.
	RemainingAmount *big.Int
	// MaxRateBps is an APR when RateBasisSeconds is set, and the offer's expectedReturn covers
	// RateBasisSeconds of it (see types.ExpectedReturn); with RateBasisSeconds 0 it is a per-term rate.
	MaxRateBps float64
	// RateBasisSeconds is 3F's rateBasisSeconds: a year for auctions created after the APR switch, the
	// settlement delay for ones created before it, 0 when the API response predates the switch.
	RateBasisSeconds int64
}

// LiveOffer is one offer the solver already holds through an adapter on an auction. The strategy uses
// these to reserve principal and request slots across discovery passes, and deduplicate pairs.
type LiveOffer struct {
	AdapterID string
	AuctionID int64
	Principal *big.Int
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
