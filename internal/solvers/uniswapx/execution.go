package uniswapx

import (
	"context"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	uxexecutor "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

var (
	errOrderNotFillable = errors.New("order is not fillable at current chain state")
	errFillPreflight    = errors.New("fill preflight failed")
)

type pendingUniswapFill struct {
	order          *resolvedOrder
	plannedSurplus *big.Int
	result         <-chan txmanager.Result
}

type uniswapFillCompletion struct {
	fill   *pendingUniswapFill
	result txmanager.Result
}

func (s *Solver) fillLoop(
	ctx context.Context,
	routes []liquidlane.Route,
	orders <-chan *resolvedOrder,
) error {
	completions := make(chan uniswapFillCompletion, orderQueueCapacity)
	pending := make(map[common.Hash]*pendingUniswapFill)
	for orders != nil || len(pending) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case completion := <-completions:
			delete(pending, completion.fill.order.Hash)
			s.completePendingFill(completion)
		case order, ok := <-orders:
			if !ok {
				orders = nil
				continue
			}
			s.log.V(1).Info(
				"order fill planning started",
				"source", order.Source,
				"orderHash", order.Hash.Hex(),
				"quoteId", order.QuoteID,
			)
			now, err := s.reader.latestBlockTime(ctx)
			if err != nil {
				s.endFillPlanning()
				s.retry(order.Hash, time.Now(), false)
				s.log.Error(err, "order fill: read current chain time", "orderHash", order.Hash.Hex())
				continue
			}
			fill, err := s.startFill(ctx, routes, order, now)
			s.endFillPlanning()
			if err != nil {
				s.retry(order.Hash, now, errors.Is(err, errFillPreflight))
				if errors.Is(err, errFillPreflight) {
					s.recordOrderFillFailure(order, now)
				}
				if errors.Is(err, errOrderNotFillable) {
					s.log.V(1).Info("order not fillable yet", "source", order.Source,
						"orderHash", order.Hash.Hex(), "quoteId", order.QuoteID)
					continue
				}
				s.log.Error(err, "order fill preparation failed", "orderHash", order.Hash.Hex(), "quoteId", order.QuoteID)
				continue
			}
			pending[order.Hash] = fill
			go awaitUniswapFill(ctx, fill, completions)
		}
	}
	return nil
}

func awaitUniswapFill(
	ctx context.Context,
	fill *pendingUniswapFill,
	out chan<- uniswapFillCompletion,
) {
	select {
	case result, ok := <-fill.result:
		if !ok {
			result.Err = errors.New("transaction result channel closed without a result")
		}
		select {
		case out <- uniswapFillCompletion{fill: fill, result: result}:
		case <-ctx.Done():
		}
	case <-ctx.Done():
	}
}

func (s *Solver) startFill(
	ctx context.Context,
	routes []liquidlane.Route,
	order *resolvedOrder,
	now time.Time,
) (*pendingUniswapFill, error) {
	if order.TokenOut == (common.Address{}) {
		return nil, errOrderNotFillable
	}
	if order.Deadline == 0 || int64(order.Deadline) <= now.Unix() {
		return nil, errOrderNotFillable
	}
	decisionRoutes, listed, discountErr := s.fillRoutesWithDiscounts(
		ctx,
		routes,
		order.TokenIn,
		order.TokenOut,
		now,
	)
	if discountErr != nil {
		s.log.Error(discountErr, "refresh fill discount routes", "orderHash", order.Hash.Hex())
	}
	snapshot, err := s.reader.fillSnapshot(
		ctx,
		decisionRoutes,
		order.Executor,
		order.TokenIn,
		order.AmountIn,
		now,
	)
	if err != nil {
		return nil, err
	}
	if s.cfg.usesDiscounts() {
		snapshot.Direct = directFillQuotesForAdapters(snapshot.Direct, s.cfg.Adapters)
		if listed != nil {
			snapshot.Direct = append(snapshot.Direct, s.discountFillQuotes(listed, snapshot.Physical, now)...)
		}
	}
	s.log.V(1).Info(
		"order fill snapshot loaded",
		"source", order.Source,
		"orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID,
		"routes", len(decisionRoutes),
		"fillQuotes", len(snapshot.Direct),
		"physicalQuotes", len(snapshot.Physical),
	)
	maxFee, err := s.txm.MaxFeePerGas(ctx)
	if err != nil {
		return nil, err
	}
	pricingMaxFee := maxFee
	if s.cfg.Gas == nil {
		pricingMaxFee = new(big.Int)
	}
	fillInput := strategytypes.FillInput{
		OrderID: order.Hash.Hex(), QuoteID: order.QuoteID,
		TokenIn: order.TokenIn, TokenOut: order.TokenOut, AmountIn: order.AmountIn, OutputAmount: order.AmountOut,
		Deadline:           order.Deadline,
		RequireSingleRoute: s.cfg.TokenPolicy.RequiresSingleRoute(order.TokenIn), Quotes: snapshot.Direct,
		Reservations: s.capacity.Snapshot(),
		GasSnapshot:  snapshot.GasSnapshot, GasPrices: snapshot.GasPrices, MaxFeePerGas: pricingMaxFee, ChainTime: now,
		Trace: s.decisionTrace(
			"source", order.Source,
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
		),
	}
	plan, err := s.strategy.DecideFill(ctx, fillInput)
	if err != nil {
		return nil, err
	}
	if plan == nil || len(plan.Routes) == 0 {
		s.log.V(1).Info(
			"order fill strategy declined",
			"source", order.Source,
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"fillQuotes", len(fillInput.Quotes),
			"amountIn", order.AmountIn.String(),
			"requiredAmountOut", order.AmountOut.String(),
		)
		return nil, errOrderNotFillable
	}
	validatedRoutes, err := liquidstrategies.ValidateFillRoutes(liquidstrategies.FillValidation{
		TokenIn: fillInput.TokenIn, TokenOut: fillInput.TokenOut, AmountIn: fillInput.AmountIn,
		RequiredAmountOut: fillInput.OutputAmount, RequireSingleRoute: fillInput.RequireSingleRoute,
		MaxRoutes: strategytypes.MaxRoutes, Quotes: fillInput.Quotes, Reservations: fillInput.Reservations,
		GasSnapshot: fillInput.GasSnapshot, GasPrices: fillInput.GasPrices, MaxFeePerGas: fillInput.MaxFeePerGas,
		GasEnvelope: strategytypes.LiquidLaneGasEnvelope(),
	}, plan.Routes)
	if err != nil {
		return nil, errors.Errorf("strategy returned invalid fill plan: %w", err)
	}
	plan.Routes = validatedRoutes
	s.logFillPlan(order, plan)
	reservations, ok := liquidstrategies.FillRouteReservations(plan.Routes)
	if !ok {
		return nil, errors.New("strategy returned invalid capacity reservations")
	}
	data, err := s.buildExecutorCalldata(ctx, order, plan, decisionRoutes, now)
	if err != nil {
		return nil, err
	}
	if _, err := s.chain.CallContract(ctx, ethereum.CallMsg{From: s.solverAddress, To: &order.Executor, Data: data}, nil); err != nil {
		return nil, errors.Errorf("%w: %v", errFillPreflight, err)
	}
	s.log.V(1).Info(
		"order fill preflight succeeded",
		"source", order.Source,
		"orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID,
		"executor", order.Executor.Hex(),
		"caller", s.solverAddress.Hex(),
		"calldataBytes", len(data),
		"maxFeePerGas", maxFee.String(),
		"deadline", order.Deadline,
		"deadlineRemaining", time.Unix(int64(order.Deadline), 0).Sub(now),
	)
	result, accepted := s.txm.SendAsync(ctx, txmanager.Request{
		To: order.Executor, Data: data, MaxFeePerGas: new(big.Int).Set(maxFee),
		Label: "uniswapx-fill",
	})
	if !accepted {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("transaction submission was not accepted")
	}
	s.setPendingReservations(order.Hash, reservations)
	s.log.V(1).Info(
		"order fill submitted",
		"source", order.Source,
		"orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID,
		"routes", len(plan.Routes),
		"reservationDomains", len(reservations),
		"maxFeePerGas", maxFee.String(),
	)
	return &pendingUniswapFill{
		order: order, plannedSurplus: liquidstrategies.PlannedSurplus(plan.Routes, order.AmountOut), result: result,
	}, nil
}

func (s *Solver) buildExecutorCalldata(
	ctx context.Context,
	order *resolvedOrder,
	plan *strategytypes.FillPlan,
	routes []liquidlane.Route,
	now time.Time,
) ([]byte, error) {
	fillRoutes := make([]uxexecutor.ILiquidLaneUniswapXExecutorFillRoute, 0, len(plan.Routes))
	discountRoutes := make([]uxexecutor.ILiquidLaneUniswapXExecutorDiscountRoute, 0, len(plan.Routes))
	for _, route := range plan.Routes {
		if route.DiscountID == nil {
			fillRoutes = append(fillRoutes, uxexecutor.ILiquidLaneUniswapXExecutorFillRoute{
				Adapter: route.Adapter, AmountIn: route.AmountIn, AmountOut: route.MinAmountOut,
			})
			continue
		}
		selectedRoute, ok := findRoute(routes, route.RouteID)
		if !ok {
			return nil, errors.Errorf("selected discount route %s is unavailable", route.RouteID)
		}
		s.log.V(1).Info(
			"selected discount route repricing",
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"discountId", route.DiscountID.Hex(),
			"routeId", route.RouteID,
			"adapter", route.Adapter.Hex(),
			"amountIn", route.AmountIn.String(),
		)
		physicalQuotes, err := s.reader.physicalFillQuotes(
			ctx,
			[]liquidlane.Route{selectedRoute},
			order.TokenIn,
			route.AmountIn,
		)
		if err != nil {
			return nil, errors.Errorf("reprice selected discount %s: %w", route.DiscountID.Hex(), err)
		}
		signed, err := s.resolveDiscount(ctx, liquiddiscounts.Selection{
			DiscountID:   *route.DiscountID,
			Adapter:      route.Adapter,
			TokenIn:      order.TokenIn,
			TokenOut:     order.TokenOut,
			AmountIn:     route.AmountIn,
			MinAmountOut: route.MinAmountOut,
		}, physicalQuotes, now)
		if err != nil {
			return nil, errors.Errorf("resolve selected discount %s: %w", route.DiscountID.Hex(), err)
		}
		s.log.V(1).Info(
			"selected discount resolved",
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"discountId", route.DiscountID.Hex(),
			"routeId", route.RouteID,
			"adapter", route.Adapter.Hex(),
			"amountIn", route.AmountIn.String(),
			"discountDeadline", signed.Terms.Deadline,
			"protocolDeadline", signed.ProtocolDeadline,
		)
		discountRoutes = append(discountRoutes, uxexecutor.ILiquidLaneUniswapXExecutorDiscountRoute{
			Adapter: route.Adapter, AmountIn: route.AmountIn,
			DiscountSwap: uxexecutor.ILiquidLaneAdapterDiscountSwap{
				Discount: uxexecutor.ILiquidLaneAdapterDiscount{
					TokenToRedeem: signed.Terms.TokenToRedeem,
					Discount:      signed.Terms.Discount,
					Signer:        signed.Terms.Signer,
					Protocol:      signed.Terms.Protocol,
					Nonce:         signed.Terms.Nonce,
					Deadline:      signed.Terms.Deadline,
				},
				SignerSignature: signed.SignerSignature, ProtocolDeadline: signed.ProtocolDeadline,
			},
			ProtocolSignature: signed.ProtocolSignature,
		})
	}
	return uniswapXExecutor.TryPackExecute(
		uxexecutor.UniswapXSignedOrder{Order: order.Encoded, Sig: order.Signature},
		uxexecutor.ILiquidLaneUniswapXExecutorFillCall{Routes: fillRoutes, DiscountRoutes: discountRoutes},
	)
}

func findRoute(routes []liquidlane.Route, id liquidlane.RouteID) (liquidlane.Route, bool) {
	for _, route := range routes {
		if route.ID == id {
			return route, true
		}
	}
	return liquidlane.Route{}, false
}

func (s *Solver) logFillPlan(order *resolvedOrder, plan *strategytypes.FillPlan) {
	discountRoutes := 0
	for index, route := range plan.Routes {
		if route.DiscountID != nil {
			discountRoutes++
		}
		fields := []any{
			"source", order.Source,
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"route", index,
			"routeId", route.RouteID,
			"adapter", route.Adapter.Hex(),
			"amountIn", route.AmountIn.String(),
			"expectedAmountOut", route.ExpectedAmountOut.String(),
			"minAmountOut", route.MinAmountOut.String(),
			"reservedAmountOut", route.ReservedAmountOut.String(),
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
		"source", order.Source,
		"orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID,
		"routes", len(plan.Routes),
		"discountRoutes", discountRoutes,
	)
}

func (s *Solver) completePendingFill(completion uniswapFillCompletion) {
	order := completion.fill.order
	now := time.Now()
	s.clearPendingReservations(order.Hash)
	outcome := completion.result.EffectiveOutcome()
	if outcome != txmanager.OutcomeConfirmed && outcome != txmanager.OutcomeIncludedUnconfirmed {
		err := completion.result.Err
		if err == nil {
			err = errors.Errorf("unknown transaction outcome %q", outcome)
		}
		s.retry(order.Hash, now, true)
		s.recordOrderFillFailure(order, now)
		s.observeFill("failed")
		s.log.Error(
			err,
			"order fill failed",
			"source", order.Source, "orderHash", order.Hash.Hex(), "quoteId", order.QuoteID,
			"tx", completion.result.Hash.Hex(),
		)
		return
	}
	if outcome == txmanager.OutcomeConfirmed {
		s.log.Info("order filled", "source", order.Source, "executor", order.Executor.Hex(),
			"orderHash", order.Hash.Hex(), "quoteId", order.QuoteID, "tx", completion.result.Hash.Hex())
	} else {
		s.log.Error(completion.result.Err, "order fill included but confirmation wait failed",
			"source", order.Source, "executor", order.Executor.Hex(),
			"orderHash", order.Hash.Hex(), "quoteId", order.QuoteID, "tx", completion.result.Hash.Hex())
	}
	s.recordFillSuccess()
	s.complete(order.Hash, now)
	s.observeFill("filled")
	if s.metrics != nil {
		s.metrics.fillAmounts.Observe(
			completion.result.Receipt,
			order.TokenIn,
			order.AmountIn,
			order.TokenOut,
			order.AmountOut,
			completion.fill.plannedSurplus,
		)
	}
}

func (s *Solver) recordOrderFillFailure(order *resolvedOrder, now time.Time) {
	// An exclusive attempt can legitimately lose to a timely soft override. Its tracked
	// obligation is classified from terminal API and canonical receipt state after the deadline.
	if order.Source != orderSourceExclusiveV2 {
		s.recordFillFailure(now)
	}
}
