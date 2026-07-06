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

// OfferInput is the 3F strategy decision snapshot. It is intentionally solver-local.
type OfferInput struct {
	Now        time.Time
	Adapters   []AdapterSnapshot
	Auctions   []AuctionSnapshot
	Candidates []OfferCandidate
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
	MinYieldBps   *big.Int
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
	RemainingAmount *big.Int
	MaxRateBps      float64
}

type OfferCandidate struct {
	ID string

	AdapterID    string
	AuctionID    int64
	Capacity     *big.Int
	HasLiveOffer bool
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
