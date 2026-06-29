package rfq

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

// priceReader is the on-chain pricing surface the quote path needs (satisfied by *reader). It's an
// interface so the quote/HTTP logic can be unit-tested without a chain backend.
type priceReader interface {
	tokenDecimals(ctx context.Context, token common.Address) (int, error)
	amountsOut(ctx context.Context, tokenIn common.Address, inventories []solverInventory, amount *big.Int) (map[common.Address]*big.Int, error)
}

// quoteService prices a backend RFQ request and persists the chosen strategy by quoteId. It is
// safe for concurrent use (the HTTP server serves quotes in parallel): its dependencies — the
// reader cache and the store — are individually synchronized, and it holds no mutable state itself.
type quoteService struct {
	chainID   int64
	executor  common.Address
	whitelist adapterWhitelist // nil disables adapter filtering
	// tokensToQuote scopes which input tokens are quotable: "all" (default), "permissioned", or
	// "permissionless" (see Config.TokensToQuote); evaluated against permissionedTokens.
	tokensToQuote      string
	permissionedTokens map[common.Address]bool
	reader             priceReader
	store              *store
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

	tokenInDecimals, err := qs.reader.tokenDecimals(ctx, req.TokenIn)
	if err != nil {
		return nil, errors.Errorf("quote: tokenIn decimals: %w", err)
	}

	// Only asset == tokenOut is fillable, so we price exactly those inventories (each priced through
	// its asset-group's representative adapter — see reader.amountsOut).
	matching := matchingInventories(inv, req.TokenOut)
	oracle, err := qs.reader.amountsOut(ctx, req.TokenIn, matching, req.Amount)
	if err != nil {
		return nil, errors.Errorf("quote: adapter getAmountOut: %w", err)
	}

	best := selectBestStrategy(req, inv, tokenInDecimals, oracle, qs.now())
	if best == nil {
		qs.log.V(1).Info("declining quote: no viable strategy", "quoteId", q.QuoteID)
		return nil, nil
	}
	qs.store.putStrategy(best)

	qs.log.V(1).Info("quoted",
		"quoteId", q.QuoteID, "amountIn", req.Amount.String(),
		"amountOut", best.QuotedAmountOut.String(), "legs", len(best.Legs))

	return &quoteResponse{
		ChainID:   qs.chainID,
		AmountIn:  req.Amount.String(),
		AmountOut: best.QuotedAmountOut.String(),
		Filler:    lowerAddr(qs.executor),
		RequestID: q.RequestID,
		Swapper:   lowerAddr(common.HexToAddress(q.Swapper)), // backend payloads use lowercase addresses
		TokenIn:   lowerAddr(req.TokenIn),
		TokenOut:  lowerAddr(req.TokenOut),
		QuoteID:   q.QuoteID,
	}, nil
}

// matchingInventories returns the inventories whose asset equals tokenOut (the only ones this filler
// can source), so the oracle prices exactly the asset-groups the selector will consider.
func matchingInventories(inv []solverInventory, tokenOut common.Address) []solverInventory {
	out := make([]solverInventory, 0, len(inv))
	for _, v := range inv {
		if v.Asset != tokenOut {
			continue
		}
		out = append(out, v)
	}
	return out
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
