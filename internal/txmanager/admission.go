package txmanager

import (
	"context"
	"math/big"
	"time"

	"github.com/go-errors/errors"
)

// Send performs pre-sign admission and transfers admission-demand ownership to the serial worker.
func (m *Manager) Send(ctx context.Context, request Request) Result {
	admissionStarted := time.Now()
	m.addAdmissionDemand()
	ownedByCaller := true
	defer func() {
		if ownedByCaller {
			m.releaseAdmissionDemand()
		}
	}()

	waitCtx, cancel := admissionContext(ctx, request)
	defer cancel()
	if err := waitCtx.Err(); err != nil {
		return m.admissionFailure(ctx, request, admissionStarted, err)
	}
	if m.isStopping() {
		return m.admissionFailure(ctx, request, admissionStarted, errManagerStopped)
	}
	if err := m.waitForNonceLane(waitCtx); err != nil {
		return m.admissionFailure(ctx, request, admissionStarted, err)
	}

	result := make(chan Result, 1)
	select {
	case m.queue <- job{req: cloneRequest(request), res: result, admissionStarted: admissionStarted}:
		ownedByCaller = false
		return <-result
	case <-waitCtx.Done():
		return m.admissionFailure(ctx, request, admissionStarted, waitCtx.Err())
	case <-m.stopping:
		return m.admissionFailure(ctx, request, admissionStarted, errManagerStopped)
	}
}

func admissionContext(parent context.Context, request Request) (context.Context, context.CancelFunc) {
	if request.CancelAt.IsZero() {
		return parent, func() {}
	}
	return context.WithDeadline(parent, request.CancelAt)
}

func (m *Manager) isStopping() bool {
	select {
	case <-m.stopping:
		return true
	default:
		return false
	}
}

func (m *Manager) waitForNonceLane(ctx context.Context) error {
	changes, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if m.isStopping() {
			return errManagerStopped
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

func (m *Manager) admissionFailure(
	ctx context.Context,
	request Request,
	started time.Time,
	cause error,
) Result {
	m.metrics.finishAdmission(request.Label, started, cause)
	if ctx.Err() != nil {
		return notAdmittedResult(ctx.Err())
	}
	return notAdmittedResult(errors.Errorf(
		"send %q before admission: %w",
		request.Label,
		cause,
	))
}

func notAdmittedResult(err error) Result {
	return Result{Outcome: OutcomeSubmissionError, Err: err, NotAdmitted: true}
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

func (m *Manager) MaxFeePerGas(ctx context.Context) (*big.Int, error) {
	limit := m.normalFeeLimit(Request{})
	fees, err := m.currentFees(ctx, reserveFeeBump(limit))
	if err != nil {
		return nil, err
	}
	maximum := bumpFee(fees.maxFee)
	if limit != nil && maximum.Cmp(limit) > 0 {
		maximum.Set(limit)
	}
	return maximum, nil
}
