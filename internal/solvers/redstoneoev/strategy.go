package redstoneoev

import (
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/webhook"
)

func newStrategy(cfg *Config, deps strategies.Deps) (types.Strategy, error) {
	name := cfg.Strategy.Name
	if name == "" {
		name = defaultStrategyName
	}
	return strategies.New(name, cfg.Strategy.Config, deps)
}

func (s *Solver) bidInput(
	a AuctionMessage,
	now time.Time,
	st cachedState,
	inFlight inFlightState,
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
		Adapter: cloneAdapterSnapshot(st.Adapter),
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
		PendingAuctions: pendingAuctionsForStrategy(inFlight.pending, now),
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

func pendingAuctionsForStrategy(in []pendingAuction, now time.Time) []types.PendingAuction {
	out := make([]types.PendingAuction, 0, len(in))
	for _, a := range in {
		if a.ID == "" {
			continue
		}
		expiresAt := a.SentAt.Add(reservationTTL)
		if !expiresAt.After(now) {
			continue
		}
		out = append(out, types.PendingAuction{
			ID:        a.ID,
			SentAt:    a.SentAt,
			Won:       a.Won,
			ExpiresAt: expiresAt,
		})
	}
	slices.SortFunc(out, func(a, b types.PendingAuction) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
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
