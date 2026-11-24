package rfq

import (
	"context"
	"strings"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
)

// pollAwards runs on the same goroutine that polls receipts and submits fills.
// Quotes read the spend ledger under its own lock.
func (s *Solver) pollAwards(ctx context.Context, forSubmission bool) (_ []rfqbackend.OrdersResponseOrdersInner, _ []discounts.ListItem, err error) {
	rows, err := s.backend.orders(ctx, strings.ToLower(s.Workflow.Filler.Hex()))
	if err != nil {
		return nil, nil, err
	}
	polledAt := time.Now()
	if s.observeOrderPoll != nil {
		s.observeOrderPoll(polledAt)
	}
	defer func() {
		if err == nil {
			s.lastOrderPoll.Store(&polledAt)
		}
	}()
	open := make(map[string]bool, len(rows))
	var queued, unreserved []rfqbackend.OrdersResponseOrdersInner
	pending := s.Pending()
	for _, row := range rows {
		id := row.OrderId
		if id == "" || open[id] {
			continue
		}
		open[id] = true
		if pending != nil && id == pending.OrderID || s.Spends.Finalized(id) {
			continue
		}
		queued = append(queued, row)
		if !s.Spends.Queued(id) {
			unreserved = append(unreserved, row)
		}
	}
	s.Spends.PruneQueued(open)
	if len(queued) == 0 {
		return nil, nil, nil
	}

	observed, err := s.Observe(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !forSubmission && len(unreserved) == 0 {
		return nil, nil, nil
	}

	listed, err := s.backend.listDiscounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	if listed == nil {
		return nil, nil, errors.New("missing connector discount inventory")
	}
	for _, row := range unreserved {
		if err := s.reserveAward(ctx, row, listed.Discounts, observed); err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			observability.Log(ctx).V(1).Info("RFQ award not reserved yet", "orderId", row.OrderId, "reason", err.Error())
		}
	}
	return queued, listed.Discounts, nil
}

// reserveAward allocates the signed payout without preparing or simulating the
// transaction. Submission checks fresh terms and runs the complete simulation.
func (s *Solver) reserveAward(ctx context.Context, row rfqbackend.OrdersResponseOrdersInner, listed []discounts.ListItem, observed filler.Observation) error {
	order, _, err := decodeOrder(row, s.Workflow.Filler)
	if err != nil {
		return err
	}
	if !s.tokenPolicy.Allows(order.Request.TokenIn) || order.Request.Deadline.Int64() <= observed.Chain.Unix() {
		return nil
	}
	token, required, err := orderTerms(order, s.Workflow.Filler)
	if err != nil {
		return err
	}
	var routes []filler.Source
	amounts := make(liquidlane.CapacityReservations)
	chainID := s.Workflow.ChainID.Int64()
	for source, err := range s.Sources(listed, order.Request.TokenIn, token) {
		if err != nil {
			return err
		}
		if s.Unreserved(source, token, "").Cmp(required) >= 0 {
			amounts.Add(liquidlane.NewCapacityID(chainID, source.Account, token), required)
			return s.Spends.ReserveQueued(row.OrderId, connector.Spend{Amounts: amounts, Deadline: order.Request.Deadline.Int64()})
		}
		routes = append(routes, source)
	}
	if s.tokenPolicy.RequiresSingleRoute(order.Request.TokenIn) {
		return nil
	}
	legs, err := s.Split(ctx, routes, order.Request.AmountIn, required, len(routes), observed, "")
	if err != nil || len(legs) == 0 {
		return err
	}
	for _, leg := range legs {
		amounts.Add(liquidlane.NewCapacityID(chainID, leg.Account, token), leg.AmountOut)
	}
	return s.Spends.ReserveQueued(row.OrderId, connector.Spend{Amounts: amounts, Deadline: order.Request.Deadline.Int64()})
}
