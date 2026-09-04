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

const (
	maxOracleDecimals = 36
	maxFutureSkew     = 15 * time.Second
)

var feedBinding = aggregator.NewAggregatorV3()

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
	cloned := make(map[common.Address]*big.Int, len(rates))
	for token, rate := range rates {
		if rate != nil {
			cloned[token] = new(big.Int).Set(rate)
		}
	}
	return &PriceSnapshot{tokenOutPerNative: cloned}
}

func (snapshot *PriceSnapshot) TokenOutPerNative(token common.Address) *big.Int {
	if snapshot == nil {
		return nil
	}
	return copyBig(snapshot.tokenOutPerNative[token])
}

func (snapshot *PriceSnapshot) MarshalJSON() ([]byte, error) {
	var rates map[common.Address]*big.Int
	if snapshot != nil {
		rates = snapshot.tokenOutPerNative
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

func NewOracleReader(client multicaller, config OracleConfig) (*OracleReader, error) {
	if client == nil {
		return nil, errors.New("gas oracle: chain client is required")
	}
	if config.NativeUSDFeed.Address == (common.Address{}) {
		return nil, errors.New("gas oracle: native USD feed is required")
	}
	if config.NativeUSDFeed.MaxAge <= 0 {
		return nil, errors.New("gas oracle: native USD feed max age must be positive")
	}
	if len(config.TokenUSDFeeds) == 0 {
		return nil, errors.New("gas oracle: at least one token USD feed is required")
	}
	feeds := make(map[common.Address]USDFeed, len(config.TokenUSDFeeds))
	for token, feed := range config.TokenUSDFeeds {
		if token == (common.Address{}) || feed.Address == (common.Address{}) {
			return nil, errors.New("gas oracle: token and feed addresses must be non-zero")
		}
		if feed.MaxAge <= 0 {
			return nil, errors.Errorf("gas oracle: token %s feed max age must be positive", token.Hex())
		}
		feeds[token] = feed
	}
	config.TokenUSDFeeds = feeds
	return &OracleReader{chain: client, cfg: config}, nil
}

func (reader *OracleReader) ValidateTokens(tokens []Token) error {
	seen := make(map[common.Address]int, len(tokens))
	for _, token := range tokens {
		if token.Address == (common.Address{}) {
			return errors.New("gas oracle: token address must be non-zero")
		}
		if decimals, exists := seen[token.Address]; exists && decimals != token.Decimals {
			return errors.Errorf(
				"gas oracle: token %s has inconsistent decimals %d and %d",
				token.Address.Hex(), decimals, token.Decimals,
			)
		}
		seen[token.Address] = token.Decimals
	}
	for _, token := range normalizedTokens(tokens) {
		if token.Decimals < 0 || token.Decimals > maxOracleDecimals {
			return errors.Errorf(
				"gas oracle: token %s decimals %d exceed supported range [0,%d]",
				token.Address.Hex(), token.Decimals, maxOracleDecimals,
			)
		}
		if reader.cfg.TokenUSDFeeds[token.Address].Address == (common.Address{}) {
			return errors.Errorf("gas oracle: missing USD feed for token %s", token.Address.Hex())
		}
	}
	return nil
}

func (reader *OracleReader) Read(ctx context.Context, tokens []Token, now time.Time) (*PriceSnapshot, error) {
	if err := reader.ValidateTokens(tokens); err != nil {
		return nil, err
	}
	tokens = normalizedTokens(tokens)
	feeds := make([]USDFeed, 0, len(tokens)+1)
	feeds = append(feeds, reader.cfg.NativeUSDFeed)
	for _, token := range tokens {
		feeds = append(feeds, reader.cfg.TokenUSDFeeds[token.Address])
	}
	calls := make([]chain.Call, 0, len(feeds)*2)
	for _, feed := range feeds {
		calls = append(calls,
			chain.Call{Target: feed.Address, AllowFailure: true, Data: feedBinding.PackLatestRoundData()},
			chain.Call{Target: feed.Address, AllowFailure: true, Data: feedBinding.PackDecimals()},
		)
	}
	results, err := reader.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, errors.Errorf("gas oracle: multicall: %w", err)
	}
	if len(results) != len(calls) {
		return nil, errors.Errorf("gas oracle: got %d results, want %d", len(results), len(calls))
	}

	prices := make([]feedPrice, len(feeds))
	for index, feed := range feeds {
		price, decodeErr := readFeed(results[index*2:(index+1)*2], feed, now)
		if decodeErr != nil {
			return nil, decodeErr
		}
		prices[index] = price
	}
	rates := make(map[common.Address]*big.Int, len(tokens))
	for index, token := range tokens {
		rate := quoteNative(prices[0], prices[index+1], token.Decimals)
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

func readFeed(results []chain.CallResult, feed USDFeed, now time.Time) (feedPrice, error) {
	if len(results) != 2 || !results[0].Success || !results[1].Success {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s call failed", feed.Address.Hex())
	}
	round, err := feedBinding.UnpackLatestRoundData(results[0].ReturnData)
	if err != nil {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s latestRoundData: %w", feed.Address.Hex(), err)
	}
	decimals, err := feedBinding.UnpackDecimals(results[1].ReturnData)
	if err != nil {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s decimals: %w", feed.Address.Hex(), err)
	}
	if round.RoundId == nil || round.Answer == nil || round.UpdatedAt == nil ||
		round.RoundId.Sign() <= 0 || round.Answer.Sign() <= 0 || round.UpdatedAt.Sign() <= 0 || !round.UpdatedAt.IsInt64() {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s returned invalid round data", feed.Address.Hex())
	}
	updated := time.Unix(round.UpdatedAt.Int64(), 0)
	if updated.After(now.Add(maxFutureSkew)) {
		return feedPrice{}, errors.Errorf(
			"gas oracle: feed %s updated %ds in the future", feed.Address.Hex(), int64(updated.Sub(now)/time.Second),
		)
	}
	age := max(now.Sub(updated), time.Duration(0))
	if age > feed.MaxAge {
		return feedPrice{}, errors.Errorf(
			"gas oracle: feed %s is stale: age %ds, max %ds",
			feed.Address.Hex(), int64(age/time.Second), int64((feed.MaxAge+time.Second-1)/time.Second),
		)
	}
	if decimals > maxOracleDecimals {
		return feedPrice{}, errors.Errorf("gas oracle: feed %s decimals %d exceed %d", feed.Address.Hex(), decimals, maxOracleDecimals)
	}
	return feedPrice{answer: copyBig(round.Answer), decimals: decimals}, nil
}

func quoteNative(native, token feedPrice, tokenDecimals int) *big.Int {
	numerator := new(big.Int).Mul(native.answer, chain.Exp10(int(token.decimals)+tokenDecimals))
	denominator := new(big.Int).Mul(token.answer, chain.Exp10(int(native.decimals)))
	return numerator.Quo(numerator, denominator)
}

func normalizedTokens(tokens []Token) []Token {
	unique := make(map[common.Address]Token, len(tokens))
	for _, token := range tokens {
		current, exists := unique[token.Address]
		if !exists || token.Decimals > current.Decimals {
			unique[token.Address] = token
		}
	}
	result := make([]Token, 0, len(unique))
	for _, token := range unique {
		result = append(result, token)
	}
	slices.SortFunc(result, func(left, right Token) int { return left.Address.Cmp(right.Address) })
	return result
}
