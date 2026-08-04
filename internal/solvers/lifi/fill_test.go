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

func unpackFinaliseCalldata(t *testing.T, calldata []byte) (
	executor.IInputSettlerStandardOrder,
	[]executor.ILiquidLaneLifiExecutorFillRoute,
	[]executor.ILiquidLaneLifiExecutorDiscountRoute,
) {
	t.Helper()
	if len(calldata) < 4 {
		t.Fatalf("finalise calldata length = %d, want at least 4", len(calldata))
	}
	if got := hexutil.Encode(calldata[:4]); got != "0xd24f1d03" {
		t.Fatalf("finalise selector = %s, want 0xd24f1d03", got)
	}
	executorABI, err := executor.LiquidLaneLifiExecutorMetaData.ParseABI()
	if err != nil {
		t.Fatalf("parse executor ABI: %v", err)
	}
	method := executorABI.Methods["finaliseWithCurrentTimestamp"]
	args, err := method.Inputs.Unpack(calldata[4:])
	if err != nil {
		t.Fatalf("unpack finaliseWithCurrentTimestamp: %v", err)
	}
	if len(args) != 3 {
		t.Fatalf("finalise arguments = %d, want order, routes, and discountRoutes", len(args))
	}
	order := *abi.ConvertType(
		args[0], new(executor.IInputSettlerStandardOrder),
	).(*executor.IInputSettlerStandardOrder)
	directRoutes := *abi.ConvertType(
		args[1], new([]executor.ILiquidLaneLifiExecutorFillRoute),
	).(*[]executor.ILiquidLaneLifiExecutorFillRoute)
	discountRoutes := *abi.ConvertType(
		args[2], new([]executor.ILiquidLaneLifiExecutorDiscountRoute),
	).(*[]executor.ILiquidLaneLifiExecutorDiscountRoute)
	return order, directRoutes, discountRoutes
}

func TestBuildFillCalldata(t *testing.T) {
	cfg := testLifiConfig()
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	submitted := testSubmittedOrder(t, cfg, tokenIn, tokenOut)
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

	encodedOrder, directRoutes, discountRoutes := unpackFinaliseCalldata(t, calldata.Finalise)
	if encodedOrder.User != submitted.Order.User || encodedOrder.Nonce.Cmp(submitted.Order.Nonce) != 0 ||
		len(encodedOrder.Outputs) != 1 || encodedOrder.Outputs[0].Amount.Cmp(submitted.OutputAmount) != 0 {
		t.Fatalf("encoded order = %+v", encodedOrder)
	}
	if len(directRoutes) != 1 || directRoutes[0].Adapter != plan.Routes[0].Adapter ||
		directRoutes[0].AmountIn.Cmp(plan.Routes[0].AmountIn) != 0 ||
		directRoutes[0].AmountOut.Cmp(plan.Routes[0].ExpectedAmountOut) != 0 {
		t.Fatalf("direct routes = %+v", directRoutes)
	}
	if len(discountRoutes) != 0 {
		t.Fatalf("discount routes = %+v, want empty", discountRoutes)
	}
}

func TestBuildExecutorRoutesRejectsInputMismatch(t *testing.T) {
	order := submittedOrder{AmountIn: big.NewInt(100)}
	plan := &types.FillPlan{Routes: []types.FillRoute{{
		Adapter:           common.HexToAddress("0x9999999999999999999999999999999999999999"),
		AmountIn:          big.NewInt(99),
		ExpectedAmountOut: big.NewInt(90),
		MinAmountOut:      big.NewInt(80),
	}}}

	_, _, err := buildExecutorRoutes(order, plan, nil)
	if err == nil || !strings.Contains(err.Error(), "input sum 99 does not match order input 100") {
		t.Fatalf("buildExecutorRoutes() error = %v", err)
	}
}

func TestBuildFillCalldataSplitsDirectAndResolvedPrivateDiscount(t *testing.T) {
	cfg := testLifiConfig()
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	directAdapter := common.HexToAddress("0x9999999999999999999999999999999999999999")
	discountAdapter := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	submitted := testSubmittedOrder(t, cfg, tokenIn, tokenOut)
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	directAmountIn := big.NewInt(400_000)
	discountAmountIn := new(big.Int).Sub(submitted.AmountIn, directAmountIn)
	plan := &types.FillPlan{Routes: []types.FillRoute{
		{
			RouteID: "route-direct", Adapter: directAdapter, AmountIn: directAmountIn,
			ExpectedAmountOut: big.NewInt(390_000), MinAmountOut: big.NewInt(380_000),
		},
		{
			RouteID: "route-discount", Adapter: discountAdapter, AmountIn: discountAmountIn,
			ExpectedAmountOut: big.NewInt(600_000), MinAmountOut: big.NewInt(590_000), DiscountID: &discountID,
		},
	}}
	resolved := &discounts.Signed{
		DiscountID: discountID, Adapter: discountAdapter,
		Terms: discounts.SignedTerms{
			TokenToRedeem: tokenIn, Discount: big.NewInt(100_000),
			Signer:   common.HexToAddress("0x1111111111111111111111111111111111111111"),
			Protocol: common.HexToAddress("0x2222222222222222222222222222222222222222"),
			Nonce:    big.NewInt(7), Deadline: big.NewInt(1_900_000_000),
		},
		SignerSignature: []byte{0x12, 0x34}, ProtocolDeadline: big.NewInt(1_799_999_999),
		ProtocolSignature: []byte{0x56, 0x78},
	}

	calldata, err := buildFillCalldata(
		*submitted,
		common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		plan,
		map[common.Hash]*discounts.Signed{discountID: resolved},
	)
	if err != nil {
		t.Fatalf("buildFillCalldata: %v", err)
	}
	if want := time.Unix(1_799_999_999, 0); !calldata.CancelAt.Equal(want) {
		t.Fatalf("CancelAt = %v, want selected discount deadline %v", calldata.CancelAt, want)
	}
	_, directRoutes, discountRoutes := unpackFinaliseCalldata(t, calldata.Finalise)
	if len(directRoutes) != 1 || directRoutes[0].Adapter != directAdapter ||
		directRoutes[0].AmountIn.Cmp(directAmountIn) != 0 ||
		directRoutes[0].AmountOut.Cmp(plan.Routes[0].ExpectedAmountOut) != 0 {
		t.Fatalf("direct routes = %+v", directRoutes)
	}
	if len(discountRoutes) != 1 {
		t.Fatalf("discount routes = %+v", discountRoutes)
	}
	discountRoute := discountRoutes[0]
	discount := discountRoute.DiscountSwap.Discount
	if discountRoute.Adapter != discountAdapter ||
		discountRoute.AmountIn.Cmp(discountAmountIn) != 0 ||
		discount.TokenToRedeem != resolved.Terms.TokenToRedeem ||
		discount.Discount.Cmp(resolved.Terms.Discount) != 0 ||
		discount.Signer != resolved.Terms.Signer ||
		discount.Protocol != resolved.Terms.Protocol ||
		discount.Nonce.Cmp(resolved.Terms.Nonce) != 0 ||
		discount.Deadline.Cmp(resolved.Terms.Deadline) != 0 ||
		!bytes.Equal(discountRoute.DiscountSwap.SignerSignature, resolved.SignerSignature) ||
		discountRoute.DiscountSwap.ProtocolDeadline.Cmp(resolved.ProtocolDeadline) != 0 ||
		!bytes.Equal(discountRoute.ProtocolSignature, resolved.ProtocolSignature) {
		t.Fatalf("discount route = %+v", discountRoute)
	}
}
