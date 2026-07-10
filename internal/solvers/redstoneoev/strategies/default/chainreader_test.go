package defaultstrategy

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/api/bindings/oev/aggregator"
)

func mustParseABI(j string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(j))
	if err != nil {
		panic("redstoneoev/defaultstrategy: parse abi: " + err.Error())
	}
	return parsed
}

var (
	feedTestABI = mustParseABI(aggregator.AggregatorV3MetaData.ABI)
)

func packOut(t *testing.T, a abi.ABI, method string, vals ...any) []byte {
	t.Helper()
	out, err := a.Methods[method].Outputs.Pack(vals...)
	if err != nil {
		t.Fatalf("pack %s outputs: %v", method, err)
	}
	return out
}

func TestFeedDecimalsInBounds(t *testing.T) {
	cases := []struct {
		name            string
		loanDec, ethDec uint8
		want            bool
	}{
		{"chainlink usd pairs", 8, 8, true},
		{"exactly at bound", maxFeedDecimals, maxFeedDecimals, true},
		{"loan over bound", maxFeedDecimals + 1, 8, false},
		{"eth over bound", 8, maxFeedDecimals + 1, false},
		{"uint8 max", 255, 255, false},
	}
	for _, c := range cases {
		if got := feedDecimalsInBounds(c.loanDec, c.ethDec); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFeedFresh(t *testing.T) {
	const now, maxAge = 1_000_000, 3600
	cases := []struct {
		name      string
		updatedAt int64
		want      bool
	}{
		{"current", now, true},
		{"recent", now - 1800, true},
		{"exactly max age", now - maxAge, true},
		{"just stale", now - maxAge - 1, false},
		{"future", now + 1, false},
	}
	for _, c := range cases {
		if got := feedFresh(c.updatedAt, now, maxAge); got != c.want {
			t.Errorf("%s: feedFresh = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAggregatorFeedDecoders(t *testing.T) {
	latest := packOut(t, feedTestABI, "latestRoundData",
		big.NewInt(10), mustBig("250000000000"), big.NewInt(900), big.NewInt(1_000), big.NewInt(10))
	answer, updatedAt, err := decodeLatestRoundData(latest)
	if err != nil {
		t.Fatal(err)
	}
	if answer.String() != "250000000000" || updatedAt.Int64() != 1_000 {
		t.Fatalf("latestRoundData decoded answer=%s updatedAt=%s", answer, updatedAt)
	}

	dec, err := decodeFeedDecimals(packOut(t, feedTestABI, "decimals", uint8(8)))
	if err != nil {
		t.Fatal(err)
	}
	if dec != 8 {
		t.Fatalf("decimals=%d, want 8", dec)
	}

	if _, _, err := decodeLatestRoundData([]byte{0x01, 0x02}); err == nil {
		t.Fatal("garbled latestRoundData must fail")
	}
	if _, err := decodeFeedDecimals([]byte{0x01, 0x02}); err == nil {
		t.Fatal("garbled decimals must fail")
	}
}

func TestVerifyAdapterPair(t *testing.T) {
	loan := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	other := common.HexToAddress("0x1111111111111111111111111111111111111111")
	collA := common.HexToAddress("0x45804880De22913dAFE09f4980848ECE6EcbAf78")
	collB := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	collX := common.HexToAddress("0x2222222222222222222222222222222222222222")

	good := common.HexToHash("0xaa")
	wrongL := common.HexToHash("0xbb")
	wrongC := common.HexToHash("0xcc")
	params := map[common.Hash]MarketParams{
		good:   {LoanToken: loan, CollateralToken: collA},
		wrongL: {LoanToken: other, CollateralToken: collB},
		wrongC: {LoanToken: loan, CollateralToken: collX},
	}
	kept := verifyAdapterPair(params, loan, []common.Address{collA, collB})
	if len(kept) != 1 || kept[0] != good {
		t.Fatalf("want exactly the matching pair, got %+v", kept)
	}
}
