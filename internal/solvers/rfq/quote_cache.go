package rfq

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// Native discount solvers without an explicit adapter list discover identities
// outside requests. Only this Run-owned loop writes; quotes read immutable cache
// snapshots. The backend client and chain client support concurrent requests.
func (s *Solver) refreshNativeQuoteAdapters(ctx context.Context) (err error) {
	ctx, end := tracer.Start(ctx, "rfq.quote.accounts")
	defer func() { end(err) }()
	listed, err := s.exec.backend.listDiscounts(ctx)
	if err != nil {
		return errors.Errorf("discover quote adapters: %w", err)
	}
	if listed == nil {
		return errors.New("discover quote adapters: empty response")
	}
	adapters := make([]common.Address, 0, len(listed.Discounts))
	for _, item := range listed.Discounts {
		if item.DiscountPrecision == "100000000" || !common.IsHexAddress(item.Adapter) {
			continue
		}
		adapter := common.HexToAddress(item.Adapter)
		if adapter == (common.Address{}) {
			continue
		}
		adapters = append(adapters, adapter)
	}
	return s.exec.reader.warmQuoteAdapters(ctx, adapters)
}

func (s *Solver) refreshNativeQuoteAdaptersLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.refreshNativeQuoteAdapters(ctx); err != nil {
				observability.ErrorUnlessCanceled(ctx, err, "quote account discovery failed")
			}
		}
	}
}
