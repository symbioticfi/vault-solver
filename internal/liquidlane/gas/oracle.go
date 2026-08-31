package gas

import (
	"context"
	"encoding/json"
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/chainlink/aggregator"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

const maxOracleDecimals = 36

var chainlinkFeed = aggregator.NewAggregatorV3()

type OracleConfig struct {
	NativeUSDFeed USDFeed
	TokenUSDFeeds map[common.Address]USDFeed
}

type USDFeed struct {
	Address common.Address
	MaxAge  time.Duration
}

type Token struct {
	Address  common.Address
	Decimals int
}

type PriceSnapshot struct {
	tokenOutPerNative map[common.Address]*big.Int
}

func NewPriceSnapshot(rates map[common.Address]*big.Int) *PriceSnapshot {
	out := make(map[common.Address]*big.Int, len(rates))
	for token, rate := range rates {
		if rate != nil {
			out[token] = new(big.Int).Set(rate)
		}
	}
	return &PriceSnapshot{tokenOutPerNative: out}
}

func (s *PriceSnapshot) TokenOutPerNative(token common.Address) *big.Int {
	if s == nil || s.tokenOutPerNative[token] == nil {
		return nil
	}
	return new(big.Int).Set(s.tokenOutPerNative[token])
}

func (s *PriceSnapshot) MarshalJSON() ([]byte, error) {
	rates := map[common.Address]*big.Int(nil)
	if s != nil {
		rates = s.tokenOutPerNative
	}
	return json.Marshal(struct {
		TokenOutPerNative map[common.Address]*big.Int `json:"tokenOutPerNative"`
	}{TokenOutPerNative: rates})
}

type multicaller interface {
	Multicall(ctx context.Context, calls []chain.Call) ([]chain.CallResult, error)
}

type OracleReader struct {
	chain multicaller
	cfg   OracleConfig
}

func NewOracleReader(c multicaller, cfg OracleConfig) (*OracleReader, error) {
	if c == nil {
		return nil, errors.New("gas oracle: chain client is required")
	}
	if cfg.NativeUSDFeed.Address == (common.Address{}) {
		return nil, errors.New("gas oracle: native USD feed is required")
	}
	if cfg.NativeUSDFeed.MaxAge <= 0 {
		return nil, errors.New("gas oracle: native USD feed max age must be positive")
	}
	if len(cfg.TokenUSDFeeds) == 0 {
		return nil, errors.New("gas oracle: at least one token USD feed is required")
	}
	feeds := make(map[common.Address]USDFeed, len(cfg.TokenUSDFeeds))
	for token, feed := range cfg.TokenUSDFeeds {
		if token == (common.Address{}) || feed.Address == (common.Address{}) {
			return nil, errors.New("gas oracle: token and feed addresses must be non-zero")
		}
		if feed.MaxAge <= 0 {
			return nil, errors.Errorf("gas oracle: token %s feed max age must be positive", token.Hex())
		}
		feeds[token] = feed
	}
	cfg.TokenUSDFeeds = feeds
	return &OracleReader{chain: c, cfg: cfg}, nil
}

func (r *OracleReader) ValidateTokens(tokens []Token) error {
	decimals := make(map[common.Address]int, len(tokens))
	for _, token := range tokens {
		if token.Address == (common.Address{}) {
			return errors.New("gas oracle: token address must be non-zero")
		}
		if current, ok := decimals[token.Address]; ok && current != token.Decimals {
			return errors.Errorf("gas oracle: token %s has inconsistent decimals %d and %d",
				token.Address.Hex(), current, token.Decimals)
		}
		decimals[token.Address] = token.Decimals
	}
	for _, token := range uniqueTokens(tokens) {
		if token.Decimals < 0 || token.Decimals > maxOracleDecimals {
			return errors.Errorf("gas oracle: token %s decimals %d exceed supported range [0,%d]",
				token.Address.Hex(), token.Decimals, maxOracleDecimals)
		}
		if r.cfg.TokenUSDFeeds[token.Address].Address == (common.Address{}) {
			return errors.Errorf("gas oracle: missing USD feed for token %s", token.Address.Hex())
		}
	}
	return nil
}

func (r *OracleReader) Read(ctx context.Context, tokens []Token, now time.Time) (*PriceSnapshot, error) {
	if err := r.ValidateTokens(tokens); err != nil {
		return nil, err
	}
	tokens = uniqueTokens(tokens)
	calls := make([]chain.Call, 0, 2+2*len(tokens))
	calls = appendFeedCalls(calls, r.cfg.NativeUSDFeed.Address)
	for _, token := range tokens {
		calls = appendFeedCalls(calls, r.cfg.TokenUSDFeeds[token.Address].Address)
	}
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, errors.Errorf("gas oracle: multicall: %w", err)
	}
	if len(results) != len(calls) {
		return nil, errors.Errorf("gas oracle: got %d results, want %d", len(results), len(calls))
	}
	native, err := decodeFeed(
		results[:2], r.cfg.NativeUSDFeed.Address, now, r.cfg.NativeUSDFeed.MaxAge,
	)
	if err != nil {
		return nil, err
	}
	rates := make(map[common.Address]*big.Int, len(tokens))
	for i, token := range tokens {
		feed := r.cfg.TokenUSDFeeds[token.Address]
		price, decodeErr := decodeFeed(results[2+i*2:4+i*2], feed.Address, now, feed.MaxAge)
		if decodeErr != nil {
			return nil, decodeErr
		}
		rate := tokenPerNative(native, price, token.Decimals)
		if rate.Sign() <= 0 {
			return nil, errors.Errorf("gas oracle: token/native rate for %s rounded to zero", token.Address.Hex())
		}
		rates[token.Address] = rate
	}
	return NewPriceSnapshot(rates), nil
}

type feedPrice struct {
	answer   *big.Int
	decimals uint8
}

func appendFeedCalls(calls []chain.Call, feed common.Address) []chain.Call {
	return append(calls,
		chain.Call{Target: feed, AllowFailure: true, Data: chainlinkFeed.PackLatestRoundData()},
		chain.Call{Target: feed, AllowFailure: true, Data: chainlinkFeed.PackDecimals()},
	)
}

func decodeFeed(results []chain.CallResult, feed common.Address, now time.Time, maxAge time.Duration) (feedPrice, error) {
	if len(results) != 2 || !results[0].Success || !results[1].Success {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s call failed", feed.Hex())
	}
	round, err := chainlinkFeed.UnpackLatestRoundData(results[0].ReturnData)
	if err != nil {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s latestRoundData: %w", feed.Hex(), err)
	}
	decimals, err := chainlinkFeed.UnpackDecimals(results[1].ReturnData)
	if err != nil {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s decimals: %w", feed.Hex(), err)
	}
	if round.RoundId == nil || round.Answer == nil || round.UpdatedAt == nil {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s returned nil round data", feed.Hex())
	}
	if round.RoundId.Sign() <= 0 || round.Answer.Sign() <= 0 ||
		round.UpdatedAt.Sign() <= 0 || !round.UpdatedAt.IsInt64() {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s returned invalid round data", feed.Hex())
	}
	const maxFutureSkewSeconds = 15
	age := now.Unix() - round.UpdatedAt.Int64()
	if age < -maxFutureSkewSeconds {
		return feedPrice{}, errors.Errorf(
			"gas oracle: feed %s updated %ds in the future", feed.Hex(), -age,
		)
	}
	// A new Ethereum block can land between the caller's timestamp read and this latest-state
	// multicall. Accept only that small race, not arbitrary future timestamps.
	age = max(age, 0)
	maxAgeSeconds := int64(maxAge / time.Second)
	if maxAge%time.Second != 0 {
		maxAgeSeconds++
	}
	if age > maxAgeSeconds {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s is stale: age %ds, max %ds", feed.Hex(), age, maxAgeSeconds)
	}
	if decimals > maxOracleDecimals {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s decimals %d exceed %d", feed.Hex(), decimals, maxOracleDecimals)
	}
	return feedPrice{answer: new(big.Int).Set(round.Answer), decimals: decimals}, nil
}

func tokenPerNative(native, token feedPrice, tokenDecimals int) *big.Int {
	numerator := new(big.Int).Mul(native.answer, chain.Exp10(int(token.decimals)+tokenDecimals))
	denominator := new(big.Int).Mul(token.answer, chain.Exp10(int(native.decimals)))
	return numerator.Div(numerator, denominator)
}

func uniqueTokens(tokens []Token) []Token {
	byAddress := make(map[common.Address]Token, len(tokens))
	for _, token := range tokens {
		if current, ok := byAddress[token.Address]; !ok || token.Decimals > current.Decimals {
			byAddress[token.Address] = token
		}
	}
	out := make([]Token, 0, len(byAddress))
	for _, token := range byAddress {
		out = append(out, token)
	}
	slices.SortFunc(out, func(a, b Token) int { return a.Address.Cmp(b.Address) })
	return out
}
