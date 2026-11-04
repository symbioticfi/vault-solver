package uniswapx

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	dexbinding "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/dex"
	uxexecutor "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

var errDexUnavailable = errors.New("dex unavailable")

func (s *Solver) sourceTokenOut(in, out common.Address) common.Address {
	if s.cfg.Dex.matches(in, out) {
		return s.cfg.Dex.BridgeToken
	}
	return out
}

func (s *Solver) dexResidual(maxFee *big.Int, prices *liquidlanegas.PriceSnapshot) (*big.Int, error) {
	rate := prices.TokenOutPerNative(s.cfg.Dex.BridgeToken)
	if maxFee == nil || maxFee.Sign() <= 0 || rate == nil || rate.Sign() <= 0 {
		return nil, errors.New("dex requires current fee and bridge gas price")
	}
	native := new(big.Int).Mul(maxFee, new(big.Int).SetUint64(s.cfg.Dex.GasUnitsBudget))
	gas := liquidlane.MulDivUp(native, rate, new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	return gas.Add(gas, s.cfg.Dex.MinProfitBridge), nil
}

func dexSpendCap(amount *big.Int, bps int) *big.Int {
	return liquidlane.MulDivUp(amount, big.NewInt(int64(10_000+bps)), big.NewInt(10_000))
}

func (s *Solver) usableDexQuote(q dexQuote) bool {
	return q.Amount != nil && q.Amount.Sign() > 0 &&
		!q.BlockTime.IsZero() && q.BlockTime.Add(s.cfg.Dex.PlanValidity).After(time.Unix(s.currentTime(), 0))
}

func (s *Solver) decideDexQuote(ctx context.Context, input types.QuoteInput) (*types.Quote, error) {
	residual, err := s.dexResidual(input.MaxFeePerGas, input.GasPrices)
	if err != nil {
		return nil, err
	}
	bridge := input
	bridge.TokenOut = s.cfg.Dex.BridgeToken
	// Composite economics owns one full gas budget; do not also subtract the direct-path gas floor.
	bridge.MaxFeePerGas, bridge.GasPrices, bridge.GasSnapshot = new(big.Int), nil, nil
	if input.AmountIn != nil {
		quote, err := s.decideQuote(ctx, bridge)
		if err != nil || quote == nil {
			return nil, err
		}
		if quote.AmountOut.Cmp(s.cfg.Dex.MaxBridgeAmount) > 0 || quote.AmountOut.Cmp(residual) <= 0 {
			return nil, nil
		}
		budget := new(big.Int).Sub(quote.AmountOut, residual)
		market, err := s.dex.quote(ctx, input.TokenOut, budget, false)
		if err != nil {
			return nil, errors.Errorf("%w: %w", errDexUnavailable, err)
		}
		if !s.usableDexQuote(market) {
			return nil, errDexUnavailable
		}
		output := new(big.Int).Mul(market.Amount, big.NewInt(int64(10_000-s.cfg.Dex.SlippageBps)))
		output.Div(output, big.NewInt(10_000))
		if output.Sign() <= 0 {
			return nil, nil
		}
		return &types.Quote{AmountIn: new(big.Int).Set(input.AmountIn), AmountOut: output, CandidateID: quote.CandidateID}, nil
	}
	market, err := s.dex.quote(ctx, input.TokenOut, input.AmountOut, true)
	if err != nil {
		return nil, errors.Errorf("%w: %w", errDexUnavailable, err)
	}
	if !s.usableDexQuote(market) {
		return nil, errDexUnavailable
	}
	bridge.AmountOut = new(big.Int).Add(dexSpendCap(market.Amount, s.cfg.Dex.SlippageBps), residual)
	if bridge.AmountOut.Cmp(s.cfg.Dex.MaxBridgeAmount) > 0 {
		return nil, nil
	}
	quote, err := s.decideQuote(ctx, bridge)
	if err != nil || quote == nil {
		return nil, err
	}
	return &types.Quote{AmountIn: quote.AmountIn, AmountOut: new(big.Int).Set(input.AmountOut), CandidateID: quote.CandidateID}, nil
}

func (s *Solver) prepareDexFill(ctx context.Context, order *resolvedOrder, input types.FillInput,
	routes []liquidlane.Route, now, observedAt time.Time, revision uint64,
) (preparedFill, error) {
	residual, err := s.dexResidual(input.MaxFeePerGas, input.GasPrices)
	if err != nil {
		return preparedFill{}, err
	}
	market, err := s.dex.quote(ctx, input.TokenOut, input.OutputAmount, true)
	if err != nil || !s.usableDexQuote(market) {
		return preparedFill{}, declineFill(ctx, "fill_declined", "dex quote unavailable or stale")
	}
	maximum := dexSpendCap(market.Amount, s.cfg.Dex.SlippageBps)
	input.TokenOut = s.cfg.Dex.BridgeToken
	input.OutputAmount = new(big.Int).Add(maximum, residual)
	input.Dex = &types.DexExecution{MaxDexInput: maximum, MinBridgeResidual: residual,
		ValidUntil: earlierTime(time.Unix(int64(order.Deadline), 0), market.BlockTime.Add(s.cfg.Dex.PlanValidity))}
	input.MaxFeePerGas, input.GasPrices, input.GasSnapshot = new(big.Int), nil, nil
	return s.prepareFill(ctx, order, input, routes, now, observedAt, revision)
}

func packDexExecutorCalldata(order *resolvedOrder, plan *types.DexExecution,
	direct []uxexecutor.ILiquidLaneUniswapXExecutorFillRoute,
	discounts []uxexecutor.ILiquidLaneUniswapXExecutorDiscountRoute,
) ([]byte, error) {
	call := dexbinding.ILiquidLaneUniswapXExecutorFillCall{
		Routes:         make([]dexbinding.ILiquidLaneUniswapXExecutorFillRoute, len(direct)),
		DiscountRoutes: make([]dexbinding.ILiquidLaneUniswapXExecutorDiscountRoute, len(discounts)),
	}
	for i, route := range direct {
		call.Routes[i] = dexbinding.ILiquidLaneUniswapXExecutorFillRoute(route)
	}
	for i, route := range discounts {
		call.DiscountRoutes[i] = dexbinding.ILiquidLaneUniswapXExecutorDiscountRoute{
			Adapter: route.Adapter, AmountIn: route.AmountIn, ProtocolSignature: route.ProtocolSignature,
			DiscountSwap: dexbinding.ILiquidLaneAdapterDiscountSwap{
				Discount:        dexbinding.ILiquidLaneAdapterDiscount(route.DiscountSwap.Discount),
				SignerSignature: route.DiscountSwap.SignerSignature, ProtocolDeadline: route.DiscountSwap.ProtocolDeadline,
			},
		}
	}
	return dexABI.TryPackExecuteWithDex(dexbinding.UniswapXSignedOrder{Order: order.Encoded, Sig: order.Signature}, call,
		dexbinding.LiquidLaneUniswapXDexExecutorDexLimits{MaxBridgePayout: plan.MaxBridgePayout,
			MaxDexInput: plan.MaxDexInput, MinBridgeResidual: plan.MinBridgeResidual,
			ValidUntil: big.NewInt(plan.ValidUntil.Unix())})
}
