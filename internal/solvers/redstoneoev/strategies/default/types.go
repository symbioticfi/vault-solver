package defaultstrategy

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type liquidLaneState = liquidlanegas.State

// MarketParams is Morpho's MarketParams tuple (loanToken, collateralToken, oracle, irm, lltv).
type MarketParams struct {
	LoanToken       common.Address
	CollateralToken common.Address
	Oracle          common.Address
	Irm             common.Address
	Lltv            *big.Int
}

type abiMarketParams = MarketParams

// MarketInfo is a Morpho market's params plus its state snapshot.
type MarketInfo struct {
	Params MarketParams
	State  morpho.MarketState
}

// AdapterQuote is the LiquidLane redemption quote used by the default Morpho strategy.
type AdapterQuote struct {
	MaxRate   *big.Int
	MaxAssets *big.Int
	LoanScale *big.Int
	CollScale *big.Int
}

type SizingParams struct {
	AllowFullLiquidation bool
	SwapHaircutBps       int
}

// selectedLeg is one sized Morpho liquidation selected by the strategy.
type selectedLeg struct {
	MarketId       common.Hash
	Borrower       common.Address
	MaxSeizeAssets *big.Int
	MinProfit      *big.Int
}

type legHint struct {
	selectedLeg

	Collateral      common.Address
	ExpectedLoanOut *big.Int
}

type TestMonitorConfig struct {
	Markets   []common.Hash
	Positions []common.Address
}

type Config struct {
	MorphoAPIURL             string
	TestMonitor              *TestMonitorConfig
	DiscoveryMaxHealthFactor float64
	MaxTrackedPositions      int
	BidWei                   *big.Int
	MinBundleProfitBidBps    int
	TotalBundleProfitBps     int
	Sizing                   SizingParams
	CallbackAuthTTL          time.Duration
	MonitorPoll              time.Duration
	MaxStateAge              time.Duration
}

type Deps struct {
	Reader              Reader
	Signer              signer
	Log                 logr.Logger
	ChainID             int64
	Adapter             common.Address
	Callback            common.Address
	LoadAdapterSnapshot func() (types.AdapterSnapshot, bool)
	GasAccounting       bool
}

type signer interface {
	Address() common.Address
	SignHash(common.Hash) ([]byte, error)
}

type Reader interface {
	ReadNativeBalance(ctx context.Context, account common.Address) (*big.Int, error)
	ResolveParams(ctx context.Context, morphoAddr common.Address, ids []common.Hash) (map[common.Hash]MarketParams, error)
	ReadHead(ctx context.Context) (number uint64, timestamp uint64, err error)
	ReadCallbackMorpho(ctx context.Context, callback common.Address) (common.Address, error)
	ReadTestMarketStates(ctx context.Context, morphoAddr common.Address, params map[common.Hash]MarketParams) (map[common.Hash]MarketInfo, map[common.Hash]*big.Int, error)
	ReadTestPositions(ctx context.Context, morphoAddr common.Address, markets map[common.Hash]MarketInfo, borrowers []common.Address) (map[common.Hash]map[common.Address]morpho.PositionState, error)
}
