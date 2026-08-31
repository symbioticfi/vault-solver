package txmanager

import (
	"context"
	"math/big"

	"github.com/go-errors/errors"
)

func (m *Manager) sendAsync(ctx context.Context, req Request) (<-chan Result, bool) {
	m.addAdmissionDemand()
	releaseDemandOnReturn := true
	defer func() {
		if releaseDemandOnReturn {
			m.releaseAdmissionDemand()
		}
	}()

	admissionCtx := ctx
	cancel := func() {}
	if !req.CancelAt.IsZero() {
		admissionCtx, cancel = context.WithDeadline(ctx, req.CancelAt)
	}
	defer cancel()
	if err := admissionCtx.Err(); err != nil {
		return admissionFailure(ctx, req, err)
	}
	select {
	case <-m.stopping:
		return admissionFailure(ctx, req, errManagerStopped)
	default:
	}
	if err := m.waitForNonceLane(admissionCtx); err != nil {
		return admissionFailure(ctx, req, err)
	}
	select {
	case m.lifecycleSlot <- struct{}{}:
	case <-admissionCtx.Done():
		return admissionFailure(ctx, req, admissionCtx.Err())
	case <-m.stopping:
		return admissionFailure(ctx, req, errManagerStopped)
	}
	if err := m.waitForNonceLane(admissionCtx); err != nil {
		<-m.lifecycleSlot
		return admissionFailure(ctx, req, err)
	}
	res := make(chan Result, 1)
	select {
	case m.queue <- job{req: cloneRequest(req), res: res}:
		releaseDemandOnReturn = false
	case <-admissionCtx.Done():
		m.releaseLifecycleSlot()
		releaseDemandOnReturn = false
		return admissionFailure(ctx, req, admissionCtx.Err())
	case <-m.stopping:
		m.releaseLifecycleSlot()
		releaseDemandOnReturn = false
		return admissionFailure(ctx, req, errManagerStopped)
	}
	return res, true
}

func (m *Manager) waitForNonceLane(ctx context.Context) error {
	changes, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-m.stopping:
			return errManagerStopped
		default:
		}
		if m.nonceConflictError() == nil {
			return nil
		}
		select {
		case <-changes:
		case <-ctx.Done():
			return ctx.Err()
		case <-m.stopping:
			return errManagerStopped
		}
	}
}

func admissionFailure(ctx context.Context, req Request, err error) (<-chan Result, bool) {
	if ctx.Err() != nil {
		return nil, false
	}
	res := make(chan Result, 1)
	res <- notAdmittedResult(errors.Errorf("send %q before admission: %w", req.Label, err))
	return res, true
}

func notAdmittedResult(err error) Result {
	return Result{Err: err, NotAdmitted: true}
}

func (m *Manager) releaseLifecycleSlot() {
	<-m.lifecycleSlot
	m.releaseAdmissionDemand()
}

func (m *Manager) addAdmissionDemand() {
	if m.admissionDemand.Add(1) == 1 {
		m.notifyLaneStateChange()
	}
}

func (m *Manager) releaseAdmissionDemand() {
	remaining := m.admissionDemand.Add(-1)
	if remaining < 0 {
		panic("txmanager: negative admission demand")
	}
	if remaining == 0 {
		m.notifyLaneStateChange()
	}
}

// MaxFeePerGas returns a profitability ceiling that includes one ordinary replacement when the
// configured limit permits it. Send recomputes the initial fees immediately before signing.
func (m *Manager) MaxFeePerGas(ctx context.Context) (*big.Int, error) {
	limit := m.normalFeeLimit(Request{})
	fees, err := m.currentFees(ctx, reserveFeeBump(limit))
	if err != nil {
		return nil, err
	}
	maxFee := bumpFee(fees.maxFee)
	if limit != nil && maxFee.Cmp(limit) > 0 {
		maxFee.Set(limit)
	}
	return maxFee, nil
}
