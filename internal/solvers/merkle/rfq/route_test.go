package rfq

import (
	"slices"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestConfiguredExecutorDerivesPaymentConnector(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Solver)
		want   string
	}{
		{name: "configured executor"},
		{name: "wrong executor", mutate: func(s *Solver) { s.recipeExecutors[0] = common.HexToAddress("0xbad") }, want: ""},
		{name: "wrong caller", mutate: func(s *Solver) { s.workflow.caller = common.HexToAddress("0xbad") }, want: "caller mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectorDiscountFixture())
			s := &Solver{recipeExecutors: []common.Address{fixture.reader.recipeExecutor}, workflow: fixture.workflow}
			s.workflow.connector = common.Address{}
			if tc.mutate != nil {
				tc.mutate(s)
			}
			venue, err := s.Initialize(t.Context())
			if tc.mutate != nil {
				if err == nil || (tc.want != "" && !strings.Contains(err.Error(), tc.want)) {
					t.Fatalf("expected %s, got %v", tc.want, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(venue) != 1 || venue[0] != vlt || !s.AcceptsRoute(fixture.reader.recipeExecutor, vlt, fixture.reader.recipeAccount, tOut) {
				t.Fatalf("wrong recipe connector %s", venue)
			}
			again, err := s.Initialize(t.Context())
			if err != nil || !slices.Equal(again, venue) {
				t.Fatalf("initialize not stable: %s %v", again, err)
			}
		})
	}
}
