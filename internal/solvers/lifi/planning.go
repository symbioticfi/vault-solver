package lifi

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

type fillState struct {
	snapshots       fillSnapshotSet
	discountQuotes  []liquidlane.FillQuote
	signedDiscounts map[common.Hash]*discounts.Signed
	chainTime       time.Time
}

type preparedFill struct {
	input           types.FillInput
	signedDiscounts map[common.Hash]*discounts.Signed
}

func (s *Solver) processOrderWithPending(
	ctx context.Context,
	routes []route,
	order *submittedOrder,
	pending *pendingFillState,
) *pendingFill {
	if !s.cfg.TokenPolicy.Allows(order.TokenIn) {
		s.log.V(1).Info("order skipped: input token out of scope",
			"orderId", order.OrderID, "quoteId", order.QuoteID,
			"tokenIn", order.TokenIn.Hex(), "scope", s.cfg.TokenPolicy.Scope())
		return nil
	}
	if err := s.reader.validateZeroGovernanceFee(ctx, s.cfg.InputSettler); err != nil {
		s.log.Error(err, "order skipped: governance fee invariant failed",
			"orderId", order.OrderID, "quoteId", order.QuoteID,
			"inputSettler", s.cfg.InputSettler.Hex())
		return nil
	}
	orderID, ok := s.openedOrderID(ctx, order)
	if !ok {
		return nil
	}
	reservationKey := orderID.Hex()
	if pending != nil && pending.contains(reservationKey) {
		s.log.V(1).Info("order skipped: already pending", "orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(), "quoteId", order.QuoteID)
		return nil
	}
	prepared := s.prepareFill(ctx, routes, order)
	if prepared == nil {
		return nil
	}
	plan, err := s.strategy.DecideFill(ctx, prepared.input)
	if err != nil {
		s.log.Error(err, "order fill: strategy", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return nil
	}
	if plan == nil {
		s.log.V(1).Info("order skipped: no immediate fill plan", "orderId", order.OrderID,
			"quoteId", order.QuoteID, "routes", len(prepared.input.Quotes))
		return nil
	}
	if err := validateFillPlan(prepared.input, plan); err != nil {
		s.log.Error(err, "order fill: reject strategy plan", "orderId", order.OrderID,
			"quoteId", order.QuoteID)
		return nil
	}
	calldata, err := buildFillCalldata(*order, orderID, plan, prepared.signedDiscounts)
	if err != nil {
		s.log.Error(err, "order fill: build calldata", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return nil
	}
	return s.submitFill(ctx, order, plan, calldata, prepared.input.MaxFeePerGas)
}

func validateFillPlan(input types.FillInput, plan *types.FillPlan) error {
	routes, err := liquidstrategies.ValidateFillRoutes(liquidstrategies.FillValidation{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn,
		RequiredAmountOut: input.OutputAmount, RequireSingleRoute: input.RequireSingleRoute,
		MaxRoutes: types.MaxRoutes, Quotes: input.Quotes, Reservations: input.Reservations,
		GasSnapshot: input.GasSnapshot, GasPrices: input.GasPrices, MaxFeePerGas: input.MaxFeePerGas,
		GasEnvelope: types.LiquidLaneGasEnvelope(),
	}, plan.Routes)
	if err != nil {
		return errors.Errorf("strategy returned invalid fill plan: %w", err)
	}
	plan.Routes = routes
	return nil
}

func (s *Solver) openedOrderID(ctx context.Context, order *submittedOrder) (common.Hash, bool) {
	orderID, err := s.reader.orderIdentifier(ctx, s.cfg.InputSettler, order.Order)
	if err != nil {
		s.log.Error(err, "order fill: identify order", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return common.Hash{}, false
	}
	status, err := s.reader.orderStatus(ctx, s.cfg.InputSettler, orderID)
	if err != nil {
		s.log.Error(err, "order fill: read initial order status", "orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(), "quoteId", order.QuoteID)
		return common.Hash{}, false
	}
	if status != lifiOrderStatusDeposited {
		s.log.Info("order skipped: on-chain order is not deposited", "orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(), "quoteId", order.QuoteID, "status", status)
		return common.Hash{}, false
	}
	return orderID, true
}

func (s *Solver) prepareFill(
	ctx context.Context,
	routes []route,
	order *submittedOrder,
) *preparedFill {
	pairRoutes := routesForPair(routes, order.TokenIn, order.TokenOut)
	if len(pairRoutes) == 0 {
		s.log.V(1).Info("order skipped: no configured route for pair", "orderId", order.OrderID,
			"quoteId", order.QuoteID, "tokenIn", order.TokenIn.Hex(), "tokenOut", order.TokenOut.Hex())
		return nil
	}
	state, err := s.loadFillState(ctx, pairRoutes, order)
	if err != nil {
		s.log.Error(err, "order fill: prepare current state", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return nil
	}
	if state == nil {
		return nil
	}
	maxFeePerGas, err := s.readMaxFeePerGas(ctx)
	if err != nil {
		s.log.Error(err, "order fill: read max fee per gas", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return nil
	}
	quotes := append([]liquidlane.FillQuote(nil), state.snapshots.Direct...)
	quotes = append(quotes, state.discountQuotes...)
	return &preparedFill{
		input: types.FillInput{
			OrderID:            order.OrderID,
			QuoteID:            order.QuoteID,
			Solver:             s.cfg.Executor,
			TokenIn:            order.TokenIn,
			TokenOut:           order.TokenOut,
			AmountIn:           order.AmountIn,
			OutputAmount:       order.OutputAmount,
			OutputContext:      order.Output.Context,
			Expires:            order.Order.Expires,
			FillDeadline:       order.Order.FillDeadline,
			RequireSingleRoute: s.cfg.TokenPolicy.RequiresSingleRoute(order.TokenIn),
			Quotes:             quotes,
			Reservations:       s.capacity.Snapshot(),
			GasSnapshot:        state.snapshots.GasSnapshot,
			GasPrices:          state.snapshots.GasPrices,
			MaxFeePerGas:       maxFeePerGas,
			ChainTime:          state.chainTime,
		},
		signedDiscounts: state.signedDiscounts,
	}
}

func (s *Solver) loadFillState(
	ctx context.Context,
	routes []route,
	order *submittedOrder,
) (*fillState, error) {
	snapshots, chainTime, err := s.readFillSnapshot(ctx, routes, order)
	if err != nil {
		return nil, err
	}
	if s.skipExpiredOrder(order, chainTime) {
		return nil, nil
	}
	state := &fillState{snapshots: snapshots, chainTime: chainTime}
	if s.discounts == nil || len(snapshots.Physical) == 0 {
		return state, nil
	}

	resolveCtx, cancel := context.WithTimeout(ctx, s.cfg.OrderServer.HTTPTimeout)
	state.discountQuotes, state.signedDiscounts = s.fillDiscountQuotes(
		resolveCtx, snapshots.Physical, chainTime,
	)
	cancel()

	state.snapshots, state.chainTime, err = s.readFillSnapshot(ctx, routes, order)
	if err != nil {
		return nil, errors.Errorf("refresh after private discount resolution: %w", err)
	}
	if s.skipExpiredOrder(order, state.chainTime) {
		return nil, nil
	}
	refreshedDiscountQuotes, discountIssues := discounts.RefreshFillQuotes(
		state.discountQuotes,
		state.signedDiscounts,
		state.snapshots.Physical,
		state.chainTime,
	)
	state.discountQuotes = refreshedDiscountQuotes
	s.logDiscountIssues(discountIssues)
	return state, nil
}

func (s *Solver) readFillSnapshot(
	ctx context.Context,
	routes []route,
	order *submittedOrder,
) (fillSnapshotSet, time.Time, error) {
	chainTime, err := s.now(ctx)
	if err != nil {
		return fillSnapshotSet{}, time.Time{}, errors.Errorf("read latest block time: %w", err)
	}
	snapshots, err := s.reader.fillSnapshots(ctx, routes, s.cfg.Executor, order.TokenIn, order.AmountIn, chainTime)
	if err != nil {
		return fillSnapshotSet{}, time.Time{}, errors.Errorf("read routes: %w", err)
	}
	return snapshots, chainTime, nil
}

func (s *Solver) skipExpiredOrder(order *submittedOrder, chainTime time.Time) bool {
	if !orderExpired(order, chainTime) {
		return false
	}
	s.log.Info("order skipped: expired", "orderId", order.OrderID, "quoteId", order.QuoteID,
		"chainTime", uint32Unix(chainTime), "expires", order.Order.Expires,
		"fillDeadline", order.Order.FillDeadline)
	return true
}

func routesForPair(routes []route, tokenIn, tokenOut common.Address) []route {
	out := make([]route, 0, len(routes))
	for _, candidate := range routes {
		if candidate.TokenIn == tokenIn && candidate.TokenOut == tokenOut {
			out = append(out, candidate)
		}
	}
	return out
}

func (s *Solver) readMaxFeePerGas(ctx context.Context) (*big.Int, error) {
	if s.maxFeePerGas == nil {
		return nil, errors.New("max fee per gas reader is not configured")
	}
	maxFee, err := s.maxFeePerGas(ctx)
	if err != nil {
		return nil, errors.Errorf("max fee per gas: %w", err)
	}
	if maxFee == nil || maxFee.Sign() <= 0 {
		return nil, errors.New("max fee per gas must be positive")
	}
	return new(big.Int).Set(maxFee), nil
}

func uint32Unix(t time.Time) uint32 {
	unix := t.Unix()
	if unix <= 0 {
		return 0
	}
	if unix > int64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(unix)
}

func orderExpired(order *submittedOrder, now time.Time) bool {
	chainTime := uint32Unix(now)
	if order.Order.Expires != 0 && chainTime >= order.Order.Expires {
		return true
	}
	return order.Order.FillDeadline != 0 && chainTime >= order.Order.FillDeadline
}
