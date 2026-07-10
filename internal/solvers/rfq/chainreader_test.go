package rfq

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	llbinding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

type fakeMulticallClient struct {
	responses [][]chain.CallResult
	calls     [][]chain.Call
}

func (f *fakeMulticallClient) Multicall(
	_ context.Context,
	calls []chain.Call,
) ([]chain.CallResult, error) {
	f.calls = append(f.calls, append([]chain.Call(nil), calls...))
	if len(f.responses) == 0 {
		return nil, nil
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

type fakeDecimalsReader struct {
	decimals int
	err      error
}

func (f fakeDecimalsReader) Get(context.Context, common.Address) (int, error) {
	return f.decimals, f.err
}

func adapterResult(t *testing.T, method string, values ...any) chain.CallResult {
	t.Helper()
	parsed, err := llbinding.LiquidLaneAdapterMetaData.ParseABI()
	if err != nil {
		t.Fatalf("parse LiquidLaneAdapter ABI: %v", err)
	}
	m, ok := parsed.Methods[method]
	if !ok {
		t.Fatalf("LiquidLaneAdapter ABI has no method %q", method)
	}
	data, err := m.Outputs.Pack(values...)
	if err != nil {
		t.Fatalf("pack %s output: %v", method, err)
	}
	return chain.CallResult{Success: true, ReturnData: data}
}

func inventoryResults(t *testing.T, paused chain.CallResult) []chain.CallResult {
	t.Helper()
	return []chain.CallResult{
		paused,
		adapterResult(t, "getMaxAssets", big.NewInt(1_000_000)),
		adapterResult(t, "getMaxRate", big.NewInt(1_000_000_000_000_000_000)),
	}
}

func TestReadVaultInventories_ABIBoundary(t *testing.T) {
	t.Parallel()
	adapterAddr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	asset := common.HexToAddress("0x0000000000000000000000000000000000000022")
	tokenIn := common.HexToAddress("0x0000000000000000000000000000000000000033")
	mc := &fakeMulticallClient{responses: [][]chain.CallResult{
		inventoryResults(t, adapterResult(t, "paused", false)),
	}}
	r := &reader{chain: mc, dec: fakeDecimalsReader{decimals: 6}, log: logr.Discard()}

	got, err := r.readVaultInventories(t.Context(), tokenIn, []recoveryVault{{
		Adapter: adapterAddr,
		Asset:   asset,
	}})
	if err != nil {
		t.Fatalf("readVaultInventories: %v", err)
	}
	if len(got) != 1 || got[0].Adapter != adapterAddr || got[0].Asset != asset ||
		got[0].AssetDecimals != 6 || got[0].MaxAssets.Cmp(big.NewInt(1_000_000)) != 0 {
		t.Fatalf("inventory = %+v", got)
	}
	if len(mc.calls) != 1 || len(mc.calls[0]) != readsPerAdapter {
		t.Fatalf("multicall layout = %+v", mc.calls)
	}
	wantData := [][]byte{
		llAdapter.PackPaused(),
		llAdapter.PackGetMaxAssets(tokenIn),
		llAdapter.PackGetMaxRate(tokenIn),
	}
	for i, call := range mc.calls[0] {
		if call.Target != adapterAddr || !call.AllowFailure || string(call.Data) != string(wantData[i]) {
			t.Fatalf("call %d = %+v, want target %s and selector %x", i, call, adapterAddr, wantData[i])
		}
	}
}

func TestReadVaultInventories_PauseReadFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		paused chain.CallResult
		want   int
	}{
		{name: "unpaused", paused: adapterResult(t, "paused", false), want: 1},
		{name: "paused", paused: adapterResult(t, "paused", true)},
		{name: "pause read reverted", paused: chain.CallResult{Success: false}},
		{name: "pause read malformed", paused: chain.CallResult{Success: true, ReturnData: []byte{0x01}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mc := &fakeMulticallClient{responses: [][]chain.CallResult{inventoryResults(t, tc.paused)}}
			r := &reader{chain: mc, dec: fakeDecimalsReader{decimals: 6}, log: logr.Discard()}
			got, err := r.readVaultInventories(
				t.Context(), tIn, []recoveryVault{{Adapter: vlt, Asset: tOut}},
			)
			if err != nil {
				t.Fatalf("readVaultInventories: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("inventories = %d, want %d", len(got), tc.want)
			}
		})
	}
}

func TestReadVaultInventories_MaxAssetsAndRateFailClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(*testing.T, []chain.CallResult)
		decimalsErr error
	}{
		{
			name:   "max assets reverted",
			mutate: func(_ *testing.T, r []chain.CallResult) { r[1] = chain.CallResult{Success: false} },
		},
		{
			name:   "rate reverted",
			mutate: func(_ *testing.T, r []chain.CallResult) { r[2] = chain.CallResult{Success: false} },
		},
		{
			name:   "max assets malformed",
			mutate: func(_ *testing.T, r []chain.CallResult) { r[1].ReturnData = []byte{0x01} },
		},
		{
			name:   "rate malformed",
			mutate: func(_ *testing.T, r []chain.CallResult) { r[2].ReturnData = []byte{0x01} },
		},
		{
			name: "zero max assets",
			mutate: func(t *testing.T, r []chain.CallResult) {
				t.Helper()
				r[1] = adapterResult(t, "getMaxAssets", new(big.Int))
			},
		},
		{
			name: "zero rate",
			mutate: func(t *testing.T, r []chain.CallResult) {
				t.Helper()
				r[2] = adapterResult(t, "getMaxRate", new(big.Int))
			},
		},
		{name: "decimals unavailable", decimalsErr: errors.New("decimals unavailable")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			results := inventoryResults(t, adapterResult(t, "paused", false))
			if tc.mutate != nil {
				tc.mutate(t, results)
			}
			mc := &fakeMulticallClient{responses: [][]chain.CallResult{results}}
			r := &reader{
				chain: mc,
				dec:   fakeDecimalsReader{decimals: 6, err: tc.decimalsErr},
				log:   logr.Discard(),
			}
			got, err := r.readVaultInventories(
				t.Context(), tIn, []recoveryVault{{Adapter: vlt, Asset: tOut}},
			)
			if err != nil {
				t.Fatalf("readVaultInventories: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("inventories = %+v, want none", got)
			}
		})
	}
}

func TestReadVaultInventories_RejectsWrongResultCount(t *testing.T) {
	t.Parallel()
	mc := &fakeMulticallClient{responses: [][]chain.CallResult{{
		adapterResult(t, "paused", false),
	}}}
	r := &reader{chain: mc, dec: fakeDecimalsReader{decimals: 6}, log: logr.Discard()}
	_, err := r.readVaultInventories(
		t.Context(), tIn, []recoveryVault{{Adapter: vlt, Asset: tOut}},
	)
	if err == nil || !strings.Contains(err.Error(), "got 1 results, want 3") {
		t.Fatalf("error = %v, want result-count mismatch", err)
	}
}

func TestReadPermissionedVaultInventories_AuthorizationBoundary(t *testing.T) {
	t.Parallel()
	executorAddr := common.HexToAddress("0x0000000000000000000000000000000000000044")
	marketMaker := common.HexToAddress("0x0000000000000000000000000000000000000055")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000066")

	tests := []struct {
		name        string
		marketMaker common.Address
		owner       common.Address
		delegated   *bool
		want        int
	}{
		{name: "market maker is executor", marketMaker: executorAddr, owner: owner, want: 1},
		{name: "owner is executor", marketMaker: marketMaker, owner: executorAddr, want: 1},
		{name: "delegated filler", marketMaker: marketMaker, owner: owner, delegated: boolPtr(true), want: 1},
		{name: "not delegated", marketMaker: marketMaker, owner: owner, delegated: boolPtr(false)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			responses := [][]chain.CallResult{
				inventoryResults(t, adapterResult(t, "paused", false)),
				{
					adapterResult(t, "marketMaker", tc.marketMaker),
					adapterResult(t, "owner", tc.owner),
				},
			}
			if tc.delegated != nil {
				responses = append(responses, []chain.CallResult{adapterResult(t, "isFiller", *tc.delegated)})
			}
			mc := &fakeMulticallClient{responses: responses}
			r := &reader{chain: mc, dec: fakeDecimalsReader{decimals: 6}, log: logr.Discard()}
			got, err := r.readPermissionedVaultInventories(
				t.Context(), executorAddr, tIn, []recoveryVault{{Adapter: vlt, Asset: tOut}},
			)
			if err != nil {
				t.Fatalf("readPermissionedVaultInventories: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("inventories = %d, want %d", len(got), tc.want)
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }

func TestReadPermissionedVaultInventories_AuthorizationReadFailsClosed(t *testing.T) {
	t.Parallel()
	executorAddr := common.HexToAddress("0x0000000000000000000000000000000000000044")
	marketMaker := common.HexToAddress("0x0000000000000000000000000000000000000055")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000066")

	tests := []struct {
		name       string
		auth       []chain.CallResult
		delegation []chain.CallResult
	}{
		{
			name: "market maker reverted",
			auth: []chain.CallResult{
				{Success: false},
				adapterResult(t, "owner", owner),
			},
		},
		{
			name: "owner malformed",
			auth: []chain.CallResult{
				adapterResult(t, "marketMaker", marketMaker),
				{Success: true, ReturnData: []byte{0x01}},
			},
		},
		{
			name: "delegation reverted",
			auth: []chain.CallResult{
				adapterResult(t, "marketMaker", marketMaker),
				adapterResult(t, "owner", owner),
			},
			delegation: []chain.CallResult{{Success: false}},
		},
		{
			name: "delegation malformed",
			auth: []chain.CallResult{
				adapterResult(t, "marketMaker", marketMaker),
				adapterResult(t, "owner", owner),
			},
			delegation: []chain.CallResult{{Success: true, ReturnData: []byte{0x01}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			responses := [][]chain.CallResult{
				inventoryResults(t, adapterResult(t, "paused", false)),
				tc.auth,
			}
			if tc.delegation != nil {
				responses = append(responses, tc.delegation)
			}
			mc := &fakeMulticallClient{responses: responses}
			r := &reader{chain: mc, dec: fakeDecimalsReader{decimals: 6}, log: logr.Discard()}
			got, err := r.readPermissionedVaultInventories(
				t.Context(), executorAddr, tIn, []recoveryVault{{Adapter: vlt, Asset: tOut}},
			)
			if err != nil {
				t.Fatalf("readPermissionedVaultInventories: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("inventories = %+v, want none when authorization is unknown", got)
			}
		})
	}
}
