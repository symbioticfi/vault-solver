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
	"github.com/symbioticfi/merkle-executor/sdk/state"
	"github.com/symbioticfi/merkle-executor/sdk/submission"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
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
	pending string
	request submission.Request
	result  submission.Result
	err     error
	ackErr  error
	before  func(submission.Request)
	acks    int
}

func (s *fakeSender) PendingID() string { return s.pending }
func (s *fakeSender) Send(_ context.Context, req submission.Request) (submission.Result, error) {
	if s.before != nil {
		s.before(req)
	}
	s.request = req
	s.pending = req.ID
	return s.result, s.err
}
func (s *fakeSender) Acknowledge(context.Context, string) error {
	s.acks++
	if s.ackErr != nil {
		return s.ackErr
	}
	s.pending = ""
	return nil
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

func TestPollerPersistsWholeAwardAndOwnsOnlyOneFill(t *testing.T) {
	fixture := connectorExecutionFixture(t)
	order, _, backend := connectorExecutionAward(fixture.workflow.caller)
	backend.rows = []rfqbackend.OrdersResponseOrdersInner{awardRow(t, order), awardRow(t, order)}
	backend.listed = &discounts.List{Discounts: []discounts.ListItem{
		{Adapter: common.HexToAddress("0x01").Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectorDiscountID().Hex(), MaxAssets: "99999"},
		{Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectorDiscountID().Hex(), MaxAssets: "900"},
	}}
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sender := &fakeSender{result: submission.Result{Pending: true}}
	sender.before = func(req submission.Request) {
		var saved fillIntent
		found, err := store.Load(intentKey, &saved)
		if err != nil || !found || saved.ID != req.ID || !bytes.Equal(saved.Data, req.Data) || saved.Deadline.IsZero() {
			t.Fatalf("fill admitted before durable intent: %v", err)
		}
	}
	s := &Solver{workflow: fixture.workflow, backend: backend, sender: sender, store: store}
	if err := s.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if fixture.reader.outerCalls != 1 || sender.acks != 0 || sender.request.To != fixture.workflow.caller {
		t.Fatal("award split, unconfigured venue used or pending work released")
	}
	reads, request := backend.reads, sender.request
	// Reload the original intent as startup does; API terms may disappear after signing.
	var restored fillIntent
	if _, err := store.Load(intentKey, &restored); err != nil {
		t.Fatal(err)
	}
	restarted := &Solver{workflow: fixture.workflow, backend: backend, sender: sender, store: store, intent: restored}
	sender.result = submission.Result{Confirmed: true, Hash: common.HexToHash("0x77")}
	if err := restarted.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if backend.reads != reads || !reflect.DeepEqual(request, sender.request) || sender.acks != 1 {
		t.Fatal("recovery rebuilt the fill or queried the backend")
	}
	var removed fillIntent
	if found, err := store.Load(intentKey, &removed); err != nil || found {
		t.Fatal("terminal intent not cleared", err)
	}
}

func TestWholeAwardRequiresVenueAndAdvertisedCapacity(t *testing.T) {
	for _, capacity := range []string{"advertised", "onchain"} {
		t.Run(capacity, func(t *testing.T) {
			f := connectorExecutionFixture(t)
			order, selected, backend := connectorExecutionAward(f.workflow.caller)
			if capacity == "advertised" {
				selected.MaxAssets = big.NewInt(455)
			} else {
				f.reader.remaining = big.NewInt(455)
			}
			if _, _, err := f.workflow.build(t.Context(), order, nil, selected, backend); err == nil || !strings.Contains(err.Error(), "whole award") {
				t.Fatalf("capacity accepted: %v", err)
			}
			if f.reader.outerCalls != 0 {
				t.Fatal("underfunded award simulated or submitted")
			}
		})
	}
}

func TestUncertainOrUnacknowledgedFillKeepsIntent(t *testing.T) {
	for _, tc := range []struct {
		name            string
		result          submission.Result
		sendErr, ackErr error
	}{
		{"pending", submission.Result{Pending: true}, nil, nil},
		{"cancelled wait", submission.Result{Pending: true}, context.Canceled, nil},
		{"canonical acknowledgement fails", submission.Result{Confirmed: true}, nil, errors.New("reorg")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := state.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			intent := fillIntent{ID: "owned", To: vlt, Data: []byte{1}, Deadline: time.Now()}
			if err := store.Save(intentKey, intent); err != nil {
				t.Fatal(err)
			}
			sender := &fakeSender{pending: intent.ID, result: tc.result, err: tc.sendErr, ackErr: tc.ackErr}
			s := &Solver{sender: sender, store: store, intent: intent}
			_ = s.tick(t.Context())
			var saved fillIntent
			if found, err := store.Load(intentKey, &saved); err != nil || !found || sender.pending != intent.ID {
				t.Fatal("uncertain work lost ownership", err)
			}
		})
	}
}

func TestConfigKeepsMerkleSeparateFromAdapterSolver(t *testing.T) {
	valid := `backendUrl: https://rfq.example
stateDir: /persistent/rfq-merkle
executor: "0x0000000000000000000000000000000000000010"
curator: {url: https://curator.example/api/v1}
recipeExecutors:
  "0x0000000000000000000000000000000000000042": "0x0000000000000000000000000000000000000043"
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
		{"no recovery store", strings.ReplaceAll(valid, "stateDir: /persistent/rfq-merkle\n", ""), false},
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
