package rfq

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
)

type notifyingSender struct {
	fakeSender

	wake    chan struct{}
	polled  chan struct{}
	results chan *transaction.Result
}

func (s *notifyingSender) Wakeup() <-chan struct{} { return s.wake }
func (s *notifyingSender) Poll(context.Context) (*transaction.Result, error) {
	select {
	case result := <-s.results:
		s.busy = false
		return result, nil
	default:
		select {
		case s.polled <- struct{}{}:
		default:
		}
		return nil, nil
	}
}

type nextAwardBackend struct {
	fakeBackend

	cancel context.CancelFunc
}

func (b *nextAwardBackend) orders(context.Context, string) ([]rfqbackend.OrdersResponseOrdersInner, error) {
	b.cancel()
	return nil, nil
}

func TestRunWakesOnConfirmationAndContinuesToNextAward(t *testing.T) {
	f := connectorExecutionFixture(t, connectorDiscountFixture())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sender := &notifyingSender{
		fakeSender: fakeSender{busy: true},
		wake:       make(chan struct{}, 1),
		polled:     make(chan struct{}, 1),
		results:    make(chan *transaction.Result, 1),
	}
	s := &Solver{
		recipeExecutors: []common.Address{f.reader.recipeExecutor},
		workflow:        f.workflow,
		backend:         &nextAwardBackend{cancel: cancel},
		sender:          sender,
		intent:          &fillIntent{OrderID: "pending"},
	}
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	select {
	case <-sender.polled:
	case err := <-done:
		t.Fatal("solver stopped before polling", err)
	case <-time.After(time.Second):
		t.Fatal("solver did not poll retained work")
	}
	sender.results <- fillResult()
	sender.wake <- struct{}{}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("confirmed fill waited for the three-second timer before next award")
	}
}
