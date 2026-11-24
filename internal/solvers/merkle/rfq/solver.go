// Package rfq fills awarded RFQ orders through caller-bound Merkle recipes.
package rfq

import (
	"context"
	"strings"
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
	// observeOrderPoll reports every successful read of the awarded orders.
	observeOrderPoll func(time.Time)
	lastOrderPoll    atomic.Pointer[time.Time]
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

func (s *Solver) Run(ctx context.Context) error { return s.Engine.Run(ctx, s.next) }

func (s *Solver) tick(ctx context.Context) error { return s.Tick(ctx, s.next) }

// CanQuote requires both a ready sender and a recent successful award poll.
func (s *Solver) CanQuote(now time.Time) bool {
	lastPoll := s.lastOrderPoll.Load()
	return lastPoll != nil && now.Sub(*lastPoll) < filler.ConfirmationPatience && s.Ready()
}

// next returns the first awarded order that a configured connector can settle.
func (s *Solver) next(ctx context.Context) (*filler.Intent, error) {
	rows, err := s.backend.orders(ctx, strings.ToLower(s.Workflow.Filler.Hex()))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	s.lastOrderPoll.Store(&now)
	if s.observeOrderPoll != nil {
		s.observeOrderPoll(now)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	listed, err := s.backend.listDiscounts(ctx)
	if err != nil {
		return nil, err
	}
	if listed == nil {
		return nil, errors.New("missing connector discount inventory")
	}
	for _, row := range rows {
		intent, err := s.prepare(ctx, row, listed.Discounts)
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

func (s *Solver) prepare(ctx context.Context, row rfqbackend.OrdersResponseOrdersInner, listed []discounts.ListItem) (*filler.Intent, error) {
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
	observed, err := s.Observe(ctx)
	if err != nil {
		return nil, err
	}
	if s.Spends.Finalized(row.OrderId) {
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
	legs, err := s.Split(ctx, routes, order.Request.AmountIn, required, len(routes), observed)
	if err != nil || len(legs) == 0 {
		return nil, err
	}
	built, err := buildFill(ctx, s.Workflow, order, signature, legs, s.backend)
	if err != nil {
		return nil, err
	}
	return intentFor(built)
}
