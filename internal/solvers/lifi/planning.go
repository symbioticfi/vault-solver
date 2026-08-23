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
	fillSnapshotObservation

	discountQuotes  []liquidlane.FillQuote
	signedDiscounts map[common.Hash]*discounts.Signed
}

type fillSnapshotObservation struct {
	snapshots       fillSnapshotSet
	chainTime       time.Time
	chainObservedAt time.Time
}

type preparedFill struct {
	input                   types.FillInput
	signedDiscounts         map[common.Hash]*discounts.Signed
	transactionMaxFeePerGas *big.Int
	chainObservedAt         time.Time
}

type orderProcessingResult struct {
	fill                 *pendingFill
	blockedOn            map[liquidlane.CapacityID]bool
	depositNotVisible    bool
	retryable            bool
	recoveryAttemptLimit int
}

var (
	errOrderDepositNotVisible = errors.New("order deposit is not visible")
	errOrderNotFillable       = errors.New("order is no longer fillable")
)

type reservationRetryProber interface {
	DecideFillWithoutReservations(
		ctx context.Context,
		input types.FillInput,
	) (*types.FillPlan, error)
}

func (s *Solver) processOrderWithPending(
	ctx context.Context,
	routes []route,
	order *submittedOrder,
	pending *pendingFillState,
) orderProcessingResult {
	result := s.processOrderUsingReservations(ctx, routes, order, pending, nil)
	if result.fill != nil {
		s.requestQuoteRefresh()
	}
	return result
}

func (s *Solver) processOrderUsingReservations(
	ctx context.Context,
	routes []route,
	order *submittedOrder,
	pending *pendingFillState,
	reservations *liquidlane.CapacityReservations,
) orderProcessingResult {
	if !s.cfg.TokenPolicy.Allows(order.TokenIn) {
		s.log.V(1).Info("order skipped: input token out of scope",
			"orderId", order.OrderID, "quoteId", order.QuoteID,
			"tokenIn", order.TokenIn.Hex(), "scope", s.cfg.TokenPolicy.Scope())
		return orderProcessingResult{}
	}
	if err := s.reader.validateZeroGovernanceFee(ctx, s.cfg.InputSettler); err != nil {
		s.log.Error(err, "order skipped: governance fee invariant failed",
			"orderId", order.OrderID, "quoteId", order.QuoteID,
			"inputSettler", s.cfg.InputSettler.Hex())
		return orderProcessingResult{retryable: true}
	}
	orderID, err := s.openedOrderID(ctx, order)
	if err != nil {
		if errors.Is(err, errOrderDepositNotVisible) {
			return orderProcessingResult{depositNotVisible: true}
		}
		return orderProcessingResult{retryable: !errors.Is(err, errOrderNotFillable)}
	}
	reservationKey := orderID.Hex()
	if pending != nil && pending.contains(reservationKey) {
		s.log.V(1).Info("order skipped: already pending", "orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(), "quoteId", order.QuoteID)
		return orderProcessingResult{}
	}
	s.log.V(1).Info(
		"order fill planning started",
		"orderId", order.OrderID,
		"onChainOrderId", orderID.Hex(),
		"quoteId", order.QuoteID,
		"tokenIn", order.TokenIn.Hex(),
		"tokenOut", order.TokenOut.Hex(),
		"amountIn", bigString(order.AmountIn),
		"requiredAmountOut", bigString(order.OutputAmount),
	)
	prepared, err := s.prepareFill(ctx, routes, order, orderID, reservations)
	if err != nil {
		s.log.Error(err, "order fill: prepare current state", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return orderProcessingResult{retryable: true}
	}
	if prepared == nil {
		return orderProcessingResult{}
	}
	plan, err := s.strategy.DecideFill(ctx, prepared.input)
	if err != nil {
		s.log.Error(err, "order fill: strategy", "orderId", order.OrderID, "quoteId", order.QuoteID)
		if types.IsPermanentFillDecisionError(err) {
			return orderProcessingResult{}
		}
		return orderProcessingResult{
			retryable:            true,
			recoveryAttemptLimit: maximumStrategyRecoveryAttempts,
		}
	}
	if plan == nil {
		s.log.V(1).Info(
			"order fill strategy declined",
			"orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(),
			"quoteId", order.QuoteID,
			"fillQuotes", len(prepared.input.Quotes),
			"reservationDomains", len(prepared.input.Reservations),
			"amountIn", bigString(order.AmountIn),
			"requiredAmountOut", bigString(order.OutputAmount),
		)
		prober, probeOK := s.strategy.(reservationRetryProber)
		if !probeOK || len(prepared.input.Reservations) == 0 {
			return orderProcessingResult{}
		}
		unreservedInput := prepared.input
		unreservedInput.Reservations = nil
		unreservedInput.Trace = s.decisionTrace(
			"orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(),
			"quoteId", order.QuoteID,
			"reservationProbe", true,
		)
		unreservedPlan, err := prober.DecideFillWithoutReservations(ctx, unreservedInput)
		if err != nil {
			s.log.Error(err, "order fill: strategy without pending reservations",
				"orderId", order.OrderID, "quoteId", order.QuoteID)
			return orderProcessingResult{}
		}
		if unreservedPlan == nil {
			return orderProcessingResult{}
		}
		if err := validateFillPlan(unreservedInput, unreservedPlan); err != nil {
			s.log.Error(err, "order fill: reject strategy plan without pending reservations",
				"orderId", order.OrderID, "quoteId", order.QuoteID)
			return orderProcessingResult{}
		}
		return orderProcessingResult{
			blockedOn: blockedPlanCapacityIDs(prepared.input.Reservations, unreservedPlan),
		}
	}
	if err := validateFillPlan(prepared.input, plan); err != nil {
		s.log.Error(err, "order fill: reject strategy plan", "orderId", order.OrderID,
			"quoteId", order.QuoteID)
		return orderProcessingResult{}
	}
	s.logFillPlan(order, orderID, plan)
	calldata, err := buildFillCalldata(*order, orderID, plan, prepared.signedDiscounts)
	if err != nil {
		s.log.Error(err, "order fill: build calldata", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return orderProcessingResult{}
	}
	fill, err := s.submitFill(
		ctx,
		order,
		plan,
		calldata,
		prepared.transactionMaxFeePerGas,
		prepared.input.ChainTime,
		prepared.chainObservedAt,
	)
	if err != nil {
		if errors.Is(err, errOrderDepositNotVisible) {
			return orderProcessingResult{depositNotVisible: true}
		}
		if errors.Is(err, errOrderNotFillable) {
			return orderProcessingResult{}
		}
		s.log.Error(err, "order fill: submit transaction", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return orderProcessingResult{retryable: true}
	}
	return orderProcessingResult{fill: fill}
}

func blockedPlanCapacityIDs(
	reservations liquidlane.CapacityReservations,
	plan *types.FillPlan,
) map[liquidlane.CapacityID]bool {
	blocked := make(map[liquidlane.CapacityID]bool)
	for _, route := range plan.Routes {
		if reserved := reservations[route.CapacityID]; reserved != nil && reserved.Sign() > 0 {
			blocked[route.CapacityID] = true
		}
	}
	if len(blocked) == 0 {
		return nil
	}
	return blocked
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

func (s *Solver) logFillPlan(order *submittedOrder, orderID common.Hash, plan *types.FillPlan) {
	discountRoutes := 0
	for index, route := range plan.Routes {
		if route.DiscountID != nil {
			discountRoutes++
		}
		fields := []any{
			"orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(),
			"quoteId", order.QuoteID,
			"route", index,
			"routeId", route.RouteID,
			"adapter", route.Adapter.Hex(),
			"amountIn", bigString(route.AmountIn),
			"expectedAmountOut", bigString(route.ExpectedAmountOut),
			"minAmountOut", bigString(route.MinAmountOut),
			"reservedAmountOut", bigString(route.ReservedAmountOut),
			"capacityId", route.CapacityID,
			"private", route.DiscountID != nil,
		}
		if route.DiscountID != nil {
			fields = append(fields, "discountId", route.DiscountID.Hex())
		}
		s.log.V(1).Info("order fill route selected", fields...)
	}
	s.log.V(1).Info(
		"order fill plan selected",
		"orderId", order.OrderID,
		"onChainOrderId", orderID.Hex(),
		"quoteId", order.QuoteID,
		"routes", len(plan.Routes),
		"discountRoutes", discountRoutes,
	)
}

func (s *Solver) openedOrderID(ctx context.Context, order *submittedOrder) (common.Hash, error) {
	orderID, err := s.reader.orderIdentifier(ctx, s.cfg.InputSettler, order.Order)
	if err != nil {
		s.log.Error(err, "order fill: identify order", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return common.Hash{}, err
	}
	status, err := s.reader.orderStatus(ctx, s.cfg.InputSettler, orderID)
	if err != nil {
		s.log.Error(err, "order fill: read initial order status", "orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(), "quoteId", order.QuoteID)
		return common.Hash{}, err
	}
	if status == lifiOrderStatusNone {
		s.log.Info("on-chain order deposit is not visible yet", "orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(), "quoteId", order.QuoteID, "status", status)
		return common.Hash{}, errOrderDepositNotVisible
	}
	if status != lifiOrderStatusDeposited {
		s.log.Info("order skipped: on-chain order is no longer fillable", "orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(), "quoteId", order.QuoteID, "status", status)
		return common.Hash{}, errOrderNotFillable
	}
	return orderID, nil
}

func (s *Solver) prepareFill(
	ctx context.Context,
	routes []route,
	order *submittedOrder,
	orderID common.Hash,
	reservationOverride *liquidlane.CapacityReservations,
) (*preparedFill, error) {
	pairRoutes := routesForPair(routes, order.TokenIn, order.TokenOut)
	if len(pairRoutes) == 0 {
		s.log.V(1).Info("order skipped: no configured route for pair", "orderId", order.OrderID,
			"quoteId", order.QuoteID, "tokenIn", order.TokenIn.Hex(), "tokenOut", order.TokenOut.Hex())
		return nil, nil
	}
	state, err := s.loadFillState(ctx, pairRoutes, order)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	pricingMaxFeePerGas := new(big.Int)
	var transactionMaxFeePerGas *big.Int
	if s.cfg.Gas != nil {
		pricingMaxFeePerGas, err = s.readMaxFeePerGas(ctx)
		if err != nil {
			return nil, err
		}
		transactionMaxFeePerGas = liquidlane.CloneBig(pricingMaxFeePerGas)
	}
	reservations := s.capacity.Snapshot()
	if reservationOverride != nil {
		reservations = *reservationOverride
	}
	quotes := append([]liquidlane.FillQuote(nil), state.snapshots.Direct...)
	quotes = append(quotes, state.discountQuotes...)
	s.log.V(1).Info(
		"order fill snapshot loaded",
		"orderId", order.OrderID,
		"onChainOrderId", orderID.Hex(),
		"quoteId", order.QuoteID,
		"routes", len(pairRoutes),
		"fillQuotes", len(quotes),
		"directQuotes", len(state.snapshots.Direct),
		"physicalQuotes", len(state.snapshots.Physical),
		"discountQuotes", len(state.discountQuotes),
		"reservationDomains", len(reservations),
		"pendingFills", s.capacity.Len(),
		"gasAccounting", s.cfg.Gas != nil,
		"pricingMaxFeePerGas", pricingMaxFeePerGas.String(),
		"chainTime", state.chainTime.Unix(),
	)
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
			Reservations:       reservations,
			GasSnapshot:        state.snapshots.GasSnapshot,
			GasPrices:          state.snapshots.GasPrices,
			MaxFeePerGas:       pricingMaxFeePerGas,
			ChainTime:          state.chainTime,
			Trace: s.decisionTrace(
				"orderId", order.OrderID,
				"onChainOrderId", orderID.Hex(),
				"quoteId", order.QuoteID,
			),
		},
		signedDiscounts:         state.signedDiscounts,
		transactionMaxFeePerGas: transactionMaxFeePerGas,
		chainObservedAt:         state.chainObservedAt,
	}, nil
}

func (s *Solver) loadFillState(
	ctx context.Context,
	routes []route,
	order *submittedOrder,
) (*fillState, error) {
	observation, err := s.readFillSnapshot(ctx, routes, order)
	if err != nil {
		return nil, err
	}
	if s.skipExpiredOrder(order, observation.chainTime) {
		return nil, nil
	}
	state := &fillState{fillSnapshotObservation: observation}
	if s.discounts == nil || len(observation.snapshots.Physical) == 0 {
		return state, nil
	}

	resolveCtx, cancel := context.WithTimeout(ctx, s.cfg.OrderServer.HTTPTimeout)
	state.discountQuotes, state.signedDiscounts = s.fillDiscountQuotes(
		resolveCtx, observation.snapshots.Physical, observation.chainTime,
	)
	cancel()

	observation, err = s.readFillSnapshot(ctx, routes, order)
	if err != nil {
		return nil, errors.Errorf("refresh after private discount resolution: %w", err)
	}
	state.fillSnapshotObservation = observation
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
) (fillSnapshotObservation, error) {
	chainObservedAt := s.wallNow()
	chainTime, err := s.now(ctx)
	if err != nil {
		return fillSnapshotObservation{}, errors.Errorf("read latest block time: %w", err)
	}
	snapshots, err := s.reader.fillSnapshots(ctx, routes, s.cfg.Executor, order.TokenIn, order.AmountIn, chainTime)
	if err != nil {
		return fillSnapshotObservation{}, errors.Errorf("read routes: %w", err)
	}
	return fillSnapshotObservation{
		snapshots: snapshots, chainTime: chainTime, chainObservedAt: chainObservedAt,
	}, nil
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
	deadline := orderDeadline(order)
	return !deadline.IsZero() && !now.Before(deadline)
}
