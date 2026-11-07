package txmanager

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/trace"
)

func TestAfterResultFollowsPublication(t *testing.T) {
	for _, lifecycle := range []string{"initial failure", "pending completion"} {
		t.Run(lifecycle, func(t *testing.T) {
			results := make(chan Result, 1)
			var notifications int
			result := Result{Hash: common.HexToHash("0x1234"), Outcome: OutcomeConfirmed}
			req := cloneRequest(Request{AfterResult: func(got Result) {
				notifications++
				select {
				case published := <-results:
					if published.Hash != got.Hash || published.Outcome != got.Outcome {
						t.Fatal("notification disagrees with published result")
					}
				default:
					t.Fatal("notification preceded result publication")
				}
			}})
			if lifecycle == "initial failure" {
				deliverJobResult(job{req: req, res: results, span: trace.SpanFromContext(t.Context())}, result)
			} else {
				pending := &pendingTransaction{req: req, result: results}
				if !pending.deliver(result) || pending.deliver(result) {
					t.Fatal("pending result was not delivered exactly once")
				}
			}
			if notifications != 1 {
				t.Fatalf("notifications=%d", notifications)
			}
		})
	}
}

func TestAfterResultFromInitialLifecycle(t *testing.T) {
	m := New(newMockBackend(), mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, PollInterval: time.Millisecond}, logr.Discard())
	startManagerForTest(t, m)
	notified := make(chan Result, 1)
	result := m.Send(t.Context(), Request{
		To: common.HexToAddress("0x1234"), GasLimit: 21000,
		AfterResult: func(result Result) { notified <- result },
	})
	if result.Outcome != OutcomeConfirmed {
		t.Fatalf("result=%+v", result)
	}
	select {
	case notification := <-notified:
		if notification.Hash != result.Hash || notification.Outcome != result.Outcome {
			t.Fatalf("notification=%+v result=%+v", notification, result)
		}
	case <-time.After(time.Second):
		t.Fatal("initial lifecycle did not notify")
	}
}
