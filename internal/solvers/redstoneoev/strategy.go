package redstoneoev

import (
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/webhook"
)

func newStrategy(cfg *Config, deps defaultstrategy.FactoryDeps) (types.Strategy, error) {
	name := cfg.Strategy.Name
	if name == "" {
		name = defaultStrategyName
	}
	switch name {
	case defaultstrategy.Name:
		return defaultstrategy.NewFromConfig(cfg.Strategy.Config, deps)
	case webhookstrategy.Name:
		return webhookstrategy.NewFromConfig(cfg.Strategy.Config)
	default:
		return nil, unknownStrategyError(name)
	}
}

func validateStrategyConfig(spec StrategyConfig, gasAccounting bool) error {
	switch spec.Name {
	case defaultstrategy.Name:
		return defaultstrategy.ValidateConfig(spec.Config, gasAccounting)
	case webhookstrategy.Name:
		return webhookstrategy.ValidateConfig(spec.Config)
	default:
		return unknownStrategyError(spec.Name)
	}
}

func strategyRequiresBidCap(name string) bool {
	return name == webhookstrategy.Name
}

func unknownStrategyError(name string) error {
	return errors.Errorf("unknown OEV strategy %q (registered: %v)", name, strategyNames())
}

func strategyNames() []string {
	return []string{defaultstrategy.Name, webhookstrategy.Name}
}

func (s *Solver) bidInput(
	a AuctionMessage,
	now time.Time,
	st cachedState,
	pendingAuctions []types.PendingAuction,
	gasPrice *big.Int,
) types.BidInput {
	return types.BidInput{
		Now: now,
		Auction: types.AuctionSnapshot{
			ID:            a.ID,
			Timestamp:     a.Timestamp,
			TimeoutMs:     a.TimeoutMs,
			RawPriceCount: len(a.Payload.Prices),
			Prices:        auctionPricesForStrategy(a),
		},
		Adapter: st.Adapter,
		Context: types.BidContext{
			ChainID:            cloneBig(s.chainID),
			Executor:           s.cfg.Executor,
			Callback:           s.cfg.Callback,
			Signer:             s.deps.Signer.Address(),
			ExecutorDeposit:    cloneBig(st.Exec.Deposit),
			ExecutorMinDeposit: cloneBig(minDeposit),
			MaxTxGasPrice:      cloneBig(gasPrice),
			GasPrices:          st.GasPrices,
			GasLimit:           st.GasLimit,
		},
		PendingAuctions: pendingAuctions,
	}
}

func auctionPricesForStrategy(a AuctionMessage) []types.AuctionPrice {
	out := make([]types.AuctionPrice, 0, len(a.Payload.Prices))
	for oracle, raw := range a.Payload.Prices {
		if !common.IsHexAddress(oracle) {
			continue
		}
		price, ok := new(big.Int).SetString(raw, 10)
		if !ok || price.Sign() <= 0 {
			continue
		}
		out = append(out, types.AuctionPrice{Oracle: common.HexToAddress(oracle), Price: price})
	}
	slices.SortFunc(out, func(a, b types.AuctionPrice) int {
		return a.Oracle.Cmp(b.Oracle)
	})
	return out
}

func checkExecutionEnvelope(out types.BidOutput) error {
	switch out.Decision {
	case types.DecisionSkip:
		if out.BidAmount != nil || len(out.OperationData) != 0 {
			return errors.New("skip output must not include bid data")
		}
		return nil
	case types.DecisionBid:
	default:
		return errors.Errorf("invalid decision %q", out.Decision)
	}
	if out.BidAmount == nil || out.BidAmount.Sign() <= 0 {
		return errors.New("bid output has invalid bidAmount")
	}
	if len(out.OperationData) == 0 {
		return errors.New("bid output has empty operationData")
	}
	return nil
}
