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
	discount  *discounts.Resolved
	resolved  map[string]*discounts.Resolved
	rows      []rfqbackend.OrdersResponseOrdersInner
	ordersErr error
	listed    *discounts.List
	reads     int
}

func (b *fakeBackend) orders(context.Context, string) ([]rfqbackend.OrdersResponseOrdersInner, error) {
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
	if err := s.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 || observed[0].IsZero() {
		t.Fatalf("empty successful poll observations = %v", observed)
	}
	backend.ordersErr = errors.New("backend unavailable")
	if err := s.tick(t.Context()); !errors.Is(err, backend.ordersErr) {
		t.Fatalf("failed poll returned %v, want %v", err, backend.ordersErr)
	}
	if len(observed) != 1 {
		t.Fatalf("failed poll recorded success: %v", observed)
	}
	if s.CanQuote(observed[0].Add(filler.ConfirmationPatience)) {
		t.Fatal("failed poll kept stale quotes available")
	}

	backend.ordersErr = nil
	if err := s.tick(t.Context()); err != nil {
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

func TestPollerRetainsWholeAwardAndOwnsOnlyOneFill(t *testing.T) {
	fixture := connectorExecutionFixture(t, connectortest.DiscountFixture())
	order, _, backend := connectorExecutionAward(fixture.workflow.Filler)
	next := awardRow(t, order)
	next.OrderId = "next-award"
	backend.rows = []rfqbackend.OrdersResponseOrdersInner{awardRow(t, order), next}
	backend.listed = &discounts.List{Discounts: []discounts.ListItem{
		{DiscountPrecision: "100000000", Adapter: common.HexToAddress("0x01").Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectortest.DiscountID().Hex(), MaxAssets: "99999"},
		{DiscountPrecision: "100000000", Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectortest.DiscountID().Hex(), MaxAssets: "900"},
	}}
	sender := &fillertest.Sender{}
	s := &Solver{Engine: &filler.Engine{Routes: connector.NewRoutes(fixture.workflow, []common.Address{common.HexToAddress("0x43")}), Workflow: fixture.workflow, Sender: sender, Spends: connector.NewSpendLedger()}, backend: backend}
	s.LaneReady = func() bool { return true }
	sender.Before = func(req transaction.ExecutionRequest) {
		if s.Pending() == nil || !bytes.Equal(s.Pending().Data, req.Data) || s.Pending().Deadline.IsZero() {
			t.Fatal("fill admitted before RAM intent")
		}
		if reserved := s.Reservations(nil)[liquidlane.NewCapacityID(1, fixture.reader.RecipeAccount, tOut)]; reserved == nil || reserved.Cmp(big.NewInt(456)) != 0 {
			t.Fatal("fill admitted before reserving its signed output")
		}
	}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if fixture.reader.OuterCalls != 1 || sender.Polls != 0 || sender.Request.To != fixture.workflow.Filler || !sender.Busy() {
		t.Fatal("award split, unconfigured venue used or pending work released")
	}
	reads, request, intent := backend.reads, sender.Request, *s.Pending()
	backend.rows, backend.listed, backend.discount = nil, nil, nil // Signed terms may disappear.
	if err := s.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if backend.reads != reads || sender.Submits != 1 || !reflect.DeepEqual(intent, *s.Pending()) {
		t.Fatal("pending fill was rebuilt or its deadline changed")
	}
	lastOrderPoll := *s.lastOrderPoll.Load()
	sender.PollErr = transaction.ErrNonceConsumed
	if err := s.tick(t.Context()); !errors.Is(err, transaction.ErrNonceConsumed) || s.Pending() == nil || backend.reads != reads {
		t.Fatal("unconfirmed fill lost ownership or read a new award", err)
	}
	if s.CanQuote(lastOrderPoll.Add(filler.ConfirmationPatience)) {
		t.Fatal("quotes stayed available while the fill prevented new award polls")
	}
	sender.PollErr = nil
	sender.Result = fillertest.Confirmed()
	sender.DuringPoll = func() {
		if reserved := s.Reservations(nil)[liquidlane.NewCapacityID(1, fixture.reader.RecipeAccount, tOut)]; reserved == nil || reserved.Cmp(big.NewInt(456)) != 0 {
			t.Fatal("manager release lost the spend before canonical accounting")
		}
	}
	if err := s.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if backend.reads != reads || !reflect.DeepEqual(request, sender.Request) || sender.Submits != 1 || s.Pending() != nil || sender.Busy() {
		t.Fatal("canonical fill was rebuilt or its ownership was not released")
	}
	if !s.Spends.Finalized("award") {
		t.Fatal("canonical accounting did not remember the completed award")
	}
	backend.rows = []rfqbackend.OrdersResponseOrdersInner{awardRow(t, order), awardRow(t, order)}
	backend.listed = &discounts.List{}
	if err := s.tick(t.Context()); err != nil || sender.Submits != 1 {
		t.Fatal("completed award was submitted again", err)
	}
	// A different award waits for canonical settlement, then starts on the same single lane.
	backend.rows = append(backend.rows, next)
	backend.listed = &discounts.List{Discounts: []discounts.ListItem{{DiscountPrecision: "100000000", Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectortest.DiscountID().Hex(), MaxAssets: "900"}}}
	backend.discount = connectortest.DiscountFixture()
	sender.Result = nil
	sender.Before = nil
	if err := s.tick(t.Context()); err != nil || sender.Submits != 2 || s.Pending() == nil || s.Pending().OrderID != next.OrderId {
		t.Fatal("next award did not start after the first completed", err)
	}
	if err := s.tick(t.Context()); err != nil || sender.Submits != 2 {
		t.Fatal("pending second award admitted parallel work", err)
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
	intent, err := s.prepare(t.Context(), awardRow(t, order), []discounts.ListItem{{DiscountPrecision: "100000000", Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectortest.DiscountID().Hex(), MaxAssets: "900"}})
	if err != nil || intent == nil {
		t.Fatalf("prepare: intent=%v err=%v", intent, err)
	}
	if intent.Deadline.After(f.reader.HeaderStarted.Add(10*time.Second + 100*time.Millisecond)) {
		t.Fatalf("header latency extended deadline: got %v, header started %v", intent.Deadline, f.reader.HeaderStarted)
	}
}

func TestSubmissionErrorRetainsOnlySignedWork(t *testing.T) {
	for _, unsigned := range []bool{false, true} {
		t.Run(map[bool]string{false: "signed ambiguity", true: "before signing"}[unsigned], func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectortest.DiscountFixture())
			order, _, backend := connectorExecutionAward(fixture.workflow.Filler)
			backend.rows = []rfqbackend.OrdersResponseOrdersInner{awardRow(t, order)}
			backend.listed = &discounts.List{Discounts: []discounts.ListItem{{DiscountPrecision: "100000000", Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectortest.DiscountID().Hex(), MaxAssets: "900"}}}
			sender := &fillertest.Sender{Unsigned: unsigned, SubmitErr: context.DeadlineExceeded}
			s := &Solver{Engine: &filler.Engine{Routes: connector.NewRoutes(fixture.workflow, []common.Address{common.HexToAddress("0x43")}), Workflow: fixture.workflow, Sender: sender, Spends: connector.NewSpendLedger()}, backend: backend}
			if err := s.Initialize(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := s.tick(t.Context()); !errors.Is(err, context.DeadlineExceeded) || (s.Pending() == nil) != unsigned {
				t.Fatalf("signed ownership changed: intent=%v err=%v", s.Pending(), err)
			}
			capacityID := liquidlane.NewCapacityID(1, fixture.reader.RecipeAccount, tOut)
			reserved := s.Reservations(nil)[capacityID]
			if unsigned && len(s.Reservations(nil)) != 0 || !unsigned && (reserved == nil || reserved.Cmp(big.NewInt(456)) != 0) {
				t.Fatal("submission error released signed capacity or retained an unsigned reservation")
			}
			if !unsigned {
				reads := backend.reads
				if err := s.tick(t.Context()); err != nil || backend.reads != reads || sender.Submits != 1 {
					t.Fatal("ambiguous signed fill was rebuilt", err)
				}
			}
		})
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
			intent, err := s.prepare(t.Context(), awardRow(t, order), []discounts.ListItem{item})
			if err != nil || intent != nil {
				t.Fatalf("precision %q: intent=%v err=%v", precision, intent, err)
			}
			if f.reader.OuterCalls != 0 {
				t.Fatal("wrong-domain discount reached simulation")
			}
		})
	}
}
