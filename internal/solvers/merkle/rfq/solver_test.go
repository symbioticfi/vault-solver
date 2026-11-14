package rfq

import (
	"bytes"
	"context"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"gopkg.in/yaml.v3"
)

var vlt = common.HexToAddress("0x42")
var tIn = common.HexToAddress("0x31")
var tOut = common.HexToAddress("0x32")

type fakeBackend struct {
	discount *discounts.Resolved
	rows     []rfqbackend.OrdersResponseOrdersInner
	listed   *discounts.List
	reads    int
}

func (b *fakeBackend) orders(context.Context, string) ([]rfqbackend.OrdersResponseOrdersInner, error) {
	b.reads++
	return b.rows, nil
}
func (b *fakeBackend) listDiscounts(context.Context) (*discounts.List, error) {
	b.reads++
	return b.listed, nil
}
func (b *fakeBackend) resolveDiscount(context.Context, string) (*discounts.Resolved, error) {
	b.reads++
	return b.discount, nil
}

type fakeSender struct {
	busy       bool
	request    transaction.ExecutionRequest
	result     *transaction.ExecutionResult
	submitErr  error
	pollErr    error
	before     func(transaction.ExecutionRequest)
	duringPoll func()
	submits    int
	polls      int
	unsigned   bool
}

func (s *fakeSender) Busy() bool { return s.busy }
func (s *fakeSender) Submit(_ context.Context, req transaction.ExecutionRequest) error {
	if s.before != nil {
		s.before(req)
	}
	s.submits++
	s.request = req
	s.busy = !s.unsigned
	return s.submitErr
}
func (s *fakeSender) Poll(context.Context) (*transaction.ExecutionResult, error) {
	s.polls++
	if s.result != nil && s.pollErr == nil {
		s.busy = false
		if s.duringPoll != nil {
			s.duringPoll()
		}
	}
	return s.result, s.pollErr
}

func fillResult() *transaction.ExecutionResult {
	hash := common.HexToHash("0x77")
	return &transaction.ExecutionResult{Hash: hash, Execution: &provider.Execution{TxHash: hash, BlockNumber: big.NewInt(8), BlockHash: common.HexToHash("0x78")}}
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

func TestPrepareRequiresCapacityHintForAmbiguousRecipes(t *testing.T) {
	for _, hint := range []string{"", common.HexToAddress("0x43").Hex()} {
		t.Run("hint="+hint, func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectorDiscountFixture())
			order, _, backend := connectorExecutionAward(fixture.workflow.caller)
			s := &Solver{recipeExecutors: []common.Address{fixture.reader.recipeExecutor}, workflow: fixture.workflow, backend: backend, spends: newSpendLedger()}
			if err := s.Initialize(t.Context()); err != nil {
				t.Fatal(err)
			}
			s.routes[common.HexToAddress("0x44")] = s.routes[fixture.reader.recipeExecutor]
			listed := []discounts.ListItem{{Adapter: vlt.Hex(), RecipeExecutor: hint, TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectorDiscountID().Hex(), MaxAssets: "900"}}
			intent, err := s.prepare(t.Context(), awardRow(t, order), listed)
			if err != nil || (intent != nil) != (hint != "") {
				t.Fatalf("intent=%+v err=%v", intent, err)
			}
			if hint == "" && fixture.reader.outerCalls != 0 {
				t.Fatal("ambiguous capacity reached settlement simulation")
			}
		})
	}
}

func TestPollerRetainsWholeAwardAndOwnsOnlyOneFill(t *testing.T) {
	fixture := connectorExecutionFixture(t, connectorDiscountFixture())
	order, _, backend := connectorExecutionAward(fixture.workflow.caller)
	next := awardRow(t, order)
	next.OrderId = "next-award"
	backend.rows = []rfqbackend.OrdersResponseOrdersInner{awardRow(t, order), next}
	backend.listed = &discounts.List{Discounts: []discounts.ListItem{
		{Adapter: common.HexToAddress("0x01").Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectorDiscountID().Hex(), MaxAssets: "99999"},
		{Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectorDiscountID().Hex(), MaxAssets: "900"},
	}}
	sender := &fakeSender{}
	s := &Solver{recipeExecutors: []common.Address{common.HexToAddress("0x43")}, workflow: fixture.workflow, backend: backend, sender: sender, spends: newSpendLedger()}
	sender.before = func(req transaction.ExecutionRequest) {
		if s.intent == nil || !bytes.Equal(s.intent.Data, req.Data) || s.intent.Deadline.IsZero() {
			t.Fatal("fill admitted before RAM intent")
		}
		if reserved := s.Reservations(nil)[s.intent.Spend.CapacityID]; reserved == nil || reserved.Cmp(big.NewInt(456)) != 0 {
			t.Fatal("fill admitted before reserving its signed output")
		}
	}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if fixture.reader.outerCalls != 1 || sender.polls != 0 || sender.request.To != fixture.workflow.caller || !sender.Busy() {
		t.Fatal("award split, unconfigured venue used or pending work released")
	}
	reads, request, intent := backend.reads, sender.request, *s.intent
	backend.rows, backend.listed, backend.discount = nil, nil, nil // Signed terms may disappear.
	if err := s.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if backend.reads != reads || sender.submits != 1 || !reflect.DeepEqual(intent, *s.intent) {
		t.Fatal("pending fill was rebuilt or its deadline changed")
	}
	sender.result = fillResult()
	sender.duringPoll = func() {
		if reserved := s.Reservations(nil)[s.intent.Spend.CapacityID]; reserved == nil || reserved.Cmp(big.NewInt(456)) != 0 {
			t.Fatal("manager release lost the spend before canonical accounting")
		}
	}
	if err := s.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if backend.reads != reads || !reflect.DeepEqual(request, sender.request) || sender.submits != 1 || s.intent != nil || sender.Busy() {
		t.Fatal("canonical fill was rebuilt or its ownership was not released")
	}
	if !s.spends.finalized("award") {
		t.Fatal("canonical accounting did not remember the completed award")
	}
	backend.rows = []rfqbackend.OrdersResponseOrdersInner{awardRow(t, order), awardRow(t, order)}
	backend.listed = &discounts.List{}
	if err := s.tick(t.Context()); err != nil || sender.submits != 1 {
		t.Fatal("completed award was submitted again", err)
	}
	// A different award waits for canonical settlement, then starts on the same single lane.
	backend.rows = append(backend.rows, next)
	backend.listed = &discounts.List{Discounts: []discounts.ListItem{{Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectorDiscountID().Hex(), MaxAssets: "900", RecipeExecutor: common.HexToAddress("0x43").Hex()}}}
	backend.discount = connectorDiscountFixture()
	sender.result = nil
	sender.before = nil
	if err := s.tick(t.Context()); err != nil || sender.submits != 2 || s.intent == nil || s.intent.OrderID != next.OrderId {
		t.Fatal("next award did not start after the first completed", err)
	}
	if err := s.tick(t.Context()); err != nil || sender.submits != 2 {
		t.Fatal("pending second award admitted parallel work", err)
	}
}

func TestWholeAwardRequiresVenueAndAdvertisedCapacity(t *testing.T) {
	for _, capacity := range []string{"advertised", "onchain"} {
		t.Run(capacity, func(t *testing.T) {
			f := connectorExecutionFixture(t, connectorDiscountFixture())
			order, selected, backend := connectorExecutionAward(f.workflow.caller)
			if capacity == "advertised" {
				selected.MaxAssets = big.NewInt(455)
			} else {
				f.reader.remaining = big.NewInt(455)
			}
			if _, err := f.workflow.build(t.Context(), order, nil, selected, backend); err == nil || !strings.Contains(err.Error(), "whole award") {
				t.Fatalf("capacity accepted: %v", err)
			}
			if f.reader.outerCalls != 0 {
				t.Fatal("underfunded award simulated or submitted")
			}
		})
	}
}

func TestPrepareCountsHeaderLatencyAgainstDeadline(t *testing.T) {
	f := connectorExecutionFixture(t, connectorDiscountFixture())
	chainTime := time.Now().Add(time.Hour).Truncate(time.Second)
	f.reader.header.Time = uint64(chainTime.Unix())
	order, _, backend := connectorExecutionAward(f.workflow.caller)
	order.Request.Deadline = big.NewInt(chainTime.Add(10 * time.Second).Unix())
	s := Solver{recipeExecutors: []common.Address{common.HexToAddress("0x43")}, workflow: f.workflow, backend: backend}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.reader.headerDelay = 250 * time.Millisecond
	f.reader.headerStarted = time.Time{}
	intent, err := s.prepare(t.Context(), awardRow(t, order), []discounts.ListItem{{Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectorDiscountID().Hex(), MaxAssets: "900", RecipeExecutor: common.HexToAddress("0x43").Hex()}})
	if err != nil || intent == nil {
		t.Fatalf("prepare: intent=%v err=%v", intent, err)
	}
	if intent.Deadline.After(f.reader.headerStarted.Add(10*time.Second + 100*time.Millisecond)) {
		t.Fatalf("header latency extended deadline: got %v, header started %v", intent.Deadline, f.reader.headerStarted)
	}
}

func TestUncertainFillKeepsIntent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result *transaction.ExecutionResult
		err    error
	}{
		{"pending", nil, nil},
		{"cancelled wait", nil, context.Canceled},
		{"canonical check fails", nil, errors.New("reorg")},
		{"invalid execution identity", &transaction.ExecutionResult{Hash: common.HexToHash("0x77")}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			intent := &fillIntent{OrderID: "owned", To: vlt, Data: []byte{1}, Deadline: time.Now()}
			sender := &fakeSender{busy: true, result: tc.result, pollErr: tc.err}
			s := &Solver{sender: sender, intent: intent}
			_ = s.tick(t.Context())
			if s.intent != intent || sender.submits != 0 || sender.polls != 1 {
				t.Fatal("uncertain work lost ownership")
			}
		})
	}
}

func TestSubmissionErrorRetainsOnlySignedWork(t *testing.T) {
	for _, unsigned := range []bool{false, true} {
		t.Run(map[bool]string{false: "signed ambiguity", true: "before signing"}[unsigned], func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectorDiscountFixture())
			order, _, backend := connectorExecutionAward(fixture.workflow.caller)
			backend.rows = []rfqbackend.OrdersResponseOrdersInner{awardRow(t, order)}
			backend.listed = &discounts.List{Discounts: []discounts.ListItem{{Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectorDiscountID().Hex(), MaxAssets: "900", RecipeExecutor: common.HexToAddress("0x43").Hex()}}}
			sender := &fakeSender{unsigned: unsigned, submitErr: context.DeadlineExceeded}
			s := &Solver{recipeExecutors: []common.Address{common.HexToAddress("0x43")}, workflow: fixture.workflow, backend: backend, sender: sender, spends: newSpendLedger()}
			if err := s.Initialize(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := s.tick(t.Context()); !errors.Is(err, context.DeadlineExceeded) || (s.intent == nil) != unsigned {
				t.Fatalf("signed ownership changed: intent=%v err=%v", s.intent, err)
			}
			capacityID := liquidlane.NewCapacityID(1, fixture.reader.recipeAccount, tOut)
			reserved := s.Reservations(nil)[capacityID]
			if unsigned && len(s.Reservations(nil)) != 0 || !unsigned && (reserved == nil || reserved.Cmp(big.NewInt(456)) != 0) {
				t.Fatal("submission error released signed capacity or retained an unsigned reservation")
			}
			if !unsigned {
				reads := backend.reads
				if err := s.tick(t.Context()); err != nil || backend.reads != reads || sender.submits != 1 {
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

// A completed fill advances directly to the next award; pending work uses polling.
func TestRunContinuesToNextAwardAfterConfirmation(t *testing.T) {
	f := connectorExecutionFixture(t, connectorDiscountFixture())
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	s := &Solver{
		recipeExecutors: []common.Address{f.reader.recipeExecutor},
		workflow:        f.workflow,
		backend:         &nextAwardBackend{cancel: cancel},
		sender:          &fakeSender{busy: true, result: fillResult()},
		intent:          &fillIntent{OrderID: "pending"},
	}
	if err := s.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("confirmed fill waited for the polling timer before next award", err)
	}
}

type nextAwardBackend struct {
	fakeBackend

	cancel context.CancelFunc
}

func (b *nextAwardBackend) orders(context.Context, string) ([]rfqbackend.OrdersResponseOrdersInner, error) {
	b.cancel()
	return nil, nil
}
