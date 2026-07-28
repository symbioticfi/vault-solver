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
	if err != nil {
		t.Fatalf("NewOracleReader: %v", err)
	}
	snapshot, err := reader.Read(t.Context(), []Token{{Address: token, Decimals: 6}}, now)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
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
	if err != nil {
		t.Fatalf("NewOracleReader: %v", err)
	}
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
	if err != nil {
		t.Fatalf("NewOracleReader: %v", err)
	}
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
	if err != nil {
		t.Fatalf("NewOracleReader: %v", err)
	}
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
	if err != nil {
		t.Fatalf("NewOracleReader: %v", err)
	}
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
	if err != nil {
		t.Fatalf("pack latestRoundData: %v", err)
	}
	return chain.CallResult{Success: true, ReturnData: data}
}

func oracleDecimalsResult(t *testing.T) chain.CallResult {
	t.Helper()
	parsed := oracleABI(t)
	data, err := parsed.Methods["decimals"].Outputs.Pack(uint8(8))
	if err != nil {
		t.Fatalf("pack decimals: %v", err)
	}
	return chain.CallResult{Success: true, ReturnData: data}
}

func oracleABI(t *testing.T) abi.ABI {
	t.Helper()
	parsed, err := abi.JSON(strings.NewReader(aggregator.AggregatorV3MetaData.ABI))
	if err != nil {
		t.Fatalf("parse AggregatorV3 ABI: %v", err)
	}
	return parsed
}
