package chain

import (
	"context"
	"unicode"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
)

// IERC4626 inherits the standard ERC-20 metadata ABI; reuse its generated symbol binding.
var tokenMetadataABI = erc4626.NewIERC4626()

// WarmMetadata enriches only tokens from trusted configuration/resolved inventory. It shares the
// existing decimals cache, scoped to one immutable chain client. Callers treat errors as optional
// observability failures. Failed symbols stay unknown until restart, without repeated token RPCs.
func (d *Decimals) WarmMetadata(ctx context.Context, tokens []common.Address) error {
	if d == nil {
		return nil
	}
	return d.readMetadata(ctx, d.claimMetadata(tokens))
}

// WarmMetadataAsync learns newly supported tokens without blocking quote/fill decisions. Call only
// after trusted discovery/validation, with the solver lifecycle context (not an HTTP request).
// Claiming under the existing cache lock gives each new token one attempt, including concurrent
// refreshes; known tokens launch no goroutine/RPC. Cancellation ends in-flight optional reads.
func (d *Decimals) WarmMetadataAsync(ctx context.Context, tokens []common.Address, log logr.Logger) {
	if d == nil || ctx.Err() != nil {
		return
	}
	missing := d.claimMetadata(tokens)
	if len(missing) == 0 {
		return
	}
	go func() {
		if err := d.readMetadata(ctx, missing); err != nil {
			log.V(1).Info("optional token metadata unavailable", "error", err.Error())
		}
		if ctx.Err() != nil {
			return
		}
		// One observation per newly trusted token, independent of future quote demand. Pair
		// panels can learn names from logs without a token-labelled metric or manual address map.
		for _, token := range missing {
			if fields := d.PairLogFields(token, common.Address{}); len(fields) > 0 {
				log.WithValues(fields...).Info("trusted token metadata", "tokenIn", token.Hex())
			}
		}
	}()
}

func (d *Decimals) claimMetadata(tokens []common.Address) []common.Address {
	var missing []common.Address
	d.mu.Lock()
	if d.symbols == nil {
		d.symbols = make(map[common.Address]string)
	}
	for _, token := range tokens {
		if token == (common.Address{}) {
			continue
		}
		if _, known := d.symbols[token]; !known {
			d.symbols[token] = ""
			missing = append(missing, token)
		}
	}
	d.mu.Unlock()
	return missing
}

func (d *Decimals) readMetadata(ctx context.Context, missing []common.Address) error {
	if len(missing) == 0 {
		return nil
	}
	if _, err := d.GetMany(ctx, missing); err != nil {
		return err
	}
	calls := make([]Call, len(missing))
	for i, token := range missing {
		calls[i] = Call{Target: token, AllowFailure: true, Data: tokenMetadataABI.PackSymbol()}
	}
	results, err := d.chain.Multicall(ctx, calls)
	if err != nil {
		return err
	}
	for i, token := range missing {
		if i >= len(results) || !results[i].Success {
			continue
		}
		symbol, unpackErr := tokenMetadataABI.UnpackSymbol(results[i].ReturnData)
		if unpackErr != nil || !safeTokenSymbol(symbol) {
			continue
		}
		d.mu.Lock()
		d.symbols[token] = symbol
		d.mu.Unlock()
	}
	return nil
}

// PairLogFields never performs I/O. Unknown decimals/symbols are omitted, including for arbitrary
// inbound addresses that were not warmed from trusted configuration or inventory. Symbols are
// display metadata; the existing chain/address fields remain the token's identity.
func (d *Decimals) PairLogFields(in, out common.Address) []any {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	var fields []any
	for _, item := range []struct {
		token common.Address
		name  string
	}{{in, "tokenIn"}, {out, "tokenOut"}} {
		symbol, trusted := d.symbols[item.token]
		if !trusted {
			continue
		}
		if decimals, known := d.cache[item.token]; known {
			fields = append(fields, item.name+"Decimals", decimals)
		}
		if symbol != "" {
			fields = append(fields, item.name+"Symbol", symbol)
		}
	}
	return fields
}

func safeTokenSymbol(symbol string) bool {
	if len(symbol) == 0 || len(symbol) > 64 || !utf8.ValidString(symbol) {
		return false
	}
	for _, r := range symbol {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
