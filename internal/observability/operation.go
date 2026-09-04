package observability

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type ExternalOperationOutcome uint8

const (
	ExternalOperationSuccess ExternalOperationOutcome = iota + 1
	ExternalOperationDegraded
	ExternalOperationSkipped
	ExternalOperationError
)

var externalOperationOutcomeLabels = [...]string{
	ExternalOperationSuccess:  "success",
	ExternalOperationDegraded: "degraded",
	ExternalOperationSkipped:  "skipped",
	ExternalOperationError:    "error",
}

type OperationObserver struct {
	observers [ExternalOperationError + 1]prometheus.Observer
}

func (o *OperationObserver) Observe(outcome ExternalOperationOutcome, elapsed time.Duration) {
	if o == nil {
		return
	}
	if outcome < ExternalOperationSuccess || outcome > ExternalOperationError {
		outcome = ExternalOperationError
	}
	o.observers[outcome].Observe(elapsed.Seconds())
}

type OperationTimer struct {
	observer *OperationObserver
	started  time.Time
}

func StartOperation(observer *OperationObserver) OperationTimer {
	return OperationTimer{observer: observer, started: time.Now()}
}

func (timer OperationTimer) Finish(ctx context.Context, outcome ExternalOperationOutcome) {
	ObserveOperation(ctx, timer.observer, outcome, time.Since(timer.started))
}

func ObserveOperation(
	ctx context.Context,
	observer *OperationObserver,
	outcome ExternalOperationOutcome,
	elapsed time.Duration,
) {
	if ctx != nil && ctx.Err() != nil &&
		(outcome == ExternalOperationError || outcome == ExternalOperationDegraded) {
		outcome = ExternalOperationSkipped
	}
	observer.Observe(outcome, elapsed)
}

func OutcomeForError(err error) ExternalOperationOutcome {
	if err != nil {
		return ExternalOperationError
	}
	return ExternalOperationSuccess
}

func Completeness(succeeded, total int) ExternalOperationOutcome {
	switch succeeded {
	case total:
		return ExternalOperationSuccess
	case 0:
		return ExternalOperationError
	default:
		return ExternalOperationDegraded
	}
}
