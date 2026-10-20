// Package rfq fills awarded RFQ orders through caller-bound Merkle recipes.
package rfq

import (
	"context"
	"math/big"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/state"
	"github.com/symbioticfi/merkle-executor/sdk/submission"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/merklesender"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

const Name = "rfq-merkle"
const intentKey = "rfq-merkle.fill.1"
const operationTimeout = 30 * time.Second

var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/merkle/rfq", Name)

//nolint:gochecknoinits // Self-registration is the solver plugin contract.
func init() { solver.Register(Name, factory) }

type orderBackend interface {
	discountResolver
	orders(ctx context.Context, filler string) ([]rfqbackend.OrdersResponseOrdersInner, error)
	listDiscounts(ctx context.Context) (*discounts.List, error)
}

type journalSender interface {
	PendingID() string
	Send(ctx context.Context, req submission.Request) (submission.Result, error)
	Acknowledge(ctx context.Context, id string) error
}

type fillIntent struct {
	ChainID  string         `json:"chainId"`
	OrderID  string         `json:"orderId"`
	ID       string         `json:"id"`
	To       common.Address `json:"to"`
	Data     hexutil.Bytes  `json:"data"`
	Deadline time.Time      `json:"deadline"`
}

type Solver struct {
	workflow *connectorWorkflow
	backend  orderBackend
	sender   journalSender
	store    *state.Store
	intent   fillIntent // Only Run accesses the intent; the sender reads its deadline at admission.
}

func (*Solver) Name() string   { return Name }
func (s *Solver) Close() error { return s.store.Close() }

func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	if deps.Chain == nil || deps.Signer == nil || deps.TxManager == nil {
		return nil, errors.New("rfq-merkle requires the shared chain, signer and transaction manager")
	}
	source, err := permissions.New(cfg.Curator.httpConfig(), &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, deps.Chain, os.Getenv)
	if err != nil {
		return nil, err
	}
	store, err := state.Open(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	complete := false
	defer func() {
		if !complete {
			_ = store.Close()
		}
	}()
	s := &Solver{store: store, backend: newBackendClient(cfg.BackendURL), workflow: &connectorWorkflow{chain: deps.Chain, chainID: deps.Chain.ChainID(), executors: cfg.RecipeExecutors, source: source, caller: cfg.Executor, signer: deps.Signer.Address()}}
	s.sender, err = submission.New(deps.Chain, deps.Signer.Address(), deps.Chain.ChainID(), store, submission.Config{Confirmations: deps.TxManager.Confirmations()}, merklesender.Transport{Manager: deps.TxManager, Solver: Name, Deadline: func() time.Time { return s.intent.Deadline }}, deps.Log)
	if err != nil {
		return nil, err
	}
	if s.sender.PendingID() != "" {
		found, err := store.Load(intentKey, &s.intent)
		if err != nil {
			return nil, err
		}
		if !found || s.intent.ChainID != deps.Chain.ChainID().String() || s.intent.To != cfg.Executor || s.intent.ID != s.sender.PendingID() || s.intent.ID != intentID(s.intent.Data) {
			return nil, errors.New("rfq-merkle signed transaction has no matching fill intent")
		}
	}
	complete = true
	return s, nil
}

func (s *Solver) Run(ctx context.Context) error {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		run, cancel := context.WithTimeout(ctx, operationTimeout)
		err := s.tick(run)
		cancel()
		if err != nil && ctx.Err() == nil {
			observability.Log(ctx).Error(err, "RFQ Merkle iteration failed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func intentID(data []byte) string { return Name + ":" + crypto.Keccak256Hash(data).Hex() }

// One admitted award owns the signer lane until its canonical receipt is acknowledged.
func (s *Solver) tick(ctx context.Context) (err error) {
	ctx, end := tracer.Start(ctx, "rfq-merkle.poll")
	defer func() { end(err) }()
	if s.sender.PendingID() != "" {
		return s.send(ctx)
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
		if err := s.store.Save(intentKey, intent); err != nil {
			return err
		}
		s.intent = *intent
		return s.send(ctx)
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
	header, err := s.workflow.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	if header == nil || header.Time > ^uint64(0)>>1 {
		return nil, errors.New("invalid chain timestamp")
	}
	chainTime, observed := time.Unix(int64(header.Time), 0), time.Now()
	if order.Request.Deadline.Cmp(new(big.Int).SetUint64(header.Time)) <= 0 {
		return nil, errors.New("order expired")
	}
	for _, item := range listed {
		venue := common.HexToAddress(item.Adapter)
		if _, ok := s.workflow.executors[venue]; !ok || common.HexToAddress(item.TokenToRedeem) != order.Request.TokenIn || common.HexToAddress(item.Collateral) != token {
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
		data, until, err := s.workflow.build(ctx, order, signature, selectedRoute{venue, common.BytesToHash(id), maximum}, s.backend)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			observability.Log(ctx).V(1).Info("connector route unavailable", "connector", venue.Hex(), "reason", err.Error())
			continue
		}
		deadline, ok := liquidlane.SubmissionDeadline(minTime(time.Unix(order.Request.Deadline.Int64(), 0), until), chainTime, observed, time.Now())
		if !ok {
			return nil, errors.New("fill deadline elapsed during preparation")
		}
		return &fillIntent{ChainID: s.workflow.chainID.String(), OrderID: row.OrderId, ID: intentID(data), To: s.workflow.caller, Data: data, Deadline: deadline}, nil
	}
	return nil, nil
}

func (s *Solver) send(ctx context.Context) (err error) {
	ctx, end := tracer.Start(ctx, "rfq-merkle.fill", observability.AttrOrderID.String(s.intent.OrderID))
	defer func() { end(err) }()
	r, err := s.sender.Send(ctx, submission.Request{ID: s.intent.ID, To: s.intent.To, Data: s.intent.Data})
	if err != nil {
		return err
	}
	if !r.Confirmed && !r.Reverted {
		return nil
	}
	if err := s.sender.Acknowledge(ctx, s.intent.ID); err != nil {
		return err
	}
	observability.Log(ctx).Info("RFQ fill transaction finalized", "orderId", s.intent.OrderID, "tx", r.Hash.Hex(), "reverted", r.Reverted)
	return s.store.Delete(intentKey)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
