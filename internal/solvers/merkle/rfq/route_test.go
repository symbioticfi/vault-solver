package rfq

import (
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
		{name: "wrong executor", mutate: func(s *Solver) { s.recipeExecutor = common.HexToAddress("0xbad") }, want: ""},
		{name: "wrong caller", mutate: func(s *Solver) { s.workflow.caller = common.HexToAddress("0xbad") }, want: "caller mismatch"},
		{name: "explicit allowlist excludes recipe", mutate: func(s *Solver) { s.workflow.connectors = map[common.Address]bool{common.HexToAddress("0xbad"): true} }, want: "outside configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectorDiscountFixture())
			s := &Solver{recipeExecutor: fixture.reader.recipeExecutor, workflow: fixture.workflow}
			s.workflow.connectors = nil
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
			if venue != vlt || !s.workflow.connectors[vlt] {
				t.Fatalf("wrong recipe connector %s", venue)
			}
			again, err := s.Initialize(t.Context())
			if err != nil || again != venue {
				t.Fatalf("initialize not stable: %s %v", again, err)
			}
		})
	}
}
