package redstoneoev

import (
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/policy"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

func newPlanner(cfg *Config, deps policy.FactoryDeps) (decision.Planner, decision.FactSource, error) {
	switch cfg.Strategy.Name {
	case policy.Name:
		return policy.NewFromConfig(cfg.Strategy.Config, deps)
	case webhookPlannerName:
		planner, err := newWebhookPlannerFromConfig(cfg.Strategy.Config)
		return planner, nil, err
	default:
		return nil, nil, errors.Errorf("unknown OEV strategy %q", cfg.Strategy.Name)
	}
}

func validatePlannerConfig(spec StrategyConfig, gasAccounting bool) error {
	switch spec.Name {
	case policy.Name:
		return policy.ValidateConfig(spec.Config, gasAccounting)
	case webhookPlannerName:
		return webhook.ValidateConfig(spec.Config)
	default:
		return errors.Errorf("unknown OEV strategy %q", spec.Name)
	}
}

func (s *Solver) bidInput(
	a AuctionMessage,
	now time.Time,
	st cachedState,
	pendingAuctions []decision.PendingAuction,
	exposure decision.Exposure,
	gasPrice *big.Int,
) decision.BidInput {
	input := decision.BidInput{
		Now: now,
		Auction: decision.AuctionSnapshot{
			ID:            a.ID,
			Timestamp:     a.Timestamp,
			TimeoutMs:     a.TimeoutMs,
			RawPriceCount: len(a.Payload.Prices),
			Prices:        auctionPricesForStrategy(a),
		},
		Adapter: st.Adapter,
		Context: decision.BidContext{
			ChainID:            cloneBig(s.chainID),
			Executor:           s.cfg.Executor,
			Callback:           s.cfg.Callback,
			CallbackNative:     cloneBig(st.CallbackNative),
			Signer:             s.signer.Address(),
			ExecutorDeposit:    cloneBig(st.Exec.Deposit),
			ExecutorMinDeposit: cloneBig(minDeposit),
			MaxTxGasPrice:      cloneBig(gasPrice),
			GasPrices:          st.GasPrices,
			GasLimit:           st.GasLimit,
		},
		PendingAuctions: pendingAuctions,
		Exposure:        cloneExposure(exposure),
	}
	if s.facts != nil {
		input.Market = s.facts.Snapshot(input.Auction, now, input.Adapter)
	}
	return input
}

func auctionPricesForStrategy(a AuctionMessage) []decision.AuctionPrice {
	out := make([]decision.AuctionPrice, 0, len(a.Payload.Prices))
	for oracle, raw := range a.Payload.Prices {
		if !common.IsHexAddress(oracle) {
			continue
		}
		price, ok := new(big.Int).SetString(raw, 10)
		if !ok || price.Sign() <= 0 {
			continue
		}
		out = append(out, decision.AuctionPrice{Oracle: common.HexToAddress(oracle), Price: price})
	}
	slices.SortFunc(out, func(a, b decision.AuctionPrice) int {
		return a.Oracle.Cmp(b.Oracle)
	})
	return out
}

func checkExecutionEnvelope(out decision.BidOutput) error {
	switch out.Decision {
	case decision.DecisionSkip:
		if out.BidAmount != nil || len(out.OperationData) != 0 ||
			out.Exposure.BidNative != nil || out.Exposure.GasNative != nil || len(out.Exposure.Positions) != 0 {
			return errors.New("skip output must not include bid data")
		}
		return nil
	case decision.DecisionBid:
	default:
		return errors.Errorf("invalid decision %q", out.Decision)
	}
	if out.BidAmount == nil || out.BidAmount.Sign() <= 0 {
		return errors.New("bid output has invalid bidAmount")
	}
	if len(out.OperationData) == 0 {
		return errors.New("bid output has empty operationData")
	}
	if out.Exposure.BidNative == nil || out.Exposure.BidNative.Cmp(out.BidAmount) != 0 {
		return errors.New("bid output exposure does not match bidAmount")
	}
	if out.Exposure.GasNative == nil || out.Exposure.GasNative.Sign() < 0 {
		return errors.New("bid output has invalid gas exposure")
	}
	if len(out.Exposure.Positions) == 0 {
		return errors.New("bid output has no position exposure")
	}
	for _, claim := range out.Exposure.Positions {
		if claim.MarketID == (common.Hash{}) || claim.Borrower == (common.Address{}) {
			return errors.New("bid output has invalid position exposure")
		}
	}
	return nil
}
