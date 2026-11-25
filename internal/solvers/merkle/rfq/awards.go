package rfq

import (
	"context"
	"maps"
	"strings"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
)

// The RFQ loop plans awards while the fill worker tracks its receipt.
func (s *Solver) pollAwards(ctx context.Context, forSubmission bool) (*filler.Intent, error) {
	return s.pollAwardsWithin(ctx, forSubmission, filler.OperationTimeout)
}

func (s *Solver) pollAwardsWithin(ctx context.Context, forSubmission bool, budget time.Duration) (*filler.Intent, error) {
	refresh, cancelRefresh := context.WithTimeout(ctx, budget)
	defer cancelRefresh()
	rows, err := s.backend.orders(refresh, strings.ToLower(s.Workflow.Filler.Hex()))
	if err != nil {
		return nil, err
	}
	polledAt := time.Now()
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
	maps.DeleteFunc(s.skipped, func(pass awardPass, _ string) bool { return !open[pass.orderID] })
	if len(queued) == 0 {
		return nil, s.markAwardRefresh(refresh, polledAt)
	}

	observed, err := s.Observe(refresh)
	if err != nil {
		return nil, err
	}
	var listed *discounts.List
	var refreshErr error
	if len(unreserved) != 0 {
		listed, refreshErr = s.awardDiscounts(refresh)
		if refreshErr == nil {
			_, refreshErr = s.planAwards(refresh, unreserved, listed.Discounts, observed, true)
		}
	}
	if refreshErr == nil {
		refreshErr = s.markAwardRefresh(refresh, polledAt)
	}
	cancelRefresh()
	if !forSubmission || ctx.Err() != nil {
		return nil, errors.Join(refreshErr, ctx.Err())
	}
	prepare, cancelPrepare := context.WithTimeout(ctx, budget)
	defer cancelPrepare()
	if listed == nil {
		listed, err = s.awardDiscounts(prepare)
		if err != nil {
			return nil, errors.Join(refreshErr, err)
		}
	}
	intent, prepareErr := s.planAwards(prepare, queued, listed.Discounts, observed, false)
	return intent, errors.Join(refreshErr, prepareErr)
}

func (s *Solver) awardDiscounts(ctx context.Context) (*discounts.List, error) {
	listed, err := s.backend.listDiscounts(ctx)
	if err != nil {
		return nil, err
	}
	if listed == nil {
		return nil, errors.New("missing connector discount inventory")
	}
	return listed, nil
}

func (s *Solver) markAwardRefresh(ctx context.Context, polledAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.lastAwardRefresh.Store(&polledAt)
	if s.observeOrderPoll != nil {
		s.observeOrderPoll(polledAt)
	}
	return nil
}

// logSkip says why an awarded order was left unfilled at Info, which a skipped award always
// deserves, but only when its reason changes: every poll repeats it otherwise.
func (s *Solver) logSkip(ctx context.Context, pass awardPass, reason string) {
	log := observability.Log(ctx)
	if s.skipped[pass] == reason {
		log = log.V(1)
	} else {
		if s.skipped == nil {
			s.skipped = make(map[awardPass]string)
		}
		s.skipped[pass] = reason
	}
	log.Info("RFQ award skipped", "orderId", pass.orderID, "reserve", pass.reserve, "reason", reason)
}

func (s *Solver) planAwards(ctx context.Context, rows []rfqbackend.OrdersResponseOrdersInner, listed []discounts.ListItem, observed filler.Observation, reserve bool) (*filler.Intent, error) {
	price := s.prices.reader(observed.BlockHash, func(ctx context.Context, offers []discounts.ListItem) ([]liquidlane.Inventory, error, error) {
		return s.InventoryAt(ctx, offers, observed.BlockNumber)
	})
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		intent, err := s.planAward(ctx, row, listed, observed, reserve, price)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			s.logSkip(ctx, awardPass{row.OrderId, reserve}, err.Error())
			continue
		}
		delete(s.skipped, awardPass{row.OrderId, reserve})
		if intent != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return intent, nil
		}
	}
	return nil, ctx.Err()
}
