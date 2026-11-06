package uniswapx

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

func TestRefreshDiscountCancelLogsNoError(t *testing.T) {
	var errorLines []string
	log := funcr.NewJSON(func(line string) {
		if !strings.Contains(line, `"level":`) { // funcr omits level on Error lines
			errorLines = append(errorLines, line)
		}
	}, funcr.Options{Verbosity: 1})
	ctx, cancel := context.WithCancel(observability.WithLogger(t.Context(), log))
	cancel()
	provider := &fakeDiscountProvider{listErr: errors.Errorf("list discounts: %w", context.Canceled)}
	solver := quoteModeSolver(&quoteModeReader{now: time.Unix(1_000, 0)}, provider)
	_ = solver.refreshQuoteState(ctx, nil)
	if provider.listCalls == 0 {
		t.Fatal("discount list was not reached")
	}
	if len(errorLines) != 0 {
		t.Fatalf("canceled refresh logged at Error: %v", errorLines)
	}
}
