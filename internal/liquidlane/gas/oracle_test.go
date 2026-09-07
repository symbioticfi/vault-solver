package gas

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/api/bindings/chainlink/aggregator"
	"github.com/symbioticfi/vault-solver/internal/chain"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

type oracleMulticaller struct {
	results []chain.CallResult
}

func (f oracleMulticaller) Multicall(context.Context, []chain.Call) ([]chain.CallResult, error) {
	return f.results, nil
}

func TestOracleReaderComposesTokenPerNative(t *testing.T) {
	nativeFeed := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenFeed := common.HexToAddress("0x2222222222222222222222222222222222222222")
	token := common.HexToAddress("0x3333333333333333333333333333333333333333")
	now := time.Unix(1_800_000_000, 0)
	reader, err := NewOracleReader(oracleMulticaller{results: []chain.CallResult{
		oracleRoundResult(t, 2000_00000000, now.Unix()), oracleDecimalsResult(t),
		oracleRoundResult(t, 2_00000000, now.Unix()), oracleDecimalsResult(t),
	}}, OracleConfig{
		NativeUSDFeed: USDFeed{Address: nativeFeed, MaxAge: time.Minute},
		TokenUSDFeeds: map[common.Address]USDFeed{
			token: {Address: tokenFeed, MaxAge: time.Minute},
		},
	})
	testcheck.NoError(t, err, "NewOracleReader: %v")
	snapshot, err := reader.Read(t.Context(), []Token{{Address: token, Decimals: 6}}, now)
	testcheck.NoError(t, err, "Read: %v")
	if got := snapshot.TokenOutPerNative(token); got == nil || got.String() != "1000000000" {
		t.Fatalf("token per native = %v, want 1000000000", got)
	}
}

func TestOracleReaderRejectsMissingAndStaleFeeds(t *testing.T) {
	nativeFeed := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenFeed := common.HexToAddress("0x2222222222222222222222222222222222222222")
	token := common.HexToAddress("0x3333333333333333333333333333333333333333")
	now := time.Unix(1_800_000_000, 0)
	reader, err := NewOracleReader(oracleMulticaller{results: []chain.CallResult{
		oracleRoundResult(t, 2000_00000000, now.Add(-2*time.Minute).Unix()), oracleDecimalsResult(t),
		oracleRoundResult(t, 2_00000000, now.Unix()), oracleDecimalsResult(t),
	}}, OracleConfig{
		NativeUSDFeed: USDFeed{Address: nativeFeed, MaxAge: time.Minute},
		TokenUSDFeeds: map[common.Address]USDFeed{
			token: {Address: tokenFeed, MaxAge: time.Minute},
		},
	})
	testcheck.NoError(t, err, "NewOracleReader: %v")
	if err := reader.ValidateTokens([]Token{{Address: common.HexToAddress("0x4444444444444444444444444444444444444444"), Decimals: 6}}); err == nil {
		t.Fatal("expected missing token feed error")
	}
	if _, err := reader.Read(t.Context(), []Token{{Address: token, Decimals: 6}}, now); err == nil ||
		!strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale Read error = %v", err)
	}
}

func TestOracleReaderAcceptsFeedUpdatedInNewerBlock(t *testing.T) {
	nativeFeed := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenFeed := common.HexToAddress("0x2222222222222222222222222222222222222222")
	token := common.HexToAddress("0x3333333333333333333333333333333333333333")
	now := time.Unix(1_800_000_000, 0)
	reader, err := NewOracleReader(oracleMulticaller{results: []chain.CallResult{
		oracleRoundResult(t, 2000_00000000, now.Add(12*time.Second).Unix()), oracleDecimalsResult(t),
		oracleRoundResult(t, 2_00000000, now.Add(12*time.Second).Unix()), oracleDecimalsResult(t),
	}}, OracleConfig{
		NativeUSDFeed: USDFeed{Address: nativeFeed, MaxAge: time.Minute},
		TokenUSDFeeds: map[common.Address]USDFeed{
			token: {Address: tokenFeed, MaxAge: time.Minute},
		},
	})
	testcheck.NoError(t, err, "NewOracleReader: %v")
	if _, err := reader.Read(t.Context(), []Token{{Address: token, Decimals: 6}}, now); err != nil {
		t.Fatalf("Read: %v", err)
	}
}

func TestOracleReaderRejectsFeedFarInTheFuture(t *testing.T) {
	nativeFeed := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenFeed := common.HexToAddress("0x2222222222222222222222222222222222222222")
	token := common.HexToAddress("0x3333333333333333333333333333333333333333")
	now := time.Unix(1_800_000_000, 0)
	reader, err := NewOracleReader(oracleMulticaller{results: []chain.CallResult{
		oracleRoundResult(t, 2000_00000000, now.Add(time.Minute).Unix()), oracleDecimalsResult(t),
		oracleRoundResult(t, 2_00000000, now.Unix()), oracleDecimalsResult(t),
	}}, OracleConfig{
		NativeUSDFeed: USDFeed{Address: nativeFeed, MaxAge: time.Minute},
		TokenUSDFeeds: map[common.Address]USDFeed{
			token: {Address: tokenFeed, MaxAge: time.Minute},
		},
	})
	testcheck.NoError(t, err, "NewOracleReader: %v")
	if _, err := reader.Read(t.Context(), []Token{{Address: token, Decimals: 6}}, now); err == nil ||
		!strings.Contains(err.Error(), "in the future") {
		t.Fatalf("future Read error = %v", err)
	}
}

func TestOracleReaderIgnoresDeprecatedAnsweredInRound(t *testing.T) {
	nativeFeed := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenFeed := common.HexToAddress("0x2222222222222222222222222222222222222222")
	token := common.HexToAddress("0x3333333333333333333333333333333333333333")
	now := time.Unix(1_800_000_000, 0)
	reader, err := NewOracleReader(oracleMulticaller{results: []chain.CallResult{
		oracleRoundResultWithAnsweredInRound(t, 2000_00000000, now.Unix(), 0), oracleDecimalsResult(t),
		oracleRoundResultWithAnsweredInRound(t, 2_00000000, now.Unix(), 0), oracleDecimalsResult(t),
	}}, OracleConfig{
		NativeUSDFeed: USDFeed{Address: nativeFeed, MaxAge: time.Minute},
		TokenUSDFeeds: map[common.Address]USDFeed{
			token: {Address: tokenFeed, MaxAge: time.Minute},
		},
	})
	testcheck.NoError(t, err, "NewOracleReader: %v")
	if _, err := reader.Read(t.Context(), []Token{{Address: token, Decimals: 6}}, now); err != nil {
		t.Fatalf("Read: %v", err)
	}
}

func oracleRoundResult(t *testing.T, answer, updatedAt int64) chain.CallResult {
	t.Helper()
	return oracleRoundResultWithAnsweredInRound(t, answer, updatedAt, 10)
}

func oracleRoundResultWithAnsweredInRound(
	t *testing.T,
	answer, updatedAt, answeredInRound int64,
) chain.CallResult {
	t.Helper()
	parsed := oracleABI(t)
	data, err := parsed.Methods["latestRoundData"].Outputs.Pack(
		big.NewInt(10),
		big.NewInt(answer),
		big.NewInt(updatedAt-1),
		big.NewInt(updatedAt),
		big.NewInt(answeredInRound),
	)
	testcheck.NoError(t, err, "pack latestRoundData: %v")
	return chain.CallResult{Success: true, ReturnData: data}
}

func oracleDecimalsResult(t *testing.T) chain.CallResult {
	t.Helper()
	parsed := oracleABI(t)
	data, err := parsed.Methods["decimals"].Outputs.Pack(uint8(8))
	testcheck.NoError(t, err, "pack decimals: %v")
	return chain.CallResult{Success: true, ReturnData: data}
}

func oracleABI(t *testing.T) abi.ABI {
	t.Helper()
	parsed, err := abi.JSON(strings.NewReader(aggregator.AggregatorV3MetaData.ABI))
	testcheck.NoError(t, err, "parse AggregatorV3 ABI: %v")
	return parsed
}

func TestOracleReaderSharedFeedUsesStrictestFreshness(t *testing.T) {
	feed, token := common.Address{19: 1}, common.Address{19: 2}
	now := time.Unix(1_800_000_000, 0)
	for _, age := range []time.Duration{30 * time.Second, 2 * time.Minute} {
		t.Run(age.String(), func(t *testing.T) {
			reader, err := NewOracleReader(oracleMulticaller{results: []chain.CallResult{
				oracleRoundResult(t, 2000_00000000, now.Add(-age).Unix()), oracleDecimalsResult(t),
			}}, OracleConfig{
				NativeUSDFeed: USDFeed{Address: feed, MaxAge: 5 * time.Minute},
				TokenUSDFeeds: map[common.Address]USDFeed{token: {Address: feed, MaxAge: time.Minute}},
			})
			testcheck.NoError(t, err)
			snapshot, err := reader.Read(t.Context(), []Token{{Address: token, Decimals: 18}}, now)
			if age > time.Minute {
				if err == nil || !strings.Contains(err.Error(), "stale") {
					t.Fatalf("error = %v", err)
				}
				return
			}
			testcheck.NoError(t, err)
			if snapshot.TokenOutPerNative(token).Cmp(big.NewInt(1e18)) != 0 {
				t.Fatal("same feed must yield one token per native")
			}
		})
	}
}
