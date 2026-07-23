package defaultstrategy

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies"
)

type gasLeg struct {
	route     liquidlane.Route
	amountOut *big.Int
	private   bool
}

type gasPricing struct {
	feePerGas         *big.Int
	tokenOutPerNative *big.Int
	snapshot          *liquidlanegas.Snapshot
}

func newGasPricing(
	maxFeePerGas *big.Int,
	tokenOut common.Address,
	prices *liquidlanegas.PriceSnapshot,
	snapshot *liquidlanegas.Snapshot,
	reserveBps int,
) (gasPricing, error) {
	if maxFeePerGas == nil || maxFeePerGas.Sign() < 0 {
		return gasPricing{}, errors.New("max fee per gas must be non-negative")
	}
	rate := prices.TokenOutPerNative(tokenOut)
	if maxFeePerGas.Sign() > 0 && (rate == nil || rate.Sign() <= 0) {
		return gasPricing{}, errors.Errorf("gas oracle: missing tokenOut rate for %s", tokenOut.Hex())
	}
	if rate == nil {
		rate = new(big.Int)
	}
	return gasPricing{
		feePerGas: new(big.Int).Set(maxFeePerGas), tokenOutPerNative: rate,
		snapshot: liquidlanegas.WithReserveBps(snapshot, reserveBps),
	}, nil
}

func (p gasPricing) cost(legs []gasLeg) *big.Int {
	shared := make([]strategies.GasLeg, 0, len(legs))
	for _, leg := range legs {
		shared = append(shared, strategies.GasLeg{
			Route: leg.route, AmountOut: leg.amountOut, Private: leg.private,
		})
	}
	return strategies.FillGasCostAtRate(p.feePerGas, p.tokenOutPerNative, p.snapshot, shared)
}
