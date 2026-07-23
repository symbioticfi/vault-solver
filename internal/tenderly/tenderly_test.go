package tenderly

import (
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestSimulatorURL(t *testing.T) {
	from := common.HexToAddress("0x9e8EB30000000000000000000000000000000001")
	to := common.HexToAddress("0x0370000000000000000000000000000000000002")
	data := []byte{0xde, 0xad, 0xbe, 0xef}

	url := SimulatorURL(big.NewInt(1), from, to, data, big.NewInt(0))

	const prefix = "https://dashboard.tenderly.co/simulator/new?draft="
	if !strings.HasPrefix(url, prefix) {
		t.Fatalf("url = %q, want prefix %q", url, prefix)
	}

	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(url, prefix))
	if err != nil {
		t.Fatalf("draft is not base64url: %v", err)
	}
	var got draft
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("draft is not JSON: %v", err)
	}

	want := draft{
		V:       1,
		Network: network{ID: "1"},
		Row: row{
			ContractAddress:  to.Hex(),
			From:             from.Hex(),
			InputDataType:    "raw",
			RawFunctionInput: "0xdeadbeef",
			Value:            "0",
		},
	}
	if got != want {
		t.Fatalf("draft = %+v, want %+v", got, want)
	}
}

func TestSimulatorURL_NilValueDefaultsToZero(t *testing.T) {
	url := SimulatorURL(big.NewInt(11155111), common.Address{0x01}, common.Address{0x02}, nil, nil)
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(url, "https://dashboard.tenderly.co/simulator/new?draft="))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var got draft
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Row.Value != "0" {
		t.Fatalf("value = %q, want 0 for nil value", got.Row.Value)
	}
	if got.Row.RawFunctionInput != "0x" {
		t.Fatalf("rawFunctionInput = %q, want 0x for nil data", got.Row.RawFunctionInput)
	}
}

func TestSimulatorURL_NilChainIDIsEmpty(t *testing.T) {
	if url := SimulatorURL(nil, common.Address{}, common.Address{}, nil, nil); url != "" {
		t.Fatalf("url = %q, want empty for nil chainID", url)
	}
}
