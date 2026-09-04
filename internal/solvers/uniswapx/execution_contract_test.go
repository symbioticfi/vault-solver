package uniswapx

import (
	"context"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type syncTestTxManager struct {
	request txmanager.Request
	result  txmanager.Result
}

func (*syncTestTxManager) MaxFeePerGas(context.Context) (*big.Int, error) { return big.NewInt(1), nil }
func (*syncTestTxManager) Available() bool                                { return true }
func (*syncTestTxManager) LaneReady() bool                                { return true }
func (m *syncTestTxManager) Send(_ context.Context, request txmanager.Request) txmanager.Result {
	m.request = request
	return m.result
}

type callContractFunc func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)

func (f callContractFunc) CallContract(
	ctx context.Context,
	call ethereum.CallMsg,
	block *big.Int,
) ([]byte, error) {
	return f(ctx, call, block)
}

func TestSubmitFillOwnsLeaseThroughSynchronousResult(t *testing.T) {
	now := time.Now()
	adapter := common.HexToAddress("0x1111111111111111111111111111111111111111")
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	order := &resolvedOrder{
		Source: orderSourcePublicV2, Hash: common.HexToHash("0x01"), QuoteID: "quote",
		Executor: executor, Deadline: uint32(now.Add(time.Minute).Unix()), Encoded: []byte{1}, Signature: []byte{2},
	}
	route := liquidlane.Route{ID: "route", CapacityID: "capacity", Adapter: adapter}
	plan := &liquidlane.Plan{Routes: []liquidlane.PlanLeg{{
		RouteID: route.ID, CapacityID: route.CapacityID, Adapter: adapter,
		AmountIn: big.NewInt(10), ExpectedAmountOut: big.NewInt(9), MinAmountOut: big.NewInt(8),
		ReservedAmountOut: big.NewInt(9),
	}}}
	reservations := capacity.Amounts{route.CapacityID: big.NewInt(9)}

	for _, test := range []struct {
		name   string
		result txmanager.Result
	}{
		{name: "success"},
		{name: "not admitted", result: txmanager.Result{Err: errors.New("paused"), NotAdmitted: true}},
		{name: "failed", result: txmanager.Result{Err: errors.New("reverted")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			txm := &syncTestTxManager{result: test.result}
			book := new(capacity.Book)
			solver := &Solver{
				cfg: &Config{Breaker: BreakerConfig{MaxFailures: 3, Window: time.Minute},
					OrderServer: OrderServerConfig{PollInterval: time.Second}},
				solverAddress: common.HexToAddress("0x3333333333333333333333333333333333333333"),
				chain: callContractFunc(func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
					if book.Len() != 0 {
						t.Fatal("capacity reserved before preflight")
					}
					return nil, nil
				}),
				txm: txm, capacity: book, log: logr.Discard(),
				quoteRuntime: quoteRuntime{refreshCh: make(chan struct{}, 1)},
				ledger:       testLifecycle(nil),
			}
			setTestExecution(solver, order.Hash, trackedOrder{inFlight: true})
			facts := &fillFacts{input: FillInput{}, decisionRoutes: []liquidlane.Route{route},
				pricingMaxFee: new(big.Int)}
			if err := solver.submitFillPlan(t.Context(), order, facts, plan, reservations, now, now); err != nil {
				t.Fatal(err)
			}
			if txm.request.To != executor || len(txm.request.Data) == 0 {
				t.Fatalf("transaction request = %+v", txm.request)
			}
			if solver.capacity.Len() != 0 {
				t.Fatal("terminal result leaked capacity")
			}
			tracked := testOrderLifecycle(solver, order.Hash).execution
			if test.result.Err == nil && tracked.completedAt.IsZero() {
				t.Fatal("successful fill was not completed")
			}
			if test.result.Err != nil && !tracked.completedAt.IsZero() {
				t.Fatal("failed fill was completed")
			}
		})
	}
}

func TestFillDeadlineUsesEarliestConstraint(t *testing.T) {
	order := &resolvedOrder{Deadline: 200}
	if got := fillDeadline(order, time.Unix(100, 0)); !got.Equal(time.Unix(100, 0)) {
		t.Fatalf("deadline = %s", got)
	}
}
