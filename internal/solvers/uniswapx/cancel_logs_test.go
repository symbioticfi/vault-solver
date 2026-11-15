package uniswapx

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// canceledLogContext returns a canceled context whose logger records every line.
func canceledLogContext(t *testing.T) (context.Context, *[]string) {
	t.Helper()
	lines := new([]string)
	log := funcr.NewJSON(func(line string) { *lines = append(*lines, line) }, funcr.Options{Verbosity: 1})
	ctx, cancel := context.WithCancel(observability.WithLogger(t.Context(), log))
	cancel()
	return ctx, lines
}

// assertCancelLogged fails unless msg was logged at V(1) and nothing at Error (funcr omits level there).
func assertCancelLogged(t *testing.T, lines []string, msg string) {
	t.Helper()
	found := false
	for _, line := range lines {
		if !strings.Contains(line, `"level":`) {
			t.Fatalf("canceled context logged at Error: %s", line)
		}
		found = found || strings.Contains(line, `"msg":"`+msg+`"`)
	}
	if !found {
		t.Fatalf("missing V(1) %q in %v", msg, lines)
	}
}

func TestRefreshDiscountCancelLogsNoError(t *testing.T) {
	ctx, lines := canceledLogContext(t)
	provider := &fakeDiscountProvider{listErr: errors.Errorf("list discounts: %w", context.Canceled)}
	solver := quoteModeSolver(&quoteModeReader{now: time.Unix(1_000, 0)}, provider)
	_ = solver.refreshQuoteState(ctx, nil)
	if provider.listCalls == 0 {
		t.Fatal("discount list was not reached")
	}
	assertCancelLogged(t, *lines, "refresh advertised discount routes")
}

func TestResolveAdvertisedAdaptersCancelLogsNoError(t *testing.T) {
	adapters := []common.Address{common.HexToAddress("0x01"), common.HexToAddress("0x02")}
	for name, set := range map[string][]common.Address{"batch": adapters, "single": adapters[:1]} {
		t.Run(name, func(t *testing.T) {
			ctx, lines := canceledLogContext(t)
			reader := &quoteModeReader{resolveErr: errors.Errorf("multicall: %w", context.Canceled)}
			solver := quoteModeSolver(reader, nil)
			if routes := solver.resolveAdvertisedAdapters(ctx, set); routes != nil {
				t.Fatalf("routes = %v", routes)
			}
			if len(reader.adapters) != len(set) {
				t.Fatalf("retried individually after cancel: %v", reader.adapters)
			}
			assertCancelLogged(t, *lines, "advertised adapter resolution canceled")
		})
	}
}
