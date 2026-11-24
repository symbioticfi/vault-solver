package rfq

import (
	"context"
	"strings"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
)

// The RFQ loop plans awards while the fill worker tracks its receipt.
func (s *Solver) pollAwards(ctx context.Context, forSubmission bool) (_ *filler.Intent, err error) {
	rows, err := s.backend.orders(ctx, strings.ToLower(s.Workflow.Filler.Hex()))
	if err != nil {
		return nil, err
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
		return nil, nil
	}

	observed, err := s.Observe(ctx)
	if err != nil {
		return nil, err
	}
	if !forSubmission && len(unreserved) == 0 {
		return nil, nil
	}

	listed, err := s.backend.listDiscounts(ctx)
	if err != nil {
		return nil, err
	}
	if listed == nil {
		return nil, errors.New("missing connector discount inventory")
	}
	for _, row := range unreserved {
		if _, err := s.planAward(ctx, row, listed.Discounts, observed, true); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			observability.Log(ctx).V(1).Info("RFQ award not reserved yet", "orderId", row.OrderId, "reason", err.Error())
		}
	}
	if !forSubmission {
		return nil, nil
	}
	for _, row := range queued {
		intent, err := s.planAward(ctx, row, listed.Discounts, observed, false)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			observability.Log(ctx).V(1).Info("RFQ award not executable", "orderId", row.OrderId, "reason", err.Error())
			continue
		}
		if intent != nil {
			return intent, nil
		}
	}
	return nil, nil
}
