package uniswapx

import (
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// A selection is only a preference. Input amount and token pair must match before it
// is reused; the signed order and fresh chain reads remain authoritative.
type quoteSelection struct {
	candidate         liquidlane.CandidateID
	tokenIn, tokenOut common.Address
	amountIn          string
	expiresAt         time.Time
}

func (s *Solver) rememberSelection(response quoteResponse, now time.Time) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.selections == nil {
		s.selections = make(map[string]quoteSelection)
	}
	var oldestKey string
	var oldest time.Time
	for key, selection := range s.selections {
		if !selection.expiresAt.After(now) {
			delete(s.selections, key)
			continue
		}
		if oldest.IsZero() || selection.expiresAt.Before(oldest) {
			oldestKey, oldest = key, selection.expiresAt
		}
	}
	limit := s.cfg.QuoteServer.MaxSelections
	if limit <= 0 {
		limit = defaultMaxSelections
	}
	if _, exists := s.selections[response.QuoteID]; !exists && len(s.selections) >= limit {
		delete(s.selections, oldestKey)
	}
	ttl := s.cfg.QuoteServer.SelectionTTL
	if ttl <= 0 {
		ttl = defaultSelectionTTL
	}
	s.selections[response.QuoteID] = quoteSelection{
		candidate: response.selectedCandidate,
		tokenIn:   common.HexToAddress(response.TokenIn), tokenOut: common.HexToAddress(response.TokenOut),
		amountIn: response.AmountIn, expiresAt: now.Add(ttl),
	}
}

func (s *Solver) preferredSource(order *resolvedOrder, now time.Time) liquidlane.CandidateID {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	selection, exists := s.selections[order.QuoteID]
	if !exists || !selection.expiresAt.After(now) || selection.tokenIn != order.TokenIn ||
		selection.tokenOut != order.TokenOut || selection.amountIn != order.AmountIn.String() {
		return ""
	}
	return selection.candidate
}
