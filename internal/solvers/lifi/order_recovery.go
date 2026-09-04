package lifi

import (
	"context"
	"time"

	"github.com/go-errors/errors"
)

func (s *Solver) recoverOrdersUntilSuccess(
	ctx context.Context,
	orders *orderBook,
) bool {
	backoff := initialOrderRecoveryBackoff
	recovered := make(map[string]struct{})
	successfulSweeps := 0
	for {
		listed, discovered, err := s.recoverOrders(ctx, orders, recovered)
		if err == nil {
			successfulSweeps++
			s.log.V(1).Info(
				"order recovery sweep completed",
				"sweep", successfulSweeps,
				"listedOrders", listed,
				"discoveredOrders", discovered,
				"seenOrders", len(recovered),
			)
			if discovered == 0 && orders.tryEndRecovery() {
				s.log.Info("order recovery completed", "listedOrders", listed, "seenOrders", len(recovered))
				return true
			}
			if successfulSweeps < maximumOrderRecoverySweeps {
				continue
			}
			err = errors.Errorf("order recovery did not converge after %d sweeps", successfulSweeps)
		}
		successfulSweeps = 0
		if ctx.Err() != nil {
			return false
		}
		recovered = make(map[string]struct{})
		s.log.Error(err, "order recovery failed; retrying", "backoff", backoff.String())
		if !waitForRetry(ctx, backoff) {
			return false
		}
		backoff = min(2*backoff, maximumOrderRecoveryBackoff)
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *Solver) recoverOrders(
	ctx context.Context,
	orders *orderBook,
	recovered map[string]struct{},
) (listed, discovered int, err error) {
	rawOrders, err := s.orders.listRecoverableOrders(ctx, s.cfg.Executor)
	if err != nil {
		return 0, 0, err
	}
	listed = len(rawOrders)
	for _, order := range orders.takeRecoveryRetries() {
		key := orderKey(order)
		delete(recovered, key)
		if err := orders.enqueueWait(ctx, order); err != nil {
			return 0, 0, errors.Errorf("re-enqueue recovery retry: %w", err)
		}
		if key != "" {
			recovered[key] = struct{}{}
			discovered++
		}
	}
	for _, raw := range rawOrders {
		if ctx.Err() != nil {
			return 0, 0, ctx.Err()
		}
		order := s.parseOrderMessage(orderMessage{Event: orderSubmitEvent, Data: raw})
		if order == nil {
			continue
		}
		key := orderKey(order)
		if key != "" {
			if _, exists := recovered[key]; exists {
				continue
			}
		}
		if err := orders.enqueueWait(ctx, order); err != nil {
			return 0, 0, errors.Errorf("enqueue recovered order: %w", err)
		}
		if key != "" {
			recovered[key] = struct{}{}
			discovered++
		}
	}
	if err = orders.waitUntilProcessed(ctx); err != nil {
		return 0, 0, errors.Errorf("wait for recovered orders: %w", err)
	}
	return listed, discovered, nil
}
