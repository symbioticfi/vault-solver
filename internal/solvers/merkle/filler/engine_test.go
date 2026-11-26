package filler

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler/fillertest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func testCapacityID() liquidlane.CapacityID {
	return liquidlane.NewCapacityID(1, connectortest.Venue, connectortest.TokenOut)
}

func testSpend() connector.Spend {
	return connector.Spend{Amounts: liquidlane.CapacityReservations{testCapacityID(): big.NewInt(100)}, Deadline: 100}
}

func pendingEngine(sender *fillertest.Sender, intent *Intent) *Engine {
	e := &Engine{Name: "test", Sender: sender, Spends: connector.NewSpendLedger(), Workflow: &connector.Workflow{Filler: connectortest.Venue, Chain: &replayChain{}}}
	e.intent.Store(intent)
	return e
}

func TestCanonicalFillRetainsReservationUntilSnapshotsReflectSpend(t *testing.T) {
	for _, outcome := range []string{"pending", "confirmed", "reverted", "unknown", "invalid receipt"} {
		t.Run(outcome, func(t *testing.T) {
			sender := &fillertest.Sender{IsBusy: true}
			switch outcome {
			case "confirmed":
				sender.Result = fillertest.Confirmed()
			case "reverted":
				sender.Result = fillertest.Confirmed()
				sender.Result.Reverted = true
			case "unknown":
				sender.PollErr = errors.New("receipt RPC unavailable")
			case "invalid receipt":
				sender.Result = fillertest.Confirmed()
				sender.Result.Execution.BlockNumber = nil
			}
			spend := testSpend()
			e := pendingEngine(sender, &Intent{OrderID: "award", Spend: spend})
			if err := e.Spends.Reserve("award", spend); err != nil {
				t.Fatal(err)
			}
			settled, err := e.Poll(t.Context())
			if (err != nil) != (outcome == "unknown" || outcome == "invalid receipt") {
				t.Fatal(err)
			}
			pending := outcome == "pending" || outcome == "unknown" || outcome == "invalid receipt"
			if (e.Pending() != nil) != pending || (settled == nil) != pending || e.Spends.Finalized("award") != (outcome == "confirmed") {
				t.Fatal("fill accounting or ownership changed incorrectly")
			}
			if settled != nil && (settled.OrderID != "award" || settled.Block != 8 || settled.Reverted != (outcome == "reverted")) {
				t.Fatalf("outcome = %+v", settled)
			}
			for _, block := range []uint64{0, 7, 8, 999} {
				inventory := []liquidlane.Inventory{{Route: liquidlane.Route{CapacityID: testCapacityID()}, MaxAssets: big.NewInt(1000), BlockNumber: block}}
				want := int64(0)
				if pending || outcome == "confirmed" && block < 8 {
					want = 100
				}
				reserved := e.Reservations(inventory)[testCapacityID()]
				if reserved == nil {
					reserved = new(big.Int)
				}
				if reserved.Cmp(big.NewInt(want)) != 0 {
					t.Fatalf("block %d: reservation = %s, want %d", block, reserved, want)
				}
			}
		})
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
			intent := &Intent{OrderID: "owned", Data: []byte{1}, Deadline: time.Now()}
			sender := &fillertest.Sender{IsBusy: true, Result: tc.result, PollErr: tc.err}
			e := pendingEngine(sender, intent)
			_, _ = e.Poll(t.Context())
			if e.Pending() != intent || sender.Submits != 0 || sender.Polls != 1 {
				t.Fatal("uncertain work lost ownership")
			}
		})
	}
}

func TestSubmitOwnsTheLaneUntilSettlement(t *testing.T) {
	sender := &fillertest.Sender{}
	e := pendingEngine(sender, nil)
	first := &Intent{OrderID: "first", Data: []byte{1}, Deadline: time.Now().Add(time.Minute), Spend: testSpend()}
	sender.Before = func(req transaction.ExecutionRequest) {
		// The sender has not taken the transaction yet: that is not a stalled lane.
		if req.To != connectortest.Venue || !e.deadline().Equal(first.Deadline) || len(e.Reservations(nil)) != 1 || e.Stalled() {
			t.Fatal("fill admitted before the lane published its intent and reservation")
		}
	}
	if err := e.Submit(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	if err := e.Submit(t.Context(), &Intent{OrderID: "second", Spend: testSpend()}); !errors.Is(err, ErrBusy) || sender.Submits != 1 ||
		!strings.Contains(err.Error(), "order first owns the signer lane") {
		t.Fatalf("second fill entered a busy lane: %v", err)
	}
	sender.Result = fillertest.Confirmed()
	settled, err := e.Await(t.Context())
	if err != nil || settled == nil || settled.OrderID != "first" || e.Pending() != nil || !e.deadline().IsZero() {
		t.Fatalf("await: outcome=%+v err=%v", settled, err)
	}
	if idle, err := e.Await(t.Context()); idle != nil || err != nil {
		t.Fatal("idle lane reported an outcome")
	}
}

func testWorkflow(t *testing.T) *connector.Workflow {
	t.Helper()
	filler := common.HexToAddress("0x10")
	terms, err := connector.ParseDiscount(connectortest.DiscountFixture(), *connectortest.DiscountID(), connectortest.Venue, connectortest.TokenIn)
	if err != nil {
		t.Fatal(err)
	}
	fixture := connectortest.NewFixture(t, terms.Swap, terms.Signature, common.HexToAddress("0x43"), filler, [4]byte{1, 2, 3, 4})
	return &connector.Workflow{Chain: fixture.Chain, ChainID: big.NewInt(1), Source: fixture.Permissions, Filler: filler, Signer: fixture.Chain.Signer}
}

func testEngine(t *testing.T, sender *fillertest.Sender) *Engine {
	t.Helper()
	workflow := testWorkflow(t)
	e := &Engine{Name: "test", Workflow: workflow, Routes: connector.NewRoutes(workflow, []common.Address{common.HexToAddress("0x43")}), Spends: connector.NewSpendLedger()}
	if sender != nil {
		e.Sender = sender
	}
	return e
}

func TestRunContinuesToNextFillAfterConfirmation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	e := testEngine(t, &fillertest.Sender{Result: fillertest.Confirmed()})
	first := &Intent{OrderID: "first", Spend: testSpend()}
	err := e.Run(ctx, func(_ context.Context, idle bool) (*Intent, error) {
		if e.Spends.Finalized(first.OrderID) && idle {
			cancel()
			return nil, nil
		}
		return first, errors.New("award reservation refresh timed out")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("confirmed fill waited for the next award poll", err)
	}
}

func TestSourcesSelectConfiguredConnectorDiscounts(t *testing.T) {
	e := testEngine(t, nil)
	if e.Sends() || e.Ready() {
		t.Fatal("quote-only engine claims a signer lane")
	}
	if err := e.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	venue, tokenIn, tokenOut := connectortest.Venue, connectortest.TokenIn, connectortest.TokenOut
	item := func(id string, mutate func(*discounts.ListItem)) discounts.ListItem {
		listed := discounts.ListItem{DiscountPrecision: connector.PrecisionText, Adapter: venue.Hex(), TokenToRedeem: tokenIn.Hex(), Collateral: tokenOut.Hex(), DiscountID: common.HexToHash(id).Hex(), MaxAssets: "900"}
		if mutate != nil {
			mutate(&listed)
		}
		return listed
	}
	listed := []discounts.ListItem{
		item("0x03", nil),
		item("0x01", func(i *discounts.ListItem) { i.DiscountPrecision = "1000000" }),
		item("0x02", func(i *discounts.ListItem) { i.Adapter = common.HexToAddress("0x77").Hex() }),
		item("0x04", func(i *discounts.ListItem) { i.TokenToRedeem = tokenOut.Hex() }),
		item("0x02", nil),
	}
	var ids []common.Hash
	for source, err := range e.Sources(listed, tokenIn, tokenOut) {
		if err != nil || source.Venue != venue || source.RecipeExecutor != common.HexToAddress("0x43") || source.Account == (common.Address{}) || source.MaxAssets.Cmp(big.NewInt(900)) != 0 {
			t.Fatalf("source=%+v err=%v", source, err)
		}
		ids = append(ids, source.DiscountID)
	}
	if len(ids) != 2 || ids[0] != common.HexToHash("0x02") || ids[1] != common.HexToHash("0x03") {
		t.Fatalf("sources = %v", ids)
	}
	for _, mutate := range []func(*discounts.ListItem){
		func(i *discounts.ListItem) { i.DiscountID = "0x00" },
		func(i *discounts.ListItem) { i.MaxAssets = "-1" },
	} {
		yielded := 0
		for _, err := range e.Sources([]discounts.ListItem{item("0x01", mutate), item("0x02", nil)}, tokenIn, tokenOut) {
			yielded++
			if err == nil {
				t.Fatal("malformed listing was skipped")
			}
		}
		if yielded != 1 {
			t.Fatalf("yielded %d items after a malformed listing", yielded)
		}
	}
}

func TestObservationBoundsSubmissionByEarliestLimit(t *testing.T) {
	e := testEngine(t, nil)
	observed, err := e.Observe(t.Context())
	if err != nil || observed.Chain.IsZero() {
		t.Fatal(observed, err)
	}
	late, early := time.Now().Add(time.Hour), time.Now().Add(time.Minute)
	deadline, ok := observed.SubmissionDeadline(late, early)
	if !ok || deadline.After(time.Now().Add(time.Minute+time.Second)) {
		t.Fatalf("deadline %v ignores the earliest limit", deadline)
	}
	if _, elapsed := observed.SubmissionDeadline(observed.Chain); elapsed {
		t.Fatal("elapsed limit accepted")
	}
	if _, unbounded := observed.SubmissionDeadline(); unbounded {
		t.Fatal("unbounded submission accepted")
	}
}

// A stopped transaction manager can never return the receipt: the wait ends like a stopped
// native manager and the lane keeps its intent and reservation.
func TestAwaitEndsWhenLaneStopped(t *testing.T) {
	sender := &fillertest.Sender{IsBusy: true, PollErr: errors.New("transaction manager stopped")}
	spend := testSpend()
	e := pendingEngine(sender, &Intent{OrderID: "award", Spend: spend})
	if err := e.Spends.Reserve("award", spend); err != nil {
		t.Fatal(err)
	}
	e.LaneReady = func() bool { return false }
	if settled, err := e.Await(t.Context()); settled != nil || !errors.Is(err, ErrLaneStopped) {
		t.Fatalf("await on a stopped lane: outcome=%v err=%v", settled, err)
	}
	if result := e.AwaitNative(t.Context()); result.Outcome != txmanager.OutcomeTrackingStopped || result.Err == nil || result.Outcome.NonceUncertain() {
		t.Fatalf("native result = %+v", result)
	}
	if e.Pending() == nil || e.Stalled() || len(e.Reservations(nil)) != 1 {
		t.Fatal("stopped lane lost its signed work")
	}
}

// A sender that dropped the transaction without a receipt leaves the lane stalled: the wait
// ends, the reservation stays, and no new fill enters until restart.
func TestAwaitReportsStalledLane(t *testing.T) {
	for _, pollErr := range []error{nil, errors.New("nonce consumed without this request's receipt")} {
		sender := &fillertest.Sender{PollErr: pollErr}
		spend := testSpend()
		e := pendingEngine(sender, &Intent{OrderID: "award", Spend: spend})
		if err := e.Spends.Reserve("award", spend); err != nil {
			t.Fatal(err)
		}
		e.LaneReady = func() bool { return true }
		if settled, err := e.Await(t.Context()); settled != nil || !errors.Is(err, ErrLaneStalled) {
			t.Fatalf("await on a stalled lane: outcome=%v err=%v", settled, err)
		}
		if result := e.AwaitNative(t.Context()); !result.Outcome.NonceUncertain() || result.Err == nil {
			t.Fatalf("native result = %+v", result)
		}
		if !e.Stalled() || e.Pending() == nil || len(e.Reservations(nil)) != 1 {
			t.Fatal("stalled lane released its unresolved spend")
		}
		if err := e.Submit(t.Context(), &Intent{OrderID: "next", Spend: testSpend()}); !errors.Is(err, ErrBusy) || sender.Submits != 0 {
			t.Fatal("stalled lane admitted another fill", err)
		}
	}
	if idle := pendingEngine(&fillertest.Sender{}, nil); idle.Stalled() {
		t.Fatal("idle lane reported as stalled")
	}
}

// Between inclusion and its confirmations the SDK reports a consumed nonce on every observation
// while the sender still owns the transaction: that is a wait, not an error.
func TestAwaitTreatsConfirmationWaitAsProgress(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		wantError bool
	}{
		{"awaiting confirmations", transaction.ErrNonceConsumed, false},
		{"receipt read fails", errors.New("receipt RPC unavailable"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var failures []string
			logger := funcr.New(func(_, args string) {
				if strings.Contains(args, `"error"=`) {
					mu.Lock()
					failures = append(failures, args)
					mu.Unlock()
				}
			}, funcr.Options{Verbosity: 1})
			ctx, cancel := context.WithTimeout(observability.WithLogger(t.Context(), logger), 50*time.Millisecond)
			defer cancel()
			e := pendingEngine(&fillertest.Sender{IsBusy: true, PollErr: tc.err}, &Intent{OrderID: "award"})
			e.LaneReady = func() bool { return true }
			if settled, err := e.Await(ctx); settled != nil || !errors.Is(err, context.DeadlineExceeded) || e.Pending() == nil {
				t.Fatalf("await: outcome=%v err=%v", settled, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if (len(failures) > 0) != tc.wantError {
				t.Fatalf("error logs = %v, want error=%v", failures, tc.wantError)
			}
			if tc.wantError && !strings.Contains(failures[0], `"orderId"="award"`) {
				t.Fatalf("receipt error is missing the award ID: %v", failures)
			}
		})
	}
}

// replayChain answers the replay of a mined fill like the host chain client.
type replayChain struct {
	recipes.Reader

	err   error
	block *big.Int
	call  ethereum.CallMsg
}

func (c *replayChain) ReplayMinedCall(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	c.call, c.block = msg, block
	return nil, c.err
}

func TestRevertedFillNamesItsReplayedRevert(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"decoded", protocolio.Failure("call contract on read RPC", connectortest.Revert("InvalidDiscount()")), "replay of block 8 reverted with InvalidDiscount()"},
		{"no payload", &connectortest.RevertError{}, "replay of block 8 reverted: execution reverted"},
		{"replay passes", nil, "replay of block 8 did not revert (an earlier transaction in the block changed the state)"},
		{"replay fails", errors.New("connection refused"), "revert reason unavailable: replay of block 8 failed: connection refused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sender := &fillertest.Sender{IsBusy: true, Result: fillertest.Confirmed()}
			sender.Result.Reverted = true
			intent := &Intent{OrderID: "award", Data: []byte{1, 2, 3, 4}, Spend: testSpend()}
			e := pendingEngine(sender, intent)
			chain := &replayChain{err: tc.err}
			e.Workflow.Chain, e.Workflow.Signer = chain, common.HexToAddress("0x5")
			if err := e.Spends.Reserve("award", intent.Spend); err != nil {
				t.Fatal(err)
			}
			got := e.AwaitNative(t.Context())
			if got.Outcome != txmanager.OutcomeReverted || !strings.HasSuffix(got.Err.Error(), "reverted in block 8: "+tc.want) {
				t.Fatalf("result = %+v, want revert %q", got, tc.want)
			}
			if chain.block.Uint64() != 8 || chain.call.From != e.Workflow.Signer || *chain.call.To != e.Workflow.Filler || !bytes.Equal(chain.call.Data, intent.Data) {
				t.Fatalf("replay call = %+v at block %v", chain.call, chain.block)
			}
		})
	}
}
