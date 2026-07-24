package lifi

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/executor"
	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

type fakeLifiReader struct {
	orderID          common.Hash
	orderIDFn        func(inputsettler.StandardOrder) common.Hash
	status           uint8
	statusErr        error
	latestBlock      uint64
	latestBlockErr   error
	fill             []liquidlane.FillQuote
	fillSet          *fillSnapshotSet
	fillSetFn        func() fillSnapshotSet
	fillSnapshotsFn  func() []liquidlane.FillQuote
	routes           []route
	executorErr      error
	directAuthErr    error
	governanceFeeErr error
}

func (f fakeLifiReader) resolveRoutes(context.Context, []common.Address) ([]route, error) {
	return f.routes, nil
}

func (f fakeLifiReader) validateExecutor(
	context.Context, common.Address, common.Address, common.Address, common.Address,
) error {
	return f.executorErr
}

func (f fakeLifiReader) validateZeroGovernanceFee(context.Context, common.Address) error {
	return f.governanceFeeErr
}

func (f fakeLifiReader) validateDirectAuthorization(context.Context, common.Address, []route) error {
	return f.directAuthErr
}

func (f fakeLifiReader) validateGasTokens([]route) error { return nil }

func (f fakeLifiReader) quoteSnapshots(context.Context, []route, common.Address, time.Time) (quoteSnapshotSet, error) {
	return quoteSnapshotSet{}, nil
}

func (f fakeLifiReader) fillSnapshots(
	context.Context, []route, common.Address, common.Address, *big.Int, time.Time,
) (fillSnapshotSet, error) {
	if f.fillSetFn != nil {
		return withFakeGasPrices(f.fillSetFn()), nil
	}
	if f.fillSet != nil {
		return withFakeGasPrices(*f.fillSet), nil
	}
	if f.fillSnapshotsFn != nil {
		fill := f.fillSnapshotsFn()
		return withFakeGasPrices(fillSnapshotSet{Direct: fill, Physical: fill}), nil
	}
	return withFakeGasPrices(fillSnapshotSet{Direct: f.fill, Physical: f.fill}), nil
}

func withFakeGasPrices(set fillSnapshotSet) fillSnapshotSet {
	rates := make(map[common.Address]*big.Int)
	for _, quote := range append(append([]liquidlane.FillQuote(nil), set.Direct...), set.Physical...) {
		rates[quote.TokenOut] = big.NewInt(1)
	}
	set.GasPrices = liquidlanegas.NewPriceSnapshot(rates)
	return set
}

func (f fakeLifiReader) orderIdentifier(
	_ context.Context,
	_ common.Address,
	order inputsettler.StandardOrder,
) (common.Hash, error) {
	if f.orderIDFn != nil {
		return f.orderIDFn(order), nil
	}
	return f.orderID, nil
}

func (f fakeLifiReader) orderStatus(context.Context, common.Address, common.Hash) (uint8, error) {
	return f.status, f.statusErr
}

func (f fakeLifiReader) latestBlockNumber(context.Context) (uint64, error) {
	return f.latestBlock, f.latestBlockErr
}

func (f fakeLifiReader) latestBlockTime(context.Context) (time.Time, error) {
	return time.Unix(1_700_000_000, 0), nil
}

func TestBuildFillCalldata(t *testing.T) {
	cfg := testLifiConfig()
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	submitted, err := parseSubmittedOrder(testOrderJSON(t, cfg, tokenIn, tokenOut), cfg, 11155111)
	if err != nil {
		t.Fatalf("parseSubmittedOrder: %v", err)
	}
	orderID := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	plan := &types.FillPlan{
		Routes: []types.FillRoute{{
			RouteID: "route-1", CapacityID: "capacity-1",
			Adapter:  common.HexToAddress("0x9999999999999999999999999999999999999999"),
			AmountIn: submitted.AmountIn, ExpectedAmountOut: big.NewInt(1_000_000),
			MinAmountOut: big.NewInt(990_000),
		}},
	}

	calldata, err := buildFillCalldata(*submitted, orderID, plan, nil)
	if err != nil {
		t.Fatalf("buildFillCalldata: %v", err)
	}
	if calldata.OrderID != orderID {
		t.Fatalf("order id = %s", calldata.OrderID)
	}

	executorABI, err := executor.LiquidLaneLifiExecutorMetaData.ParseABI()
	if err != nil {
		t.Fatalf("parse executor ABI: %v", err)
	}
	method := executorABI.Methods["finaliseWithCurrentTimestamp"]
	if !bytes.Equal(calldata.Finalise[:4], method.ID) {
		t.Fatalf("finalise selector = %s, want %s", hexutil.Encode(calldata.Finalise[:4]), hexutil.Encode(method.ID))
	}
	args, err := method.Inputs.Unpack(calldata.Finalise[4:])
	if err != nil {
		t.Fatalf("unpack finaliseWithCurrentTimestamp: %v", err)
	}
	if len(args) != 2 {
		t.Fatalf("finalise arguments = %d, want order and routes", len(args))
	}
	encodedOrder := *abi.ConvertType(
		args[0], new(executor.IInputSettlerStandardOrder),
	).(*executor.IInputSettlerStandardOrder)
	if encodedOrder.User != submitted.Order.User || encodedOrder.Nonce.Cmp(submitted.Order.Nonce) != 0 ||
		len(encodedOrder.Outputs) != 1 || encodedOrder.Outputs[0].Amount.Cmp(submitted.OutputAmount) != 0 {
		t.Fatalf("encoded order = %+v", encodedOrder)
	}
	routes := *abi.ConvertType(
		args[1], new([]executor.ILiquidLaneLifiExecutorFillRoute),
	).(*[]executor.ILiquidLaneLifiExecutorFillRoute)
	if len(routes) != 1 || routes[0].Adapter != plan.Routes[0].Adapter ||
		routes[0].AmountIn.Cmp(plan.Routes[0].AmountIn) != 0 ||
		routes[0].AmountOut.Cmp(plan.Routes[0].ExpectedAmountOut) != 0 {
		t.Fatalf("fill routes = %+v", routes)
	}
}

func TestExecutorRoutesRejectsInputMismatch(t *testing.T) {
	order := submittedOrder{AmountIn: big.NewInt(100)}
	plan := &types.FillPlan{Routes: []types.FillRoute{{
		Adapter:           common.HexToAddress("0x9999999999999999999999999999999999999999"),
		AmountIn:          big.NewInt(99),
		ExpectedAmountOut: big.NewInt(90),
		MinAmountOut:      big.NewInt(80),
	}}}

	_, err := executorRoutes(order, plan, nil)
	if err == nil || !strings.Contains(err.Error(), "input sum 99 does not match order input 100") {
		t.Fatalf("executorRoutes() error = %v", err)
	}
}

func TestExecutorRoutesIncludesResolvedPrivateDiscount(t *testing.T) {
	cfg := testLifiConfig()
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	adapter := common.HexToAddress("0x9999999999999999999999999999999999999999")
	submitted, err := parseSubmittedOrder(testOrderJSON(t, cfg, tokenIn, tokenOut), cfg, 11155111)
	if err != nil {
		t.Fatalf("parseSubmittedOrder: %v", err)
	}
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	plan := &types.FillPlan{Routes: []types.FillRoute{{
		RouteID: "route-1", Adapter: adapter, AmountIn: submitted.AmountIn,
		ExpectedAmountOut: big.NewInt(900_000), MinAmountOut: big.NewInt(850_000), DiscountID: &discountID,
	}}}
	resolved := &discounts.Signed{
		DiscountID: discountID, Adapter: adapter,
		Terms: discounts.SignedTerms{
			TokenToRedeem: tokenIn, Discount: big.NewInt(100_000),
			Signer:   common.HexToAddress("0x1111111111111111111111111111111111111111"),
			Protocol: common.HexToAddress("0x2222222222222222222222222222222222222222"),
			Nonce:    big.NewInt(7), Deadline: big.NewInt(1_900_000_000),
		},
		SignerSignature: []byte{0x12, 0x34}, ProtocolDeadline: big.NewInt(1_900_000_001),
		ProtocolSignature: []byte{0x56, 0x78},
	}

	routes, err := executorRoutes(
		*submitted, plan,
		map[common.Hash]*discounts.Signed{discountID: resolved},
	)
	if err != nil {
		t.Fatalf("executorRoutes: %v", err)
	}
	discount := routes[0].Discount
	if common.Hash(discount.DiscountId) != discountID ||
		discount.DiscountSwap.Discount.TokenToRedeem != tokenIn ||
		discount.DiscountSwap.Discount.Discount.Cmp(big.NewInt(100_000)) != 0 ||
		!bytes.Equal(discount.ProtocolSignature, resolved.ProtocolSignature) {
		t.Fatalf("encoded discount = %+v", discount)
	}
}
