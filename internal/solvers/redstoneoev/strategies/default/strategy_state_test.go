package defaultstrategy

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type decisionStateReader struct {
	Reader

	rate             *big.Int
	balance          *big.Int
	balanceErr       error
	readLoanDecimals *int
}

func (r decisionStateReader) ReadLoanEthRate(_ context.Context, loanDecimals int, _ *loanEthFeed, _ time.Time) *big.Int {
	if r.readLoanDecimals != nil {
		*r.readLoanDecimals = loanDecimals
	}
	return cloneBig(r.rate)
}

func (r decisionStateReader) ReadNativeBalance(context.Context, common.Address) (*big.Int, error) {
	return cloneBig(r.balance), r.balanceErr
}

func TestRefreshStateUsesSolverAdapterSnapshot(t *testing.T) {
	gotDecimals := -1
	s := &Strategy{
		loadAdapter: func() (types.AdapterSnapshot, bool) {
			return types.AdapterSnapshot{LoanDecimals: 6}, true
		},
		reader: decisionStateReader{
			rate:             big.NewInt(10),
			balance:          big.NewInt(20),
			readLoanDecimals: &gotDecimals,
		},
		log: logr.Discard(),
	}
	s.refreshState(t.Context())
	if gotDecimals != 6 {
		t.Fatalf("loan decimals = %d, want solver adapter snapshot value 6", gotDecimals)
	}
}

func TestRefreshStatePreservesLastGoodValuesWithoutExtendingFreshness(t *testing.T) {
	previousAt := time.Unix(1000, 0)
	s := &Strategy{
		reader: decisionStateReader{balanceErr: errors.New("rpc unavailable")},
		log:    logr.Discard(),
	}
	s.state.store(decisionState{
		Rate:              big.NewInt(10),
		CallbackNative:    big.NewInt(20),
		RateUpdatedAt:     previousAt,
		CallbackUpdatedAt: previousAt,
	})

	s.refreshState(t.Context())
	got, ok := s.state.load()
	if !ok {
		t.Fatal("decision state missing")
	}
	if got.Rate.Int64() != 10 || got.CallbackNative.Int64() != 20 {
		t.Fatalf("last good values not preserved: %+v", got)
	}
	if !got.RateUpdatedAt.Equal(previousAt) || !got.CallbackUpdatedAt.Equal(previousAt) {
		t.Fatalf("failed refresh extended freshness: %+v", got)
	}
}
