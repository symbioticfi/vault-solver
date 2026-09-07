package redstoneoev

import (
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	local "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
	remote "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/webhook"
)

func newStrategy(cfg *Config, deps types.Dependencies) (types.Strategy, error) {
	spec := cfg.Strategy

	switch spec.Name {
	case "", local.Name:
		return local.NewFromConfig(spec.Config, deps)
	case remote.Name:
		return remote.NewFromConfig(spec.Config, deps)
	default:
		return nil, errors.Errorf("unknown OEV strategy %q (available: default, webhook)", spec.Name)
	}
}

func (s *Solver) bidInput(
	a AuctionMessage,
	now time.Time,
	st cachedState,
	inFlight []types.PendingAuction,
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
		Adapter: st.Adapter.Clone(),
		Context: types.BidContext{
			ChainID:            bigmath.Clone(s.chainID),
			Executor:           s.cfg.Executor,
			Callback:           s.cfg.Callback,
			Signer:             s.deps.Signer.Address(),
			ExecutorDeposit:    bigmath.Clone(st.Exec.Deposit),
			ExecutorMinDeposit: bigmath.Clone(minDeposit),
			MaxTxGasPrice:      bigmath.Clone(gasPrice),
			GasPrices:          st.GasPrices,
			GasLimit:           st.GasLimit,
		},
		PendingAuctions: pendingAuctionsForStrategy(inFlight, now),
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

func pendingAuctionsForStrategy(pending []types.PendingAuction, now time.Time) []types.PendingAuction {
	active := pending[:0]
	for _, auction := range pending {
		snapshot := auction
		snapshot.ExpiresAt = auction.SentAt.Add(reservationTTL)
		if snapshot.ID != "" && now.Before(snapshot.ExpiresAt) {
			active = append(active, snapshot)
		}
	}
	slices.SortFunc(active, func(a, b types.PendingAuction) int { return strings.Compare(a.ID, b.ID) })
	return active
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
