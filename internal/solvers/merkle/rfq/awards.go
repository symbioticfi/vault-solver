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

type awardBatch struct {
	rows     []rfqbackend.OrdersResponseOrdersInner
	listings []discounts.ListItem
	observed filler.Observation
}

// The RFQ loop plans reservations and polls receipts. SDK submission runs
// independently, and quotes read the spend ledger under its own lock.
func (s *Solver) pollAwards(ctx context.Context, forSubmission bool) (batch awardBatch, err error) {
	rows, err := s.backend.orders(ctx, strings.ToLower(s.Workflow.Filler.Hex()))
	if err != nil {
		return batch, err
	}
	polledAt := time.Now()
	defer func() {
		if err == nil {
			s.lastAwardRefresh.Store(&polledAt)
			if s.observeOrderPoll != nil {
				s.observeOrderPoll(polledAt)
			}
		}
	}()
	open := make(map[string]bool, len(rows))
	var unreserved []rfqbackend.OrdersResponseOrdersInner
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
		batch.rows = append(batch.rows, row)
		if !s.Spends.Queued(id) {
			unreserved = append(unreserved, row)
		}
	}
	s.Spends.PruneQueued(open)
	if len(batch.rows) == 0 {
		return batch, nil
	}

	batch.observed, err = s.Observe(ctx)
	if err != nil {
		return batch, err
	}
	if !forSubmission && len(unreserved) == 0 {
		return batch, nil
	}

	listed, err := s.backend.listDiscounts(ctx)
	if err != nil {
		return batch, err
	}
	if listed == nil {
		return batch, errors.New("missing connector discount inventory")
	}
	batch.listings = listed.Discounts
	for _, row := range unreserved {
		if err := s.reserveAward(ctx, row, batch.listings, batch.observed); err != nil {
			if ctx.Err() != nil {
				return batch, ctx.Err()
			}
			observability.Log(ctx).V(1).Info("RFQ award not reserved yet", "orderId", row.OrderId, "reason", err.Error())
		}
	}
	return batch, nil
}

// reserveAward allocates the signed payout without preparing or simulating the
// transaction. Submission checks fresh terms and runs the complete simulation.
func (s *Solver) reserveAward(ctx context.Context, row rfqbackend.OrdersResponseOrdersInner, listed []discounts.ListItem, observed filler.Observation) error {
	order, _, err := decodeOrder(row, s.Workflow.Filler)
	if err != nil {
		return err
	}
	deadline := order.Request.Deadline.Int64()
	if !s.tokenPolicy.Allows(order.Request.TokenIn) || deadline <= observed.Chain.Unix() {
		return nil
	}
	token, required, err := orderTerms(order, s.Workflow.Filler)
	if err != nil {
		return err
	}
	var routes []filler.Source
	spend := connector.Spend{Amounts: make(liquidlane.CapacityReservations), Deadline: deadline}
	chainID := s.Workflow.ChainID.Int64()
	for source, err := range s.Sources(listed, order.Request.TokenIn, token) {
		if err != nil {
			return err
		}
		if s.Unreserved(source, token, "").Cmp(required) >= 0 {
			spend.Amounts.Add(liquidlane.NewCapacityID(chainID, source.Account, token), required)
			return s.Spends.ReserveQueued(row.OrderId, spend)
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
		spend.Amounts.Add(liquidlane.NewCapacityID(chainID, leg.Account, token), leg.AmountOut)
	}
	return s.Spends.ReserveQueued(row.OrderId, spend)
}
