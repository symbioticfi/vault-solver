package iris

import (
	"bytes"
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
)

// fakeChain answers the BLM requirement and the debt-token balance.
type fakeChain struct {
	requirement, balance *big.Int
	head                 *types.Header
}

func (c *fakeChain) CallContract(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	switch {
	case bytes.HasPrefix(msg.Data, blmBinding.GetABI().Methods["bondRequirement"].ID):
		return common.BigToHash(c.requirement).Bytes(), nil
	case bytes.HasPrefix(msg.Data, erc20Binding.PackBalanceOf(common.Address{})[:4]):
		return common.BigToHash(c.balance).Bytes(), nil
	}
	return nil, errors.New("unexpected call")
}

func (c *fakeChain) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return c.head, nil
}
func (*fakeChain) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	return nil, nil
}

func indicativeSolver(t *testing.T, chain *fakeChain) (*Solver, request) {
	t.Helper()
	f := loadFixture(t)
	prepared, tree := fixtureMarket(t, f)
	m, err := decodeMarket(prepared, tree, fixtureIris, big.NewInt(1))
	must(t, err)
	s := &Solver{cfg: Config{Pricing: Pricing{Default: big.NewInt(6e16), BondBufferBps: 1000}}, chain: chain, chainID: big.NewInt(1)}
	index := newMarketIndex([]Market{m})
	s.markets.Store(&index)
	r := request{
		RequestID: "r", CollateralToken: m.CollateralToken, DebtToken: m.DebtToken, Collateral: big.NewInt(1e8),
		Debt: big.NewInt(10_000e6), MaxFixedRate: big.NewInt(8e16), Duration: big.NewInt(30 * 86400), VenueBitmap: big.NewInt(2),
	}
	return s, r
}

func TestIndicativeQuotePricesTheBond(t *testing.T) {
	chain := &fakeChain{requirement: big.NewInt(30e6), balance: big.NewInt(1_000e6), head: &types.Header{Number: big.NewInt(1)}}
	s, r := indicativeSolver(t, chain)
	response, err := s.quote(context.Background(), r, true)
	must(t, err)
	equal(t, "33000000", response.Bond)
	equal(t, "60000000000000000", response.FixedRate)
	equal(t, "1", response.VenueID)
	equal(t, fixtureAccount.Hex(), response.Solver)
	ok(t, response.Indicative)
}

func TestIndicativeQuoteDeclines(t *testing.T) {
	tests := []struct {
		name   string
		chain  fakeChain
		mutate func(*Solver, *request)
		reason string
	}{
		{"unfunded bond", fakeChain{requirement: big.NewInt(30e6), balance: big.NewInt(32e6)}, nil, "insufficient_bond_balance"},
		{"zero requirement", fakeChain{requirement: new(big.Int), balance: big.NewInt(1e9)}, nil, "zero_bond_requirement"},
		{"bond cap", fakeChain{requirement: big.NewInt(30e6), balance: big.NewInt(1e9)}, func(s *Solver, _ *request) {
			s.cfg.Pricing.MaxBond = big.NewInt(32e6)
		}, "bond_above_limit"},
		{"venue not allowed", fakeChain{requirement: big.NewInt(30e6), balance: big.NewInt(1e9)}, func(_ *Solver, r *request) {
			r.VenueBitmap = big.NewInt(1)
		}, "no_market"},
		{"rate ceiling", fakeChain{requirement: big.NewInt(30e6), balance: big.NewInt(1e9)}, func(_ *Solver, r *request) {
			r.MaxFixedRate = big.NewInt(5e16)
		}, "rate_above_max_fixed_rate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := tt.chain
			chain.head = &types.Header{Number: big.NewInt(1)}
			s, r := indicativeSolver(t, &chain)
			if tt.mutate != nil {
				tt.mutate(s, &r)
			}
			_, err := s.quote(context.Background(), r, true)
			var refused *declined
			ok(t, errors.As(err, &refused), err)
			equal(t, tt.reason, refused.reason)
		})
	}
}

// fakeSender settles the submitted execution after polls polls, or drops it when stall is set.
type fakeSender struct {
	mu        sync.Mutex
	active    bool
	polls     int
	stall     bool
	submitted []transaction.ExecutionRequest
}

func (s *fakeSender) Submit(_ context.Context, req transaction.ExecutionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = true
	s.submitted = append(s.submitted, req)
	return nil
}

func (s *fakeSender) Poll(context.Context) (*transaction.ExecutionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stall {
		s.active = false
		return nil, nil
	}
	if s.polls--; s.polls > 0 {
		return nil, nil
	}
	s.active = false
	hash := common.HexToHash("0x1")
	return &transaction.ExecutionResult{Hash: hash, Execution: &provider.Execution{TxHash: hash, BlockNumber: big.NewInt(9)}}, nil
}

func (s *fakeSender) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func TestLaneOwnsOneExecutionUntilItSettles(t *testing.T) {
	sender := &fakeSender{polls: 3}
	l := &lane{sender: sender, poll: time.Millisecond}
	call := recipes.ExecutorCall{Executor: common.HexToAddress("0xE0"), Data: []byte{1, 2, 3, 4}}
	done, err := l.execute(context.Background(), context.Background(), call, time.Now().Add(time.Minute))
	must(t, err)
	_, err = l.execute(context.Background(), context.Background(), call, time.Now())
	ok(t, errors.Is(err, errLaneBusy), err)
	outcome := <-done
	must(t, outcome.err)
	equal(t, big.NewInt(9), outcome.result.Execution.BlockNumber)
	equal(t, call.Executor, sender.submitted[0].To)
	ok(t, !l.busy.Load())
}

func TestLaneStallsWhenTheSenderDropsTheExecution(t *testing.T) {
	l := &lane{sender: &fakeSender{stall: true}, poll: time.Millisecond}
	call := recipes.ExecutorCall{Executor: common.HexToAddress("0xE0"), Data: []byte{1, 2, 3, 4}}
	done, err := l.execute(context.Background(), context.Background(), call, time.Now().Add(time.Minute))
	must(t, err)
	ok(t, errors.Is((<-done).err, errLaneStalled))
	_, err = l.execute(context.Background(), context.Background(), call, time.Now().Add(time.Minute))
	ok(t, errors.Is(err, errLaneStalled), err)
}
