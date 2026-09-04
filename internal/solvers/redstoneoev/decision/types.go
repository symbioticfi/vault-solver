package decision

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/morpho"
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
	SkipReasonDepositLow      = "deposit_low"
	SkipReasonStrategy        = "strategy_skip"
)

func BoundedSkipReason(reason string) string {
	switch reason {
	case SkipReasonNoLegs, SkipReasonGasUnprofitable, SkipReasonStaleEpoch, SkipReasonStaleState, SkipReasonInFlight, SkipReasonCallbackBalance, SkipReasonDepositLow:
		return reason
	default:
		return SkipReasonStrategy
	}
}

type Planner interface {
	DecideBid(ctx context.Context, input BidInput) (BidOutput, error)
}

// FactSource owns the refresh lifecycle for the local market view. The coordinator runs it and
// captures one immutable fact set before invoking a planner.
type FactSource interface {
	Run(context.Context)
	Snapshot(auction AuctionSnapshot, now time.Time, adapter AdapterSnapshot) MarketFacts
}

type BidInput struct {
	Now             time.Time
	Auction         AuctionSnapshot
	Adapter         AdapterSnapshot
	Context         BidContext
	PendingAuctions []PendingAuction
	Exposure        Exposure
	Market          MarketFacts `json:"-"`
}

// MarketFacts is the head-coherent local Morpho view used by the default planner. It is excluded
// from the webhook contract: an external planner owns its own market data and receives the stable
// protocol decision surface only.
type MarketFacts struct {
	UpdatedAt    time.Time
	Block        uint64
	BlockTime    uint64
	HasPositions bool
	Candidates   []LiquidationCandidate
}

type MarketParams struct {
	LoanToken       common.Address
	CollateralToken common.Address
	Oracle          common.Address
	Irm             common.Address
	Lltv            *big.Int
}

type MarketInfo struct {
	Params MarketParams
	State  morpho.MarketState
}

type AdapterQuote struct {
	MaxRate   *big.Int
	MaxAssets *big.Int
	LoanScale *big.Int
	CollScale *big.Int
}

type LiquidationCandidate struct {
	MarketID common.Hash
	Borrower common.Address
	Market   MarketInfo
	Position morpho.PositionState
	Price    *big.Int
	Quote    AdapterQuote
	Accrued  *big.Int
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
	ChainID            *big.Int
	Executor           common.Address
	Callback           common.Address
	CallbackNative     *big.Int
	Signer             common.Address
	ExecutorDeposit    *big.Int
	ExecutorMinDeposit *big.Int
	MaxTxGasPrice      *big.Int
	GasPrices          *liquidlanegas.PriceSnapshot
	GasLimit           uint64
}

type PendingAuction struct {
	ID        string
	SentAt    time.Time
	Won       bool
	ExpiresAt time.Time
}

// Exposure is the complete resource claim of unresolved bids. It is owned by the OEV coordinator,
// not by a strategy implementation, so every planner sees the same callback, deposit, and position use.
type Exposure struct {
	BidNative *big.Int
	GasNative *big.Int
	Positions []PositionClaim
}

type PositionClaim struct {
	MarketID common.Hash
	Borrower common.Address
}

type BidOutput struct {
	Decision      Decision
	Reason        string
	BidAmount     *big.Int
	OperationData []byte
	Exposure      Exposure
}
