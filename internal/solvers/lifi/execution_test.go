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
	"github.com/go-logr/logr/funcr"
)

func TestOrderInboxDoesNotBlockAndPreservesOrder(t *testing.T) {
	inbox := newOrderInbox()
	const count = 5_000

	enqueued := make(chan struct{})
	go func() {
		for i := range count {
			inbox.enqueue(&submittedOrder{OrderID: strconv.Itoa(i)})
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
