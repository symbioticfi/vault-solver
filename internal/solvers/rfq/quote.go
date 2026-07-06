package rfq

import (
	"context"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategytypes"
)

// quoteService prices a backend RFQ request and persists the chosen strategy by quoteId. It is
// safe for concurrent use (the HTTP server serves quotes in parallel): its dependencies — the reader
// cache and the store — are individually synchronized, and it holds no mutable state itself.
type quoteService struct {
	chainID   int64
	executor  common.Address
	whitelist adapterWhitelist // nil disables adapter filtering
	// tokensToQuote scopes which input tokens are quotable: "all" (default), "permissioned", or
	// "permissionless" (see Config.TokensToQuote); evaluated against permissionedTokens.
	tokensToQuote      string
	permissionedTokens map[common.Address]bool
	strategy           strategytypes.Strategy
	log                logr.Logger
	now                func() time.Time
}

// quote returns a priced quote, or nil (→ HTTP 204) when the request is well-formed but this filler
// can't quote it (wrong type/chain, no whitelisted adapter, no matching asset, or no viable
// strategy). An error is returned only for malformed input or a failed chain read.
func (qs *quoteService) quote(ctx context.Context, q *quoteRequest) (*quoteResponse, error) {
	parsed, err := q.toStrategy(qs.chainID)
	if err != nil {
		return nil, &badRequestError{errors.Errorf("parse request: %w", err)}
	}
	if parsed == nil {
		qs.log.V(1).Info("declining quote: not quotable", "quoteId", q.QuoteID, "type", q.Type)
		return nil, nil
	}
	if !qs.quotesTokenIn(parsed.req.TokenIn) {
		qs.log.V(1).Info("declining quote: input token out of scope",
			"quoteId", q.QuoteID, "tokenIn", lowerAddr(parsed.req.TokenIn), "scope", qs.tokensToQuote)
		return nil, nil
	}
	req, inv := parsed.req, qs.whitelist.filter(parsed.inv)
	if len(inv) == 0 {
		qs.log.V(1).Info("declining quote: no whitelisted adapters", "quoteId", q.QuoteID)
		return nil, nil
	}

	input := newQuoteInput(qs.chainID, qs.executor, req, inv, nil, qs.now())
	out, err := qs.strategy.DecideQuote(ctx, input)
	if err != nil {
		return nil, errors.Errorf("quote: strategy: %w", err)
	}
	if out.Decision == strategytypes.DecisionDecline {
		qs.log.V(1).Info("declining quote: no viable strategy", "quoteId", q.QuoteID)
		return nil, nil
	}
	if out.QuotedAmountOut == nil {
		return nil, errors.New("quote: strategy returned quote without amountOut")
	}

	qs.log.V(1).Info("quoted",
		"quoteId", q.QuoteID, "amountIn", req.Amount.String(),
		"amountOut", out.QuotedAmountOut.String(), "legs", len(out.Legs))

	return &quoteResponse{
		ChainID:   qs.chainID,
		AmountIn:  req.Amount.String(),
		AmountOut: out.QuotedAmountOut.String(),
		Filler:    lowerAddr(qs.executor),
		RequestID: q.RequestID,
		Swapper:   lowerAddr(common.HexToAddress(q.Swapper)), // backend payloads use lowercase addresses
		TokenIn:   lowerAddr(req.TokenIn),
		TokenOut:  lowerAddr(req.TokenOut),
		QuoteID:   q.QuoteID,
	}, nil
}

// lowerAddr renders an address as lowercase hex; RFQ backend payloads use lowercase addresses.
func lowerAddr(a common.Address) string { return strings.ToLower(a.Hex()) }

// quotesTokenIn reports whether this filler's TokensToQuote scope admits the request's input token:
// "permissioned" admits only tokens in permissionedTokens, "permissionless" admits only those not in
// it, and "all" (or any unset value, for hand-built services) admits every token.
func (qs *quoteService) quotesTokenIn(tokenIn common.Address) bool {
	switch qs.tokensToQuote {
	case tokensToQuotePermissioned:
		return qs.permissionedTokens[tokenIn]
	case tokensToQuotePermissionless:
		return !qs.permissionedTokens[tokenIn]
	default:
		return true
	}
}
