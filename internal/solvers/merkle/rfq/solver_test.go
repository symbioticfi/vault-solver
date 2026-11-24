package rfq

import (
	"bytes"
	"context"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler/fillertest"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"gopkg.in/yaml.v3"
)

var vlt, tIn, tOut = connectortest.Venue, connectortest.TokenIn, connectortest.TokenOut

type fakeBackend struct {
	discount     *discounts.Resolved
	resolved     map[string]*discounts.Resolved
	rows         []rfqbackend.OrdersResponseOrdersInner
	ordersErr    error
	listed       *discounts.List
	reads        int
	beforeOrders func()
}

func (b *fakeBackend) orders(context.Context, string) ([]rfqbackend.OrdersResponseOrdersInner, error) {
	if b.beforeOrders != nil {
		b.beforeOrders()
	}
	b.reads++
	return b.rows, b.ordersErr
}
func (b *fakeBackend) listDiscounts(context.Context) (*discounts.List, error) {
	b.reads++
	return b.listed, nil
}
func (b *fakeBackend) resolveDiscount(_ context.Context, id string) (*discounts.Resolved, error) {
	b.reads++
	if b.resolved != nil {
		return b.resolved[id], nil
	}
	return b.discount, nil
}

func TestEmptyOrderPollReportsOnlySuccess(t *testing.T) {
	backend := &fakeBackend{}
	var observed []time.Time
	workflow := connectorExecutionFixture(t, connectortest.DiscountFixture()).workflow
	s := &Solver{
		Engine:  &filler.Engine{Workflow: workflow, Routes: connector.NewRoutes(workflow, []common.Address{common.HexToAddress("0x43")}), Sender: &fillertest.Sender{}},
		backend: backend, observeOrderPoll: func(at time.Time) { observed = append(observed, at) },
	}
	s.LaneReady = func() bool { return true }
	if _, err := s.pollAwards(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 || observed[0].IsZero() {
		t.Fatalf("empty successful poll observations = %v", observed)
	}
	backend.ordersErr = errors.New("backend unavailable")
	if _, err := s.pollAwards(t.Context(), true); !errors.Is(err, backend.ordersErr) {
		t.Fatalf("failed poll returned %v, want %v", err, backend.ordersErr)
	}
	if len(observed) != 1 {
		t.Fatalf("failed poll recorded success: %v", observed)
	}
	if s.CanQuote(observed[0].Add(filler.ConfirmationPatience)) {
		t.Fatal("failed poll kept stale quotes available")
	}

	backend.ordersErr = nil
	if _, err := s.pollAwards(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 2 || !s.CanQuote(observed[1]) {
		t.Fatalf("successful poll did not resume quotes: %v", observed)
	}
}

func awardRow(t *testing.T, order executor.IReactorOrder) rfqbackend.OrdersResponseOrdersInner {
	t.Helper()
	data, err := orderTupleArgs.Pack(order)
	if err != nil {
		t.Fatal(err)
	}
	encoded, sig := hexutil.Encode(data), "0x9192"
	return rfqbackend.OrdersResponseOrdersInner{OrderId: "award", OrderStatus: "open", EncodedOrder: &encoded, ProtocolSignature: &sig}
}

func TestAwardSizingOnlyUsesSignedTuple(t *testing.T) {
	order, _, _ := connectorExecutionAward(common.HexToAddress("0x10"))
	for _, tc := range []struct {
		name   string
		mutate func(*executor.IReactorOrder)
		want   string
	}{
		{"valid", func(*executor.IReactorOrder) {}, ""},
		{"different filler", func(o *executor.IReactorOrder) { o.Filler = vlt }, "filler"},
		{"zero input", func(o *executor.IReactorOrder) { o.Request.AmountIn = new(big.Int) }, "input"},
		{"deadline overflow", func(o *executor.IReactorOrder) { o.Request.Deadline = new(big.Int).Lsh(big.NewInt(1), 255) }, "deadline"},
		{"missing output", func(o *executor.IReactorOrder) { o.Outputs = nil }, "outputs"},
		{"split output tokens", func(o *executor.IReactorOrder) {
			o.Outputs = append(o.Outputs, executor.IReactorOutput{Token: tIn, Amount: big.NewInt(1), Recipient: vlt})
		}, "one output"},
		{"zero payment", func(o *executor.IReactorOrder) { o.Outputs[0].Amount = new(big.Int) }, "positive"},
		{"payment overflow", func(o *executor.IReactorOrder) { o.Outputs[0].Amount = new(big.Int).Lsh(big.NewInt(1), 256) }, "uint256"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, _, _ := connectorExecutionAward(order.Filler)
			tc.mutate(&o)
			_, total, err := orderTerms(o, order.Filler)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error=%v want %s", err, tc.want)
				}
				return
			}
			if err != nil || total.Int64() != 456 {
				t.Fatalf("total=%v error=%v", total, err)
			}
			row := awardRow(t, o)
			row.Input.Amount = "999999999" // Display metadata cannot resize the signed award.
			decoded, signature, err := decodeOrder(row, order.Filler)
			if err != nil || !reflect.DeepEqual(decoded, o) || !bytes.Equal(signature, []byte{0x91, 0x92}) {
				t.Fatalf("decoded award changed: %v", err)
			}
		})
	}
}

func awardSolver(t *testing.T, sender *fillertest.Sender) (*Solver, *fakeBackend, liquidlane.CapacityID) {
	t.Helper()
	f := connectorExecutionFixture(t, connectortest.DiscountFixture())
	order, _, backend := connectorExecutionAward(f.workflow.Filler)
	backend.rows = []rfqbackend.OrdersResponseOrdersInner{awardRow(t, order)}
	backend.listed = &discounts.List{Discounts: []discounts.ListItem{{
		DiscountPrecision: connector.PrecisionText, Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(),
		Collateral: tOut.Hex(), DiscountID: connectortest.DiscountID().Hex(), MaxAssets: "1000",
	}}}
	s := &Solver{Engine: &filler.Engine{
		Workflow: f.workflow, Routes: connector.NewRoutes(f.workflow, []common.Address{f.reader.RecipeExecutor}),
		Sender: sender, Spends: connector.NewSpendLedger(), LaneReady: func() bool { return true },
	}, backend: backend}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return s, backend, liquidlane.NewCapacityID(1, f.reader.RecipeAccount, tOut)
}

func TestAwardsStayReservedDuringSigningAndFillSequentially(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	sender := &fillertest.Sender{Result: fillertest.Confirmed()}
	s, backend, capacity := awardSolver(t, sender)
	first, second := backend.rows[0], backend.rows[0]
	second.OrderId = "second-award"
	backend.beforeOrders = func() {
		if backend.reads > 0 {
			backend.rows = []rfqbackend.OrdersResponseOrdersInner{first, second}
		}
	}
	release := make(chan struct{})
	started := make(chan string, 2)
	sender.Before = func(transaction.ExecutionRequest) {
		orderID := s.Pending().OrderID
		started <- orderID
		if orderID == first.OrderId {
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}
	polled := make(chan time.Time, 3)
	s.observeOrderPoll = func(at time.Time) { polled <- at }
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	defer func() { cancel(); <-done }()
	select {
	case orderID := <-started:
		if orderID != first.OrderId {
			t.Fatalf("sent %s first", orderID)
		}
	case <-ctx.Done():
		t.Fatal("first award did not reach the signer")
	}
	<-polled
	select {
	case <-polled:
	case <-ctx.Done():
		t.Fatal("signing blocked award polling")
	}
	if held := s.Reservations(nil)[capacity]; !s.CanQuote(time.Now()) || held == nil || held.Int64() != 912 {
		t.Fatalf("quotes must stay available against the unreserved balance, held %v", held)
	}
	close(release)
	select {
	case orderID := <-started:
		if orderID != second.OrderId || !s.Spends.Finalized(first.OrderId) {
			t.Fatalf("unexpected next award %s", orderID)
		}
	case <-ctx.Done():
		t.Fatal("second award did not reach the signer")
	}
}

func TestWholeAwardRequiresVenueAndAdvertisedCapacity(t *testing.T) {
	for _, capacity := range []string{"advertised", "onchain"} {
		t.Run(capacity, func(t *testing.T) {
			f := connectorExecutionFixture(t, connectortest.DiscountFixture())
			order, selected, backend := connectorExecutionAward(f.workflow.Filler)
			if capacity == "advertised" {
				selected.MaxAssets = big.NewInt(455)
			} else {
				f.reader.Remaining = big.NewInt(455)
			}
			if _, err := buildFill(t.Context(), f.workflow, order, nil, []filler.Leg{{Source: selected}}, backend); err == nil || !strings.Contains(err.Error(), "whole award") {
				t.Fatalf("capacity accepted: %v", err)
			}
			if f.reader.OuterCalls != 0 {
				t.Fatal("underfunded award simulated or submitted")
			}
		})
	}
}

func prepareAtHead(t *testing.T, s *Solver, row rfqbackend.OrdersResponseOrdersInner, listed []discounts.ListItem) (*filler.Intent, error) {
	t.Helper()
	observed, err := s.Observe(t.Context())
	if err != nil {
		return nil, err
	}
	return s.planAward(t.Context(), row, listed, observed, false)
}

func TestPrepareCountsHeaderLatencyAgainstDeadline(t *testing.T) {
	f := connectorExecutionFixture(t, connectortest.DiscountFixture())
	chainTime := time.Now().Add(time.Hour).Truncate(time.Second)
	f.reader.Header.Time = uint64(chainTime.Unix())
	order, _, backend := connectorExecutionAward(f.workflow.Filler)
	order.Request.Deadline = big.NewInt(chainTime.Add(10 * time.Second).Unix())
	s := Solver{Engine: &filler.Engine{Routes: connector.NewRoutes(f.workflow, []common.Address{common.HexToAddress("0x43")}), Workflow: f.workflow}, backend: backend}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.reader.HeaderDelay = 250 * time.Millisecond
	f.reader.HeaderStarted = time.Time{}
	backend.rows = []rfqbackend.OrdersResponseOrdersInner{awardRow(t, order)}
	backend.listed = &discounts.List{Discounts: []discounts.ListItem{{DiscountPrecision: connector.PrecisionText, Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectortest.DiscountID().Hex(), MaxAssets: "900"}}}
	intent, err := s.pollAwards(t.Context(), true)
	observedAt := f.reader.HeaderStarted
	if err != nil || intent == nil {
		t.Fatalf("prepare: intent=%v err=%v", intent, err)
	}
	if observedAt.IsZero() || intent.Deadline.After(observedAt.Add(10*time.Second+100*time.Millisecond)) {
		t.Fatalf("preparation extended the award deadline: got %v, header started %v", intent.Deadline, observedAt)
	}
}

func TestFailedOrderPollKeepsReservations(t *testing.T) {
	s, backend, capacity := awardSolver(t, &fillertest.Sender{Unsigned: true, SubmitErr: context.DeadlineExceeded})
	intent, err := s.pollAwards(t.Context(), true)
	if err != nil || intent == nil {
		t.Fatalf("expected an executable award: %v", err)
	}
	if err := s.Submit(t.Context(), intent); !errors.Is(err, context.DeadlineExceeded) || s.Pending() != nil {
		t.Fatalf("unsigned failure did not release signer ownership: %v", err)
	}
	backend.rows = nil
	backend.ordersErr = errors.New("orders feed unavailable")
	if _, err := s.pollAwards(t.Context(), false); !errors.Is(err, backend.ordersErr) {
		t.Fatal(err)
	}
	if held := s.Reservations(nil)[capacity]; held == nil || held.Int64() != 456 {
		t.Fatalf("failed poll released a queued award: %v", held)
	}
	backend.ordersErr = nil
	if _, err := s.pollAwards(t.Context(), false); err != nil || len(s.Reservations(nil)) != 0 {
		t.Fatalf("complete poll did not release a closed award: %v", err)
	}
}

func TestConfigKeepsMerkleSeparateFromAdapterSolver(t *testing.T) {
	valid := `backendUrl: https://rfq.example
executor: "0x0000000000000000000000000000000000000010"
recipeExecutors: ["0x0000000000000000000000000000000000000043"]
curator: {url: https://curator.example/api/v1}
`
	for _, tc := range []struct {
		name, text string
		valid      bool
	}{
		{"valid", valid, true},
		{"no adapters", valid + "adapters: []\n", false},
		{"no old mode", valid + "solverMode: internal\n", false},
		{"no quoting", valid + "listenAddr: ':8080'\n", false},
		{"no strategies", valid + "strategy: {name: default}\n", false},
		{"obsolete recovery store", valid + "stateDir: /persistent/rfq-merkle\n", false},
		{"zero executor", strings.ReplaceAll(valid, "0000000010", "0000000000"), false},
		{"unsafe source", strings.ReplaceAll(valid, "https://rfq.example", "https://secret@rfq.example"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tc.text), &node); err != nil {
				t.Fatal(err)
			}
			_, err := parseConfig(node)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

func TestPrepareRequiresConnectorPrecision(t *testing.T) {
	for _, precision := range []string{"", "1000000", "10000"} {
		t.Run("precision_"+precision, func(t *testing.T) {
			f := connectorExecutionFixture(t, connectortest.DiscountFixture())
			order, _, backend := connectorExecutionAward(f.workflow.Filler)
			s := Solver{Engine: &filler.Engine{Routes: connector.NewRoutes(f.workflow, []common.Address{common.HexToAddress("0x43")}), Workflow: f.workflow, Spends: connector.NewSpendLedger()}, backend: backend}
			if err := s.Initialize(t.Context()); err != nil {
				t.Fatal(err)
			}
			item := discounts.ListItem{DiscountPrecision: precision, Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(),
				Collateral: tOut.Hex(), DiscountID: connectortest.DiscountID().Hex(), MaxAssets: "900"}
			intent, err := prepareAtHead(t, &s, awardRow(t, order), []discounts.ListItem{item})
			if err != nil || intent != nil {
				t.Fatalf("precision %q: intent=%v err=%v", precision, intent, err)
			}
			if f.reader.OuterCalls != 0 {
				t.Fatal("wrong-domain discount reached simulation")
			}
		})
	}
}
