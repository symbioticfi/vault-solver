package defaultstrategy

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

type decisionStateReader struct {
	Reader

	rate       *big.Int
	balance    *big.Int
	balanceErr error
}

func (r decisionStateReader) ReadLoanEthRate(context.Context, common.Address, *loanEthFeed, time.Time) *big.Int {
	return cloneBig(r.rate)
}

func (r decisionStateReader) ReadNativeBalance(context.Context, common.Address) (*big.Int, error) {
	return cloneBig(r.balance), r.balanceErr
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
