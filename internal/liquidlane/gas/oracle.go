package gas

import (
	"context"
	"encoding/json"
	"math/big"
	"slices"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

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
	if s == nil {
		return nil
	}
	return bigmath.Clone(s.tokenOutPerNative[token])
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

type OracleReader struct {
	chain chain.Multicaller
	cfg   OracleConfig
}

func NewOracleReader(c chain.Multicaller, cfg OracleConfig) (*OracleReader, error) {
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
	_, err := r.tokens(tokens)
	return err
}

func (r *OracleReader) tokens(tokens []Token) ([]Token, error) {
	decimals := make(map[common.Address]int, len(tokens))
	for _, token := range tokens {
		if token.Address == (common.Address{}) {
			return nil, errors.New("gas oracle: token address must be non-zero")
		}
		if current, ok := decimals[token.Address]; ok && current != token.Decimals {
			return nil, errors.Errorf("gas oracle: token %s has inconsistent decimals %d and %d", token.Address.Hex(), current, token.Decimals)
		}
		if token.Decimals < 0 || token.Decimals > maxOracleDecimals {
			return nil, errors.Errorf("gas oracle: token %s decimals %d exceed supported range [0,%d]", token.Address.Hex(), token.Decimals, maxOracleDecimals)
		}
		if r.cfg.TokenUSDFeeds[token.Address].Address == (common.Address{}) {
			return nil, errors.Errorf("gas oracle: missing USD feed for token %s", token.Address.Hex())
		}
		decimals[token.Address] = token.Decimals
	}
	out := make([]Token, 0, len(decimals))
	for address, count := range decimals {
		out = append(out, Token{Address: address, Decimals: count})
	}
	slices.SortFunc(out, func(a, b Token) int { return a.Address.Cmp(b.Address) })
	return out, nil
}

func (r *OracleReader) Read(ctx context.Context, requested []Token, now time.Time) (*PriceSnapshot, error) {
	tokens, err := r.tokens(requested)
	if err != nil {
		return nil, err
	}
	// Each address is read once. When several configured assets share a feed,
	// its strictest freshness policy applies to the common observation.
	feeds := []USDFeed{r.cfg.NativeUSDFeed}
	index := map[common.Address]int{r.cfg.NativeUSDFeed.Address: 0}
	for _, token := range tokens {
		feed := r.cfg.TokenUSDFeeds[token.Address]
		if i, exists := index[feed.Address]; exists {
			feeds[i].MaxAge = min(feeds[i].MaxAge, feed.MaxAge)
		} else {
			index[feed.Address] = len(feeds)
			feeds = append(feeds, feed)
		}
	}
	calls := make([]chain.Call, 0, 2*len(feeds))
	for _, feed := range feeds {
		calls = appendFeedCalls(calls, feed.Address)
	}
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, errors.Errorf("gas oracle: multicall: %w", err)
	}
	if len(results) != len(calls) {
		return nil, errors.Errorf("gas oracle: got %d results, want %d", len(results), len(calls))
	}
	prices := make([]feedPrice, len(feeds))
	for i, feed := range feeds {
		prices[i], err = decodeFeed(results[2*i:2*i+2], feed.Address, now, feed.MaxAge)
		if err != nil {
			return nil, err
		}
	}
	rates := make(map[common.Address]*big.Int, len(tokens))
	for _, token := range tokens {
		price := prices[index[r.cfg.TokenUSDFeeds[token.Address].Address]]
		rate := tokenPerNative(prices[0], price, token.Decimals)
		if rate.Sign() <= 0 {
			return nil, errors.Errorf("gas oracle: token/native rate for %s rounded to zero", token.Address.Hex())
		}
		rates[token.Address] = rate
	}
	return &PriceSnapshot{tokenOutPerNative: rates}, nil
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

func decodeFeed(results []chain.CallResult, feed common.Address, now time.Time, maxAge time.Duration) (price feedPrice, err error) {
	defer func() {
		if err != nil {
			err = errors.Errorf("gas oracle: feed %s: %w", feed.Hex(), err)
		}
	}()
	if len(results) != 2 || !results[0].Success || !results[1].Success {
		return feedPrice{}, errors.New("call failed")
	}
	round, err := chainlinkFeed.UnpackLatestRoundData(results[0].ReturnData)
	if err != nil {
		return feedPrice{}, errors.Errorf("latestRoundData: %w", err)
	}
	decimals, err := chainlinkFeed.UnpackDecimals(results[1].ReturnData)
	if err != nil {
		return feedPrice{}, errors.Errorf("decimals: %w", err)
	}
	for _, number := range []*big.Int{round.RoundId, round.Answer, round.UpdatedAt} {
		if number == nil {
			return feedPrice{}, errors.New("returned nil round data")
		}
		if number.Sign() <= 0 {
			return feedPrice{}, errors.New("returned invalid round data")
		}
	}
	if !round.UpdatedAt.IsInt64() {
		return feedPrice{}, errors.New("returned invalid round data")
	}
	age := now.Unix() - round.UpdatedAt.Int64()
	// Permit a block arriving between the caller's timestamp read and multicall,
	// while rejecting arbitrary future answers. MaxAge rounds up to whole seconds.
	if age < -15 {
		return feedPrice{}, errors.Errorf("updated %ds in the future", -age)
	}
	maximum := int64(maxAge / time.Second)
	if maxAge%time.Second != 0 {
		maximum++
	}
	if max(age, 0) > maximum {
		return feedPrice{}, errors.Errorf("is stale: age %ds, max %ds", age, maximum)
	}
	if decimals > maxOracleDecimals {
		return feedPrice{}, errors.Errorf("decimals %d exceed %d", decimals, maxOracleDecimals)
	}
	return feedPrice{answer: new(big.Int).Set(round.Answer), decimals: decimals}, nil
}

func tokenPerNative(native, token feedPrice, tokenDecimals int) *big.Int {
	numerator := new(big.Int).Mul(native.answer, bigmath.Exp10(int(token.decimals)+tokenDecimals))
	denominator := new(big.Int).Mul(token.answer, bigmath.Exp10(int(native.decimals)))
	return numerator.Div(numerator, denominator)
}
