package types

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type Decision string

const (
	DecisionBid  Decision = "bid"
	DecisionSkip Decision = "skip"
)

// Standard skip reasons kept as first-class solver metric labels.
const (
	SkipReasonNoLegs          = "no_legs"
	SkipReasonGasUnprofitable = "gas_unprofitable"
	SkipReasonStaleEpoch      = "stale_epoch"
	SkipReasonStaleState      = "stale_state"
	SkipReasonInFlight        = "in_flight"
	SkipReasonCallbackBalance = "callback_balance"
	SkipReasonStrategy        = "strategy_skip"
)

func BoundedSkipReason(reason string) string {
	switch reason {
	case SkipReasonNoLegs, SkipReasonGasUnprofitable, SkipReasonStaleEpoch, SkipReasonStaleState, SkipReasonInFlight, SkipReasonCallbackBalance:
		return reason
	default:
		return SkipReasonStrategy
	}
}

type Strategy interface {
	Run(ctx context.Context)
	DecideBid(ctx context.Context, input BidInput) (BidOutput, error)
}

type BidInput struct {
	Now             time.Time
	Auction         AuctionSnapshot
	Adapter         AdapterSnapshot
	Context         BidContext
	PendingAuctions []PendingAuction
}

type AuctionSnapshot struct {
	ID            string
	Timestamp     int64
	TimeoutMs     int
	RawPriceCount int
	Prices        []AuctionPrice
}

type AuctionPrice struct {
	Oracle common.Address
	Price  *big.Int
}

type AdapterSnapshot struct {
	Address      common.Address
	Vault        common.Address
	Loan         common.Address
	LoanDecimals int
	Paused       bool
	FreeAssets   *big.Int
	Withdrawable *big.Int
	Redeemable   []RedeemableSnapshot
	Filler       bool
}

type RedeemableSnapshot struct {
	Asset          common.Address
	Decimals       int
	MaxRate        *big.Int
	MaxAssets      *big.Int
	AcquireBalance *big.Int
}

type BidContext struct {
	ChainID         *big.Int
	Executor        common.Address
	Callback        common.Address
	Signer          common.Address
	ExecutorDeposit *big.Int
	MaxTxGasPrice   *big.Int
	GasLimit        uint64
}

type PendingAuction struct {
	ID        string
	SentAt    time.Time
	Won       bool
	ExpiresAt time.Time
}

type BidOutput struct {
	Decision      Decision
	Reason        string
	BidAmount     *big.Int
	OperationData []byte
}
