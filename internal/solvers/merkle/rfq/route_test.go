package rfq

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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

// A slow recipe read during deferred publication must not stall live quotes.
func TestInitializeDoesNotBlockRouteReads(t *testing.T) {
	fixture := connectorExecutionFixture(t, connectorDiscountFixture())
	reader := &blockingLogsReader{connectorExecutionReader: fixture.reader, entered: make(chan struct{}), release: make(chan struct{})}
	workflow := *fixture.workflow
	workflow.chain = reader
	workflow.connector = common.Address{}
	s := &Solver{recipeExecutors: []common.Address{fixture.reader.recipeExecutor}, workflow: &workflow}
	done := make(chan error, 1)
	go func() {
		_, err := s.Initialize(t.Context())
		done <- err
	}()
	<-reader.entered
	declined := make(chan bool, 1)
	go func() {
		declined <- !s.AcceptsConnector(vlt) && !s.AcceptsRoute(fixture.reader.recipeExecutor, vlt, fixture.reader.recipeAccount, tOut)
	}()
	select {
	case ok := <-declined:
		if !ok {
			t.Fatal("route accepted before publication")
		}
	case <-time.After(time.Second):
		t.Fatal("route read blocked on recipe discovery")
	}
	close(reader.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !s.AcceptsConnector(vlt) {
		t.Fatal("route not published")
	}
}

type blockingLogsReader struct {
	*connectorExecutionReader

	entered, release chan struct{}
}

func (r *blockingLogsReader) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	close(r.entered)
	<-r.release
	return r.connectorExecutionReader.FilterLogs(ctx, query)
}
