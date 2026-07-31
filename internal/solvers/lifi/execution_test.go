package lifi

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestOrderInboxDoesNotBlockAndPreservesOrder(t *testing.T) {
	const count = 5_000
	inbox := newOrderInbox(count)

	enqueued := make(chan struct{})
	go func() {
		for i := range count {
			if err := inbox.enqueue(&submittedOrder{OrderID: strconv.Itoa(i)}); err != nil {
				t.Errorf("enqueue %d: %v", i, err)
				return
			}
		}
		close(enqueued)
	}()
	select {
	case <-enqueued:
	case <-time.After(time.Second):
		t.Fatal("enqueue blocked without a consumer")
	}

	orders := make(chan *submittedOrder)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- inbox.run(ctx, orders) }()
	for i := range count {
		order := <-orders
		if order.OrderID != strconv.Itoa(i) {
			t.Fatalf("order %d = %s", i, order.OrderID)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("inbox did not stop after cancellation")
	}
}

func TestParseOrderMessageIgnoresDutchAuctions(t *testing.T) {
	tests := []byte{dutchAuctionContextType, exclusiveDutchAuctionContextType}
	for _, contextType := range tests {
		t.Run(hexutil.Encode([]byte{contextType}), func(t *testing.T) {
			cfg := testLifiConfig()
			var body map[string]any
			if err := json.Unmarshal(testOrderJSON(
				t,
				cfg,
				common.HexToAddress("0x6666666666666666666666666666666666666666"),
				common.HexToAddress("0x7777777777777777777777777777777777777777"),
			), &body); err != nil {
				t.Fatalf("unmarshal order: %v", err)
			}
			output := sliceField(t, mapField(t, body, "order"), "outputs")[0].(map[string]any)
			output["context"] = hexutil.Encode([]byte{contextType})
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal order: %v", err)
			}

			var logs []string
			solver := &Solver{
				cfg:     cfg,
				chainID: 11155111,
				log:     funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{}),
			}
			if order := solver.parseOrderMessage(orderMessage{Event: orderSubmitEvent, Data: raw}); order != nil {
				t.Fatalf("parseOrderMessage() = %+v, want ignored order", order)
			}
			logged := strings.Join(logs, "\n")
			if !strings.Contains(logged, "ignored unsupported Dutch auction") ||
				!strings.Contains(logged, hexutil.Encode([]byte{contextType})) {
				t.Fatalf("unsupported auction log = %s", logged)
			}
		})
	}
}

func TestOrderInboxCoalescesQueuedReplay(t *testing.T) {
	inbox := newOrderInbox(2)
	first := &submittedOrder{OrderID: "api-1", OnChainOrderID: "chain-1"}
	if err := inbox.enqueue(first); err != nil {
		t.Fatal(err)
	}
	if err := inbox.enqueue(&submittedOrder{OrderID: "api-2", OnChainOrderID: "chain-1"}); err != nil {
		t.Fatal(err)
	}
	if len(inbox.orders) != 1 {
		t.Fatalf("queued orders = %d, want 1", len(inbox.orders))
	}
}

func TestOrderInboxRejectsOverflow(t *testing.T) {
	inbox := newOrderInbox(1)
	if err := inbox.enqueue(&submittedOrder{OrderID: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := inbox.enqueue(&submittedOrder{OrderID: "second"}); !errors.Is(err, errOrderInboxFull) {
		t.Fatalf("enqueue error = %v, want %v", err, errOrderInboxFull)
	}
}

func TestAwaitFillTreatsClosedResultChannelAsFailure(t *testing.T) {
	results := make(chan txmanager.Result)
	close(results)
	fill := &pendingFill{result: results}
	completions := make(chan fillCompletion, 1)

	awaitFill(t.Context(), fill, completions)
	completion := <-completions
	if completion.result.Err == nil {
		t.Fatal("closed transaction result channel was treated as a successful fill")
	}
}

func TestCompleteFillTreatsIncludedTransactionAsSuccess(t *testing.T) {
	var logs []string
	solver := &Solver{
		log: funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{}),
	}
	fill := &pendingFill{
		order:          &submittedOrder{OrderID: "order-1", QuoteID: "quote-1"},
		orderID:        common.HexToHash("0x1"),
		reservationKey: "order-1",
	}
	pending := &pendingFillState{byOrder: map[string]*pendingFill{"order-1": fill}}

	solver.completeFill(pending, fillCompletion{fill: fill, result: txmanager.Result{
		Outcome: txmanager.OutcomeIncludedUnconfirmed,
		Err:     errors.New("confirmation wait failed"),
	}})

	logged := strings.Join(logs, "\n")
	if pending.len() != 0 || strings.Contains(logged, `"msg":"order fill failed"`) ||
		!strings.Contains(logged, "order fill included but confirmation wait failed") {
		t.Fatalf("included completion: pending=%d logs=%s", pending.len(), logged)
	}
}
