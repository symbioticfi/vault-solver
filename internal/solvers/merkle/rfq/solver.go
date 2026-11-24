// Package rfq fills awarded RFQ orders through caller-bound Merkle recipes.
package rfq

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
	"gopkg.in/yaml.v3"
)

const Name = "rfq-merkle"

//nolint:gochecknoinits // Self-registration is the solver plugin contract.
func init() { solver.Register(Name, factory) }

type orderBackend interface {
	discountResolver
	orders(ctx context.Context, filler string) ([]rfqbackend.OrdersResponseOrdersInner, error)
	listDiscounts(ctx context.Context) (*discounts.List, error)
}

// Solver feeds awarded RFQ orders to the shared Merkle fill lane.
type Solver struct {
	*filler.Engine

	backend     orderBackend
	tokenPolicy tokenpolicy.Policy
	// observeOrderPoll reports the same successful refresh that keeps quotes available.
	observeOrderPoll func(time.Time)
	lastAwardRefresh atomic.Pointer[time.Time]
	// Only the RFQ loop receives from this channel. The SDK submitter owns the
	// sender until it publishes the result, so receipt polling waits for it.
	submitting <-chan error
}

func (*Solver) Name() string                    { return Name }
func (*Solver) RequiresTxManager() bool         { return false }
func (s *Solver) RequiresMerkleTxManager() bool { return s.Sends() }
func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	return New(cfg, deps, nil)
}

// New shares the Merkle fill path with the RFQ quote server.
func New(cfg Config, deps solver.Deps, observeOrderPoll func(time.Time)) (*Solver, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	engine, err := filler.New(Name, cfg.Config, deps)
	if err != nil {
		return nil, err
	}
	return &Solver{Engine: engine, backend: newBackendClient(cfg.BackendURL), tokenPolicy: cfg.TokenPolicy, observeOrderPoll: observeOrderPoll}, nil
}

func (s *Solver) Run(ctx context.Context) error {
	defer func() {
		if s.submitting != nil {
			<-s.submitting
			s.submitting = nil
		}
	}()
	return s.Engine.Run(ctx, func(run context.Context) error { return s.tick(run, ctx) })
}

func (s *Solver) tick(ctx, lifecycle context.Context) error {
	var laneErr error
	if s.submitting != nil {
		select {
		case laneErr = <-s.submitting:
			s.submitting = nil
		default:
		}
	}
	if s.submitting == nil && laneErr == nil && s.Pending() != nil {
		outcome, err := s.Poll(ctx)
		if outcome != nil || ctx.Err() != nil {
			return err
		}
		laneErr = err
	}
	canSubmit := s.submitting == nil && s.Pending() == nil && laneErr == nil
	if canSubmit && s.Sender.Busy() {
		return errors.New("fill sender has work without a fill owner")
	}
	batch, err := s.pollAwards(ctx, canSubmit)
	if err != nil || !canSubmit {
		return errors.Join(laneErr, err)
	}
	intent, err := s.next(ctx, batch)
	if err != nil || intent == nil {
		return err
	}
	s.submitting, err = s.SubmitAsync(lifecycle, intent)
	return err
}

// CanQuote requires a healthy execution lane and a recent successful award refresh.
func (s *Solver) CanQuote(now time.Time) bool {
	lastPoll := s.lastAwardRefresh.Load()
	return lastPoll != nil && now.Sub(*lastPoll) < filler.ConfirmationPatience && s.Ready() && !s.Stalled()
}

// next returns the first awarded order that a configured connector can settle.
func (s *Solver) next(ctx context.Context, batch awardBatch) (*filler.Intent, error) {
	for _, row := range batch.rows {
		intent, err := s.prepare(ctx, row, batch.listings, batch.observed)
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

func (s *Solver) prepare(ctx context.Context, row rfqbackend.OrdersResponseOrdersInner, listed []discounts.ListItem, observed filler.Observation) (*filler.Intent, error) {
	order, signature, err := decodeOrder(row, s.Workflow.Filler)
	if err != nil {
		return nil, err
	}
	token, required, err := orderTerms(order, s.Workflow.Filler)
	if err != nil {
		return nil, err
	}
	if !s.tokenPolicy.Allows(order.Request.TokenIn) {
		return nil, nil
	}
	expiry := time.Unix(order.Request.Deadline.Int64(), 0)
	if !expiry.After(observed.Chain) {
		return nil, errors.New("order expired")
	}
	var routes []filler.Source
	for source, err := range s.Sources(listed, order.Request.TokenIn, token) {
		if err != nil {
			return nil, err
		}
		if source.MaxAssets.Sign() > 0 {
			routes = append(routes, source)
		}
	}
	intentFor := func(built builtFill) (*filler.Intent, error) {
		deadline, ok := observed.SubmissionDeadline(expiry, built.ValidUntil)
		if !ok {
			return nil, errors.New("fill deadline elapsed during preparation")
		}
		return &filler.Intent{
			OrderID: row.OrderId, Data: built.Calldata, Deadline: deadline,
			Spend: s.Spend(token, order.Request.Deadline.Int64(), built.Legs...),
		}, nil
	}
	for _, route := range routes {
		// Keep an executable single-connector fill when one can settle the award.
		route.MaxAssets = s.Unreserved(route, token, row.OrderId)
		if route.MaxAssets.Cmp(required) < 0 {
			continue
		}
		built, err := buildFill(ctx, s.Workflow, order, signature, []filler.Leg{{Source: route}}, s.backend)
		if err == nil {
			return intentFor(built)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		observability.Log(ctx).V(1).Info("connector route unavailable", "connector", route.Venue.Hex(), "reason", err.Error())
	}
	if s.tokenPolicy.RequiresSingleRoute(order.Request.TokenIn) {
		return nil, nil
	}
	legs, err := s.Split(ctx, routes, order.Request.AmountIn, required, len(routes), observed, row.OrderId)
	if err != nil || len(legs) == 0 {
		return nil, err
	}
	built, err := buildFill(ctx, s.Workflow, order, signature, legs, s.backend)
	if err != nil {
		return nil, err
	}
	return intentFor(built)
}
