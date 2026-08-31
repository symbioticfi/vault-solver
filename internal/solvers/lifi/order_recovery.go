package lifi

import (
	"context"
	"time"

	"github.com/go-errors/errors"
)

type orderRecoveryResult struct {
	listed       int
	discovered   int
	processedGen uint64
}

func (s *Solver) recoverOrdersUntilSuccess(
	ctx context.Context,
	inbox *orderInbox,
) bool {
	backoff := initialOrderRecoveryBackoff
	recovered := make(map[string]bool)
	successfulSweeps := 0
	for {
		result, err := s.recoverOrders(ctx, inbox, recovered)
		if err == nil {
			successfulSweeps++
			s.log.V(1).Info(
				"order recovery sweep completed",
				"sweep", successfulSweeps,
				"listedOrders", result.listed,
				"discoveredOrders", result.discovered,
				"seenOrders", len(recovered),
			)
			if result.discovered == 0 && inbox.tryEndRecovery(result.processedGen) {
				s.log.Info("order recovery completed", "listedOrders", result.listed, "seenOrders", len(recovered))
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
		recovered = make(map[string]bool)
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
	inbox *orderInbox,
	recovered map[string]bool,
) (orderRecoveryResult, error) {
	rawOrders, err := s.orders.listRecoverableOrders(ctx, s.cfg.Executor)
	if err != nil {
		return orderRecoveryResult{}, err
	}
	result := orderRecoveryResult{listed: len(rawOrders)}
	for _, order := range inbox.takeRecoveryRetries() {
		key := orderInboxKey(order)
		delete(recovered, key)
		if err := inbox.enqueueWait(ctx, order); err != nil {
			return orderRecoveryResult{}, errors.Errorf("re-enqueue recovery retry: %w", err)
		}
		if key != "" {
			recovered[key] = true
			result.discovered++
		}
	}
	for _, raw := range rawOrders {
		if ctx.Err() != nil {
			return orderRecoveryResult{}, ctx.Err()
		}
		order := s.parseOrderMessage(orderMessage{Event: orderSubmitEvent, Data: raw})
		if order == nil {
			continue
		}
		key := orderInboxKey(order)
		if key != "" && recovered[key] {
			continue
		}
		if err := inbox.enqueueWait(ctx, order); err != nil {
			return orderRecoveryResult{}, errors.Errorf("enqueue recovered order: %w", err)
		}
		if key != "" {
			recovered[key] = true
			result.discovered++
		}
	}
	result.processedGen, err = inbox.waitUntilProcessed(ctx)
	if err != nil {
		return orderRecoveryResult{}, errors.Errorf("wait for recovered orders: %w", err)
	}
	return result, nil
}
