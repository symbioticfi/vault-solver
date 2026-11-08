// Package rfq fills awarded RFQ orders through caller-bound Merkle recipes.
package rfq

import (
	"context"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

const Name = "rfq-merkle"
const operationTimeout = 30 * time.Second

var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/merkle/rfq", Name)

//nolint:gochecknoinits // Self-registration is the solver plugin contract.
func init() { solver.Register(Name, factory) }

type orderBackend interface {
	discountResolver
	orders(ctx context.Context, filler string) ([]rfqbackend.OrdersResponseOrdersInner, error)
	listDiscounts(ctx context.Context) (*discounts.List, error)
}

type fillIntent struct {
	OrderID  string
	To       common.Address
	Data     []byte
	Deadline time.Time
	Spend    fillSpend
}

type Solver struct {
	recipeExecutors []common.Address
	routes          map[common.Address]recipeRoute
	routesMu        sync.RWMutex
	spends          *spendLedger
	workflow        *connectorWorkflow
	backend         orderBackend
	sender          keeper.Sender
	intent          *fillIntent // Only Run accesses the intent; the sender reads its deadline at admission.
}

func (*Solver) Name() string { return Name }
func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	return New(cfg, deps)
}

// New shares the Merkle fill path with the RFQ quote server.
func New(cfg Config, deps solver.Deps) (*Solver, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.Chain == nil || (!cfg.QuoteOnly && (deps.Signer == nil || deps.TxManager == nil)) {
		return nil, errors.New("rfq-merkle requires the shared chain, signer and transaction manager")
	}
	s := &Solver{spends: newSpendLedger(), recipeExecutors: cfg.RecipeExecutors, backend: newBackendClient(cfg.BackendURL), workflow: &connectorWorkflow{chain: deps.Chain, chainID: deps.Chain.ChainID(), caller: cfg.Executor}}
	if cfg.QuoteOnly {
		return s, nil
	}
	source, err := cfg.Curator.PermissionSource(deps.Chain)
	if err != nil {
		return nil, err
	}
	s.workflow.source, s.workflow.signer = source, deps.Signer.Address()
	sender := deps.TxManager.Sender(Name)
	sender.Deadline = func() time.Time {
		if s.intent == nil {
			return time.Time{}
		}
		return s.intent.Deadline
	}
	s.sender = sender
	return s, nil
}

func (s *Solver) Run(ctx context.Context) error {
	if err := s.Initialize(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		pending := s.intent != nil
		run, cancel := context.WithTimeout(ctx, operationTimeout)
		err := s.tick(run)
		cancel()
		if err != nil && ctx.Err() == nil {
			observability.Log(ctx).Error(err, "RFQ Merkle iteration failed")
		}
		if pending && s.intent == nil && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// One admitted award owns the signer lane until its canonical receipt is returned.
func (s *Solver) tick(ctx context.Context) (err error) {
	ctx, end := tracer.Start(ctx, "rfq-merkle.poll")
	defer func() { end(err) }()
	if s.intent != nil {
		return s.poll(ctx)
	}
	if s.sender.Busy() {
		return errors.New("RFQ sender has work without a fill owner")
	}
	if err := s.Initialize(ctx); err != nil {
		return err
	}
	rows, err := s.backend.orders(ctx, strings.ToLower(s.workflow.caller.Hex()))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	listed, err := s.backend.listDiscounts(ctx)
	if err != nil {
		return err
	}
	if listed == nil {
		return errors.New("missing connector discount inventory")
	}
	sort.Slice(listed.Discounts, func(i, j int) bool {
		a, b := listed.Discounts[i], listed.Discounts[j]
		av, bv := common.HexToAddress(a.Adapter), common.HexToAddress(b.Adapter)
		if av != bv {
			return av.Cmp(bv) < 0
		}
		return a.DiscountID < b.DiscountID
	})
	for _, row := range rows {
		intent, err := s.prepare(ctx, row, listed.Discounts)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			observability.Log(ctx).V(1).Info("RFQ award not executable", "orderId", row.OrderId, "reason", err.Error())
			continue
		}
		if intent == nil {
			continue
		}
		// Capture the awarded bytes before submission; a timeout may follow a send.
		s.intent = intent
		s.spends.setActive(true)
		err = s.sender.Submit(ctx, transaction.ExecutionRequest{To: intent.To, Data: intent.Data})
		if err != nil && !s.sender.Busy() {
			s.intent = nil
			s.spends.setActive(false)
		}
		return err
	}
	return nil
}

func (s *Solver) prepare(ctx context.Context, row rfqbackend.OrdersResponseOrdersInner, listed []discounts.ListItem) (*fillIntent, error) {
	order, signature, err := decodeOrder(row, s.workflow.caller)
	if err != nil {
		return nil, err
	}
	token, _, err := orderTerms(order, s.workflow.caller)
	if err != nil {
		return nil, err
	}
	observed := time.Now()
	header, err := s.workflow.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	if header == nil || header.Time > ^uint64(0)>>1 {
		return nil, errors.New("invalid chain timestamp")
	}
	s.spends.expire(int64(header.Time))
	if s.spends.finalized(row.OrderId) {
		return nil, nil
	}
	chainTime := time.Unix(int64(header.Time), 0)
	if order.Request.Deadline.Cmp(new(big.Int).SetUint64(header.Time)) <= 0 {
		return nil, errors.New("order expired")
	}
	for _, item := range listed {
		venue := common.HexToAddress(item.Adapter)
		recipeExecutor := common.HexToAddress(item.RecipeExecutor)
		route, configured := s.recipeRoute(recipeExecutor)
		if !configured || route.connector != venue || common.HexToAddress(item.TokenToRedeem) != order.Request.TokenIn || common.HexToAddress(item.Collateral) != token {
			continue
		}
		id, err := hexutil.Decode(item.DiscountID)
		if err != nil || len(id) != 32 {
			return nil, errors.New("invalid connector discountId")
		}
		maximum, ok := new(big.Int).SetString(item.MaxAssets, 10)
		if !ok || maximum.Sign() < 0 || maximum.BitLen() > 256 {
			return nil, errors.New("invalid connector maxAssets")
		}
		built, err := s.workflow.build(ctx, order, signature, selectedRoute{Venue: venue, Executor: recipeExecutor, DiscountID: common.BytesToHash(id), MaxAssets: maximum}, s.backend)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			observability.Log(ctx).V(1).Info("connector route unavailable", "connector", venue.Hex(), "reason", err.Error())
			continue
		}
		deadline, ok := liquidlane.SubmissionDeadline(minTime(time.Unix(order.Request.Deadline.Int64(), 0), built.ValidUntil), chainTime, observed, time.Now())
		if !ok {
			return nil, errors.New("fill deadline elapsed during preparation")
		}
		return &fillIntent{OrderID: row.OrderId, To: s.workflow.caller, Data: built.Calldata, Deadline: deadline, Spend: fillSpend{CapacityID: liquidlane.NewCapacityID(s.workflow.chainID.Int64(), built.State.Account, token), Deadline: order.Request.Deadline.Int64()}}, nil
	}
	return nil, nil
}

func (s *Solver) poll(ctx context.Context) (err error) {
	ctx, end := tracer.Start(ctx, "rfq-merkle.fill", observability.AttrOrderID.String(s.intent.OrderID))
	defer func() { end(err) }()
	result, err := s.sender.Poll(ctx)
	if err != nil || result == nil {
		return err
	}
	if result.Hash == (common.Hash{}) || result.Execution == nil || result.Execution.TxHash != result.Hash {
		return errors.New("RFQ sender returned invalid execution identity")
	}
	if err := result.Execution.Validate(); err != nil {
		return err
	}
	if !result.Reverted {
		spend := s.intent.Spend
		spend.IncludedAt = result.Execution.BlockNumber.Uint64()
		if err := s.spends.include(s.intent.OrderID, spend); err != nil {
			return err
		}
	}
	s.spends.setActive(false)
	observability.Log(ctx).Info("RFQ fill transaction finalized", "orderId", s.intent.OrderID, "tx", result.Hash.Hex(), "reverted", result.Reverted)
	s.intent = nil
	return nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
