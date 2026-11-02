package lifi

import (
	"context"
	"math/big"
	"strconv"
	"time"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// A successful header completes one full local snapshot; an empty snapshot clears prior pairs.
// Failed submissions are uncertain, not a new acknowledged state. Consumers must use the latest
// cohort per process and check its row count, then apply quote expiry (Unix seconds). No pair cache
// or metric labels are maintained here, and observation never performs I/O or changes quote state.
func (s *Solver) observeStandingQuoteSnapshot(ctx context.Context, state *quoteState, inventory []liquidlane.Inventory, submitErr error) {
	log := observability.Log(ctx)
	snapshotID := strconv.FormatInt(time.Now().UnixNano(), 10)
	if submitErr != nil {
		log.Info("standing quote snapshot", "chainId", s.chainID, "snapshotId", snapshotID, "snapshotState", "uncertain")
		return
	}
	pairs, quotes, ranges := 0, 0, 0
	for key, pair := range state.active {
		if !s.standingPairInScope(key, inventory) {
			continue
		}
		maximum := new(big.Int)
		for _, quote := range pair.quotes {
			for _, r := range quote.Ranges {
				if r.MaxAmount != nil && r.MaxAmount.Cmp(maximum) > 0 {
					maximum.Set(r.MaxAmount)
				}
			}
		}
		pairRanges := quoteRangeCount(pair.quotes)
		if len(pair.quotes) == 0 || pairRanges == 0 || maximum.Sign() <= 0 {
			continue
		}
		// Symbols are optional cached display metadata. Decimals come from the exact trusted
		// inventory direction used for this quote, so stale cache values cannot override them.
		var metadata []any
		fields := s.tokenMetadata.PairLogFields(key.fromAsset, key.toAsset)
		for i := 0; i < len(fields); i += 2 {
			if fields[i] != "tokenInDecimals" && fields[i] != "tokenOutDecimals" {
				metadata = append(metadata, fields[i], fields[i+1])
			}
		}
		if key.fromDecimals >= 0 && key.fromDecimals <= 255 {
			metadata = append(metadata, "tokenInDecimals", key.fromDecimals)
		}
		if key.toDecimals >= 0 && key.toDecimals <= 255 {
			metadata = append(metadata, "tokenOutDecimals", key.toDecimals)
		}
		earliest, latest := quoteExpiryBounds(pair.quotes)
		log.WithValues(metadata...).Info("standing quote pair state",
			"chainId", s.chainID, "snapshotId", snapshotID,
			"tokenIn", key.fromAsset.Hex(), "tokenOut", key.toAsset.Hex(),
			"pairInScope", true, "pairScope", "directed_route", "pairScopeSource", "current_inventory",
			"maxInputAmount", maximum.String(), "quoteCount", len(pair.quotes), "rangeCount", pairRanges,
			"earliestExpiry", earliest, "latestExpiry", latest,
		)
		pairs++
		quotes += len(pair.quotes)
		ranges += pairRanges
	}
	snapshotState := "reconciled"
	if len(state.active) == 0 {
		snapshotState = "suspended"
	}
	// Emit the header last as the completion marker. Counts cover exactly the emitted scoped
	// pair+decimal records, not all strategy output or remote order-server inventory.
	log.Info("standing quote snapshot", "chainId", s.chainID, "snapshotId", snapshotID,
		"snapshotState", snapshotState, "supportedPairs", pairs, "quoteCount", quotes, "rangeCount", ranges)
}

func (s *Solver) standingPairInScope(key quotePairKey, inventory []liquidlane.Inventory) bool {
	if s.pairScopeSource(key.fromAsset, key.toAsset, inventory) == "" {
		return false
	}
	for _, item := range inventory {
		if item.TokenIn == key.fromAsset && item.TokenOut == key.toAsset &&
			item.TokenInDecimals == key.fromDecimals && item.TokenOutDecimals == key.toDecimals {
			return true
		}
	}
	return false
}
