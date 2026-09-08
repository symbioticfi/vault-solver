package gas

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/chainlink/aggregator"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

type oracleMulticaller struct {
	results []chain.CallResult
	now     time.Time
	err     error
}

func (f oracleMulticaller) MulticallWithTime(context.Context, []chain.Call) ([]chain.CallResult, time.Time, error) {
	return f.results, f.now, f.err
}

func TestOracleReaderValidatesMulticallAtStartup(t *testing.T) {
	feed := common.HexToAddress("0x1111")
	token := common.HexToAddress("0x2222")
	unsupported := errors.New("getCurrentBlockTimestamp call failed")
	for _, tc := range []struct {
		name   string
		rpcErr error
	}{
		{name: "supported"},
		{name: "unsupported timestamp selector", rpcErr: unsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, err := NewOracleReader(oracleMulticaller{err: tc.rpcErr}, OracleConfig{
				NativeUSDFeed: USDFeed{Address: feed, MaxAge: time.Minute},
				TokenUSDFeeds: map[common.Address]USDFeed{token: {Address: feed, MaxAge: time.Minute}},
			})
			if err != nil {
				t.Fatal(err)
			}
			err = reader.Validate(t.Context(), []Token{{Address: token, Decimals: 18}})
			if !errors.Is(err, tc.rpcErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tc.rpcErr)
			}
			if tc.rpcErr != nil && !strings.Contains(err.Error(), "validate multicall timestamp") {
				t.Fatalf("missing startup context: %v", err)
			}
		})
	}
}

func TestOracleReaderComposesTokenPerNative(t *testing.T) {
	nativeFeed := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenFeed := common.HexToAddress("0x2222222222222222222222222222222222222222")
	token := common.HexToAddress("0x3333333333333333333333333333333333333333")
	now := time.Unix(1_800_000_000, 0)
	reader, err := NewOracleReader(oracleMulticaller{now: now, results: []chain.CallResult{
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
	snapshot, err := reader.Read(t.Context(), []Token{{Address: token, Decimals: 6}})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !snapshot.BlockTime.Equal(now) {
		t.Fatalf("block time = %s, want %s", snapshot.BlockTime, now)
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
	reader, err := NewOracleReader(oracleMulticaller{now: now, results: []chain.CallResult{
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
	if _, err := reader.Read(t.Context(), []Token{{Address: token, Decimals: 6}}); err == nil ||
		!strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale Read error = %v", err)
	}
}

// Oracle freshness is measured against the timestamp returned with the prices.
func TestOracleReaderUsesBatchTime(t *testing.T) {
	for _, tc := range []struct {
		name          string
		updatedOffset time.Duration
		wantError     string
	}{
		{name: "updated in batch block"},
		{name: "at max age", updatedOffset: -time.Minute},
		{name: "stale", updatedOffset: -time.Minute - time.Second, wantError: "stale"},
		{name: "future by one second", updatedOffset: time.Second, wantError: "updated 1s in the future"},
		{name: "future by one slot", updatedOffset: 12 * time.Second, wantError: "updated 12s in the future"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nativeFeed := common.HexToAddress("0x1111111111111111111111111111111111111111")
			tokenFeed := common.HexToAddress("0x2222222222222222222222222222222222222222")
			token := common.HexToAddress("0x3333333333333333333333333333333333333333")
			batchTime := time.Unix(1_800_000_024, 0)
			reader, err := NewOracleReader(oracleMulticaller{now: batchTime, results: []chain.CallResult{
				oracleRoundResult(t, 2000_00000000, batchTime.Unix()), oracleDecimalsResult(t),
				oracleRoundResult(t, 2_00000000, batchTime.Add(tc.updatedOffset).Unix()), oracleDecimalsResult(t),
			}}, OracleConfig{
				NativeUSDFeed: USDFeed{Address: nativeFeed, MaxAge: time.Minute},
				TokenUSDFeeds: map[common.Address]USDFeed{token: {Address: tokenFeed, MaxAge: time.Minute}},
			})
			if err != nil {
				t.Fatalf("NewOracleReader: %v", err)
			}
			_, err = reader.Read(t.Context(), []Token{{Address: token, Decimals: 6}})
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("Read: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("Read error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

func TestOracleReaderIgnoresDeprecatedAnsweredInRound(t *testing.T) {
	nativeFeed := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenFeed := common.HexToAddress("0x2222222222222222222222222222222222222222")
	token := common.HexToAddress("0x3333333333333333333333333333333333333333")
	now := time.Unix(1_800_000_000, 0)
	reader, err := NewOracleReader(oracleMulticaller{now: now, results: []chain.CallResult{
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
	if _, err := reader.Read(t.Context(), []Token{{Address: token, Decimals: 6}}); err != nil {
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
