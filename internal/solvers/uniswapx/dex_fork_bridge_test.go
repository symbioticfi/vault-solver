package uniswapx

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

// forkBridgeSpec contains facts read by Forge from real fork contracts. No private backend,
// funding, role, transport, or preflight success is inferred from this test-only IPC adapter.
type forkBridgeSpec struct {
	Mode              string `json:"Mode"`
	RequestAmount     string `json:"RequestAmount"`
	Output            string `json:"Output"`
	OraclePrice       string `json:"OraclePrice"`
	MinDiscount       string `json:"MinDiscount"`
	MaxAssets         string `json:"MaxAssets"`
	GasRate           string `json:"GasRate"`
	ExactInputBridge  string `json:"ExactInputBridge"`
	ExactInputOutput  string `json:"ExactInputOutput"`
	ExactOutputAmount string `json:"ExactOutputAmount"`
	ExactOutputCost   string `json:"ExactOutputCost"`
	PermitDomain      string `json:"PermitDomain"`
	Caller            string `json:"Caller"`
	Recipient         string `json:"Recipient"`
	Treasury          string `json:"Treasury"`
	BlockTime         uint64 `json:"BlockTime"`
}

type capturedDexMarket struct{ spec forkBridgeSpec }

func (*capturedDexMarket) validate(context.Context, common.Address) error { return nil }
func (m *capturedDexMarket) quote(_ context.Context, _ common.Address, amount *big.Int, exactOutput bool) (dexQuote, error) {
	in, out := m.spec.ExactInputBridge, m.spec.ExactInputOutput
	if exactOutput {
		in, out = m.spec.ExactOutputAmount, m.spec.ExactOutputCost
	}
	want, ok := new(big.Int).SetString(in, 10)
	if !ok || amount.Cmp(want) != 0 {
		return dexQuote{}, errors.New("Go request differs from captured actual Quoter request")
	}
	value, ok := new(big.Int).SetString(out, 10)
	if !ok {
		return dexQuote{}, errors.New("invalid captured quote amount")
	}
	return dexQuote{Amount: value,
		BlockTime: time.Unix(int64(m.spec.BlockTime), 0)}, nil
}

func TestDexForkBridge(t *testing.T) {
	path := os.Getenv("ATOMIC_DEX_FFI_INPUT")
	if path == "" {
		t.Skip("opt-in IPC for the local Forge fork E2E")
	}
	data, err := os.ReadFile(path) // #nosec G703 -- opt-in local test IPC, path is supplied by the Forge fixture.
	if err != nil {
		t.Fatal(err)
	}
	var spec forkBridgeSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	integer := func(raw string) *big.Int {
		x, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			t.Fatalf("invalid fork integer %q", raw)
		}
		return x
	}
	pricing, _, _ := dexPricingFixture(t, 6)
	f := newDirectExecutionFixture(t)
	s := f.solver
	s.chainID = 1
	prime := common.HexToAddress("0x19ebb35279A16207Ec4ba82799CC64715065F7F6")
	bridge := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	output := common.HexToAddress(spec.Output)
	output0 := common.HexToAddress("0xdAC17F958D2ee523a2206206994597C13D831ec7")
	output1 := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	s.cfg.Executor = common.HexToAddress("0x8A9BE52e4bf93ee36FEc8FD41F67B4dc87571ed6")
	s.cfg.Reactor = common.HexToAddress("0x00000011F84B9aa48e5f8aA8B9897600006289Be")
	s.cfg.Gas, s.cfg.Strategy, s.cfg.Dex = &liquidlanegas.OracleConfig{}, pricing.cfg.Strategy, pricing.cfg.Dex
	s.cfg.Dex.InputToken, s.cfg.Dex.BridgeToken = prime, bridge
	s.cfg.Dex.Outputs = [2]DexOutput{{Token: output0, Fee: 100}, {Token: output1, Fee: 500}}
	s.cfg.Dex.GasUnitsBudget, s.cfg.Dex.MinProfitBridge = 5_000_000, big.NewInt(1_000_000)
	s.cfg.Dex.MaxBridgeAmount = integer(spec.MaxAssets)
	s.strategy, s.dex = pricing.strategy, &capturedDexMarket{spec: spec}
	s.solverAddress = common.HexToAddress(spec.Caller)
	s.txm = &dexExecutionTxManager{f.txm}
	call := &dexExecutionCaller{gas: s.cfg.Dex.GasUnitsBudget}
	s.chain = call // Real execution and gas ceiling are asserted by Forge immediately after this IPC.
	now := time.Unix(int64(spec.BlockTime), 0)
	route := liquidlane.NewRoute(1, common.HexToAddress("0x59CDDE0D345c0eE6ED453FE7d3fb365Fe0721E85"),
		common.HexToAddress("0x123"), prime, bridge, 6, 6)
	price, discount := integer(spec.OraclePrice), integer(spec.MinDiscount)
	inventory := liquidlane.DirectInventory(route, integer(spec.MaxAssets), liquidlane.DiscountedRate(price, discount))
	inventory.Price, inventory.Discount, inventory.AdapterMinDiscount = price, discount, discount
	prices := liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{bridge: integer(spec.GasRate)})
	s.quoteState.Store(&quoteState{inventory: []liquidlane.Inventory{inventory}, expiresAt: now.Add(30 * time.Second),
		chainTime: now, maxFeePerGas: big.NewInt(1_000_000_000), gasPrices: prices,
		singleRouteFor: map[common.Address]bool{prime: true}})
	request := validQuoteRequest(prime, output)
	request.Amount = spec.RequestAmount
	if spec.Mode == "exact_output" {
		request.Type = quoteTypeExactOutput
	}
	response, err := s.evaluateQuote(t.Context(), request)
	if err != nil || response.AmountOut == "0" {
		t.Fatalf("fork quote=%+v err=%v", response, err)
	}
	amountIn, amountOut := integer(response.AmountIn), integer(response.AmountOut)
	order := signedForkOrder(t, s.cfg, spec, amountIn, amountOut)
	gross := liquidlane.AmountOutForRate(amountIn, price, 6, 6)
	payout := liquidlane.AmountOutAfterDiscount(gross, discount)
	physical := liquidlane.FillQuote{Inventory: inventory, AmountIn: amountIn, GrossAmountOut: gross,
		MaxAmountOut: payout, MinDiscount: discount}
	s.reader = &executionTestReader{now: now, snapshot: fillSnapshot{Direct: []liquidlane.FillQuote{physical},
		Physical: []liquidlane.FillQuote{physical}, GasPrices: prices}}
	if _, err := s.startFill(t.Context(), []liquidlane.Route{route}, order, now, now); err != nil {
		t.Fatal(err)
	}
	if len(f.txm.reqs) != 1 || s.capacity.Snapshot()[route.CapacityID].Cmp(payout) != 0 {
		t.Fatal("missing USDC reservation or submission")
	}
	bytesType, _ := abi.NewType("bytes", "", nil)
	uintType, _ := abi.NewType("uint256", "", nil)
	result, err := (abi.Arguments{{Type: uintType}, {Type: uintType}, {Type: bytesType}}).Pack(amountIn, amountOut, f.txm.reqs[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("ATOMIC_DEX_FFI_OUTPUT"), []byte(hexutil.Encode(result)), 0o600); err != nil { // #nosec G703 -- opt-in local test IPC output chosen by the Forge fixture.
		t.Fatal(err)
	}
}

func signedForkOrder(t *testing.T, cfg *Config, spec forkBridgeSpec, amountIn, amountOut *big.Int) *resolvedOrder {
	t.Helper()
	swapperKey, err := crypto.ToECDSA(crypto.Keccak256([]byte("atomic dex fork swapper 2026-11-04 v1")))
	if err != nil {
		t.Fatal(err)
	}
	cosignerKey, err := crypto.ToECDSA(common.LeftPadBytes(big.NewInt(0xc051).Bytes(), 32))
	if err != nil {
		t.Fatal(err)
	}
	order := v2Order{Info: v2OrderInfo{Reactor: cfg.Reactor, Swapper: crypto.PubkeyToAddress(swapperKey.PublicKey),
		Nonce: new(big.Int).SetBytes(crypto.Keccak256([]byte("atomic dex prototype nonce"))), Deadline: big.NewInt(int64(spec.BlockTime) + 120)},
		Cosigner:    crypto.PubkeyToAddress(cosignerKey.PublicKey),
		BaseInput:   v2Input{Token: cfg.Dex.InputToken, StartAmount: amountIn, EndAmount: amountIn},
		BaseOutputs: []v2Output{{Token: common.HexToAddress(spec.Output), StartAmount: amountOut, EndAmount: amountOut, Recipient: common.HexToAddress(spec.Recipient)}},
		CosignerData: v2CosignerData{DecayStartTime: big.NewInt(int64(spec.BlockTime) + 30), DecayEndTime: big.NewInt(int64(spec.BlockTime) + 90),
			ExclusiveFiller: cfg.Executor, ExclusivityOverrideBps: new(big.Int), InputOverride: new(big.Int), OutputOverrides: []*big.Int{new(big.Int)}}}
	hash, err := v2OrderHash(order)
	if err != nil {
		t.Fatal(err)
	}
	cosignerData, err := v2CosignerDataArguments.Pack(order.CosignerData)
	if err != nil {
		t.Fatal(err)
	}
	order.Cosignature, err = crypto.Sign(crypto.Keccak256(hash.Bytes(), cosignerData), cosignerKey)
	if err != nil {
		t.Fatal(err)
	}
	order.Cosignature[64] += 27
	uintType, _ := abi.NewType("uint256", "", nil)
	addressType, _ := abi.NewType("address", "", nil)
	hashType, _ := abi.NewType("bytes32", "", nil)
	tokenType := crypto.Keccak256Hash([]byte("TokenPermissions(address token,uint256 amount)"))
	tokenData, _ := (abi.Arguments{{Type: hashType}, {Type: addressType}, {Type: uintType}}).Pack(tokenType, cfg.Dex.InputToken, amountIn)
	permitType := crypto.Keccak256Hash([]byte("PermitWitnessTransferFrom(TokenPermissions permitted,address spender,uint256 nonce,uint256 deadline," +
		"V2DutchOrder witness)" + v2OutputType + v2OrderInfoType + "TokenPermissions(address token,uint256 amount)" +
		"V2DutchOrder(OrderInfo info,address cosigner,address baseInputToken,uint256 baseInputStartAmount,uint256 baseInputEndAmount,DutchOutput[] baseOutputs)"))
	permitData, err := (abi.Arguments{{Type: hashType}, {Type: hashType}, {Type: addressType}, {Type: uintType}, {Type: uintType}, {Type: hashType}}).
		Pack(permitType, crypto.Keccak256Hash(tokenData), cfg.Reactor, order.Info.Nonce, order.Info.Deadline, hash)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(crypto.Keccak256([]byte{0x19, 0x01}, common.HexToHash(spec.PermitDomain).Bytes(), crypto.Keccak256(permitData)), swapperKey)
	if err != nil {
		t.Fatal(err)
	}
	sig[64] += 27
	encoded, err := v2OrderArguments.Pack(order)
	if err != nil {
		t.Fatal(err)
	}
	return &resolvedOrder{Encoded: encoded, Signature: sig, Hash: hash, QuoteID: "local-fork-e2e", Executor: cfg.Executor,
		Source: orderSourceExclusiveV2, TokenIn: cfg.Dex.InputToken, TokenOut: common.HexToAddress(spec.Output),
		AmountIn: amountIn, AmountOut: amountOut, Deadline: uint32(spec.BlockTime + 120), ExclusiveUntil: spec.BlockTime + 30}
}
