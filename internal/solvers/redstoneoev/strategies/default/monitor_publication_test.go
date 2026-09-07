package defaultstrategy

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func TestMonitorPublishesOnlyCompleteRead(t *testing.T) {
	for _, tc := range []struct {
		name      string
		available bool
		next      *snapshot
		err       error
		publish   bool
	}{
		{name: "missing adapter"},
		{name: "empty source", available: true},
		{name: "failed source", available: true, err: errors.New("RPC unavailable")},
		{name: "complete source", available: true, next: &snapshot{block: 42}, publish: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := &snapshot{block: 41, updatedAt: time.Unix(1, 0)}
			reads := 0
			m := marketMonitor{log: logr.Discard(), loadAdapter: func() (types.AdapterSnapshot, bool) {
				return types.AdapterSnapshot{Loan: common.Address{1}, Redeemable: []types.RedeemableSnapshot{{Asset: common.Address{2}}}}, tc.available
			}, read: func(context.Context, common.Address, []common.Address) (*snapshot, error) {
				reads++
				return tc.next, tc.err
			}}
			m.snap.Store(old)
			m.refresh(t.Context())
			actual := m.snapshot()
			if tc.publish {
				if actual != tc.next || !actual.updatedAt.After(old.updatedAt) {
					t.Fatalf("new observation not published: %+v", actual)
				}
			} else if actual != old {
				t.Fatalf("failed observation replaced the cache: %+v", actual)
			}
			if !tc.available && reads != 0 {
				t.Fatalf("read source without adapter: %d", reads)
			}
		})
	}
}
