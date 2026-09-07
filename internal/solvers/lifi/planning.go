package lifi

import (
	"context"
	"math"
	"math/big"
	"slices"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

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
	outcome              orderProcessingOutcome
}

type orderProcessingOutcome string

const (
	orderProcessingSubmitted        orderProcessingOutcome = "submitted"
	orderProcessingDepositDeferred  orderProcessingOutcome = "deposit_deferred"
	orderProcessingCapacityDeferred orderProcessingOutcome = "capacity_deferred"
	orderProcessingCapacityDropped  orderProcessingOutcome = "capacity_dropped"
	orderProcessingNotActionable    orderProcessingOutcome = "not_actionable"
	orderProcessingStrategyDeclined orderProcessingOutcome = "strategy_declined"
	orderProcessingInvalidPlan      orderProcessingOutcome = "invalid_plan"
	orderProcessingRetryableError   orderProcessingOutcome = "retryable_error"
	orderProcessingOther            orderProcessingOutcome = "other"
)

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

// processOrderUsingReservations owns a single decision from admission checks to
// txmanager acceptance. The order worker alone consumes the returned disposition.
func (s *Solver) processOrderUsingReservations(ctx context.Context, routes []route, order *submittedOrder,
	pending map[string]bool, override *liquidlane.CapacityReservations,
) orderProcessingResult {
	log := s.orderLogger(order, common.Hash{})
	if !s.cfg.TokenPolicy.Allows(order.TokenIn) {
		log.V(1).Info("order skipped: input token out of scope", "tokenIn", order.TokenIn.Hex(), "scope", s.cfg.TokenPolicy.Scope())
		return orderProcessingResult{outcome: orderProcessingNotActionable}
	}
	if err := s.reader.validateZeroGovernanceFee(ctx, s.cfg.InputSettler); err != nil {
		log.Error(err, "order skipped: governance fee invariant failed", "inputSettler", s.cfg.InputSettler.Hex())
		return classifyOrderFailure(err)
	}
	id, err := s.openedOrderID(ctx, order)
	if err != nil {
		return classifyOrderFailure(err)
	}
	log = s.orderLogger(order, id)
	if pending[id.Hex()] {
		log.V(1).Info("order skipped: already pending")
		return orderProcessingResult{outcome: orderProcessingNotActionable}
	}
	log.V(1).Info("order fill planning started", "tokenIn", order.TokenIn.Hex(), "tokenOut", order.TokenOut.Hex(),
		"amountIn", bigString(order.AmountIn), "requiredAmountOut", bigString(order.OutputAmount))
	prepared, err := s.prepareFill(ctx, routes, order, id, override)
	if err != nil {
		log.Error(err, "order fill: prepare current state")
		return classifyOrderFailure(err)
	}
	if prepared == nil {
		return orderProcessingResult{outcome: orderProcessingNotActionable}
	}
	plan, err := s.strategy.DecideFill(ctx, prepared.input)
	if err != nil {
		log.Error(err, "order fill: strategy")
		if types.IsPermanentFillDecisionError(err) {
			return orderProcessingResult{outcome: orderProcessingStrategyDeclined}
		}
		return orderProcessingResult{outcome: orderProcessingRetryableError, retryable: true, recoveryAttemptLimit: maximumStrategyRecoveryAttempts}
	}
	if plan == nil {
		log.V(1).Info("order fill strategy declined", "fillQuotes", len(prepared.input.Quotes),
			"reservationDomains", len(prepared.input.Reservations), "amountIn", bigString(order.AmountIn),
			"requiredAmountOut", bigString(order.OutputAmount))
		return s.probeReservedFill(ctx, prepared.input, log)
	}
	if err := validateFillPlan(prepared.input, plan); err != nil {
		log.Error(err, "order fill: reject strategy plan")
		return orderProcessingResult{outcome: orderProcessingInvalidPlan}
	}
	logFillPlan(log, plan)
	calldata, err := buildFillCalldata(*order, id, plan, prepared.signedDiscounts)
	if err != nil {
		log.Error(err, "order fill: build calldata")
		return orderProcessingResult{outcome: orderProcessingInvalidPlan}
	}
	fill, err := s.submitFill(ctx, order, plan, calldata, prepared.transactionMaxFeePerGas,
		prepared.input.ChainTime, prepared.chainObservedAt)
	if err != nil {
		result := classifyOrderFailure(err)
		if result.retryable {
			log.Error(err, "order fill: submit transaction")
		}
		return result
	}
	if fill == nil {
		return orderProcessingResult{outcome: orderProcessingNotActionable}
	}
	if override == nil {
		s.requestQuoteRefresh()
	}
	return orderProcessingResult{outcome: orderProcessingSubmitted, fill: fill}
}

// The same on-chain states may be observed during initial identification and the
// final pre-send check. Both boundaries must produce the same retry disposition.
func classifyOrderFailure(err error) orderProcessingResult {
	switch {
	case errors.Is(err, errOrderDepositNotVisible):
		return orderProcessingResult{outcome: orderProcessingDepositDeferred, depositNotVisible: true}
	case errors.Is(err, errOrderNotFillable):
		return orderProcessingResult{outcome: orderProcessingNotActionable}
	default:
		return orderProcessingResult{outcome: orderProcessingRetryableError, retryable: true}
	}
}

func (s *Solver) probeReservedFill(ctx context.Context, input types.FillInput, log logr.Logger) orderProcessingResult {
	declined := orderProcessingResult{outcome: orderProcessingStrategyDeclined}
	prober, supported := s.strategy.(reservationRetryProber)
	if !supported || len(input.Reservations) == 0 {
		return declined
	}
	reservations := input.Reservations
	input.Reservations = nil
	input.Trace = planning.NewDecisionTrace(log, "reservationProbe", true)
	plan, err := prober.DecideFillWithoutReservations(ctx, input)
	if err != nil {
		log.Error(err, "order fill: strategy without pending reservations")
		return declined
	}
	if plan == nil {
		return declined
	}
	if err := validateFillPlan(input, plan); err != nil {
		log.Error(err, "order fill: reject strategy plan without pending reservations")
		return orderProcessingResult{outcome: orderProcessingInvalidPlan}
	}
	blocked := blockedPlanCapacityIDs(reservations, plan)
	if len(blocked) == 0 {
		return declined
	}
	return orderProcessingResult{outcome: orderProcessingCapacityDeferred, blockedOn: blocked}
}

func blockedPlanCapacityIDs(reservations liquidlane.CapacityReservations, plan *types.FillPlan) map[liquidlane.CapacityID]bool {
	var blocked map[liquidlane.CapacityID]bool
	for _, leg := range plan.Routes {
		reserved := reservations[leg.CapacityID]
		if reserved == nil || reserved.Sign() <= 0 {
			continue
		}
		if blocked == nil {
			blocked = make(map[liquidlane.CapacityID]bool)
		}
		blocked[leg.CapacityID] = true
	}
	return blocked
}

func validateFillPlan(input types.FillInput, plan *types.FillPlan) error {
	facts := planning.FillValidation{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn, RequiredAmountOut: input.OutputAmount,
		RequireSingleRoute: input.RequireSingleRoute, MaxRoutes: types.MaxRoutes,
		Quotes: input.Quotes, Reservations: input.Reservations,
		GasSnapshot: input.GasSnapshot, GasPrices: input.GasPrices, MaxFeePerGas: input.MaxFeePerGas,
		GasEnvelope: types.LiquidLaneGasEnvelope(),
	}
	canonical, err := planning.ValidateFillRoutes(facts, plan.Routes)
	if err != nil {
		return errors.Errorf("strategy returned invalid fill plan: %w", err)
	}
	plan.Routes = canonical
	return nil
}

func (s *Solver) orderLogger(order *submittedOrder, id common.Hash) logr.Logger {
	log := s.log.WithValues("orderId", order.OrderID, "quoteId", order.QuoteID)
	if id != (common.Hash{}) {
		log = log.WithValues("onChainOrderId", id.Hex())
	}
	return log
}

func logFillPlan(log logr.Logger, plan *types.FillPlan) {
	private := 0
	for i, route := range plan.Routes {
		legLog := log.WithValues("route", i, "routeId", route.RouteID, "adapter", route.Adapter.Hex(),
			"capacityId", route.CapacityID, "private", route.DiscountID != nil)
		if route.DiscountID != nil {
			private++
			legLog = legLog.WithValues("discountId", route.DiscountID.Hex())
		}
		legLog.V(1).Info("order fill route selected", "amountIn", bigString(route.AmountIn),
			"expectedAmountOut", bigString(route.ExpectedAmountOut), "minAmountOut", bigString(route.MinAmountOut),
			"reservedAmountOut", bigString(route.ReservedAmountOut))
	}
	log.V(1).Info("order fill plan selected", "routes", len(plan.Routes), "discountRoutes", private)
}

func (s *Solver) openedOrderID(ctx context.Context, order *submittedOrder) (common.Hash, error) {
	id, err := s.reader.orderIdentifier(ctx, s.cfg.InputSettler, order.Order)
	if err != nil {
		s.orderLogger(order, id).Error(err, "order fill: identify order")
		return common.Hash{}, err
	}
	if err := s.requireDeposited(ctx, order, id, "initial"); err != nil {
		return common.Hash{}, err
	}
	return id, nil
}

// Shared by identification and submission, so a disappeared deposit never takes
// a different retry path merely because planning finished between the reads.
func (s *Solver) requireDeposited(ctx context.Context, order *submittedOrder, id common.Hash, phase string) error {
	status, err := s.reader.orderStatus(ctx, s.cfg.InputSettler, id)
	log := s.orderLogger(order, id).WithValues("phase", phase)
	if err != nil {
		log.Error(err, "order fill: read order status")
		return errors.Errorf("read order status for %s: %w", id.Hex(), err)
	}
	switch status {
	case lifiOrderStatusDeposited:
		return nil
	case lifiOrderStatusNone:
		log.Info("on-chain order deposit is not visible yet", "status", status)
		return errOrderDepositNotVisible
	default:
		log.Info("order skipped: on-chain order is no longer fillable", "status", status)
		return errOrderNotFillable
	}
}

// prepareFill builds one immutable decision input. Resolving private terms is
// followed by a new chain observation; timestamps are anchored before the RPCs.
func (s *Solver) prepareFill(ctx context.Context, routes []route, order *submittedOrder, id common.Hash,
	override *liquidlane.CapacityReservations,
) (*preparedFill, error) {
	pair := routesForPair(routes, order.TokenIn, order.TokenOut)
	log := s.orderLogger(order, id)
	if len(pair) == 0 {
		log.V(1).Info("order skipped: no configured route for pair", "tokenIn", order.TokenIn.Hex(), "tokenOut", order.TokenOut.Hex())
		return nil, nil
	}
	observation, err := s.observeFillState(ctx, pair, order)
	if err != nil {
		return nil, err
	}
	if s.skipExpiredOrder(order, observation.chainTime) {
		return nil, nil
	}
	var signed map[common.Hash]*discounts.Signed
	var private []liquidlane.FillQuote
	if s.discounts != nil && len(observation.snapshots.Physical) > 0 {
		resolveCtx, cancel := context.WithTimeout(ctx, s.cfg.OrderServer.HTTPTimeout)
		private, signed = s.fillDiscountQuotes(resolveCtx, observation.snapshots.Physical, observation.chainTime)
		cancel()
		observation, err = s.observeFillState(ctx, pair, order)
		if err != nil {
			return nil, errors.Errorf("refresh after private discount resolution: %w", err)
		}
		if s.skipExpiredOrder(order, observation.chainTime) {
			return nil, nil
		}
		var issues []discounts.OfferIssue
		private, issues = discounts.RefreshFillQuotes(private, signed, observation.snapshots.Physical, observation.chainTime)
		s.logDiscountIssues(issues)
	}
	prepared := &preparedFill{signedDiscounts: signed, chainObservedAt: observation.observedAt}
	pricingFee := new(big.Int)
	if s.cfg.Gas != nil {
		pricingFee, err = s.readMaxFeePerGas(ctx)
		if err != nil {
			return nil, err
		}
		prepared.transactionMaxFeePerGas = bigmath.Clone(pricingFee)
	}
	reservations := s.capacity.Snapshot()
	if override != nil {
		reservations = *override
	}
	quotes := append(append([]liquidlane.FillQuote(nil), observation.snapshots.Direct...), private...)
	prepared.input = types.FillInput{
		FillInput: planning.FillInput{
			TokenIn: order.TokenIn, TokenOut: order.TokenOut, AmountIn: bigmath.Clone(order.AmountIn), OutputAmount: bigmath.Clone(order.OutputAmount),
			RequireSingleRoute: s.cfg.TokenPolicy.RequiresSingleRoute(order.TokenIn),
			Quotes:             quotes,
			Reservations:       reservations,
			GasSnapshot:        observation.snapshots.GasSnapshot,
			GasPrices:          observation.snapshots.GasPrices,
			MaxFeePerGas:       pricingFee, ChainTime: observation.chainTime,
			Trace: planning.NewDecisionTrace(log),
		},

		OrderID: order.OrderID, QuoteID: order.QuoteID, Solver: s.cfg.Executor,

		OutputContext: append([]byte(nil), order.Output.Context...),
		Expires:       order.Order.Expires, FillDeadline: order.Order.FillDeadline,
	}
	log.V(1).Info("order fill snapshot loaded", "routes", len(pair), "fillQuotes", len(quotes),
		"directQuotes", len(observation.snapshots.Direct), "physicalQuotes", len(observation.snapshots.Physical),
		"discountQuotes", len(private), "reservationDomains", len(reservations), "pendingFills", s.capacity.Len(),
		"gasAccounting", s.cfg.Gas != nil, "pricingMaxFeePerGas", pricingFee.String(), "chainTime", observation.chainTime.Unix())
	return prepared, nil
}

type fillObservation struct {
	snapshots             fillSnapshotSet
	chainTime, observedAt time.Time
}

func (s *Solver) observeFillState(ctx context.Context, routes []route, order *submittedOrder) (fillObservation, error) {
	observation := fillObservation{observedAt: s.wallNow()}
	at, err := s.now(ctx)
	if err != nil {
		return fillObservation{}, errors.Errorf("read latest block time: %w", err)
	}
	observation.chainTime = at
	state, err := s.reader.Fill(ctx, routes, s.cfg.Executor, order.TokenIn, order.AmountIn, at)
	if err != nil {
		return fillObservation{}, errors.Errorf("read routes: %w", err)
	}
	observation.snapshots = state
	return observation, nil
}

func (s *Solver) skipExpiredOrder(order *submittedOrder, at time.Time) bool {
	if orderExpired(order, at) {
		s.orderLogger(order, common.Hash{}).Info("order skipped: expired", "chainTime", uint32Unix(at),
			"expires", order.Order.Expires, "fillDeadline", order.Order.FillDeadline)
		return true
	}
	return false
}

func routesForPair(routes []route, input, output common.Address) []route {
	return slices.Collect(func(yield func(route) bool) {
		for _, item := range routes {
			if item.TokenIn == input && item.TokenOut == output && !yield(item) {
				return
			}
		}
	})
}

func (s *Solver) readMaxFeePerGas(ctx context.Context) (*big.Int, error) {
	if s.maxFeePerGas == nil {
		return nil, errors.New("max fee per gas reader is not configured")
	}
	fee, err := s.maxFeePerGas(ctx)
	switch {
	case err != nil:
		return nil, errors.Errorf("max fee per gas: %w", err)
	case fee == nil || fee.Sign() <= 0:
		return nil, errors.New("max fee per gas must be positive")
	default:
		return bigmath.Clone(fee), nil
	}
}

func uint32Unix(at time.Time) uint32 { return uint32(min(max(at.Unix(), 0), int64(math.MaxUint32))) }

func orderExpired(order *submittedOrder, now time.Time) bool {
	if deadline := orderDeadline(order); !deadline.IsZero() {
		return now.Compare(deadline) >= 0
	}
	return false
}
